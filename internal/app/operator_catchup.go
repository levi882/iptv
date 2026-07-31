package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"iptv/internal/config"
	"iptv/internal/playlist"
	"iptv/internal/portal"
)

var ErrOperatorProgrammeNotFound = errors.New("operator catch-up programme not found")

type OperatorCatchupResolver struct {
	mu       sync.Mutex
	settings Settings
	logger   *log.Logger

	client  *portal.Client
	host    string
	runtime Settings

	guidePath    string
	guideModTime time.Time
	guideSize    int64
	guide        playlist.OperatorGuide
}

func NewOperatorCatchupResolver(settings Settings, logger *log.Logger) *OperatorCatchupResolver {
	return &OperatorCatchupResolver{settings: settings, logger: logger}
}

func (r *OperatorCatchupResolver) loadGuide(location *time.Location) (playlist.OperatorGuide, error) {
	path := operatorEPGCachePath(r.settings)
	if path == "" {
		return playlist.OperatorGuide{}, fmt.Errorf("operator EPG cache is not configured")
	}
	info, err := os.Stat(path)
	if err != nil {
		return playlist.OperatorGuide{}, fmt.Errorf("stat operator EPG cache: %w", err)
	}
	if path == r.guidePath && info.Size() == r.guideSize && info.ModTime().Equal(r.guideModTime) {
		return r.guide, nil
	}
	guide, recognized, err := loadOperatorEPG(path, location)
	if err != nil {
		return playlist.OperatorGuide{}, err
	}
	if !recognized {
		return playlist.OperatorGuide{}, fmt.Errorf("operator EPG cache is unavailable")
	}
	r.guidePath = path
	r.guideModTime = info.ModTime()
	r.guideSize = info.Size()
	r.guide = guide
	return guide, nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func wallClockDelta(a, b time.Time) time.Duration {
	a = time.Date(a.Year(), a.Month(), a.Day(), a.Hour(), a.Minute(), a.Second(), 0, time.UTC)
	b = time.Date(b.Year(), b.Month(), b.Day(), b.Hour(), b.Minute(), b.Second(), 0, time.UTC)
	return absDuration(a.Sub(b))
}

func findCatchupProgramme(guide playlist.OperatorGuide, channelID string, start, end, now time.Time) (playlist.OperatorProgramme, error) {
	const startTolerance = 2 * time.Minute
	const endTolerance = 5 * time.Minute
	bestScore := time.Duration(1<<63 - 1)
	var best playlist.OperatorProgramme
	for _, programme := range guide.Programmes {
		if programme.ChannelID != channelID || programme.ID == "" || programme.Stop.After(now.Add(time.Minute)) {
			continue
		}
		// M3U catch-up placeholders carry the guide's displayed wall-clock value
		// without an offset. Compare calendar fields so a router configured as
		// UTC can still resolve an XMLTV programme explicitly marked +0800.
		startDelta := wallClockDelta(programme.Start, start)
		if startDelta > startTolerance {
			continue
		}
		endDelta := time.Duration(0)
		if !end.IsZero() {
			endDelta = wallClockDelta(programme.Stop, end)
			if endDelta > endTolerance {
				continue
			}
		}
		if score := startDelta + endDelta; score < bestScore {
			bestScore = score
			best = programme
		}
	}
	if best.ID == "" {
		return playlist.OperatorProgramme{}, ErrOperatorProgrammeNotFound
	}
	return best, nil
}

func (r *OperatorCatchupResolver) openSession(ctx context.Context) error {
	creds, err := config.Load(r.settings.CredsFile)
	if err != nil {
		return fmt.Errorf("load provider credentials: %w", err)
	}
	creds = creds.NormalizeProviderKeys()
	if creds["PROVIDER_USER_ID"] == "" || creds["PROVIDER_STBID"] == "" || creds["PROVIDER_STBINFO"] == "" {
		return fmt.Errorf("required provider credentials are missing")
	}
	runtime := resolveLocalURLs(r.settings, discoverLocalServices(ctx))
	runtime, err = resolveProviderMetadata(runtime, creds)
	if err != nil {
		return err
	}
	if runtime.R2HBaseURL == "" {
		return fmt.Errorf("rtp2httpd base URL is unavailable")
	}
	if fallback := snapshotEPGHost(runtime.SnapshotPath); fallback != "" && fallback != runtime.EPGEntry {
		runtime.EPGFallbacks = append(runtime.EPGFallbacks, fallback)
	}
	client, err := newProviderClient(runtime)
	if err != nil {
		return err
	}
	fetched, err := client.Fetch(ctx, providerCredentials(runtime, creds))
	if err != nil {
		return err
	}
	if err := client.AuthorizeCatchup(ctx, fetched.EPGHost, creds["PROVIDER_USER_ID"]); err != nil {
		return err
	}
	r.client = client
	r.host = fetched.EPGHost
	r.runtime = runtime
	return nil
}

func (r *OperatorCatchupResolver) resolveURL(ctx context.Context, programme playlist.OperatorProgramme) (string, error) {
	if r.client == nil {
		if err := r.openSession(ctx); err != nil {
			return "", err
		}
	}
	playURL, err := r.client.FetchCatchupURL(ctx, r.host, r.runtime.GuideTemplate, programme.ID, programme.ChannelID)
	if err != nil {
		return "", err
	}
	proxied := playlist.ProxyRTSPURL(playURL, r.runtime.R2HBaseURL, r.runtime.R2HToken)
	parsed, parseErr := url.Parse(proxied)
	if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !playlist.IsR2HURL(proxied, r.runtime.R2HBaseURL) {
		return "", fmt.Errorf("unable to proxy operator TVOD URL through rtp2httpd")
	}
	return proxied, nil
}

// ResolveCatchup converts an exact XMLTV programme interval into a fresh TVOD
// URL. One retry rebuilds both portal and secondary-auth sessions, covering the
// common case where a saved provider cookie expires between playback requests.
func (r *OperatorCatchupResolver) ResolveCatchup(ctx context.Context, channelID string, start, end time.Time) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	channelID = strings.TrimSpace(channelID)
	if channelID == "" || start.IsZero() {
		return "", ErrOperatorProgrammeNotFound
	}
	guide, err := r.loadGuide(start.Location())
	if err != nil {
		return "", err
	}
	programme, err := findCatchupProgramme(guide, channelID, start, end, time.Now())
	if err != nil {
		return "", err
	}
	playURL, err := r.resolveURL(ctx, programme)
	if err == nil || ctx.Err() != nil {
		return playURL, err
	}
	if r.logger != nil {
		r.logger.Printf("operator catch-up session expired; re-authenticating")
	}
	r.client = nil
	r.host = ""
	return r.resolveURL(ctx, programme)
}
