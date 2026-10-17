package router

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Ehsanamn1/alpha/internal/model"
	"github.com/Ehsanamn1/alpha/internal/socks5"
)

type testSelector struct{ backends []model.Backend }

func (s *testSelector) HealthyBackends() []model.Backend        { return s.backends }
func (s *testSelector) MarkBackendFailure(model.Backend, error) {}
func (s *testSelector) MarkBackendSuccess(model.Backend)        {}

func TestRouterForwardsThroughSOCKSBackend(t *testing.T) {
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()
	go func() {
		c, err := backendLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var greeting [3]byte
		if _, err = io.ReadFull(c, greeting[:]); err != nil {
			return
		}
		_, _ = c.Write([]byte{5, 0})
		var h [5]byte
		if _, err = io.ReadFull(c, h[:]); err != nil {
			return
		}
		if h[0] != 5 || h[3] != 3 {
			return
		}
		nameLen := int(h[4])
		rest := make([]byte, nameLen+2)
		if _, err = io.ReadFull(c, rest); err != nil {
			return
		}
		_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
		_, _ = io.Copy(c, c)
	}()

	routerLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	routerAddr := routerLn.Addr().String()
	_ = routerLn.Close()
	deadLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := deadLn.Addr().String()
	_ = deadLn.Close()
	selector := &testSelector{backends: []model.Backend{
		{ID: "first-down", Name: "unreachable test backend", Address: deadAddr, Priority: 1},
		{ID: "fake", Name: "fake", Address: backendLn.Addr().String(), Priority: 2},
	}}
	srv := New(routerAddr, selector)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	var client net.Conn
	for time.Now().Before(deadline) {
		client, err = net.DialTimeout("tcp", routerAddr, 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect router: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = client.Write([]byte{5, 1, 0})
	var method [2]byte
	if _, err = io.ReadFull(client, method[:]); err != nil {
		t.Fatal(err)
	}
	if method[0] != 5 || method[1] != 0 {
		t.Fatalf("bad method response: %v", method)
	}
	request, _ := socks5.BuildConnectRequest("example.com", 80)
	_, _ = client.Write(request)
	reply := make([]byte, 10)
	if _, err = io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0 {
		t.Fatalf("connect failed: %v", reply)
	}
	payload := []byte("hello")
	if _, err = client.Write(payload); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, len(payload))
	if _, err = io.ReadFull(client, echo); err != nil {
		t.Fatal(err)
	}
	if string(echo) != string(payload) {
		t.Fatalf("echo mismatch: %q", echo)
	}
	cancel()
	select {
	case <-serveErr:
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}
