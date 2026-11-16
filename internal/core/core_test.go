package core

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixture is one machine inside t.TempDir (TEST-1): DOT_DIR, repository and
// live files all live below root.
type fixture struct {
	t    *testing.T
	root string
	env  map[string]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	return &fixture{t: t, root: root, env: map[string]string{"DOT_DIR": filepath.Join(root, "local")}}
}

func (f *fixture) Env() Env {
	return Env{
		Getenv: func(k string) string { return f.env[k] },
		Cwd:    f.root,
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
}

func (f *fixture) local(parts ...string) string {
	return filepath.Join(append([]string{f.root, "local"}, parts...)...)
}

func (f *fixture) repo(parts ...string) string {
	return f.local(append([]string{DefaultRepo}, parts...)...)
}

func (f *fixture) live(parts ...string) string {
	return filepath.Join(append([]string{f.root, "live"}, parts...)...)
}

func (f *fixture) open() *Workspace {
	f.t.Helper()
	w, err := Open(f.Env())
	if err != nil {
		f.t.Fatalf("Open: %v", err)
	}
	return w
}

func (f *fixture) writeVars(lines ...string) {
	f.t.Helper()
	writeFile(f.t, f.local(VarsFile), strings.Join(lines, "\n")+"\n")
}

func (f *fixture) writeConfig(lines ...string) {
	f.t.Helper()
	writeFile(f.t, f.local(ConfigFile), strings.Join(lines, "\n")+"\n")
}

func (f *fixture) track(name, content string) {
	f.t.Helper()
	writeFile(f.t, f.repo(FilesDir, filepath.FromSlash(name)), content)
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
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func noHome() (string, error) { return "", errors.New("home must not be used") }

func TestLocalDir_ST1(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	homeFn := func() (string, error) { return home, nil }
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"step 1 DOT_DIR absolute", map[string]string{"DOT_DIR": filepath.Join(root, "d"), "XDG_CONFIG_HOME": filepath.Join(root, "x")}, filepath.Join(root, "d")},
		{"step 1 DOT_DIR relative to Cwd", map[string]string{"DOT_DIR": filepath.Join("sub", "..", "d2")}, filepath.Join(root, "d2")},
		{"step 2 XDG_CONFIG_HOME absolute", map[string]string{"XDG_CONFIG_HOME": filepath.Join(root, "x")}, filepath.Join(root, "x", "dot")},
		{"step 2 skipped for relative XDG_CONFIG_HOME", map[string]string{"XDG_CONFIG_HOME": "rel"}, filepath.Join(home, ".config", "dot")},
		{"step 3 home", map[string]string{}, filepath.Join(home, ".config", "dot")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := Env{Getenv: func(k string) string { return c.env[k] }, Cwd: root}
			w, err := open(env, homeFn)
			if err != nil {
				t.Fatal(err)
			}
			if w.LocalDir != c.want {
				t.Errorf("LocalDir = %q, want %q", w.LocalDir, c.want)
			}
		})
	}
}

func TestLocalDir_ST1_HomeError(t *testing.T) {
	env := Env{Getenv: func(string) string { return "" }, Cwd: t.TempDir()}
	if _, err := open(env, noHome); err == nil {
		t.Fatal("want error when no step gives a directory")
	}
}

// ST-3: nothing exists; Open reads all as empty and creates nothing.
func TestOpen_ST3_Empty(t *testing.T) {
	f := newFixture(t)
	w := f.open()
	if w.LocalDir != f.local() || w.RepoDir != f.repo() {
		t.Errorf("dirs = %q, %q", w.LocalDir, w.RepoDir)
	}
	if w.Machine != "" || len(w.Machines) != 0 || len(w.Skip) != 0 || len(w.Vars()) != 0 {
		t.Errorf("want empty workspace, got %+v", w)
	}
	if w.Delims.Prefix != "{{" || w.Delims.Suffix != "}}" {
		t.Errorf("Delims = %+v", w.Delims)
	}
	names, err := w.Files()
	if err != nil || len(names) != 0 {
		t.Errorf("Files = %v, %v", names, err)
	}
	if _, err := os.Stat(f.local()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Open created the local directory: %v", err)
	}
}

