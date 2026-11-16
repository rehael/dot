package tmpl

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// ValidName reports whether name matches [A-Za-z0-9_-]+ (VAR-1).
func ValidName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isNameByte(name[i]) {
			return false
		}
	}
	return true
}

// Validate checks the delimiter rules of VAR-5. They make the end of a name
// unambiguous and keep a placeholder inside one line.
func (d Delims) Validate() error {
	switch {
	case d.Prefix == "":
		return errors.New("placeholder prefix is empty")
	case d.Suffix == "":
		return errors.New("placeholder suffix is empty")
	case strings.ContainsFunc(d.Prefix, unicode.IsSpace):
		return fmt.Errorf("placeholder prefix %q contains white space", d.Prefix)
	case strings.ContainsFunc(d.Suffix, unicode.IsSpace):
		return fmt.Errorf("placeholder suffix %q contains white space", d.Suffix)
	case isNameByte(d.Prefix[len(d.Prefix)-1]):
		return fmt.Errorf("placeholder prefix %q ends with a name character", d.Prefix)
	case isNameByte(d.Suffix[0]):
		return fmt.Errorf("placeholder suffix %q starts with a name character", d.Suffix)
	}
	return nil
}

// token is one placeholder: content[start:end] is the whole placeholder text.
type token struct {
	start, end int
	name       string
}

// scan returns the placeholders of content in order (VAR-6). When the form
// does not match at a prefix occurrence, scanning resumes one byte after its
// start, so a later prefix that overlaps it can still begin a placeholder.
func scan(d Delims, content []byte) []token {
	var toks []token
	pre, suf := []byte(d.Prefix), []byte(d.Suffix)
	for i := 0; i < len(content); {
		k := bytes.Index(content[i:], pre)
		if k < 0 {
			break
		}
		p := i + k
		n := p + len(pre)
		e := n
		for e < len(content) && isNameByte(content[e]) {
			e++
		}
		if e > n && bytes.HasPrefix(content[e:], suf) {
			toks = append(toks, token{start: p, end: e + len(suf), name: string(content[n:e])})
			i = e + len(suf)
			continue
		}
		i = p + 1
	}
	return toks
}

// Names returns the placeholder names of content: unique, in order of first
// occurrence.
func Names(d Delims, content []byte) []string {
	var names []string
	seen := map[string]bool{}
	for _, t := range scan(d, content) {
		if !seen[t.name] {
			seen[t.name] = true
			names = append(names, t.name)
		}
	}
	return names
}
