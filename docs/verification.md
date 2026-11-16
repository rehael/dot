# dot – verification record

Date: 2026-11-16. Platform: Windows 11, Go 1.27.4, git 2.56.0. Commit under test: see the first lines of the gate output.

This file records the verification of the first complete version: the commands and their raw output. "Verified" means that the command ran on the real path and gave the output shown.

Not verified: no test ran on Linux or macOS. For Linux, only the build and `go vet` ran.

## Gate commands

`gate-test-v.log` is the verbose test log. It is not in the repository; the counts come from it.

```text
$ git log --oneline -1
d6f1581 test(e2e): name PATH-5 in scenario E1
$ go version
go version go1.27.4 windows/amd64
$ git --version
git version 2.56.0.windows.4
$ gofmt -l .
[exit 0]
$ go vet ./...
[exit 0]
$ GOOS=linux GOARCH=amd64 go build ./...
[exit 0]
$ GOOS=linux GOARCH=amd64 go vet ./...
[exit 0]
$ go test -count=1 -v ./... > gate-test-v.log
[exit 0]
$ grep -c "^--- PASS" gate-test-v.log
196
$ grep -c -- "--- PASS" gate-test-v.log
294
$ grep -c -- "--- FAIL" gate-test-v.log
0
$ grep -c -- "--- SKIP" gate-test-v.log
0
$ grep -E "^(ok|FAIL|\?)" gate-test-v.log
?   	dot	[no test files]
ok  	dot/e2e	123.670s
ok  	dot/internal/cli	76.995s
ok  	dot/internal/core	0.822s
ok  	dot/internal/gitx	5.310s
ok  	dot/internal/kv	0.121s
ok  	dot/internal/tmpl	0.153s
$ go build -o dot.exe . && ./dot.exe help
usage: dot <command> [arguments]

commands:
  init [path]              create or adopt the repository
  register [name]          register this machine
  add <file>               track a live file, or capture its edit
  edit <file>              open the repository copy in the editor
  var [name [value]]       show or set a variable; no argument: edit vars
  check                    report undefined roots and placeholder names
  meta [file]              show configuration, variables and file status
  diff [file]              show differences, rendered repository copy to live file
  apply [file] [--force]   write live files from the repository
  push [-m <message>]      capture live edits, commit, git push
  pull                     capture live edits, commit, git pull
  help                     show this text
  version                  show the version
[exit 0]
$ grep -c require go.mod
0
$ git status --porcelain
[exit 0]
```

## Live run of the built executable

The run uses a byte-identical copy of the `dot.exe` of the gate above. It uses two local directories ("machine A" and "machine B"), one bare repository between them, and an isolated git configuration with `pull.rebase = false`. `<W>` is the temporary work directory. `<home>` is the home directory of the user; `meta` shows the three built-in variables from the real environment.

Exit codes that are not 0 are expected at these steps:

- `check` and `pull` before `register` – code 1.
- `check` on machine B before its variables exist – code 1.
- `push` with a capture failure – code 1.
- `apply` on a `both-changed` file – code 1.
- An unknown command – code 2.

Script:

