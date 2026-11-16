package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// CMD-18, SYNC-5, SYNC-7: repo-changed is written with the rendered bytes and H is set.
func TestApplyRepoChanged(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "v={{N}}\n")
	filesWriteState(t, m, filesStateLine("H/f.txt", "old\n"))
	writeFile(t, filesLive(m, "f.txt"), "old\n")

	filesWantCode(t, m, m.run("apply"), 0)
	rendered := "v=" + filesSecret + "\n"
	if got := readFile(t, filesLive(m, "f.txt")); got != rendered {
		t.Errorf("live file %q", got)
	}
	if got := m.Out.String(); got != "wrote H/f.txt\n" {
		t.Errorf("stdout %q", got)
	}
	if got := filesState(t, m); got != filesStateLine("H/f.txt", rendered) {
		t.Errorf("state %q", got)
	}
	filesNoSecret(t, m)
}

// CMD-18, SYNC-5, SYNC-6: a missing live file is created with its parent
// directories and the permission bits of a new file.
func TestApplyMissing(t *testing.T) {
	m := filesSetup(t)
	m.track("H/a/b/f.txt", "x\n")
	filesWantCode(t, m, m.run("apply", "f.txt"), 0)
	path := filesLive(m, "a/b/f.txt")
	if got := readFile(t, path); got != "x\n" {
		t.Errorf("live file %q", got)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no permission bits beyond read-only.
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o644 {
		t.Errorf("permission %v, want 0644", st.Mode().Perm())
	}
	if got := filesState(t, m); got != filesStateLine("H/a/b/f.txt", "x\n") {
		t.Errorf("state %q", got)
	}
}

// SYNC-6: a write keeps the permission bits of the existing live file.
func TestApplyKeepsPermissions(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.sh", "new\n")
	filesWriteState(t, m, filesStateLine("H/f.sh", "old\n"))
	path := filesLive(m, "f.sh")
	writeFile(t, path, "old\n")
	if err := os.Chmod(path, 0o750); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	filesWantCode(t, m, m.run("apply"), 0)
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("permission %v, want %v", after.Mode().Perm(), before.Mode().Perm())
	}
	if got := readFile(t, path); got != "new\n" {
		t.Errorf("live file %q", got)
	}
}

// CMD-18, SYNC-2: in-sync writes nothing and sets H = L.
func TestApplyInSync(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "same\n")
	writeFile(t, filesLive(m, "f.txt"), "same\n")
	filesWantCode(t, m, m.run("apply"), 0)
	if m.Out.Len() != 0 || m.Err.Len() != 0 {
		t.Errorf("output: %q %q", m.Out.String(), m.Err.String())
	}
	if got := filesState(t, m); got != filesStateLine("H/f.txt", "same\n") {
		t.Errorf("state %q", got)
	}
}

// CMD-18: live-changed is kept without --force, with a note that push or
// pull captures it; --force writes the repository version.
func TestApplyLiveChanged(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "a\n")
	state := filesStateLine("H/f.txt", "a\n")
	filesWriteState(t, m, state)
	writeFile(t, filesLive(m, "f.txt"), "a\nedit\n")

	filesWantCode(t, m, m.run("apply"), 0)
	if got := readFile(t, filesLive(m, "f.txt")); got != "a\nedit\n" {
		t.Errorf("live file changed: %q", got)
	}
	if out := m.Out.String(); !strings.Contains(out, "H/f.txt") || !strings.Contains(out, "dot push") || !strings.Contains(out, "dot pull") {
		t.Errorf("stdout %q", out)
	}
	if got := filesState(t, m); got != state {
		t.Errorf("state %q", got)
	}

	filesWantCode(t, m, m.run("apply", "--force"), 0)
	if got := readFile(t, filesLive(m, "f.txt")); got != "a\n" {
		t.Errorf("live file after --force %q", got)
	}
	if got := filesState(t, m); got != filesStateLine("H/f.txt", "a\n") {
		t.Errorf("state %q", got)
	}
}

