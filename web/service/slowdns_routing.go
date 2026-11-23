package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

var slowDNSRoutingMu sync.Mutex

func slowDNSRoutingScript() (string, error) {
	if path := os.Getenv("XUI_SLOWDNS_ROUTER"); path != "" {
		return path, nil
	}
	executable, _ := os.Executable()
	for _, path := range []string{filepath.Join(filepath.Dir(executable), "resources", "slowdns-routing.sh"), filepath.Join("resources", "slowdns-routing.sh")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("SlowDNS routing resource is missing; install the complete V5 package")
}

// Configure only domain-specific rules. SSH's port-53 forwarding can be started,
// stopped or reinstalled in any order because our NAT hook has an earlier priority.
func SyncSlowDNSRouting() error {
	if runtime.GOOS != "linux" || os.Getenv("XUI_SLOWDNS_ROUTING") == "off" {
		return nil
	}
	slowDNSRoutingMu.Lock()
	defer slowDNSRoutingMu.Unlock()
	data, err := os.ReadFile(xray.GetSlowDNSConfigPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var cfg xray.SlowDNSRuntimeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(cfg.Listen)
	active := cfg.Enabled && len(cfg.Routes) > 0 && err == nil && port != "53"
	stateDir := os.Getenv("XUI_SLOWDNS_STATE_DIR")
	if stateDir == "" {
		stateDir = "/run/x-ui-slowdns"
	}
	if !active {
		if _, err := os.Stat(filepath.Join(stateDir, "state")); os.IsNotExist(err) {
			return nil
		}
	}
	script, err := slowDNSRoutingScript()
	if err != nil {
		return err
	}
	args := []string{script, "stop"}
	if active {
		var wire []byte
		for _, label := range strings.Split(cfg.NSDomain, ".") {
			wire = append(wire, byte(len(label)))
			wire = append(wire, label...)
		}
		wire = append(wire, 0)
		host, _, _ := net.SplitHostPort(cfg.Listen)
		mode := "ipv4"
		if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
			mode = "ipv6"
			if ip.IsUnspecified() {
				mode = "dual"
			}
		}
		args = []string{script, "apply", hex.EncodeToString(wire), port, mode}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "bash", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("SlowDNS DNS-port routing: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func StartSlowDNSRouting() func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var lastError string
		for {
			if err := SyncSlowDNSRouting(); err != nil {
				if err.Error() != lastError {
					logger.Warning(err)
					lastError = err.Error()
				}
			} else {
				lastError = ""
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

func checkSlowDNSRoutingRequirements(listen string) error {
	if runtime.GOOS != "linux" || os.Getenv("XUI_SLOWDNS_ROUTING") == "off" {
		return nil
	}
	_, port, err := net.SplitHostPort(listen)
	if err == nil && port == "53" {
		return nil
	}
	for _, command := range []string{"iptables", "nft", "flock"} {
		if _, err := exec.LookPath(command); err != nil {
			return fmt.Errorf("shared-port SlowDNS requires %s; install dependencies from x-ui menu 23", command)
		}
	}
	return nil
}

// No shell evaluation: read only TDZ SSH's public tunnel-domain setting.
func validateTDZSlowDNSZone(zone string) error {
	path := os.Getenv("XUI_TDZ_SLOWDNS_CONFIG")
	if path == "" {
		path = "/etc/tdztunnel/dnstt_info.conf"
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key != "TUNNEL_DOMAIN" {
			continue
		}
		value = strings.TrimSpace(value)
		if decoded, err := strconv.Unquote(value); err == nil {
			value = decoded
		} else {
			value = strings.Trim(value, "'\"")
		}
		value = strings.ToLower(strings.TrimSuffix(value, "."))
		if zone == value || strings.HasSuffix(zone, "."+value) || strings.HasSuffix(value, "."+zone) {
			return fmt.Errorf("TDZ SSH uses %s; choose a separate sibling SlowDNS zone for X-UI to keep both tunnels independent", value)
		}
	}
	return nil
}
