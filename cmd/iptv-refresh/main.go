package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"iptv/internal/app"
	"iptv/internal/capture"
	"iptv/internal/config"
	"iptv/internal/loglimit"
	"iptv/internal/server"
)

const defaultServicePort = 9100

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return errors.New("a command is required")
	}
	switch args[0] {
	case "refresh":
		return refreshCommand(args[1:])
	case "capture":
		return captureCommand(args[1:])
	case "serve":
		return serveCommand(args[1:])
	case "control":
		return controlCommand(args[1:])
	case "version", "--version", "-version":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: iptv-refresh <refresh|capture|serve|control|version> [options]")
}

func defaultRepo() string {
	if cwd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
	}
	return "/mnt/sda1/iptv"
}

func commonFlags(name string) (*flag.FlagSet, *string, *string, *string, *string) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	repo := set.String("repo-root", defaultRepo(), "repository/data root")
	env := set.String("env-file", "", "fixed hb.env path")
	creds := set.String("creds-file", "", "dynamic credentials path")
	iface := set.String("iface", "", "IPTV capture interface")
	return set, repo, env, creds, iface
}

func loadWithOverrides(repo, envPath, credsPath, iface string) (app.Settings, error) {
	settings, _, err := app.LoadSettings(repo, envPath)
	if err != nil {
		return app.Settings{}, err
	}
	if credsPath != "" {
		settings.CredsFile = credsPath
	}
	if iface != "" {
		settings.Interface = iface
		if !settings.BindInterfaceExplicit {
			settings.BindInterface = iface
		}
	}
	return settings, nil
}

func applyProviderInterfaceOverride(settings *app.Settings, value string) {
	if settings.BindInterfaceExplicit {
		return
	}
	value = strings.TrimSpace(value)
	switch {
	case value == "" || strings.EqualFold(value, "auto"):
		settings.BindInterface = settings.Interface
		settings.BindInterfaceExplicit = false
	case strings.EqualFold(value, "none") || strings.EqualFold(value, "off"):
		settings.BindInterface = ""
		settings.BindInterfaceExplicit = true
	default:
		settings.BindInterface = value
		settings.BindInterfaceExplicit = true
	}
}

func refreshCommand(args []string) error {
	set, repo, envPath, credsPath, iface := commonFlags("refresh")
	skipCapture := set.Bool("skip-capture", false, "reuse existing credentials")
	if err := set.Parse(args); err != nil {
		return err
	}
	settings, err := loadWithOverrides(*repo, *envPath, *credsPath, *iface)
	if err != nil {
		return err
	}
	settings.SkipCapture = *skipCapture
	report, err := (app.Runner{}).Run(context.Background(), settings)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func captureCommand(args []string) error {
	set, repo, envPath, credsPath, iface := commonFlags("capture")
	timeout := set.Int("timeout", 0, "capture timeout in seconds")
	dumpPath := set.String("dump-path", "", "optional raw capture path")
	if err := set.Parse(args); err != nil {
		return err
	}
	settings, err := loadWithOverrides(*repo, *envPath, *credsPath, *iface)
	if err != nil {
		return err
	}
	fallback, _ := config.Load(settings.CredsFile)
	if *timeout > 0 {
		settings.CaptureTimeout = time.Duration(*timeout) * time.Second
	}
	if *dumpPath != "" {
		settings.CaptureDump = *dumpPath
	}
	captured, err := capture.Run(context.Background(), capture.Options{Interface: settings.Interface, Timeout: settings.CaptureTimeout, OutputPath: settings.CredsFile, DumpPath: settings.CaptureDump, TokenHost: settings.TokenHost, Fallback: fallback})
	if err != nil {
		return err
	}
	stbType := captured["HB_STB_TYPE"]
	if stbType == "" {
		stbType = "unknown"
	}
	fmt.Printf("captured complete STB portal login on %s (UserToken=yes, STBType=%s)\n", settings.Interface, stbType)
	return nil
}

type stringList []string

func (s *stringList) String() string         { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error { *s = append(*s, value); return nil }

func serveCommand(args []string) error {
	set, repo, envPath, credsPath, iface := commonFlags("serve")
	providerIface := set.String("provider-iface", "auto", "provider HTTP interface: auto, none, or a device name")
	logMaxSize := set.String("log-max-size", loglimit.DefaultSize, "maximum LuCI application log size, for example 1M")
	haWebhookTimeout := set.Int("ha-webhook-timeout", 10, "HA STB power-on webhook timeout in seconds")
	host := set.String("host", "127.0.0.1", "listen host")
	port := set.Int("port", defaultServicePort, "listen port")
	token := set.String("token", "", "required API token")
	tokenFile := set.String("token-file", "", "read API token from a file")
	var allowed stringList
	set.Var(&allowed, "allow-ip", "allowed client IP (repeatable)")
	if err := set.Parse(args); err != nil {
		return err
	}
	if *tokenFile != "" {
		raw, err := os.ReadFile(*tokenFile)
		if err != nil {
			return fmt.Errorf("read token file: %w", err)
		}
		*token = strings.TrimSpace(string(raw))
	}
	if *token == "" || *token == "change-me" {
		return errors.New("set a non-default --token")
	}
	settings, err := loadWithOverrides(*repo, *envPath, *credsPath, *iface)
	if err != nil {
		return err
	}
	applyProviderInterfaceOverride(&settings, *providerIface)
	settings.STBPowerWebhookURL = strings.TrimSpace(os.Getenv("IPTV_REFRESH_HA_WEBHOOK_URL"))
	if *haWebhookTimeout <= 0 || *haWebhookTimeout > 60 {
		return fmt.Errorf("ha-webhook-timeout must be between 1 and 60 seconds")
	}
	settings.STBPowerWebhookTimeout = time.Duration(*haWebhookTimeout) * time.Second
	logMaxBytes, err := loglimit.ParseSize(*logMaxSize)
	if err != nil {
		return fmt.Errorf("invalid log-max-size %q: %w", *logMaxSize, err)
	}
	if len(allowed) == 0 {
		allowed = []string{"127.0.0.1", "::1"}
	}
	allowedMap := map[string]bool{}
	for _, value := range allowed {
		allowedMap[value] = true
	}
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	logPath := filepath.Join(settings.RepoRoot, "output", "log", "iptv_refresh.log")
	if fileLog, err := loglimit.New(logPath, logMaxBytes); err != nil {
		logger.Printf("WARNING: application log unavailable: %v", err)
	} else {
		logger.SetOutput(io.MultiWriter(os.Stdout, fileLog))
	}
	manager := server.NewManager(app.Runner{Logger: logger}, settings)
	address := *host + ":" + strconv.Itoa(*port)
	httpServer := &http.Server{
		Addr:              address,
		Handler:           server.Handler(server.Config{Token: *token, AllowedIPs: allowedMap, DefaultIface: settings.Interface, PlaylistPath: settings.OutputPath, Manager: manager, Logger: logger}),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	logger.Printf("listening on http://%s", address)
	err = httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
