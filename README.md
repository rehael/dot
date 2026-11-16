# dot

`dot` is a command-line tool that keeps personal configuration files ("dotfiles") in sync between several machines.

- One git repository stores the files and moves them between machines.
- A tracked file can have a different path on each machine.
- A file can contain placeholders. Each machine replaces them with its own values.
- An edit made directly in a live file goes back into the repository. The placeholders stay in the repository copy.

`dot` copies files. It does not use symbolic links. Git does all merges, on the repository copies.

## Contents

- [Install](#install)
- [Concepts](#concepts)
- [First machine](#first-machine)
- [Second machine](#second-machine)
- [Daily use](#daily-use)
- [File formats and locations](#file-formats-and-locations)
- [Variables and placeholders](#variables-and-placeholders)
- [Commands](#commands)
- [How capture works](#how-capture-works)
- [Manual fixes](#manual-fixes)
- [Limits and non-goals](#limits-and-non-goals)
- [Release procedure](#release-procedure)
- [License](#license)

## Install

Requirement: git on `PATH`. `dot` starts the `git` executable. It has no embedded git library.

Use a released executable, install with the Go tool, or build from source.

Released executable (Windows, x86-64): download `dot-<tag>-windows-amd64.zip` from the [releases page](https://github.com/rehael/dot/releases). The archive holds `dot.exe`, `LICENSE` and `README.md`. The file `SHA256SUMS` of the release has the SHA-256 hash of the archive. Extract `dot.exe` to a directory that is on `PATH`.

Install with the Go tool (all systems): Go 1.27 or later is necessary. This command builds `dot` and puts it in the Go executable directory (`go env GOBIN`, or `bin` in `go env GOPATH`), which must be on `PATH`:

```powershell
go install github.com/rehael/dot@latest
```

Build from source (all systems): Go 1.27 or later is necessary. Run this command in the source directory, then copy `dot.exe` to a directory that is on `PATH`:

```powershell
go build -o dot.exe .
```

Configure git once on each machine:

1. Set a pull strategy. `dot pull` runs plain `git pull`, and git stops when the local branch and the remote branch have diverged and no strategy is set. Merge:

   ```powershell
   git config --global pull.rebase false
   ```

   Rebase is the alternative: `git config --global pull.rebase true`. To set the strategy for the `dot` repository only, run the command without `--global` inside the repository.

2. Make sure that git can make commits: `user.name` and `user.email` must be set. `dot push` and `dot pull` make commits.

## Concepts

- **Local directory** — the directory of `dot` on one machine. Git does not track it. Default: `<home>\.config\dot`.
- **Repository** — the git work tree that holds the tracked files. Default: `<local directory>\repo`.
- **Live file** — the file that an application reads, at its real path on this machine.
- **Repository copy** — the tracked version of a file, below `files\` in the repository. It can contain placeholders.
- **Variable** — a name with a one-line text value. Values are local to a machine. The repository holds no values.
- **Root** — the first component of a logical name. It is a variable name. Its value is an absolute directory path on this machine.
- **Logical name** — `<ROOT>/<relative path>`, with forward slashes, for example `VIM/.gvimrc`. It identifies a tracked file on all machines.
- **Placeholder** — `{{name}}` in a repository copy. On each machine, `dot` replaces it with the value of the variable `name`.
- **Render** — replace each placeholder with its value. This changes the repository form into the live form.
- **Capture** — move an edit of a live file into its repository copy and keep the placeholders. This changes the live form into the repository form.
- **State** — for each tracked file, the SHA-256 (Secure Hash Algorithm, 256-bit) hash of the live content at the last time when the live file and the render were equal.

Example: machine A keeps the Vim configuration in `C:\vim\.gvimrc`, machine B in `C:\tools\vim\.gvimrc`. The repository has `files/VIM/.gvimrc`. Machine A has `VIM = C:\vim`, machine B has `VIM = C:\tools\vim`.

### Status

`dot` gives each tracked file one status. It compares three hashes: the state hash, the hash of the live file, and the hash of the rendered repository copy. The first rule that applies sets the status:

1. `skipped` — a `skip` entry in `config` covers the file. `dot` does not read or write the live file. Action: none.
2. `unresolved` — the root is undefined or not an absolute path, or a placeholder name is undefined. The status carries the reasons. Action: `dot check`, then `dot var`.
3. `missing` — the live file does not exist. Action: `dot apply` creates it.
4. `in-sync` — the live file is equal to the render. Action: none.
5. `repo-changed` — the repository copy changed and the live file did not. This includes a changed variable value. Action: `dot apply`.
6. `live-changed` — the live file changed and the repository copy did not. Action: `dot push` or `dot pull` captures the edit; `dot add <live file>` captures one file.
7. `both-changed` — all other cases: both sides changed, or there is no state and the two sides differ (a new machine with a different existing file). Action: `dot diff`, then select one side (see [both-changed](#both-changed)).

## First machine

The example machine is `desk`. Its home directory is `C:\Users\me`.

1. Create the repository and register the machine:

   ```powershell
   dot init
   dot register desk
   ```

   ```text
   created repository C:\Users\me\.config\dot\repo
   registered machine desk
   ```

   `register` without a name uses the host name in lower case.

2. Define a root for each directory that holds tracked files. `USERPROFILE`, `APPDATA` and `LOCALAPPDATA` come from the environment and need no definition. Other roots need one:

   ```powershell
   dot var VIM C:\vim
   ```

3. Track the files. The argument is the live path:

   ```powershell
   dot add C:\Users\me\.gitconfig
   dot add C:\vim\.gvimrc
   ```

   ```text
   added USERPROFILE/.gitconfig
   added VIM/.gvimrc
   ```

   `dot` selects the root whose value is the longest parent directory of the file.

4. Put a placeholder into a repository copy. Define the variable, then edit the repository copy and replace the machine-specific text with the placeholder:

   ```powershell
   dot var EMAIL me@desk.example
   $env:EDITOR = 'notepad'
   dot edit .gitconfig
   ```

   The repository copy then has `email = {{EMAIL}}`. The render on `desk` is equal to the live file, so the live file does not change.

5. Add a git remote named `origin`, then publish:

   ```powershell
   git -C C:\Users\me\.config\dot\repo remote add origin https://git.example.com/me/dotfiles.git
   dot push
   ```

   The first `push` commits `.gitattributes`, `dot.conf` and the repository copies with the message `desk: sync`, then runs `git push -u origin HEAD`.

## Second machine

The example machine is `laptop`. On this machine, the Vim configuration is in `C:\tools\vim`.

1. Clone the repository into the default repository location and adopt it:

   ```powershell
   git clone https://git.example.com/me/dotfiles.git C:\Users\me\.config\dot\repo
   dot init
   ```

   ```text
   adopted repository C:\Users\me\.config\dot\repo
   ```

   A clone in a different location works too: `dot init <path>` adopts it and writes its path to `config`.

2. Register the machine:

   ```powershell
   dot register laptop
   ```

3. Fill in the variables. `dot var` without an argument writes a stub for each undefined root and placeholder name into `vars`, then opens `vars` in the editor:

   ```text
   # used by: VIM/.gvimrc
   # VIM =
   ```

   Replace each stub line with a definition, for example `VIM = C:\tools\vim`. Or set each value with a command:

   ```powershell
   dot var VIM C:\tools\vim
   dot var EMAIL me@laptop.example
   ```

4. Check that all names are defined:

   ```powershell
   dot check
   ```

   No output and exit code 0 mean that there is no problem.

5. Get the latest version, look at the differences, and write the live files:

   ```powershell
   dot pull
   dot diff
   dot apply
   ```

   ```text
   missing USERPROFILE/.gitconfig
   missing VIM/.gvimrc
   wrote USERPROFILE/.gitconfig
   wrote VIM/.gvimrc
   ```

   If a live file already exists and is different, its status is `both-changed` and `apply` refuses it. See [both-changed](#both-changed).

6. Publish the `machine = laptop` line that `register` added to `dot.conf`:

   ```powershell
   dot push
   ```

## Daily use

Before you work on a machine:

```powershell
dot pull
dot diff
dot apply
```

After you edit live files:

```powershell
dot diff
dot push
```

`dot diff` shows the rendered repository copy as the old side and the live file as the new side:

```text
live-changed USERPROFILE/.gitconfig
diff --git repository/USERPROFILE/.gitconfig live/USERPROFILE/.gitconfig
index 213c0ff..4b63d47 100644
--- repository/USERPROFILE/.gitconfig
+++ live/USERPROFILE/.gitconfig
@@ -1,3 +1,5 @@
 [user]
 	name = Me
 	email = me@desk.example
+[core]
+	autocrlf = false
```

`dot push` captures the edit, commits and pushes:

```text
captured USERPROFILE/.gitconfig
[master df5e6e7] desk: sync
 1 file changed, 2 insertions(+)
```

On the other machine, `dot pull` reports the files to write:

```text
1 file(s) repo-changed; run: dot apply
```

`dot pull` does not write live files. Run `dot apply` after it.

A change of a variable value makes each file that uses the variable `repo-changed`. Run `dot apply` to write them again.

## File formats and locations

### Location of the local directory

The first match applies:

1. The environment variable `DOT_DIR`, if it is set and not empty. A relative value is relative to the current directory.
2. `%XDG_CONFIG_HOME%\dot`, if `XDG_CONFIG_HOME` is set and is an absolute path.
3. `<home>\.config\dot`. On Windows, `<home>` is `%USERPROFILE%`.

`dot` creates the local directory and its files when it first writes to them. A file that does not exist is read as empty.

### Line format

`config`, `vars` and `dot.conf` use the same line format:

- The encoding is UTF-8. `dot` reads LF and CRLF line ends and writes LF.
- An empty line, or a line with only white space, is ignored.
- A line whose first non-space character is `#` is a comment.
- `key = value`: the key is the text before the first `=`, trimmed. The value is the text after it, trimmed.
- There are no quotes and no escapes. There are no comments at the end of a line: a `#` or `=` in a value is literal text.
- When `dot` writes a file, it changes only the applicable line. All other lines, comments included, stay byte-identical.
- When a key occurs more than once, the first line is used, except for keys that can repeat.

### `config` (local directory)

```text
machine = laptop
repo = D:\sync\dotfiles
skip = APPDATA/Code/User/settings.json
```

- `machine` — the name of this machine. `dot register` writes it.
- `repo` — optional. The absolute path of the repository. `dot init <path>` writes it when the path is not the default location.
- `skip` — zero or more. A logical-name prefix. A file is skipped if its logical name is equal to the value, or starts with the value followed by `/`. A trailing `/` in the value is ignored. Write `skip` lines with a text editor; no command writes them.

### `vars` (local directory)

One variable for each line:

```text
VIM = C:\tools\vim
EMAIL = me@laptop.example
```

A line with an empty value or a name that is not valid is ignored. A comment line `# NAME =` is a stub: `dot var NAME <value>` replaces it.

### `state` (local directory)

One line for each tracked file: 64 lowercase hexadecimal characters, one space, the logical name. `dot` writes this file. Do not edit it.

```text
021bd922bdcec6cc288ee6ebcbe613b6df6a9079f04707889cdb19e41ae7b574 USERPROFILE/.gitconfig
```

`add`, `apply`, `push` and `pull` write the state. `diff`, `meta` and `check` do not. An entry for a file that is no longer tracked is removed at the next write.

### `dot.conf` (repository)

```text
prefix = {{
suffix = }}
machine = desk
machine = laptop
```

- `prefix`, `suffix` — the placeholder delimiters. Defaults: `{{` and `}}`.
- `machine` — one line for each registered machine. `dot register` adds it.

### `.gitattributes` (repository)

`dot init` writes the line `* -text`. Git then stores all bytes unchanged. `dot` does not convert encodings or line ends.

### `files/` (repository)

The repository copy of `<ROOT>/<relative path>` is `files/<ROOT>/<relative path>`. The set of tracked files is the set of regular files below `files/`. There is no other list of files. A regular file directly below `files/` (without a root directory) is not a tracked file.

## Variables and placeholders

- Name pattern: `[A-Za-z0-9_-]+`. Names are case-sensitive.
- Value: one line of text, not empty.
- Lookup order for a name:
  1. `vars`.
  2. The built-in names `USERPROFILE`, `APPDATA` and `LOCALAPPDATA`: the value of the environment variable with the same name, if it is set and not empty.
  3. Otherwise the name is undefined.
- No other environment variable is read as a variable.
- A definition in `vars` replaces a built-in name, for example `USERPROFILE = D:\me`.

Delimiters:

- The `prefix` and `suffix` values in `dot.conf` set them. Defaults: `{{` and `}}`.
- A delimiter must not be empty and must not contain white space. The prefix must not end with a name character. The suffix must not start with a name character.

A placeholder is the prefix, a name, and the suffix, with nothing between them. Other text is literal text, for example `{{ name }}` or `{{a.b}}`.

Render makes one pass over the repository copy. `dot` does not scan a value again. If a placeholder name is undefined, the render of that file fails and the file is `unresolved`.

`dot` reads files as bytes. A file in an encoding that is not ASCII-compatible (for example UTF-16) has no placeholders and is synchronized unchanged.

To keep the literal text `{{foo}}` in a file, define `foo` with its own placeholder text as the value. The render then gives `{{foo}}` again. Do this on each machine, because `vars` is local:

```powershell
dot var foo '{{foo}}'
```

PowerShell needs the quotes, because `{` starts a script block.

## Commands

```text
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
```

General rules:

- Exit codes: `0` success; `1` failure or refusal; `2` usage error. `push` and `pull` return the exit code of git when git fails, which can be larger than 2.
- Results go to standard output. Errors and warnings go to standard error, with the prefix `dot: `.
- All commands except `init`, `register`, `help` and `version` need a registered machine: `config` has a `machine` line and `dot.conf` has the same name. Otherwise they stop with exit code 1 and the message `dot: machine not registered; run: dot register`.
- `add`, `apply`, `push` and `pull` stop with exit code 1 when the repository has unmerged paths: `dot: the repository has unmerged paths; resolve the conflict with git and commit`.
- `dot` without a command, and an unknown command, print the usage text to standard error and exit with code 2.
- `dot` prints variable values only in `var` and `meta`. Exception: `diff` shows the rendered repository copy, which contains values.

File argument of `edit`, `meta`, `diff` and `apply`: a tracked file matches if the argument is one of these:

- its logical name, for example `VIM/.gvimrc`;
- a suffix of the logical name at a `/` boundary, for example `.gvimrc`;
- its live path, absolute or relative to the current directory.

The comparison with the logical name is case-sensitive. On Windows, a `\` in the argument counts as `/`, and the comparison with the live path ignores case. `edit` needs exactly one match. The other commands act on all matches. No match is an error: `dot: no tracked file matches: <argument>`.

### `init [path]`

Creates or adopts the repository. Without `path`, the target is the `repo` value of `config`, or the default location `<local directory>\repo`. A relative `path` is relative to the current directory.

- Target is not a git work tree: `dot` creates the directory, runs `git init`, and writes `.gitattributes` and `dot.conf`.
- Target is a git work tree with `dot.conf` (a clone): `dot` adopts it and changes nothing in it.
- Target is a git work tree without `dot.conf`: `dot` adds `dot.conf` and the line `* -text` in `.gitattributes`.
- `dot` writes `repo` to `config` only when the target is not the default location.
- `init` makes no commit.

Exit codes: `0` success; `1` when `config` already points to a different repository (`dot: config already points to a different repository: <path>`), or when `git init` fails; `2` for more than one argument.

```powershell
dot init D:\sync\dotfiles
```

```text
created repository D:\sync\dotfiles
```

### `register [name]`

Registers this machine. Writes `machine` to `config` and adds a `machine` line to `dot.conf`. The default name is the host name in lower case, with each character outside the name pattern replaced by `-`. A second run with the same name changes nothing. `register` makes no commit; the next `push` commits the change.

Exit codes: `0` success; `1` when the name is not valid, when there is no repository (`dot: no repository; run: dot init`), or when `config` has a different name (`dot: this machine is already registered as <name>`); `2` for more than one argument.

```powershell
dot register laptop
```

```text
registered machine laptop
```

### `add <file>`

Tracks a live file, or captures an edit of a tracked live file. The argument is a live path, absolute or relative to the current directory. It must be an existing regular file.

`dot` derives the logical name from the path. The candidates are the variables whose value is an absolute path and a parent directory of the file. `dot` uses the candidate with the longest value; on a tie, the name that sorts first. No candidate is an error: `dot: no variable is a parent directory of <path>; define one: dot var <NAME> <directory>`. A skipped logical name is an error.

- Not tracked: `dot` copies the bytes into the repository, runs `git add`, and sets the state. If the content has placeholder text, `dot` prints a warning, because the next render replaces that text or rejects an undefined name.
- `in-sync`: no change (`unchanged <name>`).
- `live-changed`: capture (`captured <name>`). See [How capture works](#how-capture-works).
- `both-changed`: the live bytes replace the repository copy ("live wins", `replaced (live wins) <name>`). `dot` prints a warning for each placeholder of the old repository copy.
- `repo-changed`: error. The live file has no new content.
- `unresolved`: error, with the reasons.

`add` of a tracked file does not run `git add`. The next `push` or `pull` stages the change.

Exit codes: `0` success; `1` failure or refusal; `2` when the number of arguments is not one.

```powershell
dot add C:\Users\me\.gitconfig
```

```text
added USERPROFILE/.gitconfig
```

### `edit <file>`

Opens the repository copy in the editor and waits until the editor exits. The editor command is `EDITOR`, else `VISUAL`. `dot` splits the command at white space and appends the file path.

Exit codes: `0` success; `1` when neither variable is set (`dot: no editor: set EDITOR or VISUAL`), when the editor fails, or when the argument matches no file or more than one file; `2` when the number of arguments is not one.

```powershell
$env:EDITOR = 'notepad'
dot edit .gitconfig
```

### `var [name [value]]`

- `var` — finds each undefined root and placeholder name of the tracked files that are not skipped. For each name that has no stub in `vars`, appends a comment `# used by: <logical names>` and a stub line `# <name> =`. Then opens `vars` in the editor (`EDITOR`, else `VISUAL`).
- `var <name>` — prints `<name> = <value>` and `origin: vars` or `origin: environment`.
- `var <name> <value>` — validates the name and the value, then replaces the first line that defines the name, or its stub line. Otherwise it appends a line. The value is trimmed. Nothing is printed.

Exit codes: `0` success; `1` for an undefined name (`dot: variable <name> is undefined`), a name that is not valid, an empty value, a value with a line break, or an editor failure; `2` for more than two arguments.

```powershell
dot var VIM C:\tools\vim
dot var VIM
```

```text
VIM = C:\tools\vim
origin: vars
```

### `check`

For each tracked file that is not skipped, checks that the root is defined and is an absolute path, and that each placeholder name is defined. Prints one line for each problem.

Exit codes: `0` no problem; `1` one or more problems; `2` for an argument.

```powershell
dot check
```

```text
VIM/.gvimrc: root VIM is undefined
USERPROFILE/tmpl.txt: placeholder NAME is undefined
```

### `meta [file]`

Without an argument: prints the machine name, the local directory, the repository path, the delimiters, the registered machines, the variables in effect with their origin, and each tracked file with its status, logical name and live path. With an argument: prints, for each match, the logical name, the repository path, the live path, the status, the reasons, and each placeholder with its value and origin. `meta` writes no state.

Exit codes: `0` success; `1` error, for example no match; `2` for more than one argument.

```powershell
dot meta .gitconfig
```

```text
name: USERPROFILE/.gitconfig
repository path: C:\Users\me\.config\dot\repo\files\USERPROFILE\.gitconfig
live path: C:\Users\me\.gitconfig
status: in-sync
placeholder: EMAIL = me@desk.example (vars)
```

### `diff [file]`

For each selected file that is not `in-sync` and not `skipped`, prints one status line. For `unresolved`, the line has the reasons. For `repo-changed`, `live-changed` and `both-changed`, a unified difference from `git diff --no-index` follows. The old side is the rendered repository copy (`repository/<logical name>`); the new side is the live file (`live/<logical name>`). For a `live-changed` file, `diff` also prints the warnings that a capture would give: dropped placeholders, or a capture failure. `diff` writes no state.

Exit codes: `0` also when files differ; `1` error, for example no match; `2` for more than one argument.

```powershell
dot diff .gvimrc
```

```text
both-changed VIM/.gvimrc
diff --git repository/VIM/.gvimrc live/VIM/.gvimrc
index 19e6c69..4963978 100644
--- repository/VIM/.gvimrc
+++ live/VIM/.gvimrc
@@ -1,3 +1,2 @@
 set number
-set guifont=Consolas:h11
-set nowrap
+set guifont=Consolas:h12
```

### `apply [file] [--force]`

Writes live files from the repository. Action by status:

- `repo-changed`, `missing`: writes the live file (`wrote <name>`).
- `in-sync`: updates the state.
- `live-changed`: no action; prints `kept <name>: live-changed; dot push or dot pull will capture it`.
- `both-changed`: refuses the file.
- `unresolved`: reports the reasons.
- `skipped`: no action.

`--force` also writes `live-changed` and `both-changed` files ("repository wins").

A write goes to a temporary file in the destination directory, which then replaces the live file. `dot` creates missing parent directories. An existing live file keeps its permission bits; a new file gets `0644`. `dot` never deletes a live file.

Files are independent: a refused file does not stop the others.

Exit codes: `0` success; `1` when a file was refused, unresolved or not written, or when nothing matches; `2` for an unknown option or more than one file argument.

```powershell
dot apply
```

```text
wrote USERPROFILE/.gitconfig
wrote VIM/.gvimrc
```

### `push [-m <message>]`

1. Captures all `live-changed` files. If one capture fails, `push` writes nothing, prints all failures and exits with code 1.
2. Prints a warning for each `both-changed` file and continues.
3. Runs `git add -A`. If there are staged changes, commits with the message `<machine>: sync`, or with the `-m` message.
4. No git remote: prints `no remote; nothing pushed` and exits with code 0. Branch without upstream: runs `git push -u origin HEAD`. Otherwise runs `git push`.

When git fails, `push` prints `dot: run: dot pull` and returns the exit code of git.

Exit codes: `0` success; `1` capture failure or other error; the exit code of git when git fails; `2` for other arguments or an empty message.

```powershell
dot push -m "laptop: font size"
```

### `pull`

1. Steps 1 to 3 of `push`: capture, warn, commit.
2. No git remote: error (`dot: no remote; pull needs a git remote`), exit code 1. The commit of step 1 stays.
3. Runs `git pull`. The git configuration selects merge or rebase (see [Install](#install)).
4. When git fails, prints `dot: git pull failed; resolve the conflict with git, then run: dot apply` and returns the exit code of git.
5. On success, prints the number of `repo-changed` files. `pull` does not write live files.

Exit codes: `0` success; `1` capture failure, no remote, or other error; the exit code of git when git fails; `2` for an argument.

```powershell
dot pull
```

```text
1 file(s) repo-changed; run: dot apply
```

### `help` and `version`

`dot help` (also `-h` and `--help`) prints the usage text to standard output. `dot version` (also `--version`) prints the version. Both exit with code 0 and work on a machine that is not registered.

```powershell
dot version
```

```text
dot 0.1.1
```

## How capture works

Capture applies to a `live-changed` file. In this status the render of the repository copy is the exact content from which the live edit started.

- Repository copy without placeholders: the live content becomes the new repository copy.
- Repository copy with placeholders:
  1. `dot` compares the render and the live file line by line.
  2. Lines that did not change keep their repository text, with their placeholders.
  3. In a block of changed lines, `dot` finds the placeholders of the replaced repository lines. Where the value of one of these placeholders occurs in the new live lines, `dot` puts the placeholder back. If two values match at the same position, the longest wins.
  4. A placeholder of the replaced lines whose value no longer occurs in the block is dropped. `dot` prints a warning.
- Guard: the render of the result must be byte-identical to the live file. If it is not, the capture fails and the repository copy stays unchanged.

A dropped placeholder is a warning, not a failure:

```text
dot: warning: USERPROFILE/.gitconfig: line 3: placeholder {{EMAIL}} dropped
```

The line number is in the live file. After the capture, the repository copy has the live text in place of the placeholder. To restore the placeholder, edit the repository copy with `dot edit`.

Limits:

- `dot` puts a placeholder back only on lines that had it before the edit. A value that a live edit adds on another line stays literal text.
- A changed value text drops the placeholder (see the warning above).
- Text in the live file that has the form of a placeholder makes the capture fail: with a defined name, the render differs from the live file; with an undefined name, the result cannot be rendered.
- A variable value with a line break makes the capture fail.

A capture failure looks like this:

```text
dot: USERPROFILE/.gitconfig: capture failed: capture result does not render to the live content; edit the repository copy by hand; fix: dot edit USERPROFILE/.gitconfig, change the repository copy until its render is equal to the live file
```

## Manual fixes

### Capture failure

1. Run `dot diff <file>` to see the live edit.
2. Run `dot edit <file>`. Change the repository copy until its render is equal to the live file.
3. Run `dot meta <file>`. The status must be `in-sync`.
4. Run `dot push`.

`push` and `pull` capture all files or none. While one capture fails, they write nothing.

### both-changed

Both sides changed, or a new machine has a different existing file. `apply` refuses the file:

```text
dot: VIM/.gvimrc: both-changed; refused; see: dot diff VIM/.gvimrc; then: dot add <live file> (live wins) or dot apply --force VIM/.gvimrc (repository wins)
```

1. Run `dot diff <file>` to compare the two sides.
2. Select one side:
   - Live wins: `dot add <live file>`. The argument is the live path, for example `dot add C:\tools\vim\.gvimrc`. The live bytes replace the repository copy. Placeholders of the old repository copy are lost; `dot` prints a warning for each. Then run `dot push`.
   - Repository wins: `dot apply --force <file>`. The render replaces the live file.

To keep parts of both sides, edit the live file or the repository copy until one side has the content that you want, then use the applicable command.

### Merge conflict after `dot pull`

`git pull` reports a conflict, and `dot pull` stops:

```text
CONFLICT (content): Merge conflict in files/VIM/.gvimrc
Automatic merge failed; fix conflicts and then commit the result.
dot: git pull failed; resolve the conflict with git, then run: dot apply
```

The repository copies hold placeholders, so the conflict is in placeholder form. Until the conflict is resolved, `add`, `apply`, `push` and `pull` refuse to work.

1. Resolve the conflict in the repository with git: edit the file, then `git add` it.
2. Commit the merge with `git commit`. With rebase, continue the rebase with git instead.
3. Run `dot apply`.
4. Run `dot push`.

### No pull strategy

When the local branch and the remote branch have diverged and git has no pull strategy, `git pull` stops:

```text
fatal: Need to specify how to reconcile divergent branches.
dot: git pull failed; resolve the conflict with git, then run: dot apply
```

`dot pull` returns exit code 128. There is no conflict to resolve.

1. Set a pull strategy, for example `git config --global pull.rebase false` (see [Install](#install)).
2. Run `dot pull` again. The capture commit of the first run already exists, so nothing is lost.

### Push rejected

When the remote has commits that this machine does not have, git rejects the push and `dot` prints `dot: run: dot pull`. Run `dot pull`, then `dot apply` if necessary, then `dot push`.

## Limits and non-goals

Not in scope:

- template logic (conditions, loops);
- secrets or encryption;
- symbolic links;
- backups of live files;
- commands to remove or rename tracked files;
- recursive add of directories.

Other limits:

- Windows 11 is the tested platform. The code builds on Linux and macOS, but these systems are not tested.
- `dot` never deletes a live file.
- To remove or rename a tracked file, use git in the repository, for example `git rm files/VIM/.gvimrc` or `git mv`, then run `dot push`. The live files on all machines stay. `dot` removes the state entry of a file that is no longer tracked.
- `dot` does not merge. Git merges the repository copies at `git pull`.
- `dot` does not convert encodings or line ends.

## Release procedure

The workflow `.github/workflows/release.yml` runs on each push to `master` and on each pull request. On Windows it runs `gofmt`, `go vet` and the tests, then builds the executable.

To publish a release:

1. Set `version` in `internal/cli/run.go` to the new version, for example `0.2.0`.
2. Commit and push `master`. Wait until the workflow passes.
3. Create and push the tag. The tag is `v` plus the version:

   ```powershell
   git tag -a v0.2.0 -m "dot 0.2.0"
   git push origin v0.2.0
   ```

For a tag, the workflow also checks that the output of `dot version` is equal to the tag. Then it publishes a release with the Windows archive and `SHA256SUMS`. If a check fails, no release is published.

## License

`dot` has the MIT No Attribution license (SPDX identifier `MIT-0`). The full text is in [`LICENSE`](LICENSE).

You can use, copy, change and distribute the software for any purpose. You do not have to keep the copyright notice or the license text. The software has no warranty.
