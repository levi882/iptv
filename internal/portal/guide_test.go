package portal

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchGuideAuthenticatesAndFetchesChannelsAndDays(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, location)
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/authZX/user-1" || r.Referer() == "" {
			http.Error(w, "bad secondary auth request", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"iptvToken":"guide-token","respCode":"00000","respMsg":"ok"}`)
	}))
	defer auth.Close()

	var mu sync.Mutex
	programmeRequests := map[string]int{}
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
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
			fmt.Fprintf(w, `<script>var ipport = %q;</script>`, auth.URL)
		case "/iptvepg/frame226/publicPage/datajsp/channelToLiveFullScreen.jsp":
			if cookie, err := r.Cookie("iptvToken"); err != nil || cookie.Value != "guide-token" {
				http.Error(w, "missing IPTV token", http.StatusForbidden)
				return
			}
			fmt.Fprint(w, `{"totalSize":2,"curPage":1,"totalPage":1,"channelDataList":[{"channelName":"二套","channelID":"two","channelIndex":"2"},{"channelName":"一套","channelID":"one","channelIndex":"1"}]}`)
		case "/iptvepg/frame226/publicPage/datajsp/prevueList.jsp":
			if !strings.Contains(r.Referer(), "/iptvepg/frame226/publicPage/channelPlayer/index.jsp?channelIndex=") {
				http.Error(w, "missing channel player referer", http.StatusBadRequest)
				return
			}
			active := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				previous := maxInFlight.Load()
				if active <= previous || maxInFlight.CompareAndSwap(previous, active) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			if cookie, err := r.Cookie("JSESSIONID"); err != nil || cookie.Value != "session" {
				http.Error(w, "missing portal session", http.StatusForbidden)
				return
			}
			date := r.URL.Query().Get("curdate")
			channel := r.URL.Query().Get("channelID")
			wantFirst := "7"
			if date == "20260730" {
				wantFirst = "6"
			}
			if r.URL.Query().Get("isFristDate") != wantFirst || r.URL.Query().Get("pageSize") != "999" {
				http.Error(w, "bad guide parameters", http.StatusBadRequest)
				return
			}
			mu.Lock()
			programmeRequests[channel+"/"+date]++
			mu.Unlock()
			parsed, _ := time.Parse("20060102", date)
			start := parsed.Format("2006.01.02") + " 00:00:00"
			stop := parsed.Format("2006.01.02") + " 01:00:00"
			fmt.Fprintf(w, `{"totalSize":1,"channelPrevueList":[{"prevueName":%q,"prevuecode":%q,"startTime":%q,"endTime":%q}]}`, channel+"节目", channel+date, start, stop)
		case "/iptvepg/frame226/publicPage/datajsp/getTVODPlayURL.jsp":
			if cookie, err := r.Cookie("iptvToken"); err != nil || cookie.Value != "guide-token" {
				http.Error(w, "missing TVOD token", http.StatusForbidden)
				return
			}
			if r.URL.Query().Get("programCode") != "programme-one" || r.URL.Query().Get("channelID") != "one" {
				http.Error(w, "bad TVOD identifiers", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"playUrl":"rtsp://media.test/HBGD/channel-one?Playseek=20260731010000-20260731020000&AuthInfo=secret"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	client, err := New(Config{EPGEntry: provider.URL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	portalResult, err := client.Fetch(context.Background(), Credentials{UserID: "user-1", STBID: "stb", STBInfo: "info", UserToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	guide, err := client.FetchGuide(context.Background(), portalResult.EPGHost, "user-1", GuideOptions{Template: "frame226", Now: now, Days: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(guide.Channels) != 2 || len(guide.Programmes) != 4 {
		t.Fatalf("guide = %#v", guide)
	}
	playURL, err := client.FetchCatchupURL(context.Background(), portalResult.EPGHost, "frame226", "programme-one", "one")
	if err != nil || !strings.HasPrefix(playURL, "rtsp://media.test/HBGD/channel-one?") {
		t.Fatalf("TVOD URL = %q, err=%v", playURL, err)
	}
	for _, key := range []string{"one/20260731", "one/20260730", "two/20260731", "two/20260730"} {
		if programmeRequests[key] != 1 {
			t.Fatalf("programme request %s count=%d", key, programmeRequests[key])
		}
	}
	if maxInFlight.Load() != 1 {
		t.Fatalf("default guide worker concurrency = %d, want 1", maxInFlight.Load())
	}
}

func TestGetJSONRetriesEmptyResponseAndDecodesGzip(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch attempts.Add(1) {
		case 1:
			_, _ = fmt.Fprint(w, `{"ok":`)
			return
		case 2:
			w.WriteHeader(http.StatusOK)
			return
		}
		var body bytes.Buffer
		writer := gzip.NewWriter(&body)
		_, _ = writer.Write([]byte(`{"ok":"yes"}`))
		_ = writer.Close()
		w.Header().Set("Content-Type", "text/html;charset=UTF-8")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(body.Bytes())
	}))
	defer server.Close()

	client, err := New(Config{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		OK string `json:"ok"`
	}
	if err := client.getJSON(context.Background(), server.URL+"/guide", "", &response); err != nil {
		t.Fatal(err)
	}
	if response.OK != "yes" {
		t.Fatalf("response = %#v, want ok=yes", response)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("request attempts = %d, want 3", got)
	}
}

func TestFetchGuideRejectsInvalidTemplate(t *testing.T) {
	client, err := New(Config{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.FetchGuide(context.Background(), "http://example.test", "user", GuideOptions{Template: "../frame226", Now: time.Now(), Days: 1})
	if err == nil {
		t.Fatal("invalid guide template was accepted")
	}
}
