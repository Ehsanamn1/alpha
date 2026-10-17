package localui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ehsanamn1/alpha/internal/subscriptions"
)

type sourceRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type importRequest struct {
	Text string `json:"text"`
}

type connectRequest struct {
	Action string `json:"action"`
}

func catalogNodeIDs(nodes []subscriptions.Node) string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return strings.Join(ids, ",")
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		catalog := s.catalog.Snapshot()
		s.writeJSON(w, http.StatusOK, map[string]any{"sources": catalog.Sources, "node_count": len(catalog.Nodes), "updated_at": catalog.UpdatedAt})
	case http.MethodPost:
		if !s.authorized(w, r) {
			return
		}
		var req sourceRequest
		if err := decodeJSONBody(w, r, &req, 64<<10); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		source, err := s.catalog.AddSource(req.Name, req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Do one bounded initial fetch in the background. The source is retained
		// if this request fails; the UI can report the reason and retry later.
		go func() {
			before := catalogNodeIDs(s.catalog.NodesForConnect())
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			s.catalog.Refresh(ctx)
			if before != catalogNodeIDs(s.catalog.NodesForConnect()) {
				_ = s.refreshActiveConfig()
			}
		}()
		s.writeJSON(w, http.StatusCreated, map[string]any{"source": source, "message": "منبع اضافه شد؛ دریافت اولیه در پس‌زمینه انجام می‌شود."})
	case http.MethodDelete:
		if !s.authorized(w, r) {
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "source id is required", http.StatusBadRequest)
			return
		}
		if err := s.catalog.DeleteSource(id); err != nil {
			http.Error(w, "source not found", http.StatusNotFound)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w, "GET, POST, DELETE")
	}
}

func (s *Server) handleRefreshSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.authorized(w, r) {
		return
	}
	before := catalogNodeIDs(s.catalog.NodesForConnect())
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	added, skipped, failures := s.catalog.Refresh(ctx)
	if before != catalogNodeIDs(s.catalog.NodesForConnect()) {
		if err := s.refreshActiveConfig(); err != nil && !errors.Is(err, errConnectionInactive) {
			s.writeJSON(w, http.StatusOK, map[string]any{"added": added, "skipped": skipped, "failed_sources": failures, "warning": "فهرست به‌روز شد اما اعمال مجدد کانفیگ فعال ناموفق بود: " + err.Error()})
			return
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"added": added, "skipped": skipped, "failed_sources": failures, "node_count": len(s.catalog.NodesForConnect()), "message": "بررسی منابع انجام شد."})
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		catalog := s.catalog.Snapshot()
		s.writeJSON(w, http.StatusOK, map[string]any{"nodes": catalog.Nodes, "node_count": len(catalog.Nodes), "updated_at": catalog.UpdatedAt})
	case http.MethodDelete:
		if !s.authorized(w, r) {
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "node id is required", http.StatusBadRequest)
			return
		}
		if err := s.catalog.DeleteNode(id); err != nil {
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}
		if err := s.refreshActiveConfig(); err != nil && !errors.Is(err, errConnectionInactive) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "node_count": len(s.catalog.NodesForConnect())})
	default:
		methodNotAllowed(w, "GET, DELETE")
	}
}

