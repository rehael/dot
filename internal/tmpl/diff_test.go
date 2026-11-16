package tmpl

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func lines(s string) [][]byte { return splitLines([]byte(s)) }

func setLCSLimit(t *testing.T, n int64) {
	old := lcsLimit
	lcsLimit = n
	t.Cleanup(func() { lcsLimit = old })
}

func TestSplitLines(t *testing.T) {
	cases := map[string][]string{
		"":           nil,
		"a":          {"a"},
		"a\n":        {"a\n"},
		"a\nb":       {"a\n", "b"},
		"\n\n":       {"\n", "\n"},
		"a\r\nb\r\n": {"a\r\n", "b\r\n"},
		"a\rb\n":     {"a\rb\n"},
	}
	for in, want := range cases {
		var got []string
		for _, l := range splitLines([]byte(in)) {
			got = append(got, string(l))
		}
		if !slices.Equal(got, want) {
			t.Errorf("splitLines(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiffLines_SYNC12(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want []block
	}{
		{"equal", "a\nb\n", "a\nb\n", nil},
		{"both empty", "", "", nil},
		{"insert into empty", "", "x\n", []block{{0, 0, 0, 1}}},
		{"delete all", "x\ny\n", "", []block{{0, 2, 0, 0}}},
		{"delete middle", "x\ny\nz\n", "x\nz\n", []block{{1, 2, 1, 1}}},
		{"insert middle", "x\nz\n", "x\ny\nz\n", []block{{1, 1, 1, 2}}},
		{"replace one", "x\ny\nz\n", "x\nY\nz\n", []block{{1, 2, 1, 2}}},
		{"two blocks", "a\nb\nc\nd\n", "a\nX\nc\nY\n", []block{{1, 2, 1, 2}, {3, 4, 3, 4}}},
		{"last line gains LF", "a\nb", "a\nb\n", []block{{1, 2, 1, 2}}},
	}
	for _, c := range cases {
		got := diffLines(lines(c.a), lines(c.b))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: diffLines = %v, want %v", c.name, got, c.want)
		}
	}
}

// SYNC-12: above the table limit the remainder after the common first and
// last lines is one changed block.
func TestDiffLines_SYNC12_Fallback(t *testing.T) {
	setLCSLimit(t, 9)
	a, b := lines("a\nb\nc\nd\ne\n"), lines("a\nX\nc\nY\ne\n")
	if got, want := diffLines(a, b), []block{{1, 2, 1, 2}, {3, 4, 3, 4}}; !slices.Equal(got, want) {
		t.Errorf("3x3 under limit: %v, want %v", got, want)
	}
	setLCSLimit(t, 2)
	if got, want := diffLines(a, b), []block{{1, 4, 1, 4}}; !slices.Equal(got, want) {
		t.Errorf("3x3 over limit: %v, want %v", got, want)
	}
}

// SYNC-12: the blocks rebuild b from a, the equal lines pair up, and the
// count of equal lines is the LCS length.
func TestDiffLines_SYNC12_Random(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	gen := func() [][]byte {
		var sb strings.Builder
		for range r.IntN(12) {
			sb.WriteByte("abc"[r.IntN(3)])
			sb.WriteByte('\n')
		}
		return lines(sb.String())
	}
	for range 2000 {
		a, b := gen(), gen()
		var rebuilt []string
		next, nextB, equal := 0, 0, 0
		for _, blk := range diffLines(a, b) {
			if blk.i1-next != blk.j1-nextB || blk.i1 < next || blk.i2 < blk.i1 || blk.j2 < blk.j1 {
				t.Fatalf("bad block %v after (%d,%d) for %q -> %q", blk, next, nextB, a, b)
			}
			for k := next; k < blk.i1; k++ {
				if string(a[k]) != string(b[nextB+k-next]) {
					t.Fatalf("unequal pair in equal run for %q -> %q", a, b)
				}
				rebuilt = append(rebuilt, string(a[k]))
				equal++
			}
			for _, l := range b[blk.j1:blk.j2] {
				rebuilt = append(rebuilt, string(l))
			}
			next, nextB = blk.i2, blk.j2
		}
		if len(a)-next != len(b)-nextB {
			t.Fatalf("tail mismatch for %q -> %q", a, b)
		}
		for k := next; k < len(a); k++ {
			rebuilt = append(rebuilt, string(a[k]))
			equal++
		}
		var want []string
		for _, l := range b {
			want = append(want, string(l))
		}
		if !slices.Equal(rebuilt, want) {
			t.Fatalf("rebuilt %q, want %q", rebuilt, want)
		}
		if l := lcsLen(a, b); equal != l {
			t.Fatalf("equal lines %d, LCS %d for %q -> %q", equal, l, a, b)
		}
	}
}

// lcsLen is a plain recursive reference for small inputs.
func lcsLen(a, b [][]byte) int {
	memo := map[[2]int]int{}
	var f func(i, j int) int
	f = func(i, j int) int {
		if i == len(a) || j == len(b) {
			return 0
		}
		k := [2]int{i, j}
		if v, ok := memo[k]; ok {
			return v
		}
		v := max(f(i+1, j), f(i, j+1))
		if string(a[i]) == string(b[j]) {
			v = max(v, 1+f(i+1, j+1))
		}
		memo[k] = v
		return v
	}
	return f(0, 0)
}
