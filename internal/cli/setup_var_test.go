package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dot/internal/core"
)

// setupEditorOut names the file to which TestVarEditorHelper writes its
// arguments; the helper does nothing when it is not set.
const setupEditorOut = "DOT_SETUP_EDITOR_OUT"

// TestVarEditorHelper is the editor of the var tests: the test binary runs
// it as a subprocess and it records the file path that it received.
func TestVarEditorHelper(t *testing.T) {
	out := os.Getenv(setupEditorOut)
	if out == "" {
		return
	}
	args := os.Args
	if err := os.WriteFile(out, []byte(args[len(args)-1]), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupEditor sets EDITOR to the test binary, so that no real editor starts,
// and returns the file that receives the path the editor got.
func setupEditor(t *testing.T, m *machine) string {
	t.Helper()
	if strings.ContainsAny(os.Args[0], " \t") {
		t.Fatalf("the test binary path has white space, EDITOR cannot hold it: %s", os.Args[0])
	}
	out := filepath.Join(t.TempDir(), "editor-arg")
	t.Setenv(setupEditorOut, out)
	m.Environ["EDITOR"] = os.Args[0] + " -test.run=^TestVarEditorHelper$"
	return out
}

// CMD-14: var with no argument appends one stub for each undefined root and
// placeholder name, once, and opens vars in the editor.
func TestVarNoArgumentStubs(t *testing.T) {
	m := newRegistered(t, "a")
	editorOut := setupEditor(t, m)
	m.Environ["LOCALAPPDATA"] = m.Home
	writeFile(t, filepath.Join(m.Dir, core.ConfigFile), "machine = a\nskip = SKIPPED\n")
	m.track("VIM/.gvimrc", "set font={{FONT}} color={{COLOR}}\n")
	m.track("HOME2/x", "{{FONT}} {{DEF}}\n")
	m.track("LOCALAPPDATA/y", "plain\n")
	m.track("DEF/z", "{{ENVNAME}}\n")
	m.track("SKIPPED/s", "{{SECRET}}\n")
	m.track("bad.root/f", "plain\n")
	varsPath := filepath.Join(m.Dir, core.VarsFile)
	original := "# my comment\nDEF = " + m.Home + "\n# COLOR =\nENVNAME = e"
	writeFile(t, varsPath, original)

	setupExitCode(t, m, 0, "var")
	want := original + "\n" +
		"# used by: HOME2/x, VIM/.gvimrc\n# FONT =\n" +
		"# used by: HOME2/x\n# HOME2 =\n" +
		"# used by: VIM/.gvimrc\n# VIM =\n"
	if got := readFile(t, varsPath); got != want {
		t.Errorf("vars =\n%q\nwant\n%q", got, want)
	}
	if got := readFile(t, editorOut); got != varsPath {
		t.Errorf("editor got %q, want %q", got, varsPath)
	}

	setupExitCode(t, m, 0, "var")
	if got := readFile(t, varsPath); got != want {
		t.Errorf("second run changed vars:\n%q", got)
	}
}

// CMD-14: var with no argument creates an empty vars file and the local
// directory when nothing is undefined.
func TestVarNoArgumentCreatesFile(t *testing.T) {
	m := newRegistered(t, "a")
	editorOut := setupEditor(t, m)
	varsPath := filepath.Join(m.Dir, core.VarsFile)
	setupExitCode(t, m, 0, "var")
	if got := readFile(t, varsPath); got != "" {
		t.Errorf("vars = %q", got)
	}
	if got := readFile(t, editorOut); got != varsPath {
		t.Errorf("editor got %q", got)
	}
}

// CMD-13, CMD-14: no editor is an error after the stubs are written.
func TestVarNoEditor(t *testing.T) {
	m := newRegistered(t, "a")
	m.track("VIM/x", "plain\n")
	setupExitCode(t, m, 1, "var")
	if !strings.Contains(m.Err.String(), "EDITOR") {
		t.Errorf("stderr %q", m.Err.String())
	}
	if got := readFile(t, filepath.Join(m.Dir, core.VarsFile)); got != "# used by: VIM/x\n# VIM =\n" {
		t.Errorf("vars = %q", got)
	}
}

// CMD-14, NF-6: var <name> prints the value and its origin.
func TestVarShow(t *testing.T) {
	m := newRegistered(t, "a")
	m.setVar("FOO", "bar baz")
	m.Environ["USERPROFILE"] = m.Home
	m.Environ["HOMEDRIVE"] = "C:"

	setupExitCode(t, m, 0, "var", "FOO")
	if got := m.Out.String(); got != "FOO = bar baz\norigin: vars\n" {
		t.Errorf("stdout %q", got)
	}
	setupExitCode(t, m, 0, "var", "USERPROFILE")
	if got := m.Out.String(); got != "USERPROFILE = "+m.Home+"\norigin: environment\n" {
		t.Errorf("stdout %q", got)
	}
	// VAR-4: only the built-in names come from the environment.
	setupExitCode(t, m, 1, "var", "HOMEDRIVE")
	setupExitCode(t, m, 1, "var", "NOPE")
	if m.Out.Len() != 0 || !strings.HasPrefix(m.Err.String(), "dot: ") || !strings.Contains(m.Err.String(), "NOPE") {
		t.Errorf("stdout %q; stderr %q", m.Out.String(), m.Err.String())
	}
}

// CMD-14, ST-10: var <name> <value> appends, replaces the first definition,
// or replaces a stub; other lines stay byte-identical; nothing is printed.
func TestVarSet(t *testing.T) {
	cases := []struct{ name, before, args0, args1, want string }{
		{"new, no file", "", "X", "val", "X = val\n"},
		{"new, appended", "# c\r\nY = 1", "X", "a b", "# c\r\nY = 1\nX = a b\n"},
		{"replace first", "# c\nX = old\r\nY = 1\nX = second\n", "X", "new", "# c\nX = new\nY = 1\nX = second\n"},
		{"replace stub", "# used by: A/b\n# X =\nZ = z\n", "X", "  v  ", "# used by: A/b\nX = v\nZ = z\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newRegistered(t, "a")
			varsPath := filepath.Join(m.Dir, core.VarsFile)
			if tc.before != "" {
				writeFile(t, varsPath, tc.before)
			}
			setupExitCode(t, m, 0, "var", tc.args0, tc.args1)
			if m.Out.Len() != 0 || m.Err.Len() != 0 {
				t.Errorf("stdout %q; stderr %q", m.Out.String(), m.Err.String())
			}
			if got := readFile(t, varsPath); got != tc.want {
				t.Errorf("vars = %q, want %q", got, tc.want)
			}
		})
	}
}

// CMD-14, VAR-1, VAR-2, NF-6: an invalid name or value is an error, the
// file stays unchanged and no message holds the value.
func TestVarSetInvalid(t *testing.T) {
	cases := []struct{ name, varName, value string }{
		{"name with dot (VAR-1)", "a.b", "secretvalue"},
		{"empty name (VAR-1)", "", "secretvalue"},
		{"empty value (VAR-2)", "X", ""},
		{"white space value (VAR-2)", "X", " \t "},
		{"LF in value (VAR-2)", "X", "secretvalue\nmore"},
		{"CR in value (VAR-2)", "X", "secretvalue\rmore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newRegistered(t, "a")
			varsPath := filepath.Join(m.Dir, core.VarsFile)
			writeFile(t, varsPath, "X = old\n")
			setupExitCode(t, m, 1, "var", tc.varName, tc.value)
			if !strings.HasPrefix(m.Err.String(), "dot: ") || m.Out.Len() != 0 {
				t.Errorf("stdout %q; stderr %q", m.Out.String(), m.Err.String())
			}
			if strings.Contains(m.Err.String(), "secretvalue") {
				t.Errorf("message prints the value (NF-6): %q", m.Err.String())
			}
			if got := readFile(t, varsPath); got != "X = old\n" {
				t.Errorf("vars = %q", got)
			}
		})
	}
}

