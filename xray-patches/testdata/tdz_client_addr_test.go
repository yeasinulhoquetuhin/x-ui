package internet

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

func TestTDZForwardedClientAddress(t *testing.T) {
	t.Setenv("TDZ_XRAY_TRUSTED_PROXIES", "127.0.0.0/8,::1,10.0.0.0/24,2001:db8:1::1")
	for _, tc := range []struct{ peer, xff, real, want string }{
		{"127.0.0.1", "192.0.2.1", "", "192.0.2.1"},
		{"::1", "::ffff:192.0.2.1", "", "192.0.2.1"},
		{"127.0.0.1", "192.0.2.99, 192.0.2.1", "", "192.0.2.1"},
		{"10.0.0.2", "192.0.2.99, 192.0.2.1, 10.0.0.3", "", "192.0.2.1"},
		{"2001:db8:1::1", "2001:db8:2::1", "", "2001:db8:2::1"},
		{"192.0.2.2", "192.0.2.1", "192.0.2.1", "192.0.2.2"},
		{"127.0.0.1", "", "192.0.2.1", "192.0.2.1"},
		{"127.0.0.1", "not-an-ip", "192.0.2.1", "127.0.0.1"},
		{"127.0.0.1", "", "", "127.0.0.1"},
		{"127.0.0.1", "127.0.0.2", "", "127.0.0.1"},
	} {
		peer := &net.TCPAddr{IP: net.ParseIP(tc.peer), Port: 12345}
		headers := http.Header{"X-Forwarded-For": {tc.xff}, "X-Real-Ip": {tc.real}}
		got := TDZClientAddr(peer, nil, headers, nil).(*net.TCPAddr).IP.String()
		if got != tc.want {
			t.Errorf("peer=%s xff=%q real=%q got=%s want=%s", tc.peer, tc.xff, tc.real, got, tc.want)
		}
	}
}

func TestTDZLocalRelayDoesNotTrustClientHeadersByDefault(t *testing.T) {
	t.Setenv("TDZ_XRAY_TRUSTED_PROXIES", "")
	headers := http.Header{"X-Forwarded-For": {"192.0.2.1"}, "X-Real-Ip": {"192.0.2.1"}}
	peer := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}
	if got := TDZClientAddr(peer, nil, headers, nil); got.String() != peer.String() {
		t.Fatalf("unconfigured relay accepted client headers: %v", got)
	}
}

func TestTDZLocalProxyIPValidatesLiveSocket(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("requires Linux procfs")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TDZ_XRAY_PROXY_IP_DIR", directory)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	raw, err := client.(syscall.Conn).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	fd := 0
	if err := raw.Control(func(number uintptr) { fd = int(number) }); err != nil {
		t.Fatal(err)
	}
	socket, err := os.Readlink(filepath.Join("/proc/self/fd", strconv.Itoa(fd)))
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(client.LocalAddr().String())
	_, backend, _ := net.SplitHostPort(client.RemoteAddr().String())
	path := filepath.Join(directory, backend+"-"+port+".json")
	data, _ := json.Marshal(map[string]any{"ip": "192.0.2.1", "pid": os.Getpid(), "fd": fd, "socket": socket})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if got, ok := TDZLocalProxyIP(server.RemoteAddr().String(), server.LocalAddr().String()); !ok || got != "192.0.2.1" {
		t.Fatalf("live mapping rejected: %s %v", got, ok)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := TDZLocalProxyIP(server.RemoteAddr().String(), server.LocalAddr().String()); ok {
		t.Fatal("public record accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	if _, ok := TDZLocalProxyIP(server.RemoteAddr().String(), server.LocalAddr().String()); ok {
		t.Fatal("stale socket record accepted")
	}
}
