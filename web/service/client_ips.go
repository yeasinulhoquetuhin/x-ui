package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

var clientIPLimitsMu sync.Mutex
var clientIPSnapshotMu sync.Mutex

// WriteClientIPLimits publishes a complete atomic snapshot for the bundled
// Xray admission limiter. Existing TLS/routing/inbound settings are untouched.
func WriteClientIPLimits() error {
	clientIPLimitsMu.Lock()
	defer clientIPLimitsMu.Unlock()
	var inbounds []model.Inbound
	if err := database.GetDB().Preload("ClientStats").Where("enable = ?", true).Find(&inbounds).Error; err != nil {
		return err
	}
	limits := make(map[string]int)
	for _, inbound := range inbounds {
		trafficEnabled := make(map[string]bool)
		for _, traffic := range inbound.ClientStats {
			trafficEnabled[traffic.Email] = traffic.Enable
		}
		var settings struct {
			Clients []model.Client `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return fmt.Errorf("inbound %s: %w", inbound.Tag, err)
		}
		for _, client := range settings.Clients {
			if client.LimitIP < 0 {
				return fmt.Errorf("negative IP limit for client %s", client.Email)
			}
			if enabled, exists := trafficEnabled[client.Email]; exists && !enabled {
				continue
			}
			if client.Enable && client.Email != "" {
				previous, exists := limits[client.Email]
				if !exists || (client.LimitIP > 0 && (previous == 0 || client.LimitIP < previous)) {
					limits[client.Email] = client.LimitIP
				}
			}
		}
	}
	data, err := json.Marshal(limits)
	if err != nil {
		return err
	}
	path := xray.GetIPLimitConfigPath()
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".ip-limits-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// RefreshClientIPs replaces every row from a complete core snapshot. Offline
// clients receive [] instead of keeping old access-log entries indefinitely.
func (s *InboundService) RefreshClientIPs() error {
	clientIPSnapshotMu.Lock()
	defer clientIPSnapshotMu.Unlock()
	raw, err := os.ReadFile(xray.GetConfigPath())
	if err != nil {
		return err
	}
	var cfg xray.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	apiPort := 0
	for _, inbound := range cfg.InboundConfigs {
		if inbound.Tag == "api" {
			apiPort = inbound.Port
			break
		}
	}
	var api xray.XrayAPI
	if err := api.Init(apiPort); err != nil {
		return err
	}
	defer api.Close()
	snapshot, err := api.GetOnlineClientIPs()
	if err != nil {
		return err
	}
	return StoreClientIPSnapshot(snapshot)
}

func StoreClientIPSnapshot(snapshot map[string][]xray.ClientIP) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.InboundClientIps{}).Where("1 = 1").Update("ips", "[]").Error; err != nil {
			return err
		}
		for email, ips := range snapshot {
			if len(ips) == 0 {
				continue
			}
			encoded, err := json.Marshal(ips)
			if err != nil {
				return err
			}
			var row model.InboundClientIps
			if err := tx.Where("client_email = ?", email).FirstOrCreate(&row, model.InboundClientIps{ClientEmail: email}).Error; err != nil {
				return err
			}
			if err := tx.Model(&row).Update("ips", string(encoded)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
