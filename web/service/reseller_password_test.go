package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestResellerOwnPasswordPermissionAndRevocation(t *testing.T) {
	setupClientResetDB(t)
	var s ResellerService
	form := ResellerAccessForm{Enabled: true, Path: "password-shop", Username: "shop", Password: "old"}
	account, err := s.Save(1, 0, form)
	if err != nil {
		t.Fatal(err)
	}
	if account.CanEditPassword {
		t.Fatal("permission enabled by default")
	}
	if err := s.ChangeOwnPassword(account, "old", "new", "192.0.2.1"); !errors.Is(err, ErrResellerPasswordPermission) {
		t.Fatal(err)
	}
	form.CanEditPassword = true
	form.Password = ""
	account, err = s.Save(1, account.ID, form)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeOwnPassword(account, "wrong", "new", "192.0.2.1"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := s.ChangeOwnPassword(account, "old", "", "192.0.2.1"); err == nil {
		t.Fatal("empty password accepted")
	}
	password := strings.Repeat("long-password", 20)
	if err := s.ChangeOwnPassword(account, "old", password, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	updated, err := s.Get(1, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version == account.Version || CheckResellerPassword(updated.PasswordHash, "old") || !CheckResellerPassword(updated.PasswordHash, password) {
		t.Fatal("password or session version not updated")
	}
	if err := s.ChangeOwnPassword(account, password, "other", "192.0.2.1"); !errors.Is(err, ErrResellerSessionRevoked) {
		t.Fatal("stale session accepted", err)
	}
	rows, err := s.AuditList(account.ID)
	if err != nil || len(rows) != 1 || rows[0].Action != "account.password" || rows[0].Target != "" {
		t.Fatal("missing/sensitive audit", rows, err)
	}
	// A permission revoked after the request's initial lookup must still win.
	database.GetDB().Model(&model.ResellerAccount{}).Where("id = ?", account.ID).Update("can_edit_password", false)
	if err := s.ChangeOwnPassword(updated, password, "other", "192.0.2.1"); !errors.Is(err, ErrResellerPasswordPermission) {
		t.Fatal("stale permission accepted", err)
	}
}
