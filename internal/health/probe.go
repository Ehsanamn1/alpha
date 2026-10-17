package health

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ehsanamn1/alpha/internal/socks5"
)

// Probe performs an end-to-end HTTP(S) request through a local SOCKS5 engine.
// A successful TCP dial or SOCKS handshake alone is deliberately insufficient.
func Probe(ctx context.Context, socksAddr, rawURL string) (time.Duration, error) {
	start := time.Now()
	u, err := url.ParseRequestURI(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return 0, fmt.Errorf("invalid probe URL")
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	portN, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portN == 0 {
		return 0, fmt.Errorf("invalid probe URL port")
	}

	dialer := net.Dialer{Timeout: 4 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", socksAddr)
	if err != nil {
		return 0, fmt.Errorf("dial SOCKS5 %s: %w", socksAddr, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := socks5.Negotiate(conn); err != nil {
		return 0, fmt.Errorf("SOCKS5 negotiation: %w", err)
	}
	request, err := socks5.BuildConnectRequest(host, uint16(portN))
	if err != nil {
		return 0, err
	}
	if _, err := conn.Write(request); err != nil {
		return 0, fmt.Errorf("SOCKS5 connect request: %w", err)
	}
	reply, code, err := socks5.ReadReply(conn)
	if err != nil {
		return 0, fmt.Errorf("SOCKS5 connect reply: %w", err)
	}
	if code != 0 {
		return 0, fmt.Errorf("SOCKS5 CONNECT failed with code %d", code)
	}
	_ = reply

	var stream net.Conn = conn
	if u.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return 0, fmt.Errorf("TLS handshake: %w", err)
		}
		stream = tlsConn
	}

	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	if strings.ContainsAny(host, "\r\n") {
		return 0, fmt.Errorf("invalid host")
	}
	requestText := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: alpha-probe/0.2\r\nAccept: */*\r\nConnection: close\r\n\r\n", path, u.Host)
	if _, err := io.WriteString(stream, requestText); err != nil {
		return 0, fmt.Errorf("write HTTP request: %w", err)
	}
	br := bufio.NewReader(stream)
	req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return 0, fmt.Errorf("read HTTP response: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return 0, fmt.Errorf("probe target returned HTTP %d", resp.StatusCode)
	}
	return time.Since(start), nil
}

// ProbeAny checks multiple user-configured targets concurrently and succeeds if
// at least one target responds correctly through the same SOCKS5 engine.
func ProbeAny(parent context.Context, socksAddr string, targets []string) (time.Duration, error) {
	if len(targets) == 0 {
		return 0, fmt.Errorf("no probe targets configured")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	type result struct {
		latency time.Duration
		err     error
	}
	results := make(chan result, len(targets))
	for _, target := range targets {
		target := target
		go func() { latency, err := Probe(ctx, socksAddr, target); results <- result{latency: latency, err: err} }()
	}
	var errs []string
	for i := 0; i < len(targets); i++ {
		r := <-results
		if r.err == nil {
			return r.latency, nil
		}
		errs = append(errs, r.err.Error())
	}
	return 0, fmt.Errorf("all %d probe targets failed: %s", len(targets), strings.Join(errs, "; "))
}
