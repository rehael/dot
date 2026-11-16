package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ST-2: state line format; LF and CRLF; other lines are ignored.
func TestLoadState_ST2(t *testing.T) {
	f := newFixture(t)
	h1, h2 := sha("one"), sha("two")
	content := strings.Join([]string{
		h1 + " A/one",
		h2 + " B/with space\r",
		strings.ToUpper(h1) + " C/upper",
		h1[:63] + " D/short",
		h1 + "  E/two-spaces",
		h1 + "\tF/tab",
		h1,
		h1 + " ",
		"# comment",
		"",
		h2 + " A/one",
	}, "\n")
	writeFile(t, f.local(StateFile), content)
	w := f.open()
	want := map[string]string{
		"A/one":         h1,
		"B/with space":  h2,
		" E/two-spaces": h1, // the name is the remainder of the line, unchanged
	}
	if len(w.state) != len(want) {
		t.Errorf("state = %v", w.state)
	}
	for k, v := range want {
		if w.state[k] != v {
			t.Errorf("state[%q] = %q, want %q", k, w.state[k], v)
		}
	}
}

func TestHash(t *testing.T) {
	// SHA-256 of "abc", lowercase hexadecimal.
	if got := hashHex([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("hashHex = %s", got)
	}
}

// SYNC-3: SaveState drops untracked names, sorts by name, uses LF; ST-3:
// it creates the local directory.
func TestSaveState_SYNC3(t *testing.T) {
	f := newFixture(t)
	f.track("B/b", "b")
	f.track("A/a", "a")
	f.track("C/c", "c")
	w := f.open()
	w.SetHash("B/b", []byte("lb"))
	w.SetHash("A/a", []byte("la"))
	w.SetHash("GONE/x", []byte("gone"))
	if err := w.SaveState(); err != nil {
		t.Fatal(err)
	}
	want := sha("la") + " A/a\n" + sha("lb") + " B/b\n"
	if got := readFile(t, f.local(StateFile)); got != want {
		t.Errorf("state file = %q, want %q", got, want)
	}

	// Entries read from an old CRLF file are rewritten with LF.
	writeFile(t, f.local(StateFile), sha("x")+" C/c\r\n"+sha("y")+" OLD/f\r\n")
	w = f.open()
	if err := w.SaveState(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, f.local(StateFile)); got != sha("x")+" C/c\n" {
		t.Errorf("state file = %q", got)
	}
}

func TestSaveState_ST3_CreatesLocalDir(t *testing.T) {
	f := newFixture(t)
	w := f.open()
	if _, err := os.Stat(f.local()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local dir exists before SaveState: %v", err)
	}
	if err := w.SaveState(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, f.local(StateFile)); got != "" {
		t.Errorf("state file = %q", got)
	}
}

// SYNC-7: SetHash, SaveState, and a new Open sees the hash.
func TestSetHash_SYNC7(t *testing.T) {
	f := newFixture(t)
	f.writeVars("R = " + f.live())
	f.track("R/f", "repo")
	writeFile(t, f.live("f"), "written")
	w := f.open()
	w.SetHash("R/f", []byte("written"))
	if _, err := os.Stat(f.local(StateFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("SetHash must not write the file")
	}
	if err := w.SaveState(); err != nil {
		t.Fatal(err)
	}
	w2 := f.open()
	if w2.state["R/f"] != sha("written") {
		t.Errorf("state = %v", w2.state)
	}
	fi, err := w2.Info("R/f")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Status != RepoChanged {
		t.Errorf("Status = %v, want repo-changed", fi.Status)
	}
}
