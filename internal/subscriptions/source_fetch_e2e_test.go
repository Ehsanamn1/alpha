package subscriptions

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Ehsanamn1/alpha/internal/model"
	"github.com/Ehsanamn1/alpha/internal/router"
)

type e2eSelector struct{ backends []model.Backend }

func (s *e2eSelector) HealthyBackends() []model.Backend { return s.backends }
func (s *e2eSelector) MarkBackendFailure(model.Backend, error) {}
func (s *e2eSelector) MarkBackendSuccess(model.Backend)       {}

// This test exercises real HTTP/TLS bytes through the Alpha router and the
// official sing-box process. The final SOCKS endpoint maps the pinned public IP
// to a local test source, making the fallback scenario reproducible in CI.
func TestSourceFetchFallbackEndToEndThroughAlphaAndSingBox(t *testing.T) {
	enginePath, err := exec.LookPath("sing-box")
	if err != nil {
		enginePath, err = exec.LookPath("sing-box.exe")
	}
	if err != nil {
		t.Skip("pinned official sing-box binary is not installed")
	}

	const pinnedHost = "1.1.1.1"
	payload := "vless://01234567-89ab-cdef-0123-456789abcdef@example.net:443?security=tls#AlphaE2E"
	roots, tlsServer := startPinnedIPTLSServer(t, pinnedHost, payload)
	defer tlsServer.Close()

	upstreamListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upstreamListener.Close() })
	observedTarget := make(chan string, 8)
	go serveMappedSOCKS5(upstreamListener, tlsServer.Listener.Addr().String(), observedTarget)

	singBoxAddr := freeLoopbackAddress(t)
	engineConfig := map[string]any{
		"log": map[string]any{"level": "warn"},
		"inbounds": []any{map[string]any{
			"type": "socks", "tag": "alpha-e2e-in", "listen": "127.0.0.1",
			"listen_port": portNumber(t, singBoxAddr),
		}},
		"outbounds": []any{map[string]any{
			"type": "socks", "tag": "alpha-e2e-out", "server": "127.0.0.1",
			"server_port": portNumber(t, upstreamListener.Addr().String()),
		}},
		"route": map[string]any{"final": "alpha-e2e-out"},
	}
	configPath := filepath.Join(t.TempDir(), "sing-box-e2e.json")
	rawConfig, err := json.MarshalIndent(engineConfig, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, rawConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	check := exec.Command(enginePath, "check", "-c", configPath)
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("sing-box rejected the E2E config: %v\n%s", err, output)
	}
	engineDone := startSingBoxProcess(t, enginePath, configPath)
	waitForLoopback(t, singBoxAddr, engineDone)

	alphaAddr := freeLoopbackAddress(t)
	selector := &e2eSelector{backends: []model.Backend{{
		ID: "sing-box-e2e", Name: "pinned sing-box test engine",
		Address: singBoxAddr, Priority: 1,
	}}}
	alphaCtx, stopAlpha := context.WithCancel(context.Background())
	defer stopAlpha()
	alphaDone := make(chan error, 1)
	go func() { alphaDone <- router.New(alphaAddr, selector).Serve(alphaCtx) }()
	waitForLoopback(t, alphaAddr, alphaDone)

	catalog, err := NewStoreAt(filepath.Join(t.TempDir(), "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog.fallbackProxyAddr = alphaAddr
	catalog.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	body, err := catalog.fetch(ctx, "https://"+pinnedHost+"/subscription.txt")
	if err != nil {
		t.Fatalf("fallback should recover via Alpha + sing-box after direct TLS fails: %v", err)
	}
	links := ParseLinks(body)
	if len(links) != 1 || links[0] != payload {
		t.Fatalf("unexpected subscription body after fallback: %#v", links)
	}
	node, _, err := ParseNode(links[0], "e2e-source", "E2E source")
	if err != nil || node.Protocol != "vless" {
		t.Fatalf("fetched node is not importable, node=%+v err=%v", node, err)
	}
	select {
	case target := <-observedTarget:
		if target != net.JoinHostPort(pinnedHost, "443") {
			t.Fatalf("expected pinned-IP CONNECT, got %q", target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SOCKS upstream never observed the pinned-IP request")
	}
}

func startPinnedIPTLSServer(t *testing.T, pinnedIP, payload string) (*x509.CertPool, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(17), Subject: pkix.Name{CommonName: "Alpha E2E source"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(2 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true, IPAddresses: []net.IP{net.ParseIP(pinnedIP)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pair := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Connection", "close")
		_, _ = io.WriteString(w, payload+"\n")
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	return roots, server
}

func serveMappedSOCKS5(listener net.Listener, mappedTarget string, observed chan<- string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(30 * time.Second))
			var g [2]byte
			if _, err := io.ReadFull(c, g[:]); err != nil || g[0] != 5 || g[1] == 0 {
				return
			}
			methods := make([]byte, int(g[1]))
			if _, err := io.ReadFull(c, methods); err != nil {
				return
			}
			noAuth := false
			for _, method := range methods {
				if method == 0 { noAuth = true }
			}
			if !noAuth { _, _ = c.Write([]byte{5, 0xff}); return }
			if _, err := c.Write([]byte{5, 0}); err != nil { return }

			var h [4]byte
			if _, err := io.ReadFull(c, h[:]); err != nil || h[0] != 5 || h[1] != 1 || h[2] != 0 { return }
			host := ""
			switch h[3] {
			case 1:
				var ip [4]byte
				if _, err := io.ReadFull(c, ip[:]); err != nil { return }
				host = net.IP(ip[:]).String()
			case 4:
				var ip [16]byte
				if _, err := io.ReadFull(c, ip[:]); err != nil { return }
				host = net.IP(ip[:]).String()
			case 3:
				var n [1]byte
				if _, err := io.ReadFull(c, n[:]); err != nil { return }
				name := make([]byte, int(n[0]))
				if _, err := io.ReadFull(c, name); err != nil { return }
				host = string(name)
			default:
				return
			}
			var p [2]byte
			if _, err := io.ReadFull(c, p[:]); err != nil { return }
			target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(p[:]))))
			select { case observed <- target: default: }
			if target != net.JoinHostPort("1.1.1.1", "443") {
				_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
				return
			}
			dst, err := net.DialTimeout("tcp", mappedTarget, 4*time.Second)
			if err != nil { _, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0}); return }
			defer dst.Close()
			if _, err := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil { return }
			go func() { _, _ = io.Copy(dst, c); _ = dst.Close() }()
			_, _ = io.Copy(c, dst)
		}(conn)
	}
}

func startSingBoxProcess(t *testing.T, path, configPath string) <-chan error {
	t.Helper()
	cmd := exec.Command(path, "run", "-c", configPath)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil { t.Fatalf("start sing-box: %v", err) }
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		if cmd.Process != nil { _ = cmd.Process.Kill() }
		select { case <-done: case <-time.After(3 * time.Second): }
	})
	return done
}

func waitForLoopback(t *testing.T, address string, processDone <-chan error) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 150*time.Millisecond)
		if err == nil { _ = conn.Close(); return }
		select {
		case processErr := <-processDone:
			t.Fatalf("process stopped before opening %s: %v", address, processErr)
		default:
		}
		time.Sleep(80 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for loopback listener %s", address)
}

func freeLoopbackAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func portNumber(t *testing.T, address string) int {
	t.Helper()
	_, text, err := net.SplitHostPort(address)
	if err != nil { t.Fatal(err) }
	port, err := strconv.Atoi(text)
	if err != nil { t.Fatal(err) }
	return port
}

var _ router.Selector = (*e2eSelector)(nil)
