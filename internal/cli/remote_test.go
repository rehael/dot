package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehael/dot/internal/core"
	"github.com/rehael/dot/internal/gitx"
)

// remoteLive returns the live path of a file below root H of m.
func remoteLive(m *machine, rel string) string {
	return filepath.Join(m.Home, filepath.FromSlash(rel))
}

// remoteSetup returns a registered machine with root H in its home, the
// variable NAME, and one tracked file whose live copy is the render.
func remoteSetup(t *testing.T, name string) *machine {
	t.Helper()
	m := newRegistered(t, name)
	m.setVar("H", m.Home)
	m.setVar("NAME", "alice")
	m.track("H/f.txt", "user={{NAME}}\nline2\n")
	writeFile(t, remoteLive(m, "f.txt"), "user=alice\nline2\n")
	return m
}

func remoteState(m *machine) string {
	data, err := os.ReadFile(filepath.Join(m.Dir, core.StateFile))
	if err != nil && !os.IsNotExist(err) {
		m.t.Fatal(err)
	}
	return string(data)
}

func remoteHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func remoteSubject(m *machine) string {
	return strings.TrimSpace(m.git("log", "-1", "--format=%s"))
}

func remoteCount(m *machine) string {
	return strings.TrimSpace(m.git("rev-list", "--count", "HEAD"))
}

func remoteRun(t *testing.T, m *machine, want int, args ...string) {
	t.Helper()
	if code := m.run(args...); code != want {
		t.Fatalf("dot %v: exit code %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, want, m.Out.String(), m.Err.String())
	}
}

// remoteBare creates a bare repository and makes it the origin of m.
func remoteBare(t *testing.T, m *machine) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "remote.git")
	if _, err := (gitx.Git{Dir: filepath.Dir(bare)}).Output("init", "--bare", "-q", bare); err != nil {
		t.Fatal(err)
	}
	m.git("remote", "add", "origin", bare)
	return bare
}

// remoteClone returns a second machine whose repository is a clone of bare
// and whose config names it. Its machine line must already be in dot.conf.
func remoteClone(t *testing.T, bare, name string) *machine {
	t.Helper()
	m := newMachine(t)
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (gitx.Git{Dir: m.Dir}).Output("clone", "-q", bare, m.Repo); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(m.Dir, core.ConfigFile), "machine = "+name+"\n")
	return m
}

// remotePair returns machine a with a pushed repository that registers a and
// b, and machine b as its clone. Both track H/f.txt with live content "v1\n"
// and are in-sync with a state hash.
func remotePair(t *testing.T) (a, b *machine) {
	t.Helper()
	a = newRegistered(t, "a")
	writeFile(t, filepath.Join(a.Repo, core.RepoConfFile), "prefix = {{\nsuffix = }}\nmachine = a\nmachine = b\n")
	a.setVar("H", a.Home)
	a.track("H/f.txt", "v1\n")
	writeFile(t, remoteLive(a, "f.txt"), "v1\n")
	bare := remoteBare(t, a)
	remoteRun(t, a, 0, "push")

	b = remoteClone(t, bare, "b")
	b.setVar("H", b.Home)
	writeFile(t, remoteLive(b, "f.txt"), "v1\n")
	remoteRun(t, b, 0, "pull") // in-sync sets the state hash of b (SYNC-2)
	if got := b.Out.String(); !strings.Contains(got, "0 file(s) repo-changed\n") || strings.Contains(got, "dot apply") {
		t.Fatalf("first pull of b: stdout %q", got)
	}
	return a, b
}

// CMD-19, A7, A8, SYNC-2: without a remote, push commits with the default
// message, prints a note and exits 0; an in-sync file gets its state hash.
func TestPushNoRemoteCommitsDefaultMessage(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 0, "push")
	if got := remoteSubject(m); got != "a: sync" {
		t.Errorf("commit subject %q, want %q", got, "a: sync")
	}
	if !strings.Contains(m.Out.String(), "no remote") {
		t.Errorf("stdout has no note: %q", m.Out.String())
	}
	if st := m.git("status", "--porcelain"); st != "" {
		t.Errorf("work tree not clean: %q", st)
	}
	want := remoteHash("user=alice\nline2\n") + " H/f.txt\n"
	if got := remoteState(m); got != want {
		t.Errorf("state %q, want %q", got, want)
	}
}

