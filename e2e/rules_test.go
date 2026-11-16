package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehael/dot/internal/core"
)

// E8. SYNC-4, CMD-14, CMD-16, CMD-17, CMD-18.
func TestE8VarChangeMakesRepoChanged(t *testing.T) {
	name := cfg("app/e8.conf")
	p := newPair(t,
		map[string]string{"user": "alice"},
		map[string]string{"user": "bob"},
		fileSpec{name: name, live: "user = alice\n", repo: "user = {{user}}\n"})

	p.B.setVar("user", "robert")
	p.B.ok("meta", name)
	wantContains(t, "meta on B", p.B.stdout(), "status: repo-changed")
	p.B.ok("diff")
	wantContains(t, "diff on B", p.B.stdout(), "repo-changed "+name)
	wantBytes(t, "live file on B before apply", p.B.readLive(name), "user = bob\n")

	p.B.ok("apply")
	wantContains(t, "apply on B", p.B.stdout(), "wrote "+name)
	wantBytes(t, "live file on B", p.B.readLive(name), "user = robert\n")
	wantBytes(t, "repository copy on B", p.B.readRepo(name), "user = {{user}}\n")
	p.B.ok("meta", name)
	wantContains(t, "meta on B", p.B.stdout(), "status: in-sync")
}

// E10. CMD-3, CMD-5: the gated commands refuse an unregistered machine.
func TestE10MachineNotRegistered(t *testing.T) {
	isolateGit(t)
	const msg = "dot: machine not registered; run: dot register\n"
	live := func(m *machine) string {
		p := filepath.Join(m.Live, "e10.txt")
		writeFile(t, p, "x\n")
		return p
	}
	refuseAll := func(m *machine, file string) {
		t.Helper()
		gated := [][]string{
			{"add", file}, {"edit", "e10.txt"}, {"var"}, {"var", "user", "x"}, {"check"},
			{"meta"}, {"diff"}, {"apply"}, {"push"}, {"pull"},
		}
		for _, args := range gated {
			code := m.run(args...)
			if code != 1 || m.stderr() != msg || m.stdout() != "" {
				t.Errorf("machine %s: dot %s: exit %d, stdout %q, stderr %q; want exit 1, stderr %q",
					m.Name, strings.Join(args, " "), code, m.stdout(), m.stderr(), msg)
			}
		}
	}

	// config has no machine.
	m := newMachine(t, "u")
	for _, args := range [][]string{{"help"}, {"version"}, {"init"}, {"init"}} {
		m.ok(args...)
		wantNotContains(t, "dot "+args[0], m.stderr(), "not registered")
	}
	file := live(m)
	refuseAll(m, file)

	// config has a machine, dot.conf has no line for it.
	m.addConfig("machine", "ghost")
	refuseAll(m, file)

	// register opens the gate.
	n := newMachine(t, "r")
	n.ok("init")
	n.ok("register", "r")
	wantContains(t, "register", n.stdout(), "registered machine r")
	n.ok("check")
	n.ok("meta")
	wantContains(t, "meta", n.stdout(), "machine: r")
}

// E11. PATH-4, A10: a skipped file is neither written nor captured.
func TestE11Skip(t *testing.T) {
	keep := cfg("app/keep.conf")
	skip := cfg("app/local/only.conf")
	p := newPair(t, nil, nil,
		fileSpec{name: keep, live: "keep 1\n"},
		fileSpec{name: skip, live: "shared 1\n"})

	// A trailing / in the value is ignored (PATH-4).
	p.B.addConfig("skip", cfg("app/local/"))
	p.B.writeLive(skip, "local on B\n")
	headB := p.B.head()
	p.B.ok("push")
	wantNotContains(t, "push on B", p.B.stdout(), "captured")
	wantBytes(t, "repository copy of the skipped file on B", p.B.readRepo(skip), "shared 1\n")
	if h := p.B.head(); h != headB {
		t.Errorf("push on B made a commit for a skipped file: %s", p.B.git("show", "--stat", "HEAD"))
	}

	p.A.writeLive(keep, "keep 2\n")
	p.A.writeLive(skip, "shared 2\n")
	p.A.ok("push")
	wantContains(t, "push on A", p.A.stdout(), "captured "+skip)
	p.B.ok("pull")
	wantContains(t, "pull on B", p.B.stdout(), "1 file(s) repo-changed")
	p.B.ok("apply")
	wantContains(t, "apply on B", p.B.stdout(), "wrote "+keep)
	wantNotContains(t, "apply on B", p.B.stdout(), skip)
	wantBytes(t, "live file on B", p.B.readLive(keep), "keep 2\n")
	wantBytes(t, "repository copy of the skipped file on B", p.B.readRepo(skip), "shared 2\n")
	wantBytes(t, "skipped live file on B", p.B.readLive(skip), "local on B\n")
}

