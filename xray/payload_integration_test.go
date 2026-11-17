package xray

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestNativePayloadSinglePort(t *testing.T) {
	core := os.Getenv("TDZ_TEST_XRAY")
	if core == "" {
		t.Skip("set TDZ_TEST_XRAY for real-core payload tests")
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
	target := echo.Addr().(*net.TCPAddr).Port
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost"}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600)
	for _, security := range []string{"none", "tls"} {
		t.Run(security, func(t *testing.T) {
			listen := port()
			stream := map[string]any{"network": "tcp", "security": security, "tcpSettings": map[string]any{"injectPayload": true}}
			if security == "tls" {
				stream["tlsSettings"] = map[string]any{"certificates": []any{map[string]any{"certificateFile": certPath, "keyFile": keyPath}}}
			}
			cfg := map[string]any{"log": map[string]any{"loglevel": "warning"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": listen, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "email": "payload-client"}}}, "streamSettings": stream}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"ipsBlocked": []string{}}}}}
			body, _ := json.Marshal(cfg)
			file := filepath.Join(dir, security+".json")
			os.WriteFile(file, body, 0600)
			log, err := os.Create(filepath.Join(dir, security+".log"))
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(core, "run", "-config", file)
			cmd.Stdout = log
			cmd.Stderr = log
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cmd.Process.Kill()
				cmd.Wait()
				log.Close()
				if t.Failed() {
					data, _ := os.ReadFile(filepath.Join(dir, security+".log"))
					t.Log(string(data))
				}
			})
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(listen))
			deadline := time.Now().Add(8 * time.Second)
			for {
				c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
				if err == nil {
					c.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatal(err)
				}
				time.Sleep(30 * time.Millisecond)
			}
			for _, preface := range []string{"\r\n\r\n", "GET / HTTP/1.1\r\nHost: test\r\n\r\n", "CONNECT test:443 HTTP/1.1\r\n\r\n", "CUSTOM opaque bytes\r\n\r\n", "GET / HTTP/1.1 extra\r\n\r\n", "CONNECT test:443\r\n\r\n", "GET / HTTP/1.1\r\nHost: test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"} {
				c, err := net.Dial("tcp", address)
				if err != nil {
					t.Fatal(err)
				}
				c.SetDeadline(time.Now().Add(3 * time.Second))
				c.Write([]byte(preface))
				want := "HTTP/1.1 101 <b><font color=\"red\" size=\"7\">Script By:</font> <font color=\"#0057B7\" size=\"7\">tuhinbro.com</font></b>\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
				header := make([]byte, len(want))
				if _, err := io.ReadFull(c, header); err != nil || string(header) != want {
					t.Fatalf("TDZ response %q %v", header, err)
				}
				if security == "tls" {
					tlsConn := tls.Client(c, &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
					if err := tlsConn.Handshake(); err != nil {
						t.Fatal(err)
					}
					c = tlsConn
				}
				// Raw VLESS request to the local echo service; successful response
				// proves the preface and coalesced first data reached auth intact.
				request := []byte{0, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x41, 0x11, 0x81, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0, 1, 0, 0, 1, 127, 0, 0, 1}
				binary.BigEndian.PutUint16(request[19:21], uint16(target))
				payload := []byte("payload-single-port-data")
				c.Write(append(request, payload...))
				response := make([]byte, 2+len(payload))
				_, err = io.ReadFull(c, response)
				c.Close()
				if err != nil || !bytes.Equal(response[2:], payload) {
					t.Fatalf("%s %q: %x %v", security, preface, response, err)
				}
			}
		})
	}
}
