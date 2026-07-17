package app

import (
	"context"
	"fmt"
	"log"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"iptv/internal/capture"
	"iptv/internal/config"
	"iptv/internal/hbiptv"
	"iptv/internal/logocache"
	"iptv/internal/playlist"
	"iptv/internal/runlock"
	"iptv/internal/source"
)

type Report struct {
	Channels      int       `json:"channels"`
	Timeshift     int       `json:"timeshift"`
	EPGMapped     int       `json:"epg_mapped"`
	NamesReplaced int       `json:"names_replaced"`
	LogosMatched  int       `json:"logos_matched"`
	OutputPath    string    `json:"output_path"`
	SnapshotPath  string    `json:"snapshot_path,omitempty"`
	EPGHost       string    `json:"epg_host"`
	CompletedAt   time.Time `json:"completed_at"`
}

type Runner struct {
	Logger *log.Logger
}

func (r Runner) logger() *log.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return log.Default()
}

func (r Runner) Run(ctx context.Context, settings Settings) (Report, error) {
	logger := r.logger()
	lock, err := runlock.Acquire(filepath.Join(os.TempDir(), "iptv_refresh.lock"))
	if err != nil {
		return Report{}, err
	}
	defer lock.Release()
	creds, _ := config.Load(settings.CredsFile)
	if creds == nil {
		creds = config.Env{}
	}
	if !settings.SkipCapture {
		logger.Printf("[1/3] capturing STB credentials on %s", settings.Interface)
		captured, err := capture.Run(ctx, capture.Options{
			Interface: settings.Interface, Timeout: settings.CaptureTimeout, OutputPath: settings.CredsFile,
			DumpPath: settings.CaptureDump, TokenHost: settings.TokenHost, Fallback: creds,
		})
		if err != nil {
			if len(creds) == 0 {
				return Report{}, fmt.Errorf("capture credentials: %w", err)
			}
			logger.Printf("WARNING: credential capture failed, reusing existing credentials: %v", err)
		} else {
			creds = captured
		}
	}
	if creds["HB_USER_ID"] == "" || creds["HB_STBID"] == "" || creds["HB_STBINFO"] == "" {
		return Report{}, fmt.Errorf("required credentials missing in %s", settings.CredsFile)
	}

	reader := source.Reader{CacheDir: filepath.Join(settings.RepoRoot, "cache", "sources"), UseCache: settings.UseCache}
	if settings.EPGURL != "" {
		logger.Printf("EPG: downloading %s", settings.EPGURL)
		if raw, err := reader.Read(ctx, settings.EPGURL); err != nil {
			logger.Printf("WARNING: EPG download failed: %v", err)
		} else if err := atomicWrite(settings.EPGFile, raw, 0o644); err != nil {
			logger.Printf("WARNING: EPG write failed: %v", err)
		}
	}
	if settings.EPGFile != "" && settings.EPGPublicFile != "" {
		if raw, err := os.ReadFile(settings.EPGFile); err == nil {
			if err := atomicWrite(settings.EPGPublicFile, raw, 0o644); err != nil {
				logger.Printf("WARNING: EPG publish failed: %v", err)
			}
		}
	}
	if settings.EPGCompareSource == "" {
		if _, err := os.Stat(settings.EPGFile); err == nil {
			settings.EPGCompareSource = settings.EPGFile
		}
	}

	settings.TokenServer = resolveValue(settings.TokenServer, creds["HB_TOKEN_SERVER"], "http://121.60.255.37:4338")
	settings.PlatformOrigin = resolveValue(settings.PlatformOrigin, creds["HB_PLATFORM_ORIGIN"], "http://121.60.255.6:8080")
	settings.EPGEntry = resolveValue(settings.EPGEntry, creds["HB_EPG_ENTRY"], "http://121.60.255.4:8080")
	settings.EASIP = resolveValue(settings.EASIP, creds["HB_EASIP"], "121.60.255.4")
	settings.NetworkID = resolveValue(settings.NetworkID, creds["HB_NETWORKID"], "1")
	settings.CityCode = resolveValue(settings.CityCode, creds["HB_CITYCODE"], "")
	if settings.BindSourceIP == "" && settings.BindInterface != "" {
		settings.BindSourceIP = interfaceIPv4(settings.BindInterface)
	}
	if fallback := snapshotEPGHost(settings.SnapshotPath); fallback != "" && fallback != settings.EPGEntry {
		settings.EPGFallbacks = append(settings.EPGFallbacks, fallback)
	}

	logger.Printf("[2/3] authenticating and fetching channels from %s", settings.EPGEntry)
	client, err := hbiptv.New(hbiptv.Config{
		TokenServer: settings.TokenServer, PlatformOrigin: settings.PlatformOrigin, EPGEntry: settings.EPGEntry,
		EPGFallbacks: settings.EPGFallbacks, EASIP: settings.EASIP, NetworkID: settings.NetworkID, CityCode: settings.CityCode,
		UserAgent: settings.UserAgent, BindInterface: settings.BindInterface, BindSourceIP: settings.BindSourceIP, Timeout: settings.HBTimeout,
	})
	if err != nil {
		return Report{}, err
	}
	fetched, err := client.Fetch(ctx, hbiptv.Credentials{
		UserID: creds["HB_USER_ID"], STBID: creds["HB_STBID"], Authenticator: creds["HB_AUTHENTICATOR"],
		STBInfo: creds["HB_STBINFO"], UserToken: creds["HB_USER_TOKEN"],
	})
	if err != nil {
		return Report{}, err
	}
	if err := atomicWrite(settings.SnapshotPath, []byte(fetched.Frameset), 0o600); err != nil {
		return Report{}, err
	}
	channels := playlist.ParseChannels(fetched.Frameset, playlist.URLSelectParams{
		Mode: settings.Mode, IGMPHTTPPrefix: settings.IGMPHTTPPrefix, R2HBaseURL: settings.R2HBaseURL,
		R2HIGMPPath: settings.R2HIGMPPath, R2HToken: settings.R2HToken, R2HAddFCC: settings.R2HAddFCC,
		R2HFCCTYPE: settings.R2HFCCTYPE, R2HProxyRTSP: settings.R2HProxyRTSP,
	}, settings.LineTagRule, settings.LineTagUHD, settings.LineTagHD, settings.LineTagSD)
	if len(channels) == 0 {
		return Report{}, fmt.Errorf("no channel records extracted from frameset")
	}
	// Build name-keyed timeshift metadata before sorting because providers can
	// emit duplicate display names.
	_, catchup, timeshiftLengths := playlist.ChannelsToRows(channels)
	playlist.SortChannels(channels, settings.SortBy)
	rows, _, _ := playlist.ChannelsToRows(channels)
	report := Report{Channels: len(rows), OutputPath: settings.OutputPath, EPGHost: fetched.EPGHost}
	for _, item := range catchup {
		if item.Days > 0 {
			report.Timeshift++
		}
	}
	if settings.EPGCompareSource != "" {
		if raw, err := reader.Read(ctx, settings.EPGCompareSource); err != nil {
			logger.Printf("WARNING: EPG matching disabled: %v", err)
		} else if names, err := playlist.ParseEPG(raw); err != nil {
			logger.Printf("WARNING: EPG parsing disabled: %v", err)
		} else {
			report.EPGMapped, report.NamesReplaced = playlist.AttachEPG(rows, names, settings.EPGReplaceName)
		}
	}
	groups := map[string]string{}
	if settings.GroupBy51ZMT {
		groups = playlist.LoadGroupNames(ctx, reader)
	}
	logos := map[string]string{}
	if settings.LogoMatchSource != "" {
		var logoErr error
		for _, candidate := range playlist.LogoSourceCandidates(settings.LogoMatchSource) {
			var raw []byte
			raw, logoErr = reader.Read(ctx, candidate)
			if logoErr == nil {
				logos, logoErr = playlist.ParseLogoCandidates(raw, settings.LogoURLBase)
			}
			if logoErr == nil {
				break
			}
		}
		if logoErr != nil {
			logger.Printf("WARNING: logo source unavailable: %v", logoErr)
		}
	}
	if settings.LogoOverridesFile != "" {
		if raw, err := os.ReadFile(settings.LogoOverridesFile); err == nil {
			if overrides, err := playlist.ParseLogoOverrides(raw); err == nil {
				maps.Copy(logos, overrides)
			}
		}
	}
	report.LogosMatched = playlist.AttachLogos(rows, logos, settings.LogoMatchThreshold)
	if settings.OrderReference != "" {
		if raw, err := os.ReadFile(settings.OrderReference); err == nil {
			text := string(raw)
			refs := playlist.ParseTXTReference(text)
			if strings.Contains(strings.ToUpper(text), "#EXTINF") || strings.Contains(strings.ToUpper(text), "#EXTM3U") {
				refs = playlist.ParseM3UReference(text)
			}
			rows, _ = playlist.Reorder(rows, refs, settings.KeepUnmatched)
		}
	}
	catchup = playlist.ConvertCatchup(catchup, settings.R2HCatchupHost, settings.CatchupPlayseek, settings.CatchupSeekOffset)
	renderOptions := playlist.RenderOptions{DisplayNameMode: settings.DisplayNameMode, XTvgURL: settings.XTvgURL, GroupNames: groups, Catchup: catchup, TimeShiftLength: timeshiftLengths, CatchupType: settings.CatchupType}
	if err := writePlaylist(settings.OutputPath, settings.OutputFormat, rows, renderOptions); err != nil {
		return Report{}, err
	}
	if settings.SnapshotOutputPath != "" {
		snapshotRows := make([]playlist.Row, 0, len(rows))
		for _, row := range rows {
			if value := playlist.SnapshotURL(row.URL, settings.R2HBaseURL); value != "" {
				row.URL = value
				snapshotRows = append(snapshotRows, row)
			}
		}
		if len(snapshotRows) > 0 {
			snapshotOptions := renderOptions
			snapshotOptions.Catchup = nil
			snapshotOptions.TimeShiftLength = nil
			if err := writePlaylist(settings.SnapshotOutputPath, settings.OutputFormat, snapshotRows, snapshotOptions); err != nil {
				return Report{}, err
			}
			report.SnapshotPath = settings.SnapshotOutputPath
		}
	}
	if settings.LocalLogoCache && settings.LocalLogoURLBase != "" {
		for _, path := range []string{settings.OutputPath, settings.SnapshotOutputPath} {
			if path == "" {
				continue
			}
			if result, err := logocache.Rewrite(ctx, path, settings.LocalLogoDir, settings.LocalLogoURLBase, settings.LocalLogoTimeout, "Mozilla/5.0"); err != nil {
				logger.Printf("WARNING: logo cache %s: %v", path, err)
			} else if result.Failed > 0 {
				logger.Printf("WARNING: logo cache %s downloaded=%d reused=%d failed=%d", path, result.Downloaded, result.Reused, result.Failed)
			}
		}
	}
	report.CompletedAt = time.Now()
	logger.Printf("[3/3] done: channels=%d output=%s", report.Channels, report.OutputPath)
	return report, nil
}

func writePlaylist(path, format string, rows []playlist.Row, options playlist.RenderOptions) error {
	if format == "auto" {
		if strings.EqualFold(filepath.Ext(path), ".m3u") {
			format = "m3u"
		} else {
			format = "txt"
		}
	}
	text := playlist.RenderTXT(rows)
	if format == "m3u" {
		text = playlist.RenderM3U(rows, options)
	}
	return atomicWrite(path, []byte(text), 0o644)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".iptv-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func interfaceIPv4(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addresses, _ := iface.Addrs()
	for _, address := range addresses {
		if ip, _, err := net.ParseCIDR(address.String()); err == nil && ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}

var snapshotHostRE = regexp.MustCompile(`(http://[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:8080)/iptvepg`)

func snapshotEPGHost(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	match := snapshotHostRE.FindSubmatch(raw)
	if match == nil {
		return ""
	}
	return string(match[1])
}

func tokenHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "121.60.255.37"
	}
	host, _, err := net.SplitHostPort(u.Host)
	if err == nil {
		return host
	}
	return u.Hostname()
}
