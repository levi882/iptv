package playlist

import "testing"

func TestParseGitHubContentsLogoDirectory(t *testing.T) {
	raw := []byte(`[
  {"type":"file","name":"CGTN.png","download_url":"https://raw.githubusercontent.com/fanmingming/live/main/tv/CGTN.png"},
  {"type":"dir","name":"m3u","download_url":null}
]`)
	candidates, err := ParseLogoCandidates(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	const want = "https://raw.githubusercontent.com/fanmingming/live/main/tv/CGTN.png"
	if got := MatchLogo("CGTN阿拉伯语", candidates, .65); got != want {
		t.Fatalf("CGTN logo = %q, want %q", got, want)
	}
}
