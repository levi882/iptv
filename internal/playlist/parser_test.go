package playlist

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSanitizedSnapshot(t *testing.T) {
	path := filepath.Join("testdata", "frameset_builder.jsp")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	channels := ParseChannels(string(raw), URLSelectParams{Mode: "auto"}, "none", "超高清", "高清", "标清")
	if len(channels) != 4 {
		t.Fatalf("channel count = %d, want 4", len(channels))
	}
	timeshiftNames := map[string]bool{}
	for _, channel := range channels {
		if channel.TimeShiftDays > 0 {
			timeshiftNames[channel.Name] = true
		}
		if channel.Name == "" || channel.URL == "" {
			t.Fatalf("invalid channel: %#v", channel)
		}
	}
	if len(timeshiftNames) != 3 {
		t.Fatalf("timeshift count = %d, want 3", len(timeshiftNames))
	}
}

func TestSelectURL(t *testing.T) {
	p := URLSelectParams{Mode: "auto", R2HBaseURL: "http://192.0.2.1:7088", R2HIGMPPath: "udp", R2HToken: "x", R2HAddFCC: true, R2HFCCTYPE: "telecom"}
	got := SelectURL("", "rtsp://192.0.2.1/live?x=1", "igmp://239.1.1.1:1234|rtsp://192.0.2.2/live", "192.0.2.3", "123", p)
	if got != "http://192.0.2.1:7088/udp/239.1.1.1:1234?r2h-token=x&fcc=192.0.2.3%3A123&fcc-type=telecom" {
		t.Fatalf("unexpected URL: %s", got)
	}
	if got := SnapshotURL("http://192.0.2.1:7088/udp/239.1.1.1:1234$高清", p.R2HBaseURL); got != "http://192.0.2.1:7088/udp/239.1.1.1:1234?snapshot=1$高清" {
		t.Fatalf("unexpected snapshot URL: %s", got)
	}
}

func TestRenderM3U(t *testing.T) {
	rows := []Row{{Name: "CCTV-1 HD", URL: "igmp://239.1.1.1:1234", EPGID: "cctv1", EPGName: "CCTV1", LogoURL: "http://logo/1.png"}}
	got := RenderM3U(rows, RenderOptions{XTvgURL: "http://epg/e1.xml.gz", DisplayNameMode: "tvg_name", TimeShiftLength: map[string]int{"CCTV-1 HD": 86400}})
	want := "#EXTM3U x-tvg-url=\"http://epg/e1.xml.gz\"\n#EXTINF:-1 tvg-id=\"cctv1\" tvg-name=\"CCTV1\" tvg-logo=\"http://logo/1.png\" group-title=\"央视\" x-r2h-timeshift-length=\"86400\",CCTV1\nigmp://239.1.1.1:1234\n"
	if got != want {
		t.Fatalf("render mismatch\n got: %q\nwant: %q", got, want)
	}
}

// This golden hash covers the sanitized frameset fixture with auto mode,
// user_channel_id sorting, and M3U output. It prevents subtle attribute,
// ordering, or catch-up regressions.
func TestPlaylistGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "frameset_builder.jsp"))
	if err != nil {
		t.Fatal(err)
	}
	channels := ParseChannels(string(raw), URLSelectParams{Mode: "auto"}, "none", "超高清", "高清", "标清")
	_, catchup, lengths := ChannelsToRows(channels)
	SortChannels(channels, "user_channel_id")
	rows, _, _ := ChannelsToRows(channels)
	output := RenderM3U(rows, RenderOptions{Catchup: catchup, TimeShiftLength: lengths, CatchupType: "shift"})
	sum := sha256.Sum256([]byte(output))
	got := hex.EncodeToString(sum[:])
	const want = "a467c268d12f01d4dfcf917380ab5e8e01aecbb8df9499861e8ef483b3210bf2"
	if got != want {
		t.Fatalf("playlist golden hash = %s, want %s (bytes=%d)", got, want, len(output))
	}
}

func TestR2HPlaylistGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "frameset_builder.jsp"))
	if err != nil {
		t.Fatal(err)
	}
	params := URLSelectParams{
		Mode: "auto", R2HBaseURL: "http://192.0.2.1:7088", R2HIGMPPath: "udp",
		R2HAddFCC: true, R2HFCCTYPE: "telecom", R2HProxyRTSP: true,
	}
	channels := ParseChannels(string(raw), params, "hd_sd", "UHD", "HD", "SD")
	_, catchup, lengths := ChannelsToRows(channels)
	SortChannels(channels, "user_channel_id")
	rows, _, _ := ChannelsToRows(channels)
	catchup = ConvertCatchup(catchup, "192.0.2.1:7088", "{(b)YmdHMS}-{(e)YmdHMS}", "-900")
	output := RenderM3U(rows, RenderOptions{
		XTvgURL: "http://192.0.2.1/iptv_epg/e1.xml.gz", Catchup: catchup,
		TimeShiftLength: lengths, CatchupType: "shift",
	})
	sum := sha256.Sum256([]byte(output))
	got := hex.EncodeToString(sum[:])
	const want = "5502c69b713303c6874088ff03757fd47f788e1ca0d4c18cee4cfc15641fb22f"
	if got != want {
		t.Fatalf("R2H playlist golden hash = %s, want %s (bytes=%d)", got, want, len(output))
	}
}

func TestReferenceEmptyAttributes(t *testing.T) {
	rows := []Row{{Name: "Test", URL: "igmp://1", Ref: &Reference{EXTINF: `#EXTINF:-1 group-title="old",Test`}}}
	got := RenderM3U(rows, RenderOptions{})
	if !strings.Contains(got, `#EXTINF:-1 tvg-logo="" tvg-id="" group-title="old" tvg-name="Test",Test`) {
		t.Fatalf("missing empty attributes: %s", got)
	}
}
