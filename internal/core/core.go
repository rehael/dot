// Package core holds the workspace model: configuration, variables, paths, state and status.
package core

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rehael/dot/internal/gitx"
	"github.com/rehael/dot/internal/kv"
	"github.com/rehael/dot/internal/tmpl"
)

const (
	ConfigFile   = "config"
	VarsFile     = "vars"
	StateFile    = "state"
	RepoConfFile = "dot.conf"
	FilesDir     = "files"
	DefaultRepo  = "repo"
)

const (
	OriginVars = "vars"
	OriginEnv  = "environment"
)

// Default delimiters when dot.conf has no prefix or suffix line (VAR-5).
const (
	defaultPrefix = "{{"
	defaultSuffix = "}}"
)

type Env struct {
	Getenv         func(string) string
	Cwd            string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// getenv treats a nil Getenv as an empty environment.
func (e Env) getenv(key string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(key)
}

type Workspace struct {
	Env      Env
	LocalDir string
	RepoDir  string
	Machine  string
	Machines []string
	Delims   tmpl.Delims
	Skip     []string

	vars  map[string]string // valid entries of the vars file, first definition wins
	state map[string]string // logical name to lowercase hexadecimal SHA-256
}

// Open loads config, dot.conf, vars and state. Missing files and a missing
// repository are read as empty (ST-3); Open creates nothing.
func Open(env Env) (*Workspace, error) {
	return open(env, os.UserHomeDir)
}

// open takes the home lookup as a parameter so that tests never touch the
// real home directory (TEST-1).
func open(env Env, home func() (string, error)) (*Workspace, error) {
	dir, err := localDir(env, home)
	if err != nil {
		return nil, err
	}
	w := &Workspace{Env: env, LocalDir: dir}
	if err := w.loadConfig(); err != nil {
		return nil, err
	}
	if err := w.loadRepoConf(); err != nil {
		return nil, err
	}
	if err := w.loadVars(); err != nil {
		return nil, err
	}
	if err := w.loadState(); err != nil {
		return nil, err
	}
	return w, nil
}

// localDir applies ST-1.
func localDir(env Env, home func() (string, error)) (string, error) {
	if d := env.getenv("DOT_DIR"); d != "" {
		if !filepath.IsAbs(d) {
			d = filepath.Join(env.Cwd, d)
		}
		return filepath.Clean(d), nil
	}
	if x := env.getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) {
		return filepath.Join(x, "dot"), nil
	}
	h, err := home()
	if err != nil {
		return "", fmt.Errorf("cannot find the local directory: %w", err)
	}
	return filepath.Join(h, ".config", "dot"), nil
}

// readOptional reads a file that may not exist yet (ST-3).
func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// loadConfig applies ST-2 and ST-4.
func (w *Workspace) loadConfig() error {
	data, err := readOptional(filepath.Join(w.LocalDir, ConfigFile))
	if err != nil {
		return err
	}
	entries := kv.Parse(data)
	w.Machine, _ = kv.Get(entries, "machine")
	w.Skip = kv.All(entries, "skip")
	w.RepoDir = filepath.Join(w.LocalDir, DefaultRepo)
	if repo, ok := kv.Get(entries, "repo"); ok {
		if !filepath.IsAbs(repo) {
			return fmt.Errorf("%s: repo is not an absolute path: %s", filepath.Join(w.LocalDir, ConfigFile), repo)
		}
		w.RepoDir = filepath.Clean(repo)
	}
	return nil
}

// loadRepoConf applies ST-5 and VAR-5.
func (w *Workspace) loadRepoConf() error {
	path := filepath.Join(w.RepoDir, RepoConfFile)
	data, err := readOptional(path)
	if err != nil {
		return err
	}
	entries := kv.Parse(data)
	w.Delims = tmpl.Delims{Prefix: defaultPrefix, Suffix: defaultSuffix}
	if p, ok := kv.Get(entries, "prefix"); ok {
		w.Delims.Prefix = p
	}
	if s, ok := kv.Get(entries, "suffix"); ok {
		w.Delims.Suffix = s
	}
	if err := w.Delims.Validate(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	w.Machines = kv.All(entries, "machine")
	return nil
}

// Registered applies CMD-3.
func (w *Workspace) Registered() bool {
	if w.Machine == "" {
		return false
	}
	for _, m := range w.Machines {
		if m == w.Machine {
			return true
		}
	}
	return false
}

func (w *Workspace) Git() gitx.Git {
	return gitx.Git{Dir: w.RepoDir, Stdout: w.Env.Stdout, Stderr: w.Env.Stderr}
}
