// Package kv reads and writes line-format files.
package kv

import (
	"bytes"
	"strings"
)

type Entry struct{ Key, Value string }

func Parse(data []byte) []Entry {
	var entries []Entry
	for _, line := range lines(data) {
		if k, v, ok := parseLine(line); ok {
			entries = append(entries, Entry{Key: k, Value: v})
		}
	}
	return entries
}

func Get(entries []Entry, key string) (string, bool) {
	for _, e := range entries {
		if e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}

func All(entries []Entry, key string) []string {
	var values []string
	for _, e := range entries {
		if e.Key == key {
			values = append(values, e.Value)
		}
	}
	return values
}

// Set falls back to the stub line so that a value entered after "dot var"
// takes the place of the stub that CMD-14 wrote (ST-10).
func Set(data []byte, key, value string) []byte {
	ls := lines(data)
	i := indexOf(ls, func(l []byte) bool {
		k, _, ok := parseLine(l)
		return ok && k == key
	})
	if i < 0 {
		i = indexOf(ls, func(l []byte) bool { return isStub(l, key) })
	}
	if i < 0 {
		return Append(data, key, value)
	}
	out := make([]byte, 0, len(data)+len(key)+len(value)+4)
	for j, l := range ls {
		if j == i {
			out = append(out, formatLine(key, value)...)
		} else {
			out = append(out, l...)
		}
	}
	return out
}

func Append(data []byte, key, value string) []byte {
	out := make([]byte, 0, len(data)+len(key)+len(value)+5)
	out = append(out, data...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(out, formatLine(key, value)...)
}

// lines splits data into lines that keep their line ends, so that a
// rewrite can copy untouched lines byte for byte.
func lines(data []byte) [][]byte {
	ls := bytes.SplitAfter(data, []byte("\n"))
	if len(ls) > 0 && len(ls[len(ls)-1]) == 0 {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// parseLine applies ST-8. TrimSpace also removes the CR of a CRLF line end.
func parseLine(line []byte) (key, value string, ok bool) {
	s := strings.TrimSpace(string(line))
	if s == "" || s[0] == '#' {
		return "", "", false
	}
	k, v, found := strings.Cut(s, "=")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

func isStub(line []byte, key string) bool {
	s := strings.TrimSpace(string(line))
	rest, isComment := strings.CutPrefix(s, "#")
	if !isComment {
		return false
	}
	k, v, found := strings.Cut(rest, "=")
	return found && strings.TrimSpace(k) == key && strings.TrimSpace(v) == ""
}

func indexOf(ls [][]byte, match func([]byte) bool) int {
	for i, l := range ls {
		if match(l) {
			return i
		}
	}
	return -1
}

func formatLine(key, value string) string {
	return key + " = " + value + "\n"
}
