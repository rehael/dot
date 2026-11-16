package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"dot/internal/core"
)

// roundTrip publishes the live edits of A and writes them on B: push on A,
// pull and apply on B. It returns the output of push on A.
func (p *pair) roundTrip() (pushOut, pushErr string) {
	p.t.Helper()
	p.A.ok("push")
	pushOut, pushErr = p.A.stdout(), p.A.stderr()
	p.B.ok("pull")
	wantContains(p.t, "pull on B", p.B.stdout(), "1 file(s) repo-changed; run: dot apply")
	p.B.ok("apply")
	return pushOut, pushErr
}

// E1. PATH-1, PATH-3, CMD-10, CMD-11, CMD-12, CMD-18, CMD-19, CMD-20, ST-5.
func TestE1PlainFileDifferentRoots(t *testing.T) {
	name := cfg("app/settings.ini")
	content := "[main]\nalpha = 1\nbeta = 2\n"
	p := newPair(t, nil, nil, fileSpec{name: name, live: content})

	if p.A.live(name) == p.B.live(name) {
		t.Fatalf("the live paths must differ: %s", p.A.live(name))
	}
	wantContains(t, "apply on B", p.applyB, "wrote "+name)
	wantBytes(t, "live file on A", p.A.readLive(name), content)
	wantBytes(t, "live file on B", p.B.readLive(name), content)
	wantBytes(t, "repository copy on B", p.B.readRepo(name), content)
	wantContains(t, "dot.conf on A", readFile(t, filepath.Join(p.A.Repo, core.RepoConfFile)), "machine = b")
}

// E2. VAR-3, VAR-6, VAR-7, D5.
func TestE2PlaceholdersDifferentValues(t *testing.T) {
	name := cfg("app/profile.txt")
	p := newPair(t,
		map[string]string{"user": "alice", "host": "alpha"},
		map[string]string{"user": "bob", "host": "beta"},
		fileSpec{name: name, live: "name = alice\nhost = alpha\n", repo: "name = {{user}}\nhost = {{host}}\n"})

	wantBytes(t, "live file on A", p.A.readLive(name), "name = alice\nhost = alpha\n")
	wantBytes(t, "live file on B", p.B.readLive(name), "name = bob\nhost = beta\n")
	wantBytes(t, "repository copy on A", p.A.readRepo(name), "name = {{user}}\nhost = {{host}}\n")
	wantBytes(t, "repository copy on B", p.B.readRepo(name), "name = {{user}}\nhost = {{host}}\n")
}

// E3. SYNC-10 (equal runs keep the lines of S), SYNC-13.
func TestE3LiveEditAwayFromPlaceholders(t *testing.T) {
	name := cfg("app/e3.conf")
	p := newPair(t,
		map[string]string{"user": "alice", "host": "alpha"},
		map[string]string{"user": "bob", "host": "beta"},
		fileSpec{
			name: name,
			live: "user = alice\ncolor = red\nhost = alpha\n",
			repo: "user = {{user}}\ncolor = red\nhost = {{host}}\n",
		})

	p.A.writeLive(name, "user = alice\ncolor = blue\nhost = alpha\n")
	out, errOut := p.roundTrip()
	wantContains(t, "push on A", out, "captured "+name)
	wantNotContains(t, "push on A", errOut, "dropped")

	want := "user = {{user}}\ncolor = blue\nhost = {{host}}\n"
	wantBytes(t, "repository copy on A", p.A.readRepo(name), want)
	wantBytes(t, "repository copy on B", p.B.readRepo(name), want)
	wantContains(t, "apply on B", p.B.stdout(), "wrote "+name)
	wantBytes(t, "live file on B", p.B.readLive(name), "user = bob\ncolor = blue\nhost = beta\n")
	wantBytes(t, "live file on A", p.A.readLive(name), "user = alice\ncolor = blue\nhost = alpha\n")
}

// E4. SYNC-10 step 4: the value in a changed line becomes the placeholder again.
func TestE4LiveEditOnPlaceholderLine(t *testing.T) {
	name := cfg("app/e4.conf")
	p := newPair(t,
		map[string]string{"user": "alice"},
		map[string]string{"user": "bob"},
		fileSpec{name: name, live: "# paths\npath = alice/bin\n", repo: "# paths\npath = {{user}}/bin\n"})

	p.A.writeLive(name, "# paths\npath = /opt/alice/bin\n")
	out, errOut := p.roundTrip()
	wantContains(t, "push on A", out, "captured "+name)
	wantNotContains(t, "push on A", errOut, "dropped")

	wantBytes(t, "repository copy on A", p.A.readRepo(name), "# paths\npath = /opt/{{user}}/bin\n")
	wantBytes(t, "live file on B", p.B.readLive(name), "# paths\npath = /opt/bob/bin\n")
}

