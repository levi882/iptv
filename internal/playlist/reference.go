package playlist

import "strings"

func ParseM3UReference(text string) []Reference {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	refs := []Reference{}
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#EXTINF") {
			i++
			continue
		}
		ref := Reference{EXTINF: line}
		if _, name, ok := strings.Cut(line, ","); ok {
			ref.Name = strings.TrimSpace(name)
		}
		i++
		for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "#EXTVLCOPT:") {
			ref.Options = append(ref.Options, strings.TrimSpace(lines[i]))
			i++
		}
		for i < len(lines) {
			current := strings.TrimSpace(lines[i])
			if current == "" {
				i++
				continue
			}
			if strings.HasPrefix(current, "#") {
				break
			}
			refs = append(refs, ref)
			i++
			break
		}
	}
	return refs
}

func ParseTXTReference(text string) []Reference {
	refs := []Reference{}
	for line := range strings.SplitSeq(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.Contains(line, "#genre#") {
			continue
		}
		name, _, _ := strings.Cut(line, ",")
		if name = strings.TrimSpace(name); name != "" {
			refs = append(refs, Reference{Name: name})
		}
	}
	return refs
}

func Reorder(rows []Row, refs []Reference, keepUnmatched bool) ([]Row, int) {
	byKey := map[string][]int{}
	for i := range rows {
		key := NormalizeName(rows[i].Name)
		byKey[key] = append(byKey[key], i)
	}
	used := make([]bool, len(rows))
	ordered := make([]Row, 0, len(rows))
	matched := 0
	for i := range refs {
		key := NormalizeName(refs[i].Name)
		indexes := byKey[key]
		for len(indexes) > 0 && used[indexes[0]] {
			indexes = indexes[1:]
		}
		byKey[key] = indexes
		if len(indexes) == 0 {
			continue
		}
		idx := indexes[0]
		byKey[key] = indexes[1:]
		used[idx] = true
		row := rows[idx]
		refCopy := refs[i]
		row.Ref = &refCopy
		ordered = append(ordered, row)
		matched++
	}
	if keepUnmatched {
		for i := range rows {
			if !used[i] {
				ordered = append(ordered, rows[i])
			}
		}
	}
	return ordered, matched
}
