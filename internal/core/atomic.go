package core

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const newFilePerm fs.FileMode = 0o644

// WriteAtomic applies SYNC-5 and SYNC-6. The temporary file is in the
// destination directory so that the rename stays on one volume.
func WriteAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	perm, err := destPerm(path)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// destPerm returns the permission bits of an existing regular file, else
// those of a new file. A destination that is not a regular file makes the
// rename fail, which reports it.
func destPerm(path string) (fs.FileMode, error) {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return newFilePerm, nil
	}
	if err != nil {
		return 0, err
	}
	if !st.Mode().IsRegular() {
		return newFilePerm, nil
	}
	return st.Mode().Perm(), nil
}
