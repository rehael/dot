// Package gitx runs the git executable.
package gitx

import "io"

type Git struct {
	Dir            string
	Env            []string
	Stdout, Stderr io.Writer
}

func (g Git) Run(args ...string) (exitCode int, err error) { panic("not implemented") }

func (g Git) Output(args ...string) (string, error) { panic("not implemented") }
