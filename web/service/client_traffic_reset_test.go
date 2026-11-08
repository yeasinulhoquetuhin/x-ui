package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func setupClientResetDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "reset.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func TestClientTrafficResetSchedule(t *testing.T) {
	anchor := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		period      string
		days        int
		after, want time.Time
	}{
		{"daily", 0, anchor, anchor.AddDate(0, 0, 1)},
		{"weekly", 0, anchor.AddDate(0, 0, 7), anchor.AddDate(0, 0, 14)},
		{"custom", 15, anchor.AddDate(0, 0, 46), anchor.AddDate(0, 0, 60)},
		{"monthly", 0, anchor, time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)},
		{"monthly", 0, time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC), time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)},
		{"monthly", 0, time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC), time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.period+tc.after.Format("2006-01-02"), func(t *testing.T) {
			if got := nextClientTrafficReset(tc.period, tc.days, anchor, tc.after); !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	if !nextClientTrafficReset("never", 0, anchor, anchor).IsZero() {
		t.Fatal("disabled schedule has a deadline")
	}
	for _, days := range []int{-1, 0, 36501} {
		if validateClientTrafficReset("custom", days) == nil {
			t.Fatalf("invalid days accepted: %d", days)
		}
	}
}

func TestClientTrafficResetPreservesSixMonthAccount(t *testing.T) {
	setupClientResetDB(t)
	var service InboundService
	start := time.Date(2026, 1, 31, 12, 0, 0, 0, time.Local)
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.Local)
	const quota = int64(100 * 1024 * 1024 * 1024)
	clients := []model.Client{
		{Email: "monthly", Enable: true, TotalGB: quota, ExpiryTime: start.AddDate(0, 6, 0).UnixMilli(), TrafficReset: "monthly"},
		{Email: "expired", Enable: true, TotalGB: quota, ExpiryTime: start.AddDate(0, 0, 10).UnixMilli(), TrafficReset: "monthly"},
		{Email: "manual-off", Enable: false, TotalGB: quota, ExpiryTime: start.AddDate(0, 6, 0).UnixMilli(), TrafficReset: "monthly"},
	}
	settings, _ := json.Marshal(map[string]any{"clients": clients, "decryption": "none"})
	inbound := model.Inbound{Tag: "test", Enable: true, Protocol: model.VLESS, Settings: string(settings)}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	for _, client := range clients {
		traffic := xray.ClientTraffic{InboundId: inbound.Id, Email: client.Email, Total: quota, ExpiryTime: client.ExpiryTime, Up: quota, Down: 1024, AllTime: quota + 1024, Enable: false}
		if err := configureClientTrafficReset(&traffic, &client, start); err != nil {
			t.Fatal(err)
		}
		if err := database.GetDB().Create(&traffic).Error; err != nil {
			t.Fatal(err)
		}
	}
	restart, err := service.ResetDueClientTraffics(now)
	if err != nil {
		t.Fatal(err)
	}
	if !restart {
		t.Fatal("quota-disabled account needs core reload to be enabled")
	}
	for _, client := range clients {
		var traffic xray.ClientTraffic
		database.GetDB().Where("email = ?", client.Email).First(&traffic)
		if traffic.Up != 0 || traffic.Down != 0 {
			t.Fatalf("usage not reset: %+v", traffic)
		}
		if traffic.Total != quota || traffic.AllTime != quota+1024 || traffic.ExpiryTime != client.ExpiryTime {
			t.Fatalf("quota/all-time/expiry changed: %+v", traffic)
		}
		if traffic.Enable != (client.Email == "monthly") {
			t.Fatalf("wrong enable state for %s", client.Email)
		}
		if traffic.NextTrafficReset != time.Date(2026, 3, 31, 12, 0, 0, 0, time.Local).UnixMilli() {
			t.Fatalf("monthly schedule drifted: %+v", traffic)
		}
	}
	// Re-running at the same time must not wipe this cycle's new usage.
	database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "monthly").Update("up", 1234)
	if _, err := service.ResetDueClientTraffics(now); err != nil {
		t.Fatal(err)
	}
	var traffic xray.ClientTraffic
	database.GetDB().Where("email = ?", "monthly").First(&traffic)
	if traffic.Up != 1234 {
		t.Fatal("traffic reset ran twice in the same cycle")
	}
}

func TestEditingClientDoesNotRestartTrafficCycle(t *testing.T) {
	setupClientResetDB(t)
	var service InboundService
	start := time.Now().AddDate(0, 0, -10)
	client := model.Client{Email: "client", Enable: true, TotalGB: 100, TrafficReset: "monthly"}
	traffic := xray.ClientTraffic{Email: client.Email, Enable: true, Up: 75}
	if err := configureClientTrafficReset(&traffic, &client, start); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	client.Comment = "updated comment"
	if err := service.UpdateClientStat(database.GetDB(), client.Email, &client); err != nil {
		t.Fatal(err)
	}
	var updated xray.ClientTraffic
	database.GetDB().First(&updated, traffic.Id)
	if updated.TrafficResetStart != traffic.TrafficResetStart || updated.NextTrafficReset != traffic.NextTrafficReset || updated.Up != 75 {
		t.Fatalf("editing moved cycle or usage: %+v", updated)
	}
	client.TrafficReset = "never"
	if err := service.UpdateClientStat(database.GetDB(), client.Email, &client); err != nil {
		t.Fatal(err)
	}
	database.GetDB().First(&updated, traffic.Id)
	if updated.NextTrafficReset != 0 || updated.TrafficResetStart != 0 {
		t.Fatal("disabling schedule retained deadline")
	}
}

func TestDelayedStartTrafficResetWaitsForActivation(t *testing.T) {
	client := model.Client{TrafficReset: "monthly", ExpiryTime: -180 * 86400000}
	var traffic xray.ClientTraffic
	if err := configureClientTrafficReset(&traffic, &client, time.Now()); err != nil {
		t.Fatal(err)
	}
	if traffic.NextTrafficReset != 0 || traffic.TrafficResetStart != 0 {
		t.Fatal("cycle started before delayed account activation")
	}
	client.ExpiryTime = time.Now().AddDate(0, 6, 0).UnixMilli()
	if err := configureClientTrafficReset(&traffic, &client, time.Now()); err != nil {
		t.Fatal(err)
	}
	if traffic.NextTrafficReset <= time.Now().UnixMilli() {
		t.Fatal("cycle did not start on activation")
	}
}
