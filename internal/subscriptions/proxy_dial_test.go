package subscriptions

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestDialPublicSourceViaSOCKS5PinsPublicIP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	observed := make(chan string, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		greeting := make([]byte, 3)
		if _, err := io.ReadFull(conn, greeting); err != nil {
			serverErr <- err
			return
		}
		if string(greeting) != string([]byte{5, 1, 0}) {
			serverErr <- fmt.Errorf("unexpected SOCKS greeting: %v", greeting)
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			serverErr <- err
			return
		}

		header := make([]byte, 4)
		if _, err := io.ReadFull(conn, header); err != nil {
			serverErr <- err
			return
		}
		if header[0] != 5 || header[1] != 1 || header[3] != 1 {
			serverErr <- fmt.Errorf("expected IPv4 CONNECT request, got %v", header)
			return
		}
		address := make([]byte, 6)
		if _, err := io.ReadFull(conn, address); err != nil {
			serverErr <- err
			return
		}
		target := net.JoinHostPort(net.IP(address[:4]).String(), strconv.Itoa(int(binary.BigEndian.Uint16(address[4:]))))
		observed <- target
		_, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
		serverErr <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialPublicSourceViaSOCKS5(ctx, listener.Addr().String(), "tcp", "1.1.1.1:443")
	if err != nil {
		t.Fatalf("dial through fake SOCKS5 server: %v", err)
	}
	_ = conn.Close()

	select {
	case got := <-observed:
		if got != "1.1.1.1:443" {
			t.Fatalf("SOCKS5 target must be the validated public IP, got %q", got)
		}
	case <-ctx.Done():
		t.Fatal("fake SOCKS5 server did not observe a CONNECT request")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestDialPublicSourceViaSOCKS5RejectsPrivateTargets(t *testing.T) {
	_, err := dialPublicSourceViaSOCKS5(context.Background(), "127.0.0.1:1", "tcp", "127.0.0.1:443")
	if err == nil {
		t.Fatal("private/local source target must be rejected before proxy dialing")
	}
}
