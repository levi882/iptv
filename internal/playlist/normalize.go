package playlist

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	cetvRE       = regexp.MustCompile(`(?i)cetv\s*([0-9]+)`)
	eduRE        = regexp.MustCompile(`中国教育\s*([0-9]+)\s*台?`)
	noiseCharRE  = regexp.MustCompile(`[ \t\-_/|·•_.]`)
	noiseWordsRE = regexp.MustCompile(`(?i)(频道|高清|标清|超清|超高清|hd|4k|央视|中央|电视台|台)`)
	trailingHDRE = regexp.MustCompile(`(?i)(?:\s*[-_ ]?\s*HD)$`)
)

func NormalizeName(name string) string {
	raw := strings.TrimSpace(name)
	if match := cetvRE.FindStringSubmatch(raw); match != nil {
		return "cetv" + match[1]
	}
	if match := eduRE.FindStringSubmatch(raw); match != nil {
		return "cetv" + match[1]
	}
	value := strings.ToLower(raw)
	value = strings.ReplaceAll(value, "cctv-", "cctv")
	value = strings.ReplaceAll(value, "＋", "+")
	value = noiseCharRE.ReplaceAllString(value, "")
	return noiseWordsRE.ReplaceAllString(value, "")
}

func cleanTVGName(name string) string {
	return strings.TrimSpace(trailingHDRE.ReplaceAllString(strings.TrimSpace(name), ""))
}

func cjkCount(value string) int {
	n := 0
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n
}

func InferGroupTitle(name, tvgName string) string {
	value := strings.ToUpper(tvgName)
	if value == "" {
		value = strings.ToUpper(name)
	}
	if strings.Contains(value, "CCTV") || strings.Contains(value, "央视") {
		return "央视"
	}
	if strings.Contains(value, "卫视") {
		return "卫视"
	}
	return "地方"
}
