package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
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

func TestResellerSettingsValidationAndSecretRedaction(t *testing.T) {
	setupClientResetDB(t)
	inbound := resellerTestInbound(t, "reseller", []model.Client{{Email: "own", ID: "own-id", Enable: true, SubID: "own-sub"}})
	other := resellerTestInbound(t, "other", []model.Client{{Email: "foreign", ID: "foreign-id", Enable: true, SubID: "foreign-sub"}})
	var service ResellerService
	form := ResellerAccessForm{Enabled: true, Path: "shop-one", Username: "reseller", Password: "long-password"}
	account, err := service.Save(inbound.Id, form)
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
	account, err = service.Save(inbound.Id, form)
	if err != nil || account.Version != version {
		t.Fatal("unchanged settings revoked session", err)
	}
	form.Path = "shop-new"
	account, err = service.Save(inbound.Id, form)
	if err != nil || account.Version == version {
		t.Fatal("path edit did not revoke sessions", err)
	}
	if _, err = service.ByPath("shop-one"); err == nil {
		t.Fatal("old path remains enabled")
	}
	if _, err = service.Save(other.Id, form); err == nil {
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
	inbound := resellerTestInbound(t, "lengths", nil)
	var service ResellerService
	for _, form := range []ResellerAccessForm{
		{Enabled: true, Path: "a", Username: "u", Password: "p"},
		{Enabled: true, Path: strings.Repeat("long-", 30), Username: strings.Repeat("user", 40), Password: strings.Repeat("password", 30)},
		{Enabled: true, Path: "xui-shop", Username: "name with spaces", Password: " x "},
	} {
		account, err := service.Save(inbound.Id, form)
		if err != nil {
			t.Fatal(err)
		}
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
	if _, err := service.Save(inbound.Id, ResellerAccessForm{Enabled: true, Path: "shared-shop", Username: "shop", Password: "shop-password"}); err == nil {
		t.Fatal("cross-inbound subscription exposed to reseller")
	}
}

func TestResellerAccessRejectsExistingCrossInboundClientIdentity(t *testing.T) {
	setupClientResetDB(t)
	inbound := resellerTestInbound(t, "identity-one", []model.Client{{Email: "shared-email", ID: "one", SubID: "sub-one"}})
	resellerTestInbound(t, "identity-two", []model.Client{{Email: "SHARED-EMAIL", ID: "two", SubID: "sub-two"}})
	var service ResellerService
	if _, err := service.Save(inbound.Id, ResellerAccessForm{Enabled: true, Path: "identity-shop", Username: "shop", Password: "shop-password"}); err == nil || !strings.Contains(err.Error(), `"shared-email"`) || !strings.Contains(err.Error(), "inbound 2") {
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
				if _, err := service.Save(inbound.Id, ResellerAccessForm{Enabled: true, Path: path, Username: "shop", Password: "password"}); err != nil {
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
