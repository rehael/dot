package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dot/internal/core"
	"dot/internal/gitx"
	"dot/internal/kv"
)

// isolateGit keeps git away from the configuration of the real user (TEST-1).
func isolateGit(t *testing.T) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	writeFile(t, cfg, "")
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("GIT_AUTHOR_NAME", "Test User")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test User")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

// machine is one simulated machine: a local directory, a repository and a
// directory for live files, all below t.TempDir().
type machine struct {
	t    *testing.T
	Dir  string
	Repo string
	Home string
	// Environ is the only environment that dot sees.
	Environ map[string]string
	Out     bytes.Buffer
	Err     bytes.Buffer
}

// newMachine returns a machine with no local directory and no repository yet.
func newMachine(t *testing.T) *machine {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	m := &machine{
		t:    t,
		Dir:  filepath.Join(root, "dot"),
		Home: filepath.Join(root, "home"),
	}
	m.Repo = filepath.Join(m.Dir, core.DefaultRepo)
	m.Environ = map[string]string{"DOT_DIR": m.Dir}
	if err := os.MkdirAll(m.Home, 0o755); err != nil {
		t.Fatal(err)
	}
	return m
}

// newRegistered returns a machine with an initialized repository and a
// registered name. It writes the files directly, so that a test of one
// command does not depend on init and register.
func newRegistered(t *testing.T, name string) *machine {
	t.Helper()
	m := newMachine(t)
	if err := os.MkdirAll(m.Repo, 0o755); err != nil {
		t.Fatal(err)
	}
	m.git("init", "-q")
	writeFile(t, filepath.Join(m.Repo, ".gitattributes"), "* -text\n")
	writeFile(t, filepath.Join(m.Repo, core.RepoConfFile), "prefix = {{\nsuffix = }}\nmachine = "+name+"\n")
	writeFile(t, filepath.Join(m.Dir, core.ConfigFile), "machine = "+name+"\n")
	return m
}

func (m *machine) env() core.Env {
	return core.Env{
		Getenv: func(key string) string { return m.Environ[key] },
		Cwd:    m.Home,
		Stdin:  strings.NewReader(""),
		Stdout: &m.Out,
		Stderr: &m.Err,
	}
}

// run executes one dot command in-process. Out and Err hold its output only.
func (m *machine) run(args ...string) int {
	m.t.Helper()
	m.Out.Reset()
	m.Err.Reset()
	return Run(args, m.env())
}

// git runs git in the repository and returns its standard output.
func (m *machine) git(args ...string) string {
	m.t.Helper()
	out, err := m.gitTry(args...)
	if err != nil {
		m.t.Fatalf("git %v: %v", args, err)
	}
	return out
}

// gitTry is git for a command that can fail.
func (m *machine) gitTry(args ...string) (string, error) {
	return gitx.Git{Dir: m.Repo}.Output(args...)
}

func (m *machine) setVar(name, value string) {
	m.t.Helper()
	path := filepath.Join(m.Dir, core.VarsFile)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		m.t.Fatal(err)
	}
	writeFile(m.t, path, string(kv.Set(data, name, value)))
}

// repoCopy returns the path of the repository copy of a logical name.
func (m *machine) repoCopy(name string) string {
	return filepath.Join(m.Repo, core.FilesDir, filepath.FromSlash(name))
}

// track writes a repository copy directly and stages it.
func (m *machine) track(name, content string) {
	m.t.Helper()
	writeFile(m.t, m.repoCopy(name), content)
	m.git("add", "-A")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
