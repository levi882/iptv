package playlist

import (
	"net/url"
	"strings"
)

func BuildCatchupSource(rtspURL, host, playseekTemplate, seekOffset, r2hToken string) string {
	u, err := url.Parse(rtspURL)
	if err != nil {
		return rtspURL
	}
	parts := []string{}
	for item := range strings.SplitSeq(u.RawQuery, "&") {
		if item == "" {
			continue
		}
		rawKey, _, _ := strings.Cut(item, "=")
		key, err := url.QueryUnescape(rawKey)
		if err != nil {
			key = rawKey
		}
		if strings.EqualFold(key, "playseek") || strings.EqualFold(key, "tvdr") || (r2hToken != "" && strings.EqualFold(key, "r2h-token")) {
			continue
		}
		parts = append(parts, item)
	}
	parts = append(parts, "playseek="+playseekTemplate)
	if seekOffset != "" {
		parts = append(parts, "r2h-seek-offset="+seekOffset)
	}
	if r2hToken != "" {
		parts = append(parts, "r2h-token="+url.QueryEscape(r2hToken))
	}
	return "http://" + host + "/rtsp/" + u.Host + u.Path + "?" + strings.Join(parts, "&")
}

func ConvertCatchup(input map[string]Catchup, host, playseekTemplate, seekOffset, r2hToken string) map[string]Catchup {
	out := make(map[string]Catchup, len(input))
	for name, item := range input {
		if host != "" && strings.HasPrefix(item.Source, "rtsp://") {
			item.Source = BuildCatchupSource(item.Source, host, playseekTemplate, seekOffset, r2hToken)
		}
		out[name] = item
	}
	return out
}
