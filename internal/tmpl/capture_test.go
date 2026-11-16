package tmpl

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

var capVars = map[string]string{
	"host": "alpha",
	"user": "bob",
	"a":    "foo",
	"b":    "foobar",
	"x":    "same",
	"y":    "same",
	"foo":  "{{foo}}",
	"nl":   "a\nb",
}

type capCase struct {
	name       string
	src, live  string
	want       string
	wantDrops  []Drop
	wantErrSub string // non-empty: an error that contains this text is expected
}

func runCapture(t *testing.T, d Delims, cases []capCase) {
	t.Helper()
	look := mapLook(capVars)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, drops, err := Capture(d, []byte(c.src), []byte(c.live), look)
			if c.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErrSub) {
					t.Fatalf("err = %v, want error with %q", err, c.wantErrSub)
				}
				if got != nil || drops != nil {
					t.Fatalf("on error got %q, %v; want nil, nil", got, drops)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatalf("newSource is nil without error")
			}
			if string(got) != c.want {
				t.Errorf("newSource = %q, want %q", got, c.want)
			}
			if !slices.Equal(drops, c.wantDrops) {
				t.Errorf("dropped = %v, want %v", drops, c.wantDrops)
			}
			// SYNC-11 holds for every successful result.
			if r, u := Render(d, got, look); u != nil || !bytes.Equal(r, []byte(c.live)) {
				t.Errorf("render of result = %q, %q; want live", r, u)
			}
		})
	}
}

func TestCapture(t *testing.T) {
	runCapture(t, def, []capCase{
		// SYNC-9
		{name: "SYNC-9 no placeholder", src: "x=1\n", live: "x=2\ny\n", want: "x=2\ny\n"},
		{name: "SYNC-9 empty live", src: "x", live: "", want: ""},
		{name: "SYNC-9 placeholder-like literal", src: "{{ a }}\n", live: "{{ a }} {{b\n", want: "{{ a }} {{b\n"},
		// SYNC-10
		{name: "SYNC-10 unchanged", src: "h={{host}}\nu={{user}}\n", live: "h=alpha\nu=bob\n", want: "h={{host}}\nu={{user}}\n"},
		{name: "SYNC-10 edit away from placeholders", src: "host={{host}}\nmode=1\n", live: "host=alpha\nmode=2\n", want: "host={{host}}\nmode=2\n"},
		{name: "SYNC-10 edit on placeholder line value kept", src: "url=http://{{host}}:80\n", live: "url=https://alpha:443\n", want: "url=https://{{host}}:443\n"},
		{name: "SYNC-10 two placeholders on edited line", src: "{{user}}@{{host}}\n", live: "ssh bob@alpha -p 22\n", want: "ssh {{user}}@{{host}} -p 22\n"},
		{name: "SYNC-10 value repeated in block", src: "h={{host}}\n", live: "h=alpha,alpha\n", want: "h={{host}},{{host}}\n"},
		{
			name: "SYNC-10 value changed drop", src: "a\nhost={{host}}\n", live: "a\nhost=beta\n",
			want: "a\nhost=beta\n", wantDrops: []Drop{{"host", 2}},
		},
		{
			name: "SYNC-10 drops in two blocks", src: "{{host}}\nmid\n{{user}}\n", live: "beta\nmid\ncarol\n",
			want: "beta\nmid\ncarol\n", wantDrops: []Drop{{"host", 1}, {"user", 3}},
		},
		// An inserted line is a block with no source lines, so it gains no placeholder.
		{name: "SYNC-10 line inserted", src: "host={{host}}\nend\n", live: "host=alpha\nnew alpha\nend\n", want: "host={{host}}\nnew alpha\nend\n"},
		{name: "SYNC-10 line inserted at start", src: "h={{host}}\n", live: "# top\nh=alpha\n", want: "# top\nh={{host}}\n"},
		{
			name: "SYNC-10 line deleted", src: "host={{host}}\nuser={{user}}\nend\n", live: "host=alpha\nend\n",
			want: "host={{host}}\nend\n", wantDrops: []Drop{{"user", 2}},
		},
		{name: "SYNC-10 line without placeholder deleted", src: "h={{host}}\nx\n", live: "h=alpha\n", want: "h={{host}}\n"},
		{
			name: "SYNC-10 last line deleted", src: "x\n{{user}}\n", live: "x\n",
			want: "x\n", wantDrops: []Drop{{"user", 2}},
		},
		{
			name: "SYNC-10 all deleted", src: "{{user}}\n", live: "",
			want: "", wantDrops: []Drop{{"user", 1}},
		},
		{name: "SYNC-10 value is prefix of other value", src: "{{a}} {{b}}\n", live: "foobar foo!\n", want: "{{b}} {{a}}!\n"},
		{name: "SYNC-10 longer value wins", src: "{{a}}={{b}}\n", live: "foobar=foo\n", want: "{{b}}={{a}}\n"},
		{
			name: "SYNC-10 equal values name that sorts first", src: "{{y}} {{x}}\n", live: "same same.\n",
			want: "{{x}} {{x}}.\n", wantDrops: []Drop{{"y", 1}},
		},
		{name: "SYNC-10 CRLF edit away", src: "host={{host}}\r\nmode=1\r\n", live: "host=alpha\r\nmode=2\r\n", want: "host={{host}}\r\nmode=2\r\n"},
		{name: "SYNC-10 CRLF edit on placeholder line", src: "u={{user}}\r\nz\r\n", live: "u=bob!\r\nz\r\n", want: "u={{user}}!\r\nz\r\n"},
		{name: "SYNC-10 CRLF to LF on one line", src: "u={{user}}\r\nz\r\n", live: "u=bob\nz\r\n", want: "u={{user}}\nz\r\n"},
		{name: "SYNC-10 no final line end", src: "a\nhost={{host}}", live: "a\nhost=alpha;", want: "a\nhost={{host}};"},
		{name: "SYNC-10 final line end added", src: "host={{host}}", live: "host=alpha\n", want: "host={{host}}\n"},
		{name: "SYNC-10 final line end removed", src: "host={{host}}\n", live: "host=alpha", want: "host={{host}}"},
		// VAR-8 through capture: the literal survives.
		{name: "VAR-8 literal kept", src: "x {{foo}}\n", live: "y {{foo}}\n", want: "y {{foo}}\n"},
		// VAR-9: UTF-16 content takes the SYNC-9 path.
		{name: "VAR-9 UTF-16", src: string(utf16le("k={{host}}\r\n")), live: string(utf16le("k=other\r\n")), want: string(utf16le("k=other\r\n"))},
		// Errors.
		{name: "undefined name in source", src: "{{nope}}\n", live: "x\n", wantErrSub: "nope"},
		{name: "value with LF changes line count", src: "{{nl}}\n", live: "a\nb\n", wantErrSub: "line"},
		// SYNC-11 guard failures.
		{name: "SYNC-11 placeholder text of defined name in edited line", src: "a={{user}}\nb=1\n", live: "a=bob\nb={{user}}\n", wantErrSub: "render"},
		{name: "SYNC-11 placeholder text on placeholder line", src: "a={{user}}\n", live: "a={{user}}\n", wantErrSub: "render"},
		{name: "SYNC-11 undefined name in result", src: "a={{user}}\nb=1\n", live: "a=bob\nb={{nope}}\n", wantErrSub: "nope"},
		{name: "SYNC-11 guard on SYNC-9 path", src: "plain\n", live: "{{user}}\n", wantErrSub: "render"},
	})
}

