package tmpl

import (
	"slices"
	"testing"
)

var def = Delims{Prefix: "{{", Suffix: "}}"}

func mapLook(m map[string]string) Lookup {
	return func(name string) (string, bool) {
		v, ok := m[name]
		return v, ok
	}
}

// utf16le encodes ASCII text as UTF-16LE: each byte is followed by NUL.
func utf16le(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for i := 0; i < len(s); i++ {
		out = append(out, s[i], 0)
	}
	return out
}

func TestValidName_VAR1(t *testing.T) {
	cases := map[string]bool{
		"a":        true,
		"Z":        true,
		"9":        true,
		"A_b-9":    true,
		"-":        true,
		"_":        true,
		"":         false,
		"a b":      false,
		"a.b":      false,
		"a/b":      false,
		"a\n":      false,
		"\xc3\xa9": false,
		"{{a}}":    false,
		"USER$":    false,
		"Case_OK":  true,
	}
	for name, want := range cases {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestValidate_VAR5(t *testing.T) {
	cases := []struct {
		d  Delims
		ok bool
	}{
		{def, true},
		{Delims{"<%", "%>"}, true},
		{Delims{"${", "}"}, true},
		{Delims{"x{", "}x"}, true},
		{Delims{"", "}}"}, false},
		{Delims{"{{", ""}, false},
		{Delims{"{ {", "}}"}, false},
		{Delims{"{{", "}\t}"}, false},
		{Delims{"{{\n", "}}"}, false},
		{Delims{"{{\xc2\xa0", "}}"}, false},
		{Delims{"{a", "}}"}, false},
		{Delims{"{_", "}}"}, false},
		{Delims{"{-", "}}"}, false},
		{Delims{"{{", "a}"}, false},
		{Delims{"{{", "-}"}, false},
		{Delims{"{{", "9}"}, false},
	}
	for _, c := range cases {
		err := c.d.Validate()
		if (err == nil) != c.ok {
			t.Errorf("Validate(%q, %q) = %v, want ok=%v", c.d.Prefix, c.d.Suffix, err, c.ok)
		}
	}
}

func TestNames_VAR6(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"no placeholder", nil},
		{"{{a}}", []string{"a"}},
		{"x{{a-b_C9}}y", []string{"a-b_C9"}},
		{"{{ a }}", nil},
		{"{{a }}", nil},
		{"{{a b}}", nil},
		{"{{a.b}}", nil},
		{"{{}}", nil},
		{"{{a}", nil},
		{"{a}}", nil},
		{"{{\xc3\xa9}}", nil},
		// A failed form resumes one byte after the prefix start.
		{"{{{a}}}", []string{"a"}},
		{"{{{{a}}}}", []string{"a"}},
		{"{{x {{a}}", []string{"a"}},
		{"{{a}}{{b}}", []string{"a", "b"}},
		{"{{a}}}}", []string{"a"}},
		// Unique, order of first occurrence.
		{"{{b}} {{a}} {{b}} {{a}}", []string{"b", "a"}},
		// Case-sensitive (VAR-1).
		{"{{a}} {{A}}", []string{"a", "A"}},
	}
	for _, c := range cases {
		if got := Names(def, []byte(c.in)); !slices.Equal(got, c.want) {
			t.Errorf("Names(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNames_VAR6_CustomDelims(t *testing.T) {
	d := Delims{"${", "}"}
	got := Names(d, []byte("$HOME ${HOME} ${ X} ${USER}} {{a}}"))
	if want := []string{"HOME", "USER"}; !slices.Equal(got, want) {
		t.Errorf("Names = %q, want %q", got, want)
	}
}

// VAR-9: a UTF-16 file has no placeholders because NUL bytes separate the
// delimiter and name bytes.
func TestNames_VAR9_UTF16(t *testing.T) {
	in := append([]byte{0xff, 0xfe}, utf16le("key={{a}}\r\n")...)
	if got := Names(def, in); got != nil {
		t.Errorf("Names(UTF-16) = %q, want none", got)
	}
}
