package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
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
)

// Native DNS must stay unlimited and absent from IP stats even with limit=1.
// Ordinary traffic to the SAME DNS-enabled inbound still uses real IP slots.
func TestNativeSlowDNSIPLimitExemption(t *testing.T) {
	core, dnstt := os.Getenv("TDZ_TEST_XRAY"), os.Getenv("TDZ_TEST_DNSTT_CLIENT")
	if core == "" || dnstt == "" {
		t.Skip("requires TDZ_TEST_XRAY and TDZ_TEST_DNSTT_CLIENT")
	}
	for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks"} {
		for _, network := range []string{"tcp", "ws", "httpupgrade", "grpc", "xhttp"} {
			t.Run(protocol+"/"+network, func(t *testing.T) {
				dir := t.TempDir()
				port := func() int {
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					if err != nil {
						t.Fatal(err)
					}
					defer listener.Close()
					return listener.Addr().(*net.TCPAddr).Port
				}
				echo, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer echo.Close()
				go func() {
					for {
						conn, err := echo.Accept()
						if err != nil {
							return
						}
						go func() { defer conn.Close(); io.Copy(conn, conn) }()
					}
				}()
				udp, err := net.ListenPacket("udp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				dnsAddress := udp.LocalAddr().String()
				udp.Close()
				serverPort, secondaryPort, apiPort := port(), port(), port()
				key, err := ecdh.X25519().GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				runtimePath, limitsPath := filepath.Join(dir, "slowdns.json"), filepath.Join(dir, "limits.json")
				paths := []string{"/dns", "/other-dns"}
				if network == "xhttp" {
					paths = []string{"/dns/", "/other-dns/"}
				}
				routes := []SlowDNSRoute{{Domain: "i1.t.example.test", Port: serverPort, Network: SlowDNSNetwork(network), Path: paths[0], ServiceName: "dns", AcceptPayloads: true}, {Domain: "i2.t.example.test", Port: secondaryPort, Network: SlowDNSNetwork(network), Path: paths[1], ServiceName: "other-dns", AcceptPayloads: true}}
				runtime := SlowDNSRuntimeConfig{Enabled: true, Listen: dnsAddress, HostDomain: "ns.example.test", NSDomain: "t.example.test", MTU: 1232, PrivateKey: hex.EncodeToString(key.Bytes()), Routes: routes}
				data, _ := json.Marshal(runtime)
				if err := os.WriteFile(runtimePath, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(limitsPath, []byte(`{"limited":1,"other":1}`), 0600); err != nil {
					t.Fatal(err)
				}
				start := func(name string, command *exec.Cmd) *exec.Cmd {
					logPath := filepath.Join(dir, name+".log")
					log, err := os.Create(logPath)
					if err != nil {
						t.Fatal(err)
					}
					command.Stdout, command.Stderr, command.Dir = log, log, dir
					if err := command.Start(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						command.Process.Kill()
						command.Wait()
						log.Close()
						if t.Failed() {
							data, _ := os.ReadFile(logPath)
							t.Logf("%s: %s", name, data)
						}
					})
					return command
				}
				startCore := func(name string, cfg any, server bool) {
					path := filepath.Join(dir, name+".json")
					data, _ := json.Marshal(cfg)
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
					command := exec.Command(core, "run", "-config", path)
					command.Env = append(os.Environ(), "TDZ_XRAY_SLOWDNS=", "TDZ_XRAY_IP_LIMITS=")
					if server {
						command.Env = append(command.Env, "TDZ_XRAY_SLOWDNS="+runtimePath, "TDZ_XRAY_IP_LIMITS="+limitsPath)
					}
					start(name, command)
				}
				stream := func(secondary bool) map[string]any {
					index := 0
					service := "dns"
					if secondary {
						index, service = 1, "other-dns"
					}
					settings := map[string]any{"network": network, "security": "none", "acceptPayloads": true}
					switch network {
					case "ws":
						settings["wsSettings"] = map[string]any{"path": paths[index], "headers": map[string]string{"X-Forwarded-For": "198.51.100.99"}}
					case "httpupgrade":
						settings["httpupgradeSettings"] = map[string]any{"path": paths[index]}
					case "grpc":
						settings["grpcSettings"] = map[string]any{"serviceName": service}
					case "xhttp":
						settings["xhttpSettings"] = map[string]any{"path": paths[index], "mode": "packet-up"}
					}
					return settings
				}
				id := "11111111-1111-4111-8111-111111111111"
				serverSettings := map[string]any{"clients": []any{map[string]any{"id": id, "password": "dns-password", "email": "limited"}}, "decryption": "none"}
				secondaryID := "22222222-2222-4222-8222-222222222222"
				secondarySettings := map[string]any{"clients": []any{map[string]any{"id": secondaryID, "password": "other-password", "email": "other"}}, "decryption": "none"}
				if protocol == "shadowsocks" {
					serverSettings = map[string]any{"method": "chacha20-ietf-poly1305", "password": "dns-password", "email": "limited", "network": "tcp"}
				}
				if protocol == "shadowsocks" {
					secondarySettings = map[string]any{"method": "chacha20-ietf-poly1305", "password": "other-password", "email": "other", "network": "tcp"}
				}
				startCore("server", map[string]any{
					"log": map[string]any{"loglevel": "warning"}, "stats": map[string]any{},
					"api":       map[string]any{"tag": "api", "services": []string{"StatsService"}},
					"policy":    map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserOnline": true}}},
					"inbounds":  []any{map[string]any{"tag": "api", "listen": "127.0.0.1", "port": apiPort, "protocol": "tunnel", "settings": map[string]any{"address": "127.0.0.1"}}, map[string]any{"tag": "dns", "listen": "127.0.0.1", "port": serverPort, "protocol": protocol, "settings": serverSettings, "streamSettings": stream(false)}, map[string]any{"tag": "other-dns", "listen": "127.0.0.1", "port": secondaryPort, "protocol": protocol, "settings": secondarySettings, "streamSettings": stream(true)}},
					"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{"ipsBlocked": []string{}}}},
					"routing":   map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
				}, true)
				socks := []int{port(), port(), port()}
				if protocol == "vless" && network == "tcp" {
					socks = append(socks, port(), port())
				}
				dnsClients := make([]*exec.Cmd, 3)
				clientDomain := "t.example.test"
				for index := range socks {
					frontend := serverPort
					if index < 3 {
						frontend = port()
						dnsClients[index] = start("dnstt"+strconv.Itoa(index), exec.Command(dnstt, "-udp", dnsAddress, "-pubkey", hex.EncodeToString(key.PublicKey().Bytes()), clientDomain, net.JoinHostPort("127.0.0.1", strconv.Itoa(frontend))))
					}
					endpoint := map[string]any{"address": "127.0.0.1", "port": frontend, "users": []any{map[string]any{"id": id, "encryption": "none", "security": "auto"}}}
					password := "dns-password"
					if index == 2 {
						endpoint["users"] = []any{map[string]any{"id": secondaryID, "encryption": "none", "security": "auto"}}
						password = "other-password"
					}
					settings := map[string]any{"vnext": []any{endpoint}}
					if protocol == "trojan" || protocol == "shadowsocks" {
						endpoint["password"], endpoint["method"] = password, "chacha20-ietf-poly1305"
						settings = map[string]any{"servers": []any{endpoint}}
					}
					outbound := map[string]any{"protocol": protocol, "settings": settings, "streamSettings": stream(index == 2)}
					if index >= 3 {
						outbound["sendThrough"] = "192.0.2." + strconv.Itoa(index-1)
					}
					startCore("client"+strconv.Itoa(index), map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": socks[index], "protocol": "socks", "settings": map[string]any{"auth": "noauth"}}}, "outbounds": []any{outbound}}, false)
				}
				dial := func(socksPort int) (net.Conn, error) {
					conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort)), time.Second)
					if err != nil {
						return nil, err
					}
					fail := func(err error) (net.Conn, error) { conn.Close(); return nil, err }
					conn.SetDeadline(time.Now().Add(6 * time.Second))
					if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
						return fail(err)
					}
					var greeting [2]byte
					if _, err := io.ReadFull(conn, greeting[:]); err != nil {
						return fail(err)
					}
					request := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 0}
					binarypkgPutPort(request[8:], echo.Addr().(*net.TCPAddr).Port)
					if _, err := conn.Write(request); err != nil {
						return fail(err)
					}
					var reply [10]byte
					if _, err := io.ReadFull(conn, reply[:]); err != nil {
						return fail(err)
					}
					if _, err := conn.Write([]byte("ping")); err != nil {
						return fail(err)
					}
					var payload [4]byte
					if _, err := io.ReadFull(conn, payload[:]); err != nil {
						return fail(err)
					}
					if string(payload[:]) != "ping" {
						return fail(fmt.Errorf("bad echo %q", payload))
					}
					conn.SetDeadline(time.Time{})
					return conn, nil
				}
				waitDial := func(port int) net.Conn {
					deadline := time.Now().Add(20 * time.Second)
					for {
						conn, err := dial(port)
						if err == nil {
							return conn
						}
						if time.Now().After(deadline) {
							t.Fatal(err)
						}
						time.Sleep(100 * time.Millisecond)
					}
				}
				first := waitDial(socks[0])
				defer first.Close()
				shared := waitDial(socks[0])
				defer shared.Close()
				other := waitDial(socks[2])
				defer other.Close()
				second := waitDial(socks[1])
				defer second.Close()
				var api XrayAPI
				if err := api.Init(apiPort); err != nil {
					t.Fatal(err)
				}
				defer api.Close()
				ips, err := api.GetOnlineClientIPs()
				if err != nil || len(ips["limited"]) != 0 || len(ips["other"]) != 0 {
					t.Fatalf("DNSTT entered IP-limit counts/logs: %v (%v)", ips, err)
				}
				first.SetDeadline(time.Now().Add(6 * time.Second))
				if _, err := first.Write([]byte("live")); err != nil {
					t.Fatal(err)
				}
				var reply [4]byte
				if _, err := io.ReadFull(first, reply[:]); err != nil || string(reply[:]) != "live" {
					t.Fatal("the first DNS connection stopped after opening another", err)
				}
				if err := os.WriteFile(limitsPath+".new", []byte(`{"limited":2,"other":1}`), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(limitsPath+".new", limitsPath); err != nil {
					t.Fatal(err)
				}
				ips, err = api.GetOnlineClientIPs()
				if err != nil || len(ips["limited"]) != 0 {
					t.Fatalf("changing an IP limit counted DNS traffic: %v (%v)", ips, err)
				}
				if protocol == "vless" && network == "tcp" {
					if err := os.WriteFile(limitsPath, []byte(`{"limited":1,"other":1}`), 0600); err != nil {
						t.Fatal(err)
					}
					direct := waitDial(socks[3])
					defer direct.Close()
					if extra, err := dial(socks[4]); err == nil {
						extra.Close()
						t.Fatal("ordinary traffic bypassed limit=1 on a DNS-enabled inbound")
					}
					ips, err = api.GetOnlineClientIPs()
					if err != nil || len(ips["limited"]) != 1 || ips["limited"][0].IP != "192.0.2.2" {
						t.Fatalf("mixed traffic must count only the real source IP: %v (%v)", ips, err)
					}
					for _, conn := range []net.Conn{direct, first, second} {
						conn.SetDeadline(time.Now().Add(6 * time.Second))
						if _, err := conn.Write([]byte("safe")); err != nil {
							t.Fatal(err)
						}
						if _, err := io.ReadFull(conn, reply[:]); err != nil || string(reply[:]) != "safe" {
							t.Fatal("blocking another real IP disrupted admitted traffic", err)
						}
						conn.SetDeadline(time.Time{})
					}
					// Quiet but connected clients must survive the lease interval.
					time.Sleep(17 * time.Second)
					first.SetDeadline(time.Now().Add(6 * time.Second))
					if _, err := first.Write([]byte("idle")); err != nil {
						t.Fatal(err)
					}
					if _, err := io.ReadFull(first, reply[:]); err != nil || string(reply[:]) != "idle" {
						t.Fatal("a healthy idle DNS tunnel was expired", err)
					}
					// Disconnect without sending a DNS/KCP FIN, just like stopping
					// a phone app or losing its network. The replacement must not
					// inherit the old tunnel's occupied slot for two minutes.
					for _, client := range dnsClients {
						client.Process.Kill()
						client.Wait()
					}
					if err := os.WriteFile(limitsPath, []byte(`{"limited":1,"other":1}`), 0600); err != nil {
						t.Fatal(err)
					}
					previous := dnsClients[0]
					start("dnstt-reconnected", exec.Command(previous.Path, previous.Args[1:]...))
					reconnected := waitDial(socks[0])
					defer reconnected.Close()
					ips, err = api.GetOnlineClientIPs()
					if err != nil || len(ips["limited"]) != 1 || ips["limited"][0].IP != "192.0.2.2" {
						t.Fatalf("DNS reconnect changed normal IP slots: %v (%v)", ips, err)
					}
				}
				first.Close()
				shared.Close()
				second.Close()
			})
		}
	}
}
