package core

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// ST-6, A15: regular files below files/<ROOT>/ are tracked; files directly
// below files/ are not; the work tree is read at each call.
func TestFiles_ST6(t *testing.T) {
	f := newFixture(t)
	w := f.open()
	if names, err := w.Files(); err != nil || names != nil {
		t.Fatalf("missing files dir: %v, %v", names, err)
	}
	f.track("VIM/.gvimrc", "a")
	f.track("B/x/y/z.txt", "b")
	f.track("A/one", "c")
	f.track("bad.root/f", "d")
	writeFile(t, f.repo(FilesDir, "toplevel"), "e")
	if err := os.MkdirAll(f.repo(FilesDir, "EMPTY", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, f.repo(RepoConfFile), "")
	names, err := w.Files()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A/one", "B/x/y/z.txt", "VIM/.gvimrc", "bad.root/f"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Files = %v, want %v", names, want)
	}
	f.track("A/two", "f")
	names, _ = w.Files()
	if len(names) != 5 {
		t.Errorf("no cache: Files = %v", names)
	}
}

func TestRepoPath(t *testing.T) {
	f := newFixture(t)
	w := f.open()
	if got, want := w.RepoPath("VIM/a/.gvimrc"), f.repo(FilesDir, "VIM", "a", ".gvimrc"); got != want {
		t.Errorf("RepoPath = %q, want %q", got, want)
	}
}

func TestLivePath_PATH1_PATH2(t *testing.T) {
	f := newFixture(t)
	vim := f.live("vim")
	f.writeVars(
		"VIM = "+filepath.ToSlash(vim)+"/sub/..//", // PATH-2: forward slashes and Clean
		"REL = relative/secret-value",
	)
	w := f.open()
	got, err := w.LivePath("VIM/a/.gvimrc")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(vim, "a", ".gvimrc"); got != want {
		t.Errorf("LivePath = %q, want %q", got, want)
	}
	_, err = w.LivePath("REL/x")
	if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
		t.Errorf("relative root: err = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "secret-value") {
		t.Errorf("NF-6: error shows the value: %v", err)
	}
	if _, err := w.LivePath("NOPE/x"); err == nil || !strings.Contains(err.Error(), "NOPE is undefined") {
		t.Errorf("undefined root: err = %v", err)
	}
	if _, err := w.LivePath("bad.root/x"); err == nil {
		t.Error("A15: invalid root name must fail")
	}
}

