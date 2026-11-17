package sub

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestBotLocalSubscriptionAPIUsesNormalGenerator(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	inbound := model.Inbound{UserId: 1, Tag: "bot", Port: 443, Enable: true, Protocol: model.VLESS, Settings: `{"clients":[{"id":"11111111-1111-4111-8111-111111111111","email":"client","subId":"local-sub","enable":true}],"decryption":"none"}`, StreamSettings: `{"network":"tcp","security":"none"}`}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	links, err := NewServer().GetSubscriptionLinks("local-sub", "sgx.tuhinbro.online")
	if err != nil || len(links) != 1 || !strings.Contains(links[0], "@sgx.tuhinbro.online:443") {
		t.Fatal(links, err)
	}
	if _, err := NewServer().GetSubscriptionLinks("unknown", "sgx.tuhinbro.online"); err == nil {
		t.Fatal("unknown subscription allowed")
	}
}
