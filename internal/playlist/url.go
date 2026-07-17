package playlist

import (
	"net/url"
	"strings"
)

func parseChannelSDP(sdp string) (igmp, rtsp string) {
	for item := range strings.SplitSeq(sdp, "|") {
		item = strings.TrimSpace(item)
		if igmp == "" && strings.HasPrefix(item, "igmp://") {
			igmp = item
		}
		if rtsp == "" && strings.HasPrefix(item, "rtsp://") {
			rtsp = item
		}
	}
	return igmp, rtsp
}

type queryParam struct{ key, value string }

func appendQuery(raw string, extra []queryParam) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parts := []string{}
	if u.RawQuery != "" {
		parts = strings.Split(u.RawQuery, "&")
	}
	for _, param := range extra {
		if param.value == "" {
			continue
		}
		replaced := false
		for i, part := range parts {
			rawKey, _, _ := strings.Cut(part, "=")
			key, err := url.QueryUnescape(rawKey)
			if err == nil && key == param.key {
				parts[i] = url.QueryEscape(param.key) + "=" + url.QueryEscape(param.value)
				replaced = true
			}
		}
		if !replaced {
			parts = append(parts, url.QueryEscape(param.key)+"="+url.QueryEscape(param.value))
		}
	}
	u.RawQuery = strings.Join(parts, "&")
	return u.String()
}

func toUDPProxy(raw, prefix string) string {
	if !strings.HasPrefix(raw, "igmp://") || prefix == "" {
		return raw
	}
	return strings.TrimRight(prefix, "/") + "/udp/" + strings.TrimPrefix(raw, "igmp://")
}

func toR2HIGMP(raw string, p URLSelectParams, fccIP, fccPort string) string {
	if !strings.HasPrefix(raw, "igmp://") || p.R2HBaseURL == "" {
		return raw
	}
	suffix := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(raw, "igmp://"), "?", 2)[0])
	if suffix == "" {
		return raw
	}
	pathMode := "udp"
	if p.R2HIGMPPath == "rtp" {
		pathMode = "rtp"
	}
	out := strings.TrimRight(p.R2HBaseURL, "/") + "/" + pathMode + "/" + suffix
	extra := []queryParam{{"r2h-token", p.R2HToken}}
	if p.R2HAddFCC && fccIP != "" && fccPort != "" {
		extra = append(extra, queryParam{"fcc", fccIP + ":" + fccPort}, queryParam{"fcc-type", p.R2HFCCTYPE})
	}
	return appendQuery(out, extra)
}

func toR2HRTSP(raw string, p URLSelectParams) string {
	if !strings.HasPrefix(raw, "rtsp://") || p.R2HBaseURL == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	base, err := url.Parse(strings.TrimRight(p.R2HBaseURL, "/") + "/rtsp/" + u.Host + u.Path)
	if err != nil {
		return raw
	}
	base.Scheme = "http"
	base.RawQuery = u.RawQuery
	return appendQuery(base.String(), []queryParam{{"r2h-token", p.R2HToken}})
}

func pickIGMP(sdp, channelURL, timeshift, fccIP, fccPort string, p URLSelectParams) string {
	for _, raw := range []string{sdp, channelURL, timeshift} {
		if strings.HasPrefix(raw, "igmp://") {
			if p.R2HBaseURL != "" {
				return toR2HIGMP(raw, p, fccIP, fccPort)
			}
			return toUDPProxy(raw, p.IGMPHTTPPrefix)
		}
	}
	return ""
}

func pickRTSP(sdp, channelURL, timeshift string, p URLSelectParams, applyR2H bool) string {
	for _, raw := range []string{sdp, timeshift, channelURL} {
		if strings.HasPrefix(raw, "rtsp://") {
			if applyR2H && p.R2HProxyRTSP && p.R2HBaseURL != "" {
				return toR2HRTSP(raw, p)
			}
			return raw
		}
	}
	return ""
}

func SelectURL(channelURL, timeshift, sdp, fccIP, fccPort string, p URLSelectParams) string {
	sdpIGMP, sdpRTSP := parseChannelSDP(sdp)
	if p.Mode == "rtsp" {
		return pickRTSP(sdpRTSP, channelURL, timeshift, p, false)
	}
	igmp := pickIGMP(sdpIGMP, channelURL, timeshift, fccIP, fccPort, p)
	if p.Mode == "igmp" || igmp != "" {
		return igmp
	}
	return pickRTSP(sdpRTSP, channelURL, timeshift, p, true)
}

func ApplyLineTag(raw, channelName, rule, uhd, hd, sd string) string {
	if raw == "" || rule != "hd_sd" || strings.Contains(raw, "$") {
		return raw
	}
	upper := strings.ToUpper(channelName)
	label := sd
	if strings.Contains(upper, "4K") {
		label = uhd
	} else if strings.Contains(upper, "HD") {
		label = hd
	}
	if label == "" {
		return raw
	}
	return raw + "$" + label
}

func IsR2HURL(raw, baseURL string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	b, err := url.Parse(baseURL)
	if err != nil || !strings.EqualFold(u.Scheme, b.Scheme) || !strings.EqualFold(u.Host, b.Host) {
		return false
	}
	basePath := strings.TrimRight(b.Path, "/")
	return basePath == "" || u.Path == basePath || strings.HasPrefix(u.Path, basePath+"/")
}

func SnapshotURL(raw, baseURL string) string {
	if !IsR2HURL(raw, baseURL) {
		return ""
	}
	label := ""
	queryPos, dollarPos := strings.Index(raw, "?"), strings.LastIndex(raw, "$")
	if dollarPos >= 0 && (queryPos < 0 || dollarPos > queryPos) {
		candidate := raw[dollarPos:]
		if len(candidate) > 1 && !strings.ContainsAny(candidate, "&=#/") {
			label, raw = candidate, raw[:dollarPos]
		}
	}
	return appendQuery(raw, []queryParam{{"snapshot", "1"}}) + label
}
