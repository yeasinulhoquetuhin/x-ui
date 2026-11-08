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
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
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
		unique := make(map[string]int64)
		for raw, seen := range response.Ips {
			if ip, ok := NormalizeClientIP(raw); ok {
				if previous, exists := unique[ip]; !exists || seen > previous {
					unique[ip] = seen
				}
			}
		}
		for ip, seen := range unique {
			result[email] = append(result[email], ClientIP{IP: ip, Timestamp: seen})
		}
		sort.Slice(result[email], func(i, j int) bool { return result[email][i].IP < result[email][j].IP })
	}
	return result, nil
}