// CMD-19, A7: -m replaces the default commit message.
func TestPushMessageOverride(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 0, "push", "-m", "my message")
	if got := remoteSubject(m); got != "my message" {
		t.Errorf("commit subject %q", got)
	}
}

// CMD-19: with no staged change, push makes no commit.
func TestPushNothingToCommit(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 0, "push")
	before := remoteCount(m)
	remoteRun(t, m, 0, "push")
	if got := remoteCount(m); got != before {
		t.Errorf("commit count %s, want %s", got, before)
	}
}

// CMD-19: the first push sets the upstream with "git push -u origin HEAD".
func TestPushFirstPushSetsUpstream(t *testing.T) {
	m := remoteSetup(t, "a")
	bare := remoteBare(t, m)
	remoteRun(t, m, 0, "push")
	branch := strings.TrimSpace(m.git("rev-parse", "--abbrev-ref", "HEAD"))
	if got := strings.TrimSpace(m.git("rev-parse", "--abbrev-ref", "@{u}")); got != "origin/"+branch {
		t.Errorf("upstream %q, want origin/%s", got, branch)
	}
	remoteHead, err := (gitx.Git{Dir: bare}).Output("rev-parse", branch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(remoteHead) != strings.TrimSpace(m.git("rev-parse", "HEAD")) {
		t.Errorf("remote %s is not at the local HEAD", branch)
	}
}

// CMD-19: with an upstream, push runs plain "git push". The upstream points
// to a branch with another name, so that only plain "git push" (with
// push.default=upstream) updates it and "push -u origin HEAD" would not.
func TestPushWithUpstreamUsesPlainPush(t *testing.T) {
	m := remoteSetup(t, "a")
	bare := remoteBare(t, m)
	remoteRun(t, m, 0, "push")
	branch := strings.TrimSpace(m.git("rev-parse", "--abbrev-ref", "HEAD"))
	m.git("push", "-q", "origin", "HEAD:other")
	m.git("branch", "--set-upstream-to=origin/other")
	m.git("config", "push.default", "upstream")
	branchBefore, err := (gitx.Git{Dir: bare}).Output("rev-parse", branch)
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, remoteLive(m, "f.txt"), "user=alice\nline2 edited\n")
	remoteRun(t, m, 0, "push")
	bg := gitx.Git{Dir: bare}
	other, err := bg.Output("rev-parse", "other")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(other) != strings.TrimSpace(m.git("rev-parse", "HEAD")) {
		t.Errorf("upstream branch other was not pushed")
	}
	branchAfter, err := bg.Output("rev-parse", branch)
	if err != nil {
		t.Fatal(err)
	}
	if branchAfter != branchBefore {
		t.Errorf("remote %s moved; push did not use the upstream", branch)
	}
}

// SYNC-13, SYNC-2, CMD-19: push captures a live edit, keeps the placeholder
// in the repository copy, commits it, and sets the state hash to the live bytes.
func TestPushCapturesLiveChange(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 0, "push")
	live := "user=alice\nline2 edited\n"
	writeFile(t, remoteLive(m, "f.txt"), live)
	remoteRun(t, m, 0, "push")
	if !strings.Contains(m.Out.String(), "captured H/f.txt\n") {
		t.Errorf("stdout: %q", m.Out.String())
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "user={{NAME}}\nline2 edited\n" {
		t.Errorf("repository copy %q", got)
	}
	if got := m.git("show", "HEAD:files/H/f.txt"); got != "user={{NAME}}\nline2 edited\n" {
		t.Errorf("committed copy %q", got)
	}
	if want := remoteHash(live) + " H/f.txt\n"; remoteState(m) != want {
		t.Errorf("state %q, want %q", remoteState(m), want)
	}
	if strings.Contains(m.Out.String()+m.Err.String(), "alice") {
		t.Errorf("output contains a variable value (NF-6)")
	}
}

// SYNC-15, A9: a dropped placeholder is a warning; push still succeeds.
func TestPushDroppedPlaceholderWarns(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 0, "push")
	writeFile(t, remoteLive(m, "f.txt"), "user=bob\nline2\n")
	remoteRun(t, m, 0, "push")
	if want := "dot: warning: H/f.txt: line 1: placeholder {{NAME}} dropped\n"; !strings.Contains(m.Err.String(), want) {
		t.Errorf("stderr %q, want %q", m.Err.String(), want)
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "user=bob\nline2\n" {
		t.Errorf("repository copy %q", got)
	}
	if strings.Contains(m.Out.String()+m.Err.String(), "alice") {
		t.Errorf("output contains a variable value (NF-6)")
	}
}

