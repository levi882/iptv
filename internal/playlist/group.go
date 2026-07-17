package playlist

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"iptv/internal/source"
)

var (
	channelSlugRE = regexp.MustCompile(`(?i)/channel/([^/"'?#]+)/`)
	altNameRE     = regexp.MustCompile(`(?i)alt="([^"]+)"`)
)

var DefaultGroupSources = map[string]string{
	"央视": "https://epg.51zmt.top:8001/category/cctv/",
	"卫视": "https://epg.51zmt.top:8001/category/satellite/",
	"数字": "https://epg.51zmt.top:8001/category/digital/",
	"地方": "https://epg.51zmt.top:8001/category/local/",
}

func LoadGroupNames(ctx context.Context, reader source.Reader) map[string]string {
	reader.UseCache = true
	type result struct {
		group string
		raw   []byte
	}
	results := make(chan result, len(DefaultGroupSources))
	var wg sync.WaitGroup
	for group, endpoint := range DefaultGroupSources {
		wg.Go(func() {
			raw, err := reader.Read(ctx, endpoint)
			if err == nil {
				results <- result{group: group, raw: raw}
			}
		})
	}
	wg.Wait()
	close(results)
	out := map[string]string{}
	for item := range results {
		text := string(item.raw)
		names := []string{}
		for _, match := range channelSlugRE.FindAllStringSubmatch(text, -1) {
			if decoded, err := url.PathUnescape(match[1]); err == nil {
				names = append(names, decoded)
			}
		}
		for _, match := range altNameRE.FindAllStringSubmatch(text, -1) {
			names = append(names, match[1])
		}
		for _, name := range names {
			key := NormalizeName(strings.TrimSpace(name))
			if key != "" {
				if _, exists := out[key]; !exists {
					out[key] = item.group
				}
			}
		}
	}
	return out
}
