// Package tmpl scans, renders and captures placeholders.
//
// All operations work on bytes, so content in an encoding that is not
// ASCII-compatible has no placeholders and passes through unchanged (VAR-9).
package tmpl

// Delims are the placeholder delimiters from dot.conf (VAR-5).
type Delims struct{ Prefix, Suffix string }

// Lookup returns the value of a variable and whether it is defined.
type Lookup func(name string) (value string, ok bool)

// Drop is a placeholder that a capture could not keep (SYNC-10, SYNC-15).
type Drop struct {
	Name string
	Line int // 1-based, in the live file
}
