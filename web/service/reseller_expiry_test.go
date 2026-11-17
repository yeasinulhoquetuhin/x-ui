package service

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/session"
)

func TestResellerOptionalExpiryPreservationAndSessionDenial(t *testing.T) {
	setupClientResetDB(t)
	var service ResellerService
	form := ResellerAccessForm{Enabled: true, Path: "expiry-shop", Username: "shop", Password: "password", CanEditPassword: true}
	account, err := service.Save(1, 0, form)
	if err != nil || account.ExpiryTime != 0 || account.IsExpired() {
		t.Fatal(account, err)
	}
	future := time.Now().Add(time.Hour).UnixMilli()
	form.ExpiryTime = &future
	form.Password = ""
	account, err = service.Save(1, account.ID, form)
	if err != nil || account.ExpiryTime != future {
		t.Fatal(account, err)
	}
	form.ExpiryTime = nil
	account, err = service.Save(1, account.ID, form)
	if err != nil || account.ExpiryTime != future {
		t.Fatal("omitted expiry removed the saved date", account, err)
	}
	principal := session.ResellerPrincipal{ID: account.ID, Path: account.Path, Version: account.Version}
	if session.CurrentReseller(principal) == nil {
		t.Fatal("future expiry denied access")
	}
	past := time.Now().Add(-time.Second).UnixMilli()
	if err := database.GetDB().Model(&model.ResellerAccount{}).Where("id = ?", account.ID).Update("expiry_time", past).Error; err != nil {
		t.Fatal(err)
	}
	if session.CurrentReseller(principal) != nil {
		t.Fatal("expired session remained active")
	}
	if _, err := service.ByPath(account.Path); !errors.Is(err, ErrResellerExpired) {
		t.Fatal("expired login path allowed", err)
	}
	if err := service.ChangeOwnPassword(account, "password", "new", "127.0.0.1"); !errors.Is(err, ErrResellerExpired) {
		t.Fatal("expired password write allowed", err)
	}
	zero := int64(0)
	form.ExpiryTime = &zero
	account, err = service.Save(1, account.ID, form)
	if err != nil || account.IsExpired() || account.ExpiryTime != 0 {
		t.Fatal("expiry removal failed", account, err)
	}
	negative := int64(-1)
	form.ExpiryTime = &negative
	if _, err := service.Save(1, account.ID, form); err == nil {
		t.Fatal("negative expiry accepted")
	}
}

func TestResellerDurationDaysAndUnrelatedEditPreservation(t *testing.T) {
	setupClientResetDB(t)
	var s ResellerService
	days := int64(7)
	form := ResellerAccessForm{Enabled: true, Path: "duration", Username: "shop", Password: "password", Duration: &days}
	before := time.Now().UnixMilli()
	account, err := s.Save(1, 0, form)
	if err != nil {
		t.Fatal(err)
	}
	const week int64 = 7 * 86400000
	if account.ExpiryTime < before+week || account.ExpiryTime > time.Now().UnixMilli()+week {
		t.Fatal("duration did not produce the correct stored expiry", account.ExpiryTime)
	}
	expiry := account.ExpiryTime
	form.Password = ""
	form.Duration = nil
	form.CanEditPassword = true
	account, err = s.Save(1, account.ID, form)
	if err != nil || account.ExpiryTime != expiry {
		t.Fatal("unrelated edit changed expiry", account, err)
	}
	zero := int64(0)
	form.Duration = &zero
	account, err = s.Save(1, account.ID, form)
	if err != nil || account.ExpiryTime != 0 {
		t.Fatal("zero duration is not unlimited", account, err)
	}
	for _, bad := range []int64{-1, math.MaxInt64} {
		form.Duration = &bad
		if _, err := s.Save(1, account.ID, form); err == nil {
			t.Fatal("invalid/overflow duration accepted", bad)
		}
	}
}
