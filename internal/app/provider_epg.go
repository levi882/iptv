package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"iptv/internal/atomicfile"
	"iptv/internal/playlist"
	"iptv/internal/portal"
)

func loadOperatorEPG(path string, location *time.Location) (playlist.OperatorGuide, bool, error) {
	if path == "" {
		return playlist.OperatorGuide{}, false, nil
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return playlist.OperatorGuide{}, false, nil
	}
	if err != nil {
		return playlist.OperatorGuide{}, false, err
	}
	return playlist.ParseOperatorEPG(raw, location)
}

func writeOperatorEPG(path string, raw []byte) error {
	if path == "" {
		return nil
	}
	encoded, err := epgBytesForPath(raw, path)
	if err != nil {
		return err
	}
	if _, err := atomicfile.WriteIfChanged(path, encoded, 0o644); err != nil {
		return err
	}
	return nil
}

func operatorEPGCachePath(settings Settings) string {
	if settings.EPGFile != "" {
		return settings.EPGFile
	}
	return settings.EPGPublicFile
}

func refreshOperatorEPG(ctx context.Context, client *portal.Client, host, userID string, settings Settings, now time.Time) (playlist.OperatorGuide, int, error) {
	cachePath := operatorEPGCachePath(settings)
	existing, recognized, err := loadOperatorEPG(cachePath, now.Location())
	if err != nil {
		// A legacy or damaged cache must not prevent a clean operator bootstrap.
		existing = playlist.OperatorGuide{}
		recognized = false
	}
	days := 1
	if !recognized {
		days = settings.GuideHistoryDays + 1
	}
	fresh, err := client.FetchGuide(ctx, host, userID, portal.GuideOptions{
		Template: settings.GuideTemplate,
		Now:      now,
		Days:     days,
	})
	if err != nil {
		return playlist.OperatorGuide{}, days, err
	}
	merged := playlist.MergeOperatorEPG(existing, fresh, now, settings.GuideHistoryDays)
	raw, err := playlist.RenderOperatorEPG(merged)
	if err != nil {
		return playlist.OperatorGuide{}, days, err
	}
	if err := writeOperatorEPG(settings.EPGFile, raw); err != nil {
		return playlist.OperatorGuide{}, days, fmt.Errorf("write operator EPG cache: %w", err)
	}
	if err := writeOperatorEPG(settings.EPGPublicFile, raw); err != nil {
		return playlist.OperatorGuide{}, days, fmt.Errorf("publish operator EPG: %w", err)
	}
	return merged, days, nil
}
