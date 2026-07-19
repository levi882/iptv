package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"iptv/internal/config"
)

type Settings struct {
	RepoRoot  string
	EnvFile   string
	CredsFile string
	Interface string

	CaptureTimeout         time.Duration
	RefreshTimeout         time.Duration
	CaptureDump            string
	TokenHost              string
	SkipCapture            bool
	STBPowerWebhookURL     string
	STBPowerWebhookTimeout time.Duration

	OutputPath         string
	SnapshotOutputPath string
	SnapshotPath       string
	OutputFormat       string
	Mode               string
	SortBy             string
	OrderReference     string
	KeepUnmatched      bool

	EPGURL           string
	EPGFile          string
	EPGPublicFile    string
	EPGCompareSource string
	EPGReplaceName   bool
	XTvgURL          string

	TokenServer           string
	PlatformOrigin        string
	EPGEntry              string
	EPGFallbacks          []string
	EASIP                 string
	NetworkID             string
	CityCode              string
	STBType               string
	PRMID                 string
	DRMSupplier           string
	BindInterface         string
	BindInterfaceExplicit bool
	BindSourceIP          string
	UserAgent             string
	ProviderTimeout       time.Duration

	IGMPHTTPPrefix    string
	R2HBaseURL        string
	R2HToken          string
	R2HIGMPPath       string
	R2HAddFCC         bool
	R2HFCCTYPE        string
	R2HProxyRTSP      bool
	R2HCatchupHost    string
	CatchupType       string
	CatchupPlayseek   string
	CatchupSeekOffset string

	LineTagRule string
	LineTagUHD  string
	LineTagHD   string
	LineTagSD   string

	LogoMatchSource    string
	LogoURLBase        string
	LogoOverridesFile  string
	LogoMatchThreshold float64
	LocalLogoCache     bool
	LocalLogoDir       string
	LocalLogoURLBase   string
	LocalLogoTimeout   time.Duration
	GroupBy51ZMT       bool
	DisplayNameMode    string
	UseCache           bool
}

