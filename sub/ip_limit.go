package sub

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

type subscriptionIPLimit struct {
	Show  bool
	Count string
	Limit string
	Color string
}

func subscriptionIPClients(inbounds []*model.Inbound, subID string) map[string]int {
	clients := make(map[string]int)
	var inboundService service.InboundService
	for _, inbound := range inbounds {
		rows, err := inboundService.GetClients(inbound)
		if err != nil {
			continue
		}
		for _, client := range rows {
			if !client.Enable || !client.ShowIPLimit || client.SubID != subID || client.Email == "" {
				continue
			}
			previous, exists := clients[client.Email]
			if !exists || client.LimitIP > 0 && (previous == 0 || client.LimitIP < previous) {
				clients[client.Email] = client.LimitIP
			}
		}
	}
	return clients
}

func summarizeSubscriptionIPs(clients map[string]int, snapshot map[string][]xray.ClientIP, live bool) subscriptionIPLimit {
	if len(clients) == 0 {
		return subscriptionIPLimit{}
	}
	count, total := 0, 0
	unlimited := false
	for email, limit := range clients {
		if limit == 0 {
			unlimited = true
		} else {
			total += limit
		}
		unique := make(map[string]bool)
		for _, record := range snapshot[email] {
			if ip, ok := xray.NormalizeClientIP(record.IP); ok {
				unique[ip] = true
			}
		}
		count += len(unique)
	}
	maximum := strconv.Itoa(total)
	if unlimited {
		maximum = "∞"
	}
	active := strconv.Itoa(count)
	if !live {
		active = "Unavailable"
	}
	color := "green"
	if !live {
		color = "orange"
	} else if !unlimited && total > 0 && count >= total {
		color = "red"
	}
	return subscriptionIPLimit{Show: true, Count: active, Limit: maximum, Color: color}
}

func (s *SubService) subscriptionIPLimit(subID string) subscriptionIPLimit {
	inbounds, err := s.getInboundsBySubId(subID)
	if err != nil {
		return subscriptionIPLimit{}
	}
	clients := subscriptionIPClients(inbounds, subID)
	if len(clients) == 0 {
		return subscriptionIPLimit{}
	}
	snapshot, err := service.GetLiveClientIPSnapshot()
	return summarizeSubscriptionIPs(clients, snapshot, err == nil)
}
