package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/op/go-logging"
)

func resellerTestInbound(t *testing.T, tag string, clients []model.Client) *model.Inbound {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"clients": clients, "decryption": "none", "fallbacks": []any{map[string]any{"dest": "private.internal:8080"}}})
	inbound := &model.Inbound{UserId: 1, Tag: tag, Protocol: model.VLESS, Settings: string(data), StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"privateKey":"server-secret","shortIds":["1234"],"settings":{"publicKey":"public"}},"tlsSettings":{"certificates":[{"keyFile":"/secret/key"}]}}`}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	return inbound
}

func TestResellerIndependentAssignmentsAndPermission(t *testing.T) {
	setupClientResetDB(t)
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	one := resellerTestInbound(t, "one", []model.Client{{Email: "own", ID: "one", SubID: "one-sub"}})
	two := resellerTestInbound(t, "two", []model.Client{{Email: "other", ID: "two", SubID: "two-sub"}})
	foreign := resellerTestInbound(t, "foreign", nil)
	database.GetDB().Model(foreign).Update("user_id", 9)
	var service ResellerService
	form := ResellerAccessForm{Enabled: true, Path: "independent", Username: "shop", Password: "password"}
	account, err := service.Save(1, 0, form)
	if err != nil || account.CanEditInbounds || len(account.InboundIDs) != 0 {
		t.Fatal("zero-inbound/default permission failed", err)
	}
	if _, err := service.ByPath(form.Path); err != nil {
		t.Fatal("unassigned account cannot log in", err)
	}
	rows, err := service.Inbounds(account)
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatal("empty dashboard must return []", err)
	}
	version := account.Version
	form.Password = ""
	form.InboundIDs = []int{two.Id, one.Id, one.Id}
	form.CanEditInbounds = true
	account, err = service.Save(1, account.ID, form)
	if err != nil || account.Version != version || !reflect.DeepEqual(account.InboundIDs, []int{one.Id, two.Id}) {
		t.Fatal("live scope update failed", account, err)
	}
	if !service.HasInbound(account, one.Id) || service.HasInbound(account, foreign.Id) {
		t.Fatal("assignment scope incorrect")
	}
	rows, err = service.Inbounds(account)
	if err != nil || len(rows) != 2 || !strings.Contains(rows[0].StreamSettings, "server-secret") {
		t.Fatal("editable scoped settings missing", err)
	}
	second, err := service.Save(1, 0, ResellerAccessForm{Enabled: true, Path: "shared", Username: "second", Password: "p", InboundIDs: []int{one.Id}})
	if err != nil || !service.HasInbound(second, one.Id) {
		t.Fatal("shared inbound rejected", err)
	}
	rows, err = service.Inbounds(second)
	if err != nil || len(rows) != 1 || strings.Contains(rows[0].StreamSettings, "server-secret") {
		t.Fatal("clients-only reseller exposed secrets", err)
	}
	form.InboundIDs = []int{foreign.Id}
	if _, err = service.Save(1, account.ID, form); err == nil {
		t.Fatal("foreign administrator assignment accepted")
	}
	if _, err = service.Get(9, account.ID); err == nil {
		t.Fatal("foreign administrator read accepted")
	}
	form.InboundIDs = nil
	form.CanEditInbounds = false
	account, err = service.Save(1, account.ID, form)
	if err != nil || service.HasInbound(account, one.Id) || account.CanEditInbounds {
		t.Fatal("scope removal failed", err)
	}
	var inboundService InboundService
	if _, err = inboundService.DelInbound(one.Id); err != nil {
		t.Fatal(err)
	}
	if saved, err := service.ByPath(second.Path); err != nil || saved.ID != second.ID {
		t.Fatal("deleting assigned inbound deleted the independent account", err)
	}
	if ids, err := service.AssignedIDs(second); err != nil || len(ids) != 0 {
		t.Fatal("deleted inbound assignment retained", err)
	}
	if err = service.Delete(1, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ByPath(second.Path); err == nil {
		t.Fatal("deleted account remains active")
	}
}

func TestResellerInboundEditPreservesCurrentClientsAndCounters(t *testing.T) {
	setupClientResetDB(t)
	inbound := resellerTestInbound(t, "editable", []model.Client{{Email: "current", ID: "current-id", SubID: "current-sub"}})
	database.GetDB().Model(inbound).Updates(map[string]any{"port": 12345, "up": 100, "down": 200, "all_time": 900, "last_traffic_reset_time": 400})
	var service ResellerService
	account, err := service.Save(1, 0, ResellerAccessForm{Enabled: true, Path: "editor", Username: "u", Password: "p", InboundIDs: []int{inbound.Id}})
	if err != nil {
		t.Fatal(err)
	}
	input := &model.Inbound{Id: 999, UserId: 999, Protocol: model.VLESS, Port: 12346, Enable: true, Settings: `{"clients":[{"email":"injected","id":"bad"}],"decryption":"none"}`, StreamSettings: `{"network":"tcp"}`, Sniffing: `{}`, Up: 9999, Down: 9999}
	if _, err = service.InboundInput(account, inbound.Id, input); err == nil {
		t.Fatal("default permission allowed inbound edit")
	}
	account.CanEditInbounds = true
	result, err := service.InboundInput(account, inbound.Id, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Id != inbound.Id || result.UserId != 1 || result.Up != 100 || result.Down != 200 || result.AllTime != 900 || result.LastTrafficResetTime != 400 || strings.Contains(result.Settings, "injected") || !strings.Contains(result.Settings, "current-id") {
		t.Fatalf("untrusted/stale fields used: %+v", result)
	}
	result.Protocol = model.Trojan
	if _, err = service.InboundInput(account, inbound.Id, result); err == nil {
		t.Fatal("protocol change accepted")
	}
}

func TestResellerSettingsValidationAndSecretRedaction(t *testing.T) {
	setupClientResetDB(t)
	inbound := resellerTestInbound(t, "reseller", []model.Client{{Email: "own", ID: "own-id", Enable: true, SubID: "own-sub"}})
	other := resellerTestInbound(t, "other", []model.Client{{Email: "foreign", ID: "foreign-id", Enable: true, SubID: "foreign-sub"}})
	var service ResellerService
	form := ResellerAccessForm{Enabled: true, Path: "shop-one", Username: "reseller", Password: "long-password", InboundIDs: []int{inbound.Id}}
	account, err := service.Save(1, 0, form)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckResellerPassword(account.PasswordHash, form.Password) {
		t.Fatal("password not hashed")
	}
	encoded, _ := json.Marshal(account)
	if strings.Contains(string(encoded), account.PasswordHash) || strings.Contains(string(encoded), account.Version) {
		t.Fatal("authentication secrets exposed")
	}
	version := account.Version
	form.Password = ""
	account, err = service.Save(1, account.ID, form)
	if err != nil || account.Version != version {
		t.Fatal("unchanged settings revoked session", err)
	}
	form.Path = "shop-new"
	account, err = service.Save(1, account.ID, form)
	if err != nil || account.Version == version {
		t.Fatal("path edit did not revoke sessions", err)
	}
	if _, err = service.ByPath("shop-one"); err == nil {
		t.Fatal("old path remains enabled")
	}
	form.InboundIDs = []int{other.Id}
	if _, err = service.Save(1, 0, form); err == nil {
		t.Fatal("duplicate path accepted")
	}
	for _, path := range []string{"panel", "assets", "a/b", "..", "a b", "login"} {
		if ValidResellerPath(path) {
			t.Fatalf("invalid path accepted %q", path)
		}
	}
	safe, err := service.Inbound(inbound.Id)
	if err != nil {
		t.Fatal(err)
	}
	combined := safe.Settings + safe.StreamSettings
	for _, secret := range []string{"server-secret", "/secret/key", "private.internal"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("secret exposed: %s", secret)
		}
	}
	if !strings.Contains(combined, "public") {
		t.Fatal("client connection public key removed")
	}
}

func TestResellerCredentialsHaveNoArbitraryLengthRestrictions(t *testing.T) {
	setupClientResetDB(t)
	var service ResellerService
	id := 0
	for _, form := range []ResellerAccessForm{
		{Enabled: true, Path: "a", Username: "u", Password: "p"},
		{Enabled: true, Path: strings.Repeat("long-", 30), Username: strings.Repeat("user", 40), Password: strings.Repeat("password", 30)},
		{Enabled: true, Path: "xui-shop", Username: "name with spaces", Password: " x "},
	} {
		account, err := service.Save(1, id, form)
		if err != nil {
			t.Fatal(err)
		}
		id = account.ID
		if !CheckResellerPassword(account.PasswordHash, form.Password) || CheckResellerPassword(account.PasswordHash, form.Password+"different") {
			t.Fatal("password comparison truncated or changed")
		}
	}
	legacy, err := crypto.HashPasswordAsBcrypt("old-password")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckResellerPassword(legacy, "old-password") {
		t.Fatal("existing reseller hash stopped working")
	}
}

func TestResellerClientIdentitiesAreIsolated(t *testing.T) {
	setupClientResetDB(t)
	owner := resellerTestInbound(t, "owner", []model.Client{{Email: "own", ID: "own-id", Enable: true, SubID: "own-sub"}})
	resellerTestInbound(t, "other", []model.Client{{Email: "foreign", ID: "foreign-id", Enable: true, SubID: "foreign-sub"}})
	var service ResellerService
	for _, client := range []model.Client{
		{Email: "new", ID: "new-id", SubID: "foreign-sub"},
		{Email: "foreign", ID: "new-id", SubID: "new-sub"},
		{Email: "new", ID: "own-id", SubID: "new-sub"},
	} {
		if err := service.ValidateClients(owner, []model.Client{client}, ""); err == nil {
			t.Fatalf("foreign identity accepted %+v", client)
		}
	}
	if err := service.ValidateClients(owner, []model.Client{{Email: "FOREIGN", ID: "new-id"}}, ""); err == nil {
		t.Fatal("case variant of foreign email was accepted")
	}
	input, err := service.ClientInput(owner.Id, `{"clients":[{"email":"own","id":"own-id","subId":"own-sub","enable":true,"showIpLimit":true}],"fallbacks":[{"dest":"changed"}],"decryption":"changed"}`, "own-id")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(input.Settings, "fallbacks") || strings.Contains(input.Settings, "decryption") || !strings.Contains(input.Settings, `"showIpLimit":true`) {
		t.Fatal("client write was not restricted to client records")
	}
}

func TestResellerAccessRejectsExistingCrossInboundSubscription(t *testing.T) {
	setupClientResetDB(t)
	inbound := resellerTestInbound(t, "shared-one", []model.Client{{Email: "first", ID: "first", SubID: "shared"}})
	resellerTestInbound(t, "shared-two", []model.Client{{Email: "second", ID: "second", SubID: "shared"}})
	var service ResellerService
	if _, err := service.Save(1, 0, ResellerAccessForm{Enabled: true, Path: "shared-shop", Username: "shop", Password: "shop-password", InboundIDs: []int{inbound.Id}}); err == nil {
		t.Fatal("cross-inbound subscription exposed to reseller")
	}
}

func TestResellerSharedSubscriptionsRequireCompleteAssignments(t *testing.T) {
	setupClientResetDB(t)
	first := resellerTestInbound(t, "first", []model.Client{{Email: "first", ID: "first-id", SubID: "shared-sub"}})
	second := resellerTestInbound(t, "second", []model.Client{{Email: "second", ID: "second-id", SubID: "shared-sub"}})
	var service ResellerService
	form := ResellerAccessForm{Enabled: true, Path: "shared-group", Username: "shop", Password: "p", InboundIDs: []int{first.Id, second.Id}}
	account, err := service.Save(1, 0, form)
	if err != nil {
		t.Fatal("complete shared subscription assignment rejected", err)
	}
	input := `{"clients":[{"email":"first","id":"first-id","subId":"shared-sub","comment":"edited"}]}`
	if _, err = service.ClientInput(first.Id, input, "first-id", account); err != nil {
		t.Fatal("existing assigned shared subscription edit rejected", err)
	}
	if _, err = service.ClientInput(first.Id, `{"clients":[{"email":"intruder","id":"new-id","subId":"shared-sub"}]}`, "", account); err == nil {
		t.Fatal("new client joined another client's shared subscription")
	}
	form.Password = ""
	form.InboundIDs = []int{first.Id}
	if _, err = service.Save(1, account.ID, form); err == nil {
		t.Fatal("partial subscription scope exposed another inbound")
	}
	form.Enabled = false
	if _, err = service.Save(1, account.ID, form); err != nil {
		t.Fatal("administrator could not disable account with a conflicting assignment", err)
	}
}

func TestResellerAccessRejectsExistingCrossInboundClientIdentity(t *testing.T) {
	setupClientResetDB(t)
	inbound := resellerTestInbound(t, "identity-one", []model.Client{{Email: "shared-email", ID: "one", SubID: "sub-one"}})
	resellerTestInbound(t, "identity-two", []model.Client{{Email: "SHARED-EMAIL", ID: "two", SubID: "sub-two"}})
	var service ResellerService
	if _, err := service.Save(1, 0, ResellerAccessForm{Enabled: true, Path: "identity-shop", Username: "shop", Password: "shop-password", InboundIDs: []int{inbound.Id}}); err == nil || !strings.Contains(err.Error(), `"shared-email"`) || !strings.Contains(err.Error(), "inbound 2") {
		t.Fatalf("cross-inbound email conflict was not identified: %v", err)
	}
}

func TestResellerAccessAllowsInboundScopedAuthentication(t *testing.T) {
	for _, protocol := range []model.Protocol{model.VLESS, model.VMESS, model.Trojan, model.Hysteria, model.Hysteria2} {
		t.Run(string(protocol), func(t *testing.T) {
			setupClientResetDB(t)
			first := model.Client{Email: "first-client", ID: "127bb13c-fec7-4b96-a6c8-7cc16b847803", Password: "shared-password", Auth: "shared-auth", SubID: "first-sub", Enable: true}
			second := first
			second.Email, second.SubID = "second-client", "second-sub"
			owner := resellerTestInbound(t, "first-inbound", []model.Client{first})
			other := resellerTestInbound(t, "second-inbound", []model.Client{second})
			for _, inbound := range []*model.Inbound{owner, other} {
				inbound.Protocol = protocol
				if err := database.GetDB().Save(inbound).Error; err != nil {
					t.Fatal(err)
				}
			}
			var service ResellerService
			for i, inbound := range []*model.Inbound{owner, other} {
				path := []string{"first-shop", "second-shop"}[i]
				if _, err := service.Save(1, 0, ResellerAccessForm{Enabled: true, Path: path, Username: "shop", Password: "password", InboundIDs: []int{inbound.Id}}); err != nil {
					t.Fatalf("independent inbound authentication blocked reseller access: %v", err)
				}
			}
			if err := service.ValidateClients(owner, []model.Client{first}, ResellerClientID(protocol, first)); err != nil {
				t.Fatalf("editing own client blocked by another inbound's credentials: %v", err)
			}
			newClient := first
			newClient.Email, newClient.SubID = "third-client", "third-sub"
			if err := service.ValidateClients(owner, []model.Client{newClient}, ""); err == nil {
				t.Fatal("duplicate authentication accepted inside the same inbound")
			}
			empty := resellerTestInbound(t, "third-inbound", nil)
			empty.Protocol = protocol
			if err := service.ValidateClients(empty, []model.Client{newClient}, ""); err != nil {
				t.Fatalf("creating a client with inbound-scoped credentials failed: %v", err)
			}
		})
	}
}
