package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehael/dot/internal/core"
)

// filesSecret is a variable value that only meta may print (NF-6).
const filesSecret = "s3cr3t-value"

// filesSetup returns a registered machine whose root H is its home directory
// and whose variable N has the value filesSecret.
func filesSetup(t *testing.T) *machine {
	t.Helper()
	m := newRegistered(t, "a")
	m.setVar("H", m.Home)
	m.setVar("N", filesSecret)
	return m
}

func filesHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// filesStateLine is one line of the state file (ST-2).
func filesStateLine(name, content string) string {
	return filesHash(content) + " " + name + "\n"
}

// filesWriteState writes the state file directly; lines must be sorted by name.
func filesWriteState(t *testing.T, m *machine, lines ...string) {
	t.Helper()
	writeFile(t, filepath.Join(m.Dir, core.StateFile), strings.Join(lines, ""))
}

// filesState returns the state file, or "<none>" if it does not exist.
func filesState(t *testing.T, m *machine) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(m.Dir, core.StateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "<none>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func filesStatus(t *testing.T, m *machine, name string) core.Status {
	t.Helper()
	w, err := core.Open(m.env())
	if err != nil {
		t.Fatal(err)
	}
	info, err := w.Info(name)
	if err != nil {
		t.Fatal(err)
	}
	return info.Status
}

func filesLive(m *machine, rel string) string {
	return filepath.Join(m.Home, filepath.FromSlash(rel))
}

func filesWantCode(t *testing.T, m *machine, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("exit code %d, want %d\nstdout: %s\nstderr: %s", got, want, m.Out.String(), m.Err.String())
	}
}

// filesNoSecret checks NF-6 for a command other than var and meta.
func filesNoSecret(t *testing.T, m *machine) {
	t.Helper()
	if strings.Contains(m.Out.String()+m.Err.String(), filesSecret) {
		t.Errorf("output holds a variable value (NF-6):\nstdout: %s\nstderr: %s", m.Out.String(), m.Err.String())
	}
}

