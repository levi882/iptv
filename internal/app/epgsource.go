package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"iptv/internal/playlist"
)

const epgFreshnessGrace = 15 * time.Minute

type selectedEPG struct {
	Raw      []byte
	URL      string
	Coverage playlist.EPGCoverage
	Fresh    bool
	Notes    []string
}

func selectEPGSource(ctx context.Context, urls []string, now time.Time, location *time.Location, read func(context.Context, string) ([]byte, error)) (selectedEPG, error) {
	seen := make(map[string]struct{}, len(urls))
	var stale *selectedEPG
	errors := make([]string, 0, len(urls))
	notes := make([]string, 0, len(urls))
	for _, value := range urls {
		url := strings.TrimSpace(value)
		if url == "" {
			continue
		}
		if _, exists := seen[url]; exists {
			continue
		}
		seen[url] = struct{}{}

		raw, err := read(ctx, url)
		if err != nil {
			note := fmt.Sprintf("%s: download failed: %v", url, err)
			errors = append(errors, note)
			notes = append(notes, note)
			continue
		}
		coverage, err := playlist.InspectEPGCoverage(raw, location)
		if err != nil {
			note := fmt.Sprintf("%s: unusable guide: %v", url, err)
			errors = append(errors, note)
			notes = append(notes, note)
			continue
		}
		candidate := selectedEPG{Raw: raw, URL: url, Coverage: coverage}
		candidate.Fresh = !coverage.Latest.Before(now.Add(-epgFreshnessGrace))
		if candidate.Fresh {
			candidate.Notes = notes
			return candidate, nil
		}
		notes = append(notes, fmt.Sprintf("%s: expired at %s", url, coverage.Latest.Format(time.RFC3339)))
		if stale == nil || coverage.Latest.After(stale.Coverage.Latest) {
			copy := candidate
			stale = &copy
		}
	}
	if stale != nil {
		stale.Notes = notes
		return *stale, nil
	}
	if len(errors) == 0 {
		return selectedEPG{}, fmt.Errorf("no EPG source configured")
	}
	return selectedEPG{}, fmt.Errorf("no usable EPG source: %s", strings.Join(errors, "; "))
}
