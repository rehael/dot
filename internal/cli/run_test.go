package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehael/dot/internal/core"
)

var allCommands = []string{"init", "register", "add", "edit", "var", "check", "meta", "diff", "apply", "push", "pull"}

// CMD-5: help lists each command and exits with code 0.
func TestHelpListsAllCommands(t *testing.T) {
	m := newMachine(t)
	if code := m.run("help"); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	for _, name := range append(allCommands, "help", "version") {
		if !strings.Contains(m.Out.String(), "\n  "+name+" ") {
			t.Errorf("help does not list %q:\n%s", name, m.Out.String())
		}
	}
	if m.Err.Len() != 0 {
		t.Errorf("unexpected standard error: %s", m.Err.String())
	}
}

// CMD-5: version prints to standard output.
func TestVersion(t *testing.T) {
	m := newMachine(t)
	if code := m.run("version"); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := m.Out.String(); got != "dot "+version+"\n" {
		t.Errorf("got %q", got)
	}
}

// CMD-1, CMD-2, CMD-5: an unknown command is a usage error on standard error.
func TestUnknownCommand(t *testing.T) {
	m := newMachine(t)
	if code := m.run("frobnicate"); code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.HasPrefix(m.Err.String(), "dot: ") || !strings.Contains(m.Err.String(), "usage: dot") {
		t.Errorf("standard error: %s", m.Err.String())
	}
	if m.Out.Len() != 0 {
		t.Errorf("unexpected standard output: %s", m.Out.String())
	}
}

// CMD-1: no command is a usage error.
func TestNoCommand(t *testing.T) {
	m := newMachine(t)
	if code := m.run(); code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
}

// CMD-3: each gated command stops with code 1 on a machine that is not registered.
func TestGateNotRegistered(t *testing.T) {
	gated := []string{"add", "edit", "var", "check", "meta", "diff", "apply", "push", "pull"}
	for _, name := range gated {
		t.Run(name, func(t *testing.T) {
			m := newMachine(t)
			if code := m.run(name); code != 1 {
				t.Fatalf("exit code %d, want 1", code)
			}
			if got := m.Err.String(); got != "dot: machine not registered; run: dot register\n" {
				t.Errorf("standard error: %q", got)
			}
		})
	}
}

// CMD-3: a machine name in config without its line in dot.conf is not registered.
func TestGateMachineNotInRepoConf(t *testing.T) {
	m := newRegistered(t, "a")
	writeFile(t, filepath.Join(m.Dir, core.ConfigFile), "machine = b\n")
	if code := m.run("check"); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(m.Err.String(), "machine not registered") {
		t.Errorf("standard error: %q", m.Err.String())
	}
}

// SYNC-17: a repository with unmerged paths stops apply, push, pull and add.
func TestUnmergedRefused(t *testing.T) {
	for _, name := range []string{"apply", "push", "pull", "add"} {
		t.Run(name, func(t *testing.T) {
			m := newRegistered(t, "a")
			m.track("H/f.txt", "base\n")
			m.git("commit", "-q", "-m", "base")
			m.git("checkout", "-q", "-b", "other")
			writeFile(t, m.repoCopy("H/f.txt"), "other\n")
			m.git("commit", "-q", "-am", "other")
			m.git("checkout", "-q", "-")
			writeFile(t, m.repoCopy("H/f.txt"), "mine\n")
			m.git("commit", "-q", "-am", "mine")
			if _, err := m.gitTry("merge", "other"); err == nil {
				t.Fatal("merge did not conflict")
			}
			if code := m.run(name); code != 1 {
				t.Fatalf("exit code %d, want 1", code)
			}
			if !strings.Contains(m.Err.String(), "unmerged paths") {
				t.Errorf("standard error: %q", m.Err.String())
			}
		})
	}
}
