package capture

import (
	"path/filepath"
	"strings"
	"testing"

	"iptv/internal/config"
)

func TestParsePreservesCompleteOpaqueUserToken(t *testing.T) {
	tests := []struct{ name, raw, token string }{
		{"raw address suffix", "UserToken=17000000000000001@192.0.2.44&UserID=demo", "17000000000000001@192.0.2.44"},
		{"form escaped address suffix", "UserToken=17000000000000001%40192.0.2.44&UserID=demo", "17000000000000001@192.0.2.44"},
		{"cookie delimiter", "Cookie: UserToken=17000000000000001@192.0.2.44; Path=/", "17000000000000001@192.0.2.44"},
		{"javascript token", "CTCSetConfig('UserToken','a+b@192.0.2.44');", "a+b@192.0.2.44"},
		{"literal plus", "UserToken=a+b%40192.0.2.44&UserID=demo", "a+b@192.0.2.44"},
		{"escaped opaque punctuation", "UserToken=a%2Bb%2Fc%3D%40192.0.2.44&UserID=demo", "a+b/c=@192.0.2.44"},
		{"encoded controls", "UserToken=token%0D%0A%00%40192.0.2.44&UserID=demo", "token@192.0.2.44"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := Parse([]byte(test.raw), config.Env{}, "")
			if values["PROVIDER_USER_TOKEN"] != test.token {
				t.Fatalf("token was changed: got %q, want %q", values["PROVIDER_USER_TOKEN"], test.token)
			}
			path := filepath.Join(t.TempDir(), "credentials.env")
			if err := Save(path, values); err != nil {
				t.Fatal(err)
			}
			loaded, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if loaded["PROVIDER_USER_TOKEN"] != test.token {
				t.Fatal("full token did not survive credential persistence")
			}
			if strings.ContainsAny(loaded["PROVIDER_USER_TOKEN"], "\r\n\x00") {
				t.Fatal("credential contains control characters")
			}
		})
	}
}