// E14. CMD-15, section 7.1 rule 2.
func TestE14CheckUndefinedNames(t *testing.T) {
	name := cfg("app/e14.conf")
	tool := "OPT/tool.conf"
	p := newOrigin(t, map[string]string{"user": "alice"},
		fileSpec{name: name, live: "user = alice\n", repo: "user = {{user}}\n"})
	p.A.setVar("OPT", filepath.Join(p.A.Root, "opt"))
	p.A.writeLive(tool, "tool\n")
	p.A.ok("add", p.A.live(tool))
	p.A.ok("push")
	p.A.ok("check")

	// Root defined, placeholder name undefined.
	b1 := p.clone("b", map[string]string{"OPT": filepath.Join(t.TempDir(), "opt")})
	b1.want(1, "check")
	wantBytes(t, "check, placeholder undefined", b1.stdout(), name+": placeholder user is undefined\n")

	// Placeholder name defined, root undefined.
	b2 := p.clone("b", map[string]string{"user": "bob"})
	b2.want(1, "check")
	wantBytes(t, "check, root undefined", b2.stdout(), tool+": root OPT is undefined\n")

	b2.setVar("OPT", filepath.Join(b2.Root, "opt"))
	b2.ok("check")
	wantBytes(t, "check, all defined", b2.stdout(), "")
}

// E15. SYNC-11, SYNC-14, A23: one failed capture stops the whole push before
// any write.
func TestE15CaptureGuard(t *testing.T) {
	guarded := cfg("app/e15.conf")
	plain := cfg("app/plain.conf")
	p := newPair(t,
		map[string]string{"user": "alice"},
		map[string]string{"user": "bob"},
		fileSpec{name: guarded, live: "user = alice\n", repo: "user = {{user}}\n"},
		fileSpec{name: plain, live: "plain\n"})

	a := p.A
	statePath := filepath.Join(a.Dir, core.StateFile)
	state := readFile(t, statePath)
	head := a.head()
	bareHead := strings.TrimSpace(gitIn(t, p.Bare, "rev-parse", "HEAD"))

	// The capture of plain alone would succeed; SYNC-14 forbids writing it.
	a.writeLive(plain, "plain edited\n")
	a.writeLive(guarded, "user = alice\nliteral {{user}}\n")
	a.want(1, "push")
	wantContains(t, "push on A", a.stderr(), "dot: "+guarded+": capture failed")
	wantNotContains(t, "push on A", a.stdout(), "captured")

	wantBytes(t, "repository copy of the guarded file", a.readRepo(guarded), "user = {{user}}\n")
	wantBytes(t, "repository copy of the plain file", a.readRepo(plain), "plain\n")
	wantBytes(t, "state file", readFile(t, statePath), state)
	if h := a.head(); h != head {
		t.Errorf("push made a commit: HEAD %s, want %s", h, head)
	}
	if st := a.git("status", "--porcelain"); st != "" {
		t.Errorf("push changed the work tree:\n%s", st)
	}
	if h := strings.TrimSpace(gitIn(t, p.Bare, "rev-parse", "HEAD")); h != bareHead {
		t.Errorf("push reached the remote: HEAD %s, want %s", h, bareHead)
	}
	wantBytes(t, "live file of the guarded file", a.readLive(guarded), "user = alice\nliteral {{user}}\n")
}