// SYNC-14, SYNC-11, SYNC-16, A23: one good and two failing captures. Push
// writes no repository copy and no state, makes no commit, prints every
// failure with its manual fix, and exits 1.
func TestPushCaptureFailureWritesNothing(t *testing.T) {
	m := newRegistered(t, "a")
	m.setVar("H", m.Home)
	m.setVar("NAME", "alice")
	files := map[string]string{"a.txt": "x={{NAME}}\n", "b.txt": "y\n", "c.txt": "z\n"}
	for rel, src := range files {
		m.track("H/"+rel, src)
	}
	writeFile(t, remoteLive(m, "a.txt"), "x=alice\n")
	writeFile(t, remoteLive(m, "b.txt"), "y\n")
	writeFile(t, remoteLive(m, "c.txt"), "z\n")
	remoteRun(t, m, 0, "push")
	stateBefore := remoteState(m)
	headBefore := m.git("rev-parse", "HEAD")

	writeFile(t, remoteLive(m, "a.txt"), "x=alice\nmore\n")
	// Placeholder-like text of a defined variable cannot be captured.
	writeFile(t, remoteLive(m, "b.txt"), "y\n{{NAME}}\n")
	writeFile(t, remoteLive(m, "c.txt"), "z {{NAME}}\n")
	remoteRun(t, m, 1, "push")

	for rel, src := range files {
		if got := readFile(t, m.repoCopy("H/"+rel)); got != src {
			t.Errorf("repository copy %s changed to %q", rel, got)
		}
	}
	if got := remoteState(m); got != stateBefore {
		t.Errorf("state changed:\n%q\nwant\n%q", got, stateBefore)
	}
	if got := m.git("rev-parse", "HEAD"); got != headBefore {
		t.Errorf("a commit was made")
	}
	if st := m.git("status", "--porcelain"); st != "" {
		t.Errorf("work tree changed: %q", st)
	}
	for _, name := range []string{"H/b.txt", "H/c.txt"} {
		if !strings.Contains(m.Err.String(), name+": capture failed") || !strings.Contains(m.Err.String(), "dot edit "+name) {
			t.Errorf("stderr does not report %s with its fix:\n%s", name, m.Err.String())
		}
	}
	if strings.Contains(m.Out.String(), "captured") {
		t.Errorf("stdout reports a capture: %q", m.Out.String())
	}
	if strings.Contains(m.Out.String()+m.Err.String(), "alice") {
		t.Errorf("output contains a variable value (NF-6)")
	}
}

