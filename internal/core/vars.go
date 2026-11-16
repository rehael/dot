package core

import (
	"path/filepath"
	"sort"

	"github.com/rehael/dot/internal/kv"
	"github.com/rehael/dot/internal/tmpl"
)

type Var struct{ Name, Value, Origin string }

// builtins are the only environment variables read as variables (VAR-3, VAR-4).
var builtins = []string{"USERPROFILE", "APPDATA", "LOCALAPPDATA"}

// loadVars ignores entries that VAR-1 or VAR-2 rejects, so that a bad line
// cannot shadow a built-in name.
func (w *Workspace) loadVars() error {
	data, err := readOptional(filepath.Join(w.LocalDir, VarsFile))
	if err != nil {
		return err
	}
	w.vars = map[string]string{}
	for _, e := range kv.Parse(data) {
		if e.Value == "" || !tmpl.ValidName(e.Key) {
			continue
		}
		if _, dup := w.vars[e.Key]; !dup {
			w.vars[e.Key] = e.Value
		}
	}
	return nil
}

// Lookup applies VAR-3.
func (w *Workspace) Lookup(name string) (value, origin string, ok bool) {
	if v, ok := w.vars[name]; ok {
		return v, OriginVars, true
	}
	for _, b := range builtins {
		if b == name {
			if v := w.Env.getenv(name); v != "" {
				return v, OriginEnv, true
			}
			return "", "", false
		}
	}
	return "", "", false
}

func (w *Workspace) Vars() []Var {
	var out []Var
	for name, value := range w.vars {
		out = append(out, Var{Name: name, Value: value, Origin: OriginVars})
	}
	for _, b := range builtins {
		if _, shadowed := w.vars[b]; shadowed {
			continue
		}
		if v := w.Env.getenv(b); v != "" {
			out = append(out, Var{Name: b, Value: v, Origin: OriginEnv})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// lookup adapts Lookup to the template package.
func (w *Workspace) lookup(name string) (string, bool) {
	v, _, ok := w.Lookup(name)
	return v, ok
}
