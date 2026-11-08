package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestClientTelegramIDCompatibility(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{
		{`0`, 0}, {`123456789`, 123456789}, {`-1001234567890`, -1001234567890},
		{`""`, 0}, {`"   "`, 0}, {`"0"`, 0}, {`"123456789"`, 123456789},
		{`" -1001234567890 "`, -1001234567890}, {`null`, 0},
		{`"9223372036854775807"`, 9223372036854775807},
	} {
		t.Run(tc.value, func(t *testing.T) {
			var client Client
			data := `{"id":"uuid","email":"client","limitIp":1,"enable":true,"totalGB":100,"expiryTime":1234,"trafficReset":"monthly","tgId":` + tc.value + `}`
			if err := json.Unmarshal([]byte(data), &client); err != nil {
				t.Fatal(err)
			}
			want := Client{ID: "uuid", Email: "client", LimitIP: 1, Enable: true, TotalGB: 100, ExpiryTime: 1234, TrafficReset: "monthly", TgID: tc.want}
			if !reflect.DeepEqual(client, want) {
				t.Fatalf("client fields changed: got %+v, want %+v", client, want)
			}
			encoded, err := json.Marshal(client)
			if err != nil {
				t.Fatal(err)
			}
			var again Client
			if err := json.Unmarshal(encoded, &again); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(again, client) {
				t.Fatal("client did not round-trip")
			}
		})
	}
	client := Client{TgID: 99}
	if err := json.Unmarshal([]byte(`{"email":"missing-id"}`), &client); err != nil || client.TgID != 0 {
		t.Fatalf("missing Telegram ID was not reset: %+v (%v)", client, err)
	}
}

func TestInvalidTelegramIDDoesNotCorruptClient(t *testing.T) {
	for _, value := range []string{`"bad-id"`, `true`, `{}`, `[]`, `1.5`, `"9223372036854775808"`} {
		before := Client{Email: "existing", TgID: 42}
		client := before
		if err := json.Unmarshal([]byte(`{"email":"changed","tgId":`+value+`}`), &client); err == nil {
			t.Fatalf("invalid tgId accepted: %s", value)
		}
		if !reflect.DeepEqual(client, before) {
			t.Fatalf("invalid input partially modified client: %+v", client)
		}
	}
}
