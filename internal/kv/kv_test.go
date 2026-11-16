package kv

import (
	"reflect"
	"testing"
)

func TestParseST7LineEnds(t *testing.T) {
	want := []Entry{{"a", "1"}, {"b", "2"}, {"c", "3"}}
	cases := map[string]string{
		"LF":             "a = 1\nb = 2\nc = 3\n",
		"CRLF":           "a = 1\r\nb = 2\r\nc = 3\r\n",
		"mixed":          "a = 1\r\nb = 2\nc = 3\r\n",
		"no final end":   "a = 1\nb = 2\nc = 3",
		"CRLF no final":  "a = 1\r\nb = 2\r\nc = 3",
		"blank CRLF gap": "a = 1\r\n\r\n  \r\nb = 2\r\nc = 3\r\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Parse([]byte(in)); !reflect.DeepEqual(got, want) {
				t.Errorf("Parse(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

func TestParseST7UTF8(t *testing.T) {
	got := Parse([]byte("nazwa = zażółć gęślą jaźń\n"))
	want := []Entry{{"nazwa", "zażółć gęślą jaźń"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseST8LineKinds(t *testing.T) {
	in := "" +
		"\n" +
		"   \t \n" +
		"# comment = not a key\n" +
		"   # indented comment\n" +
		"key = value\n" +
		"  spaced\t=\t value with  inner  spaces  \n" +
		"tight=x\n" +
		"no equals sign here\n" +
		"empty =\n" +
		"eq = a = b\n"
	want := []Entry{
		{"key", "value"},
		{"spaced", "value with  inner  spaces"},
		{"tight", "x"},
		{"empty", ""},
		{"eq", "a = b"},
	}
	if got := Parse([]byte(in)); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseST8Empty(t *testing.T) {
	if got := Parse(nil); len(got) != 0 {
		t.Errorf("Parse(nil) = %v, want none", got)
	}
}

func TestParseST9Literal(t *testing.T) {
	in := "a = x # not a comment\n" +
		"b = \"quoted\"\n" +
		"c = C:\\path\\n\n" +
		"d = #start\n" +
		"e = k=v\n"
	want := []Entry{
		{"a", "x # not a comment"},
		{"b", "\"quoted\""},
		{"c", "C:\\path\\n"},
		{"d", "#start"},
		{"e", "k=v"},
	}
	if got := Parse([]byte(in)); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGetAll(t *testing.T) {
	e := Parse([]byte("machine = a\nprefix = {{\nmachine = b\nmachine = c\n"))
	if v, ok := Get(e, "machine"); !ok || v != "a" {
		t.Errorf("Get machine = %q, %v; want first definition \"a\"", v, ok)
	}
	if v, ok := Get(e, "missing"); ok || v != "" {
		t.Errorf("Get missing = %q, %v; want \"\", false", v, ok)
	}
	if got, want := All(e, "machine"), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("All machine = %v, want %v", got, want)
	}
	if got := All(e, "missing"); len(got) != 0 {
		t.Errorf("All missing = %v, want none", got)
	}
}

func TestGetEmptyValueIsDefined(t *testing.T) {
	if v, ok := Get(Parse([]byte("k =\n")), "k"); !ok || v != "" {
		t.Errorf("got %q, %v; want \"\", true", v, ok)
	}
}

// ST-10: only the applicable line changes; ST-7: the written line ends with LF.
func TestSetST10(t *testing.T) {
	cases := []struct {
		name, in, key, value, want string
	}{
		{
			name:  "replace first definition only",
			in:    "# head\nk = old\nother = 1\nk = second\n",
			key:   "k",
			value: "new",
			want:  "# head\nk = new\nother = 1\nk = second\n",
		},
		{
			name:  "replace indented definition",
			in:    "a = 1\n   k\t=  old  \nb = 2\n",
			key:   "k",
			value: "new",
			want:  "a = 1\nk = new\nb = 2\n",
		},
		{
			name:  "CRLF neighbours kept, written line LF",
			in:    "# c\r\na = 1\r\nk = old\r\nb = 2\r\n",
			key:   "k",
			value: "new",
			want:  "# c\r\na = 1\r\nk = new\nb = 2\r\n",
		},
		{
			name:  "replace stub",
			in:    "# used by: VIM/.gvimrc\n# k =\nz = 9\n",
			key:   "k",
			value: "v",
			want:  "# used by: VIM/.gvimrc\nk = v\nz = 9\n",
		},
		{
			name:  "replace stub without spaces",
			in:    "#k=\n",
			key:   "k",
			value: "v",
			want:  "k = v\n",
		},
		{
			name:  "replace indented stub with trailing space and CRLF",
			in:    "a = 1\r\n  #  k  =  \r\nb = 2\r\n",
			key:   "k",
			value: "v",
			want:  "a = 1\r\nk = v\nb = 2\r\n",
		},
		{
			name:  "definition wins over earlier stub",
			in:    "# k =\nk = old\n",
			key:   "k",
			value: "new",
			want:  "# k =\nk = new\n",
		},
		{
			name:  "only first stub replaced",
			in:    "# k =\n# k =\n",
			key:   "k",
			value: "v",
			want:  "k = v\n# k =\n",
		},
		{
			name:  "commented line with value is not a stub",
			in:    "# k = old\n",
			key:   "k",
			value: "new",
			want:  "# k = old\nk = new\n",
		},
		{
			name:  "stub of other key is not touched",
			in:    "# kk =\n# k2 =\n",
			key:   "k",
			value: "v",
			want:  "# kk =\n# k2 =\nk = v\n",
		},
		{
			name:  "key prefix does not match",
			in:    "key = 1\n",
			key:   "k",
			value: "v",
			want:  "key = 1\nk = v\n",
		},
		{
			name:  "append to empty data",
			in:    "",
			key:   "k",
			value: "v",
			want:  "k = v\n",
		},
		{
			name:  "append adds missing final LF",
			in:    "a = 1",
			key:   "k",
			value: "v",
			want:  "a = 1\nk = v\n",
		},
		{
			name:  "append after CRLF keeps it",
			in:    "a = 1\r\n",
			key:   "k",
			value: "v",
			want:  "a = 1\r\nk = v\n",
		},
		{
			name:  "replace last line without final end",
			in:    "a = 1\nk = old",
			key:   "k",
			value: "new",
			want:  "a = 1\nk = new\n",
		},
		{
			name:  "same value rewrites to canonical form only",
			in:    "a=1\nk=v\n",
			key:   "k",
			value: "v",
			want:  "a=1\nk = v\n",
		},
		{
			name:  "ST-9 value with hash and equals written literally",
			in:    "",
			key:   "k",
			value: "a#b=c",
			want:  "k = a#b=c\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := []byte(c.in)
			got := string(Set(in, c.key, c.value))
			if got != c.want {
				t.Errorf("Set(%q, %q, %q) =\n%q\nwant\n%q", c.in, c.key, c.value, got, c.want)
			}
			if string(in) != c.in {
				t.Errorf("Set changed its input: %q", in)
			}
			if v, ok := Get(Parse([]byte(got)), c.key); !ok || v != c.value {
				t.Errorf("Get after Set = %q, %v; want %q", v, ok, c.value)
			}
		})
	}
}

// ST-10: a write keeps comments, blank lines and odd spacing of other lines.
func TestSetST10PreservesBytes(t *testing.T) {
	head := "# vars of this machine\r\n\r\n   \t\n#   used by: VIM/.gvimrc\n"
	tail := "\nfoo=bar # literal\r\n  weird   spacing =  x  \nno equals line\n"
	in := head + "k = old\n" + tail
	want := head + "k = new\n" + tail
	if got := string(Set([]byte(in), "k", "new")); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestAppend(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", "machine = b\n"},
		{"repeated key", "machine = a\n", "machine = a\nmachine = b\n"},
		{"no final end", "machine = a", "machine = a\nmachine = b\n"},
		{"CRLF kept", "machine = a\r\n", "machine = a\r\nmachine = b\n"},
		{"after comment", "# list\n", "# list\nmachine = b\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := []byte(c.in)
			if got := string(Append(in, "machine", "b")); got != c.want {
				t.Errorf("Append(%q) = %q, want %q", c.in, got, c.want)
			}
			if string(in) != c.in {
				t.Errorf("Append changed its input: %q", in)
			}
		})
	}
}

// Append must not write into spare capacity of the caller's slice.
func TestAppendNoAlias(t *testing.T) {
	buf := make([]byte, 0, 64)
	buf = append(buf, "a = 1\n"...)
	_ = Append(buf, "b", "2")
	if extra := buf[len(buf):cap(buf)]; len(extra) > 0 && extra[0] != 0 {
		t.Errorf("Append wrote into the caller's backing array: %q", extra[:8])
	}
}
