package service

import (
	_ "embed"
	"encoding/json"

	"github.com/mhsanaei/3x-ui/v2/util/common"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// XraySettingService provides business logic for Xray configuration management.
// It handles validation and storage of Xray template configurations.
type XraySettingService struct {
	SettingService
}

func (s *XraySettingService) SaveXraySetting(newXraySettings string) error {
	// The frontend round-trips the whole getXraySetting response back
	// through the textarea, so if it has ever received a wrapped
	// payload (see UnwrapXrayTemplateConfig) it sends that same wrapper
	// back here. Strip it before validation/storage, otherwise we save
	// garbage the next read can't recover from without this same call.
	newXraySettings = UnwrapXrayTemplateConfig(newXraySettings)
	var err error
	newXraySettings, err = normalizeXrayTrackingConfig(newXraySettings)
	if err != nil {
		return err
	}
	if err := s.CheckXrayConfig(newXraySettings); err != nil {
		return err
	}
	return s.SettingService.saveSetting("xrayTemplateConfig", newXraySettings)
}

// normalizeXrayTrackingConfig repairs the old installer default and enables the
// core's connection-based IP counters. Other logging and policy fields survive.
func normalizeXrayTrackingConfig(raw string) (string, error) {
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return "", err
	}
	if cfg == nil {
		return "", common.NewError("xray template config must be an object")
	}
	var logConfig map[string]any
	if err := json.Unmarshal(cfg["log"], &logConfig); len(cfg["log"]) > 0 && err != nil {
		return "", err
	}
	if logConfig == nil {
		logConfig = make(map[string]any)
	}
	if path, _ := logConfig["access"].(string); path == "" || path == "/var/log/xray/access.log" {
		logConfig["access"] = "./access.log"
	}
	cfg["log"], _ = json.Marshal(logConfig)
	var policy map[string]json.RawMessage
	if err := json.Unmarshal(cfg["policy"], &policy); len(cfg["policy"]) > 0 && err != nil {
		return "", err
	}
	if policy == nil {
		policy = make(map[string]json.RawMessage)
	}
	var levels map[string]map[string]any
	if err := json.Unmarshal(policy["levels"], &levels); len(policy["levels"]) > 0 && err != nil {
		return "", err
	}
	if levels == nil {
		levels = make(map[string]map[string]any)
	}
	if levels["0"] == nil {
		levels["0"] = make(map[string]any)
	}
	for _, level := range levels {
		if level != nil {
			level["statsUserOnline"] = true
		}
	}
	policy["levels"], _ = json.Marshal(levels)
	cfg["policy"], _ = json.Marshal(policy)
	var api map[string]any
	if err := json.Unmarshal(cfg["api"], &api); len(cfg["api"]) > 0 && err != nil {
		return "", err
	}
	if api == nil {
		api = map[string]any{"tag": "api"}
	}
	services, _ := api["services"].([]any)
	for _, required := range []string{"StatsService"} {
		found := false
		for _, value := range services {
			found = found || value == required
		}
		if !found {
			services = append(services, required)
		}
	}
	api["services"] = services
	cfg["api"], _ = json.Marshal(api)
	data, err := json.MarshalIndent(cfg, "", "  ")
	return string(data), err
}

func (s *XraySettingService) CheckXrayConfig(XrayTemplateConfig string) error {
	xrayConfig := &xray.Config{}
	err := json.Unmarshal([]byte(XrayTemplateConfig), xrayConfig)
	if err != nil {
		return common.NewError("xray template config invalid:", err)
	}
	return nil
}

// UnwrapXrayTemplateConfig returns the raw xray config JSON from `raw`,
// peeling off any number of `{ "inboundTags": ..., "outboundTestUrl": ...,
// "xraySetting": <real config> }` response-shaped wrappers that may have
// ended up in the database.
//
// How it got there: getXraySetting used to embed the raw DB value as
// `xraySetting` in its response without checking whether the stored
// value was already that exact response shape. If the frontend then
// saved it verbatim (the textarea is a round-trip of the JSON it was
// handed), the wrapper got persisted — and each subsequent save nested
// another layer, producing the blank Xray Settings page reported in
// issue #4059.
//
// If `raw` does not look like a wrapper, it is returned unchanged.
func UnwrapXrayTemplateConfig(raw string) string {
	const maxDepth = 8 // defensive cap against pathological multi-nest values
	for i := 0; i < maxDepth; i++ {
		var top map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &top); err != nil {
			return raw
		}
		inner, ok := top["xraySetting"]
		if !ok {
			return raw
		}
		// Real xray configs never contain a top-level "xraySetting" key,
		// but they do contain things like "inbounds"/"outbounds"/"api".
		// If any of those are present, we're already at the real config
		// and the "xraySetting" field is either user data or coincidence
		// — don't touch it.
		for _, k := range []string{"inbounds", "outbounds", "routing", "api", "dns", "log", "policy", "stats"} {
			if _, hit := top[k]; hit {
				return raw
			}
		}
		// Peel off one layer.
		unwrapped := string(inner)
		// `xraySetting` may be stored either as a JSON object or as a
		// JSON-encoded string of an object. Handle both.
		var asStr string
		if err := json.Unmarshal(inner, &asStr); err == nil {
			unwrapped = asStr
		}
		raw = unwrapped
	}
	return raw
}
