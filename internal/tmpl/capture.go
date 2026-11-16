package tmpl

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Capture moves an edit of the live content into the repository copy and
// keeps the placeholders (SYNC-9 to SYNC-12). On error newSource is nil.
// Error messages name placeholders, never values (NF-6).
func Capture(d Delims, source, live []byte, look Lookup) (newSource []byte, dropped []Drop, err error) {
	if err := d.Validate(); err != nil {
		return nil, nil, err
	}
	rendered, undefined := Render(d, source, look)
	if len(undefined) > 0 {
		return nil, nil, fmt.Errorf("repository copy has undefined placeholder names: %s", strings.Join(undefined, ", "))
	}

	var result []byte
	if len(scan(d, source)) == 0 {
		result = append([]byte{}, live...) // SYNC-9
	} else {
		result, dropped, err = merge(d, source, rendered, live, look)
		if err != nil {
			return nil, nil, err
		}
	}

	// SYNC-11: applied to both paths, because a result that does not render
	// back to the live bytes would change the live file at the next apply.
	check, undefined := Render(d, result, look)
	if len(undefined) > 0 {
		return nil, nil, fmt.Errorf("capture result has undefined placeholder names: %s", strings.Join(undefined, ", "))
	}
	if !bytes.Equal(check, live) {
		return nil, nil, errors.New("capture result does not render to the live content; edit the repository copy by hand")
	}
	return result, dropped, nil
}

// merge implements SYNC-10.
func merge(d Delims, source, rendered, live []byte, look Lookup) ([]byte, []Drop, error) {
	s, b, v := splitLines(source), splitLines(rendered), splitLines(live)
	if len(s) != len(b) {
		return nil, nil, errors.New("rendered repository copy has a different number of lines; a variable value contains a line end")
	}
	out := make([]byte, 0, len(source)+len(live))
	var dropped []Drop
	next := 0
	for _, blk := range diffLines(b, v) {
		for _, l := range s[next:blk.i1] {
			out = append(out, l...)
		}
		text, drops := replaceBlock(d, s[blk.i1:blk.i2], v[blk.j1:blk.j2], blk.j1+1, look)
		out = append(out, text...)
		dropped = append(dropped, drops...)
		next = blk.i2
	}
	for _, l := range s[next:] {
		out = append(out, l...)
	}
	return out, dropped, nil
}

// replaceBlock puts back the placeholders of the source lines src into the
// live lines of one changed block (SYNC-10 steps 4 and 5). line is the
// 1-based line number in the live content where the block starts.
func replaceBlock(d Delims, src, live [][]byte, line int, look Lookup) ([]byte, []Drop) {
	text := bytes.Join(live, nil)
	names := Names(d, bytes.Join(src, nil))
	if len(names) == 0 {
		return text, nil
	}

	type cand struct {
		name  string
		value []byte
	}
	var cands []cand
	for _, n := range names {
		if v, ok := look(n); ok && v != "" {
			cands = append(cands, cand{n, []byte(v)})
		}
	}
	// The first candidate that matches is then the longest; equal length
	// goes to the name that sorts first.
	slices.SortFunc(cands, func(p, q cand) int {
		if c := cmp.Compare(len(q.value), len(p.value)); c != 0 {
			return c
		}
		return strings.Compare(p.name, q.name)
	})

	used := map[string]bool{}
	out := make([]byte, 0, len(text))
	for i := 0; i < len(text); {
		matched := false
		for _, c := range cands {
			if bytes.HasPrefix(text[i:], c.value) {
				out = append(out, d.Prefix...)
				out = append(out, c.name...)
				out = append(out, d.Suffix...)
				used[c.name] = true
				i += len(c.value)
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, text[i])
			i++
		}
	}

	var drops []Drop
	for _, n := range names {
		if !used[n] {
			drops = append(drops, Drop{Name: n, Line: max(line, 1)})
		}
	}
	return out, drops
}

// splitLines splits content after each LF and keeps the terminators. In a
// CRLF file the CR stays part of the line content. The last line can lack a
// terminator.
func splitLines(content []byte) [][]byte {
	var lines [][]byte
	for len(content) > 0 {
		k := bytes.IndexByte(content, '\n')
		if k < 0 {
			lines = append(lines, content)
			break
		}
		lines = append(lines, content[:k+1])
		content = content[k+1:]
	}
	return lines
}
