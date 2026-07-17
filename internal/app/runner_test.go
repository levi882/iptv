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
	"testing"
	"time"

	"iptv/internal/playlist"
)

func TestOfflineRefresh(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "playlist", "testdata", "frameset_builder.jsp"))
	if err != nil {
		t.Fatal(err)
	}
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iptvepg/function/index.jsp", "/iptvepg/function/funcportalauth.jsp":
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/frameset_builder.jsp":
			_, _ = w.Write(fixture)
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()
	root := t.TempDir()
	creds := filepath.Join(root, "hb.creds.env")
	if err := os.WriteFile(creds, []byte("HB_USER_ID=u\nHB_STBID=AA\nHB_STBINFO=BB\nHB_USER_TOKEN=token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "config", "local", "local_stb.m3u")
	settings := Settings{
		RepoRoot: root, CredsFile: creds, SkipCapture: true, OutputPath: output,
		SnapshotPath: filepath.Join(root, "frameset_builder_latest.jsp"), OutputFormat: "m3u", Mode: "auto", SortBy: "user_channel_id",
		TokenServer: portal.URL, PlatformOrigin: portal.URL, EPGEntry: portal.URL, EASIP: "127.0.0.1", NetworkID: "1", HBTimeout: 3 * time.Second,
		R2HIGMPPath: "udp", R2HFCCTYPE: "telecom", LineTagRule: "none", DisplayNameMode: "name", CatchupType: "shift",
		CatchupPlayseek: "{(b)YmdHMS}-{(e)YmdHMS}", LogoMatchThreshold: .65,
	}
	report, err := (Runner{}).Run(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if report.Channels != 4 || report.Timeshift != 3 {
		t.Fatalf("unexpected report: %#v", report)
	}
	playlist, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(playlist), "#EXTM3U\n") || strings.Count(string(playlist), "#EXTINF:") != 4 {
		t.Fatalf("invalid playlist, size=%d", len(playlist))
	}
}

func TestCurrentGeneratedArtifactsParityWhenPresent(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	settings, _, err := LoadSettings(repo, filepath.Join(repo, "scripts", "hb.env"))
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
		if names, err := playlist.ParseEPG(epgRaw); err == nil {
			mapped, _ := playlist.AttachEPG(rows, names, settings.EPGReplaceName)
			t.Logf("local artifact EPG mapped=%d names=%d", mapped, len(names))
		}
	}
	logoCandidates, err := playlist.ParseLogoCandidates(currentMain, "")
	if err != nil {
		t.Fatal(err)
	}
	logoMatched := playlist.AttachLogos(rows, logoCandidates, settings.LogoMatchThreshold)
	t.Logf("local artifact logos matched=%d candidates=%d", logoMatched, len(logoCandidates))
	groups := groupsFromM3U(string(currentMain))
	catchup = playlist.ConvertCatchup(catchup, settings.R2HCatchupHost, settings.CatchupPlayseek, settings.CatchupSeekOffset)
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
	const openWrtRoot = "/mnt/sda1/iptv/"
	if after, ok := strings.CutPrefix(filepath.ToSlash(path), openWrtRoot); ok {
		return filepath.Join(repo, filepath.FromSlash(after))
	}
	return path
}
