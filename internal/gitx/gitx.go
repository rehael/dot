// Package gitx runs the git executable.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Git runs git from PATH in Dir. Env holds "KEY=VALUE" entries added to the
// process environment; an entry overrides an inherited variable of the same name.
// A nil Stdout or Stderr discards that stream.
type Git struct {
	Dir            string
	Env            []string
	Stdout, Stderr io.Writer
}

// ExitError reports that git ran and exited with a non-zero code. Any other
// error from Output means that git did not start or its output could not be read.
type ExitError struct {
	Subcommand string // first argument given to git
	Code       int
	Stderr     string // trimmed standard error text
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("git %s: exit status %d", e.Subcommand, e.Code)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

func (g Git) command(args []string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.Dir
	cmd.Env = append(os.Environ(), g.Env...)
	return cmd
}

// Run returns git's exit code. err is non-nil only when git did not run to
// completion (not found, Dir missing, output not delivered); exitCode is then -1.
func (g Git) Run(args ...string) (exitCode int, err error) {
	cmd := g.command(args)
	cmd.Stdout = g.Stdout
	cmd.Stderr = g.Stderr
	err = cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code != 0 {
			return code, nil
		}
	}
	return -1, err
}

// Output returns git's standard output unchanged, also when it fails. A
// non-zero exit gives an *ExitError; standard error is never written to g.Stderr.
func (g Git) Output(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := g.command(args)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() != 0 {
		sub := ""
		if len(args) > 0 {
			sub = args[0]
		}
		return stdout.String(), &ExitError{
			Subcommand: sub,
			Code:       ee.ExitCode(),
			Stderr:     strings.TrimSpace(stderr.String()),
		}
	}
	return stdout.String(), err
}
