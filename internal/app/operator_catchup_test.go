package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iptv/internal/playlist"
)

func TestOperatorCatchupResolverUsesCachedProgrammeID(t *testing.T) {
	var providerURL string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iptvepg/function/index.jsp":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session", Path: "/"})
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/funcportalauth.jsp":
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/frameset_builder.jsp":
			fmt.Fprint(w, `jsSetConfig('Channel','ChannelID="one",ChannelName="一套",ChannelURL="igmp://239.1.1.1:1234"');`)
		case "/iptvepg/frame234/authBySecond.jsp":
			fmt.Fprintf(w, `<script>var ipport=%q;</script>`, providerURL)
		case "/authZX/u":
			fmt.Fprint(w, `{"iptvToken":"secondary","respCode":"00000"}`)
		case "/iptvepg/frame226/publicPage/datajsp/getTVODPlayURL.jsp":
			if r.URL.Query().Get("programCode") != "programme-one" || r.URL.Query().Get("channelID") != "one" {
				http.Error(w, "bad identifiers", http.StatusBadRequest)
				return
			}
			if cookie, err := r.Cookie("iptvToken"); err != nil || cookie.Value != "secondary" {
				http.Error(w, "missing token", http.StatusForbidden)
				return
			}
			fmt.Fprint(w, `{"playUrl":"rtsp://media.test/HBGD/channel-one?Playseek=20260801010000-20260801020000&AuthInfo=test"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	providerURL = provider.URL
	defer provider.Close()

	root := t.TempDir()
	credsPath := filepath.Join(root, "provider.creds.env")
	if err := os.WriteFile(credsPath, []byte("PROVIDER_USER_ID=u\nPROVIDER_STBID=stb\nPROVIDER_STBINFO=info\nPROVIDER_USER_TOKEN=token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	stop := start.Add(time.Hour)
	guide := playlist.OperatorGuide{
		Channels: []playlist.OperatorChannel{{ID: "one", Name: "一套", Number: "1"}},
		Programmes: []playlist.OperatorProgramme{{
			ID: "programme-one", ChannelID: "one", Title: "节目", Start: start, Stop: stop,
		}},
	}
	raw, err := playlist.RenderOperatorEPG(guide)
	if err != nil {
		t.Fatal(err)
	}
	epgPath := filepath.Join(root, "operator.xml.gz")
	if err := writeOperatorEPG(epgPath, raw); err != nil {
		t.Fatal(err)
	}
	settings := Settings{
		RepoRoot: root, CredsFile: credsPath, EPGFile: epgPath,
		TokenServer: provider.URL, PlatformOrigin: provider.URL, EPGEntry: provider.URL,
		EASIP: "127.0.0.1", NetworkID: "1", STBType: "test", UserAgent: "test", ProviderTimeout: 3 * time.Second,
		GuideTemplate: "frame226", R2HBaseURL: "http://router.test:5140", R2HToken: "r2h-test",
	}
	resolver := NewOperatorCatchupResolver(settings, nil)
	got, err := resolver.ResolveCatchup(context.Background(), "one", start, stop)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"http://router.test:5140/rtsp/media.test/HBGD/channel-one?", "Playseek=", "r2h-token=r2h-test"} {
		if !strings.Contains(got, want) {
			t.Fatalf("resolved URL %q does not contain %q", got, want)
		}
	}
	if _, err := resolver.ResolveCatchup(context.Background(), "one", start.Add(-time.Hour), stop.Add(-time.Hour)); err != ErrOperatorProgrammeNotFound {
		t.Fatalf("missing programme error = %v", err)
	}
}
