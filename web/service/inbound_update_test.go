package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"github.com/op/go-logging"
)

func TestInboundUpdateCommitsAccountingOrRollsBack(t *testing.T) {
	setupClientResetDB(t)
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	previousProcess := p
	p = nil
	t.Cleanup(func() { p = previousProcess })
	settings := func(email string) string {
		data, _ := json.Marshal(map[string]any{"decryption": "none", "clients": []model.Client{{Email: email, ID: "11111111-1111-4111-8111-111111111111", Enable: true}}})
		return string(data)
	}
	original := &model.Inbound{UserId: 1, Enable: true, Tag: "inbound-13110", Port: 13110, Protocol: model.VLESS, Settings: settings("existing"), StreamSettings: `{"network":"tcp"}`, Sniffing: `{}`}
	if err := database.GetDB().Create(original).Error; err != nil {
		t.Fatal(err)
	}
	traffic := &xray.ClientTraffic{InboundId: original.Id, Email: "existing", Enable: false, Up: 30, Down: 40, AllTime: 70}
	if err := database.GetDB().Create(traffic).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Exec(`CREATE TRIGGER reject_new_client BEFORE INSERT ON client_traffics WHEN NEW.email = 'added' BEGIN SELECT RAISE(ABORT, 'simulated accounting failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	var service InboundService
	input := *original
	input.Port, input.Remark, input.Settings = 13111, "changed", settings("added")
	if _, _, err := service.UpdateInbound(&input); err == nil || !strings.Contains(err.Error(), "simulated accounting failure") {
		t.Fatal("expected accounting failure before runtime update", err)
	}
	saved, err := service.GetInbound(original.Id)
	if err != nil || saved.Port != original.Port || saved.Settings != original.Settings || saved.Remark != "" {
		t.Fatal("failed update changed the inbound", saved, err)
	}
	var rows []xray.ClientTraffic
	if err := database.GetDB().Where("inbound_id = ?", original.Id).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Email != "existing" || rows[0].Up != 30 || rows[0].Down != 40 || rows[0].Enable {
		t.Fatal("failed update changed accounting", rows)
	}
	if err := database.GetDB().Exec(`DROP TRIGGER reject_new_client`).Error; err != nil {
		t.Fatal(err)
	}
	saved, restart, err := service.UpdateInbound(&input)
	if err != nil || !restart || saved.Port != 13111 {
		t.Fatal("committed update failed with offline core", saved, restart, err)
	}
	rows = nil
	if err := database.GetDB().Where("inbound_id = ?", original.Id).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Email != "added" || !rows[0].Enable {
		t.Fatal("successful update did not commit accounting", rows)
	}
}
