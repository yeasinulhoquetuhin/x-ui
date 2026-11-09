package sub

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestSubscriptionIPLimitIsOptInAndCountsCanonicalIPs(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"clients": []model.Client{
		{Email: "shown", Enable: true, SubID: "own", LimitIP: 2, ShowIPLimit: true},
		{Email: "hidden", Enable: true, SubID: "own", LimitIP: 5},
		{Email: "other", Enable: true, SubID: "foreign", LimitIP: 3, ShowIPLimit: true},
	}})
	clients := subscriptionIPClients([]*model.Inbound{{Settings: string(data)}}, "own")
	snapshot := map[string][]xray.ClientIP{"shown": {{IP: "192.0.2.1"}, {IP: "::ffff:192.0.2.1"}}, "hidden": {{IP: "192.0.2.2"}}, "other": {{IP: "192.0.2.3"}}}
	got := summarizeSubscriptionIPs(clients, snapshot, true)
	if !got.Show || got.Count != "1" || got.Limit != "2" {
		t.Fatalf("wrong display: %+v", got)
	}
	if got := summarizeSubscriptionIPs(clients, map[string][]xray.ClientIP{}, true); got.Count != "0" {
		t.Fatal("offline count must be zero")
	}
	if got := summarizeSubscriptionIPs(clients, nil, false); got.Count != "Unavailable" {
		t.Fatal("failed live API presented as offline")
	}
	if got := summarizeSubscriptionIPs(nil, snapshot, true); got.Show {
		t.Fatal("hidden client revealed IP limit")
	}
}
