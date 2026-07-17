package config

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"
)

// Env is the deliberately small KEY=VALUE format used by the existing hb.env.
// It supports blank lines, comments, export prefixes, and single/double quoted values.
type Env map[string]string

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
