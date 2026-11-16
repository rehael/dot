package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/rehael/dot/internal/core"
	"github.com/rehael/dot/internal/gitx"
	"github.com/rehael/dot/internal/kv"
	"github.com/rehael/dot/internal/tmpl"
)

const (
	setupAttributes = "* -text\n"
	setupRepoConf   = "prefix = {{\nsuffix = }}\n"
)

// cmdInit applies CMD-10.
func cmdInit(c *ctx, args []string) int {
	if len(args) > 1 {
		return c.usage("init")
	}
	target := c.w.RepoDir
	if len(args) == 1 {
		target = args[0]
		if !filepath.IsAbs(target) {
			target = filepath.Join(c.env.Cwd, target)
		}
		target = filepath.Clean(target)
	}
	configPath := filepath.Join(c.w.LocalDir, core.ConfigFile)
	config, err := setupReadOptional(configPath)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	repo, hasRepo := kv.Get(kv.Parse(config), "repo")
	if hasRepo && !setupSamePath(repo, target) {
		c.errf("config already points to a different repository: %s", repo)
		return exitFail
	}

	created, err := setupRepository(c, target)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	if !hasRepo && !setupSamePath(target, filepath.Join(c.w.LocalDir, core.DefaultRepo)) {
		if err := core.WriteAtomic(configPath, kv.Set(config, "repo", target)); err != nil {
			c.errf("%v", err)
			return exitFail
		}
	}
	if created {
		c.outf("created repository %s", target)
	} else {
		c.outf("adopted repository %s", target)
	}
	return exitOK
}

// setupRepository makes target a repository of dot and reports whether it
// had to create the git work tree.
func setupRepository(c *ctx, target string) (created bool, err error) {
	isTree, err := setupExists(filepath.Join(target, ".git"))
	if err != nil {
		return false, err
	}
	if isTree {
		hasConf, err := setupExists(filepath.Join(target, core.RepoConfFile))
		if err != nil {
			return false, err
		}
		if hasConf {
			// A clone from another machine: its files belong to the repository.
			return false, nil
		}
		return false, setupRepoFiles(target)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return false, err
	}
	g := gitx.Git{Dir: target, Stdout: c.env.Stdout, Stderr: c.env.Stderr}
	code, err := g.Run("init", "-q")
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, errors.New("git init failed in " + target)
	}
	return true, setupRepoFiles(target)
}

// setupRepoFiles adds dot.conf and the attribute line without loss of
// content that the directory already has.
func setupRepoFiles(target string) error {
	confPath := filepath.Join(target, core.RepoConfFile)
	hasConf, err := setupExists(confPath)
	if err != nil {
		return err
	}
	if !hasConf {
		if err := core.WriteAtomic(confPath, []byte(setupRepoConf)); err != nil {
			return err
		}
	}
	attrPath := filepath.Join(target, ".gitattributes")
	attr, err := setupReadOptional(attrPath)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(attr), "\n") {
		if strings.TrimSpace(line) == strings.TrimSpace(setupAttributes) {
			return nil
		}
	}
	if len(attr) > 0 && attr[len(attr)-1] != '\n' {
		attr = append(attr, '\n')
	}
	return core.WriteAtomic(attrPath, append(attr, setupAttributes...))
}

// cmdRegister applies CMD-11.
func cmdRegister(c *ctx, args []string) int {
	if len(args) > 1 {
		return c.usage("register")
	}
	var name string
	if len(args) == 1 {
		name = args[0]
	} else {
		host, err := os.Hostname()
		if err != nil {
			c.errf("cannot read the host name: %v", err)
			return exitFail
		}
		name = setupMachineName(host)
	}
	if !tmpl.ValidName(name) {
		c.errf("machine name is not valid: %q; allowed characters: A-Z a-z 0-9 _ -", name)
		return exitFail
	}
	confPath := filepath.Join(c.w.RepoDir, core.RepoConfFile)
	hasConf, err := setupExists(confPath)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	if !hasConf {
		c.errf("no repository; run: dot init")
		return exitFail
	}
	conf, err := os.ReadFile(confPath)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	if c.w.Machine != "" && c.w.Machine != name {
		c.errf("this machine is already registered as %s", c.w.Machine)
		return exitFail
	}
	// Write only on a change, so that a repeated run changes no byte (CMD-11).
	if c.w.Machine != name {
		configPath := filepath.Join(c.w.LocalDir, core.ConfigFile)
		config, err := setupReadOptional(configPath)
		if err == nil {
			err = core.WriteAtomic(configPath, kv.Set(config, "machine", name))
		}
		if err != nil {
			c.errf("%v", err)
			return exitFail
		}
	}
	if !setupContains(c.w.Machines, name) {
		if err := core.WriteAtomic(confPath, kv.Append(conf, "machine", name)); err != nil {
			c.errf("%v", err)
			return exitFail
		}
	}
	c.outf("registered machine %s", name)
	return exitOK
}

