// Package e2e runs whole user flows of dot on two simulated machines that
// share one bare repository (TEST-5).
package e2e

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"dot/internal/cli"
	"dot/internal/core"
	"dot/internal/gitx"
	"dot/internal/kv"
)

// rootVar is the root variable of most tracked files. Each machine gives it a
// different directory, so each round trip also checks PATH-1.
const rootVar = "CFG"

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
	// A merge by git pull must not wait for an editor.
	t.Setenv("GIT_MERGE_AUTOEDIT", "no")
}

// machine is one simulated machine. All its directories are below
// t.TempDir(), and its environment holds DOT_DIR only.
type machine struct {
	t    *testing.T
	Name string
	Root string
	Dir  string // local directory (DOT_DIR)
	Repo string
	Live string // value of rootVar
	// Environ is the only environment that dot sees.
	Environ map[string]string
	// vars records the values set with dot var, to find live paths.
	vars map[string]string
	Out  bytes.Buffer
	Err  bytes.Buffer
}

func newMachine(t *testing.T, name string) *machine {
	t.Helper()
	r := t.TempDir()
	m := &machine{
		t:    t,
		Name: name,
		Root: r,
		Dir:  filepath.Join(r, "dot"),
		Live: filepath.Join(r, "live"),
		vars: map[string]string{},
	}
	m.Repo = filepath.Join(m.Dir, core.DefaultRepo)
	m.Environ = map[string]string{"DOT_DIR": m.Dir}
	if err := os.MkdirAll(m.Live, 0o755); err != nil {
		t.Fatal(err)
	}
	return m
}

// run executes one dot command in-process. Out and Err hold its output only.
func (m *machine) run(args ...string) int {
	m.Out.Reset()
	m.Err.Reset()
	return cli.Run(args, core.Env{
		Getenv: func(key string) string { return m.Environ[key] },
		Cwd:    m.Root,
		Stdin:  strings.NewReader(""),
		Stdout: &m.Out,
		Stderr: &m.Err,
	})
}

// want runs a dot command and stops the test if the exit code differs.
func (m *machine) want(code int, args ...string) {
	m.t.Helper()
	if got := m.run(args...); got != code {
		m.t.Fatalf("machine %s: dot %s: exit %d, want %d\nstdout:\n%s\nstderr:\n%s",
			m.Name, strings.Join(args, " "), got, code, m.Out.String(), m.Err.String())
	}
}

func (m *machine) ok(args ...string) {
	m.t.Helper()
	m.want(0, args...)
}

func (m *machine) stdout() string { return m.Out.String() }
func (m *machine) stderr() string { return m.Err.String() }

