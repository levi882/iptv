package authrecover

import (
	"context"
	"crypto/des"
	"encoding/hex"
	"testing"
)

func TestRecoverNumericReturnsParityEquivalentKey(t *testing.T) {
	sample := Sample{
		EncryptToken: "@@202607181306590256379160000000",
		UserID:       "test-user",
		STBID:        "ABCDEF0123456789",
		IP:           "192.0.2.2",
		MAC:          "00:11:22:33:44:55",
	}
	plain := "7654321$" + sample.EncryptToken + "$" + sample.UserID + "$" + sample.STBID + "$" + sample.IP + "$" + sample.MAC + "$$CTC"
	sample.Authenticator = encryptForTest(t, "135", []byte(plain))

	result, err := RecoverNumeric(context.Background(), sample, 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.EquivalentKey != "024" || result.KeyPattern != "[01][23][45]" || result.Random != "7654321" || result.Reserved != "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRecoverNumericRejectsIncompleteCiphertext(t *testing.T) {
	sample := Sample{Authenticator: "0011", EncryptToken: "t", UserID: "u"}
	if _, err := RecoverNumeric(context.Background(), sample, 7); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRecoverNumericAllowsUnknownDeviceFields(t *testing.T) {
	sample := Sample{EncryptToken: "token", UserID: "user"}
	plain := "1$token$user$stb$192.0.2.3$AA:BB:CC:DD:EE:FF$Reserved$CTC"
	sample.Authenticator = encryptForTest(t, "1", []byte(plain))
	result, err := RecoverNumeric(context.Background(), sample, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.STBID != "stb" || result.IP != "192.0.2.3" || result.MAC != "AA:BB:CC:DD:EE:FF" || result.Reserved != "Reserved" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func encryptForTest(t *testing.T, password string, plain []byte) string {
	t.Helper()
	key := []byte(password)
	for len(key) < 24 {
		key = append(key, '0')
	}
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padding := block.BlockSize() - len(plain)%block.BlockSize()
	for range padding {
		plain = append(plain, byte(padding))
	}
	ciphertext := make([]byte, len(plain))
	for offset := 0; offset < len(plain); offset += block.BlockSize() {
		block.Encrypt(ciphertext[offset:offset+block.BlockSize()], plain[offset:offset+block.BlockSize()])
	}
	return hex.EncodeToString(ciphertext)
}
