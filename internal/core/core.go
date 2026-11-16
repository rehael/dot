// Package core holds the workspace model: configuration, variables, paths, state and status.
package core

import (
	"io"

	"dot/internal/gitx"
	"dot/internal/tmpl"
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

type Env struct {
	Getenv         func(string) string
	Cwd            string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

type Status int

const (
	InSync Status = iota
	RepoChanged
	LiveChanged
	BothChanged
	Missing
	Skipped
	Unresolved
)

func (s Status) String() string { panic("not implemented") }

type FileInfo struct {
	Name, RepoPath, LivePath string
	Status                   Status
	Reasons                  []string
	Source, Rendered, Live   []byte
}

type Var struct{ Name, Value, Origin string }

type Workspace struct {
	Env      Env
	LocalDir string
	RepoDir  string
	Machine  string
	Machines []string
	Delims   tmpl.Delims
	Skip     []string
}

func Open(env Env) (*Workspace, error) { panic("not implemented") }

func (w *Workspace) Registered() bool { panic("not implemented") }

func (w *Workspace) Lookup(name string) (value, origin string, ok bool) {
	panic("not implemented")
}

func (w *Workspace) Vars() []Var { panic("not implemented") }

func (w *Workspace) Files() ([]string, error) { panic("not implemented") }

func (w *Workspace) Derive(livePath string) (string, error) { panic("not implemented") }

func (w *Workspace) RepoPath(name string) string { panic("not implemented") }

func (w *Workspace) LivePath(name string) (string, error) { panic("not implemented") }

func (w *Workspace) IsSkipped(name string) bool { panic("not implemented") }

func (w *Workspace) Info(name string) (FileInfo, error) { panic("not implemented") }

func (w *Workspace) Match(arg string) ([]string, error) { panic("not implemented") }

func (w *Workspace) SetHash(name string, data []byte) { panic("not implemented") }

func (w *Workspace) SaveState() error { panic("not implemented") }

func (w *Workspace) Git() gitx.Git { panic("not implemented") }

func WriteAtomic(path string, data []byte) error { panic("not implemented") }
