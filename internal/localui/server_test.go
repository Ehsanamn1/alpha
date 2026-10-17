package localui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Ehsanamn1/alpha/internal/config"
	"github.com/Ehsanamn1/alpha/internal/orchestrator"
	"github.com/Ehsanamn1/alpha/internal/subscriptions"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Config{
		Listen:                 "127.0.0.1:1088",
		ProbeURLs:              []string{"https://example.com/"},
		ProbeIntervalSeconds:   15,
		ProbeTimeoutSeconds:    2,
		EngineStartTimeoutSecs: 2,
		FailureThreshold:       2,
	}
	manager := orchestrator.New(cfg)
	srv, err := New(cfg, manager, "127.0.0.1:18888")
	if err != nil {
		t.Fatal(err)
	}
	store, err := subscriptions.NewStoreAt(t.TempDir() + "/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	srv.catalog = store
	return srv
}

func TestUIRendersSessionTokenAndSecurityHeaders(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET / status = %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "__ALPHA_SESSION__") {
		t.Fatal("session placeholder was not replaced")
	}
	if !regexp.MustCompile(`<meta name="alpha-session" content="[a-f0-9]{64}">`).MatchString(body) {
		t.Fatal("page has no valid session token meta tag")
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing anti-framing header")
	}
}

func TestAPIStateAndPOSTRequiresSession(t *testing.T) {
	srv := testServer(t)
	stateReq := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	stateRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(stateRes, stateReq)
	if stateRes.Code != http.StatusOK {
		t.Fatalf("GET state status = %d", stateRes.Code)
	}
	var state stateResponse
	if err := json.Unmarshal(stateRes.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Product != "Alpha" || state.Listen != "127.0.0.1:1088" {
		t.Fatalf("unexpected state: %+v", state)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/api/probe", strings.NewReader(`{}`))
	badRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(badRes, badReq)
	if badRes.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated POST status = %d, expected 403", badRes.Code)
	}
}

func TestAPIPostWithToken(t *testing.T) {
	srv := testServer(t)
	pageReq := httptest.NewRequest(http.MethodGet, "/", nil)
	pageRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRes, pageReq)
	match := regexp.MustCompile(`<meta name="alpha-session" content="([a-f0-9]{64})">`).FindStringSubmatch(pageRes.Body.String())
	if len(match) != 2 {
		t.Fatal("could not extract test token")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/probe", strings.NewReader(`{}`))
	req.Header.Set("X-Alpha-Session", match[1])
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("authenticated POST status = %d body=%s", res.Code, res.Body.String())
	}
}

func TestUIServerRejectsNonLoopbackAddress(t *testing.T) {
	srv := testServer(t)
	if err := srv.Serve(neverContext(), "0.0.0.0:18888"); err == nil || !strings.Contains(err.Error(), "loopback only") {
		t.Fatalf("expected non-loopback to be rejected, got %v", err)
	}
}

func neverContext() context.Context { return context.Background() }

func sessionToken(t *testing.T, srv *Server) string {
	t.Helper()
	pageReq := httptest.NewRequest(http.MethodGet, "/", nil)
	pageRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRes, pageReq)
	match := regexp.MustCompile(`<meta name="alpha-session" content="([a-f0-9]{64})">`).FindStringSubmatch(pageRes.Body.String())
	if len(match) != 2 {
		t.Fatal("could not extract local session token")
	}
	return match[1]
}

func TestImportAPIStoresNodeWithoutExposingRawCredential(t *testing.T) {
	srv := testServer(t)
	token := sessionToken(t, srv)
	uri := "vless://01234567-89ab-cdef-0123-456789abcdef@edge.example.net:443?security=tls&sni=edge.example.net#VisibleNode"
	body, _ := json.Marshal(map[string]string{"text": uri})
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/import", strings.NewReader(string(body)))
	req.Header.Set("X-Alpha-Session", token)
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("import status = %d: %s", res.Code, res.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	got := httptest.NewRecorder()
	srv.Handler().ServeHTTP(got, get)
	if got.Code != http.StatusOK {
		t.Fatalf("nodes status = %d", got.Code)
	}
	if strings.Contains(got.Body.String(), "01234567-89ab-cdef-0123-456789abcdef") {
		t.Fatal("raw node UUID leaked from read-only catalog API")
	}
	if !strings.Contains(got.Body.String(), "VisibleNode") {
		t.Fatal("public node metadata missing")
	}
}

func TestSubscriptionMutationRequiresSessionAndBlocksLocalSources(t *testing.T) {
	srv := testServer(t)
	body := `{"name":"local","url":"https://127.0.0.1/secret"}`
	unauth := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	unauthRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated source write returned %d", unauthRes.Code)
	}

	auth := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	auth.Header.Set("X-Alpha-Session", sessionToken(t, srv))
	authRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authRes, auth)
	if authRes.Code != http.StatusBadRequest {
		t.Fatalf("private source URL should be rejected, got %d: %s", authRes.Code, authRes.Body.String())
	}
}

func TestConnectionRequiresImportedNodeBeforeEngineLaunch(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/connect", strings.NewReader(`{"action":"connect"}`))
	req.Header.Set("X-Alpha-Session", sessionToken(t, srv))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("empty library connect status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "نودی وارد نشده") {
		t.Fatalf("unexpected response: %s", res.Body.String())
	}
}
