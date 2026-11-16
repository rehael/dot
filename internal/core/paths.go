package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/rehael/dot/internal/tmpl"
)

// foldCase is the PATH-3 rule: Windows file names compare without case.
var foldCase = runtime.GOOS == "windows"

// Files applies ST-6 and A15.
func (w *Workspace) Files() ([]string, error) {
	root := filepath.Join(w.RepoDir, FilesDir)
	fi, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, nil
	}
	var names []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if strings.Contains(name, "/") {
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (w *Workspace) RepoPath(name string) string {
	return filepath.Join(w.RepoDir, FilesDir, filepath.FromSlash(name))
}

// LivePath applies PATH-1 and PATH-2.
func (w *Workspace) LivePath(name string) (string, error) {
	p, reason := w.livePath(name)
	if reason != "" {
		return "", fmt.Errorf("%s: %s", name, reason)
	}
	return p, nil
}

// livePath returns a reason instead of an error so that Info can report it.
// The reason never holds a variable value (NF-6).
func (w *Workspace) livePath(name string) (path, reason string) {
	root, rel, ok := strings.Cut(name, "/")
	if !ok || rel == "" {
		return "", "logical name has no root directory"
	}
	if !tmpl.ValidName(root) {
		return "", "root name is not valid"
	}
	value, _, defined := w.Lookup(root)
	if !defined {
		return "", "root " + root + " is undefined"
	}
	if !filepath.IsAbs(value) {
		return "", "root " + root + " is not an absolute path"
	}
	return filepath.Join(filepath.Clean(value), filepath.FromSlash(rel)), ""
}

// IsSkipped applies PATH-4.
func (w *Workspace) IsSkipped(name string) bool {
	for _, s := range w.Skip {
		s = strings.TrimRight(s, "/")
		if s == "" {
			continue
		}
		if name == s || strings.HasPrefix(name, s+"/") {
			return true
		}
	}
	return false
}

// absPath makes a path from the command line absolute against Env.Cwd.
func (w *Workspace) absPath(p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(w.Env.Cwd, p)
	}
	return filepath.Clean(p)
}

// Derive applies PATH-3.
func (w *Workspace) Derive(livePath string) (string, error) {
	p := w.absPath(livePath)
	best, bestLen, bestRel := "", -1, ""
	// Vars is sorted by name, so the strict comparison keeps the first name on a tie.
	for _, v := range w.Vars() {
		if !filepath.IsAbs(v.Value) {
			continue
		}
		dir := filepath.Clean(v.Value)
		rel, ok := relBelow(dir, p, foldCase)
		if ok && len(dir) > bestLen {
			best, bestLen, bestRel = v.Name, len(dir), rel
		}
	}
	if best == "" {
		return "", fmt.Errorf("no variable is a parent directory of %s; define one: dot var <NAME> <directory>", p)
	}
	return best + "/" + bestRel, nil
}

// relBelow returns the path of p relative to dir, with forward slashes, if p
// is strictly below dir. Both paths must be clean. The comparison is by path
// component because the prefix always ends with a separator.
func relBelow(dir, p string, fold bool) (string, bool) {
	prefix := dir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if len(p) <= len(prefix) {
		return "", false
	}
	head := p[:len(prefix)]
	if head != prefix && !(fold && strings.EqualFold(head, prefix)) {
		return "", false
	}
	return filepath.ToSlash(p[len(prefix):]), true
}

// Match applies CMD-4.
func (w *Workspace) Match(arg string) ([]string, error) {
	names, err := w.Files()
	if err != nil {
		return nil, err
	}
	byName := arg
	if runtime.GOOS == "windows" {
		byName = strings.ReplaceAll(arg, `\`, "/")
	}
	abs := w.absPath(arg)
	var out []string
	for _, name := range names {
		if name == byName || strings.HasSuffix(name, "/"+byName) || w.isLivePath(name, abs) {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no tracked file matches: %s", arg)
	}
	return out, nil
}

func (w *Workspace) isLivePath(name, abs string) bool {
	p, reason := w.livePath(name)
	if reason != "" {
		return false
	}
	return p == abs || foldCase && strings.EqualFold(p, abs)
}