func TestIsSkipped_PATH4(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("skip = VIM/colors/", "skip = APP/a.txt", "skip =")
	w := f.open()
	cases := map[string]bool{
		"VIM/colors":       true,
		"VIM/colors/x.vim": true,
		"VIM/colorsx":      false,
		"VIM/.gvimrc":      false,
		"APP/a.txt":        true,
		"APP/a.txt.bak":    false,
		"APP/a.txt/x":      true,
		"X/y":              false,
	}
	for name, want := range cases {
		if got := w.IsSkipped(name); got != want {
			t.Errorf("IsSkipped(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestDerive_PATH3(t *testing.T) {
	f := newFixture(t)
	base := f.live()
	f.writeVars(
		"SHORT = "+base,
		"LONG = "+filepath.Join(base, "a", "b"),
		"TIE_B = "+filepath.Join(base, "t"),
		"TIE_A = "+filepath.Join(base, "t", "."),
		"EXACT = "+filepath.Join(base, "file.txt"),
		"PART = "+filepath.Join(base, "ab"),
		"REL = relative",
	)
	w := f.open()
	cases := []struct{ arg, want string }{
		{filepath.Join(base, "a", "b", "c", "f"), "LONG/c/f"}, // longest value wins
		{filepath.Join(base, "a", "x"), "SHORT/a/x"},
		{filepath.Join(base, "t", "f"), "TIE_A/f"},              // equal length: first name
		{filepath.Join(base, "file.txt"), "SHORT/file.txt"},     // a value equal to the file is not a parent
		{filepath.Join(base, "abc", "f"), "SHORT/abc/f"},        // compare by component
		{filepath.Join("live", "a", "..", "q"), "SHORT/q"},      // relative to Cwd
		{filepath.Join(base, "a", "b", "..", "z"), "SHORT/a/z"}, // cleaned first
	}
	for _, c := range cases {
		got, err := w.Derive(c.arg)
		if err != nil || got != c.want {
			t.Errorf("Derive(%q) = %q, %v; want %q", c.arg, got, err, c.want)
		}
	}
}

func TestDerive_PATH3_NoCandidate(t *testing.T) {
	f := newFixture(t)
	f.writeVars("A = " + f.live("a"))
	w := f.open()
	_, err := w.Derive(filepath.Join(f.root, "other", "f"))
	if err == nil || !strings.Contains(err.Error(), "dot var <NAME> <directory>") {
		t.Errorf("err = %v", err)
	}
}

// PATH-3: case rule. relBelow takes the choice so both run on any platform.
func TestRelBelow_PATH3_Case(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Conf")
	p := filepath.Join(dir, "Sub", "f.txt")
	upper := strings.ToUpper(dir)
	if rel, ok := relBelow(dir, p, false); !ok || rel != "Sub/f.txt" {
		t.Errorf("exact case: %q %v", rel, ok)
	}
	if _, ok := relBelow(upper, p, false); ok {
		t.Error("case-sensitive: other case must not match")
	}
	if rel, ok := relBelow(upper, p, true); !ok || rel != "Sub/f.txt" {
		t.Errorf("case-insensitive: %q %v", rel, ok)
	}
	if _, ok := relBelow(dir, dir, true); ok {
		t.Error("equal path is not below")
	}
	if _, ok := relBelow(dir, dir+"x", true); ok {
		t.Error("name prefix is not a parent")
	}
	vol := filepath.VolumeName(dir) + string(filepath.Separator)
	if _, ok := relBelow(vol, p, false); !ok {
		t.Error("root directory must be a parent")
	}
}

func TestDerive_PATH3_PlatformCase(t *testing.T) {
	f := newFixture(t)
	f.writeVars("UP = " + strings.ToUpper(f.live("conf")))
	w := f.open()
	got, err := w.Derive(f.live("conf", "f"))
	if runtime.GOOS == "windows" {
		if err != nil || got != "UP/f" {
			t.Errorf("Windows compares without case: %q %v", got, err)
		}
	} else if err == nil {
		t.Errorf("case-sensitive platform: got %q", got)
	}
}

func TestMatch_CMD4(t *testing.T) {
	f := newFixture(t)
	f.writeVars("VIM = " + f.live("vim"))
	f.track("VIM/.gvimrc", "a")
	f.track("VIM/colors/.gvimrc", "b")
	f.track("VIM/colors/dark.vim", "c")
	f.track("NOROOT/dark.vim", "d")
	w := f.open()
	cases := []struct {
		arg  string
		want []string
	}{
		{"VIM/.gvimrc", []string{"VIM/.gvimrc"}},                               // equal
		{".gvimrc", []string{"VIM/.gvimrc", "VIM/colors/.gvimrc"}},             // base name
		{"colors/dark.vim", []string{"VIM/colors/dark.vim"}},                   // suffix at a / boundary
		{"dark.vim", []string{"NOROOT/dark.vim", "VIM/colors/dark.vim"}},       // unresolved root still matches by name
		{f.live("vim", "colors", "dark.vim"), []string{"VIM/colors/dark.vim"}}, // absolute live path
		{filepath.Join("live", "vim", ".gvimrc"), []string{"VIM/.gvimrc"}},     // live path relative to Cwd
		{filepath.Join("live", "vim", "..", "vim", ".gvimrc"), []string{"VIM/.gvimrc"}},
	}
	for _, c := range cases {
		got, err := w.Match(c.arg)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Match(%q) = %v, %v; want %v", c.arg, got, err, c.want)
		}
	}
	for _, arg := range []string{"vimrc", "rc", "M/.gvimrc", "nothing"} {
		_, err := w.Match(arg)
		if err == nil || err.Error() != "no tracked file matches: "+arg {
			t.Errorf("Match(%q): err = %v", arg, err)
		}
	}
}

func TestMatch_CMD4_PlatformRules(t *testing.T) {
	f := newFixture(t)
	f.writeVars("VIM = " + f.live("vim"))
	f.track("VIM/colors/dark.vim", "c")
	w := f.open()
	got, err := w.Match(`colors\dark.vim`)
	upper, uerr := w.Match(strings.ToUpper(f.live("vim", "colors", "dark.vim")))
	if runtime.GOOS == "windows" {
		if err != nil || len(got) != 1 {
			t.Errorf(`backslash counts as /: %v %v`, got, err)
		}
		if uerr != nil || len(upper) != 1 {
			t.Errorf("live path compares without case: %v %v", upper, uerr)
		}
	} else {
		if err == nil {
			t.Errorf("backslash is a name character: %v", got)
		}
		if uerr == nil {
			t.Errorf("case-sensitive live path: %v", upper)
		}
	}
}
