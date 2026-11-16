package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CMD-16, NF-6: meta without an argument shows configuration, variables with
// origin and values, and each tracked file; it writes no state (SYNC-1).
func TestMetaAll(t *testing.T) {
	m := filesSetup(t)
	m.Environ["APPDATA"] = filepath.Join(m.Home, "appdata")
	m.track("H/f.txt", "v={{N}}\n")
	writeFile(t, filesLive(m, "f.txt"), "v="+filesSecret+"\n")
	m.track("X/g.txt", "x\n")

	filesWantCode(t, m, m.run("meta"), 0)
	out := m.Out.String()
	want := []string{
		"machine: a\n",
		"local directory: " + m.Dir + "\n",
		"repository: " + m.Repo + "\n",
		"delimiters: {{ }}\n",
		"registered machines: a\n",
		"variables:\n",
		"  APPDATA = " + filepath.Join(m.Home, "appdata") + " (environment)\n",
		"  H = " + m.Home + " (vars)\n",
		"  N = " + filesSecret + " (vars)\n",
		"files:\n",
		"  in-sync H/f.txt " + filesLive(m, "f.txt") + "\n",
		"  unresolved X/g.txt (root X is undefined)\n",
	}
	pos := 0
	for _, w := range want {
		i := strings.Index(out[pos:], w)
		if i < 0 {
			t.Fatalf("stdout lacks %q after offset %d:\n%s", w, pos, out)
		}
		pos += i + len(w)
	}
	if m.Err.Len() != 0 {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := filesState(t, m); got != "<none>" {
		t.Errorf("meta wrote state (SYNC-1): %q", got)
	}
}

// CMD-16: meta with an argument shows each match with its placeholders and values.
func TestMetaFile(t *testing.T) {
	m := filesSetup(t)
	m.track("H/a/f.txt", "{{N}} {{UNDEF}}\n")
	m.track("H/b/f.txt", "plain\n")
	writeFile(t, filesLive(m, "b/f.txt"), "plain\n")

	filesWantCode(t, m, m.run("meta", "f.txt"), 0)
	want := strings.Join([]string{
		"name: H/a/f.txt",
		"repository path: " + m.repoCopy("H/a/f.txt"),
		"live path: " + filesLive(m, "a/f.txt"),
		"status: unresolved",
		"reason: placeholder UNDEF is undefined",
		"placeholder: N = " + filesSecret + " (vars)",
		"placeholder: UNDEF undefined",
		"",
		"name: H/b/f.txt",
		"repository path: " + m.repoCopy("H/b/f.txt"),
		"live path: " + filesLive(m, "b/f.txt"),
		"status: in-sync",
		"",
	}, "\n")
	if got := m.Out.String(); got != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", got, want)
	}
	if got := filesState(t, m); got != "<none>" {
		t.Errorf("meta wrote state (SYNC-1): %q", got)
	}
}

// CMD-4: no match is an error; CMD-1: more than one argument is a usage error.
func TestMetaErrors(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\n")
	filesWantCode(t, m, m.run("meta", "nothing"), 1)
	filesWantCode(t, m, m.run("meta", "a", "b"), 2)
}

// filesTempDir points the temporary directory of this process and its
// children to a new directory, so that a test can see what diff leaves there.
func filesTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, k := range []string{"TMP", "TEMP", "TMPDIR"} {
		t.Setenv(k, dir)
	}
	return dir
}

