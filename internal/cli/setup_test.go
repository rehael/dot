package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dot/internal/core"
)

// setupRepo makes the default repository of m without the init command.
func setupRepo(t *testing.T, m *machine) {
	t.Helper()
	if err := os.MkdirAll(m.Repo, 0o755); err != nil {
		t.Fatal(err)
	}
	m.git("init", "-q")
	writeFile(t, filepath.Join(m.Repo, core.RepoConfFile), "prefix = {{\nsuffix = }}\n")
}

// setupSnapshot returns the content of each file below dir, .git excluded.
func setupSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[path] = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func setupSameSnapshot(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("file count %d, want %d: %v", len(after), len(before), after)
	}
	for path, content := range before {
		if got, ok := after[path]; !ok || got != content {
			t.Errorf("%s changed: %q, want %q", path, got, content)
		}
	}
}

func setupNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s exists (err %v)", path, err)
	}
}

func setupExitCode(t *testing.T, m *machine, want int, args ...string) {
	t.Helper()
	if code := m.run(args...); code != want {
		t.Fatalf("dot %v: exit code %d, want %d; stdout %q; stderr %q", args, code, want, m.Out.String(), m.Err.String())
	}
}

// CMD-10: init with no argument creates the default repository, makes no
// commit and writes no repo line to config.
func TestInitCreatesDefault(t *testing.T) {
	m := newMachine(t)
	setupExitCode(t, m, 0, "init")
	if got := readFile(t, filepath.Join(m.Repo, ".gitattributes")); got != "* -text\n" {
		t.Errorf(".gitattributes = %q", got)
	}
	if got := readFile(t, filepath.Join(m.Repo, core.RepoConfFile)); got != "prefix = {{\nsuffix = }}\n" {
		t.Errorf("dot.conf = %q", got)
	}
	if _, err := os.Stat(filepath.Join(m.Repo, ".git")); err != nil {
		t.Errorf("no git work tree: %v", err)
	}
	if _, err := m.gitTry("rev-parse", "--verify", "-q", "HEAD"); err == nil {
		t.Error("init made a commit")
	}
	setupNotExist(t, filepath.Join(m.Dir, core.ConfigFile))
	if out := m.Out.String(); !strings.Contains(out, "created") || !strings.Contains(out, m.Repo) {
		t.Errorf("stdout %q", out)
	}
}

// CMD-10: init with a relative path resolves it against the current
// directory and writes repo to config.
func TestInitPathWritesRepo(t *testing.T) {
	m := newMachine(t)
	setupExitCode(t, m, 0, "init", "myrepo")
	target := filepath.Join(m.Home, "myrepo")
	if got := readFile(t, filepath.Join(m.Dir, core.ConfigFile)); got != "repo = "+target+"\n" {
		t.Errorf("config = %q", got)
	}
	if got := readFile(t, filepath.Join(target, core.RepoConfFile)); got != "prefix = {{\nsuffix = }}\n" {
		t.Errorf("dot.conf = %q", got)
	}
	setupNotExist(t, m.Repo)
}

// CMD-10: init of the default location given as a path writes no repo line.
func TestInitPathDefaultLocation(t *testing.T) {
	m := newMachine(t)
	setupExitCode(t, m, 0, "init", m.Repo)
	setupNotExist(t, filepath.Join(m.Dir, core.ConfigFile))
}

// CMD-10: a work tree with dot.conf is adopted byte for byte.
func TestInitAdoptsUnchanged(t *testing.T) {
	m := newMachine(t)
	if err := os.MkdirAll(m.Repo, 0o755); err != nil {
		t.Fatal(err)
	}
	m.git("init", "-q")
	writeFile(t, filepath.Join(m.Repo, core.RepoConfFile), "prefix=<%\nsuffix=%>\nmachine = a")
	writeFile(t, filepath.Join(m.Repo, "files", "VIM", ".gvimrc"), "x\r\n")
	before := setupSnapshot(t, m.Repo)
	setupExitCode(t, m, 0, "init")
	setupSameSnapshot(t, before, setupSnapshot(t, m.Repo))
	if out := m.Out.String(); !strings.Contains(out, "adopted") || !strings.Contains(out, m.Repo) {
		t.Errorf("stdout %q", out)
	}
	setupNotExist(t, filepath.Join(m.Dir, core.ConfigFile))
}

