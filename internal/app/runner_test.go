package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"iptv/internal/capture"
	"iptv/internal/config"
	"iptv/internal/playlist"
)

func TestOfflineRefresh(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "playlist", "testdata", "frameset_builder.jsp"))
	if err != nil {
		t.Fatal(err)
	}
	var portalURL string
	var operatorGuideFailure atomic.Bool
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iptvepg/function/index.jsp":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session", Path: "/"})
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/funcportalauth.jsp":
			if err := r.ParseForm(); err != nil || r.Form.Get("stbtype") != "Captured-STB" || r.Form.Get("prmid") != "captured-prmid" || r.Form.Get("drmsupplier") != "captured-drm" || r.UserAgent() != "Captured-UA" {
				http.Error(w, "captured portal parameters were not replayed", http.StatusForbidden)
				return
			}
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/frameset_builder.jsp":
			_, _ = w.Write(fixture)
		case "/iptvepg/frame234/authBySecond.jsp":
			fmt.Fprintf(w, `<script>var ipport = %q;</script>`, portalURL)
		case "/authZX/u":
			fmt.Fprint(w, `{"iptvToken":"guide-token","respCode":"00000"}`)
		case "/iptvepg/frame226/publicPage/datajsp/channelToLiveFullScreen.jsp":
			if cookie, err := r.Cookie("iptvToken"); err != nil || cookie.Value != "guide-token" {
				http.Error(w, "missing guide token", http.StatusForbidden)
				return
			}
			fmt.Fprint(w, `{"totalSize":2,"channelDataList":[{"channelName":"CCTV1HD","channelID":"channel-1","channelIndex":"1"},{"channelName":"Demo4K","channelID":"channel-2","channelIndex":"2"}]}`)
		case "/iptvepg/frame226/publicPage/datajsp/prevueList.jsp":
			if operatorGuideFailure.Load() {
				http.Error(w, "temporary guide failure", http.StatusServiceUnavailable)
				return
			}
			date, err := time.Parse("20060102", r.URL.Query().Get("curdate"))
			if err != nil || r.URL.Query().Get("pageSize") != "999" {
				http.Error(w, "bad guide request", http.StatusBadRequest)
				return
			}
			channelID := r.URL.Query().Get("channelID")
			start := date.Format("2006.01.02") + " 00:00:00"
			stop := date.Format("2006.01.02") + " 01:00:00"
			fmt.Fprintf(w, `{"totalSize":1,"channelPrevueList":[{"prevueName":%q,"prevuecode":%q,"startTime":%q,"endTime":%q}]}`, channelID+"节目", channelID+date.Format("20060102"), start, stop)
		default:
			http.NotFound(w, r)
		}
	}))
	portalURL = portal.URL
	defer portal.Close()
	root := t.TempDir()
	creds := filepath.Join(root, "provider.creds.env")
	if err := os.WriteFile(creds, []byte("PROVIDER_USER_ID=u\nPROVIDER_STBID=AA\nPROVIDER_STBINFO=BB\nPROVIDER_USER_TOKEN=token\nPROVIDER_STB_TYPE=Captured-STB\nPROVIDER_PRMID=captured-prmid\nPROVIDER_DRM_SUPPLIER=captured-drm\nPROVIDER_USER_AGENT=Captured-UA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "config", "local", "local_stb.m3u")
	epgFile := filepath.Join(root, "cache", "operator.xml.gz")
	epgPublicFile := filepath.Join(root, "public", "operator.xml.gz")
	settings := Settings{
		RepoRoot: root, CredsFile: creds, SkipCapture: true, OutputPath: output,
		SnapshotPath: filepath.Join(root, "frameset_builder_latest.jsp"), OutputFormat: "m3u", Mode: "auto", SortBy: "user_channel_id",
		TokenServer: portal.URL, PlatformOrigin: portal.URL, EPGEntry: portal.URL, EASIP: "127.0.0.1", NetworkID: "1", ProviderTimeout: 3 * time.Second,
		EPGFile: epgFile, EPGPublicFile: epgPublicFile, XTvgURL: "http://router.test/operator.xml.gz", GuideTemplate: "frame226", GuideHistoryDays: 1,
		ProviderCatchupURL: "http://router.test/iptv/catchup",
		R2HIGMPPath:        "udp", R2HFCCTYPE: "telecom", LineTagRule: "none", DisplayNameMode: "name", CatchupType: "shift",
		CatchupPlayseek: "{(b)YmdHMS}-{(e)YmdHMS}", LogoMatchThreshold: .65,
		RestartRTP2HTTPDAfterCapture: true,
	}
	restartCalls := 0
	runner := Runner{RestartRTP2HTTPD: func(context.Context) error {
		restartCalls++
		return nil
	}}
	report, err := runner.Run(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if restartCalls != 0 {
		t.Fatalf("saved-credential refresh restarted rtp2httpd %d times", restartCalls)
	}
	if report.Channels != 4 || report.Timeshift != 3 || report.EPGMapped != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	playlistRaw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(playlistRaw), "#EXTM3U") || strings.Count(string(playlistRaw), "#EXTINF:") != 4 {
		t.Fatalf("invalid playlist, size=%d", len(playlistRaw))
	}
	if !strings.Contains(string(playlistRaw), `tvg-id="channel-1"`) || !strings.Contains(string(playlistRaw), `x-tvg-url="http://router.test/operator.xml.gz"`) {
		t.Fatalf("playlist did not use operator EPG IDs:\n%s", playlistRaw)
	}
	if !strings.Contains(string(playlistRaw), `catchup-source="http://router.test/iptv/catchup?channel=channel-1&start={(b)YmdHMS}&end={(e)YmdHMS}"`) {
		t.Fatalf("playlist did not use operator TVOD catch-up:\n%s", playlistRaw)
	}
	epgRaw, err := os.ReadFile(epgFile)
	if err != nil {
		t.Fatal(err)
	}
	guide, recognized, err := playlist.ParseOperatorEPG(epgRaw, time.Local)
	if err != nil || !recognized || len(guide.Channels) != 2 || len(guide.Programmes) != 4 {
		t.Fatalf("generated operator EPG recognized=%v err=%v guide=%#v", recognized, err, guide)
	}
	if publicRaw, err := os.ReadFile(epgPublicFile); err != nil || !bytes.Equal(epgRaw, publicRaw) {
		t.Fatalf("published EPG mismatch: err=%v cache=%d public=%d", err, len(epgRaw), len(publicRaw))
	}

	operatorGuideFailure.Store(true)
	if _, err := runner.Run(context.Background(), settings); err != nil {
		t.Fatalf("refresh with operator guide failure: %v", err)
	}
	fallbackPlaylist, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fallbackPlaylist), `catchup-source="http://router.test/iptv/catchup?channel=channel-1&start={(b)YmdHMS}&end={(e)YmdHMS}"`) {
		t.Fatalf("cached operator EPG was not retained after guide failure:\n%s", fallbackPlaylist)
	}
	operatorGuideFailure.Store(false)

	settings.SkipCapture = false
	runner.Capture = func(context.Context, capture.Options) (config.Env, error) {
		return config.Load(creds)
	}
	if _, err := runner.Run(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if restartCalls != 1 {
		t.Fatalf("successful credential capture refresh restarted rtp2httpd %d times", restartCalls)
	}
}

