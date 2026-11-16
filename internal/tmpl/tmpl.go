// Package tmpl scans, renders and captures placeholders.
package tmpl

type Delims struct{ Prefix, Suffix string }

type Lookup func(name string) (value string, ok bool)

type Drop struct {
	Name string
	Line int
}

func (d Delims) Validate() error { panic("not implemented") }

func ValidName(name string) bool { panic("not implemented") }

func Names(d Delims, content []byte) []string { panic("not implemented") }

func Render(d Delims, content []byte, look Lookup) (out []byte, undefined []string) {
	panic("not implemented")
}

func Capture(d Delims, source, live []byte, look Lookup) (newSource []byte, dropped []Drop, err error) {
	panic("not implemented")
}
