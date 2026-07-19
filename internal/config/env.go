package config

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"
)

// Env is the deliberately small KEY=VALUE format used by provider.env.
// It supports blank lines, comments, export prefixes, and single/double quoted values.
type Env map[string]string

var legacyProviderKeys = map[string]string{
	"HB_USER_ID":             "PROVIDER_USER_ID",
	"HB_AUTHENTICATOR":       "PROVIDER_AUTHENTICATOR",
	"HB_STBID":               "PROVIDER_STBID",
	"HB_STBINFO":             "PROVIDER_STBINFO",
	"HB_USER_TOKEN":          "PROVIDER_USER_TOKEN",
	"HB_STB_TYPE":            "PROVIDER_STB_TYPE",
	"HB_PRMID":               "PROVIDER_PRMID",
	"HB_DRM_SUPPLIER":        "PROVIDER_DRM_SUPPLIER",
	"HB_CITYCODE":            "PROVIDER_CITYCODE",
	"HB_NETWORKID":           "PROVIDER_NETWORKID",
	"HB_EASIP":               "PROVIDER_EASIP",
	"HB_PLATFORM_ORIGIN":     "PROVIDER_PLATFORM_ORIGIN",
	"HB_EPG_ENTRY":           "PROVIDER_EPG_ENTRY",
	"HB_EPG_ENTRY_FALLBACKS": "PROVIDER_EPG_ENTRY_FALLBACKS",
	"HB_TOKEN_SERVER":        "PROVIDER_TOKEN_SERVER",
	"HB_BIND_INTERFACE":      "PROVIDER_BIND_INTERFACE",
	"HB_BIND_SOURCE_IP":      "PROVIDER_BIND_SOURCE_IP",
	"HB_USER_AGENT":          "PROVIDER_USER_AGENT",
	"HB_TIMEOUT":             "PROVIDER_TIMEOUT",
}

func Load(path string) (Env, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := Env{}
	s := bufio.NewScanner(f)
	for lineNo := 1; s.Scan(); lineNo++ {
		line := strings.TrimSpace(strings.TrimPrefix(string(s.Bytes()), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			if value[0] == '"' {
				if unquoted, err := strconv.Unquote(value); err == nil {
					value = unquoted
				} else {
					return nil, fmt.Errorf("%s:%d: invalid quoted value: %w", path, lineNo, err)
				}
			} else {
				value = value[1 : len(value)-1]
			}
		}
		out[key] = value
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (e Env) Overlay(other Env) Env {
	out := make(Env, len(e)+len(other))
	maps.Copy(out, e)
	maps.Copy(out, other)
	return out
}

// NormalizeProviderKeys returns a copy using the provider-neutral key names.
// Legacy HB_* values remain accepted so existing installations can upgrade
// without rewriting their environment or captured-credential files first.
func (e Env) NormalizeProviderKeys() Env {
	out := make(Env, len(e)+len(legacyProviderKeys))
	maps.Copy(out, e)
	for legacy, canonical := range legacyProviderKeys {
		if _, exists := out[canonical]; exists {
			continue
		}
		if value, exists := out[legacy]; exists {
			out[canonical] = value
		}
	}
	return out
}

func (e Env) String(key, fallback string) string {
	if value, ok := e[key]; ok && value != "" {
		return value
	}
	return fallback
}

func (e Env) Bool(key string, fallback bool) bool {
	value, ok := e[key]
	if !ok || value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func (e Env) Int(key string, fallback int) int {
	value, ok := e[key]
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func (e Env) Float(key string, fallback float64) float64 {
	value, ok := e[key]
	if !ok {
		return fallback
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return n
}
