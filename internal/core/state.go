package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

const hashLen = 64

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// loadState applies ST-2. A line that does not have the form is ignored, so
// a damaged entry only costs the state hash of one file.
func (w *Workspace) loadState() error {
	data, err := readOptional(filepath.Join(w.LocalDir, StateFile))
	if err != nil {
		return err
	}
	w.state = map[string]string{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		hash, name, ok := parseStateLine(string(line))
		if !ok {
			continue
		}
		if _, dup := w.state[name]; !dup {
			w.state[name] = hash
		}
	}
	return nil
}

func parseStateLine(line string) (hash, name string, ok bool) {
	if len(line) < hashLen+2 || line[hashLen] != ' ' {
		return "", "", false
	}
	for i := 0; i < hashLen; i++ {
		c := line[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", "", false
		}
	}
	return line[:hashLen], line[hashLen+1:], true
}

// SetHash changes the state in memory only; SaveState writes it.
func (w *Workspace) SetHash(name string, data []byte) {
	w.state[name] = hashHex(data)
}

// SaveState applies SYNC-3.
func (w *Workspace) SaveState() error {
	names, err := w.Files()
	if err != nil {
		return err
	}
	// Files is sorted, and iterating it drops the entries of untracked files.
	var b strings.Builder
	for _, name := range names {
		if h, ok := w.state[name]; ok {
			b.WriteString(h + " " + name + "\n")
		}
	}
	return WriteAtomic(filepath.Join(w.LocalDir, StateFile), []byte(b.String()))
}
