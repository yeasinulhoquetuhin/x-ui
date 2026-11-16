package xray

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Compare the original shipped Python proxy against the native listener using
// an identical backend and byte stream. This catches accidental HTTP parsing,
// CONNECT status changes, second-header removal and WebSocket frame rewriting.
func TestNativePayloadMatchesOriginalProxy(t *testing.T) {
	core, proxy := os.Getenv("TDZ_TEST_XRAY"), os.Getenv("TDZ_TEST_PROXY")
	if core == "" || proxy == "" {
		t.Skip("set TDZ_TEST_XRAY and TDZ_TEST_PROXY")
	}
	dir := t.TempDir()
	port := func() int {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	nativePort, legacyPort := port(), port()
	backend := echo.Addr().(*net.TCPAddr).Port
	cfg := map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": nativePort, "protocol": "tunnel", "settings": map[string]any{"address": "127.0.0.1", "port": backend, "network": "tcp"}, "streamSettings": map[string]any{"network": "tcp", "acceptPayloads": true}}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"ipsBlocked": []string{}}}}}
	data, _ := json.Marshal(cfg)
	configPath := filepath.Join(dir, "native.json")
	os.WriteFile(configPath, data, 0600)
	base := filepath.Join(dir, "proxy")
	os.MkdirAll(filepath.Join(base, "instances"), 0700)
	os.WriteFile(filepath.Join(base, "instances/1.conf"), []byte(fmt.Sprintf("PUBLIC_PORT=%d\nBACKEND_PORT=%d\n", legacyPort, backend)), 0600)
	start := func(name string, cmd *exec.Cmd) {
		f, err := os.Create(filepath.Join(dir, name+".log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout = f
		cmd.Stderr = f
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cmd.Process.Kill()
			cmd.Wait()
			f.Close()
			if t.Failed() {
				b, _ := os.ReadFile(filepath.Join(dir, name+".log"))
				t.Logf("%s: %s", name, b)
			}
		})
	}
	start("native", exec.Command(core, "run", "-config", configPath))
	old := exec.Command("bash", proxy, "--run-proxy", "1")
	old.Env = append(os.Environ(), "XUI_PROXY_BASE_DIR="+base, "XUI_PROXY_IP_DIR="+filepath.Join(dir, "client-ips"))
	start("legacy", old)
	for _, p := range []int{legacyPort, nativePort} {
		deadline := time.Now().Add(5 * time.Second)
		for {
			c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)), time.Second)
			if err == nil {
				c.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
	response := []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	headers := [][]byte{
		[]byte("GET / HTTP/1.1\r\nHost: example\r\n\r\n"),
		[]byte("CONNECT arbitrary:443 HTTP/1.1\r\n\r\n"),
		[]byte("GET / HTTP/1.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: invalid-key-is-opaque\r\n\r\n"),
		[]byte("X-Online-Host: example\r\n\r\n"),
		[]byte("no-http-syntax\r\n\r\n"),
		[]byte{0, 255, 128, 13, 10, 13, 10},
		[]byte("\r\n\r\n"),
		append(bytes.Repeat([]byte{'A'}, 64000), []byte("\r\n\r\n")...),
	}
	pending := append([]byte("GET /native HTTP/1.1\r\nHost: native\r\nUpgrade: websocket\r\n\r\n"), bytes.Repeat([]byte{0, 255, 128, 13, 10, 13, 10}, 1200)...)
	for i, header := range headers {
		for _, fragmented := range []bool{false, true} {
			t.Run(fmt.Sprintf("header-%d/split=%v", i, fragmented), func(t *testing.T) {
				run := func(p int) []byte {
					c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					c.SetDeadline(time.Now().Add(5 * time.Second))
					if fragmented {
						for off := 0; off < len(header); off += 7 {
							end := min(off+7, len(header))
							if _, err := c.Write(header[off:end]); err != nil {
								t.Fatal(err)
							}
						}
					} else {
						if _, err := c.Write(append(append([]byte{}, header...), pending...)); err != nil {
							t.Fatal(err)
						}
					}
					got := make([]byte, len(response))
					if _, err := io.ReadFull(c, got); err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(got, response) {
						t.Fatalf("port %d changed TDZ response: %q", p, got)
					}
					if fragmented {
						if _, err := c.Write(pending); err != nil {
							t.Fatal(err)
						}
					}
					body := make([]byte, len(pending))
					if _, err := io.ReadFull(c, body); err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(body, pending) {
						t.Fatal("post-header bytes were consumed or changed")
					}
					// Once established, subsequent headers are ordinary tunnel data.
					follow := []byte("CONNECT second HTTP/1.1\r\n\r\ntrailing\x00\xff")
					c.Write(follow)
					again := make([]byte, len(follow))
					if _, err := io.ReadFull(c, again); err != nil {
						t.Fatal(err)
					}
					return append(append(got, body...), again...)
				}
				if !bytes.Equal(run(legacyPort), run(nativePort)) {
					t.Fatal("native stream differs from original TDZ proxy")
				}
			})
		}
	}
}

// The preface may look exactly like a real WS handshake. The old relay always
// strips it, then the backend receives the second/native handshake unchanged.
func TestNativePayloadLegacyWebSocketHandshake(t *testing.T) {
	core := os.Getenv("TDZ_TEST_XRAY")
	if core == "" {
		t.Skip("requires TDZ_TEST_XRAY")
	}
	dir := t.TempDir()
	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserve.Addr().(*net.TCPAddr).Port
	reserve.Close()
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	cfg := map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": port, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111"}}}, "streamSettings": map[string]any{"network": "ws", "acceptPayloads": true, "wsSettings": map[string]any{"path": "/"}}}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"ipsBlocked": []string{}}}}}
	data, _ := json.Marshal(cfg)
	path := filepath.Join(dir, "ws.json")
	os.WriteFile(path, data, 0600)
	log, _ := os.Create(filepath.Join(dir, "core.log"))
	cmd := exec.Command(core, "run", "-config", path)
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait(); log.Close() }()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	preface := "GET / HTTP/1.1\r\nHost: example.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second, NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := net.DialTimeout(network, addr, time.Second)
		if err != nil {
			return nil, err
		}
		c.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err = c.Write([]byte(preface)); err != nil {
			c.Close()
			return nil, err
		}
		response := make([]byte, len("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))
		if _, err = io.ReadFull(c, response); err != nil {
			c.Close()
			return nil, err
		}
		c.SetDeadline(time.Time{})
		return c, nil
	}}
	ws, _, err := dialer.Dial("ws://"+address+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	request := []byte{0, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x41, 0x11, 0x81, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0, 1, 0, 0, 1, 127, 0, 0, 1}
	binary.BigEndian.PutUint16(request[19:21], uint16(echo.Addr().(*net.TCPAddr).Port))
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := ws.WriteMessage(websocket.BinaryMessage, append(request, []byte("ws-through-tdz")...)); err != nil {
		t.Fatal(err)
	}
	var received []byte
	for len(received) < 16 {
		_, b, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		received = append(received, b...)
	}
	if !bytes.Equal(received, append([]byte{0, 0}, []byte("ws-through-tdz")...)) {
		t.Fatal("WS stream differs", received)
	}
}
