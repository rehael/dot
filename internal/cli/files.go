package cli

import (
	"os"
	"path/filepath"
	"strings"

	"dot/internal/core"
	"dot/internal/tmpl"
)

// cmdAdd implements CMD-12.
func cmdAdd(c *ctx, args []string) int {
	if len(args) != 1 {
		return c.usage("add")
	}
	path := args[0]
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.env.Cwd, path)
	}
	path = filepath.Clean(path)
	st, err := os.Stat(path)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	if !st.Mode().IsRegular() {
		c.errf("%s: not a regular file", path)
		return exitFail
	}
	name, err := c.w.Derive(path)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	if c.w.IsSkipped(name) {
		c.errf("%s is skipped by a skip entry in config", name)
		return exitFail
	}
	tracked, err := fileTracked(c, name)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	if !tracked {
		live, err := os.ReadFile(path)
		if err != nil {
			c.errf("%v", err)
			return exitFail
		}
		return fileAddNew(c, name, live)
	}
	return fileAddTracked(c, name)
}

func fileTracked(c *ctx, name string) (bool, error) {
	names, err := c.w.Files()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

func fileAddNew(c *ctx, name string, live []byte) int {
	if err := core.WriteAtomic(c.w.RepoPath(name), live); err != nil {
		c.errf("%v", err)
		return exitFail
	}
	rel := core.FilesDir + "/" + name
	if _, err := c.w.Git().Output("add", "--", rel); err != nil {
		c.errf("%s: %v", name, err)
		return exitFail
	}
	c.w.SetHash(name, live)
	if err := c.w.SaveState(); err != nil {
		c.errf("%v", err)
		return exitFail
	}
	c.outf("added %s", name)
	// The copy is now the repository form, so placeholder-like text in it is
	// rendered at the next apply.
	if names := tmpl.Names(c.w.Delims, live); len(names) > 0 {
		c.warnf("%s: the content has placeholder text %s; the next render replaces it with the value or rejects an undefined name; keep it literal with a variable NAME = %sNAME%s, or change the repository copy: dot edit %s",
			name, fileQuote(c, names), c.w.Delims.Prefix, c.w.Delims.Suffix, name)
	}
	return exitOK
}

func fileAddTracked(c *ctx, name string) int {
	info, err := c.w.Info(name)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	var result string
	switch info.Status {
	case core.InSync:
		result = "unchanged"
	case core.LiveChanged:
		source, dropped, err := tmpl.Capture(c.w.Delims, info.Source, info.Live, c.lookup())
		if err != nil {
			c.errf("%s: capture failed: %v", name, err)
			c.errf("%s: fix by hand: dot edit %s, and change the repository copy until its render is equal to the live file", name, name)
			return exitFail
		}
		if err := core.WriteAtomic(info.RepoPath, source); err != nil {
			c.errf("%v", err)
			return exitFail
		}
		c.warnDrops(name, dropped)
		result = "captured"
	case core.BothChanged:
		if err := core.WriteAtomic(info.RepoPath, info.Live); err != nil {
			c.errf("%v", err)
			return exitFail
		}
		for _, n := range tmpl.Names(c.w.Delims, info.Source) {
			c.warnf("%s: placeholder %s of the old repository copy is replaced by live text", name, fileQuote(c, []string{n}))
		}
		result = "replaced (live wins)"
	case core.RepoChanged:
		c.errf("%s: repo-changed: the live file has no new content; to write it: dot apply %s", name, name)
		return exitFail
	case core.Unresolved:
		c.errf("%s: unresolved: %s", name, strings.Join(info.Reasons, "; "))
		return exitFail
	default:
		c.errf("%s: cannot add a file with status %s", name, info.Status)
		return exitFail
	}
	c.w.SetHash(name, info.Live)
	if err := c.w.SaveState(); err != nil {
		c.errf("%v", err)
		return exitFail
	}
	c.outf("%s %s", result, name)
	return exitOK
}

// fileQuote shows placeholder names in placeholder form.
func fileQuote(c *ctx, names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = c.w.Delims.Prefix + n + c.w.Delims.Suffix
	}
	return strings.Join(q, ", ")
}

// cmdEdit implements CMD-13.
func cmdEdit(c *ctx, args []string) int {
	if len(args) != 1 {
		return c.usage("edit")
	}
	names, err := c.w.Match(args[0])
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	if len(names) > 1 {
		c.errf("%s matches more than one tracked file: %s", args[0], strings.Join(names, ", "))
		return exitFail
	}
	return c.runEditor(c.w.RepoPath(names[0]))
}

// fileSelect returns all tracked files, or the matches of the one argument (CMD-4).
func fileSelect(c *ctx, args []string) ([]string, error) {
	if len(args) == 0 {
		return c.w.Files()
	}
	return c.w.Match(args[0])
}

// cmdApply implements CMD-18 and section 7.2.
func cmdApply(c *ctx, args []string) int {
	force := false
	var rest []string
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "-"):
			return c.usage("apply")
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 1 {
		return c.usage("apply")
	}
	names, err := fileSelect(c, rest)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	code := exitOK
	for _, name := range names {
		if !fileApplyOne(c, name, force) {
			code = exitFail
		}
	}
	// Refused files do not stop the state of the other files from being saved.
	if err := c.w.SaveState(); err != nil {
		c.errf("%v", err)
		code = exitFail
	}
	return code
}

// fileApplyOne returns false if the file was refused, unresolved or not written.
func fileApplyOne(c *ctx, name string, force bool) bool {
	info, err := c.w.Info(name)
	if err != nil {
		c.errf("%v", err)
		return false
	}
	switch info.Status {
	case core.Skipped:
		return true
	case core.InSync:
		c.w.SetHash(name, info.Live)
		return true
	case core.RepoChanged, core.Missing:
		return fileWriteLive(c, info)
	case core.LiveChanged:
		if force {
			return fileWriteLive(c, info)
		}
		c.outf("kept %s: live-changed; dot push or dot pull will capture it", name)
		return true
	case core.BothChanged:
		if force {
			return fileWriteLive(c, info)
		}
		c.errf("%s: both-changed; refused. Live wins: dot add %s. Repository wins: dot apply --force %s", name, name, name)
		return false
	case core.Unresolved:
		c.errf("%s: unresolved: %s", name, strings.Join(info.Reasons, "; "))
		return false
	}
	c.errf("%s: unknown status %s", name, info.Status)
	return false
}

func fileWriteLive(c *ctx, info core.FileInfo) bool {
	if err := core.WriteAtomic(info.LivePath, info.Rendered); err != nil {
		c.errf("%s: %v", info.Name, err)
		return false
	}
	c.w.SetHash(info.Name, info.Rendered)
	c.outf("wrote %s", info.Name)
	return true
}
