package core

import (
	"os"
	"path/filepath"
	"testing"
)

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// SYNC-5: parent directories are created; no temporary file stays.
func TestWriteAtomic_SYNC5(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "f.txt")
	if err := WriteAtomic(path, []byte("one\r\n")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "one\r\n" {
		t.Errorf("content = %q", got)
	}
	if err := WriteAtomic(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "two" {
		t.Errorf("content = %q", got)
	}
	if names := dirNames(t, filepath.Dir(path)); len(names) != 1 {
		t.Errorf("directory = %v", names)
	}
}

// SYNC-5: on failure the temporary file is removed and the destination stays.
func TestWriteAtomic_SYNC5_FailureCleanup(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "dest")
	writeFile(t, filepath.Join(dest, "keep"), "k")
	if err := WriteAtomic(dest, []byte("x")); err == nil {
		t.Fatal("want error when the destination is a directory")
	}
	if names := dirNames(t, root); len(names) != 1 || names[0] != "dest" {
		t.Errorf("directory = %v", names)
	}
	if got := readFile(t, filepath.Join(dest, "keep")); got != "k" {
		t.Errorf("destination changed: %q", got)
	}
}

// SYNC-6: an existing file keeps its permission bits; a new file gets 0644.
// Each expectation comes from the platform, because Windows keeps only the
// read-only bit.
func TestWriteAtomic_SYNC6(t *testing.T) {
	root := t.TempDir()
	ref := filepath.Join(root, "ref")
	writeFile(t, ref, "")
	if err := os.Chmod(ref, 0o644); err != nil {
		t.Fatal(err)
	}
	refSt, _ := os.Stat(ref)

	fresh := filepath.Join(root, "new")
	if err := WriteAtomic(fresh, []byte("n")); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(fresh)
	if st.Mode().Perm() != refSt.Mode().Perm() {
		t.Errorf("new file perm = %v, want %v", st.Mode().Perm(), refSt.Mode().Perm())
	}

	old := filepath.Join(root, "old")
	writeFile(t, old, "o")
	if err := os.Chmod(old, 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(old)
	if err := WriteAtomic(old, []byte("p")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(old)
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("perm = %v, want %v", after.Mode().Perm(), before.Mode().Perm())
	}
	if perm, err := destPerm(old); err != nil || perm != before.Mode().Perm() {
		t.Errorf("destPerm(existing) = %v, %v", perm, err)
	}
	if perm, err := destPerm(filepath.Join(root, "none")); err != nil || perm != 0o644 {
		t.Errorf("destPerm(new) = %v, %v", perm, err)
	}
}
