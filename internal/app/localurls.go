package app

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// localServices contains the router-facing addresses that can be discovered
// from OpenWrt at runtime. They are deliberately kept out of provider.env so
// LAN address and rtp2httpd port changes do not leave stale playlist URLs.
type localServices struct {
	LANHost       string
	R2HHost       string
	R2HPort       string
	R2HToken      string
	R2HConfigFile bool
}

func automaticValue(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || strings.EqualFold(value, "auto")
}

func disabledValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "none", "disabled":
		return true
	default:
		return false
	}
}

func commandOutput(ctx context.Context, name string, args ...string) string {
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func validHost(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "[]"))
	if before, _, ok := strings.Cut(value, "/"); ok {
		value = before
	}
	if value == "" || value == "*" || value == "0.0.0.0" || value == "::" || strings.EqualFold(value, "localhost") {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil && ip.IsLoopback() {
		return ""
	}
	return value
}

func discoverLANHost(ctx context.Context) string {
	if value := validHost(os.Getenv("IPTV_REFRESH_LAN_HOST")); value != "" {
		return value
	}

	var status struct {
		IPv4 []struct {
			Address string `json:"address"`
		} `json:"ipv4-address"`
	}
	if raw := commandOutput(ctx, "ubus", "call", "network.interface.lan", "status"); raw != "" {
		if json.Unmarshal([]byte(raw), &status) == nil {
			for _, address := range status.IPv4 {
				if value := validHost(address.Address); value != "" {
					return value
				}
			}
		}
	}
	if value := validHost(commandOutput(ctx, "uci", "-q", "get", "network.lan.ipaddr")); value != "" {
		return value
	}
	for _, name := range []string{"br-lan", "lan"} {
		iface, err := net.InterfaceByName(name)
		if err != nil {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return ""
}

func unquoteUCI(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
		if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
			return value[1 : end+1]
		}
	}
	if field, _, ok := strings.Cut(value, " "); ok {
		return strings.Trim(field, "'\"")
	}
	return strings.Trim(value, "'\"")
}

func parseListen(value string) (host, port string) {
	value = strings.TrimSpace(unquoteUCI(value))
	if n, err := strconv.Atoi(value); err == nil && n > 0 && n <= 65535 {
		return "", value
	}
	if parsedHost, parsedPort, err := net.SplitHostPort(value); err == nil {
		if n, err := strconv.Atoi(parsedPort); err == nil && n > 0 && n <= 65535 {
			return validHost(parsedHost), parsedPort
		}
	}
	if pos := strings.LastIndexByte(value, ':'); pos > 0 {
		parsedPort := value[pos+1:]
		if n, err := strconv.Atoi(parsedPort); err == nil && n > 0 && n <= 65535 {
			return validHost(value[:pos]), parsedPort
		}
	}
	return "", ""
}

func parseR2HUCI(raw string) localServices {
	sections := map[string]map[string]string{}
	order := []string{}
	for line := range strings.SplitSeq(raw, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !strings.HasPrefix(key, "rtp2httpd.") {
			continue
		}
		pos := strings.LastIndexByte(key, '.')
		if pos < len("rtp2httpd.") {
			continue
		}
		section, option := key[:pos], key[pos+1:]
		if sections[section] == nil {
			sections[section] = map[string]string{}
			order = append(order, section)
		}
		sections[section][option] = value
	}
	for _, section := range order {
		values := sections[section]
		if unquoteUCI(values["disabled"]) == "1" {
			continue
		}
		configFile := unquoteUCI(values["use_config_file"]) == "1"
		host, port := parseListen(values["listen"])
		if port == "" {
			host, port = parseListen(values["port"])
		}
		if port == "" && !configFile {
			// rtp2httpd uses 5140 when an enabled UCI instance omits both
			// the current listen list and the legacy port option.
			port = "5140"
		}
		return localServices{
			R2HHost:       host,
			R2HPort:       port,
			R2HToken:      unquoteUCI(values["r2h_token"]),
			R2HConfigFile: configFile,
		}
	}
	return localServices{}
}

