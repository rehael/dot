package cli

import (
	"errors"
	"strings"

	"dot/internal/core"
	"dot/internal/gitx"
	"dot/internal/tmpl"
)

// remoteCapture holds one capture result until all captures are known (SYNC-14).
type remoteCapture struct {
	info    core.FileInfo
	source  []byte
	dropped []tmpl.Drop
}

// cmdPush applies CMD-19.
func cmdPush(c *ctx, args []string) int {
	message := remoteDefaultMessage(c)
	switch {
	case len(args) == 0:
	case len(args) == 2 && args[0] == "-m" && strings.TrimSpace(args[1]) != "":
		message = args[1]
	default:
		return c.usage("push")
	}
	if code, ok := remoteSync(c, message); !ok {
		return code
	}
	has, ok := remoteHasRemote(c)
	if !ok {
		return exitFail
	}
	if !has {
		c.outf("no remote; nothing pushed")
		return exitOK
	}
	pushArgs := []string{"push"}
	if _, err := c.w.Git().Output("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err != nil {
		var ee *gitx.ExitError
		if !errors.As(err, &ee) {
			c.errf("%v", err)
			return exitFail
		}
		pushArgs = []string{"push", "-u", "origin", "HEAD"}
	}
	code := remoteGit(c, pushArgs...)
	if code != exitOK {
		c.errf("run: dot pull")
	}
	return code
}

// cmdPull applies CMD-20. It never writes a live file.
func cmdPull(c *ctx, args []string) int {
	if len(args) != 0 {
		return c.usage("pull")
	}
	if code, ok := remoteSync(c, remoteDefaultMessage(c)); !ok {
		return code
	}
	has, ok := remoteHasRemote(c)
	if !ok {
		return exitFail
	}
	if !has {
		c.errf("no remote; pull needs a git remote")
		return exitFail
	}
	if code := remoteGit(c, "pull"); code != exitOK {
		c.errf("git pull failed; resolve the conflict with git, then run: dot apply")
		return code
	}
	// The pull can change dot.conf (delimiters), so the status is computed
	// from a workspace that is loaded again.
	w, err := core.Open(c.env)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	names, err := w.Files()
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	n := 0
	for _, name := range names {
		info, err := w.Info(name)
		if err != nil {
			c.errf("%s: %v", name, err)
			return exitFail
		}
		if info.Status == core.RepoChanged {
			n++
		}
	}
	if n == 0 {
		c.outf("0 file(s) repo-changed")
	} else {
		c.outf("%d file(s) repo-changed; run: dot apply", n)
	}
	return exitOK
}

func remoteDefaultMessage(c *ctx) string {
	return c.w.Machine + ": sync" // A7
}

// remoteSync is steps 1 to 3 of CMD-19: capture, warn, commit. ok is false
// when the command must stop with code.
func remoteSync(c *ctx, message string) (code int, ok bool) {
	names, err := c.w.Files()
	if err != nil {
		c.errf("%v", err)
		return exitFail, false
	}
	// All status reads come first, so that an error stops before any write.
	infos := make([]core.FileInfo, 0, len(names))
	for _, name := range names {
		info, err := c.w.Info(name)
		if err != nil {
			c.errf("%s: %v", name, err)
			return exitFail, false
		}
		infos = append(infos, info)
	}

	var captures []remoteCapture
	failed := false
	for _, info := range infos {
		if info.Status != core.LiveChanged {
			continue
		}
		source, dropped, err := tmpl.Capture(c.w.Delims, info.Source, info.Live, c.lookup())
		if err != nil {
			c.errf("%s: capture failed: %v; fix: dot edit %s, change the repository copy until its render is equal to the live file", info.Name, err, info.Name)
			failed = true
			continue
		}
		captures = append(captures, remoteCapture{info: info, source: source, dropped: dropped})
	}
	if failed {
		return exitFail, false
	}

	for _, cp := range captures {
		if err := core.WriteAtomic(cp.info.RepoPath, cp.source); err != nil {
			c.errf("%s: %v", cp.info.Name, err)
			return exitFail, false
		}
		c.warnDrops(cp.info.Name, cp.dropped)
		c.w.SetHash(cp.info.Name, cp.info.Live)
		c.outf("captured %s", cp.info.Name)
	}
	for _, info := range infos {
		switch info.Status {
		case core.InSync:
			c.w.SetHash(info.Name, info.Live)
		case core.BothChanged:
			// add takes a live path; printing it would print a root value (NF-6).
			c.warnf("%s: both-changed, not captured; see: dot diff %s; then: dot add <live file> (live wins) or dot apply --force %s (repository wins)", info.Name, info.Name, info.Name)
		}
	}
	if err := c.w.SaveState(); err != nil {
		c.errf("%v", err)
		return exitFail, false
	}

	if code := remoteGit(c, "add", "-A"); code != exitOK {
		return code, false
	}
	switch code := remoteGit(c, "diff", "--cached", "--quiet"); code {
	case 0:
		return exitOK, true
	case 1:
		if code := remoteGit(c, "commit", "-m", message); code != exitOK {
			return code, false
		}
		return exitOK, true
	default:
		return code, false
	}
}

// remoteGit runs git with its output on the streams of dot and returns its
// exit code.
func remoteGit(c *ctx, args ...string) int {
	code, err := c.w.Git().Run(args...)
	if err != nil {
		c.errf("git %s: %v", args[0], err)
		return exitFail
	}
	return code
}

// remoteHasRemote reports whether the repository has a git remote. ok is
// false, after a message, when git fails.
func remoteHasRemote(c *ctx) (has, ok bool) {
	out, err := c.w.Git().Output("remote")
	if err != nil {
		c.errf("%v", err)
		return false, false
	}
	return strings.TrimSpace(out) != "", true
}
