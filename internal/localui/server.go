package localui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Ehsanamn1/alpha/internal/config"
	"github.com/Ehsanamn1/alpha/internal/orchestrator"
	"github.com/Ehsanamn1/alpha/internal/subscriptions"
)

//go:embed web/*
var webFiles embed.FS

const Version = "0.4.3-alpha"

type Server struct {
	cfg         config.Config
	manager     *orchestrator.Manager
	token       string
	uiAddress   string
	started     time.Time
	catalog     *subscriptions.Store
	connMu      sync.Mutex
	runCtx      context.Context
	connActive  bool
	connStatus  string
	connError   string
	connPID     int
	connGen     uint64
	connCmd     *exec.Cmd
	connPath    string
	configPath  string
	activeSince string
}

type engineView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Priority  int    `json:"priority"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	LastCheck string `json:"last_check,omitempty"`
	Failures  int    `json:"failures"`
}

type stateResponse struct {
	Product       string       `json:"product"`
	Version       string       `json:"version"`
	Status        string       `json:"status"`
	Listen        string       `json:"listen"`
	UIAddress     string       `json:"ui_address"`
	StartedAt     string       `json:"started_at"`
	CheckedAt     string       `json:"checked_at"`
	Healthy       int          `json:"healthy"`
	Total         int          `json:"total"`
	Engines       []engineView `json:"engines"`
	ProbeTargets  []string     `json:"probe_targets"`
	SafetyMessage string       `json:"safety_message"`
}

func New(cfg config.Config, manager *orchestrator.Manager, uiAddress string) (*Server, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("create local UI session token: %w", err)
	}
	if uiAddress == "" {
		uiAddress = "127.0.0.1:18888"
	}
	catalog, err := subscriptions.NewStore()
	if err != nil {
		return nil, fmt.Errorf("initialize local subscription catalog: %w", err)
	}
	return &Server{cfg: cfg, manager: manager, token: hex.EncodeToString(raw[:]), uiAddress: uiAddress, started: time.Now(), catalog: catalog, connStatus: "disconnected"}, nil
}

// Handler returns an HTTP handler intended to bind only to loopback.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	staticFS, _ := fs.Sub(webFiles, "web")
	files := http.FileServer(http.FS(staticFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		b, err := webFiles.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "Alpha UI asset unavailable", http.StatusInternalServerError)
			return
		}
		page := strings.ReplaceAll(string(b), "__ALPHA_SESSION__", s.token)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
	})
	mux.Handle("/assets/", http.StripPrefix("/assets/", files))
	mux.HandleFunc("/manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		b, err := webFiles.ReadFile("web/manifest.webmanifest")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		b, err := webFiles.ReadFile("web/sw.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Service-Worker-Allowed", "/")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		s.writeJSON(w, http.StatusOK, s.state())
	})
	mux.HandleFunc("/api/probe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if !s.authorized(w, r) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.cfg.ProbeTimeoutSeconds+2)*time.Second)
		defer cancel()
		s.manager.CheckNow(ctx)
		s.writeJSON(w, http.StatusOK, s.state())
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "product": "Alpha", "version": Version})
	})
	mux.HandleFunc("/api/sources", s.handleSources)
	mux.HandleFunc("/api/sources/refresh", s.handleRefreshSources)
	mux.HandleFunc("/api/nodes", s.handleNodes)
	mux.HandleFunc("/api/nodes/import", s.handleNodeImport)
	mux.HandleFunc("/api/connect", s.handleConnect)
	mux.HandleFunc("/api/connection", s.handleConnection)

	return securityHeaders(mux)
}

func (s *Server) state() stateResponse {
	sts := s.manager.Snapshot()
	engines := make([]engineView, 0, len(sts))
	healthy := 0
	var checked time.Time
	for _, st := range sts {
		if st.Healthy {
			healthy++
		}
		if st.LastCheck.After(checked) {
			checked = st.LastCheck
		}
		v := engineView{ID: st.ID, Name: st.Name, Address: st.Address, Priority: st.Priority, Healthy: st.Healthy, LatencyMs: st.Latency.Milliseconds(), Error: st.Error, Failures: st.Failures}
		if !st.LastCheck.IsZero() {
			v.LastCheck = st.LastCheck.UTC().Format(time.RFC3339)
		}
		engines = append(engines, v)
	}
	checkedAt := ""
	if !checked.IsZero() {
		checkedAt = checked.UTC().Format(time.RFC3339)
	}
	return stateResponse{
		Product: "Alpha", Version: Version, Status: "running", Listen: s.cfg.Listen,
		UIAddress: s.uiAddress, StartedAt: s.started.UTC().Format(time.RFC3339), CheckedAt: checkedAt,
		Healthy: healthy, Total: len(sts), Engines: engines, ProbeTargets: s.cfg.ProbeTargets(),
		SafetyMessage: "آلفا فقط مسیرهایی را استفاده می‌کند که آزمون انتهابه‌انتها را گذرانده‌اند؛ اتصال در خاموشی کامل تضمین‌پذیر نیست.",
	}
}

func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	// Browser UI is same-origin only. No CORS headers are emitted. A custom
	// header also forces cross-origin browser requests through a preflight.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "http" && u.Scheme != "https") {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return false
		}
	}
	got := r.Header.Get("X-Alpha-Session")
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
		http.Error(w, "local session required", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) Serve(ctx context.Context, address string) error {
	s.connMu.Lock()
	s.runCtx = ctx
	s.connMu.Unlock()
	go s.backgroundSourceRefresh(ctx)
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("UI listen address must be host:port: %w", err)
	}
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return errors.New("Alpha UI binds to loopback only; LAN/public binding is disabled in this alpha")
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen for Alpha UI on %s: %w", address, err)
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) || ctx.Err() != nil {
		return nil
	}
	return err
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