// ST-2: config keys machine, repo, skip.
func TestOpen_ST2_Config(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "elsewhere", "..", "myrepo")
	f.writeConfig("# comment", "machine = alpha", "repo = "+repo, "skip = A/b", "skip = C/")
	w := f.open()
	if w.Machine != "alpha" {
		t.Errorf("Machine = %q", w.Machine)
	}
	if w.RepoDir != filepath.Join(f.root, "myrepo") {
		t.Errorf("RepoDir = %q", w.RepoDir)
	}
	if !reflect.DeepEqual(w.Skip, []string{"A/b", "C/"}) {
		t.Errorf("Skip = %v", w.Skip)
	}
}

// ST-4: repo must be absolute; default is <local>/repo.
func TestOpen_ST4_Repo(t *testing.T) {
	f := newFixture(t)
	if w := f.open(); w.RepoDir != filepath.Join(f.local(), DefaultRepo) {
		t.Errorf("default RepoDir = %q", w.RepoDir)
	}
	f.writeConfig("repo = relative/repo")
	if _, err := Open(f.Env()); err == nil {
		t.Error("want error for a relative repo")
	}
	f.writeConfig("repo =")
	if _, err := Open(f.Env()); err == nil {
		t.Error("want error for an empty repo")
	}
}

// ST-5: delimiters and machines come from dot.conf.
func TestOpen_ST5_RepoConf(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.repo(RepoConfFile), "prefix = <%\r\nsuffix = %>\r\nmachine = alpha\r\nmachine = beta\r\n")
	w := f.open()
	if w.Delims.Prefix != "<%" || w.Delims.Suffix != "%>" {
		t.Errorf("Delims = %+v", w.Delims)
	}
	if !reflect.DeepEqual(w.Machines, []string{"alpha", "beta"}) {
		t.Errorf("Machines = %v", w.Machines)
	}

	writeFile(t, f.repo(RepoConfFile), "suffix = ]]\n")
	w = f.open()
	if w.Delims.Prefix != "{{" || w.Delims.Suffix != "]]" {
		t.Errorf("missing prefix key: Delims = %+v", w.Delims)
	}

	writeFile(t, f.repo(RepoConfFile), "prefix = {a\n")
	if _, err := Open(f.Env()); err == nil {
		t.Error("want error for an invalid prefix")
	}
	writeFile(t, f.repo(RepoConfFile), "prefix =\n")
	if _, err := Open(f.Env()); err == nil {
		t.Error("want error for an empty prefix")
	}
}

// CMD-3: registered needs a machine in config and the same name in dot.conf.
func TestRegistered_CMD3(t *testing.T) {
	f := newFixture(t)
	if f.open().Registered() {
		t.Error("no machine: want not registered")
	}
	f.writeConfig("machine = alpha")
	if f.open().Registered() {
		t.Error("no dot.conf line: want not registered")
	}
	writeFile(t, f.repo(RepoConfFile), "machine = beta\n")
	if f.open().Registered() {
		t.Error("other machine in dot.conf: want not registered")
	}
	writeFile(t, f.repo(RepoConfFile), "machine = beta\nmachine = alpha\n")
	if !f.open().Registered() {
		t.Error("want registered")
	}
}

func TestGit(t *testing.T) {
	f := newFixture(t)
	var out, errOut strings.Builder
	env := f.Env()
	env.Stdout, env.Stderr = &out, &errOut
	w, err := Open(env)
	if err != nil {
		t.Fatal(err)
	}
	g := w.Git()
	if g.Dir != w.RepoDir || g.Stdout != env.Stdout || g.Stderr != env.Stderr || g.Env != nil {
		t.Errorf("Git = %+v", g)
	}
}
