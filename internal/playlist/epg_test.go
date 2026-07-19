package playlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEPGAndReference(t *testing.T) {
	raw := []byte(`<tv><channel id="cctv1"><display-name>CCTV-1</display-name><display-name>央视一套</display-name></channel></tv>`)
	names, err := ParseEPG(raw)
	if err != nil {
		t.Fatal(err)
	}
	rows := []Row{{Name: "CCTV1", URL: "igmp://1"}, {Name: "示例卫视", URL: "igmp://2"}}
	mapped, replaced := AttachEPG(rows, names, true)
	if mapped != 1 || replaced != 1 || rows[0].EPGID != "cctv1" || rows[0].Name != "CCTV-1" {
		t.Fatalf("unexpected EPG result: %#v mapped=%d replaced=%d", rows, mapped, replaced)
	}
	refs := ParseM3UReference("#EXTM3U\n#EXTINF:-1 group-title=\"x\",示例卫视\n#EXTVLCOPT:foo=bar\nigmp://old\n")
	ordered, matched := Reorder(rows, refs, true)
	if matched != 1 || ordered[0].Name != "示例卫视" || len(ordered[0].Ref.Options) != 1 {
		t.Fatalf("unexpected reorder: %#v", ordered)
	}
}

func TestLogoMatching(t *testing.T) {
	raw := []byte("#EXTM3U\n#EXTINF:-1 tvg-name=\"CCTV1\" tvg-logo=\"http://logo/1.png\",CCTV-1\nhttp://stream\n")
	candidates, err := ParseLogoCandidates(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := MatchLogo("CCTV-1高清", candidates, .65); got != "http://logo/1.png" {
		t.Fatalf("logo = %q", got)
	}
}

func TestSequenceRatioMatchesDifflibExamples(t *testing.T) {
	for _, test := range []struct {
		a, b string
		want float64
	}{
		{"abcd", "bcde", .75},
		{"CCTV1", "CCTV一套", 2.0 * 4.0 / 11.0},
		{"示例卫视高清", "示例卫视", .8},
	} {
		if got := sequenceRatio(test.a, test.b); got != test.want {
			t.Fatalf("sequenceRatio(%q,%q)=%v want %v", test.a, test.b, got, test.want)
		}
	}
}

func TestParseLocalEPGCacheWhenPresent(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "cache", "e1.xml.gz")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("local EPG cache is not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	names, err := ParseEPG(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 100 {
		t.Fatalf("EPG cache yielded only %d normalized names", len(names))
	}
}
