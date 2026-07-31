package playlist

import (
	"strings"
	"testing"
)

func TestBuildCatchupSourceAddsR2HToken(t *testing.T) {
	raw := "rtsp://provider.test/live/channel?AuthInfo=abc&r2h-token=old&tvdr=old"
	got := BuildCatchupSource(raw, "10.1.1.1:7088", "{(b)YmdHMS}-{(e)YmdHMS}", "-900", "new token")
	for _, want := range []string{
		"http://10.1.1.1:7088/rtsp/provider.test/live/channel?",
		"AuthInfo=abc",
		"playseek={(b)YmdHMS}-{(e)YmdHMS}",
		"r2h-seek-offset=-900",
		"r2h-token=new+token",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("catch-up URL %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "r2h-token=old") || strings.Contains(got, "tvdr=") {
		t.Fatalf("catch-up URL retained stale parameters: %q", got)
	}
}

func TestApplyOperatorCatchupOverridesOnlyGuideChannels(t *testing.T) {
	rows := []Row{
		{ProviderID: "one", Name: "一套"},
		{ProviderID: "two", Name: "二套"},
	}
	current := map[string]Catchup{
		"一套": {Source: "http://short/one", Days: 1},
		"二套": {Source: "http://short/two", Days: 1},
	}
	guide := OperatorGuide{
		Channels:   []OperatorChannel{{ID: "one", Name: "一套"}},
		Programmes: []OperatorProgramme{{ID: "programme-one", ChannelID: "one"}},
	}
	got := ApplyOperatorCatchup(rows, guide, current, "http://router/iptv/catchup", 7)
	if got["一套"].Days != 7 || got["一套"].Source != "http://router/iptv/catchup?channel=one&start={(b)YmdHMS}&end={(e)YmdHMS}" {
		t.Fatalf("operator catch-up = %#v", got["一套"])
	}
	if got["二套"] != current["二套"] {
		t.Fatalf("non-guide channel changed: %#v", got["二套"])
	}
}
