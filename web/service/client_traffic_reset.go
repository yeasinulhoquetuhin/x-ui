package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

func validateClientTrafficReset(period string, days int) error {
	switch period {
	case "", "never", "daily", "weekly", "monthly":
		return nil
	case "custom":
		if days >= 1 && days <= 36500 {
			return nil
		}
	}
	return fmt.Errorf("invalid auto traffic reset; custom days must be 1-36500")
}

// nextClientTrafficReset is anchored to activation time. Monthly cycles clamp
// to the last day of shorter months without drifting Jan 31 -> Feb 28 -> Mar 28.
func nextClientTrafficReset(period string, days int, anchor, after time.Time) time.Time {
	if period == "" || period == "never" {
		return time.Time{}
	}
	if period == "monthly" {
		months := (after.Year()-anchor.Year())*12 + int(after.Month()-anchor.Month())
		if months < 1 {
			months = 1
		}
		for {
			first := time.Date(anchor.Year(), anchor.Month()+time.Month(months), 1, anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), anchor.Location())
			lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, anchor.Location()).Day()
			candidate := first.AddDate(0, 0, min(anchor.Day(), lastDay)-1)
			if candidate.After(after) {
				return candidate
			}
			months++
		}
	}
	if period == "daily" {
		days = 1
	}
	if period == "weekly" {
		days = 7
	}
	if days <= 0 {
		return time.Time{}
	}
	cycles := int(after.Sub(anchor) / (time.Duration(days) * 24 * time.Hour))
	if cycles < 1 {
		cycles = 1
	}
	for {
		candidate := anchor.AddDate(0, 0, cycles*days)
		if candidate.After(after) {
			return candidate
		}
		cycles++
	}
}

func configureClientTrafficReset(traffic *xray.ClientTraffic, client *model.Client, now time.Time) error {
	if err := validateClientTrafficReset(client.TrafficReset, client.TrafficResetDays); err != nil {
		return err
	}
	period := client.TrafficReset
	if period == "" {
		period = "never"
	}
	if traffic.TrafficReset == period && traffic.TrafficResetDays == client.TrafficResetDays && traffic.TrafficResetStart > 0 {
		return nil
	}
	traffic.TrafficReset, traffic.TrafficResetDays = period, client.TrafficResetDays
	traffic.TrafficResetStart, traffic.NextTrafficReset = 0, 0
	if period != "never" {
		if client.ExpiryTime < 0 {
			return nil
		} // Start the cycle on first use.
		traffic.TrafficResetStart = now.UnixMilli()
		traffic.NextTrafficReset = nextClientTrafficReset(period, client.TrafficResetDays, now, now).UnixMilli()
	}
	return nil
}

// ResetDueClientTraffics catches up missed cycles after downtime. Only current
// usage is reset; total quota, all-time usage, manual enable flag and account
// expiry are preserved. Quota-disabled, unexpired clients become usable again.
func (s *InboundService) ResetDueClientTraffics(now time.Time) (bool, error) {
	clientTrafficMu.Lock()
	defer clientTrafficMu.Unlock()
	needRestart := false
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var due []xray.ClientTraffic
		if err := tx.Where("next_traffic_reset > 0 AND next_traffic_reset <= ?", now.UnixMilli()).Find(&due).Error; err != nil {
			return err
		}
		for _, traffic := range due {
			var inbound model.Inbound
			if err := tx.First(&inbound, traffic.InboundId).Error; err != nil {
				return err
			}
			var settings struct {
				Clients []model.Client `json:"clients"`
			}
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				return err
			}
			var client *model.Client
			for i := range settings.Clients {
				if settings.Clients[i].Email == traffic.Email {
					client = &settings.Clients[i]
					break
				}
			}
			if client == nil {
				continue
			}
			if err := validateClientTrafficReset(traffic.TrafficReset, traffic.TrafficResetDays); err != nil {
				return err
			}
			next := nextClientTrafficReset(traffic.TrafficReset, traffic.TrafficResetDays, time.UnixMilli(traffic.TrafficResetStart).Local(), now)
			nextMS := int64(0)
			if !next.IsZero() {
				nextMS = next.UnixMilli()
			}
			enabled := client.Enable && (traffic.ExpiryTime <= 0 || traffic.ExpiryTime > now.UnixMilli())
			if !traffic.Enable && enabled && inbound.Enable {
				needRestart = true
			}
			if err := tx.Model(&traffic).Updates(map[string]any{"up": 0, "down": 0, "enable": enabled, "next_traffic_reset": nextMS}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return needRestart, err
}
