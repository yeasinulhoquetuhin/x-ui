package service

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestSlowDNSKeySurvivesReconfigureAndUninstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := udp.LocalAddr().String()
	udp.Close()
	var service SlowDNSService
	cfg, err := service.Configure("ns.example.test", "t.example.test", address, 1232, true)
	if err != nil {
		t.Fatal(err)
	}
	private, public := cfg.PrivateKey, cfg.PublicKey
	if len(private) != 64 || len(public) != 64 {
		t.Fatal("expected DNSTT X25519 keys")
	}
	cfg, err = service.Configure("", "new.example.test", "", 512, false)
	if err != nil || cfg.PrivateKey != private || cfg.PublicKey != public || cfg.MTU != 512 {
		t.Fatalf("reconfiguration changed keys: %+v (%v)", cfg, err)
	}
	inbound := model.Inbound{Tag: "dns-test", Protocol: model.VLESS, StreamSettings: `{"network":"tcp","security":"none","slowDNS":true}`, Settings: `{"clients":[]}`, Enable: true, Port: 18000}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(cfg)
	if strings.Contains(string(data), private) || strings.Contains(string(data), "privateKey") {
		t.Fatal("private key leaked through public JSON")
	}
	cfg, err = service.Uninstall()
	if err != nil || cfg.Installed || cfg.PrivateKey != private || cfg.PublicKey != public {
		t.Fatal("uninstall must preserve the keypair", err)
	}
	if err := database.GetDB().First(&inbound, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(inbound.StreamSettings, `"slowDNS":true`) {
		t.Fatal("uninstall must disable inbound opt-ins")
	}
	cfg, err = service.Configure("", "", "", 0, true)
	if err != nil || cfg.PublicKey != public || cfg.PrivateKey != private {
		t.Fatal("reinstall revoked the public key", err)
	}
	cfg, err = service.RegenerateKey()
	if err != nil || cfg.PublicKey == public || cfg.PrivateKey == private {
		t.Fatal("explicit regenerate did not replace keys", err)
	}
}

func TestSlowDNSRejectsUnsafeDomainsWithoutChangingStoredKeys(t *testing.T) {
	for _, domain := range []string{"", "https://example.test", "bad..example", "-bad.example", "example.test:53"} {
		if _, err := normalizeSlowDNSDomain(domain, 100); err == nil {
			t.Fatalf("accepted invalid domain %q", domain)
		}
	}
	cfg := &model.SlowDNSConfig{HostDomain: "ns.t.example.test", NSDomain: "t.example.test", Listen: "127.0.0.1:53", MTU: 1232}
	if err := generateSlowDNSKey(cfg); err != nil {
		t.Fatal(err)
	}
	if err := validateSlowDNSConfig(cfg); err == nil {
		t.Fatal("nameserver cannot live beneath its own delegated tunnel zone")
	}
}
