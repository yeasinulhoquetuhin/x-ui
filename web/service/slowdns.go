package service

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

var slowDNSMu sync.Mutex

type SlowDNSService struct{}

func (s *SlowDNSService) GetConfig() (*model.SlowDNSConfig, error) {
	cfg := &model.SlowDNSConfig{ID: 1, MTU: xray.SlowDNSDefaultMTU}
	db := database.GetDB()
	// Older import/test databases can be read before their new table is migrated.
	if !db.Migrator().HasTable(&model.SlowDNSConfig{}) {
		return cfg, nil
	}
	if err := db.First(cfg, 1).Error; err != nil && !database.IsNotFound(err) {
		return nil, err
	}
	return cfg, nil
}

func slowDNSPublicKey(privateKey string) (string, error) {
	data, err := hex.DecodeString(privateKey)
	if err != nil || len(data) != 32 {
		return "", errors.New("invalid stored SlowDNS private key; restore a backup or explicitly regenerate it")
	}
	key, err := ecdh.X25519().NewPrivateKey(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key.PublicKey().Bytes()), nil
}

func generateSlowDNSKey(cfg *model.SlowDNSConfig) error {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	cfg.PrivateKey = hex.EncodeToString(key.Bytes())
	cfg.PublicKey = hex.EncodeToString(key.PublicKey().Bytes())
	return nil
}

func normalizeSlowDNSDomain(domain string, maximum int) (string, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if len(domain) == 0 || len(domain) > maximum || !strings.Contains(domain, ".") {
		return "", fmt.Errorf("enter a DNS domain between 3 and %d characters", maximum)
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("invalid DNS label")
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return "", errors.New("use an ASCII/Punycode DNS domain without a scheme, port or wildcard")
			}
		}
	}
	return domain, nil
}

func validateSlowDNSConfig(cfg *model.SlowDNSConfig) error {
	var err error
	cfg.HostDomain, err = normalizeSlowDNSDomain(cfg.HostDomain, 253)
	if err != nil {
		return fmt.Errorf("host domain: %w", err)
	}
	cfg.NSDomain, err = normalizeSlowDNSDomain(cfg.NSDomain, 100)
	if err != nil {
		return fmt.Errorf("NS domain: %w", err)
	}
	if cfg.HostDomain == cfg.NSDomain || strings.HasSuffix(cfg.HostDomain, "."+cfg.NSDomain) {
		return errors.New("the nameserver host must be outside the delegated SlowDNS NS zone")
	}
	if cfg.MTU < 512 || cfg.MTU > 4096 {
		return errors.New("SlowDNS MTU must be between 512 and 4096 bytes (default 1232)")
	}
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil || net.ParseIP(host) == nil {
		return errors.New("SlowDNS listen address must be an IP:port, e.g. 203.0.113.2:53 or [2001:db8::2]:53")
	}
	if _, err := net.LookupPort("udp", port); err != nil {
		return errors.New("invalid UDP listen port")
	}
	publicKey, err := slowDNSPublicKey(cfg.PrivateKey)
	if err != nil {
		return err
	}
	if cfg.PublicKey != "" && cfg.PublicKey != publicKey {
		return errors.New("stored SlowDNS public/private keys do not match; restore a backup or explicitly regenerate them")
	}
	cfg.PublicKey = publicKey
	return nil
}

func DefaultSlowDNSListen() string {
	if runtime.GOOS == "linux" {
		if listener, err := net.ListenPacket("udp", "[::]:0"); err == nil {
			listener.Close()
			return "[::]:5301"
		}
		return "0.0.0.0:5301"
	}
	addresses, _ := net.InterfaceAddrs()
	fallback := "0.0.0.0"
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || ip.To4() == nil || ip.IsLoopback() || !ip.IsGlobalUnicast() {
			continue
		}
		if !ip.IsPrivate() {
			return net.JoinHostPort(ip.String(), "53")
		}
		fallback = ip.String()
	}
	return net.JoinHostPort(fallback, "53")
}

func (s *SlowDNSService) Configure(hostDomain, nsDomain, listen string, mtu int, install bool) (*model.SlowDNSConfig, error) {
	slowDNSMu.Lock()
	defer slowDNSMu.Unlock()
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	previous := *cfg
	if !install && !cfg.Installed {
		return nil, errors.New("install SlowDNS first using x-ui menu 23")
	}
	if hostDomain != "" {
		cfg.HostDomain = hostDomain
	}
	if nsDomain != "" {
		cfg.NSDomain = nsDomain
	}
	if listen != "" {
		cfg.Listen = listen
	}
	if cfg.Listen == "" {
		cfg.Listen = DefaultSlowDNSListen()
	}
	if mtu != 0 {
		cfg.MTU = mtu
	}
	if cfg.PrivateKey == "" {
		if err := generateSlowDNSKey(cfg); err != nil {
			return nil, err
		}
	}
	if err := validateSlowDNSConfig(cfg); err != nil {
		return nil, err
	}
	if err := validateTDZSlowDNSZone(cfg.NSDomain); err != nil {
		return nil, err
	}
	if !previous.Installed || previous.Listen != cfg.Listen {
		listener, err := net.ListenPacket("udp", cfg.Listen)
		if err != nil {
			return nil, fmt.Errorf("SlowDNS UDP address is unavailable: %w; choose the server's public IP to avoid local DNS stub conflicts", err)
		}
		listener.Close()
	}
	cfg.Installed = true
	if install {
		cfg.Enabled = true
	}
	if err := database.GetDB().Save(cfg).Error; err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *SlowDNSService) SetEnabled(enabled bool) (*model.SlowDNSConfig, error) {
	slowDNSMu.Lock()
	defer slowDNSMu.Unlock()
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Installed {
		return nil, errors.New("SlowDNS is not installed")
	}
	cfg.Enabled = enabled
	return cfg, database.GetDB().Save(cfg).Error
}