func TestRefreshRejectsUnavailableProviderInterface(t *testing.T) {
	root := t.TempDir()
	creds := filepath.Join(root, "provider.creds.env")
	if err := os.WriteFile(creds, []byte("PROVIDER_USER_ID=u\nPROVIDER_STBID=AA\nPROVIDER_STBINFO=BB\nPROVIDER_USER_TOKEN=token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := Settings{
		RepoRoot: root, CredsFile: creds, SkipCapture: true,
		BindInterface: "interface-that-does-not-exist", RefreshTimeout: time.Second,
	}
	_, err := (Runner{}).Run(context.Background(), settings)
	if err == nil || !strings.Contains(err.Error(), "provider HTTP interface") || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unexpected interface error: %v", err)
	}
}

func TestRefreshHonorsOverallTimeout(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer portal.Close()

	root := t.TempDir()
	creds := filepath.Join(root, "provider.creds.env")
	if err := os.WriteFile(creds, []byte("PROVIDER_USER_ID=u\nPROVIDER_STBID=AA\nPROVIDER_STBINFO=BB\nPROVIDER_USER_TOKEN=token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := Settings{
		RepoRoot: root, CredsFile: creds, SkipCapture: true,
		TokenServer: portal.URL, PlatformOrigin: portal.URL, EPGEntry: portal.URL, EASIP: "127.0.0.1", NetworkID: "1", STBType: "Demo-STB",
		ProviderTimeout: 3 * time.Second, RefreshTimeout: 50 * time.Millisecond,
	}
	started := time.Now()
	_, err := (Runner{}).Run(context.Background(), settings)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("overall timeout took too long: %s", elapsed)
	}
}

