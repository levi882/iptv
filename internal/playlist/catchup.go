package playlist

import (
	"net/url"
	"strings"
)

const operatorCatchupStart = "{(b)YmdHMS}"
const operatorCatchupEnd = "{(e)YmdHMS}"

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

// BuildOperatorCatchupSource creates a stable LAN URL. The service resolves
// the programme start/end pair to the operator's programme code only when a
// player actually requests catch-up, so short-lived TVOD URLs are never baked
// into the playlist.
func BuildOperatorCatchupSource(baseURL, channelID string) string {
	baseURL = strings.TrimSpace(baseURL)
	channelID = strings.TrimSpace(channelID)
	if baseURL == "" || channelID == "" {
		return ""
	}
	separator := "?"
	if strings.Contains(baseURL, "?") {
		separator = "&"
	}
	return baseURL + separator + "channel=" + url.QueryEscape(channelID) +
		"&start=" + operatorCatchupStart + "&end=" + operatorCatchupEnd
}

// ApplyOperatorCatchup replaces the provider's short rolling-timeshift URL
// with programme-level TVOD for channels represented in the operator guide.
// Channels outside the guide retain their original timeshift configuration.
func ApplyOperatorCatchup(rows []Row, guide OperatorGuide, current map[string]Catchup, baseURL string, days int) map[string]Catchup {
	out := make(map[string]Catchup, len(current)+len(rows))
	for name, item := range current {
		out[name] = item
	}
	if strings.TrimSpace(baseURL) == "" || days <= 0 {
		return out
	}
	channels := make(map[string]bool, len(guide.Channels))
	for _, channel := range guide.Channels {
		channels[channel.ID] = true
	}
	programmes := make(map[string]bool, len(channels))
	for _, programme := range guide.Programmes {
		if programme.ID != "" {
			programmes[programme.ChannelID] = true
		}
	}
	for _, row := range rows {
		if !channels[row.ProviderID] || !programmes[row.ProviderID] {
			continue
		}
		if source := BuildOperatorCatchupSource(baseURL, row.ProviderID); source != "" {
			out[row.Name] = Catchup{Source: source, Days: days}
		}
	}
	return out
}