// CMD-17, A6, SYNC-1: diff shows each status that is not in-sync or skipped,
// with a unified difference from the rendered repository copy (old side) to
// the live file (new side); it exits 0 and writes no state and no file.
func TestDiffStatuses(t *testing.T) {
	m := filesSetup(t)
	writeFile(t, filepath.Join(m.Dir, "config"), "machine = a\nskip = H/skip.txt\n")
	m.track("H/sync.txt", "same\n")
	writeFile(t, filesLive(m, "sync.txt"), "same\n")
	m.track("H/repo.txt", "line1\nnew\n")
	writeFile(t, filesLive(m, "repo.txt"), "line1\nold\n")
	m.track("H/live.txt", "a\nb\n")
	writeFile(t, filesLive(m, "live.txt"), "a\nb\nextra\n")
	m.track("H/both.txt", "theirs\n")
	writeFile(t, filesLive(m, "both.txt"), "mine\n")
	m.track("H/miss.txt", "x\n")
	m.track("H/skip.txt", "repo\n")
	writeFile(t, filesLive(m, "skip.txt"), "live\n")
	m.track("X/u.txt", "x\n")
	state := filesStateLine("H/live.txt", "a\nb\n") + filesStateLine("H/repo.txt", "line1\nold\n")
	filesWriteState(t, m, state)
	tmp := filesTempDir(t)

	filesWantCode(t, m, m.run("diff"), 0)
	out := m.Out.String()
	want := []string{
		"both-changed H/both.txt\n",
		"--- repository/H/both.txt\n",
		"+++ live/H/both.txt\n",
		"-theirs\n",
		"+mine\n",
		"live-changed H/live.txt\n",
		"--- repository/H/live.txt\n",
		"+++ live/H/live.txt\n",
		"+extra\n",
		"missing H/miss.txt\n",
		"repo-changed H/repo.txt\n",
		"--- repository/H/repo.txt\n",
		"+++ live/H/repo.txt\n",
		"-new\n",
		"+old\n",
		"unresolved X/u.txt: root X is undefined\n",
	}
	pos := 0
	for _, w := range want {
		i := strings.Index(out[pos:], w)
		if i < 0 {
			t.Fatalf("stdout lacks %q after offset %d:\n%s", w, pos, out)
		}
		pos += i + len(w)
	}
	for _, absent := range []string{"sync.txt", "skip.txt", "-b\n", "-a\n"} {
		if strings.Contains(out, absent) {
			t.Errorf("stdout holds %q:\n%s", absent, out)
		}
	}
	if m.Err.Len() != 0 {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := filesState(t, m); got != state {
		t.Errorf("diff wrote state (SYNC-1): %q", got)
	}
	for rel, content := range map[string]string{"repo.txt": "line1\nold\n", "live.txt": "a\nb\nextra\n", "both.txt": "mine\n", "skip.txt": "live\n"} {
		if got := readFile(t, filesLive(m, rel)); got != content {
			t.Errorf("live file %s changed: %q", rel, got)
		}
	}
	if _, err := os.Stat(filesLive(m, "miss.txt")); err == nil {
		t.Error("diff created a missing live file")
	}
	if entries, err := os.ReadDir(tmp); err != nil || len(entries) != 0 {
		t.Errorf("temporary directory not removed: %v %v", entries, err)
	}
}

// CMD-17, SYNC-15: diff of a live-changed file prints the warnings that a
// capture would give, and writes nothing.
func TestDiffLiveChangedDropWarning(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\nv={{N}}\n")
	state := filesStateLine("H/f.txt", "x\nv="+filesSecret+"\n")
	filesWriteState(t, m, state)
	writeFile(t, filesLive(m, "f.txt"), "x\nv=other\n")

	filesWantCode(t, m, m.run("diff", "f.txt"), 0)
	if !strings.HasPrefix(m.Out.String(), "live-changed H/f.txt\n") || !strings.Contains(m.Out.String(), "+v=other\n") {
		t.Errorf("stdout %q", m.Out.String())
	}
	if got := m.Err.String(); got != "dot: warning: H/f.txt: line 2: placeholder {{N}} dropped\n" {
		t.Errorf("stderr %q", got)
	}
	if got := readFile(t, m.repoCopy("H/f.txt")); got != "x\nv={{N}}\n" {
		t.Errorf("repository copy changed: %q", got)
	}
	if got := filesState(t, m); got != state {
		t.Errorf("state changed: %q", got)
	}
}

// CMD-17, SYNC-11, SYNC-16: a capture that would fail gives a warning that names dot edit.
func TestDiffCaptureWouldFail(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "a\n")
	filesWriteState(t, m, filesStateLine("H/f.txt", "a\n"))
	writeFile(t, filesLive(m, "f.txt"), "a\n{{N}}\n")

	filesWantCode(t, m, m.run("diff"), 0)
	if !strings.Contains(m.Err.String(), "capture would fail") || !strings.Contains(m.Err.String(), "dot edit H/f.txt") {
		t.Errorf("stderr %q", m.Err.String())
	}
}

// CMD-17, D8: line ends are bytes; core.autocrlf of the user must not hide a
// difference that is only LF against CRLF.
func TestDiffLineEndsIgnoreAutocrlf(t *testing.T) {
	m := filesSetup(t)
	cfg := os.Getenv("GIT_CONFIG_GLOBAL")
	if cfg == "" {
		t.Fatal("GIT_CONFIG_GLOBAL is not set; isolateGit did not run")
	}
	writeFile(t, cfg, "[core]\n\tautocrlf = true\n")
	m.track("H/g.txt", "a\nb\n")
	filesWriteState(t, m, filesStateLine("H/g.txt", "a\nb\n"))
	writeFile(t, filesLive(m, "g.txt"), "a\r\nb\r\n")

	filesWantCode(t, m, m.run("diff"), 0)
	out := m.Out.String()
	for _, s := range []string{"live-changed H/g.txt\n", "-a\n", "-b\n", "+a\r\n", "+b\r\n"} {
		if !strings.Contains(out, s) {
			t.Errorf("stdout lacks %q:\n%q", s, out)
		}
	}
	if strings.Contains(m.Err.String(), "will be replaced") {
		t.Errorf("stderr has a line-end conversion warning: %q", m.Err.String())
	}
}

// CMD-4, CMD-17: with an argument, only the matches are shown.
func TestDiffSelected(t *testing.T) {
	m := filesSetup(t)
	m.track("H/a.txt", "x\n")
	m.track("H/b.txt", "x\n")
	filesWantCode(t, m, m.run("diff", "a.txt"), 0)
	if got := m.Out.String(); got != "missing H/a.txt\n" {
		t.Errorf("stdout %q", got)
	}
}

// CMD-4: no match is an error; CMD-1: more than one argument is a usage error.
func TestDiffErrors(t *testing.T) {
	m := filesSetup(t)
	m.track("H/f.txt", "x\n")
	filesWantCode(t, m, m.run("diff", "nothing"), 1)
	filesWantCode(t, m, m.run("diff", "a", "b"), 2)
}