// git runs git in the repository of the machine and returns its standard output.
func (m *machine) git(args ...string) string {
	m.t.Helper()
	return gitIn(m.t, m.Repo, args...)
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitx.Git{Dir: dir}.Output(args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func (m *machine) head() string {
	m.t.Helper()
	return strings.TrimSpace(m.git("rev-parse", "HEAD"))
}

// setVar defines a variable with the real command (CMD-14).
func (m *machine) setVar(name, value string) {
	m.t.Helper()
	m.ok("var", name, value)
	m.vars[name] = value
}

func (m *machine) setVars(vars map[string]string) {
	m.t.Helper()
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		m.setVar(k, vars[k])
	}
}

// live returns the live path of a logical name on this machine (PATH-1).
func (m *machine) live(name string) string {
	m.t.Helper()
	r, rel, ok := strings.Cut(name, "/")
	v, defined := m.vars[r]
	if !ok || !defined {
		m.t.Fatalf("machine %s: no root for %s", m.Name, name)
	}
	return filepath.Join(v, filepath.FromSlash(rel))
}

func (m *machine) repoCopy(name string) string {
	return filepath.Join(m.Repo, core.FilesDir, filepath.FromSlash(name))
}

func (m *machine) writeLive(name, content string) {
	m.t.Helper()
	writeFile(m.t, m.live(name), content)
}

func (m *machine) readLive(name string) string {
	m.t.Helper()
	return readFile(m.t, m.live(name))
}

func (m *machine) readRepo(name string) string {
	m.t.Helper()
	return readFile(m.t, m.repoCopy(name))
}

// editRepo stands for dot edit: the user changes the repository copy.
func (m *machine) editRepo(name, content string) {
	m.t.Helper()
	writeFile(m.t, m.repoCopy(name), content)
}

// addConfig appends a line to the local config; dot has no command for skip.
func (m *machine) addConfig(key, value string) {
	m.t.Helper()
	path := filepath.Join(m.Dir, core.ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		m.t.Fatal(err)
	}
	writeFile(m.t, path, string(kv.Append(data, key, value)))
}

// fileSpec is a file that machine A tracks before machine B exists.
type fileSpec struct {
	name string // logical name
	live string // live content on A
	repo string // repository copy after the edit of the user; empty: no edit
}

// pair is two machines with clones of one bare repository.
type pair struct {
	t    *testing.T
	Bare string
	A, B *machine
	// Output of the first pull and apply on B.
	pullB, applyB string
}

// newOrigin builds machine A with the real commands: init, register, var,
// add, the edit of the user, a git remote and push.
func newOrigin(t *testing.T, varsA map[string]string, files ...fileSpec) *pair {
	t.Helper()
	isolateGit(t)
	p := &pair{t: t, Bare: filepath.Join(t.TempDir(), "remote.git")}
	gitIn(t, filepath.Dir(p.Bare), "init", "-q", "--bare", p.Bare)

	a := newMachine(t, "a")
	p.A = a
	a.ok("init")
	wantContains(t, "init on A", a.stdout(), "created repository")
	a.ok("register", "a")
	a.git("config", "pull.rebase", "false")
	a.setVar(rootVar, a.Live)
	a.setVars(varsA)
	for _, f := range files {
		a.writeLive(f.name, f.live)
		a.ok("add", a.live(f.name))
		wantContains(t, "add on A", a.stdout(), "added "+f.name)
		if f.repo != "" {
			a.editRepo(f.name, f.repo)
		}
	}
	a.git("remote", "add", "origin", p.Bare)
	a.ok("push")
	return p
}

// clone builds one more machine from the bare repository: git clone, init
// (adopts), register, var. It does not pull or apply.
func (p *pair) clone(name string, vars map[string]string) *machine {
	p.t.Helper()
	m := newMachine(p.t, name)
	gitIn(p.t, m.Root, "clone", "-q", p.Bare, m.Repo)
	m.ok("init")
	wantContains(p.t, "init on "+name, m.stdout(), "adopted repository")
	m.ok("register", name)
	m.git("config", "pull.rebase", "false")
	m.setVar(rootVar, m.Live)
	m.setVars(vars)
	return m
}

// syncB runs the first session of B (pull, apply, push of its machine line)
// and then pull on A, so that both clones end at the same commit.
func (p *pair) syncB() {
	p.t.Helper()
	p.B.ok("pull")
	p.pullB = p.B.stdout()
	p.B.ok("apply")
	p.applyB = p.B.stdout()
	p.B.ok("push")
	p.A.ok("pull")
	if a, b := p.A.head(), p.B.head(); a != b {
		p.t.Fatalf("after the first sync: HEAD of A %s, HEAD of B %s", a, b)
	}
}

func newPair(t *testing.T, varsA, varsB map[string]string, files ...fileSpec) *pair {
	t.Helper()
	p := newOrigin(t, varsA, files...)
	p.B = p.clone("b", varsB)
	p.syncB()
	return p
}

// cfg returns a logical name below rootVar.
func cfg(rel string) string { return rootVar + "/" + rel }

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

// wantBytes compares as strings so that a mismatch shows each byte (%q).
func wantBytes(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

func wantContains(t *testing.T, what, got, sub string) {
	t.Helper()
	if !strings.Contains(got, sub) {
		t.Errorf("%s: %q does not contain %q", what, got, sub)
	}
}

func wantNotContains(t *testing.T, what, got, sub string) {
	t.Helper()
	if strings.Contains(got, sub) {
		t.Errorf("%s: %q contains %q", what, got, sub)
	}
}
