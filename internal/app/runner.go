package app

import (
	"context"
	"fmt"
	"log"
	"maps"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"iptv/internal/atomicfile"
	"iptv/internal/capture"
	"iptv/internal/config"
	"iptv/internal/logocache"
	"iptv/internal/playlist"
	"iptv/internal/portal"
	"iptv/internal/runlock"
	"iptv/internal/source"
	"iptv/internal/stbpower"
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
	Logger           *log.Logger
	Capture          func(context.Context, capture.Options) (config.Env, error)
	RestartRTP2HTTPD func(context.Context) error
}

func (r Runner) logger() *log.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return log.Default()
}

func (r Runner) capture(ctx context.Context, options capture.Options) (config.Env, error) {
	if r.Capture != nil {
		return r.Capture(ctx, options)
	}
	return capture.Run(ctx, options)
}

func (r Runner) restartRTP2HTTPD(ctx context.Context) error {
	if r.RestartRTP2HTTPD != nil {
		return r.RestartRTP2HTTPD(ctx)
	}
	output, err := exec.CommandContext(ctx, "/etc/init.d/rtp2httpd", "restart").CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if detail != "" {
		return fmt.Errorf("restart rtp2httpd: %w: %s", err, detail)
	}
	return fmt.Errorf("restart rtp2httpd: %w", err)
}