// CMD-1, CMD-14: more than two arguments is a usage error.
func TestVarUsage(t *testing.T) {
	m := newRegistered(t, "a")
	setupExitCode(t, m, 2, "var", "X", "a", "b")
	setupNotExist(t, filepath.Join(m.Dir, core.VarsFile))
}

// CMD-15: no problem gives exit 0 and no output.
func TestCheckClean(t *testing.T) {
	m := newRegistered(t, "a")
	m.setVar("VIM", m.Home)
	m.setVar("FONT", "Consolas")
	m.track("VIM/.gvimrc", "set guifont={{FONT}}\n")
	setupExitCode(t, m, 0, "check")
	if m.Out.Len() != 0 || m.Err.Len() != 0 {
		t.Errorf("stdout %q; stderr %q", m.Out.String(), m.Err.String())
	}
}

// CMD-15, NF-6: each problem names the file; exit 1; no value is printed.
func TestCheckProblems(t *testing.T) {
	cases := []struct{ name, file, content, want string }{
		{"undefined placeholder", "VIM/.gvimrc", "{{FONT}} {{SIZE}}\n", "VIM/.gvimrc: placeholder FONT is undefined\nVIM/.gvimrc: placeholder SIZE is undefined\n"},
		{"undefined root", "NOPE/x", "plain\n", "NOPE/x: root NOPE is undefined\n"},
		{"root not absolute", "REL/x", "plain\n", "REL/x: root REL is not an absolute path\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newRegistered(t, "a")
			m.setVar("VIM", m.Home)
			m.setVar("REL", "relsecret")
			m.track(tc.file, tc.content)
			setupExitCode(t, m, 1, "check")
			if got := m.Out.String(); got != tc.want {
				t.Errorf("stdout %q, want %q", got, tc.want)
			}
			if strings.Contains(m.Out.String()+m.Err.String(), "relsecret") {
				t.Error("check prints a value (NF-6)")
			}
		})
	}
}

// CMD-15, PATH-4: a skipped file is not checked.
func TestCheckSkipped(t *testing.T) {
	m := newRegistered(t, "a")
	writeFile(t, filepath.Join(m.Dir, core.ConfigFile), "machine = a\nskip = NOPE/\n")
	m.track("NOPE/x", "{{UNDEFINED}}\n")
	setupExitCode(t, m, 0, "check")
	if m.Out.Len() != 0 {
		t.Errorf("stdout %q", m.Out.String())
	}
}

// CMD-1: check takes no argument.
func TestCheckUsage(t *testing.T) {
	m := newRegistered(t, "a")
	setupExitCode(t, m, 2, "check", "x")
}