// CMD-10: a work tree without dot.conf gets dot.conf and the attribute line;
// other attribute lines stay.
func TestInitWorkTreeWithoutConf(t *testing.T) {
	cases := []struct{ name, attr, want string }{
		{"no attributes", "", "* -text\n"},
		{"other line, no final LF", "*.png binary", "*.png binary\n* -text\n"},
		{"line present", "*.png binary\n* -text\n", "*.png binary\n* -text\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMachine(t)
			if err := os.MkdirAll(m.Repo, 0o755); err != nil {
				t.Fatal(err)
			}
			m.git("init", "-q")
			attrPath := filepath.Join(m.Repo, ".gitattributes")
			if tc.attr != "" {
				writeFile(t, attrPath, tc.attr)
			}
			setupExitCode(t, m, 0, "init")
			if got := readFile(t, attrPath); got != tc.want {
				t.Errorf(".gitattributes = %q, want %q", got, tc.want)
			}
			if got := readFile(t, filepath.Join(m.Repo, core.RepoConfFile)); got != "prefix = {{\nsuffix = }}\n" {
				t.Errorf("dot.conf = %q", got)
			}
			if _, err := m.gitTry("rev-parse", "--verify", "-q", "HEAD"); err == nil {
				t.Error("init made a commit")
			}
		})
	}
}

// CMD-10: config that points to a different repository is an error.
func TestInitConfigPointsElsewhere(t *testing.T) {
	m := newMachine(t)
	other := filepath.Join(t.TempDir(), "other")
	configPath := filepath.Join(m.Dir, core.ConfigFile)
	writeFile(t, configPath, "repo = "+other+"\n")
	setupExitCode(t, m, 1, "init", m.Repo)
	if !strings.HasPrefix(m.Err.String(), "dot: ") || !strings.Contains(m.Err.String(), "different repository") {
		t.Errorf("stderr %q", m.Err.String())
	}
	setupNotExist(t, m.Repo)
	if got := readFile(t, configPath); got != "repo = "+other+"\n" {
		t.Errorf("config = %q", got)
	}
}

// CMD-10: config that already points to the target is not a conflict and
// stays unchanged; Windows compares paths without case.
func TestInitConfigPointsToTarget(t *testing.T) {
	m := newMachine(t)
	target := filepath.Join(t.TempDir(), "repo")
	configPath := filepath.Join(m.Dir, core.ConfigFile)
	writeFile(t, configPath, "machine = a\nrepo = "+target+"\n")
	arg := target + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		arg = strings.ToUpper(arg)
	}
	setupExitCode(t, m, 0, "init", arg)
	if got := readFile(t, configPath); got != "machine = a\nrepo = "+target+"\n" {
		t.Errorf("config = %q", got)
	}
	setupExitCode(t, m, 0, "init")
	if !strings.Contains(m.Out.String(), "adopted") {
		t.Errorf("stdout %q", m.Out.String())
	}
}

// CMD-1: init takes at most one argument.
func TestInitUsage(t *testing.T) {
	m := newMachine(t)
	setupExitCode(t, m, 2, "init", "a", "b")
	setupNotExist(t, m.Dir)
}

// CMD-11: the default name is the host name in lower case with each
// character outside the name pattern (VAR-1) replaced with "-".
func TestRegisterMachineName(t *testing.T) {
	cases := map[string]string{
		"DESKTOP-AB12":      "desktop-ab12",
		"host.example.com":  "host-example-com",
		"my_host":           "my_host",
		"Zürich PC":         "z-rich-pc",
		"a\tb":              "a-b",
		"ALREADY-lower-123": "already-lower-123",
	}
	for host, want := range cases {
		if got := setupMachineName(host); got != want {
			t.Errorf("setupMachineName(%q) = %q, want %q", host, got, want)
		}
	}
}

