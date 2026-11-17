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

func TestTDZAutoPayloadNativeAndInjectedHandshakes(t *testing.T) {
	ws := "GET /native HTTP/1.1\r\nHost: test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
	for _, tc := range []struct {
		name, network, input string
		payload              bool
	}{
		{"raw-vless", "", "\x00\x11\xff\x80\r\n\r\n", false},
		{"tls-clienthello", "websocket", "\x16\x03\x01\x00\x05hello", false},
		{"http2", "", "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n", false},
		{"normal-ws", "websocket", ws, false},
		{"normal-httpupgrade", "httpupgrade", "GET /native HTTP/1.1\r\nHost: test\r\nConnection: upgrade\r\nUpgrade: websocket\r\n\r\n", false},
		{"normal-xhttp", "splithttp", "POST /native/session/0 HTTP/1.1\r\nHost: test\r\nContent-Length: 0\r\n\r\n", false},
		{"connect-payload", "websocket", "CONNECT endpoint:443 HTTP/1.1\r\n\r\n", true},
		{"absolute-uri-payload", "", "GET http://example.test/ HTTP/1.1\r\nHost: example.test\r\n\r\n", true},
		{"custom-payload", "", "CUSTOM opaque preface\r\n\r\n", true},
		{"ws-looking-payload", "websocket", strings.Replace(ws, "/native", "/payload", 1), true},
		{"explicit-ws-payload", "websocket", strings.Replace(ws, "Host: test", "Host: test\r\nX-TDZ-Payload: 1", 1), true},
	} {
		for _, fragmented := range []bool{false, true} {
			t.Run(tc.name+"/split="+string(rune('0'+boolInt(fragmented))), func(t *testing.T) {
				server, client := net.Pipe()
				defer server.Close()
				defer client.Close()
				client.SetDeadline(time.Now().Add(2 * time.Second))
				pending := []byte("native bytes\x00\xff\r\n\r\nsecond header stays")
				want := pending
				if !tc.payload {
					want = append([]byte(tc.input), pending...)
				}
				done := make(chan error, 1)
				go func() {
					conn, err := tdzAcceptAutoPayload(server, tc.network, "/native", time.Second)
					if err == nil {
						data := make([]byte, len(want))
						_, err = io.ReadFull(conn, data)
						if err == nil && !bytes.Equal(data, want) {
							err = io.ErrUnexpectedEOF
						}
						if err == nil && (conn.LocalAddr() != server.LocalAddr() || conn.RemoteAddr() != server.RemoteAddr()) {
							err = io.ErrUnexpectedEOF
						}
						if err == nil {
							_, err = conn.Write(data)
						}
					}
					done <- err
				}()
				go func() {
					if fragmented {
						for _, b := range []byte(tc.input) {
							if _, err := client.Write([]byte{b}); err != nil {
								return
							}
						}
						client.Write(pending)
					} else {
						client.Write(append([]byte(tc.input), pending...))
					}
				}()
				response := want
				if tc.payload {
					response = append([]byte(tdzPayloadResponse), want...)
				}
				got := make([]byte, len(response))
				if _, err := io.ReadFull(client, got); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, response) {
					t.Fatalf("handshake/stream changed: %q", got)
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