// CMD-12: add of an untracked file copies the bytes, stages them and sets H.
func TestAddUntracked(t *testing.T) {
	m := filesSetup(t)
	content := "hello\r\nworld"
	writeFile(t, filesLive(m, "sub/f.txt"), content)

	filesWantCode(t, m, m.run("add", filepath.Join("sub", "f.txt")), 0)
	if got := m.Out.String(); got != "added H/sub/f.txt\n" {
		t.Errorf("stdout %q", got)
	}
	if m.Err.Len() != 0 {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := readFile(t, m.repoCopy("H/sub/f.txt")); got != content {
		t.Errorf("repository copy %q, want %q", got, content)
	}
	if staged := m.git("diff", "--cached", "--name-only"); !strings.Contains(staged, "files/H/sub/f.txt") {
		t.Errorf("not staged: %q", staged)
	}
	if got := filesState(t, m); got != filesStateLine("H/sub/f.txt", content) {
		t.Errorf("state %q", got)
	}
	if s := filesStatus(t, m, "H/sub/f.txt"); s != core.InSync {
		t.Errorf("status %s, want in-sync", s)
	}
}

// CMD-12: an absolute path is accepted too.
func TestAddUntrackedAbsolute(t *testing.T) {
	m := filesSetup(t)
	writeFile(t, filesLive(m, "g.conf"), "x\n")
	filesWantCode(t, m, m.run("add", filesLive(m, "g.conf")), 0)
	if got := m.Out.String(); got != "added H/g.conf\n" {
		t.Errorf("stdout %q", got)
	}
}

// CMD-12, VAR-8: placeholder text in a new file gives one warning that names it.
func TestAddUntrackedPlaceholderWarning(t *testing.T) {
	m := filesSetup(t)
	writeFile(t, filesLive(m, "f.txt"), "a {{N}} b {{foo}}\n")
	filesWantCode(t, m, m.run("add", "f.txt"), 0)
	errText := m.Err.String()
	if strings.Count(errText, "\n") != 1 || !strings.HasPrefix(errText, "dot: warning: ") {
		t.Fatalf("want one warning, got %q", errText)
	}
	for _, s := range []string{"{{N}}", "{{foo}}", "dot edit H/f.txt"} {
		if !strings.Contains(errText, s) {
			t.Errorf("warning does not name %q: %q", s, errText)
		}
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "a {{N}} b {{foo}}\n" {
		t.Errorf("repository copy %q", got)
	}
	filesNoSecret(t, m)
}

// PATH-3: no root variable is a parent of the file; the error names the fix.
func TestAddNoRoot(t *testing.T) {
	m := filesSetup(t)
	outside := filepath.Join(t.TempDir(), "f.txt")
	writeFile(t, outside, "x\n")
	filesWantCode(t, m, m.run("add", outside), 1)
	if !strings.Contains(m.Err.String(), "dot var") {
		t.Errorf("stderr does not name dot var: %q", m.Err.String())
	}
	if got := filesState(t, m); got != "<none>" {
		t.Errorf("state written: %q", got)
	}
}

// CMD-12, PATH-4: a skipped name is an error.
func TestAddSkipped(t *testing.T) {
	m := filesSetup(t)
	writeFile(t, filepath.Join(m.Dir, core.ConfigFile), "machine = a\nskip = H/sub/\n")
	writeFile(t, filesLive(m, "sub/f.txt"), "x\n")
	filesWantCode(t, m, m.run("add", "sub/f.txt"), 1)
	if !strings.Contains(m.Err.String(), "skip") {
		t.Errorf("stderr %q", m.Err.String())
	}
	if _, err := os.Stat(m.repoCopy("H/sub/f.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("repository copy exists: %v", err)
	}
}

// CMD-12: the argument must be an existing regular file.
func TestAddNotRegularFile(t *testing.T) {
	m := filesSetup(t)
	if err := os.MkdirAll(filesLive(m, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	filesWantCode(t, m, m.run("add", "dir"), 1)
	if !strings.Contains(m.Err.String(), "not a regular file") {
		t.Errorf("stderr %q", m.Err.String())
	}
	filesWantCode(t, m, m.run("add", "absent.txt"), 1)
}

// CMD-1: add takes exactly one argument.
func TestAddUsage(t *testing.T) {
	m := filesSetup(t)
	filesWantCode(t, m, m.run("add"), 2)
	filesWantCode(t, m, m.run("add", "a", "b"), 2)
}

// CMD-12, SYNC-2: in-sync changes no file and sets H = L.
func TestAddInSync(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "v={{N}}\n")
	live := "v=" + filesSecret + "\n"
	writeFile(t, filesLive(m, "f.txt"), live)
	filesWantCode(t, m, m.run("add", "f.txt"), 0)
	if got := m.Out.String(); got != "unchanged H/f.txt\n" {
		t.Errorf("stdout %q", got)
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "v={{N}}\n" {
		t.Errorf("repository copy %q", got)
	}
	if got := filesState(t, m); got != filesStateLine("H/f.txt", live) {
		t.Errorf("state %q", got)
	}
	filesNoSecret(t, m)
}

// CMD-12, SYNC-13: live-changed is captured; an edit on a placeholder line
// with the value kept keeps the placeholder.
func TestAddLiveChangedKeepsPlaceholder(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "v={{N}}\nx\n")
	filesWriteState(t, m, filesStateLine("H/f.txt", "v="+filesSecret+"\nx\n"))
	live := "v=" + filesSecret + ";\nx\n"
	writeFile(t, filesLive(m, "f.txt"), live)

	filesWantCode(t, m, m.run("add", "f.txt"), 0)
	if got := m.Out.String(); got != "captured H/f.txt\n" {
		t.Errorf("stdout %q", got)
	}
	if m.Err.Len() != 0 {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "v={{N}};\nx\n" {
		t.Errorf("repository copy %q", got)
	}
	if got := filesState(t, m); got != filesStateLine("H/f.txt", live) {
		t.Errorf("state %q", got)
	}
	if s := filesStatus(t, m, "H/f.txt"); s != core.InSync {
		t.Errorf("status %s", s)
	}
	filesNoSecret(t, m)
}

// CMD-12, SYNC-15: a capture that drops a placeholder warns and succeeds.
func TestAddLiveChangedDropWarning(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\nv={{N}}\n")
	filesWriteState(t, m, filesStateLine("H/f.txt", "x\nv="+filesSecret+"\n"))
	live := "x\nv=other\n"
	writeFile(t, filesLive(m, "f.txt"), live)

	filesWantCode(t, m, m.run("add", "f.txt"), 0)
	if got := m.Err.String(); got != "dot: warning: H/f.txt: line 2: placeholder {{N}} dropped\n" {
		t.Errorf("stderr %q", got)
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != live {
		t.Errorf("repository copy %q", got)
	}
	if got := filesState(t, m); got != filesStateLine("H/f.txt", live) {
		t.Errorf("state %q", got)
	}
	filesNoSecret(t, m)
}

// SYNC-11, SYNC-16, A23: a failed capture leaves the repository copy and the
// state unchanged and names the manual fix.
func TestAddCaptureFailure(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "a\n")
	state := filesStateLine("H/f.txt", "a\n")
	filesWriteState(t, m, state)
	writeFile(t, filesLive(m, "f.txt"), "a\n{{N}}\n")

	filesWantCode(t, m, m.run("add", "f.txt"), 1)
	if !strings.Contains(m.Err.String(), "H/f.txt") || !strings.Contains(m.Err.String(), "dot edit H/f.txt") {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "a\n" {
		t.Errorf("repository copy changed: %q", got)
	}
	if got := filesState(t, m); got != state {
		t.Errorf("state changed: %q", got)
	}
	filesNoSecret(t, m)
}

// CMD-12, A5: both-changed copies the live bytes ("live wins") and warns
// about each placeholder of the old repository copy.
func TestAddBothChangedLiveWins(t *testing.T) {
	m := filesSetup(t)
	m.setVar("M", "other-value")
	m.track("H/f.txt", "{{N}} {{M}}\n")
	live := "mine\n"
	writeFile(t, filesLive(m, "f.txt"), live)
	if s := filesStatus(t, m, "H/f.txt"); s != core.BothChanged {
		t.Fatalf("precondition: status %s", s)
	}

	filesWantCode(t, m, m.run("add", "f.txt"), 0)
	if got := readFile(t, m.repoCopy("H/f.txt")); got != live {
		t.Errorf("repository copy %q", got)
	}
	errText := m.Err.String()
	if strings.Count(errText, "dot: warning: ") != 2 || !strings.Contains(errText, "{{N}}") || !strings.Contains(errText, "{{M}}") {
		t.Errorf("stderr %q", errText)
	}
	if got := filesState(t, m); got != filesStateLine("H/f.txt", live) {
		t.Errorf("state %q", got)
	}
	if s := filesStatus(t, m, "H/f.txt"); s != core.InSync {
		t.Errorf("status %s", s)
	}
	filesNoSecret(t, m)
}

// CMD-12: repo-changed is an error; nothing changes.
func TestAddRepoChanged(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "new {{N}}\n")
	state := filesStateLine("H/f.txt", "old\n")
	filesWriteState(t, m, state)
	writeFile(t, filesLive(m, "f.txt"), "old\n")

	filesWantCode(t, m, m.run("add", "f.txt"), 1)
	if !strings.Contains(m.Err.String(), "dot apply") {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "new {{N}}\n" {
		t.Errorf("repository copy %q", got)
	}
	if got := filesState(t, m); got != state {
		t.Errorf("state %q", got)
	}
}

// CMD-12: an unresolved file is an error with the reasons.
func TestAddUnresolved(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "{{UNDEF}}\n")
	writeFile(t, filesLive(m, "f.txt"), "x\n")
	filesWantCode(t, m, m.run("add", "f.txt"), 1)
	if !strings.Contains(m.Err.String(), "placeholder UNDEF is undefined") {
		t.Errorf("stderr %q", m.Err.String())
	}
}

// filesEditorOut is the environment variable that tells
// TestEditHelperProcess where to record its arguments.
const filesEditorOut = "DOT_FILES_TEST_EDITOR_OUT"

// TestEditHelperProcess is the editor for the edit tests: the test binary,
// started by runEditor, records the appended file path. In a normal test run
// the variable is not set and it does nothing.
func TestEditHelperProcess(t *testing.T) {
	out := os.Getenv(filesEditorOut)
	if out == "" {
		return
	}
	args := os.Args[len(os.Args)-1]
	if err := os.WriteFile(out, []byte(args), 0o644); err != nil {
		t.Fatal(err)
	}
}

// filesEditor sets EDITOR to the test binary and returns the file in which
// the editor records the path that it received.
func filesEditor(t *testing.T, m *machine) string {
	t.Helper()
	if strings.ContainsAny(os.Args[0], " \t") {
		t.Fatalf("the path of the test binary has white space, which CMD-13 splits: %q", os.Args[0])
	}
	out := filepath.Join(t.TempDir(), "editor-args")
	t.Setenv(filesEditorOut, out)
	m.Environ["EDITOR"] = os.Args[0] + " -test.run=^TestEditHelperProcess$"
	return out
}

// CMD-13: edit opens the repository copy of the one match and waits for exit.
func TestEditOneMatch(t *testing.T) {
	m := filesSetup(t)
	m.track("H/sub/f.txt", "x\n")
	out := filesEditor(t, m)
	filesWantCode(t, m, m.run("edit", "f.txt"), 0)
	if got := readFile(t, out); got != m.repoCopy("H/sub/f.txt") {
		t.Errorf("editor got %q, want %q", got, m.repoCopy("H/sub/f.txt"))
	}
}

// CMD-13: the bare form of the test binary as editor runs no test and exits 0.
func TestEditExitZero(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\n")
	if strings.ContainsAny(os.Args[0], " \t") {
		t.Fatalf("the path of the test binary has white space, which CMD-13 splits: %q", os.Args[0])
	}
	m.Environ["EDITOR"] = os.Args[0] + " -test.run=^$"
	filesWantCode(t, m, m.run("edit", "H/f.txt"), 0)
}

// CMD-4: no match is an error.
func TestEditNoMatch(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\n")
	filesEditor(t, m)
	filesWantCode(t, m, m.run("edit", "g.txt"), 1)
	if !strings.Contains(m.Err.String(), "g.txt") {
		t.Errorf("stderr %q", m.Err.String())
	}
}

// CMD-4, CMD-13: more than one match is an error that lists the candidates.
func TestEditTwoMatches(t *testing.T) {
	m := filesSetup(t)
	m.track("H/a/f.txt", "x\n")
	m.track("H/b/f.txt", "y\n")
	out := filesEditor(t, m)
	filesWantCode(t, m, m.run("edit", "f.txt"), 1)
	if !strings.Contains(m.Err.String(), "H/a/f.txt") || !strings.Contains(m.Err.String(), "H/b/f.txt") {
		t.Errorf("stderr %q", m.Err.String())
	}
	if _, err := os.Stat(out); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("editor started: %v", err)
	}
}

// CMD-13: neither EDITOR nor VISUAL is an error.
func TestEditNoEditor(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\n")
	filesWantCode(t, m, m.run("edit", "f.txt"), 1)
	if !strings.Contains(m.Err.String(), "EDITOR") {
		t.Errorf("stderr %q", m.Err.String())
	}
}

// CMD-1: edit takes exactly one argument.
func TestEditUsage(t *testing.T) {
	m := filesSetup(t)
	filesWantCode(t, m, m.run("edit"), 2)
	filesWantCode(t, m, m.run("edit", "a", "b"), 2)
}