// CMD-18, SYNC-8, E9: a new machine with a different live file and no state
// is both-changed; apply refuses, exits 1, and the live file stays
// byte-identical; --force in either position writes it.
func TestApplyBothChanged(t *testing.T) {
	for _, args := range [][]string{{"apply", "--force", "f.txt"}, {"apply", "f.txt", "--force"}} {
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			m := filesSetup(t)
			m.track("H/f.txt", "theirs {{N}}\n")
			live := "mine\r\n\x00"
			writeFile(t, filesLive(m, "f.txt"), live)

			filesWantCode(t, m, m.run("apply"), 1)
			if got := readFile(t, filesLive(m, "f.txt")); got != live {
				t.Errorf("live file changed: %q", got)
			}
			errText := m.Err.String()
			for _, s := range []string{"H/f.txt", "dot add H/f.txt", "dot apply --force H/f.txt"} {
				if !strings.Contains(errText, s) {
					t.Errorf("stderr lacks %q: %q", s, errText)
				}
			}
			if got := filesState(t, m); got != "" {
				t.Errorf("state %q, want empty", got)
			}
			filesNoSecret(t, m)

			filesWantCode(t, m, m.run(args...), 0)
			rendered := "theirs " + filesSecret + "\n"
			if got := readFile(t, filesLive(m, "f.txt")); got != rendered {
				t.Errorf("live file after --force %q", got)
			}
			if got := filesState(t, m); got != filesStateLine("H/f.txt", rendered) {
				t.Errorf("state %q", got)
			}
		})
	}
}

// CMD-18: unresolved reports the reasons and exits 1.
func TestApplyUnresolved(t *testing.T) {
	m := filesSetup(t)
	m.track("X/f.txt", "x\n")
	filesWantCode(t, m, m.run("apply"), 1)
	if !strings.Contains(m.Err.String(), "X/f.txt") || !strings.Contains(m.Err.String(), "root X is undefined") {
		t.Errorf("stderr %q", m.Err.String())
	}
}

// CMD-18, PATH-4: a skipped file is not written.
func TestApplySkipped(t *testing.T) {
	m := filesSetup(t)
	writeFile(t, filepath.Join(m.Dir, "config"), "machine = a\nskip = H\n")
	m.track("H/f.txt", "x\n")
	filesWantCode(t, m, m.run("apply"), 0)
	if _, err := os.Stat(filesLive(m, "f.txt")); err == nil {
		t.Error("skipped file written")
	}
	if m.Out.Len() != 0 {
		t.Errorf("stdout %q", m.Out.String())
	}
}

// CMD-18: files are independent; a refusal and an unreadable live path do not
// stop the write of another file, the state is saved, and the exit code is 1.
func TestApplyIndependent(t *testing.T) {
	m := filesSetup(t)
	m.track("H/a.txt", "theirs\n")
	writeFile(t, filesLive(m, "a.txt"), "mine\n")
	m.track("H/b.txt", "x\n")
	if err := os.MkdirAll(filesLive(m, "b.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.track("H/c.txt", "new\n")

	filesWantCode(t, m, m.run("apply"), 1)
	if got := readFile(t, filesLive(m, "c.txt")); got != "new\n" {
		t.Errorf("live file c.txt %q", got)
	}
	if got := m.Out.String(); got != "wrote H/c.txt\n" {
		t.Errorf("stdout %q", got)
	}
	if !strings.Contains(m.Err.String(), "H/a.txt") || !strings.Contains(m.Err.String(), "b.txt") {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := readFile(t, filesLive(m, "a.txt")); got != "mine\n" {
		t.Errorf("live file a.txt changed: %q", got)
	}
	if got := filesState(t, m); got != filesStateLine("H/c.txt", "new\n") {
		t.Errorf("state %q", got)
	}
}

// CMD-1, CMD-4: argument errors.
func TestApplyArguments(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\n")
	filesWantCode(t, m, m.run("apply", "a", "b"), 2)
	filesWantCode(t, m, m.run("apply", "--bogus"), 2)
	filesWantCode(t, m, m.run("apply", "nothing"), 1)
	if !strings.Contains(m.Err.String(), "nothing") {
		t.Errorf("stderr %q", m.Err.String())
	}
}
