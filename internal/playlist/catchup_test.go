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