func (s *Server) handleNodeImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.authorized(w, r) {
		return
	}
	var req importRequest
	if err := decodeJSONBody(w, r, &req, 1<<20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	added, skipped, err := s.catalog.ImportText(req.Text, "", "واردسازی دستی")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.refreshActiveConfig(); err != nil && !errors.Is(err, errConnectionInactive) {
		s.writeJSON(w, http.StatusOK, map[string]any{"added": added, "skipped": skipped, "node_count": len(s.catalog.NodesForConnect()), "warning": "نودها ذخیره شدند اما اتصال فعال به‌روز نشد: " + err.Error()})
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]any{"added": added, "skipped": skipped, "node_count": len(s.catalog.NodesForConnect())})
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.authorized(w, r) {
		return
	}
	var req connectRequest
	if err := decodeJSONBody(w, r, &req, 64<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch req.Action {
	case "connect":
		if err := s.startConnection(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case "disconnect":
		s.stopConnection()
	default:
		http.Error(w, "action must be connect or disconnect", http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, s.connectionState())
}

func (s *Server) handleConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	s.writeJSON(w, http.StatusOK, s.connectionState())
}

func (s *Server) connectionState() map[string]any {
	s.connMu.Lock()
	active, status, problem, pid, since := s.connActive, s.connStatus, s.connError, s.connPID, s.activeSince
	s.connMu.Unlock()
	enginePath, engineErr := s.findSingBox()
	return map[string]any{
		"active": active, "status": status, "error": problem, "pid": pid,
		"active_since": since, "node_count": len(s.catalog.NodesForConnect()),
		"engine_available": engineErr == nil, "engine_path": enginePath,
		"proxy": s.cfg.Listen, "singbox_socks": "127.0.0.1:2089",
		"notice": "اتصال محلی SOCKS5 را برای برنامه‌ها تنظیم کن؛ این رابط هنوز VPN سراسری Android/iOS نیست.",
	}
}

var errConnectionInactive = errors.New("no active managed connection")

func (s *Server) findSingBox() (string, error) {
	if path, err := exec.LookPath("sing-box"); err == nil {
		return path, nil
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		for _, name := range []string{"sing-box", "sing-box.exe"} {
			candidate := filepath.Join(base, "engines", name)
			if st, e := os.Stat(candidate); e == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", errors.New("sing-box پیدا نشد؛ موتور sing-box را از منبع رسمی نصب کن و به PATH اضافه کن")
}

func (s *Server) startConnection() error {
	nodes := s.catalog.NodesForConnect()
	if len(nodes) == 0 {
		return errors.New("هنوز نودی وارد نشده است؛ ابتدا یک اشتراک HTTPS یا لینک VLESS/VMess/Trojan/SS/Hysteria2/TUIC وارد کن")
	}
	enginePath, err := s.findSingBox()
	if err != nil {
		return err
	}
	generated, err := subscriptions.SingBoxConfig(nodes)
	if err != nil {
		return err
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(base, "Alpha")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	configPath := filepath.Join(dir, "sing-box.active.json")
	if err := writeAtomicPrivate(configPath, generated); err != nil {
		return err
	}
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 12*time.Second)
	checkCmd := exec.CommandContext(checkCtx, enginePath, "check", "-c", configPath)
	checkOutput, checkErr := checkCmd.CombinedOutput()
	checkCancel()
	if checkErr != nil {
		message := strings.TrimSpace(string(checkOutput))
		if len(message) > 600 {
			message = message[:600]
		}
		if message == "" {
			message = checkErr.Error()
		}
		return fmt.Errorf("sing-box configuration validation failed: %s", message)
	}

	s.connMu.Lock()
	if s.runCtx == nil || s.runCtx.Err() != nil {
		s.connMu.Unlock()
		return errors.New("سرور محلی هنوز آماده نیست؛ صفحه را ببند و آلفا را دوباره اجرا کن")
	}
	if s.connCmd != nil && s.connCmd.Process != nil {
		_ = s.connCmd.Process.Kill()
	}
	s.connGen++
	gen := s.connGen
	s.connActive, s.connStatus, s.connError, s.connPID = true, "starting", "", 0
	s.activeSince = time.Now().UTC().Format(time.RFC3339)
	s.connPath, s.configPath = enginePath, configPath
	ctx := s.runCtx
	s.connMu.Unlock()
	go s.superviseConnection(ctx, gen, enginePath, configPath)
	return nil
}

func (s *Server) superviseConnection(ctx context.Context, gen uint64, enginePath, configPath string) {
	for {
		s.connMu.Lock()
		if !s.connActive || s.connGen != gen || ctx.Err() != nil {
			s.connMu.Unlock()
			return
		}
		s.connStatus = "starting"
		cmd := exec.CommandContext(ctx, enginePath, "run", "-c", configPath)
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		if err := cmd.Start(); err != nil {
			s.connStatus, s.connError, s.connPID = "retrying", err.Error(), 0
			s.connMu.Unlock()
			if !waitOrDone(ctx, 3*time.Second) {
				return
			}
			continue
		}
		s.connCmd, s.connPID, s.connStatus, s.connError = cmd, cmd.Process.Pid, "connecting", ""
		s.connMu.Unlock()
		err := cmd.Wait()
		s.connMu.Lock()
		if s.connCmd == cmd {
			s.connCmd = nil
		}
		if !s.connActive || s.connGen != gen || ctx.Err() != nil {
			s.connPID = 0
			s.connMu.Unlock()
			return
		}
		s.connPID, s.connStatus = 0, "reconnecting"
		if err != nil {
			s.connError = err.Error()
		} else {
			s.connError = "sing-box exited; restarting automatically"
		}
		s.connMu.Unlock()
		if !waitOrDone(ctx, 2*time.Second) {
			return
		}
	}
}

func (s *Server) stopConnection() {
	s.connMu.Lock()
	s.connActive = false
	s.connGen++
	if s.connCmd != nil && s.connCmd.Process != nil {
		_ = s.connCmd.Process.Kill()
		s.connCmd = nil
	}
	s.connPID, s.connStatus, s.connError, s.activeSince = 0, "disconnected", "", ""
	s.connMu.Unlock()
}

// refreshActiveConfig hot-reloads subscriptions by replacing the generated config
// and stopping the current child; the supervisor brings sing-box back up.
func (s *Server) refreshActiveConfig() error {
	s.connMu.Lock()
	active, path := s.connActive, s.configPath
	s.connMu.Unlock()
	if !active {
		return errConnectionInactive
	}
	nodes := s.catalog.NodesForConnect()
	if len(nodes) == 0 {
		s.stopConnection()
		return nil
	}
	generated, err := subscriptions.SingBoxConfig(nodes)
	if err != nil {
		return err
	}
	if err := writeAtomicPrivate(path, generated); err != nil {
		return err
	}
	s.connMu.Lock()
	if s.connActive && s.connCmd != nil && s.connCmd.Process != nil {
		s.connStatus = "applying-updated-nodes"
		_ = s.connCmd.Process.Kill()
	}
	s.connMu.Unlock()
	return nil
}

func (s *Server) backgroundSourceRefresh(ctx context.Context) {
	// Fast initial refresh helps newly configured sources, followed by a conservative interval.
	if !waitOrDone(ctx, 2*time.Second) {
		return
	}
	for {
		if len(s.catalog.Snapshot().Sources) > 0 {
			before := catalogNodeIDs(s.catalog.NodesForConnect())
			work, cancel := context.WithTimeout(ctx, 90*time.Second)
			s.catalog.Refresh(work)
			cancel()
			after := catalogNodeIDs(s.catalog.NodesForConnect())
			// Reapply if the catalog changed in either direction, including a
			// replacement/removal that did not increase the total node count.
			if before != after {
				_ = s.refreshActiveConfig()
			}
		}
		if !waitOrDone(ctx, 15*time.Minute) {
			return
		}
	}
}

func waitOrDone(ctx context.Context, delay time.Duration) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func writeAtomicPrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, destination any, max int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("request must contain a single JSON object")
	}
	return nil
}
