// Package cli dispatches the commands of dot.
package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/rehael/dot/internal/core"
	"github.com/rehael/dot/internal/tmpl"
)

const version = "0.1.1"

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

// ctx is the state that one command invocation works with.
type ctx struct {
	env core.Env
	w   *core.Workspace
}

type command struct {
	name    string
	args    string
	summary string
	// gated commands need a registered machine (CMD-3).
	gated bool
	// clean commands refuse a repository with unmerged paths (SYNC-17).
	clean bool
	run   func(c *ctx, args []string) int
}

func commands() []command {
	return []command{
		{name: "init", args: "[path]", summary: "create or adopt the repository", run: cmdInit},
		{name: "register", args: "[name]", summary: "register this machine", run: cmdRegister},
		{name: "add", args: "<file>", summary: "track a live file, or capture its edit", gated: true, clean: true, run: cmdAdd},
		{name: "edit", args: "<file>", summary: "open the repository copy in the editor", gated: true, run: cmdEdit},
		{name: "var", args: "[name [value]]", summary: "show or set a variable; no argument: edit vars", gated: true, run: cmdVar},
		{name: "check", args: "", summary: "report undefined roots and placeholder names", gated: true, run: cmdCheck},
		{name: "meta", args: "[file]", summary: "show configuration, variables and file status", gated: true, run: cmdMeta},
		{name: "diff", args: "[file]", summary: "show differences, rendered repository copy to live file", gated: true, run: cmdDiff},
		{name: "apply", args: "[file] [--force]", summary: "write live files from the repository", gated: true, clean: true, run: cmdApply},
		{name: "push", args: "[-m <message>]", summary: "capture live edits, commit, git push", gated: true, clean: true, run: cmdPush},
		{name: "pull", args: "", summary: "capture live edits, commit, git pull", gated: true, clean: true, run: cmdPull},
	}
}

func find(name string) (command, bool) {
	for _, cmd := range commands() {
		if cmd.name == name {
			return cmd, true
		}
	}
	return command{}, false
}

func usageText() string {
	var b strings.Builder
	b.WriteString("usage: dot <command> [arguments]\n\ncommands:\n")
	line := func(synopsis, summary string) {
		fmt.Fprintf(&b, "  %-24s %s\n", synopsis, summary)
	}
	for _, cmd := range commands() {
		line(strings.TrimSpace(cmd.name+" "+cmd.args), cmd.summary)
	}
	line("help", "show this text")
	line("version", "show the version")
	return b.String()
}

// Run executes one command and returns the exit code (CMD-1).
func Run(args []string, env core.Env) int {
	c := &ctx{env: env}
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usageText())
		return exitUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(env.Stdout, usageText())
		return exitOK
	case "version", "--version":
		c.outf("dot %s", version)
		return exitOK
	}
	cmd, ok := find(args[0])
	if !ok {
		c.errf("unknown command: %s", args[0])
		fmt.Fprint(env.Stderr, usageText())
		return exitUsage
	}
	w, err := core.Open(env)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	c.w = w
	if cmd.gated && !w.Registered() {
		c.errf("machine not registered; run: dot register")
		return exitFail
	}
	if cmd.clean && c.unmerged() {
		return exitFail
	}
	return cmd.run(c, args[1:])
}

func (c *ctx) outf(format string, a ...any) {
	fmt.Fprintf(c.env.Stdout, format+"\n", a...)
}

func (c *ctx) errf(format string, a ...any) {
	fmt.Fprintf(c.env.Stderr, "dot: "+format+"\n", a...)
}

func (c *ctx) warnf(format string, a ...any) {
	fmt.Fprintf(c.env.Stderr, "dot: warning: "+format+"\n", a...)
}

// usage reports a usage error of the named command (CMD-1).
func (c *ctx) usage(name string) int {
	cmd, _ := find(name)
	c.errf("usage: dot %s", strings.TrimSpace(cmd.name+" "+cmd.args))
	return exitUsage
}

// unmerged reports, with a message, a repository that has unmerged paths (SYNC-17).
func (c *ctx) unmerged() bool {
	out, err := c.w.Git().Output("ls-files", "-u")
	if err != nil {
		c.errf("%v", err)
		return true
	}
	if strings.TrimSpace(out) != "" {
		c.errf("the repository has unmerged paths; resolve the conflict with git and commit")
		return true
	}
	return false
}

// lookup adapts the variables of the workspace to the template package.
func (c *ctx) lookup() tmpl.Lookup {
	return func(name string) (string, bool) {
		value, _, ok := c.w.Lookup(name)
		return value, ok
	}
}

// warnDrops prints one warning for each dropped placeholder (SYNC-15).
func (c *ctx) warnDrops(name string, drops []tmpl.Drop) {
	for _, d := range drops {
		c.warnf("%s: line %d: placeholder %s%s%s dropped", name, d.Line, c.w.Delims.Prefix, d.Name, c.w.Delims.Suffix)
	}
}

// runEditor opens path in the editor of the user and waits for exit (CMD-13).
func (c *ctx) runEditor(path string) int {
	editor := c.env.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		editor = c.env.Getenv("VISUAL")
	}
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		c.errf("no editor: set EDITOR or VISUAL")
		return exitFail
	}
	cmd := exec.Command(fields[0], append(fields[1:], path)...)
	cmd.Dir = c.env.Cwd
	cmd.Stdin = c.env.Stdin
	cmd.Stdout = c.env.Stdout
	cmd.Stderr = c.env.Stderr
	if err := cmd.Run(); err != nil {
		c.errf("editor: %v", err)
		return exitFail
	}
	return exitOK
}