func TestCurrentGeneratedArtifactsParityWhenPresent(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	settings, _, err := LoadSettings(repo, filepath.Join(repo, "scripts", "provider.env"))
	if err != nil {
		t.Skipf("local runtime config unavailable: %v", err)
	}
	settings.OutputPath = localArtifactPath(repo, settings.OutputPath)
	settings.SnapshotOutputPath = localArtifactPath(repo, settings.SnapshotOutputPath)
	settings.EPGFile = localArtifactPath(repo, settings.EPGFile)
	frameset, err := os.ReadFile(settings.SnapshotPath)
	if err != nil {
		t.Skipf("local frameset unavailable: %v", err)
	}
	currentMain, err := os.ReadFile(settings.OutputPath)
	if err != nil {
		t.Skipf("current generated playlist unavailable: %v", err)
	}
	channels := playlist.ParseChannels(string(frameset), playlist.URLSelectParams{
		Mode: settings.Mode, IGMPHTTPPrefix: settings.IGMPHTTPPrefix, R2HBaseURL: settings.R2HBaseURL,
		R2HIGMPPath: settings.R2HIGMPPath, R2HToken: settings.R2HToken, R2HAddFCC: settings.R2HAddFCC,
		R2HFCCTYPE: settings.R2HFCCTYPE, R2HProxyRTSP: settings.R2HProxyRTSP,
	}, settings.LineTagRule, settings.LineTagUHD, settings.LineTagHD, settings.LineTagSD)
	_, catchup, lengths := playlist.ChannelsToRows(channels)
	playlist.SortChannels(channels, settings.SortBy)
	rows, _, _ := playlist.ChannelsToRows(channels)
	if epgRaw, err := os.ReadFile(settings.EPGFile); err == nil {
		if guide, recognized, err := playlist.ParseOperatorEPG(epgRaw, time.Local); err == nil && recognized {
			mapped := playlist.AttachOperatorEPG(rows, guide)
			t.Logf("local operator EPG mapped=%d channels=%d", mapped, len(guide.Channels))
		}
	}
	logoCandidates, err := playlist.ParseLogoCandidates(currentMain, "")
	if err != nil {
		t.Fatal(err)
	}
	logoMatched := playlist.AttachLogos(rows, logoCandidates, settings.LogoMatchThreshold)
	t.Logf("local artifact logos matched=%d candidates=%d", logoMatched, len(logoCandidates))
	groups := groupsFromM3U(string(currentMain))
	catchup = playlist.ConvertCatchup(catchup, settings.R2HCatchupHost, settings.CatchupPlayseek, settings.CatchupSeekOffset, settings.R2HToken)
	options := playlist.RenderOptions{
		DisplayNameMode: settings.DisplayNameMode, XTvgURL: settings.XTvgURL, GroupNames: groups,
		Catchup: catchup, TimeShiftLength: lengths, CatchupType: settings.CatchupType,
	}
	gotMain := []byte(playlist.RenderM3U(rows, options))
	// Normalize an invalid duplicated end placeholder that may be present in
	// previously generated local artifacts.
	buggyPlayseek := []byte(`playseek={(b)YmdHMS}-{(e)YmdHMS}-{(e)YmdHMS}}`)
	fixedPlayseek := []byte(`playseek={(b)YmdHMS}-{(e)YmdHMS}`)
	normalizedCurrent := bytes.ReplaceAll(currentMain, buggyPlayseek, fixedPlayseek)
	if !bytes.Equal(gotMain, normalizedCurrent) {
		t.Fatalf("Go output differs from current generated main playlist beyond the known shell playseek bug: got=%d bytes want=%d", len(gotMain), len(normalizedCurrent))
	}
	if settings.SnapshotOutputPath == "" {
		return
	}
	currentSnapshot, err := os.ReadFile(settings.SnapshotOutputPath)
	if err != nil {
		t.Skipf("current snapshot playlist unavailable: %v", err)
	}
	snapshotRows := make([]playlist.Row, 0, len(rows))
	for _, row := range rows {
		if value := playlist.SnapshotURL(row.URL, settings.R2HBaseURL); value != "" {
			row.URL = value
			snapshotRows = append(snapshotRows, row)
		}
	}
	options.Catchup = nil
	options.TimeShiftLength = nil
	gotSnapshot := []byte(playlist.RenderM3U(snapshotRows, options))
	if !bytes.Equal(gotSnapshot, currentSnapshot) {
		t.Fatalf("Go output differs from current generated snapshot playlist: got=%d bytes want=%d", len(gotSnapshot), len(currentSnapshot))
	}
}

var extinfLineRE = regexp.MustCompile(`(?m)^#EXTINF[^\r\n]*`)
var groupAttrRE = regexp.MustCompile(`group-title="([^"]*)"`)
var tvgNameAttrRE = regexp.MustCompile(`tvg-name="([^"]*)"`)

func groupsFromM3U(text string) map[string]string {
	out := map[string]string{}
	for _, line := range extinfLineRE.FindAllString(text, -1) {
		groupMatch := groupAttrRE.FindStringSubmatch(line)
		if groupMatch == nil {
			continue
		}
		names := []string{}
		if match := tvgNameAttrRE.FindStringSubmatch(line); match != nil {
			names = append(names, match[1])
		}
		if _, title, ok := strings.Cut(line, ","); ok {
			names = append(names, title)
		}
		for _, name := range names {
			if key := playlist.NormalizeName(name); key != "" {
				out[key] = groupMatch[1]
			}
		}
	}
	return out
}

func localArtifactPath(repo, path string) string {
	const openWrtRoot = "/mnt/iptv/iptv-refresh/"
	if after, ok := strings.CutPrefix(filepath.ToSlash(path), openWrtRoot); ok {
		return filepath.Join(repo, filepath.FromSlash(after))
	}
	return path
}

func TestTokenHostSupportsAutomaticDiscovery(t *testing.T) {
	if got := tokenHost("auto"); got != "" {
		t.Fatalf("automatic token host = %q", got)
	}
	if got := tokenHost("http://203.0.113.10:4338"); got != "203.0.113.10" {
		t.Fatalf("explicit token host = %q", got)
	}
}