func TestCapture_CustomDelims(t *testing.T) {
	runCapture(t, Delims{"<%", "%>"}, []capCase{
		{name: "SYNC-10 custom delims", src: "h=<%host%>\n{{host}}\n", live: "h=alpha2\n{{host}}\n", want: "h=<%host%>2\n{{host}}\n"},
	})
}

func TestCapture_InvalidDelims(t *testing.T) {
	got, drops, err := Capture(Delims{"{{", ""}, []byte("x"), []byte("y"), mapLook(capVars))
	if err == nil || got != nil || drops != nil {
		t.Errorf("Capture with invalid delims = %q, %v, %v", got, drops, err)
	}
}

// SYNC-10 step 4: a name whose value is empty is ignored and then dropped.
func TestCapture_EmptyValueIgnored(t *testing.T) {
	look := mapLook(map[string]string{"e": "", "host": "alpha"})
	got, drops, err := Capture(def, []byte("{{e}}h={{host}}\n"), []byte("h=alpha!\n"), look)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "h={{host}}!\n" || !slices.Equal(drops, []Drop{{"e", 1}}) {
		t.Errorf("Capture = %q, %v", got, drops)
	}
}

// SYNC-10 step 4: inserted placeholder text is never scanned, even when it
// contains the value of another name.
func TestCapture_InsertedTextNotScanned(t *testing.T) {
	look := mapLook(map[string]string{"long": "abc", "s": "ong"})
	// After "abc" becomes {{long}}, the "ong" inside {{long}} is not replaced.
	got, drops, err := Capture(def, []byte("{{long}} {{s}}\n"), []byte("abc hong\n"), look)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{{long}} h{{s}}\n" || drops != nil {
		t.Errorf("Capture = %q, %v", got, drops)
	}
}

// SYNC-12 fallback: one changed block over the whole remainder still gives
// a correct result, and the drop line is the first line of that block.
func TestCapture_SYNC12_Fallback(t *testing.T) {
	setLCSLimit(t, 0)
	look := mapLook(capVars)
	src := "top\n{{host}}\na\nb\n{{user}}\nend\n"
	live := "top\nalpha!\na\nb\ncarol\nend\n"
	got, drops, err := Capture(def, []byte(src), []byte(live), look)
	if err != nil {
		t.Fatal(err)
	}
	if want := "top\n{{host}}!\na\nb\ncarol\nend\n"; string(got) != want {
		t.Errorf("newSource = %q, want %q", got, want)
	}
	if want := []Drop{{"user", 2}}; !slices.Equal(drops, want) {
		t.Errorf("dropped = %v, want %v", drops, want)
	}
}

// SYNC-15: the Drop carries the placeholder name and the 1-based live line
// that the warning needs; a drop does not fail the capture.
func TestCapture_SYNC15_DropData(t *testing.T) {
	look := mapLook(capVars)
	src := "l1\nl2\nuser={{user}}\nhost={{host}}\n"
	live := "l1\nl2\nuser=carol\nhost=alpha\n"
	got, drops, err := Capture(def, []byte(src), []byte(live), look)
	if err != nil {
		t.Fatalf("drop must not be an error: %v", err)
	}
	if string(got) != "l1\nl2\nuser=carol\nhost={{host}}\n" {
		t.Errorf("newSource = %q", got)
	}
	if want := []Drop{{Name: "user", Line: 3}}; !slices.Equal(drops, want) {
		t.Errorf("dropped = %v, want %v", drops, want)
	}
}
