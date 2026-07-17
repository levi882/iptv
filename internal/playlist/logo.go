package playlist

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	logoAttrRE = regexp.MustCompile(`(?i)tvg-logo="([^"]+)"`)
	nameAttrRE = regexp.MustCompile(`(?i)tvg-name="([^"]+)"`)
	idAttrRE   = regexp.MustCompile(`(?i)tvg-id="([^"]+)"`)
	imageRE    = regexp.MustCompile(`(?i)\.(png|jpg|jpeg|webp|gif)$`)
)

func LogoSourceCandidates(source string) []string {
	values := []string{source}
	u, err := url.Parse(source)
	if err == nil {
		switch strings.ToLower(u.Host) {
		case "raw.githubusercontent.com":
			if strings.HasPrefix(u.Path, "/fanmingming/live/main/") {
				copy := *u
				copy.Host = "live.fanmingming.com"
				copy.Path = strings.TrimPrefix(u.Path, "/fanmingming/live/main")
				values = append(values, copy.String())
			}
		case "live.fanmingming.com":
			copy := *u
			copy.Host = "raw.githubusercontent.com"
			copy.Path = "/fanmingming/live/main" + u.Path
			values = append(values, copy.String())
		}
	}
	if after, ok := strings.CutPrefix(source, "https://"); ok {
		values = append(values, "http://"+after)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func ParseLogoCandidates(raw []byte, baseURL string) (map[string]string, error) {
	text := strings.TrimSpace(string(raw))
	out := map[string]string{}
	if strings.HasPrefix(text, "[") {
		var items []struct {
			Type        string `json:"type"`
			Name        string `json:"name"`
			DownloadURL string `json:"download_url"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Type != "file" || !imageRE.MatchString(item.Name) {
				continue
			}
			key := NormalizeName(strings.TrimSuffix(item.Name, filepath.Ext(item.Name)))
			value := item.DownloadURL
			if value == "" && baseURL != "" {
				value = strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(item.Name)
			}
			if key != "" && value != "" {
				if _, exists := out[key]; !exists {
					out[key] = value
				}
			}
		}
		return out, nil
	}
	if strings.Contains(strings.ToUpper(text), "#EXTINF") {
		for line := range strings.SplitSeq(text, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "#EXTINF") {
				continue
			}
			logoMatch := logoAttrRE.FindStringSubmatch(line)
			if logoMatch == nil || strings.TrimSpace(logoMatch[1]) == "" {
				continue
			}
			names := []string{}
			if match := nameAttrRE.FindStringSubmatch(line); match != nil {
				names = append(names, match[1])
			}
			if match := idAttrRE.FindStringSubmatch(line); match != nil {
				names = append(names, match[1])
			}
			if _, name, ok := strings.Cut(line, ","); ok {
				names = append(names, name)
			}
			for _, name := range names {
				key := NormalizeName(name)
				if key != "" {
					if _, exists := out[key]; !exists {
						out[key] = strings.TrimSpace(logoMatch[1])
					}
				}
			}
		}
		return out, nil
	}
	return ParseLogoOverrides(raw)
}

func ParseLogoOverrides(raw []byte) (map[string]string, error) {
	out := map[string]string{}
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) < 2 || strings.HasPrefix(strings.TrimSpace(record[0]), "#") {
			continue
		}
		name, value := strings.TrimSpace(record[0]), strings.TrimSpace(record[1])
		if name != "" && value != "" {
			for _, key := range logoAliases(name) {
				out[key] = value
			}
		}
	}
	return out, nil
}

func logoAliases(name string) []string {
	key := NormalizeName(name)
	values := []string{key}
	if strings.HasPrefix(key, "cgtn") {
		values = append(values, "cgtn")
	}
	if key == "卡酷少儿" {
		values = append(values, NormalizeName("卡酷动画"), NormalizeName("北京卡酷"))
	}
	if strings.HasPrefix(key, "cctv4中文国际") {
		values = append(values, NormalizeName("CCTV4"), NormalizeName("CCTV4中文国际"))
	}
	return values
}

// sequenceRatio implements the Ratcliff/Obershelp similarity ratio for these
// short, junk-free channel names.
func sequenceRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	if a == b {
		return 1
	}
	type span struct{ alo, ahi, blo, bhi int }
	stack := []span{{0, len(ra), 0, len(rb)}}
	matches := 0
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		bestI, bestJ, bestSize := current.alo, current.blo, 0
		previous := map[int]int{}
		for i := current.alo; i < current.ahi; i++ {
			next := map[int]int{}
			for j := current.blo; j < current.bhi; j++ {
				if ra[i] != rb[j] {
					continue
				}
				size := previous[j-1] + 1
				next[j] = size
				if size > bestSize {
					bestI, bestJ, bestSize = i-size+1, j-size+1, size
				}
			}
			previous = next
		}
		if bestSize == 0 {
			continue
		}
		matches += bestSize
		if current.alo < bestI && current.blo < bestJ {
			stack = append(stack, span{current.alo, bestI, current.blo, bestJ})
		}
		if bestI+bestSize < current.ahi && bestJ+bestSize < current.bhi {
			stack = append(stack, span{bestI + bestSize, current.ahi, bestJ + bestSize, current.bhi})
		}
	}
	return float64(2*matches) / float64(len(ra)+len(rb))
}

func MatchLogo(name string, candidates map[string]string, threshold float64) string {
	aliases := logoAliases(name)
	for _, key := range aliases {
		if value := candidates[key]; value != "" {
			return value
		}
	}
	if len(aliases) == 0 || (cjkCount(aliases[0]) > 0 && cjkCount(aliases[0]) <= 4) {
		return ""
	}
	bestKey, bestScore := "", threshold
	for key := range candidates {
		if score := sequenceRatio(aliases[0], key); score > bestScore || (score == bestScore && key > bestKey) {
			bestKey, bestScore = key, score
		}
	}
	return candidates[bestKey]
}

func AttachLogos(rows []Row, candidates map[string]string, threshold float64) int {
	matched := 0
	for i := range rows {
		for _, name := range []string{rows[i].Name, rows[i].EPGName} {
			if logo := MatchLogo(name, candidates, threshold); logo != "" {
				rows[i].LogoURL = logo
				matched++
				break
			}
		}
	}
	return matched
}
