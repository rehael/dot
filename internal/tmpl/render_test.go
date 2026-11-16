package tmpl

import (
	"bytes"
	"slices"
	"testing"
)

func TestRender(t *testing.T) {
	look := mapLook(map[string]string{"a": "X", "host": "alpha", "A": "upper"})
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain\n", "plain\n"},
		{"h={{host}}\n", "h=alpha\n"},
		{"{{a}}{{a}}", "XX"},
		{"{{a}} {{A}}", "X upper"},
		{"{{{a}}}", "{X}"},
		{"{{ a }} {{a}", "{{ a }} {{a}"},
		{"h={{host}}\r\nend\r\n", "h=alpha\r\nend\r\n"},
	}
	for _, c := range cases {
		out, undef := Render(def, []byte(c.in), look)
		if undef != nil {
			t.Errorf("Render(%q): undefined %q", c.in, undef)
			continue
		}
		if out == nil || string(out) != c.want {
			t.Errorf("Render(%q) = %q, want %q", c.in, out, c.want)
		}
	}
}

// VAR-7: one pass; a value that looks like a placeholder is not scanned again.
func TestRender_VAR7_OnePass(t *testing.T) {
	look := mapLook(map[string]string{"a": "{{b}}", "b": "B", "self": "{{self}}"})
	out, undef := Render(def, []byte("{{a}} {{self}} {{b}}"), look)
	if undef != nil || string(out) != "{{b}} {{self}} B" {
		t.Errorf("Render = %q, %q", out, undef)
	}
}

// VAR-7: an undefined name fails the whole render; no partial output.
func TestRender_VAR7_Undefined(t *testing.T) {
	look := mapLook(map[string]string{"a": "X"})
	out, undef := Render(def, []byte("{{y}} {{a}} {{x}} {{y}}"), look)
	if out != nil {
		t.Errorf("out = %q, want nil", out)
	}
	if want := []string{"y", "x"}; !slices.Equal(undef, want) {
		t.Errorf("undefined = %q, want %q", undef, want)
	}
}

// VAR-8: foo = {{foo}} keeps the literal text.
func TestRender_VAR8_Literal(t *testing.T) {
	look := mapLook(map[string]string{"foo": "{{foo}}"})
	out, undef := Render(def, []byte("x {{foo}} y\n"), look)
	if undef != nil || string(out) != "x {{foo}} y\n" {
		t.Errorf("Render = %q, %q", out, undef)
	}
}

// VAR-9: render works on bytes; UTF-16 content and invalid UTF-8 pass
// through unchanged except for ASCII placeholders.
func TestRender_VAR9_Bytes(t *testing.T) {
	look := mapLook(map[string]string{"a": "X"})
	in := append([]byte{0xff, 0xfe}, utf16le("v={{a}}\r\n")...)
	out, undef := Render(def, in, look)
	if undef != nil || !bytes.Equal(out, in) {
		t.Errorf("Render(UTF-16) = %q, %q", out, undef)
	}
	out, undef = Render(def, []byte("\xff\x00{{a}}\x00\xfe"), look)
	if undef != nil || string(out) != "\xff\x00X\x00\xfe" {
		t.Errorf("Render(binary) = %q, %q", out, undef)
	}
}

func TestRender_EmptyIsNotNil(t *testing.T) {
	out, undef := Render(def, nil, mapLook(nil))
	if out == nil || len(out) != 0 || undef != nil {
		t.Errorf("Render(nil) = %#v, %q", out, undef)
	}
}

func TestRender_CustomDelims(t *testing.T) {
	out, undef := Render(Delims{"<%", "%>"}, []byte("<%a%> {{a}} <% a %>"), mapLook(map[string]string{"a": "X"}))
	if undef != nil || string(out) != "X {{a}} <% a %>" {
		t.Errorf("Render = %q, %q", out, undef)
	}
}
