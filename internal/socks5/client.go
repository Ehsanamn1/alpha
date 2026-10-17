package socks5

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

var ErrNoAuth = errors.New("SOCKS5 upstream does not accept no-authentication")

func Negotiate(conn net.Conn) error {
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return err
	}
	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}
	if resp[0] != 5 || resp[1] != 0 {
		return ErrNoAuth
	}
	return nil
}

func BuildConnectRequest(host string, port uint16) ([]byte, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			out := []byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0}
			copy(out[4:8], v4)
			binary.BigEndian.PutUint16(out[8:10], port)
			return out, nil
		}
		v6 := ip.To16()
		out := []byte{5, 1, 0, 4}
		out = append(out, v6...)
		p := make([]byte, 2)
		binary.BigEndian.PutUint16(p, port)
		return append(out, p...), nil
	}
	if len(host) == 0 || len(host) > 255 || strings.ContainsAny(host, "\x00\r\n") {
		return nil, errors.New("invalid target hostname")
	}
	out := []byte{5, 1, 0, 3, byte(len(host))}
	out = append(out, []byte(host)...)
	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, port)
	return append(out, p...), nil
}

func ReadReply(r io.Reader) ([]byte, byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, 0, err
	}
	if header[0] != 5 {
		return nil, 0, fmt.Errorf("invalid SOCKS5 reply version %d", header[0])
	}
	addrLen := 0
	switch header[3] {
	case 1:
		addrLen = 4
	case 4:
		addrLen = 16
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return nil, 0, err
		}
		addrLen = int(n[0])
		header = append(header, n[0])
	default:
		return nil, 0, fmt.Errorf("invalid SOCKS5 address type %d", header[3])
	}
	rest := make([]byte, addrLen+2)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, 0, err
	}
	return append(header, rest...), header[1], nil
}