```bash
#!/usr/bin/env bash
# Live run of the built dot.exe. Usage: live-run.sh <path to dot.exe> <work directory>
set -u
DOT="$1"
W="$2"
mkdir -p "$W"
W="$(cd "$W" && pwd)"
win() { cygpath -w "$1"; }

# Stands for the git configuration of the user: git needs a pull strategy for divergent branches.
printf '[pull]
	rebase = false
' > "$W/gitconfig"
export GIT_CONFIG_GLOBAL="$(win "$W/gitconfig")"
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0
export GIT_AUTHOR_NAME="Test User" GIT_AUTHOR_EMAIL="test@example.invalid"
export GIT_COMMITTER_NAME="Test User" GIT_COMMITTER_EMAIL="test@example.invalid"
unset XDG_CONFIG_HOME VISUAL

mkdir -p "$W/a/home" "$W/b/elsewhere/cfg"
A_DIR="$(win "$W/a/dot")"
B_DIR="$(win "$W/b/dot")"
A_HOME="$(win "$W/a/home")"
B_HOME="$(win "$W/b/elsewhere/cfg")"
BARE="$(win "$W/remote.git")"

# dot <machine dir> <args...>: prints the command, runs it, prints the exit code.
dot() {
  local dir="$1"; shift
  echo "\$ DOT_DIR=$dir dot $*"
  DOT_DIR="$dir" "$DOT" "$@"
  echo "[exit $?]"
}
sh_() { echo "\$ $*"; "$@"; echo "[exit $?]"; }

echo "===== machine A: init, register, var, add ====="
dot "$A_DIR" version
dot "$A_DIR" check
dot "$A_DIR" init
dot "$A_DIR" register alpha
dot "$A_DIR" var TOOL "$A_HOME"
dot "$A_DIR" var user alice
dot "$A_DIR" var TOOL
printf 'name = alice\ncolor = blue\npath = /srv/alice/data\n' > "$W/a/home/tool.conf"
dot "$A_DIR" add "$(win "$W/a/home/tool.conf")"

echo "===== machine A: edit (EDITOR exits at once), then put placeholders in the repository copy ====="
export EDITOR="cmd /c rem"
dot "$A_DIR" edit tool.conf
REPO_COPY="$W/a/dot/repo/files/TOOL/tool.conf"
printf 'name = {{user}}\ncolor = blue\npath = /srv/{{user}}/data\n' > "$REPO_COPY"
sh_ cat "$REPO_COPY"
dot "$A_DIR" check
dot "$A_DIR" meta
dot "$A_DIR" meta tool.conf
dot "$A_DIR" diff

echo "===== machine A: push without a remote, then with a bare remote ====="
dot "$A_DIR" push
sh_ git init --bare -q "$BARE"
sh_ git -C "$(win "$W/a/dot/repo")" remote add origin "$BARE"
dot "$A_DIR" push

echo "===== machine B: clone, init (adopt), register, vars, pull, apply ====="
sh_ git clone -q "$BARE" "$(win "$W/b/dot/repo")"
dot "$B_DIR" pull
dot "$B_DIR" init
dot "$B_DIR" register beta
dot "$B_DIR" check
export EDITOR="cmd /c rem"
dot "$B_DIR" var
sh_ cat "$W/b/dot/vars"
dot "$B_DIR" var TOOL "$B_HOME"
dot "$B_DIR" var user bob
dot "$B_DIR" check
dot "$B_DIR" diff
dot "$B_DIR" apply
sh_ cat "$W/b/elsewhere/cfg/tool.conf"
dot "$B_DIR" push -m "beta: register"

echo "===== machine A: live edit, diff, push; machine B: pull, diff, apply ====="
printf 'name = alice\ncolor = green\npath = /srv/alice/data\nextra = 1\n' > "$W/a/home/tool.conf"
dot "$A_DIR" pull
dot "$A_DIR" diff
dot "$A_DIR" push
sh_ cat "$REPO_COPY"
dot "$B_DIR" pull
dot "$B_DIR" diff
dot "$B_DIR" apply
sh_ cat "$W/b/elsewhere/cfg/tool.conf"
dot "$B_DIR" meta

echo "===== machine A: edit that changes a value text (dropped placeholder), then a capture failure ====="
printf 'name = alicia
color = green
path = /srv/alice/data
extra = 1
' > "$W/a/home/tool.conf"
dot "$A_DIR" diff
dot "$A_DIR" push
sh_ cat "$REPO_COPY"
printf 'name = alicia
color = green
path = /srv/alice/data
extra = {{user}}
' > "$W/a/home/tool.conf"
dot "$A_DIR" push
sh_ git -C "$(win "$W/a/dot/repo")" status --short
echo "===== refusals ====="
printf 'local edit on B\n' > "$W/b/elsewhere/cfg/tool.conf"
printf 'name = {{user}}\ncolor = red\n' > "$W/b/dot/repo/files/TOOL/tool.conf"
dot "$B_DIR" apply
dot "$B_DIR" apply --force
sh_ cat "$W/b/elsewhere/cfg/tool.conf"
dot "$B_DIR" frobnicate
echo "===== state files ====="
sh_ cat "$W/a/dot/config" "$W/a/dot/vars" "$W/a/dot/state" "$W/a/dot/repo/dot.conf" "$W/a/dot/repo/.gitattributes"
sh_ git -C "$(win "$W/a/dot/repo")" log --oneline
```