// CMD-19: an error from the status of one file stops push before any write.
func TestPushInfoErrorWritesNothing(t *testing.T) {
	m := newRegistered(t, "a")
	m.setVar("H", m.Home)
	m.track("H/a.txt", "a\n")
	m.track("H/b.txt", "b\n")
	writeFile(t, remoteLive(m, "a.txt"), "a\n")
	remoteRun(t, m, 0, "push") // b is missing; a gets its state hash
	stateBefore := remoteState(m)
	headBefore := m.git("rev-parse", "HEAD")

	writeFile(t, remoteLive(m, "a.txt"), "a edited\n")
	if err := os.MkdirAll(remoteLive(m, "b.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	remoteRun(t, m, 1, "push")
	if got := readFile(t, m.repoCopy("H/a.txt")); got != "a\n" {
		t.Errorf("repository copy changed to %q", got)
	}
	if remoteState(m) != stateBefore || m.git("rev-parse", "HEAD") != headBefore {
		t.Errorf("state or HEAD changed")
	}
}

// CMD-19 step 2: a both-changed file gives a warning and is not captured.
func TestPushBothChangedWarns(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 0, "push")
	stateBefore := remoteState(m)
	writeFile(t, m.repoCopy("H/f.txt"), "user={{NAME}}\nrepo edit\n")
	m.git("commit", "-q", "-am", "repo edit")
	writeFile(t, remoteLive(m, "f.txt"), "user=alice\nlive edit\n")
	remoteRun(t, m, 0, "push")
	if !strings.Contains(m.Err.String(), "dot: warning: H/f.txt: both-changed, not captured") {
		t.Errorf("stderr: %q", m.Err.String())
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "user={{NAME}}\nrepo edit\n" {
		t.Errorf("repository copy %q", got)
	}
	if got := remoteState(m); got != stateBefore {
		t.Errorf("state changed")
	}
	if strings.Contains(m.Out.String(), "captured") {
		t.Errorf("stdout reports a capture: %q", m.Out.String())
	}
}

// CMD-19 step 5: a rejected push returns the code of git and the hint.
func TestPushRejected(t *testing.T) {
	a, b := remotePair(t)
	writeFile(t, b.repoCopy("H/other.txt"), "from b\n")
	b.git("add", "-A")
	b.git("commit", "-q", "-m", "b change")
	b.git("push", "-q")

	writeFile(t, remoteLive(a, "f.txt"), "v2\n")
	code := a.run("push")
	if code == 0 {
		t.Fatalf("exit code 0, want the non-zero code of git")
	}
	if !strings.Contains(a.Err.String(), "dot: run: dot pull\n") {
		t.Errorf("stderr: %q", a.Err.String())
	}
}

// CMD-1: bad arguments are usage errors, before anything is committed.
func TestPushUsage(t *testing.T) {
	cases := [][]string{{"x"}, {"-m"}, {"-m", "a", "b"}, {"-m", ""}, {"--message", "a"}}
	for _, args := range cases {
		m := remoteSetup(t, "a")
		remoteRun(t, m, 2, append([]string{"push"}, args...)...)
		if !strings.Contains(m.Err.String(), "usage: dot push") {
			t.Errorf("push %q: stderr %q", args, m.Err.String())
		}
		if _, err := m.gitTry("rev-parse", "-q", "--verify", "HEAD"); err == nil {
			t.Errorf("push %q made a commit", args)
		}
	}
}

// CMD-1: pull takes no argument.
func TestPullUsage(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 2, "pull", "x")
	if !strings.Contains(m.Err.String(), "usage: dot pull") {
		t.Errorf("stderr %q", m.Err.String())
	}
}

// CMD-20, A8: pull without a remote is an error.
func TestPullNoRemote(t *testing.T) {
	m := remoteSetup(t, "a")
	remoteRun(t, m, 1, "pull")
	if !strings.Contains(m.Err.String(), "no remote") {
		t.Errorf("stderr %q", m.Err.String())
	}
}

// CMD-20: pull brings a change of the other machine, leaves the live file
// unchanged, and reports the repo-changed count.
func TestPullBringsChange(t *testing.T) {
	a, b := remotePair(t)
	writeFile(t, remoteLive(a, "f.txt"), "v2\n")
	remoteRun(t, a, 0, "push")

	remoteRun(t, b, 0, "pull")
	if got := readFile(t, remoteLive(b, "f.txt")); got != "v1\n" {
		t.Errorf("live file of b changed to %q", got)
	}
	if got := readFile(t, b.repoCopy("H/f.txt")); got != "v2\n" {
		t.Errorf("repository copy of b %q", got)
	}
	if !strings.Contains(b.Out.String(), "1 file(s) repo-changed; run: dot apply\n") {
		t.Errorf("stdout %q", b.Out.String())
	}
}

// CMD-20 step 4: a conflict returns the code of git and the hint; the live
// file stays unchanged.
func TestPullConflict(t *testing.T) {
	a, b := remotePair(t)
	writeFile(t, remoteLive(a, "f.txt"), "from a\n")
	remoteRun(t, a, 0, "push")

	b.git("config", "pull.rebase", "false")
	writeFile(t, remoteLive(b, "f.txt"), "from b\n")
	code := b.run("pull")
	if code == 0 {
		t.Fatalf("exit code 0, want the non-zero code of git\nstdout:\n%s", b.Out.String())
	}
	if !strings.Contains(b.Out.String(), "captured H/f.txt") {
		t.Errorf("the edit of b was not captured before the pull: %q", b.Out.String())
	}
	if !strings.Contains(b.Err.String(), "resolve the conflict with git, then run: dot apply") {
		t.Errorf("stderr %q", b.Err.String())
	}
	if got := readFile(t, remoteLive(b, "f.txt")); got != "from b\n" {
		t.Errorf("live file of b changed to %q", got)
	}
	if out := b.git("ls-files", "-u"); out == "" {
		t.Errorf("no unmerged paths after the conflict")
	}
}
