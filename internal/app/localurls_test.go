package app

import "testing"

func TestParseR2HUCI(t *testing.T) {
	raw := "rtp2httpd.config=rtp2httpd\n" +
		"rtp2httpd.config.disabled='0'\n" +
		"rtp2httpd.config.listen='0.0.0.0:7088' '127.0.0.1:5140'\n" +
		"rtp2httpd.config.r2h_token='secret value'\n"
	got := parseR2HUCI(raw)
	if got.R2HHost != "" || got.R2HPort != "7088" || got.R2HToken != "secret value" {
		t.Fatalf("parsed rtp2httpd UCI = %#v", got)
	}
}

func TestParseR2HUCIUsesDefaultPort(t *testing.T) {
	got := parseR2HUCI("rtp2httpd.config.disabled='0'\n")
	if got.R2HPort != "5140" {
		t.Fatalf("default rtp2httpd port = %q", got.R2HPort)
	}
	fromFile := parseR2HUCI("rtp2httpd.config.use_config_file='1'\nrtp2httpd.config.port='7088'\n")
	if !fromFile.R2HConfigFile || fromFile.R2HPort != "7088" {
		t.Fatalf("config-file mode = %#v", fromFile)
	}
}

func TestParseR2HConfig(t *testing.T) {
	got := parseR2HConfig("[global]\nlisten = 5140\nr2h-token = token-1\n")
	if got.R2HPort != "5140" || got.R2HToken != "token-1" {
		t.Fatalf("parsed current config = %#v", got)
	}
	legacy := parseR2HConfig("* 7088\n")
	if legacy.R2HHost != "" || legacy.R2HPort != "7088" {
		t.Fatalf("parsed legacy config = %#v", legacy)
	}
}

func TestResolveLocalURLs(t *testing.T) {
	settings := Settings{
		EPGPublicFile:    "/www/iptv_epg/e1.xml.gz",
		LocalLogoDir:     "/www/iptv_logo",
		R2HBaseURL:       "auto",
		R2HCatchupHost:   "auto",
		XTvgURL:          "auto",
		LocalLogoURLBase: "auto",
	}
	got := resolveLocalURLs(settings, localServices{LANHost: "10.1.1.1", R2HPort: "7088", R2HToken: "token-1"})
	if got.R2HBaseURL != "http://10.1.1.1:7088" || got.R2HCatchupHost != "10.1.1.1:7088" {
		t.Fatalf("automatic rtp2httpd URLs = %#v", got)
	}
	if got.XTvgURL != "http://10.1.1.1/iptv_epg/e1.xml.gz" || got.LocalLogoURLBase != "http://10.1.1.1/iptv_logo" || got.R2HToken != "token-1" {
		t.Fatalf("automatic published URLs = %#v", got)
	}
}

func TestResolveLocalURLsPreservesExplicitAndSupportsOff(t *testing.T) {
	settings := Settings{
		R2HBaseURL:       "https://media.example.test/r2h",
		R2HCatchupHost:   "catchup.example.test:8443",
		XTvgURL:          "off",
		LocalLogoURLBase: "https://static.example.test/logo",
	}
	got := resolveLocalURLs(settings, localServices{LANHost: "10.1.1.1", R2HPort: "5140"})
	if got.R2HBaseURL != settings.R2HBaseURL || got.R2HCatchupHost != settings.R2HCatchupHost || got.XTvgURL != "" || got.LocalLogoURLBase != settings.LocalLogoURLBase {
		t.Fatalf("explicit URLs were not preserved: %#v", got)
	}
}