Output:

```text
===== machine A: init, register, var, add =====
$ DOT_DIR=<W>\a\dot dot version
dot 0.1.0
[exit 0]
$ DOT_DIR=<W>\a\dot dot check
dot: machine not registered; run: dot register
[exit 1]
$ DOT_DIR=<W>\a\dot dot init
created repository <W>\a\dot\repo
[exit 0]
$ DOT_DIR=<W>\a\dot dot register alpha
registered machine alpha
[exit 0]
$ DOT_DIR=<W>\a\dot dot var TOOL <W>\a\home
[exit 0]
$ DOT_DIR=<W>\a\dot dot var user alice
[exit 0]
$ DOT_DIR=<W>\a\dot dot var TOOL
TOOL = <W>\a\home
origin: vars
[exit 0]
$ DOT_DIR=<W>\a\dot dot add <W>\a\home\tool.conf
added TOOL/tool.conf
[exit 0]
===== machine A: edit (EDITOR exits at once), then put placeholders in the repository copy =====
$ DOT_DIR=<W>\a\dot dot edit tool.conf
[exit 0]
$ cat <W>/a/dot/repo/files/TOOL/tool.conf
name = {{user}}
color = blue
path = /srv/{{user}}/data
[exit 0]
$ DOT_DIR=<W>\a\dot dot check
[exit 0]
$ DOT_DIR=<W>\a\dot dot meta
machine: alpha
local directory: <W>\a\dot
repository: <W>\a\dot\repo
delimiters: {{ }}
registered machines: alpha
variables:
  APPDATA = <home>\AppData\Roaming (environment)
  LOCALAPPDATA = <home>\AppData\Local (environment)
  TOOL = <W>\a\home (vars)
  USERPROFILE = <home> (environment)
  user = alice (vars)
files:
  in-sync TOOL/tool.conf <W>\a\home\tool.conf
[exit 0]
$ DOT_DIR=<W>\a\dot dot meta tool.conf
name: TOOL/tool.conf
repository path: <W>\a\dot\repo\files\TOOL\tool.conf
live path: <W>\a\home\tool.conf
status: in-sync
placeholder: user = alice (vars)
[exit 0]
$ DOT_DIR=<W>\a\dot dot diff
[exit 0]
===== machine A: push without a remote, then with a bare remote =====
$ DOT_DIR=<W>\a\dot dot push
[master (root-commit) 8f2d820] alpha: sync
 3 files changed, 7 insertions(+)
 create mode 100644 .gitattributes
 create mode 100644 dot.conf
 create mode 100644 files/TOOL/tool.conf
no remote; nothing pushed
[exit 0]
$ git init --bare -q <W>\remote.git
[exit 0]
$ git -C <W>\a\dot\repo remote add origin <W>\remote.git
[exit 0]
$ DOT_DIR=<W>\a\dot dot push
To <W>\remote.git
 * [new branch]      HEAD -> master
branch 'master' set up to track 'origin/master'.
[exit 0]
===== machine B: clone, init (adopt), register, vars, pull, apply =====
$ git clone -q <W>\remote.git <W>\b\dot\repo
[exit 0]
$ DOT_DIR=<W>\b\dot dot pull
dot: machine not registered; run: dot register
[exit 1]
$ DOT_DIR=<W>\b\dot dot init
adopted repository <W>\b\dot\repo
[exit 0]
$ DOT_DIR=<W>\b\dot dot register beta
registered machine beta
[exit 0]
$ DOT_DIR=<W>\b\dot dot check
TOOL/tool.conf: root TOOL is undefined
TOOL/tool.conf: placeholder user is undefined
[exit 1]
$ DOT_DIR=<W>\b\dot dot var
[exit 0]
$ cat <W>/b/dot/vars
# used by: TOOL/tool.conf
# TOOL =
# used by: TOOL/tool.conf
# user =
[exit 0]
$ DOT_DIR=<W>\b\dot dot var TOOL <W>\b\elsewhere\cfg
[exit 0]
$ DOT_DIR=<W>\b\dot dot var user bob
[exit 0]
$ DOT_DIR=<W>\b\dot dot check
[exit 0]
$ DOT_DIR=<W>\b\dot dot diff
missing TOOL/tool.conf
[exit 0]
$ DOT_DIR=<W>\b\dot dot apply
wrote TOOL/tool.conf
[exit 0]
$ cat <W>/b/elsewhere/cfg/tool.conf
name = bob
color = blue
path = /srv/bob/data
[exit 0]
$ DOT_DIR=<W>\b\dot dot push -m beta: register
[master 340b1ec] beta: register
 1 file changed, 1 insertion(+)
To <W>\remote.git
   8f2d820..340b1ec  master -> master
[exit 0]
===== machine A: live edit, diff, push; machine B: pull, diff, apply =====
$ DOT_DIR=<W>\a\dot dot pull
captured TOOL/tool.conf
[master 2a1f6bb] alpha: sync
 1 file changed, 2 insertions(+), 1 deletion(-)
From <W>\remote
   8f2d820..340b1ec  master     -> origin/master
Merge made by the 'ort' strategy.
 dot.conf | 1 +
 1 file changed, 1 insertion(+)
0 file(s) repo-changed
[exit 0]
$ DOT_DIR=<W>\a\dot dot diff
[exit 0]
$ DOT_DIR=<W>\a\dot dot push
To <W>\remote.git
   340b1ec..b4eb0b3  master -> master
[exit 0]
$ cat <W>/a/dot/repo/files/TOOL/tool.conf
name = {{user}}
color = green
path = /srv/{{user}}/data
extra = 1
[exit 0]
$ DOT_DIR=<W>\b\dot dot pull
From <W>\remote
   340b1ec..b4eb0b3  master     -> origin/master
Updating 340b1ec..b4eb0b3
Fast-forward
 files/TOOL/tool.conf | 3 ++-
 1 file changed, 2 insertions(+), 1 deletion(-)
1 file(s) repo-changed; run: dot apply
[exit 0]
$ DOT_DIR=<W>\b\dot dot diff
repo-changed TOOL/tool.conf
diff --git repository/TOOL/tool.conf live/TOOL/tool.conf
index a24d605..f9275c6 100644
--- repository/TOOL/tool.conf
+++ live/TOOL/tool.conf
@@ -1,4 +1,3 @@
 name = bob
-color = green
+color = blue
 path = /srv/bob/data
-extra = 1
[exit 0]
$ DOT_DIR=<W>\b\dot dot apply
wrote TOOL/tool.conf
[exit 0]
$ cat <W>/b/elsewhere/cfg/tool.conf
name = bob
color = green
path = /srv/bob/data
extra = 1
[exit 0]
$ DOT_DIR=<W>\b\dot dot meta
machine: beta
local directory: <W>\b\dot
repository: <W>\b\dot\repo
delimiters: {{ }}
registered machines: alpha, beta
variables:
  APPDATA = <home>\AppData\Roaming (environment)
  LOCALAPPDATA = <home>\AppData\Local (environment)
  TOOL = <W>\b\elsewhere\cfg (vars)
  USERPROFILE = <home> (environment)
  user = bob (vars)
files:
  in-sync TOOL/tool.conf <W>\b\elsewhere\cfg\tool.conf
[exit 0]
===== machine A: edit that changes a value text (dropped placeholder), then a capture failure =====
$ DOT_DIR=<W>\a\dot dot diff
live-changed TOOL/tool.conf
diff --git repository/TOOL/tool.conf live/TOOL/tool.conf
index b643298..0bf339d 100644
--- repository/TOOL/tool.conf
+++ live/TOOL/tool.conf
@@ -1,4 +1,4 @@
-name = alice
+name = alicia
 color = green
 path = /srv/alice/data
 extra = 1
dot: warning: TOOL/tool.conf: line 1: placeholder {{user}} dropped
[exit 0]
$ DOT_DIR=<W>\a\dot dot push
dot: warning: TOOL/tool.conf: line 1: placeholder {{user}} dropped
captured TOOL/tool.conf
[master c9c4e80] alpha: sync
 1 file changed, 1 insertion(+), 1 deletion(-)
To <W>\remote.git
   b4eb0b3..c9c4e80  master -> master
[exit 0]
$ cat <W>/a/dot/repo/files/TOOL/tool.conf
name = alicia
color = green
path = /srv/{{user}}/data
extra = 1
[exit 0]
$ DOT_DIR=<W>\a\dot dot push
dot: TOOL/tool.conf: capture failed: capture result does not render to the live content; edit the repository copy by hand; fix: dot edit TOOL/tool.conf, change the repository copy until its render is equal to the live file
[exit 1]
$ git -C <W>\a\dot\repo status --short
[exit 0]
===== refusals =====
$ DOT_DIR=<W>\b\dot dot apply
dot: TOOL/tool.conf: both-changed; refused; see: dot diff TOOL/tool.conf; then: dot add <live file> (live wins) or dot apply --force TOOL/tool.conf (repository wins)
[exit 1]
$ DOT_DIR=<W>\b\dot dot apply --force
wrote TOOL/tool.conf
[exit 0]
$ cat <W>/b/elsewhere/cfg/tool.conf
name = bob
color = red
[exit 0]
$ DOT_DIR=<W>\b\dot dot frobnicate
dot: unknown command: frobnicate
usage: dot <command> [arguments]

commands:
  init [path]              create or adopt the repository
  register [name]          register this machine
  add <file>               track a live file, or capture its edit
  edit <file>              open the repository copy in the editor
  var [name [value]]       show or set a variable; no argument: edit vars
  check                    report undefined roots and placeholder names
  meta [file]              show configuration, variables and file status
  diff [file]              show differences, rendered repository copy to live file
  apply [file] [--force]   write live files from the repository
  push [-m <message>]      capture live edits, commit, git push
  pull                     capture live edits, commit, git pull
  help                     show this text
  version                  show the version
[exit 2]
===== state files =====
$ cat <W>/a/dot/config <W>/a/dot/vars <W>/a/dot/state <W>/a/dot/repo/dot.conf <W>/a/dot/repo/.gitattributes
machine = alpha
TOOL = <W>\a\home
user = alice
d13910a095596d4094956eba2729f682419841551919647d24285454eaa1a543 TOOL/tool.conf
prefix = {{
suffix = }}
machine = alpha
machine = beta
* -text
[exit 0]
$ git -C <W>\a\dot\repo log --oneline
c9c4e80 alpha: sync
b4eb0b3 Merge branch 'master' of <W>\remote
2a1f6bb alpha: sync
340b1ec beta: register
8f2d820 alpha: sync
[exit 0]
```

## Additional checks during the run

- `git pull` without a pull strategy: with an empty git configuration and diverged branches, `dot pull` returned code 128 with the git message `fatal: Need to specify how to reconcile divergent branches.` `README.md` gives the pull strategy as a required step.
- `core.autocrlf = true`: a round trip of one CRLF file and one LF file between two machines kept the bytes equal in both live files and in the repository copies (`cmp`). Before a fix, `dot diff` did not show a difference of line ends; the regression test is `TestDiffLineEndsIgnoreAutocrlf`.
- Read-only live file on Windows: before a fix, `os.Rename` over a read-only file returned `Access is denied.`; the regression test is `TestWriteAtomic_SYNC6_ReadOnly`.
