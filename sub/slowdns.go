package sub

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// These are connection hints, separate from standard URI/JSON/Clash profiles.
// Private keys and actual source addresses are never subscription metadata.
type subscriptionSlowDNS struct {
	Client    string `json:"client"`
	Protocol  string `json:"protocol"`
	Transport string `json:"transport"`
	NSDomain  string `json:"nsDomain"`
	PublicKey string `json:"publicKey"`
}

func (s *SubService) subscriptionSlowDNS(subID string) []subscriptionSlowDNS {
	result := make([]subscriptionSlowDNS, 0)
	var slowDNSService service.SlowDNSService
	cfg, err := slowDNSService.GetConfig()
	if err != nil || !cfg.Installed || cfg.PublicKey == "" {
		return result
	}
	inbounds, err := s.getInboundsBySubId(subID)
	if err != nil {
		return result
	}
	for _, inbound := range inbounds {
		enabled, err := xray.SlowDNSEnabled(string(inbound.Protocol), []byte(inbound.StreamSettings))
		if err != nil || !enabled {
			continue
		}
		clients, err := s.inboundService.GetClients(inbound)
		if err != nil {
			continue
		}
		var stream struct {
			Network string `json:"network"`
		}
		if json.Unmarshal([]byte(inbound.StreamSettings), &stream) != nil {
			continue
		}
		for _, client := range clients {
			if client.Enable && client.SubID == subID && client.ShowSlowDNS {
				result = append(result, subscriptionSlowDNS{Client: client.Email, Protocol: string(inbound.Protocol), Transport: xray.SlowDNSNetwork(stream.Network), NSDomain: cfg.NSDomain, PublicKey: cfg.PublicKey})
			}
		}
	}
	return result
}