// setupMachineName derives the default machine name from a host name (CMD-11).
func setupMachineName(host string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(host) {
		if r < 0x80 && tmpl.ValidName(string(r)) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// cmdVar applies CMD-14.
func cmdVar(c *ctx, args []string) int {
	switch len(args) {
	case 0:
		return setupEditVars(c)
	case 1:
		value, origin, ok := c.w.Lookup(args[0])
		if !ok {
			c.errf("variable %s is undefined", args[0])
			return exitFail
		}
		c.outf("%s = %s", args[0], value)
		c.outf("origin: %s", origin)
		return exitOK
	case 2:
		return setupSetVar(c, args[0], args[1])
	}
	return c.usage("var")
}

// setupSetVar never prints the value (NF-6).
func setupSetVar(c *ctx, name, value string) int {
	if !tmpl.ValidName(name) {
		c.errf("variable name is not valid: %q; allowed characters: A-Z a-z 0-9 _ -", name)
		return exitFail
	}
	value = strings.TrimSpace(value)
	if value == "" {
		c.errf("value of %s is empty", name)
		return exitFail
	}
	if strings.ContainsAny(value, "\r\n") {
		c.errf("value of %s has a line break; a value is one line", name)
		return exitFail
	}
	path := filepath.Join(c.w.LocalDir, core.VarsFile)
	data, err := setupReadOptional(path)
	if err == nil {
		err = core.WriteAtomic(path, kv.Set(data, name, value))
	}
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	return exitOK
}

// setupEditVars appends a stub for each undefined name, then opens vars.
func setupEditVars(c *ctx) int {
	usedBy, err := setupUndefined(c)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	path := filepath.Join(c.w.LocalDir, core.VarsFile)
	exists, err := setupExists(path)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	data, err := setupReadOptional(path)
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	stubs := setupStubs(data)
	names := make([]string, 0, len(usedBy))
	for name := range usedBy {
		if !stubs[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := data
	for _, name := range names {
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, "# used by: "+strings.Join(usedBy[name], ", ")+"\n"...)
		out = append(out, "# "+name+" =\n"...)
	}
	// The editor needs the file, also when it stays empty.
	if !exists || len(names) > 0 {
		if err := core.WriteAtomic(path, out); err != nil {
			c.errf("%v", err)
			return exitFail
		}
	}
	return c.runEditor(path)
}

// setupUndefined maps each undefined root or placeholder name of the tracked
// files that are not skipped to the sorted logical names that use it.
func setupUndefined(c *ctx) (map[string][]string, error) {
	files, err := c.w.Files()
	if err != nil {
		return nil, err
	}
	usedBy := map[string][]string{}
	add := func(name, file string) {
		if _, _, ok := c.w.Lookup(name); ok {
			return
		}
		list := usedBy[name]
		if len(list) > 0 && list[len(list)-1] == file {
			return
		}
		usedBy[name] = append(list, file)
	}
	// Files is sorted, so each list of logical names is sorted too.
	for _, file := range files {
		if c.w.IsSkipped(file) {
			continue
		}
		// A root that is not a valid name can never be defined, so it is no variable (A15).
		if root, _, _ := strings.Cut(file, "/"); tmpl.ValidName(root) {
			add(root, file)
		}
		src, err := os.ReadFile(c.w.RepoPath(file))
		if err != nil {
			return nil, err
		}
		for _, name := range tmpl.Names(c.w.Delims, src) {
			add(name, file)
		}
	}
	return usedBy, nil
}

// setupStubs returns the names that have a stub line "# <name> =" (A20).
func setupStubs(data []byte) map[string]bool {
	stubs := map[string]bool{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(string(line)), "#")
		if !ok {
			continue
		}
		key, value, found := strings.Cut(rest, "=")
		if found && strings.TrimSpace(value) == "" {
			stubs[strings.TrimSpace(key)] = true
		}
	}
	return stubs
}

// cmdCheck applies CMD-15.
func cmdCheck(c *ctx, args []string) int {
	if len(args) > 0 {
		return c.usage("check")
	}
	files, err := c.w.Files()
	if err != nil {
		c.errf("%v", err)
		return exitFail
	}
	code := exitOK
	for _, file := range files {
		if c.w.IsSkipped(file) {
			continue
		}
		info, err := c.w.Info(file)
		if err != nil {
			c.errf("%v", err)
			code = exitFail
			continue
		}
		if info.Status != core.Unresolved {
			continue
		}
		for _, reason := range info.Reasons {
			c.outf("%s: %s", file, reason)
		}
		code = exitFail
	}
	return code
}

// setupReadOptional reads a file that may not exist yet (ST-3): nil, no error.
func setupReadOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func setupExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// setupSamePath compares two paths after Clean; Windows paths compare without case.
func setupSamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func setupContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
