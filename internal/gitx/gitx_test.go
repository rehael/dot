package gitx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testName  = "Test User"
	testEmail = "test@example.invalid"
)

// isolation returns the environment entries that keep git away from the real
// configuration and give it a neutral identity (TEST-1).
func isolation(t *testing.T) []string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return []string{
		"GIT_CONFIG_GLOBAL=" + cfg,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=" + testName,
		"GIT_AUTHOR_EMAIL=" + testEmail,
		"GIT_COMMITTER_NAME=" + testName,
		"GIT_COMMITTER_EMAIL=" + testEmail,
	}
}

// setIsolation applies isolation through the test process environment (A17).
func setIsolation(t *testing.T) {
	t.Helper()
	for _, kv := range isolation(t) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustOutput(t *testing.T, g Git, args ...string) string {
	t.Helper()
	out, err := g.Output(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

// newRepo creates a repository with branch main and one commit.
func newRepo(t *testing.T, g Git) {
	t.Helper()
	mustOutput(t, g, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(g.Dir, "a.txt"), "a\n")
	mustOutput(t, g, "add", "a.txt")
	mustOutput(t, g, "commit", "-m", "init")
}

func TestOutputRepositoryEnvField(t *testing.T) {
	g := Git{Dir: t.TempDir(), Env: isolation(t)}
	newRepo(t, g)

	if got := mustOutput(t, g, "rev-parse", "--abbrev-ref", "HEAD"); got != "main\n" {
		t.Errorf("rev-parse = %q, want %q (untrimmed)", got, "main\n")
	}
	if got := mustOutput(t, g, "ls-files"); got != "a.txt\n" {
		t.Errorf("ls-files = %q, want %q", got, "a.txt\n")
	}
	if got := mustOutput(t, g, "log", "-1", "--format=%an <%ae>|%cn <%ce>"); got != "Test User <test@example.invalid>|Test User <test@example.invalid>\n" {
		t.Errorf("commit identity = %q", got)
	}
}

func TestOutputRepositorySetenv(t *testing.T) {
	setIsolation(t)
	g := Git{Dir: t.TempDir()}
	newRepo(t, g)

	if got := mustOutput(t, g, "log", "-1", "--format=%an <%ae>"); got != "Test User <test@example.invalid>\n" {
		t.Errorf("author = %q", got)
	}
	// The empty global file is the only configuration git reads.
	if got := mustOutput(t, g, "config", "--global", "--list"); got != "" {
		t.Errorf("global config = %q, want empty", got)
	}
}

func TestEnvReachesChildAndOverrides(t *testing.T) {
	setIsolation(t)
	t.Setenv("GIT_AUTHOR_NAME", "Inherited Name")
	g := Git{Dir: t.TempDir(), Env: []string{"GIT_AUTHOR_NAME=Env Name", "GIT_AUTHOR_EMAIL=env@example.invalid"}}

	got := mustOutput(t, g, "var", "GIT_AUTHOR_IDENT")
	if !strings.HasPrefix(got, "Env Name <env@example.invalid> ") {
		t.Errorf("GIT_AUTHOR_IDENT = %q, want Env entries to win", got)
	}
	got = mustOutput(t, g, "var", "GIT_COMMITTER_IDENT")
	if !strings.HasPrefix(got, "Test User <test@example.invalid> ") {
		t.Errorf("GIT_COMMITTER_IDENT = %q, want inherited environment", got)
	}
}

func TestOutputStdinNotConnected(t *testing.T) {
	g := Git{Dir: t.TempDir(), Env: isolation(t)}
	// Hash of the empty blob: standard input reads as empty.
	if got := mustOutput(t, g, "hash-object", "--stdin"); got != "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391\n" {
		t.Errorf("hash-object --stdin = %q", got)
	}
}

func TestOutputFailureCarriesStderr(t *testing.T) {
	var stderr bytes.Buffer
	g := Git{Dir: t.TempDir(), Env: isolation(t), Stderr: &stderr}
	newRepo(t, g)

	out, err := g.Output("show-ref", "--verify", "refs/heads/nope")
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v (%T), want *ExitError", err, err)
	}
	if ee.Code == 0 || ee.Subcommand != "show-ref" || !strings.Contains(ee.Stderr, "refs/heads/nope") {
		t.Errorf("ExitError = %+v", ee)
	}
	if ee.Stderr != strings.TrimSpace(ee.Stderr) {
		t.Errorf("Stderr not trimmed: %q", ee.Stderr)
	}
	msg := err.Error()
	if !strings.Contains(msg, "show-ref") || !strings.Contains(msg, ee.Stderr) {
		t.Errorf("Error() = %q, want subcommand and stderr text", msg)
	}
	if stderr.Len() != 0 {
		t.Errorf("g.Stderr received %q, want nothing", stderr.String())
	}
}

func TestRunSuccessRoutesStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	g := Git{Dir: t.TempDir(), Env: isolation(t), Stdout: &stdout, Stderr: &stderr}
	newRepo(t, Git{Dir: g.Dir, Env: g.Env})

	code, err := g.Run("ls-files")
	if code != 0 || err != nil {
		t.Fatalf("Run = (%d, %v), want (0, nil)", code, err)
	}
	if stdout.String() != "a.txt\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRunNonZeroExit(t *testing.T) {
	setIsolation(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x"), "x\n")
	writeFile(t, filepath.Join(dir, "y"), "y\n")
	var stdout bytes.Buffer
	g := Git{Dir: dir, Stdout: &stdout}

	code, err := g.Run("diff", "--no-index", "--quiet", "x", "y")
	if code != 1 || err != nil {
		t.Errorf("Run = (%d, %v), want (1, nil)", code, err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty with --quiet", stdout.String())
	}
}

func TestRunRoutesStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	g := Git{Dir: t.TempDir(), Env: isolation(t), Stdout: &stdout, Stderr: &stderr}
	newRepo(t, Git{Dir: g.Dir, Env: g.Env})

	code, err := g.Run("show-ref", "--verify", "refs/heads/nope")
	if code == 0 || err != nil {
		t.Fatalf("Run = (%d, %v), want non-zero code and nil error", code, err)
	}
	if !strings.Contains(stderr.String(), "refs/heads/nope") {
		t.Errorf("stderr = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunNilWritersDiscard(t *testing.T) {
	g := Git{Dir: t.TempDir(), Env: isolation(t)}
	if code, err := g.Run("init"); code != 0 || err != nil {
		t.Errorf("Run = (%d, %v), want (0, nil)", code, err)
	}
	if code, err := g.Run("show-ref", "--verify", "refs/heads/nope"); code == 0 || err != nil {
		t.Errorf("Run = (%d, %v), want non-zero code and nil error", code, err)
	}
}

func TestRunMissingDir(t *testing.T) {
	g := Git{Dir: filepath.Join(t.TempDir(), "absent"), Env: isolation(t)}
	code, err := g.Run("status")
	if err == nil || code == 0 {
		t.Fatalf("Run = (%d, %v), want non-zero code and an error", code, err)
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		t.Errorf("err is *ExitError, want a start error")
	}
}

func TestOutputMissingDir(t *testing.T) {
	g := Git{Dir: filepath.Join(t.TempDir(), "absent"), Env: isolation(t)}
	_, err := g.Output("status")
	if err == nil {
		t.Fatal("Output: nil error, want a start error")
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		t.Errorf("err is *ExitError, want a start error")
	}
}
