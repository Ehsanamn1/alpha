package socks5

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestBuildConnectRequestDomain(t *testing.T) {
	got, err := BuildConnectRequest("example.com", 443)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 5 || got[1] != 1 || got[3] != 3 || got[4] != byte(len("example.com")) {
		t.Fatalf("unexpected request: %v", got)
	}
	if string(got[5:16]) != "example.com" {
		t.Fatalf("wrong host: %q", got[5:16])
	}
	if binary.BigEndian.Uint16(got[len(got)-2:]) != 443 {
		t.Fatal("wrong port")
	}
}

func TestReadReplyPreservesIPv4Reply(t *testing.T) {
	want := []byte{5, 0, 0, 1, 127, 0, 0, 1, 0x1f, 0x90}
	got, code, err := ReadReply(bytes.NewReader(want))
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("unexpected code %d", code)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("reply mismatch: %v != %v", got, want)
	}
}

func TestReadReplyRejectsTruncatedReply(t *testing.T) {
	_, _, err := ReadReply(bytes.NewReader([]byte{5, 0, 0, 1, 127}))
	if err == nil || err == io.EOF {
		t.Fatalf("expected truncated reply error, got %v", err)
	}
}