func LoadSettings(repoRoot, envPath string) (Settings, config.Env, error) {
	if repoRoot == "" {
		return Settings{}, nil, fmt.Errorf("repo root is required")
	}
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Settings{}, nil, err
	}
	if envPath == "" {
		envPath = "/etc/iptv-refresh/provider.env"
	}
	env := config.Env{}
	if loaded, err := config.Load(envPath); err == nil {
		env = loaded.NormalizeProviderKeys()
	} else {
		return Settings{}, nil, fmt.Errorf("load config %s: %w", envPath, err)
	}
	keepUnmatched := env.String("KEEP_UNMATCHED", "append")
	if keepUnmatched != "append" && keepUnmatched != "drop" {
		return Settings{}, nil, fmt.Errorf("KEEP_UNMATCHED must be append or drop")
	}
	captureInterface := env.String("IFACE", "any")
	bindInterface := strings.TrimSpace(env["PROVIDER_BIND_INTERFACE"])
	bindInterfaceExplicit := bindInterface != "" && !strings.EqualFold(bindInterface, "auto")
	switch {
	case bindInterface == "" || strings.EqualFold(bindInterface, "auto"):
		bindInterface = captureInterface
	case strings.EqualFold(bindInterface, "none") || strings.EqualFold(bindInterface, "off"):
		bindInterface = ""
	}

	s := Settings{
		RepoRoot: repoRoot, EnvFile: envPath,
		CredsFile:          env.String("CREDS_FILE", "/etc/iptv-refresh/provider.creds.env"),
		Interface:          captureInterface,
		CaptureTimeout:     time.Duration(env.Int("CAPTURE_TIMEOUT", 180)) * time.Second,
		RefreshTimeout:     time.Duration(env.Int("REFRESH_TIMEOUT", 300)) * time.Second,
		CaptureDump:        env["DUMP_PATH"],
		OutputPath:         env.String("OUTPUT_PATH", filepath.Join(repoRoot, "config", "local", "local_stb.m3u")),
		SnapshotOutputPath: env["R2H_SNAPSHOT_OUTPUT_PATH"], SnapshotPath: filepath.Join(repoRoot, "frameset_builder_latest.jsp"),
		OutputFormat: env.String("OUTPUT_FORMAT", "auto"), Mode: env.String("MODE", "auto"),
		SortBy: env.String("SORT_BY", "user_channel_id"), OrderReference: env["ORDER_REF"], KeepUnmatched: keepUnmatched == "append",
		EPGURL: env["EPG_URL"], EPGFile: env.String("EPG_FILE", filepath.Join(repoRoot, "cache", "e1.xml.gz")),
		EPGPublicFile: env.String("EPG_PUBLIC_FILE", "/www/iptv_epg/e1.xml.gz"), EPGCompareSource: env["EPG_COMPARE_SOURCE"],
		EPGReplaceName: env.Bool("EPG_REPLACE_NAME", false), XTvgURL: env["X_TVG_URL"],
		TokenServer: env.String("PROVIDER_TOKEN_SERVER", "auto"), PlatformOrigin: env.String("PROVIDER_PLATFORM_ORIGIN", "auto"),
		EPGEntry: env.String("PROVIDER_EPG_ENTRY", "auto"), EPGFallbacks: splitList(env["PROVIDER_EPG_ENTRY_FALLBACKS"]),
		EASIP: env.String("PROVIDER_EASIP", "auto"), NetworkID: env.String("PROVIDER_NETWORKID", "auto"), CityCode: env["PROVIDER_CITYCODE"],
		STBType: env["PROVIDER_STB_TYPE"], PRMID: env["PROVIDER_PRMID"], DRMSupplier: env["PROVIDER_DRM_SUPPLIER"],
		BindInterface: bindInterface, BindInterfaceExplicit: bindInterfaceExplicit, BindSourceIP: env["PROVIDER_BIND_SOURCE_IP"],
		UserAgent: env["PROVIDER_USER_AGENT"], ProviderTimeout: time.Duration(env.Int("PROVIDER_TIMEOUT", 20)) * time.Second,
		IGMPHTTPPrefix: env["IGMP_HTTP_PREFIX"], R2HBaseURL: env["R2H_BASE_URL"], R2HToken: env["R2H_TOKEN"],
		R2HIGMPPath: env.String("R2H_IGMP_PATH", "udp"), R2HAddFCC: env.Bool("R2H_ADD_FCC", false), R2HFCCTYPE: env.String("R2H_FCC_TYPE", "telecom"),
		R2HProxyRTSP: env.Bool("R2H_PROXY_RTSP", false), R2HCatchupHost: env["R2H_CATCHUP_HOST"], CatchupType: env.String("CATCHUP_TYPE", "shift"),
		CatchupPlayseek: env.String("CATCHUP_PLAYSEEK_TEMPLATE", "{(b)YmdHMS}-{(e)YmdHMS}"), CatchupSeekOffset: env["CATCHUP_SEEK_OFFSET"],
		LineTagRule: env.String("LINE_TAG_RULE", "none"), LineTagUHD: env.String("LINE_TAG_UHD", "超高清"), LineTagHD: env.String("LINE_TAG_HD", "高清"), LineTagSD: env.String("LINE_TAG_SD", "标清"),
		LogoMatchSource: env["LOGO_MATCH_SOURCE"], LogoURLBase: env["LOGO_URL_BASE"], LogoOverridesFile: env["LOGO_OVERRIDES_FILE"],
		LogoMatchThreshold: env.Float("LOGO_MATCH_THRESHOLD", .75), LocalLogoCache: env.Bool("LOCAL_LOGO_CACHE", false),
		LocalLogoDir: env.String("LOCAL_LOGO_DIR", "/www/iptv_logo"), LocalLogoURLBase: env["LOCAL_LOGO_URL_BASE"], LocalLogoTimeout: time.Duration(env.Int("LOCAL_LOGO_TIMEOUT", 20)) * time.Second,
		GroupBy51ZMT: env.Bool("GROUP_BY_51ZMT", false), DisplayNameMode: env.String("DISPLAY_NAME_MODE", "name"), UseCache: env.Bool("USE_CACHE", false),
	}
	s.TokenHost = tokenHost(s.TokenServer)
	if err := s.Validate(); err != nil {
		return Settings{}, nil, err
	}
	return s, env, nil
}

func splitList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' })
}

func (s Settings) Validate() error {
	if s.CaptureTimeout <= 0 {
		return fmt.Errorf("CAPTURE_TIMEOUT must be greater than zero")
	}
	if s.RefreshTimeout <= 0 {
		return fmt.Errorf("REFRESH_TIMEOUT must be greater than zero")
	}
	if s.ProviderTimeout <= 0 {
		return fmt.Errorf("PROVIDER_TIMEOUT must be greater than zero")
	}
	if s.Mode != "auto" && s.Mode != "rtsp" && s.Mode != "igmp" {
		return fmt.Errorf("MODE must be auto, rtsp, or igmp")
	}
	if s.OutputFormat != "auto" && s.OutputFormat != "m3u" && s.OutputFormat != "txt" {
		return fmt.Errorf("OUTPUT_FORMAT must be auto, m3u, or txt")
	}
	if s.SortBy != "input" && s.SortBy != "user_channel_id" {
		return fmt.Errorf("SORT_BY must be input or user_channel_id")
	}
	if s.R2HIGMPPath != "udp" && s.R2HIGMPPath != "rtp" {
		return fmt.Errorf("R2H_IGMP_PATH must be udp or rtp")
	}
	if s.R2HFCCTYPE != "telecom" && s.R2HFCCTYPE != "huawei" {
		return fmt.Errorf("R2H_FCC_TYPE must be telecom or huawei")
	}
	if s.LineTagRule != "none" && s.LineTagRule != "hd_sd" {
		return fmt.Errorf("LINE_TAG_RULE must be none or hd_sd")
	}
	if s.DisplayNameMode != "name" && s.DisplayNameMode != "tvg_name" {
		return fmt.Errorf("DISPLAY_NAME_MODE must be name or tvg_name")
	}
	return nil
}

func resolveValue(configured, captured, fallback string) string {
	if configured == "" || configured == "auto" {
		if captured != "" {
			return captured
		}
		return fallback
	}
	return configured
}