// E5. SYNC-10 step 5, SYNC-15, CMD-17 (warnings of diff), A9.
func TestE5ValueChangedDropsPlaceholder(t *testing.T) {
	name := cfg("app/e5.conf")
	p := newPair(t,
		map[string]string{"user": "alice"},
		map[string]string{"user": "bob"},
		fileSpec{name: name, live: "# owner\nowner = alice\n", repo: "# owner\nowner = {{user}}\n"})

	p.A.writeLive(name, "# owner\nowner = carol\n")
	warning := "dot: warning: " + name + ": line 2: placeholder {{user}} dropped"

	p.A.ok("diff")
	wantContains(t, "diff on A", p.A.stdout(), "live-changed "+name)
	wantContains(t, "diff on A", p.A.stderr(), warning)

	_, errOut := p.roundTrip()
	wantContains(t, "push on A", errOut, warning)
	wantBytes(t, "repository copy on A", p.A.readRepo(name), "# owner\nowner = carol\n")
	wantBytes(t, "live file on B", p.B.readLive(name), "# owner\nowner = carol\n")
}

// utf16le encodes ASCII text as UTF-16 little-endian with a byte order mark.
func utf16le(s string) string {
	var b strings.Builder
	b.WriteString("\xff\xfe")
	for i := 0; i < len(s); i++ {
		b.WriteByte(s[i])
		b.WriteByte(0)
	}
	return b.String()
}

// E12. D8, VAR-9, ST-5 (* -text), SYNC-9.
func TestE12UTF16RoundTrip(t *testing.T) {
	name := cfg("app/e12.ini")
	// The placeholder-like text has NUL bytes between its characters, so it
	// is no placeholder (VAR-9).
	content := utf16le("[main]\r\nuser={{user}}\r\n")
	p := newPair(t,
		map[string]string{"user": "alice"},
		map[string]string{"user": "bob"},
		fileSpec{name: name, live: content})

	if !strings.Contains(content, "\x00") {
		t.Fatal("the sample must contain NUL bytes")
	}
	wantBytes(t, "live file on B", p.B.readLive(name), content)
	wantBytes(t, "repository copy on B", p.B.readRepo(name), content)

	edited := utf16le("[main]\r\nuser={{user}}\r\ncolor=blue\r\n")
	p.A.writeLive(name, edited)
	out, _ := p.roundTrip()
	wantContains(t, "push on A", out, "captured "+name)
	wantBytes(t, "repository copy on A", p.A.readRepo(name), edited)
	wantBytes(t, "live file on A", p.A.readLive(name), edited)
	wantBytes(t, "live file on B", p.B.readLive(name), edited)
}

// E13. D8, A16, SYNC-10: CRLF line ends stay unchanged in every copy.
func TestE13CRLFRoundTrip(t *testing.T) {
	name := cfg("app/e13.conf")
	p := newPair(t,
		map[string]string{"user": "alice", "host": "alpha"},
		map[string]string{"user": "bob", "host": "beta"},
		fileSpec{
			name: name,
			live: "user = alice\r\ncolor = red\r\nhost = alpha\r\n",
			repo: "user = {{user}}\r\ncolor = red\r\nhost = {{host}}\r\n",
		})
	wantBytes(t, "live file on B after the first sync", p.B.readLive(name), "user = bob\r\ncolor = red\r\nhost = beta\r\n")

	p.A.writeLive(name, "user = alice (main)\r\ncolor = blue\r\nhost = alpha\r\n")
	out, _ := p.roundTrip()
	wantContains(t, "push on A", out, "captured "+name)

	wantRepo := "user = {{user}} (main)\r\ncolor = blue\r\nhost = {{host}}\r\n"
	wantBytes(t, "repository copy on A", p.A.readRepo(name), wantRepo)
	wantBytes(t, "repository copy on B", p.B.readRepo(name), wantRepo)
	wantBytes(t, "live file on A", p.A.readLive(name), "user = alice (main)\r\ncolor = blue\r\nhost = alpha\r\n")
	wantBytes(t, "live file on B", p.B.readLive(name), "user = bob (main)\r\ncolor = blue\r\nhost = beta\r\n")
	for what, data := range map[string]string{
		"repository copy on A": p.A.readRepo(name),
		"repository copy on B": p.B.readRepo(name),
		"live file on A":       p.A.readLive(name),
		"live file on B":       p.B.readLive(name),
	} {
		if n, crlf := strings.Count(data, "\n"), strings.Count(data, "\r\n"); n != crlf {
			t.Errorf("%s: %d line ends, %d of them CRLF", what, n, crlf)
		}
	}
}