func (s *SlowDNSService) Uninstall() (*model.SlowDNSConfig, error) {
	slowDNSMu.Lock()
	defer slowDNSMu.Unlock()
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		cfg.Installed, cfg.Enabled = false, false
		if err := tx.Save(cfg).Error; err != nil {
			return err
		}
		// A reinstall starts with opt-in disabled on every inbound, retaining keys.
		return tx.Model(&model.Inbound{}).
			Where("json_valid(stream_settings) AND json_extract(stream_settings, '$.slowDNS') = 1").
			Update("stream_settings", gorm.Expr("json_set(stream_settings, '$.slowDNS', json('false'))")).Error
	}); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *SlowDNSService) RegenerateKey() (*model.SlowDNSConfig, error) {
	slowDNSMu.Lock()
	defer slowDNSMu.Unlock()
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if err := generateSlowDNSKey(cfg); err != nil {
		return nil, err
	}
	return cfg, database.GetDB().Save(cfg).Error
}

func validateInboundSlowDNS(inbound *model.Inbound) error {
	enabled, err := xray.SlowDNSEnabled(string(inbound.Protocol), []byte(inbound.StreamSettings))
	if err != nil || !enabled {
		return err
	}
	var service SlowDNSService
	cfg, err := service.GetConfig()
	if err != nil {
		return err
	}
	if !cfg.Installed {
		return errors.New("install SlowDNS using x-ui menu 23 before enabling it on an inbound")
	}
	return validateSlowDNSConfig(cfg)
}

func WriteSlowDNSRuntimeConfig() error {
	slowDNSMu.Lock()
	defer slowDNSMu.Unlock()
	var service SlowDNSService
	cfg, err := service.GetConfig()
	if err != nil {
		return err
	}
	runtime := xray.SlowDNSRuntimeConfig{}
	if cfg.Installed && cfg.Enabled {
		if err := validateSlowDNSConfig(cfg); err != nil {
			return err
		}
		runtime.Enabled, runtime.Listen, runtime.MTU, runtime.PrivateKey = true, cfg.Listen, cfg.MTU, cfg.PrivateKey
		runtime.HostDomain, runtime.NSDomain = cfg.HostDomain, cfg.NSDomain
		var inbounds []model.Inbound
		if err := database.GetDB().Where("enable = ?", true).Order("id ASC").Find(&inbounds).Error; err != nil {
			return err
		}
		for _, inbound := range inbounds {
			enabled, err := xray.SlowDNSEnabled(string(inbound.Protocol), []byte(inbound.StreamSettings))
			if err != nil {
				return fmt.Errorf("inbound %s: %w", inbound.Tag, err)
			}
			if !enabled {
				continue
			}
			var stream struct {
				Network        string                       `json:"network"`
				AcceptPayloads bool                         `json:"acceptPayloads"`
				WS             struct{ Path, Host string }  `json:"wsSettings"`
				HTTPUpgrade    struct{ Path, Host string }  `json:"httpupgradeSettings"`
				GRPC           struct{ ServiceName string } `json:"grpcSettings"`
				XHTTP          struct{ Path, Host string }  `json:"xhttpSettings"`
			}
			if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
				return err
			}
			route := xray.SlowDNSRoute{Domain: fmt.Sprintf("i%d.%s", inbound.Id, cfg.NSDomain), Listen: inbound.Listen, Port: inbound.Port, Network: xray.SlowDNSNetwork(stream.Network), AcceptPayloads: stream.AcceptPayloads}
			switch route.Network {
			case "websocket":
				route.Path, route.Host = stream.WS.Path, stream.WS.Host
			case "httpupgrade":
				route.Path, route.Host = stream.HTTPUpgrade.Path, stream.HTTPUpgrade.Host
			case "splithttp":
				route.Path, route.Host = stream.XHTTP.Path, stream.XHTTP.Host
			case "grpc":
				route.ServiceName = stream.GRPC.ServiceName
			}
			route.Path = strings.SplitN(route.Path, "?", 2)[0]
			if route.Path != "" && !strings.HasPrefix(route.Path, "/") {
				route.Path = "/" + route.Path
			}
			runtime.Routes = append(runtime.Routes, route)
		}
	}
	data, err := json.Marshal(runtime)
	if err != nil {
		return err
	}
	path := xray.GetSlowDNSConfigPath()
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return os.Chmod(path, 0600)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".slowdns-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
