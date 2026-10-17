package router

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/Ehsanamn1/alpha/internal/model"
	"github.com/Ehsanamn1/alpha/internal/socks5"
)

type Selector interface {
	HealthyBackends() []model.Backend
	MarkBackendFailure(model.Backend, error)
	MarkBackendSuccess(model.Backend)
}

type Server struct {
	addr     string
	selector Selector
}

func New(addr string, selector Selector) *Server { return &Server{addr: addr, selector: selector} }

func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	defer ln.Close()
	go func() { <-ctx.Done(); _ = ln.Close() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(20 * time.Second))
	var head [2]byte
	if _, err := io.ReadFull(client, head[:]); err != nil {
		return
	}
	if head[0] != 5 || head[1] == 0 {
		return
	}
	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(client, methods); err != nil {
		return
	}
	hasNoAuth := false
	for _, m := range methods {
		if m == 0 {
			hasNoAuth = true
		}
	}
	if !hasNoAuth {
		_, _ = client.Write([]byte{5, 0xff})
		return
	}
	if _, err := client.Write([]byte{5, 0}); err != nil {
		return
	}

	request, cmd, err := readClientRequest(client)
	if err != nil {
		sendFailure(client, 1)
		return
	}
	if cmd != 1 {
		sendFailure(client, 7)
		return
	}

	backends := s.selector.HealthyBackends()
	if len(backends) == 0 {
		sendFailure(client, 1)
		return
	}
	var chosen net.Conn
	var chosenReader io.Reader
	var chosenBackend model.Backend
	var successfulReply []byte
	for _, backend := range backends {
		upstream, err := net.DialTimeout("tcp", backend.Address, 3*time.Second)
		if err != nil {
			s.selector.MarkBackendFailure(backend, fmt.Errorf("dial local engine: %w", err))
			continue
		}
		_ = upstream.SetDeadline(time.Now().Add(12 * time.Second))
		if err := socks5.Negotiate(upstream); err != nil {
			_ = upstream.Close()
			s.selector.MarkBackendFailure(backend, fmt.Errorf("upstream handshake: %w", err))
			continue
		}
		if _, err := upstream.Write(request); err != nil {
			_ = upstream.Close()
			s.selector.MarkBackendFailure(backend, fmt.Errorf("write upstream request: %w", err))
			continue
		}
		br := makeBufferedReader(upstream)
		reply, code, err := socks5.ReadReply(br)
		if err != nil {
			_ = upstream.Close()
			s.selector.MarkBackendFailure(backend, fmt.Errorf("upstream reply: %w", err))
			continue
		}
		if code != 0 {
			_ = upstream.Close()
			continue
		}
		chosen, chosenReader, chosenBackend, successfulReply = upstream, br, backend, reply
		break
	}
	if chosen == nil {
		sendFailure(client, 5)
		return
	}
	if _, err := client.Write(successfulReply); err != nil {
		_ = chosen.Close()
		return
	}
	_ = client.SetDeadline(time.Time{})
	_ = chosen.SetDeadline(time.Time{})
	s.selector.MarkBackendSuccess(chosenBackend)
	log.Printf("SOCKS route selected: %s (%s)", chosenBackend.Name, chosenBackend.Address)
	pipe(client, chosen, chosenReader)
}

// Use a buffered reader to preserve any payload bytes already read ahead with the SOCKS reply.
func makeBufferedReader(c net.Conn) *bufio.Reader { return bufio.NewReaderSize(c, 32*1024) }

func readClientRequest(r io.Reader) ([]byte, byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, 0, err
	}
	if h[0] != 5 {
		return nil, 0, errors.New("invalid SOCKS5 request version")
	}
	if h[2] != 0 {
		return nil, 0, errors.New("invalid SOCKS5 reserved byte")
	}
	var addr []byte
	switch h[3] {
	case 1:
		addr = make([]byte, 4)
	case 4:
		addr = make([]byte, 16)
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return nil, 0, err
		}
		addr = append([]byte{n[0]}, make([]byte, int(n[0]))...)
	default:
		return nil, 0, errors.New("unsupported SOCKS5 address type")
	}
	if len(addr) > 0 {
		if h[3] != 3 {
			if _, err := io.ReadFull(r, addr); err != nil {
				return nil, 0, err
			}
		} else {
			if _, err := io.ReadFull(r, addr[1:]); err != nil {
				return nil, 0, err
			}
		}
	}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return nil, 0, err
	}
	request := append([]byte(nil), h[:]...)
	request = append(request, addr...)
	request = append(request, port[:]...)
	_ = binary.BigEndian.Uint16(port[:]) // Parse/consume validated target port.
	return request, h[1], nil
}

func sendFailure(w io.Writer, code byte) { _, _ = w.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0}) }

func pipe(client, upstream net.Conn, upstreamReader io.Reader) {
	defer client.Close()
	defer upstream.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); closeWrite(upstream); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstreamReader); closeWrite(client); done <- struct{}{} }()
	<-done
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	_ = upstream.SetDeadline(time.Now().Add(2 * time.Second))
	<-done
}

func closeWrite(c net.Conn) {
	if tcp, ok := c.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
}
