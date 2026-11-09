package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/features/stats"
	"github.com/xtls/xray-core/transport/internet"
)

type tdzIPState struct {
	firstSeen uint64
	streams   map[uint64]context.CancelFunc
}

// Slots are shared by authenticated email and canonical IP across all inbounds
// in one core instance. A mutex makes concurrent admission checks atomic.
type tdzIPLimiter struct {
	mu       sync.Mutex
	limits   map[string]int
	active   map[string]map[string]*tdzIPState
	sequence uint64
	info     os.FileInfo
}

func (l *tdzIPLimiter) reloadLocked(path string) ([]context.CancelFunc, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if l.info != nil && os.SameFile(l.info, info) && l.info.ModTime() == info.ModTime() && l.info.Size() == info.Size() {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var limits map[string]int
	if err := json.Unmarshal(data, &limits); err != nil {
		return nil, err
	}
	if limits == nil {
		return nil, fmt.Errorf("IP-limit configuration must be an object")
	}
	for email, limit := range limits {
		if email == "" || limit < 0 {
			return nil, fmt.Errorf("invalid IP limit for %q", email)
		}
	}
	l.limits, l.info = limits, info
	var stops []context.CancelFunc
	for email, ips := range l.active {
		limit, enabled := limits[email]
		if !enabled {
			for _, state := range ips {
				for _, stop := range state.streams {
					stops = append(stops, stop)
				}
			}
			delete(l.active, email)
			continue
		}
		if limit > 0 {
			for source, state := range ips {
				ip, err := netip.ParseAddr(source)
				if err != nil || !tdzPublicClientIP(ip) {
					for _, stop := range state.streams {
						stops = append(stops, stop)
					}
					delete(ips, source)
				}
			}
		}
		if limit <= 0 || len(ips) <= limit {
			continue
		}
		type candidate struct {
			ip        string
			firstSeen uint64
		}
		ordered := make([]candidate, 0, len(ips))
		for ip, state := range ips {
			ordered = append(ordered, candidate{ip, state.firstSeen})
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].firstSeen < ordered[j].firstSeen })
		for _, excess := range ordered[limit:] {
			for _, stop := range ips[excess.ip].streams {
				stops = append(stops, stop)
			}
			delete(ips, excess.ip)
		}
	}
	return stops, nil
}

// A local/private relay address cannot identify the clients behind that relay.
// Limited accounts must have a real source rather than sharing one proxy slot.
func tdzPublicClientIP(ip netip.Addr) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate()
}

func (l *tdzIPLimiter) reload(path string) {
	l.mu.Lock()
	stops, _ := l.reloadLocked(path)
	l.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}

// acquire counts streams until their outbound completes or their context is
// cancelled, including MUX and long-lived streams with no new access-log lines.
func (l *tdzIPLimiter) acquire(ctx context.Context, email, source, path string, online stats.OnlineMap) (context.Context, func(), error) {
	ip, err := netip.ParseAddr(strings.Trim(source, "[]"))
	if err != nil || ip.Zone() != "" {
		return nil, nil, fmt.Errorf("invalid client source IP %q", source)
	}
	ip = ip.Unmap()
	if ip.IsUnspecified() || ip.IsMulticast() {
		return nil, nil, fmt.Errorf("invalid client source IP %q", source)
	}
	canonical := ip.String()
	l.mu.Lock()
	var stops []context.CancelFunc
	if path != "" {
		stops, err = l.reloadLocked(path)
		if err != nil {
			l.mu.Unlock()
			return nil, nil, fmt.Errorf("IP-limit configuration unavailable: %w", err)
		}
		if _, published := l.limits[email]; !published {
			l.mu.Unlock()
			for _, stop := range stops {
				stop()
			}
			return nil, nil, fmt.Errorf("client IP policy is not published for %q", email)
		}
	}
	if l.limits[email] > 0 && !tdzPublicClientIP(ip) {
		l.mu.Unlock()
		for _, stop := range stops {
			stop()
		}
		return nil, nil, fmt.Errorf("real client IP unavailable for limited client %q (observed %s); proxy must forward the client IP", email, canonical)
	}
	if l.active == nil {
		l.active = make(map[string]map[string]*tdzIPState)
	}
	ips := l.active[email]
	if ips == nil {
		ips = make(map[string]*tdzIPState)
		l.active[email] = ips
	}
	state := ips[canonical]
	if limit := l.limits[email]; state == nil && limit > 0 && len(ips) >= limit {
		l.mu.Unlock()
		for _, stop := range stops {
			stop()
		}
		return nil, nil, fmt.Errorf("client IP limit exceeded for %q (limit %d)", email, limit)
	}
	l.sequence++
	id := l.sequence
	if state == nil {
		state = &tdzIPState{firstSeen: id, streams: make(map[uint64]context.CancelFunc)}
		ips[canonical] = state
	}
	child, cancel := context.WithCancel(ctx)
	state.streams[id] = cancel
	if online != nil {
		online.AddIP(canonical)
	}
	l.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			// Parent contexts can outlive streams (notably MUX); explicit release
			// must detach its cancellation callback as well.
			l.mu.Lock()
			delete(state.streams, id)
			if len(state.streams) == 0 && l.active[email][canonical] == state {
				delete(l.active[email], canonical)
				if len(l.active[email]) == 0 {
					delete(l.active, email)
				}
			}
			if online != nil {
				online.RemoveIP(canonical)
			}
			l.mu.Unlock()
		})
	}
	stopRelease := context.AfterFunc(child, release)
	wrappedRelease := func() { stopRelease(); release() }
	return child, wrappedRelease, nil
}

func (d *DefaultDispatcher) startTDZIPLimits() {
	path := os.Getenv("TDZ_XRAY_IP_LIMITS")
	if path == "" {
		return
	}
	d.tdzIPStop = make(chan struct{})
	stop := d.tdzIPStop
	d.tdzIPs.reload(path)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.tdzIPs.reload(path)
			case <-stop:
				return
			}
		}
	}()
}

func (d *DefaultDispatcher) admitTDZClient(ctx context.Context) (context.Context, func(), error) {
	inbound := session.InboundFromContext(ctx)
	if inbound == nil || inbound.User == nil || inbound.User.Email == "" {
		return ctx, func() {}, nil
	}
	var online stats.OnlineMap
	if d.policy.ForLevel(inbound.User.Level).Stats.UserOnline {
		online, _ = stats.GetOrRegisterOnlineMap(d.stats, "user>>>"+inbound.User.Email+">>>online")
	}
	source := inbound.Source.Address.String()
	if inbound.Local.IsValid() {
		if mapped, ok := internet.TDZLocalProxyIP(inbound.Source.NetAddr(), inbound.Local.NetAddr()); ok {
			source = mapped
		}
	}
	child, release, err := d.tdzIPs.acquire(ctx, inbound.User.Email, source, os.Getenv("TDZ_XRAY_IP_LIMITS"), online)
	if err != nil {
		return nil, nil, err
	}
	return context.WithValue(child, session.TDZWaitForMuxKey{}, true), release, nil
}
