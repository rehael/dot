package core

import (
	"reflect"
	"testing"
)

func TestVars_VAR2_InvalidEntriesIgnored(t *testing.T) {
	f := newFixture(t)
	f.writeVars("EMPTY =", "BAD.NAME = x", "OK = 1", "OK = 2", "USERPROFILE =")
	f.env["USERPROFILE"] = "from-env"
	w := f.open()
	if _, _, ok := w.Lookup("EMPTY"); ok {
		t.Error("VAR-2: empty value must be ignored")
	}
	if _, _, ok := w.Lookup("BAD.NAME"); ok {
		t.Error("VAR-1: invalid name must be ignored")
	}
	if v, o, ok := w.Lookup("OK"); !ok || v != "1" || o != OriginVars {
		t.Errorf("first definition wins: got %q %q %v", v, o, ok)
	}
	// An empty vars entry does not shadow the built-in.
	if v, o, ok := w.Lookup("USERPROFILE"); !ok || v != "from-env" || o != OriginEnv {
		t.Errorf("USERPROFILE = %q %q %v", v, o, ok)
	}
}

func TestLookup_VAR3_Order(t *testing.T) {
	f := newFixture(t)
	f.writeVars("APPDATA = from-vars")
	f.env["APPDATA"] = "from-env-a"
	f.env["LOCALAPPDATA"] = "from-env-l"
	w := f.open()
	if v, o, ok := w.Lookup("APPDATA"); !ok || v != "from-vars" || o != OriginVars {
		t.Errorf("vars first: %q %q %v", v, o, ok)
	}
	if v, o, ok := w.Lookup("LOCALAPPDATA"); !ok || v != "from-env-l" || o != OriginEnv {
		t.Errorf("built-in: %q %q %v", v, o, ok)
	}
	if _, _, ok := w.Lookup("USERPROFILE"); ok {
		t.Error("unset built-in must be undefined")
	}
	if _, _, ok := w.Lookup("NOPE"); ok {
		t.Error("NOPE must be undefined")
	}
}

func TestLookup_VAR4_NoOtherEnv(t *testing.T) {
	f := newFixture(t)
	f.env["HOME"] = "h"
	f.env["VIM"] = "v"
	f.env["USERPROFILE"] = ""
	w := f.open()
	for _, n := range []string{"HOME", "VIM", "DOT_DIR", "USERPROFILE"} {
		if _, _, ok := w.Lookup(n); ok {
			t.Errorf("VAR-4: %s must be undefined", n)
		}
	}
}

func TestVars_InEffectSorted(t *testing.T) {
	f := newFixture(t)
	f.writeVars("Z = 26", "APPDATA = mine", "B = 2")
	f.env["APPDATA"] = "env-a"
	f.env["USERPROFILE"] = "env-u"
	f.env["OTHER"] = "x"
	w := f.open()
	want := []Var{
		{"APPDATA", "mine", OriginVars},
		{"B", "2", OriginVars},
		{"USERPROFILE", "env-u", OriginEnv},
		{"Z", "26", OriginVars},
	}
	if got := w.Vars(); !reflect.DeepEqual(got, want) {
		t.Errorf("Vars = %v, want %v", got, want)
	}
}
