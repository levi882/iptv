package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"iptv/internal/app"
	"iptv/internal/runlock"
)

type Status struct {
	Running    bool        `json:"running"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt time.Time   `json:"finished_at"`
	LastError  string      `json:"last_error,omitempty"`
	Report     *app.Report `json:"report,omitempty"`
}

type Manager struct {
	mu       sync.RWMutex
	status   Status
	run      func(context.Context, app.Settings) (app.Report, error)
	settings app.Settings
}

func NewManager(runner app.Runner, settings app.Settings) *Manager {
	return &Manager{run: runner.Run, settings: settings}
}

func (m *Manager) Trigger(interfaceName string) error {
	return m.TriggerWithOptions(TriggerOptions{Interface: interfaceName})
}

type TriggerOptions struct {
	Interface string
	Capture   bool
}

func (m *Manager) TriggerWithOptions(options TriggerOptions) error {
	m.mu.Lock()
	if m.status.Running {
		m.mu.Unlock()
		return runlock.ErrAlreadyRunning
	}
	settings := m.settings
	settings.SkipCapture = !options.Capture
	if options.Interface != "" {
		settings.Interface = options.Interface
		if !settings.BindInterfaceExplicit {
			settings.BindInterface = options.Interface
		}
	}
	status := m.status
	status.Running = true
	status.StartedAt = time.Now()
	status.FinishedAt = time.Time{}
	status.LastError = ""
	m.status = status
	m.mu.Unlock()
	go func() {
		report, err := m.run(context.Background(), settings)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.status.Running = false
		m.status.FinishedAt = time.Now()
		if err != nil {
			m.status.LastError = err.Error()
			return
		}
		m.status.Report = &report
	}()
	return nil
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	if status.Report != nil {
		copy := *status.Report
		status.Report = &copy
	}
	return status
}

type Config struct {
	Address      string
	Token        string
	AllowedIPs   map[string]bool
	DefaultIface string
	PlaylistPath string
	Manager      *Manager
	Logger       *log.Logger
}

func Handler(config Config) http.Handler {
	mux := http.NewServeMux()
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	jsonResponse := func(w http.ResponseWriter, status int, value any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	allowed := func(r *http.Request) bool {
		if len(config.AllowedIPs) == 0 {
			return true
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		return config.AllowedIPs[host]
	}
	authorized := func(r *http.Request) bool {
		provided := r.URL.Query().Get("token")
		if header := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(header), "bearer ") {
			provided = strings.TrimSpace(header[7:])
		}
		return config.Token != "" && len(provided) == len(config.Token) && subtle.ConstantTimeCompare([]byte(provided), []byte(config.Token)) == 1
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonResponse(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "msg": "method not allowed"})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "msg": "healthy"})
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !allowed(r) {
			jsonResponse(w, http.StatusForbidden, map[string]any{"ok": false, "msg": "forbidden"})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "status": config.Manager.Status()})
	})
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			jsonResponse(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "msg": "method not allowed"})
			return
		}
		if !allowed(r) {
			jsonResponse(w, http.StatusForbidden, map[string]any{"ok": false, "msg": "ip forbidden"})
			return
		}
		if !authorized(r) {
			jsonResponse(w, http.StatusForbidden, map[string]any{"ok": false, "msg": "bad token"})
			return
		}
		iface := r.URL.Query().Get("iface")
		if iface == "" {
			iface = config.DefaultIface
		}
		capture := false
		if value := r.URL.Query().Get("capture"); value != "" {
			var err error
			capture, err = strconv.ParseBool(value)
			if err != nil {
				jsonResponse(w, http.StatusBadRequest, map[string]any{"ok": false, "msg": "capture must be true, false, 1, or 0"})
				return
			}
		}
		if err := config.Manager.TriggerWithOptions(TriggerOptions{Interface: iface, Capture: capture}); err != nil {
			if errors.Is(err, runlock.ErrAlreadyRunning) {
				jsonResponse(w, http.StatusConflict, map[string]any{"ok": false, "msg": "already running"})
				return
			}
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"ok": false, "msg": err.Error()})
			return
		}
		mode := "saved credentials"
		if capture {
			mode = "credential capture"
		}
		logger.Printf("refresh started from %s on interface %s using %s", r.RemoteAddr, iface, mode)
		jsonResponse(w, http.StatusAccepted, map[string]any{"ok": true, "msg": "started", "capture": capture})
	})
	mux.HandleFunc("/playlist", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !allowed(r) || !authorized(r) {
			jsonResponse(w, http.StatusForbidden, map[string]any{"ok": false, "msg": "forbidden"})
			return
		}
		if config.PlaylistPath == "" {
			jsonResponse(w, http.StatusNotFound, map[string]any{"ok": false, "msg": "playlist not configured"})
			return
		}
		info, err := os.Stat(config.PlaylistPath)
		if err != nil || info.IsDir() {
			jsonResponse(w, http.StatusNotFound, map[string]any{"ok": false, "msg": "playlist not found"})
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl; charset=utf-8")
		w.Header().Set("Content-Disposition", `inline; filename="playlist.m3u"`)
		http.ServeFile(w, r, config.PlaylistPath)
	})
	return mux
}
