package xray

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNativePayloadTransportMatrix(t *testing.T) {
	core := os.Getenv("TDZ_TEST_XRAY")
	if core == "" {
		t.Skip("requires TDZ_TEST_XRAY")
	}
	certDir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(der)
	pin := hex.EncodeToString(digest[:])
	certPath, keyPath := filepath.Join(certDir, "cert.pem"), filepath.Join(certDir, "key.pem")
	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600)
	for _, security := range []string{"none", "tls"} {
		for _, network := range []string{"tcp", "ws", "httpupgrade", "grpc", "xhttp"} {
			for _, mode := range []string{"off/direct", "on/direct", "on/payload"} {
				enabled, inject := mode != "off/direct", mode == "on/payload"
				t.Run(network+"/"+security+"/"+mode, func(t *testing.T) {
					dir := t.TempDir()
					freePort := func() int {
						l, e := net.Listen("tcp", "127.0.0.1:0")
						if e != nil {
							t.Fatal(e)
						}
						defer l.Close()
						return l.Addr().(*net.TCPAddr).Port
					}
					serverPort, socksPort := freePort(), freePort()
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
							go func() { defer c.Close(); io.Copy(c, c) }()
						}
					}()
					settings := func() map[string]any {
						s := map[string]any{"network": network, "security": security}
						if security == "tls" {
							s["tlsSettings"] = map[string]any{"serverName": "localhost", "pinnedPeerCertSha256": pin}
						}
						switch network {
						case "ws":
							s["wsSettings"] = map[string]any{"path": "/native"}
						case "httpupgrade":
							s["httpupgradeSettings"] = map[string]any{"path": "/native"}
						case "grpc":
							s["grpcSettings"] = map[string]any{"serviceName": "native"}
						case "xhttp":
							s["xhttpSettings"] = map[string]any{"path": "/native/", "mode": "packet-up"}
						}
						return s
					}
					start := func(name string, cfg any) {
						data, _ := json.Marshal(cfg)
						p := filepath.Join(dir, name+".json")
						os.WriteFile(p, data, 0600)
						f, _ := os.Create(filepath.Join(dir, name+".log"))
						cmd := exec.Command(core, "run", "-config", p)
						cmd.Stdout = f
						cmd.Stderr = f
						if e := cmd.Start(); e != nil {
							t.Fatal(e)
						}
						t.Cleanup(func() {
							cmd.Process.Kill()
							cmd.Wait()
							f.Close()
							if t.Failed() {
								b, _ := os.ReadFile(filepath.Join(dir, name+".log"))
								t.Log(string(b))
							}
						})
					}
					serverStream := settings()
					serverStream["acceptPayloads"] = enabled
					if security == "tls" {
						tlsSettings := map[string]any{"certificates": []any{map[string]any{"certificateFile": certPath, "keyFile": keyPath}}}
						if network == "grpc" || network == "xhttp" {
							tlsSettings["alpn"] = []string{"h2", "http/1.1"}
						}
						serverStream["tlsSettings"] = tlsSettings
					}
					id := "11111111-1111-4111-8111-111111111111"
					start("server", map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": serverPort, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": id}}}, "streamSettings": serverStream}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"ipsBlocked": []string{}}}}})
					// Real client transport passes through a test-only preface injector.
					gate, e := net.Listen("tcp", "127.0.0.1:0")
					if e != nil {
						t.Fatal(e)
					}
					defer gate.Close()
					go func() {
						for {
							c, e := gate.Accept()
							if e != nil {
								return
							}
							go func() {
								defer c.Close()
								out, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(serverPort)), time.Second)
								if e != nil {
									return
								}
								defer out.Close()
								out.SetDeadline(time.Now().Add(5 * time.Second))
								r := bufio.NewReader(out)
								if inject {
									io.WriteString(out, "CONNECT endpoint:443 HTTP/1.1\r\nHost: example.test\r\n\r\n")
									var response strings.Builder
									for {
										line, e := r.ReadString('\n')
										if e != nil {
											return
										}
										response.WriteString(line)
										if line == "\r\n" {
											break
										}
									}
									if response.String() != "HTTP/1.1 101 <b><font color=\"red\" size=\"7\">Script By:</font> <font color=\"#0057B7\" size=\"7\">tuhinbro.com</font></b>\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n" {
										return
									}
								}
								out.SetDeadline(time.Time{})
								done := make(chan bool, 2)
								go func() { io.Copy(out, c); done <- true }()
								go func() { io.Copy(c, r); done <- true }()
								<-done
							}()
						}
					}()
					clientStream := settings()
					start("client", map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": socksPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}}}, "outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": gate.Addr().(*net.TCPAddr).Port, "users": []any{map[string]any{"id": id, "encryption": "none"}}}}}, "streamSettings": clientStream}}})
					dial := func() error {
						c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort)), time.Second)
						if e != nil {
							return e
						}
						defer c.Close()
						c.SetDeadline(time.Now().Add(4 * time.Second))
						c.Write([]byte{5, 1, 0})
						reply := make([]byte, 2)
						if _, e = io.ReadFull(c, reply); e != nil {
							return e
						}
						req := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 0}
						binary.BigEndian.PutUint16(req[8:], uint16(echo.Addr().(*net.TCPAddr).Port))
						c.Write(req)
						reply = make([]byte, 10)
						if _, e = io.ReadFull(c, reply); e != nil {
							return e
						}
						data := bytes.Repeat([]byte("transport-ok"), 1000)
						c.Write(data)
						got := make([]byte, len(data))
						if _, e = io.ReadFull(c, got); e != nil {
							return e
						}
						if !bytes.Equal(got, data) {
							return io.ErrUnexpectedEOF
						}
						return nil
					}
					deadline := time.Now().Add(12 * time.Second)
					for {
						e := dial()
						if e == nil {
							break
						}
						if time.Now().After(deadline) {
							t.Fatal(e)
						}
						time.Sleep(100 * time.Millisecond)
					}
				})
			}
		}
	}
}
