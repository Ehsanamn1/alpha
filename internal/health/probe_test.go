package health

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestProbeChecksHTTPThroughSOCKS5(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer httpServer.Close()

	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyLn.Close()
	go func() {
		for {
			client, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go handleTestSOCKS(client)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	latency, err := Probe(ctx, proxyLn.Addr().String(), httpServer.URL+"/health")
	if err != nil {
		t.Fatalf("expected end-to-end probe to succeed: %v", err)
	}
	if latency <= 0 {
		t.Fatalf("expected positive latency, got %v", latency)
	}
}

func handleTestSOCKS(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	var greeting [3]byte
	if _, err := io.ReadFull(client, greeting[:]); err != nil {
		return
	}
	_, _ = client.Write([]byte{5, 0})
	var req [4]byte
	if _, err := io.ReadFull(client, req[:]); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 1:
		var addr [4]byte
		if _, err := io.ReadFull(client, addr[:]); err != nil {
			return
		}
		host = net.IP(addr[:]).String()
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(client, n[:]); err != nil {
			return
		}
		b := make([]byte, int(n[0]))
		if _, err := io.ReadFull(client, b); err != nil {
			return
		}
		host = string(b)
	default:
		return
	}
	var pb [2]byte
	if _, err := io.ReadFull(client, pb[:]); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(pb[:])
	target, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))), time.Second)
	if err != nil {
		_, _ = client.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer target.Close()
	_, _ = client.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(target, client)
		if c, ok := target.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, target)
		if c, ok := client.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
}

func TestProbeAnySucceedsWhenAtLeastOneTargetWorks(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer httpServer.Close()
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyLn.Close()
	go func() {
		for {
			c, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go handleTestSOCKS(c)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	latency, err := ProbeAny(ctx, proxyLn.Addr().String(), []string{"http://127.0.0.1:1/unreachable", httpServer.URL + "/"})
	if err != nil {
		t.Fatalf("expected a successful target to make ProbeAny healthy: %v", err)
	}
	if latency <= 0 {
		t.Fatalf("expected positive latency, got %v", latency)
	}
}