// CMD-11: register writes config and dot.conf; a second run changes no byte.
func TestRegisterWritesBothIdempotent(t *testing.T) {
	m := newMachine(t)
	setupRepo(t, m)
	setupExitCode(t, m, 0, "register", "laptop")
	configPath := filepath.Join(m.Dir, core.ConfigFile)
	confPath := filepath.Join(m.Repo, core.RepoConfFile)
	if got := readFile(t, configPath); got != "machine = laptop\n" {
		t.Errorf("config = %q", got)
	}
	if got := readFile(t, confPath); got != "prefix = {{\nsuffix = }}\nmachine = laptop\n" {
		t.Errorf("dot.conf = %q", got)
	}
	if !strings.Contains(m.Out.String(), "laptop") {
		t.Errorf("stdout %q", m.Out.String())
	}
	if _, err := m.gitTry("rev-parse", "--verify", "-q", "HEAD"); err == nil {
		t.Error("register made a commit")
	}
	// A hand-written line form must also survive a repeated run unchanged.
	writeFile(t, configPath, "# mine\nmachine=laptop")
	setupExitCode(t, m, 0, "register", "laptop")
	if got := readFile(t, configPath); got != "# mine\nmachine=laptop" {
		t.Errorf("config changed on a repeated run: %q", got)
	}
	if got := readFile(t, confPath); got != "prefix = {{\nsuffix = }}\nmachine = laptop\n" {
		t.Errorf("dot.conf changed on a repeated run: %q", got)
	}
	// The registration gate (CMD-3) now passes.
	setupExitCode(t, m, 0, "check")
}

// CMD-11: a second machine adds its line after the lines of the others.
func TestRegisterSecondMachine(t *testing.T) {
	m := newMachine(t)
	setupRepo(t, m)
	confPath := filepath.Join(m.Repo, core.RepoConfFile)
	writeFile(t, confPath, "prefix = {{\r\nsuffix = }}\r\nmachine = a\r\n")
	setupExitCode(t, m, 0, "register", "b")
	if got := readFile(t, confPath); got != "prefix = {{\r\nsuffix = }}\r\nmachine = a\r\nmachine = b\n" {
		t.Errorf("dot.conf = %q", got)
	}
}

// CMD-11: with no argument the name comes from the host name.
func TestRegisterDefaultName(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	want := setupMachineName(host)
	m := newMachine(t)
	setupRepo(t, m)
	setupExitCode(t, m, 0, "register")
	if got := readFile(t, filepath.Join(m.Dir, core.ConfigFile)); got != "machine = "+want+"\n" {
		t.Errorf("config = %q, want machine %q", got, want)
	}
}

// CMD-11: config with a different machine name is an error; nothing changes.
func TestRegisterDifferentName(t *testing.T) {
	m := newMachine(t)
	setupRepo(t, m)
	configPath := filepath.Join(m.Dir, core.ConfigFile)
	writeFile(t, configPath, "machine = a\n")
	before := readFile(t, filepath.Join(m.Repo, core.RepoConfFile))
	setupExitCode(t, m, 1, "register", "b")
	if !strings.HasPrefix(m.Err.String(), "dot: ") {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := readFile(t, configPath); got != "machine = a\n" {
		t.Errorf("config = %q", got)
	}
	if got := readFile(t, filepath.Join(m.Repo, core.RepoConfFile)); got != before {
		t.Errorf("dot.conf = %q", got)
	}
}

// CMD-11: without dot.conf there is no repository to register in.
func TestRegisterNoRepository(t *testing.T) {
	m := newMachine(t)
	setupExitCode(t, m, 1, "register", "a")
	if got := m.Err.String(); got != "dot: no repository; run: dot init\n" {
		t.Errorf("stderr %q", got)
	}
	setupNotExist(t, filepath.Join(m.Dir, core.ConfigFile))
}

// CMD-11, VAR-1: a name outside the name pattern is an error.
func TestRegisterInvalidName(t *testing.T) {
	m := newMachine(t)
	setupRepo(t, m)
	setupExitCode(t, m, 1, "register", "a.b")
	setupNotExist(t, filepath.Join(m.Dir, core.ConfigFile))
	setupExitCode(t, m, 2, "register", "a", "b")
}
