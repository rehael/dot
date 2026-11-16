package tmpl

// Render replaces each placeholder with its value in one pass; a value is
// never scanned again (VAR-7). If a name is undefined, out is nil and
// undefined lists the undefined names, unique, in order of first occurrence.
func Render(d Delims, content []byte, look Lookup) (out []byte, undefined []string) {
	out = make([]byte, 0, len(content))
	seen := map[string]bool{}
	last := 0
	for _, t := range scan(d, content) {
		out = append(out, content[last:t.start]...)
		last = t.end
		v, ok := look(t.name)
		if !ok {
			if !seen[t.name] {
				seen[t.name] = true
				undefined = append(undefined, t.name)
			}
			continue
		}
		out = append(out, v...)
	}
	if len(undefined) > 0 {
		return nil, undefined
	}
	return append(out, content[last:]...), nil
}
