package core

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/rehael/dot/internal/tmpl"
)

type Status int

const (
	InSync Status = iota
	RepoChanged
	LiveChanged
	BothChanged
	Missing
	Skipped
	Unresolved
)

func (s Status) String() string {
	switch s {
	case InSync:
		return "in-sync"
	case RepoChanged:
		return "repo-changed"
	case LiveChanged:
		return "live-changed"
	case BothChanged:
		return "both-changed"
	case Missing:
		return "missing"
	case Skipped:
		return "skipped"
	case Unresolved:
		return "unresolved"
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

type FileInfo struct {
	Name, RepoPath, LivePath string
	Status                   Status
	Reasons                  []string
	Source, Rendered, Live   []byte
}

// Info applies the status rules of section 7.1 in order; the first match wins.
func (w *Workspace) Info(name string) (FileInfo, error) {
	tracked, err := w.isTracked(name)
	if err != nil {
		return FileInfo{}, err
	}
	if !tracked {
		return FileInfo{}, fmt.Errorf("not a tracked file: %s", name)
	}
	fi := FileInfo{Name: name, RepoPath: w.RepoPath(name)}
	if fi.Source, err = os.ReadFile(fi.RepoPath); err != nil {
		return FileInfo{}, err
	}
	live, rootReason := w.livePath(name)
	fi.LivePath = live
	rendered, undefined := tmpl.Render(w.Delims, fi.Source, w.lookup)
	fi.Rendered = rendered

	if w.IsSkipped(name) {
		// A skipped live file is never touched, so it is not read either.
		fi.Status = Skipped
		return fi, nil
	}
	if rootReason != "" {
		fi.Reasons = append(fi.Reasons, rootReason)
	}
	for _, n := range undefined {
		fi.Reasons = append(fi.Reasons, "placeholder "+n+" is undefined")
	}
	exists := false
	if live != "" {
		if fi.Live, exists, err = readLive(live); err != nil {
			return FileInfo{}, err
		}
	}
	h, hasH := w.state[name]
	switch {
	case len(fi.Reasons) > 0:
		fi.Status = Unresolved
	case !exists:
		fi.Status = Missing
	case bytes.Equal(fi.Live, fi.Rendered):
		fi.Status = InSync
	case hasH && hashHex(fi.Live) == h:
		fi.Status = RepoChanged
	case hasH && hashHex(fi.Rendered) == h:
		fi.Status = LiveChanged
	default:
		fi.Status = BothChanged
	}
	return fi, nil
}

func (w *Workspace) isTracked(name string) (bool, error) {
	names, err := w.Files()
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

// readLive reports a live path that is not a regular file as an error,
// because no status rule describes it.
func readLive(path string) (data []byte, exists bool, err error) {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s: not a regular file", path)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}
