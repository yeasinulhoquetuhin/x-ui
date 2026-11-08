package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnwrapXrayTemplateConfig(t *testing.T) {
	real := `{"log":{},"inbounds":[],"outbounds":[],"routing":{}}`

	t.Run("passes through a clean config", func(t *testing.T) {
		if got := UnwrapXrayTemplateConfig(real); got != real {
			t.Fatalf("clean config was modified: %s", got)
		}
	})

	t.Run("passes through invalid JSON unchanged", func(t *testing.T) {
		in := "not json at all"
		if got := UnwrapXrayTemplateConfig(in); got != in {
			t.Fatalf("invalid input was modified: %s", got)
		}
	})

	t.Run("unwraps one layer of response-shaped wrapper", func(t *testing.T) {
		wrapper := `{"inboundTags":["tag"],"outboundTestUrl":"x","xraySetting":` + real + `}`
		got := UnwrapXrayTemplateConfig(wrapper)
		if !equalJSON(t, got, real) {
			t.Fatalf("want %s, got %s", real, got)
		}
	})

	t.Run("unwraps multiple stacked layers", func(t *testing.T) {
		lvl1 := `{"xraySetting":` + real + `}`
		lvl2 := `{"xraySetting":` + lvl1 + `}`
		lvl3 := `{"xraySetting":` + lvl2 + `}`
		got := UnwrapXrayTemplateConfig(lvl3)
		if !equalJSON(t, got, real) {
			t.Fatalf("want %s, got %s", real, got)
		}
	})

	t.Run("handles an xraySetting stored as a JSON-encoded string", func(t *testing.T) {
		encoded, _ := json.Marshal(real) // becomes a quoted string
		wrapper := `{"xraySetting":` + string(encoded) + `}`
		got := UnwrapXrayTemplateConfig(wrapper)
		if !equalJSON(t, got, real) {
			t.Fatalf("want %s, got %s", real, got)
		}
	})

	t.Run("does not unwrap when top level already has real xray keys", func(t *testing.T) {
		// Pathological but defensible: if a user's actual config somehow
		// has both the real keys and an unrelated `xraySetting` key, we
		// must not strip it.
		in := `{"inbounds":[],"xraySetting":{"some":"thing"}}`
		got := UnwrapXrayTemplateConfig(in)
		if got != in {
			t.Fatalf("should have left real config alone, got %s", got)
		}
	})

	t.Run("stops at a reasonable depth", func(t *testing.T) {
		// Build a deeper-than-maxDepth chain that ends at something
		// non-wrapped, and confirm we end up at some valid JSON (we
		// don't loop forever and we don't blow the stack).
		s := real
		for i := 0; i < 16; i++ {
			s = `{"xraySetting":` + s + `}`
		}
		got := UnwrapXrayTemplateConfig(s)
		if !strings.Contains(got, `"inbounds"`) && !strings.Contains(got, `"xraySetting"`) {
			t.Fatalf("unexpected tail: %s", got)
		}
	})
}

func equalJSON(t *testing.T, a, b string) bool {
	t.Helper()
	var va, vb any
	if err := json.Unmarshal([]byte(a), &va); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &vb); err != nil {
		return false
	}
	ja, _ := json.Marshal(va)
	jb, _ := json.Marshal(vb)
	return string(ja) == string(jb)
}

func TestTrackingConfigRepairsLegacyPathAndPreservesSettings(t *testing.T) {
	for _, path := range []string{"/var/log/xray/access.log", "./access.log", "/custom/access.log", "none"} {
		t.Run(path, func(t *testing.T) {
			raw := `{"log":{"access":"` + path + `","error":"./errors.log","loglevel":"debug"},"policy":{"levels":{"0":{"connIdle":90},"3":{"statsUserUplink":true}},"system":{"statsInboundUplink":true}},"api":{"tag":"api","services":["HandlerService"]},"routing":{"rules":[{"type":"field","port":"53","outboundTag":"dns"}]}}`
			got, err := normalizeXrayTrackingConfig(raw)
			if err != nil {
				t.Fatal(err)
			}
			var cfg struct {
				Log    map[string]any `json:"log"`
				Policy struct {
					Levels map[string]map[string]any `json:"levels"`
					System map[string]any            `json:"system"`
				} `json:"policy"`
				API struct {
					Services []string `json:"services"`
				} `json:"api"`
				Routing json.RawMessage `json:"routing"`
			}
			if err := json.Unmarshal([]byte(got), &cfg); err != nil {
				t.Fatal(err)
			}
			want := path
			if path == "/var/log/xray/access.log" {
				want = "./access.log"
			}
			if cfg.Log["access"] != want || cfg.Log["error"] != "./errors.log" || cfg.Log["loglevel"] != "debug" {
				t.Fatalf("logging damaged: %s", got)
			}
			if cfg.Policy.Levels["0"]["connIdle"] != float64(90) || cfg.Policy.Levels["3"]["statsUserOnline"] != true || cfg.Policy.System["statsInboundUplink"] != true {
				t.Fatalf("policy damaged: %s", got)
			}
			if len(cfg.API.Services) != 2 || !equalJSON(t, string(cfg.Routing), `{"rules":[{"type":"field","port":"53","outboundTag":"dns"}]}`) {
				t.Fatalf("custom config lost: %s", got)
			}
			second, err := normalizeXrayTrackingConfig(got)
			if err != nil || second != got {
				t.Fatal("configuration repair is not idempotent")
			}
		})
	}
}
