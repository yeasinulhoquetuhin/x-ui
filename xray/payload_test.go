package xray

import (
	"encoding/json"
	"testing"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/infra/conf"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestPayloadAPIWireConfig(t *testing.T) {
	var input conf.InboundDetourConfig
	err := json.Unmarshal([]byte(`{"listen":"127.0.0.1","port":12345,"protocol":"vless","settings":{"clients":[],"decryption":"none"},"streamSettings":{"network":"tcp","tcpSettings":{"injectPayload":true}}}`), &input)
	if err != nil {
		t.Fatal(err)
	}
	config, err := input.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err = applyPayloadToAPI(config, true); err != nil {
		t.Fatal(err)
	}
	instance, err := config.ReceiverSettings.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	receiver := instance.(*proxyman.ReceiverConfig)
	found := false
	{
		data := receiver.StreamSettings.ProtoReflect().GetUnknown()
		for len(data) > 0 {
			number, kind, n := protowire.ConsumeTag(data)
			if n < 0 {
				t.Fatal("bad transport wire data")
			}
			data = data[n:]
			if number == 13 && kind == protowire.VarintType {
				value, _ := protowire.ConsumeVarint(data)
				found = value == 1
			}
			n = protowire.ConsumeFieldValue(number, kind, data)
			if n < 0 {
				t.Fatal("bad field")
			}
			data = data[n:]
		}
	}
	if !found {
		t.Fatal("live API lost Accept Payloads")
	}
}

func TestPayloadValidation(t *testing.T) {
	for _, value := range []string{`{"network":"kcp","acceptPayloads":true}`, `{"network":"tcp","tcpSettings":{"injectPayload":true,"header":{"type":"http"}}}`, `{"tcpSettings":{"injectPayload":"true"}}`} {
		if _, err := InjectPayloadEnabled([]byte(value)); err == nil {
			t.Fatalf("invalid combination accepted: %s", value)
		}
	}
	for _, value := range []string{`{}`, `{"network":"tcp"}`, `{"network":"ws","wsSettings":{"path":"/"}}`} {
		if enabled, err := InjectPayloadEnabled([]byte(value)); err != nil || enabled {
			t.Fatalf("default changed: %s %v", value, err)
		}
	}
}
