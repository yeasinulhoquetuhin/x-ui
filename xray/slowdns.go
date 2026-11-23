package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/config"
)

const SlowDNSDefaultMTU = 1232

// SlowDNS is a server-side DNS-to-stream listener, not an Xray client transport.
// Keeping it out of the stream protobuf also preserves live API compatibility.
type SlowDNSRoute struct {
	Domain         string `json:"domain"`
	Listen         string `json:"listen"`
	Port           int    `json:"port"`
	Network        string `json:"network"`
	Path           string `json:"path,omitempty"`
	Host           string `json:"host,omitempty"`
	ServiceName    string `json:"serviceName,omitempty"`
	AcceptPayloads bool   `json:"acceptPayloads,omitempty"`
}

type SlowDNSRuntimeConfig struct {
	Enabled    bool           `json:"enabled"`
	Listen     string         `json:"listen,omitempty"`
	HostDomain string         `json:"hostDomain,omitempty"`
	NSDomain   string         `json:"nsDomain,omitempty"`
	MTU        int            `json:"mtu,omitempty"`
	PrivateKey string         `json:"privateKey,omitempty"`
	Routes     []SlowDNSRoute `json:"routes,omitempty"`
}

func GetSlowDNSConfigPath() string {
	return filepath.Join(config.GetDBFolderPath(), "slowdns-runtime.json")
}

func NativeSlowDNSAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, GetBinaryPath(), "-version").Output()
	return err == nil && bytes.Contains(output, []byte("SlowDNS/1"))
}

func SlowDNSNetwork(network string) string {
	switch strings.ToLower(network) {
	case "", "tcp", "raw":
		return "tcp"
	case "ws", "websocket":
		return "websocket"
	case "xhttp", "splithttp":
		return "splithttp"
	default:
		return strings.ToLower(network)
	}
}

func SlowDNSCapable(protocol, network, security string) bool {
	switch strings.ToLower(protocol) {
	case "vless", "vmess", "trojan", "shadowsocks":
	default:
		return false
	}
	if security != "" && security != "none" {
		return false
	}
	switch SlowDNSNetwork(network) {
	case "tcp", "websocket", "httpupgrade", "grpc", "splithttp":
		return true
	}
	return false
}

func SlowDNSEnabled(protocol string, raw []byte) (bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, nil
	}
	var stream struct {
		SlowDNS  bool   `json:"slowDNS"`
		Network  string `json:"network"`
		Security string `json:"security"`
	}
	if err := json.Unmarshal(raw, &stream); err != nil {
		return false, err
	}
	if stream.SlowDNS && !SlowDNSCapable(protocol, stream.Network, stream.Security) {
		return false, fmt.Errorf("SlowDNS supports non-TLS VLESS, VMess, Trojan and Shadowsocks over TCP/RAW, WebSocket, HTTPUpgrade, gRPC or XHTTP over TCP")
	}
	return stream.SlowDNS, nil
}

func StripSlowDNSSettings(raw string) string {
	var stream map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &stream) != nil {
		return raw
	}
	if _, exists := stream["slowDNS"]; !exists {
		return raw
	}
	delete(stream, "slowDNS")
	data, err := json.Marshal(stream)
	if err != nil {
		return raw
	}
	return string(data)
}

func SlowDNSDomain(inboundID int, nsDomain string) string {
	return nsDomain
}
