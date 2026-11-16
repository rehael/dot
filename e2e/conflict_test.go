package e2e

import (
	"strings"
	"testing"
)

const e6Repo = "one = {{user}}\ntwo\nthree\nfour\nfive = {{user}}\n"

func newE6Pair(t *testing.T) (*pair, string) {
	t.Helper()
	name := cfg("app/e6.conf")
	p := newPair(t,
		map[string]string{"user": "alice"},
		map[string]string{"user": "bob"},
		fileSpec{name: name, live: "one = alice\ntwo\nthree\nfour\nfive = alice\n", repo: e6Repo})
	return p, name
}

// E6, different lines. D4 (git merges repository copies), CMD-20.
func TestE6aMergeDifferentLines(t *testing.T) {
	p, name := newE6Pair(t)

	p.A.writeLive(name, "one = alice\ntwo A\nthree\nfour\nfive = alice\n")
	p.A.ok("push")
	p.B.writeLive(name, "one = bob\ntwo\nthree\nfour B\nfive = bob\n")
	p.B.ok("pull")
	wantContains(t, "pull on B", p.B.stdout(), "captured "+name)
	wantContains(t, "pull on B", p.B.stdout(), "1 file(s) repo-changed; run: dot apply")
	p.B.ok("apply")

	union := "one = {{user}}\ntwo A\nthree\nfour B\nfive = {{user}}\n"
	wantBytes(t, "repository copy on B", p.B.readRepo(name), union)
	wantBytes(t, "live file on B", p.B.readLive(name), "one = bob\ntwo A\nthree\nfour B\nfive = bob\n")

	p.B.ok("push")
	p.A.ok("pull")
	p.A.ok("apply")
	wantBytes(t, "live file on A", p.A.readLive(name), "one = alice\ntwo A\nthree\nfour B\nfive = alice\n")
}

// E6, same line. CMD-20 step 4, SYNC-17.
func TestE6bConflictSameLine(t *testing.T) {
	p, name := newE6Pair(t)

	p.A.writeLive(name, "one = alice\ntwo A\nthree\nfour\nfive = alice\n")
	p.A.ok("push")
	liveB := "one = bob\ntwo B\nthree\nfour\nfive = bob\n"
	p.B.writeLive(name, liveB)

	if code := p.B.run("pull"); code == 0 {
		t.Fatalf("pull on B with a conflict: exit 0\nstdout:\n%s\nstderr:\n%s", p.B.stdout(), p.B.stderr())
	}
	wantContains(t, "pull on B", p.B.stdout()+p.B.stderr(), "CONFLICT")
	wantContains(t, "pull on B", p.B.stderr(), "dot: git pull failed; resolve the conflict with git, then run: dot apply")
	if out := p.B.git("ls-files", "-u"); strings.TrimSpace(out) == "" {
		t.Fatal("git reports no unmerged path after the conflict")
	}

	p.B.want(1, "apply")
	wantContains(t, "apply on B", p.B.stderr(), "dot: the repository has unmerged paths; resolve the conflict with git and commit")
	wantBytes(t, "live file on B after the refused apply", p.B.readLive(name), liveB)

	resolved := "one = {{user}}\ntwo A and B\nthree\nfour\nfive = {{user}}\n"
	p.B.editRepo(name, resolved)
	p.B.git("add", "--", "files/"+name)
	p.B.git("commit", "-q", "-m", "resolve")

	p.B.ok("apply")
	wantContains(t, "apply on B", p.B.stdout(), "wrote "+name)
	wantBytes(t, "live file on B", p.B.readLive(name), "one = bob\ntwo A and B\nthree\nfour\nfive = bob\n")
}

// newBothChanged makes the file both-changed on B: A publishes an edit, B
// edits its live file and then takes A's commit with git pull, which does not
// capture (section 7.1 rule 7).
func newBothChanged(t *testing.T) (*pair, string, string) {
	t.Helper()
	name := cfg("app/e7.conf")
	p := newPair(t,
		map[string]string{"user": "alice"},
		map[string]string{"user": "bob"},
		fileSpec{name: name, live: "user = alice\nmode = 1\n", repo: "user = {{user}}\nmode = 1\n"})

	p.A.writeLive(name, "user = alice\nmode = 2\n")
	p.A.ok("push")
	liveB := "user = bob\nmode = 3\n"
	p.B.writeLive(name, liveB)
	p.B.git("pull", "-q")

	p.B.ok("meta", name)
	wantContains(t, "meta on B", p.B.stdout(), "status: both-changed")
	return p, name, liveB
}

// E7, repository wins. CMD-18 (both-changed refused, --force).
func TestE7BothChangedApplyForce(t *testing.T) {
	p, name, liveB := newBothChanged(t)

	p.B.want(1, "apply")
	wantContains(t, "apply on B", p.B.stderr(), "dot: "+name+": both-changed; refused")
	wantBytes(t, "live file on B after the refused apply", p.B.readLive(name), liveB)

	p.B.ok("apply", "--force")
	wantContains(t, "apply --force on B", p.B.stdout(), "wrote "+name)
	wantBytes(t, "live file on B", p.B.readLive(name), "user = bob\nmode = 2\n")
	wantBytes(t, "repository copy on B", p.B.readRepo(name), "user = {{user}}\nmode = 2\n")

	p.B.ok("meta", name)
	wantContains(t, "meta on B", p.B.stdout(), "status: in-sync")
}

// E7, live wins. CMD-12 (both-changed), A5.
func TestE7BothChangedAddLiveWins(t *testing.T) {
	p, name, liveB := newBothChanged(t)

	p.B.ok("add", p.B.live(name))
	wantContains(t, "add on B", p.B.stdout(), "replaced (live wins) "+name)
	wantContains(t, "add on B", p.B.stderr(), "placeholder {{user}} of the old repository copy is replaced by live text")
	wantBytes(t, "repository copy on B", p.B.readRepo(name), liveB)
	wantBytes(t, "live file on B", p.B.readLive(name), liveB)

	p.B.ok("meta", name)
	wantContains(t, "meta on B", p.B.stdout(), "status: in-sync")
}

// E9. Section 7.1 rule 7 (no H and L != R), CMD-18, SYNC-8.
func TestE9NewMachineDifferentLiveFile(t *testing.T) {
	name := cfg("app/e9.conf")
	p := newOrigin(t, nil, fileSpec{name: name, live: "from A\n"})
	p.B = p.clone("b", nil)

	liveB := "existing on B\n"
	p.B.writeLive(name, liveB)

	p.B.ok("pull")
	wantContains(t, "pull on B", p.B.stderr(), name+": both-changed, not captured")
	wantBytes(t, "repository copy on B after pull", p.B.readRepo(name), "from A\n")

	p.B.want(1, "apply")
	wantContains(t, "apply on B", p.B.stderr(), "dot: "+name+": both-changed; refused")
	wantBytes(t, "live file on B", p.B.readLive(name), liveB)
}
