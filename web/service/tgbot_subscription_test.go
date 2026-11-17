package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/global"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

type testBotSubscriptionAPI struct{ subID, host string }

func (p *testBotSubscriptionAPI) GetCtx() context.Context { return context.Background() }
func (p *testBotSubscriptionAPI) GetSubscriptionLinks(id, host string) ([]string, error) {
	p.subID, p.host = id, host
	return []string{"vless://local-api-result"}, nil
}

func TestBotSubscriptionUsesCertificateNameAndLocalAPI(t *testing.T) {
	setupClientResetDB(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"sgx.tuhinbro.online"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"subCertFile": certFile, "subKeyFile": "private-key-path", "subPath": "/sgx-tdz/"} {
		if err := database.GetDB().Create(&model.Setting{Key: name, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	inbound := model.Inbound{UserId: 1, Tag: "bot", Protocol: model.VLESS, Settings: `{"clients":[{"id":"11111111-1111-4111-8111-111111111111","email":"client","subId":"local-sub","enable":true}],"decryption":"none"}`, StreamSettings: `{"network":"tcp"}`}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&xray.ClientTraffic{InboundId: inbound.Id, Email: "client", Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	old := global.GetSubServer()
	api := &testBotSubscriptionAPI{}
	global.SetSubServer(api)
	t.Cleanup(func() { global.SetSubServer(old) })
	var bot Tgbot
	url, _, err := bot.buildSubscriptionURLs("client")
	if err != nil || url != "https://sgx.tuhinbro.online:2096/sgx-tdz/local-sub" {
		t.Fatal(url, err)
	}
	links, err := bot.ClientIndividualLinks("client")
	if err != nil || len(links) != 1 || api.subID != "local-sub" || api.host != "sgx.tuhinbro.online" {
		t.Fatal(links, err, api)
	}
	// This intentionally unreachable public URL must not be fetched.
	if err := database.GetDB().Create(&model.Setting{Key: "subURI", Value: "https://myx.tdz-server.store:2096/myx-tdz/"}).Error; err != nil {
		t.Fatal(err)
	}
	links, err = bot.ClientIndividualLinks("client")
	if err != nil || len(links) != 1 || !strings.EqualFold(api.host, "myx.tdz-server.store") {
		t.Fatal(links, err, api.host)
	}
}
