package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dot/internal/core"
	"dot/internal/gitx"
	"dot/internal/tmpl"
)

// cmdMeta implements CMD-16. It is one of the two commands that print
// variable values (NF-6) and it writes no state (SYNC-1).
func cmdMeta(c *ctx, args []string) int {
	switch len(args) {
	case 0:
		return fileMetaAll(c)
	case 1:
		return fileMetaMatches(c, args[0])
	}
	return c.usage("meta")
}

func fileMetaAll(c *ctx) int {
	c.outf("machine: %s", c.w.Machine)
	c.outf("local directory: %s", c.w.LocalDir)
	c.outf("repository: %s", c.w.RepoDir)
	c.outf("delimiters: %s %s", c.w.Delims.Prefix, c.w.Delims.Suffix)
	c.outf("registered machines: %s", strings.Join(c.w.Machines, ", "))
	c.outf("variables:")
	for _, v := range c.w.Vars() {
		c.outf("  %s = %s (%s)", v.Name, v.Value, v.Origin)
	}
	c.outf("files:")
	names, err := c.w.Files()
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	code := exitOK
	for _, name := range names {
		info, err := c.w.Info(name)
		if err != nil {
			c.errf("%v", err)
			code = exitFail
			continue
		}
		line := "  " + info.Status.String() + " " + name
		if info.LivePath != "" {
			line += " " + info.LivePath
		}
		if len(info.Reasons) > 0 {
			line += " (" + strings.Join(info.Reasons, "; ") + ")"
		}
		c.outf("%s", line)
	}
	return code
}

func fileMetaMatches(c *ctx, arg string) int {
	names, err := c.w.Match(arg)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	code := exitOK
	for i, name := range names {
		info, err := c.w.Info(name)
		if err != nil {
			c.errf("%v", err)
			code = exitFail
			continue
		}
		if i > 0 {
			c.outf("")
		}
		live := info.LivePath
		if live == "" {
			live = "none"
		}
		c.outf("name: %s", name)
		c.outf("repository path: %s", info.RepoPath)
		c.outf("live path: %s", live)
		c.outf("status: %s", info.Status)
		for _, r := range info.Reasons {
			c.outf("reason: %s", r)
		}
		for _, n := range tmpl.Names(c.w.Delims, info.Source) {
			if value, origin, ok := c.w.Lookup(n); ok {
				c.outf("placeholder: %s = %s (%s)", n, value, origin)
			} else {
				c.outf("placeholder: %s undefined", n)
			}
		}
	}
	return code
}

// cmdDiff implements CMD-17. It writes no state and no file outside its
// temporary directory (SYNC-1).
func cmdDiff(c *ctx, args []string) int {
	if len(args) > 1 {
		return c.usage("diff")
	}
	names, err := fileSelect(c, args)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	code := exitOK
	for _, name := range names {
		if !fileDiffOne(c, name) {
			code = exitFail
		}
	}
	return code
}

// fileDiffOne returns false on an error.
func fileDiffOne(c *ctx, name string) bool {
	info, err := c.w.Info(name)
	if err != nil {
		c.errf("%v", err)
		return false
	}
	switch info.Status {
	case core.InSync, core.Skipped:
		return true
	case core.Unresolved:
		c.outf("%s %s: %s", info.Status, name, strings.Join(info.Reasons, "; "))
		return true
	case core.Missing:
		c.outf("%s %s", info.Status, name)
		return true
	}
	c.outf("%s %s", info.Status, name)
	if err := fileGitDiff(c, name, info.Rendered, info.Live); err != nil {
		c.errf("%s: %v", name, err)
		return false
	}
	if info.Status == core.LiveChanged {
		if _, dropped, err := tmpl.Capture(c.w.Delims, info.Source, info.Live, c.lookup()); err != nil {
			c.warnf("%s: a capture would fail: %v; fix by hand: dot edit %s", name, err, name)
		} else {
			c.warnDrops(name, dropped)
		}
	}
	return true
}

// fileGitDiff runs git diff --no-index on copies in a temporary directory,
// so that the header names the sides "repository" and "live" on every platform.
func fileGitDiff(c *ctx, name string, rendered, live []byte) (err error) {
	tmp, err := os.MkdirTemp("", "dot-diff-")
	if err != nil {
		return err
	}
	defer func() {
		if rerr := os.RemoveAll(tmp); rerr != nil && err == nil {
			err = rerr
		}
	}()
	repoSide := "repository/" + name
	liveSide := "live/" + name
	for _, f := range []struct {
		rel  string
		data []byte
	}{{repoSide, rendered}, {liveSide, live}} {
		p := filepath.Join(tmp, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.data, 0o644); err != nil {
			return err
		}
	}
	g := gitx.Git{Dir: tmp, Stdout: c.env.Stdout, Stderr: c.env.Stderr}
	// File content is bytes (D8): a core.autocrlf of the user would hide a
	// difference that is only LF against CRLF.
	code, err := g.Run("-c", "core.autocrlf=false", "-c", "core.safecrlf=false",
		"diff", "--no-index", "--no-prefix", "--", repoSide, liveSide)
	if err != nil {
		return err
	}
	// Exit code 1 means that the sides differ.
	if code > 1 {
		return fmt.Errorf("git diff: exit status %d", code)
	}
	return nil
}
