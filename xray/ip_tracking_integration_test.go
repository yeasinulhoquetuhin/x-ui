package xray

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Uses real core processes, two source IPs and a long-lived echo stream. This
// catches scan-only limits, transport-specific bypasses and stale MUX leases.
func TestNativeIPLimitTransports(t *testing.T) {
	binary := os.Getenv("TDZ_TEST_XRAY")
	if binary == "" {
		t.Skip("set TDZ_TEST_XRAY to run real-core integration tests")
	}
	for _, tc := range []struct {
		protocol, network string
		mux               bool
	}{
		{"vless", "tcp", false}, {"vless", "tcp", true}, {"vless", "ws", false},
		{"vless", "grpc", false}, {"vless", "xhttp", false}, {"vless", "httpupgrade", false},
		{"vless", "kcp", false}, {"vmess", "tcp", false}, {"vmess", "ws", true},
		{"trojan", "tcp", false}, {"shadowsocks", "tcp", false},
	} {
		t.Run(tc.protocol+"/"+tc.network+"/mux="+strconv.FormatBool(tc.mux), func(t *testing.T) {
			dir := t.TempDir()
			port := func() int {
				l, e := net.Listen("tcp", "127.0.0.1:0")
				if e != nil {
					t.Fatal(e)
				}
				defer l.Close()
				return l.Addr().(*net.TCPAddr).Port
			}
			echo, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer echo.Close()
			go func() {
				for {
					c, e := echo.Accept()
					if e != nil {
						return
					}
					go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
				}
			}()
			targetPort := echo.Addr().(*net.TCPAddr).Port
			serverPort, apiPort, socks1, socks2 := port(), port(), port(), port()
			stream := func() map[string]any {
				s := map[string]any{"network": tc.network, "security": "none"}
				if tc.network == "grpc" {
					s["grpcSettings"] = map[string]any{"serviceName": "test"}
				}
				if tc.network == "xhttp" {
					s["xhttpSettings"] = map[string]any{"path": "/test", "mode": "auto"}
				}
				if tc.network == "ws" {
					s["wsSettings"] = map[string]any{"path": "/test"}
				}
				if tc.network == "httpupgrade" {
					s["httpupgradeSettings"] = map[string]any{"path": "/test"}
				}
				return s
			}
			user := map[string]any{"id": "a92f8a44-a1ae-4113-9071-48f8fd41959d", "email": "limited", "password": "test-password", "security": "auto", "encryption": "none"}
			serverUser := map[string]any{"id": user["id"], "email": "limited", "password": "test-password"}
			serverSettings := map[string]any{"clients": []any{serverUser}, "decryption": "none"}
			if tc.protocol == "shadowsocks" {
				serverSettings = map[string]any{"method": "chacha20-ietf-poly1305", "password": "test-password", "email": "limited", "network": "tcp,udp"}
			}
			server := map[string]any{
				"log":   map[string]any{"loglevel": "debug", "access": "none"},
				"api":   map[string]any{"tag": "api", "services": []string{"StatsService"}},
				"stats": map[string]any{}, "policy": map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserOnline": true}}},
				"inbounds": []any{
					map[string]any{"tag": "api", "listen": "127.0.0.1", "port": apiPort, "protocol": "tunnel", "settings": map[string]any{"address": "127.0.0.1"}},
					map[string]any{"tag": "test", "listen": "127.0.0.1", "port": serverPort, "protocol": tc.protocol, "settings": serverSettings, "streamSettings": stream()},
				}, "outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{"ipsBlocked": []string{}}}},
				"routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
			}
			start := func(name string, cfg any, limitPath string) {
				data, _ := json.Marshal(cfg)
				path := filepath.Join(dir, name+".json")
				if e := os.WriteFile(path, data, 0600); e != nil {
					t.Fatal(e)
				}
				log, e := os.Create(filepath.Join(dir, name+".log"))
				if e != nil {
					t.Fatal(e)
				}
				cmd := exec.Command(binary, "run", "-config", path)
				cmd.Dir = dir
				cmd.Stdout, cmd.Stderr = log, log
				if limitPath != "" {
					cmd.Env = append(os.Environ(), "TDZ_XRAY_IP_LIMITS="+limitPath)
				}
				if e := cmd.Start(); e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					_ = log.Close()
					if t.Failed() {
						body, _ := os.ReadFile(filepath.Join(dir, name+".log"))
						t.Logf("%s: %s", name, body)
					}
				})
			}
			limits := filepath.Join(dir, "limits.json")
			_ = os.WriteFile(limits, []byte(`{"limited":1}`), 0600)
			start("server", server, limits)
			for i, socks := range []int{socks1, socks2} {
				endpoint := map[string]any{"address": "127.0.0.1", "port": serverPort, "users": []any{user}}
				settings := map[string]any{"vnext": []any{endpoint}}
				if tc.protocol == "trojan" || tc.protocol == "shadowsocks" {
					endpoint["password"] = "test-password"
					endpoint["method"] = "chacha20-ietf-poly1305"
					settings = map[string]any{"servers": []any{endpoint}}
				}
				client := map[string]any{"log": map[string]any{"loglevel": "debug", "access": "none"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": socks, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}}}, "outbounds": []any{map[string]any{"protocol": tc.protocol, "sendThrough": "127.0.0." + strconv.Itoa(i+2), "settings": settings, "streamSettings": stream(), "mux": map[string]any{"enabled": tc.mux, "concurrency": 8}}}}
				start("client"+strconv.Itoa(i), client, "")
			}
			var api XrayAPI
			if e := api.Init(apiPort); e != nil {
				t.Fatal(e)
			}
			defer api.Close()
			dial := func(socks int) (net.Conn, error) {
				c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(socks)), time.Second)
				if e != nil {
					return nil, e
				}
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				fail := func(e error) (net.Conn, error) { _ = c.Close(); return nil, e }
				if _, e = c.Write([]byte{5, 1, 0}); e != nil {
					return fail(e)
				}
				var response [2]byte
				if _, e = io.ReadFull(c, response[:]); e != nil {
					return fail(e)
				}
				request := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 0}
				binarypkgPutPort(request[8:], targetPort)
				if _, e = c.Write(request); e != nil {
					return fail(e)
				}
				var reply [10]byte
				if _, e = io.ReadFull(c, reply[:]); e != nil {
					return fail(e)
				}
				if _, e = c.Write([]byte("ping")); e != nil {
					return fail(e)
				}
				var payload [4]byte
				if _, e = io.ReadFull(c, payload[:]); e != nil {
					return fail(e)
				}
				_ = c.SetDeadline(time.Time{})
				return c, nil
			}
			waitDial := func(socks int) net.Conn {
				deadline := time.Now().Add(12 * time.Second)
				for {
					c, e := dial(socks)
					if e == nil {
						return c
					}
					if time.Now().After(deadline) {
						t.Fatal(e)
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
			first := waitDial(socks1)
			defer first.Close()
			if extra, e := dial(socks2); e == nil {
				extra.Close()
				t.Fatal("second source IP bypassed limit=1")
			}
			ips, e := api.GetOnlineClientIPs()
			if e != nil {
				t.Fatal(e)
			}
			if len(ips["limited"]) != 1 || ips["limited"][0].IP != "127.0.0.2" {
				t.Fatalf("wrong online IPs: %v", ips)
			}
			_ = first.Close()
			second := waitDial(socks2)
			_ = second.Close()
			deadline := time.Now().Add(5 * time.Second)
			for {
				ips, e := api.GetOnlineClientIPs()
				if e == nil && len(ips["limited"]) == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("disconnected IPs retained: %v (%v)", ips, e)
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}

func binarypkgPutPort(buf []byte, port int) { binary.BigEndian.PutUint16(buf, uint16(port)) }
