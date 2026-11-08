package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func setupIPTrackingDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func TestLiveSnapshotDropsDisconnectedIPs(t *testing.T) {
	setupIPTrackingDB(t)
	for _, email := range []string{"client", "offline"} {
		row := model.InboundClientIps{ClientEmail: email, Ips: `[{"ip":"192.0.2.1","timestamp":1}]`}
		if err := database.GetDB().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	snapshot := map[string][]xray.ClientIP{"client": {{IP: "2001:db8::1", Timestamp: 100}}}
	if err := service.StoreClientIPSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"client", "offline"} {
		var row model.InboundClientIps
		if err := database.GetDB().Where("client_email = ?", email).First(&row).Error; err != nil {
			t.Fatal(err)
		}
		var ips []xray.ClientIP
		if err := json.Unmarshal([]byte(row.Ips), &ips); err != nil {
			t.Fatal(err)
		}
		if email == "offline" && len(ips) != 0 {
			t.Fatalf("offline IP retained: %v", ips)
		}
		if email == "client" && (len(ips) != 1 || ips[0].IP != "2001:db8::1") {
			t.Fatalf("wrong live IPs: %v", ips)
		}
	}
	if err := service.StoreClientIPSnapshot(map[string][]xray.ClientIP{}); err != nil {
		t.Fatal(err)
	}
	var rows []model.InboundClientIps
	database.GetDB().Find(&rows)
	for _, row := range rows {
		if row.Ips != "[]" {
			t.Fatalf("stale IPs resurrected: %s", row.Ips)
		}
	}
}

func TestLimitsIncludeEveryClientProtocolAndExactEmail(t *testing.T) {
	setupIPTrackingDB(t)
	for i, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks", "hysteria", "hysteria2"} {
		settings := map[string]any{"decryption": "none", "method": "2022-blake3-aes-256-gcm", "version": 2, "clients": []map[string]any{
			{"email": protocol + "_1", "limitIp": 1, "enable": true},
			{"email": protocol + "_10", "limitIp": 3, "enable": true},
			{"email": protocol + "_off", "limitIp": 1, "enable": false},
		}}
		data, _ := json.Marshal(settings)
		row := model.Inbound{Tag: protocol, Port: 30000 + i, Enable: true, Protocol: model.Protocol(protocol), Settings: string(data)}
		if err := database.GetDB().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := service.WriteClientIPLimits(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(xray.GetIPLimitConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	var limits map[string]int
	if err := json.Unmarshal(data, &limits); err != nil {
		t.Fatal(err)
	}
	if len(limits) != 12 {
		t.Fatalf("missing limits: %s", data)
	}
	for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks", "hysteria", "hysteria2"} {
		if limits[protocol+"_1"] != 1 || limits[protocol+"_10"] != 3 {
			t.Fatalf("wrong email/limit match: %s", data)
		}
	}
	if err := database.GetDB().Model(&model.Inbound{}).Where("1 = 1").Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.WriteClientIPLimits(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(xray.GetIPLimitConfigPath())
	if string(data) != "{}" {
		t.Fatalf("removed limits retained: %s", data)
	}
}
