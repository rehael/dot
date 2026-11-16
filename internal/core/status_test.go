package core

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestStatusString(t *testing.T) {
	want := map[Status]string{
		InSync: "in-sync", RepoChanged: "repo-changed", LiveChanged: "live-changed",
		BothChanged: "both-changed", Missing: "missing", Skipped: "skipped", Unresolved: "unresolved",
	}
	for s, w := range want {
		if s.String() != w {
			t.Errorf("%d: %q, want %q", int(s), s.String(), w)
		}
	}
}

// statusFixture: tracked file R/f = "a {{X}}\n", X = 1, rendered "a 1\n".
func statusFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.writeVars("R = "+f.live(), "X = 1")
	f.track("R/f", "a {{X}}\n")
	return f
}

func info(t *testing.T, w *Workspace, name string) FileInfo {
	t.Helper()
	fi, err := w.Info(name)
	if err != nil {
		t.Fatalf("Info(%q): %v", name, err)
	}
	return fi
}

func TestInfo_Fields(t *testing.T) {
	f := statusFixture(t)
	writeFile(t, f.live("f"), "a 1\n")
	fi := info(t, f.open(), "R/f")
	want := FileInfo{
		Name: "R/f", RepoPath: f.repo(FilesDir, "R", "f"), LivePath: f.live("f"),
		Status: InSync, Source: []byte("a {{X}}\n"), Rendered: []byte("a 1\n"), Live: []byte("a 1\n"),
	}
	if !reflect.DeepEqual(fi, want) {
		t.Errorf("Info = %+v\nwant %+v", fi, want)
	}
}

func TestInfo_NotTracked(t *testing.T) {
	f := statusFixture(t)
	if _, err := f.open().Info("R/other"); err == nil {
		t.Error("want error for an untracked name")
	}
}

func TestInfo_LiveNotRegular(t *testing.T) {
	f := statusFixture(t)
	if err := os.MkdirAll(f.live("f"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := f.open().Info("R/f"); err == nil {
		t.Error("want error when the live path is a directory")
	}
}

// Rule 1: skipped comes before unresolved.
func TestInfo_Rule1_Skipped(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("skip = R")
	f.track("R/f", "{{NOPE}}")
	if fi := info(t, f.open(), "R/f"); fi.Status != Skipped || fi.Reasons != nil {
		t.Errorf("Status = %v, Reasons = %v", fi.Status, fi.Reasons)
	}
}

// Rule 2: unresolved comes before missing; reasons never hold a value (NF-6).
func TestInfo_Rule2_Unresolved(t *testing.T) {
	f := newFixture(t)
	f.writeVars("REL = relative/secret-value", "ABS = "+f.live())
	f.track("NOPE/f", "x")
	f.track("REL/f", "x {{U1}} {{U2}} {{U1}}")
	f.track("bad.root/f", "x")
	f.track("ABS/f", "{{U3}}")
	w := f.open()
	cases := map[string][]string{
		"NOPE/f":     {"root NOPE is undefined"},
		"REL/f":      {"root REL is not an absolute path", "placeholder U1 is undefined", "placeholder U2 is undefined"},
		"bad.root/f": {"root name is not valid"}, // A15
		"ABS/f":      {"placeholder U3 is undefined"},
	}
	for name, reasons := range cases {
		fi := info(t, w, name)
		if fi.Status != Unresolved || !reflect.DeepEqual(fi.Reasons, reasons) {
			t.Errorf("%s: Status = %v, Reasons = %q", name, fi.Status, fi.Reasons)
		}
		if strings.Contains(strings.Join(fi.Reasons, " "), "secret-value") {
			t.Errorf("%s: NF-6: reasons show a value", name)
		}
	}
	if fi := info(t, w, "ABS/f"); fi.LivePath == "" || fi.Rendered != nil {
		t.Errorf("ABS/f: LivePath = %q, Rendered = %q", fi.LivePath, fi.Rendered)
	}
}

// Rule 3: missing.
func TestInfo_Rule3_Missing(t *testing.T) {
	f := statusFixture(t)
	w := f.open()
	w.SetHash("R/f", []byte("a 1\n"))
	if fi := info(t, w, "R/f"); fi.Status != Missing || fi.Live != nil {
		t.Errorf("Status = %v, Live = %q", fi.Status, fi.Live)
	}
}

// Rule 4: in-sync whenever L = R, whatever H is.
func TestInfo_Rule4_InSync(t *testing.T) {
	f := statusFixture(t)
	writeFile(t, f.live("f"), "a 1\n")
	w := f.open()
	if fi := info(t, w, "R/f"); fi.Status != InSync {
		t.Errorf("no H: Status = %v", fi.Status)
	}
	w.SetHash("R/f", []byte("old"))
	if fi := info(t, w, "R/f"); fi.Status != InSync {
		t.Errorf("other H: Status = %v", fi.Status)
	}
}

// Rule 5: repo-changed when L = H.
func TestInfo_Rule5_RepoChanged(t *testing.T) {
	f := statusFixture(t)
	writeFile(t, f.live("f"), "old\n")
	w := f.open()
	w.SetHash("R/f", []byte("old\n"))
	if fi := info(t, w, "R/f"); fi.Status != RepoChanged {
		t.Errorf("Status = %v", fi.Status)
	}
}

// SYNC-4: a changed variable value changes R; the file becomes repo-changed.
func TestInfo_SYNC4_VariableChange(t *testing.T) {
	f := statusFixture(t)
	writeFile(t, f.live("f"), "a 1\n")
	w := f.open()
	if fi := info(t, w, "R/f"); fi.Status != InSync {
		t.Fatalf("Status = %v", fi.Status)
	}
	w.SetHash("R/f", []byte("a 1\n"))
	if err := w.SaveState(); err != nil {
		t.Fatal(err)
	}
	f.writeVars("R = "+f.live(), "X = 2")
	fi := info(t, f.open(), "R/f")
	if fi.Status != RepoChanged || string(fi.Rendered) != "a 2\n" {
		t.Errorf("Status = %v, Rendered = %q", fi.Status, fi.Rendered)
	}
}

// Rule 6: live-changed when R = H.
func TestInfo_Rule6_LiveChanged(t *testing.T) {
	f := statusFixture(t)
	writeFile(t, f.live("f"), "a 1 edited\n")
	w := f.open()
	w.SetHash("R/f", []byte("a 1\n"))
	if fi := info(t, w, "R/f"); fi.Status != LiveChanged {
		t.Errorf("Status = %v", fi.Status)
	}
}

// Rule 7: both-changed, with and without H.
func TestInfo_Rule7_BothChanged(t *testing.T) {
	f := statusFixture(t)
	writeFile(t, f.live("f"), "other\n")
	w := f.open()
	if fi := info(t, w, "R/f"); fi.Status != BothChanged {
		t.Errorf("no H and L != R: Status = %v", fi.Status)
	}
	w.SetHash("R/f", []byte("base\n"))
	if fi := info(t, w, "R/f"); fi.Status != BothChanged {
		t.Errorf("H, L and R all differ: Status = %v", fi.Status)
	}
}

// Info reads both files at each call.
func TestInfo_NoCache(t *testing.T) {
	f := statusFixture(t)
	writeFile(t, f.live("f"), "a 1\n")
	w := f.open()
	info(t, w, "R/f")
	f.track("R/f", "b {{X}}\n")
	if fi := info(t, w, "R/f"); fi.Status != BothChanged || string(fi.Source) != "b {{X}}\n" {
		t.Errorf("Status = %v, Source = %q", fi.Status, fi.Source)
	}
}
