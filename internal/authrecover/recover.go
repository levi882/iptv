package authrecover

import (
	"context"
	"crypto/des"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const canonicalDigits = "02468"

type Sample struct {
	Authenticator string
	EncryptToken  string
	UserID        string
	STBID         string
	IP            string
	MAC           string
}

type Result struct {
	EquivalentKey string
	KeyPattern    string
	Random        string
	Reserved      string
	STBID         string
	IP            string
	MAC           string
}

func (s Sample) Validate() error {
	if s.Authenticator == "" || s.EncryptToken == "" || s.UserID == "" {
		return errors.New("authenticator, token and user are required")
	}
	data, err := hex.DecodeString(strings.TrimSpace(s.Authenticator))
	if err != nil {
		return fmt.Errorf("decode authenticator: %w", err)
	}
	if len(data) == 0 || len(data)%des.BlockSize != 0 {
		return fmt.Errorf("authenticator decodes to %d bytes; expected a non-zero multiple of %d", len(data), des.BlockSize)
	}
	return nil
}

// RecoverNumeric searches one representative from each DES parity-equivalent
// class. ASCII decimal digit pairs 0/1, 2/3, 4/5, 6/7 and 8/9 differ only in
// the parity bit ignored by DES, so a seven-digit search needs only 5^7 keys.
func RecoverNumeric(ctx context.Context, sample Sample, digits int) (Result, error) {
	if err := sample.Validate(); err != nil {
		return Result{}, err
	}
	if digits < 1 || digits > 8 {
		return Result{}, errors.New("numeric password length must be between 1 and 8")
	}
	ciphertext, _ := hex.DecodeString(strings.TrimSpace(sample.Authenticator))
	limit := pow(len(canonicalDigits), digits)
	for index := 0; index < limit; index++ {
		if index&1023 == 0 {
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			default:
			}
		}
		candidate := canonicalCandidate(index, digits)
		plain, ok := decryptJavaDESede(candidate, ciphertext)
		if !ok {
			continue
		}
		parts := strings.Split(string(plain), "$")
		if !matches(sample, parts) {
			continue
		}
		return Result{
			EquivalentKey: candidate,
			KeyPattern:    equivalentPattern(candidate),
			Random:        parts[0],
			Reserved:      parts[6],
			STBID:         parts[3],
			IP:            parts[4],
			MAC:           parts[5],
		}, nil
	}
	return Result{}, errors.New("no matching numeric key found")
}

func decryptJavaDESede(password string, ciphertext []byte) ([]byte, bool) {
	key := []byte(password)
	if len(key) > 24 {
		key = key[:24]
	}
	for len(key) < 24 {
		key = append(key, '0')
	}
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, false
	}
	plain := make([]byte, len(ciphertext))
	for offset := 0; offset < len(ciphertext); offset += block.BlockSize() {
		block.Decrypt(plain[offset:offset+block.BlockSize()], ciphertext[offset:offset+block.BlockSize()])
	}
	return unpadPKCS5(plain)
}

func unpadPKCS5(data []byte) ([]byte, bool) {
	if len(data) == 0 {
		return nil, false
	}
	padding := int(data[len(data)-1])
	if padding < 1 || padding > des.BlockSize || padding > len(data) {
		return nil, false
	}
	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, false
		}
	}
	return data[:len(data)-padding], true
}

func matches(sample Sample, parts []string) bool {
	if len(parts) != 8 || parts[7] != "CTC" {
		return false
	}
	if len(parts[0]) < 1 || len(parts[0]) > 7 || !allDecimal(parts[0]) {
		return false
	}
	if parts[1] != sample.EncryptToken || parts[2] != sample.UserID {
		return false
	}
	if sample.STBID != "" && !strings.EqualFold(parts[3], sample.STBID) {
		return false
	}
	if sample.IP != "" && parts[4] != sample.IP {
		return false
	}
	if sample.MAC != "" && !strings.EqualFold(parts[5], sample.MAC) {
		return false
	}
	return parts[6] == "" || parts[6] == "Reserved"
}

func allDecimal(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func canonicalCandidate(index, digits int) string {
	result := make([]byte, digits)
	for position := digits - 1; position >= 0; position-- {
		result[position] = canonicalDigits[index%len(canonicalDigits)]
		index /= len(canonicalDigits)
	}
	return string(result)
}

func equivalentPattern(candidate string) string {
	var result strings.Builder
	for _, char := range candidate {
		result.WriteByte('[')
		result.WriteRune(char)
		result.WriteRune(char + 1)
		result.WriteByte(']')
	}
	return result.String()
}

func pow(base, exponent int) int {
	result := 1
	for range exponent {
		result *= base
	}
	return result
}
