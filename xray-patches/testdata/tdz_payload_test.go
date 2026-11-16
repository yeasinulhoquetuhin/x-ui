package internet

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTDZPayloadMatchesLegacyHeaderAndBytes(t *testing.T) {
	for _, header := range []string{
		"GET / HTTP/1.1\r\nHost: test\r\n\r\n",
		"CONNECT example:443 HTTP/1.1\r\n\r\n",
		"GET / HTTP/1.1\r\nUpgrade: websocket\r\nSec-WebSocket-Key: anything\r\n\r\n",
		"not an HTTP request at all\r\n\r\n",
		"\x00\xff\x01opaque binary preface\r\n\r\n",
		"\r\n\r\n",
	} {
		for _, fragmented := range []bool{false, true} {
			t.Run(strings.ReplaceAll(header[:min(len(header), 12)], "\r\n", "_")+"/fragmented="+string(rune('0'+boolInt(fragmented))), func(t *testing.T) {
				server, client := net.Pipe()
				defer server.Close()
				defer client.Close()
				client.SetDeadline(time.Now().Add(2 * time.Second))
				// This second HTTP header and binary data must never be parsed or
				// consumed. It can be the real WS/HTTPUpgrade transport handshake.
				pending := []byte("GET /native HTTP/1.1\r\nHost: backend\r\n\r\n\x00\xff\x80\r\n")
				done := make(chan error, 1)
				go func() {
					conn, err := tdzAcceptPayload(server)
					if err == nil {
						got := make([]byte, len(pending))
						_, err = io.ReadFull(conn, got)
						if err == nil && !bytes.Equal(got, pending) {
							err = io.ErrUnexpectedEOF
						}
						if err == nil && (conn.RemoteAddr() != server.RemoteAddr() || conn.LocalAddr() != server.LocalAddr()) {
							err = io.ErrUnexpectedEOF
						}
						if err == nil {
							_, err = conn.Write(got)
						}
					}
					done <- err
				}()
				go func() {
					if fragmented {
						for _, b := range []byte(header) {
							if _, err := client.Write([]byte{b}); err != nil {
								return
							}
						}
						client.Write(pending)
					} else {
						client.Write(append([]byte(header), pending...))
					}
				}()
				response := make([]byte, len(tdzPayloadResponse))
				if _, err := io.ReadFull(client, response); err != nil {
					t.Fatal(err)
				}
				if string(response) != tdzPayloadResponse {
					t.Fatalf("response changed: %q", response)
				}
				got := make([]byte, len(pending))
				if _, err := io.ReadFull(client, got); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, pending) {
					t.Fatal("post-header data changed")
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestTDZPayloadReturnsBeforeTunnelSendsAnything(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	client.SetDeadline(time.Now().Add(time.Second))
	done := make(chan error, 1)
	go func() {
		conn, err := tdzAcceptPayload(server)
		if err == nil {
			_, err = conn.Write([]byte("backend banner"))
		}
		done <- err
	}()
	client.Write([]byte("opaque\r\n\r\n"))
	got := make([]byte, len(tdzPayloadResponse)+len("backend banner"))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != tdzPayloadResponse+"backend banner" {
		t.Fatal("waited for or rewrote backend bytes")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTDZPayloadIncompleteAndOversized(t *testing.T) {
	for _, input := range []string{"GET / HTTP/1.1\n\n", strings.Repeat("x", tdzPayloadMaxHeader)} {
		server, client := net.Pipe()
		go client.Write([]byte(input))
		_, err := tdzAcceptPayloadTimeout(server, 25*time.Millisecond)
		server.Close()
		client.Close()
		if err == nil {
			t.Fatal("incomplete/oversized payload accepted")
		}
	}
}

func TestTDZPayloadTimeoutIsIdlePerRead(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan error, 1)
	go func() { _, err := tdzAcceptPayloadTimeout(server, 250*time.Millisecond); done <- err }()
	for _, part := range []string{"first", " second", " third", "\r\n\r\n"} {
		if _, err := client.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
		if part != "\r\n\r\n" {
			time.Sleep(120 * time.Millisecond)
		}
	}
	got := make([]byte, len(tdzPayloadResponse))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
