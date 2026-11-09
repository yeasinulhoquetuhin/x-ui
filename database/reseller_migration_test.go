package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestLegacyResellerMigrationPreservesCredentialsAndDoesNotResurrect(t *testing.T) {
	file := filepath.Join(t.TempDir(), "migration.db")
	if err := InitDB(file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	inbound := model.Inbound{UserId: 1, Tag: "legacy", Protocol: model.VLESS, Settings: `{"clients":[]}`}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	legacy := model.ResellerAccess{InboundID: inbound.Id, Enabled: true, Path: "legacy-shop", Username: "old-user", PasswordHash: "old-hash", Version: "old-version"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerAudit{InboundID: inbound.Id, Action: "client.create", Username: "old-user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("seeder_name = ?", "IndependentResellerAccountsV3").Delete(&model.HistoryOfSeeders{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(file); err != nil {
		t.Fatal(err)
	}
	var account model.ResellerAccount
	if err := db.First(&account, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if account.Path != legacy.Path || account.Username != legacy.Username || account.PasswordHash != legacy.PasswordHash || account.Version != legacy.Version || account.AdminID != 1 || !account.Enabled || account.CanEditInbounds {
		t.Fatalf("migration changed credentials/permissions: %+v", account)
	}
	var assignment model.ResellerInbound
	if err := db.First(&assignment).Error; err != nil || assignment.ResellerID != account.ID || assignment.InboundID != inbound.Id {
		t.Fatal("original assignment not migrated", err)
	}
	var audit model.ResellerAudit
	if err := db.First(&audit).Error; err != nil || audit.ResellerID != account.ID {
		t.Fatal("historical audit not linked", err)
	}
	if err := db.Where("reseller_id = ?", account.ID).Delete(&model.ResellerInbound{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&account).Error; err != nil {
		t.Fatal(err)
	}
	_ = CloseDB()
	if err := InitDB(file); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&model.ResellerAccount{}).Count(&count)
	if count != 0 {
		t.Fatal("deleted legacy reseller was recreated")
	}
	db.Model(&model.ResellerInbound{}).Count(&count)
	if count != 0 {
		t.Fatal("removed legacy assignment was recreated")
	}
}
