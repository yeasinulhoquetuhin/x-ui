package internet

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TDZLocalProxyIP validates a record against the still-open proxy-side socket.
// Stale files, reused ports, exited processes and public/writable records cannot
// assign an IP. This works without changing payload, TCP, WS or TLS bytes.
func TDZLocalProxyIP(remote, local string) (string, bool) {
	host, port, err := net.SplitHostPort(remote)
	if err != nil {
		return "", false
	}
	address, err := netip.ParseAddr(host)
	if err != nil || !address.IsLoopback() {
		return "", false
	}
	_, backend, err := net.SplitHostPort(local)
	if err != nil {
		return "", false
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return "", false
	}
	if _, err := strconv.ParseUint(backend, 10, 16); err != nil {
		return "", false
	}
	directory := os.Getenv("TDZ_XRAY_PROXY_IP_DIR")
	if directory == "" {
		directory = "/run/tdzp-client-ips"
	}
	if info, err := os.Lstat(directory); err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", false
	}
	path := filepath.Join(directory, backend+"-"+port+".json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var record struct {
		IP     string `json:"ip"`
		PID    int    `json:"pid"`
		FD     int    `json:"fd"`
		Socket string `json:"socket"`
	}
	if json.Unmarshal(data, &record) != nil || record.PID <= 0 || record.FD < 0 || !strings.HasPrefix(record.Socket, "socket:[") {
		return "", false
	}
	socket, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(record.PID), "fd", strconv.Itoa(record.FD)))
	if err != nil || socket != record.Socket {
		return "", false
	}
	ip, err := netip.ParseAddr(record.IP)
	if err != nil || ip.Zone() != "" {
		return "", false
	}
	return ip.Unmap().String(), true
}

func tdzTrustedProxy(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, raw := range strings.Split(os.Getenv("TDZ_XRAY_TRUSTED_PROXIES"), ",") {
		raw = strings.TrimSpace(raw)
		if prefix, err := netip.ParsePrefix(raw); err == nil && prefix.Contains(ip) {
			return true
		}
		if address, err := netip.ParseAddr(raw); err == nil && address.Unmap() == ip {
			return true
		}
	}
	return false
}

// TDZClientAddr prefers the private, socket-validated TDZP channel. Other
// forwarded addresses require an explicitly trusted immediate proxy peer.
// Walk from the proxy end of XFF, so client-supplied prefixes cannot choose a slot.
func TDZClientAddr(peer, local net.Addr, headers http.Header, names []string) net.Addr {
	if peer == nil {
		return peer
	}
	if local != nil {
		if mapped, ok := TDZLocalProxyIP(peer.String(), local.String()); ok {
			return &net.TCPAddr{IP: net.ParseIP(mapped)}
		}
	}
	host, _, err := net.SplitHostPort(peer.String())
	if err != nil {
		return peer
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !tdzTrustedProxy(ip) {
		return peer
	}
	if len(names) == 0 {
		names = []string{"X-Forwarded-For", "X-Real-IP"}
	}
	for _, name := range names {
		values := strings.Join(headers.Values(name), ",")
		if values == "" {
			continue
		}
		parts := strings.Split(values, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if err != nil || candidate.Zone() != "" || candidate.IsUnspecified() || candidate.IsMulticast() {
				return peer
			}
			candidate = candidate.Unmap()
			if !tdzTrustedProxy(candidate) {
				return &net.TCPAddr{IP: net.IP(candidate.AsSlice())}
			}
		}
	}
	return peer
}