func (r Runner) Run(ctx context.Context, settings Settings) (Report, error) {
	if settings.RefreshTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, settings.RefreshTimeout)
		defer cancel()
	}
	logger := r.logger()
	lock, err := runlock.Acquire(filepath.Join(os.TempDir(), "iptv_refresh.lock"))
	if err != nil {
		return Report{}, err
	}
	defer lock.Release()
	if settings.BindInterface != "" {
		if _, err := net.InterfaceByName(settings.BindInterface); err != nil {
			return Report{}, fmt.Errorf("provider HTTP interface %q is unavailable: %w", settings.BindInterface, err)
		}
		if settings.BindSourceIP == "" {
			var err error
			settings.BindSourceIP, err = interfaceIPv4(settings.BindInterface)
			if err != nil {
				return Report{}, err
			}
		}
	}
	creds, _ := config.Load(settings.CredsFile)
	if creds == nil {
		creds = config.Env{}
	}
	creds = creds.NormalizeProviderKeys()
	capturedFreshCredentials := false
	if !settings.SkipCapture {
		logger.Printf("[1/3] capturing STB credentials on %s", settings.Interface)
		var onCaptureReady func(context.Context) error
		if settings.STBPowerWebhookURL != "" {
			onCaptureReady = func(ctx context.Context) error {
				logger.Printf("capture listener ready; requesting STB power-on through Home Assistant")
				err := (stbpower.Webhook{URL: settings.STBPowerWebhookURL, Timeout: settings.STBPowerWebhookTimeout}).Trigger(ctx)
				if err == nil {
					logger.Printf("Home Assistant accepted the STB power-on webhook")
				}
				return err
			}
		}
		captured, err := r.capture(ctx, capture.Options{
			Interface: settings.Interface, Timeout: settings.CaptureTimeout, OutputPath: settings.CredsFile,
			DumpPath: settings.CaptureDump, TokenHost: settings.TokenHost, Fallback: creds, OnReady: onCaptureReady,
		})
		if err != nil {
			if len(creds) == 0 {
				return Report{}, fmt.Errorf("capture credentials: %w", err)
			}
			logger.Printf("WARNING: credential capture failed, reusing existing credentials: %v", err)
		} else {
			creds = captured
			capturedFreshCredentials = true
		}
	}
	if creds["PROVIDER_USER_ID"] == "" || creds["PROVIDER_STBID"] == "" || creds["PROVIDER_STBINFO"] == "" {
		return Report{}, fmt.Errorf("required credentials missing in %s", settings.CredsFile)
	}

	reader := source.Reader{CacheDir: filepath.Join(settings.RepoRoot, "cache", "sources"), UseCache: settings.UseCache}
	if settings.EPGURL != "" {
		logger.Printf("EPG: downloading %s", settings.EPGURL)
		if raw, err := reader.Read(ctx, settings.EPGURL); err != nil {
			logger.Printf("WARNING: EPG download failed: %v", err)
		} else if _, err := atomicfile.WriteIfChanged(settings.EPGFile, raw, 0o644); err != nil {
			logger.Printf("WARNING: EPG write failed: %v", err)
		}
	}
	if settings.EPGFile != "" && settings.EPGPublicFile != "" {
		if raw, err := os.ReadFile(settings.EPGFile); err == nil {
			if _, err := atomicfile.WriteIfChanged(settings.EPGPublicFile, raw, 0o644); err != nil {
				logger.Printf("WARNING: EPG publish failed: %v", err)
			}
		}
	}
	if settings.EPGCompareSource == "" {
		if _, err := os.Stat(settings.EPGFile); err == nil {
			settings.EPGCompareSource = settings.EPGFile
		}
	}

	settings.TokenServer = resolveValue(settings.TokenServer, creds["PROVIDER_TOKEN_SERVER"], "")
	settings.PlatformOrigin = resolveValue(settings.PlatformOrigin, creds["PROVIDER_PLATFORM_ORIGIN"], "")
	settings.EPGEntry = resolveValue(settings.EPGEntry, creds["PROVIDER_EPG_ENTRY"], "")
	settings.EASIP = resolveValue(settings.EASIP, creds["PROVIDER_EASIP"], "")
	settings.NetworkID = resolveValue(settings.NetworkID, creds["PROVIDER_NETWORKID"], "")
	settings.CityCode = resolveValue(settings.CityCode, creds["PROVIDER_CITYCODE"], "")
	settings.STBType = resolveValue(settings.STBType, creds["PROVIDER_STB_TYPE"], "")
	settings.PRMID = resolveValue(settings.PRMID, creds["PROVIDER_PRMID"], "")
	settings.DRMSupplier = resolveValue(settings.DRMSupplier, creds["PROVIDER_DRM_SUPPLIER"], "")
	settings.UserAgent = resolveValue(settings.UserAgent, creds["PROVIDER_USER_AGENT"], "")
	missingProviderValues := []string{}
	for key, value := range map[string]string{
		"TOKEN_SERVER":    settings.TokenServer,
		"PLATFORM_ORIGIN": settings.PlatformOrigin,
		"EPG_ENTRY":       settings.EPGEntry,
		"EASIP":           settings.EASIP,
		"NETWORKID":       settings.NetworkID,
		"STB_TYPE":        settings.STBType,
	} {
		if value == "" || strings.EqualFold(value, "auto") {
			missingProviderValues = append(missingProviderValues, key)
		}
	}
	if len(missingProviderValues) > 0 {
		slices.Sort(missingProviderValues)
		return Report{}, fmt.Errorf("provider metadata missing (%s); recapture credentials or configure the provider environment", strings.Join(missingProviderValues, ", "))
	}
	if fallback := snapshotEPGHost(settings.SnapshotPath); fallback != "" && fallback != settings.EPGEntry {
		settings.EPGFallbacks = append(settings.EPGFallbacks, fallback)
	}

	logger.Printf("[2/3] authenticating and fetching channels from %s", settings.EPGEntry)
	client, err := portal.New(portal.Config{
		TokenServer: settings.TokenServer, PlatformOrigin: settings.PlatformOrigin, EPGEntry: settings.EPGEntry,
		EPGFallbacks: settings.EPGFallbacks, EASIP: settings.EASIP, NetworkID: settings.NetworkID, CityCode: settings.CityCode,
		UserAgent: settings.UserAgent, BindInterface: settings.BindInterface, BindSourceIP: settings.BindSourceIP, Timeout: settings.ProviderTimeout,
	})
	if err != nil {
		return Report{}, err
	}
	fetched, err := client.Fetch(ctx, portal.Credentials{
		UserID: creds["PROVIDER_USER_ID"], STBID: creds["PROVIDER_STBID"], Authenticator: creds["PROVIDER_AUTHENTICATOR"],
		STBInfo: creds["PROVIDER_STBINFO"], UserToken: creds["PROVIDER_USER_TOKEN"], STBType: settings.STBType,
		PRMID: settings.PRMID, DRMSupplier: settings.DRMSupplier,
	})
	if err != nil {
		return Report{}, err
	}
	if err := atomicfile.Write(settings.SnapshotPath, []byte(fetched.Frameset), 0o600); err != nil {
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
	if capturedFreshCredentials && settings.RestartRTP2HTTPDAfterCapture {
		logger.Printf("credential capture refresh complete; restarting rtp2httpd")
		if err := r.restartRTP2HTTPD(ctx); err != nil {
			logger.Printf("WARNING: %v", err)
		} else {
			logger.Printf("rtp2httpd restarted after credential capture")
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
	var text string
	if format == "m3u" {
		text = playlist.RenderM3U(rows, options)
	} else {
		text = playlist.RenderTXT(rows)
	}
	_, err := atomicfile.WriteIfChanged(path, []byte(text), 0o644)
	return err
}

func interfaceIPv4(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", fmt.Errorf("provider HTTP interface %q is unavailable: %w", name, err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("read provider HTTP interface %q addresses: %w", name, err)
	}
	for _, address := range addresses {
		if ip, _, err := net.ParseCIDR(address.String()); err == nil && ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("provider HTTP interface %q has no IPv4 address; select the logical IPTV interface or set PROVIDER_BIND_INTERFACE=none", name)
}

var snapshotHostRE = regexp.MustCompile(`(https?://[A-Za-z0-9.-]+(?::[0-9]+)?)/iptvepg`)

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
	if endpoint == "" || strings.EqualFold(endpoint, "auto") {
		return ""
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	host, _, err := net.SplitHostPort(u.Host)
	if err == nil {
		return host
	}
	return u.Hostname()
}
