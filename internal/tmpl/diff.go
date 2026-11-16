package tmpl

// block is a changed block: lines a[i1:i2] are replaced by lines b[j1:j2].
type block struct{ i1, i2, j1, j2 int }

// lcsLimit is the largest LCS table, in cells, that diffLines builds
// (SYNC-12). A variable so that tests can reach the fallback with small input.
var lcsLimit int64 = 25_000_000

// diffLines returns the changed blocks between a and b, in order (SYNC-12).
// All lines outside the blocks are equal and pair up in order.
func diffLines(a, b [][]byte) []block {
	x, y := intern(a, b)
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	x, y = x[pre:len(x)-suf], y[pre:len(y)-suf]
	n, m := len(x), len(y)
	if n == 0 && m == 0 {
		return nil
	}
	if n == 0 || m == 0 || int64(n)*int64(m) > lcsLimit {
		return []block{{pre, pre + n, pre, pre + m}}
	}

	// dp[i*w+j] is the LCS length of x[i:] and y[j:].
	w := m + 1
	dp := make([]int32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				dp[i*w+j] = dp[(i+1)*w+j+1] + 1
			} else {
				dp[i*w+j] = max(dp[(i+1)*w+j], dp[i*w+j+1])
			}
		}
	}

	var blocks []block
	var cur block
	open := false
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && x[i] == y[j] {
			if open {
				cur.i2, cur.j2 = pre+i, pre+j
				blocks = append(blocks, cur)
				open = false
			}
			i++
			j++
			continue
		}
		if !open {
			cur = block{i1: pre + i, j1: pre + j}
			open = true
		}
		if j == m || i < n && dp[(i+1)*w+j] >= dp[i*w+j+1] {
			i++
		} else {
			j++
		}
	}
	if open {
		cur.i2, cur.j2 = pre+n, pre+m
		blocks = append(blocks, cur)
	}
	return blocks
}

// intern maps equal lines to equal integers, so that the table loop compares
// integers instead of byte slices.
func intern(a, b [][]byte) (x, y []int) {
	ids := map[string]int{}
	conv := func(lines [][]byte) []int {
		out := make([]int, len(lines))
		for k, l := range lines {
			id, ok := ids[string(l)]
			if !ok {
				id = len(ids)
				ids[string(l)] = id
			}
			out[k] = id
		}
		return out
	}
	return conv(a), conv(b)
}
