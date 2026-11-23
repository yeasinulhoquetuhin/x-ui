package xray

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	statsService "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ClientIP represents a currently open connection, not an access-log history.
type ClientIP struct {
	IP        string `json:"ip,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Source    string `json:"source,omitempty"`
	TunnelID  string `json:"tunnelId,omitempty"`
}

func clientSourceRecord(raw string, seen int64) (ClientIP, string, bool) {
	if strings.HasPrefix(raw, "slowdns|") {
		return ClientIP{}, "", false
	}
	ip, ok := NormalizeClientIP(raw)
	if !ok {
		return ClientIP{}, "", false
	}
	// DNSTT identifiers from older cores are excluded from IP logs/counts too.
	if strings.HasPrefix(ip, "fd53:") {
		return ClientIP{}, "", false
	}
	return ClientIP{IP: ip, Timestamp: seen}, ip, true
}

func NormalizeClientIP(raw string) (string, bool) {
	ip, err := netip.ParseAddr(strings.Trim(raw, "[]"))
	if err != nil || ip.Zone() != "" {
		return "", false
	}
	ip = ip.Unmap()
	if ip.IsUnspecified() || ip.IsMulticast() {
		return "", false
	}
	return ip.String(), true
}

// GetOnlineClientIPs returns a complete live snapshot. A failed RPC is an error,
// never an empty snapshot and never a reason to label historical IPs as online.
func (x *XrayAPI) GetOnlineClientIPs() (map[string][]ClientIP, error) {
	if x.StatsServiceClient == nil {
		return nil, errors.New("xray stats API is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := *x.StatsServiceClient
	users, err := client.GetAllOnlineUsers(ctx, &statsService.GetAllOnlineUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("query online users: %w", err)
	}
	result := make(map[string][]ClientIP, len(users.Users))
	for _, name := range users.Users {
		if !strings.HasPrefix(name, "user>>>") || !strings.HasSuffix(name, ">>>online") {
			continue
		}
		email := strings.TrimSuffix(strings.TrimPrefix(name, "user>>>"), ">>>online")
		if email == "" {
			continue
		}
		response, err := client.GetStatsOnlineIpList(ctx, &statsService.GetStatsRequest{Name: name})
		if status.Code(err) == codes.NotFound {
			continue // The last connection closed between the two RPCs.
		}
		if err != nil {
			return nil, fmt.Errorf("query IPs for %s: %w", email, err)
		}
		unique := make(map[string]ClientIP)
		for raw, seen := range response.Ips {
			if record, key, ok := clientSourceRecord(raw, seen); ok {
				if previous, exists := unique[key]; !exists || seen > previous.Timestamp {
					unique[key] = record
				}
			}
		}
		for _, record := range unique {
			result[email] = append(result[email], record)
		}
		sort.Slice(result[email], func(i, j int) bool {
			a, b := result[email][i], result[email][j]
			if a.IP != b.IP {
				return a.IP < b.IP
			}
			return a.TunnelID < b.TunnelID
		})
	}
	return result, nil
}