func parseR2HConfig(raw string) localServices {
	var out localServices
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, hasEquals := strings.Cut(line, "=")
		if hasEquals {
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			switch strings.ToLower(key) {
			case "listen":
				if out.R2HPort == "" {
					out.R2HHost, out.R2HPort = parseListen(value)
				}
			case "r2h-token":
				out.R2HToken = strings.Trim(value, "'\"")
			}
			continue
		}
		// Older rtp2httpd configuration files use "* 7088" instead of
		// the current "listen = 7088" form.
		fields := strings.Fields(line)
		if len(fields) == 2 && out.R2HPort == "" {
			out.R2HHost, out.R2HPort = parseListen(fields[0] + ":" + fields[1])
		}
	}
	return out
}

func discoverLocalServices(ctx context.Context) localServices {
	discoveryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	out := parseR2HUCI(commandOutput(discoveryCtx, "uci", "-q", "show", "rtp2httpd"))
	if raw, err := os.ReadFile("/etc/rtp2httpd.conf"); err == nil {
		fromFile := parseR2HConfig(string(raw))
		if out.R2HPort == "" || out.R2HConfigFile {
			out.R2HHost, out.R2HPort = fromFile.R2HHost, fromFile.R2HPort
		}
		if out.R2HToken == "" || out.R2HConfigFile {
			out.R2HToken = fromFile.R2HToken
		}
	}
	out.LANHost = discoverLANHost(discoveryCtx)
	return out
}

func urlHost(host, port string) string {
	host = validHost(host)
	if host == "" {
		return ""
	}
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + strings.Trim(host, "[]") + "]"
	}
	return host
}

func originFromURL(value string) string {
	if automaticValue(value) || disabledValue(value) {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return ""
	}
	return u.Scheme + "://" + urlHost(u.Hostname(), "")
}

func publicFileURL(origin, path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(clean, "/www/") || origin == "" {
		return ""
	}
	return strings.TrimRight(origin, "/") + strings.TrimPrefix(clean, "/www")
}

func resolveLocalURLs(settings Settings, discovered localServices) Settings {
	lanHost := discovered.LANHost
	if lanHost == "" {
		lanHost = discovered.R2HHost
	}
	origin := ""
	if lanHost != "" {
		origin = "http://" + urlHost(lanHost, "")
	}
	if origin == "" {
		for _, candidate := range []string{settings.R2HBaseURL, settings.XTvgURL, settings.LocalLogoURLBase} {
			if origin = originFromURL(candidate); origin != "" {
				break
			}
		}
	}

	if disabledValue(settings.R2HBaseURL) {
		settings.R2HBaseURL = ""
	} else if automaticValue(settings.R2HBaseURL) {
		host := lanHost
		if host == "" {
			host = discovered.R2HHost
		}
		if host != "" && discovered.R2HPort != "" {
			settings.R2HBaseURL = "http://" + urlHost(host, discovered.R2HPort)
		} else {
			settings.R2HBaseURL = ""
		}
	}
	if settings.R2HToken == "" && discovered.R2HToken != "" {
		settings.R2HToken = discovered.R2HToken
	}

	if disabledValue(settings.R2HCatchupHost) {
		settings.R2HCatchupHost = ""
	} else if automaticValue(settings.R2HCatchupHost) {
		settings.R2HCatchupHost = ""
		if u, err := url.Parse(settings.R2HBaseURL); err == nil {
			settings.R2HCatchupHost = u.Host
		}
	}
	if disabledValue(settings.XTvgURL) {
		settings.XTvgURL = ""
	} else if automaticValue(settings.XTvgURL) {
		settings.XTvgURL = publicFileURL(origin, settings.EPGPublicFile)
	}
	if disabledValue(settings.LocalLogoURLBase) {
		settings.LocalLogoURLBase = ""
	} else if automaticValue(settings.LocalLogoURLBase) {
		settings.LocalLogoURLBase = publicFileURL(origin, settings.LocalLogoDir)
	}
	return settings
}
