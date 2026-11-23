package internet

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xtls/xray-core/transport/internet/stat"
)

// Probes only select a handler. The selected protocol must still authenticate
// the replayed, unmodified request, including its normal anti-replay checks.
type TDZSlowDNSMatcher interface {
	TDZSlowDNSHeaderSize() int
	TDZSlowDNSMatch([]byte) bool
}

type tdzDNSWorker struct {
	matcher  TDZSlowDNSMatcher
	dispatch func(stat.Connection)
	order    int
}

var tdzDNSWorkersMu sync.RWMutex
var tdzDNSWorkers = make(map[*tdzDNSWorker]struct{})

func TDZSlowDNSGRPCServices() []string {
	data, err := os.ReadFile(os.Getenv("TDZ_XRAY_SLOWDNS"))
	if err != nil {
		return nil
	}
	var cfg tdzDNSConfig
	if json.Unmarshal(data, &cfg) != nil || !cfg.Enabled {
		return nil
	}
	var names []string
	for _, route := range cfg.Routes {
		if route.Network == "grpc" {
			names = append(names, route.ServiceName)
		}
	}
	return names
}

func TDZRegisterSlowDNSWorker(address string, port int, matcher TDZSlowDNSMatcher, dispatch func(stat.Connection)) func() {
	data, err := os.ReadFile(os.Getenv("TDZ_XRAY_SLOWDNS"))
	if err != nil {
		return func() {}
	}
	var cfg tdzDNSConfig
	if json.Unmarshal(data, &cfg) != nil || !cfg.Enabled {
		return func() {}
	}
	for index, route := range cfg.Routes {
		if route.Port != port {
			continue
		}
		if route.Listen != "" && route.Listen != "0.0.0.0" && route.Listen != "::" && route.Listen != "::0" && route.Listen != address {
			continue
		}
		worker := &tdzDNSWorker{matcher: matcher, dispatch: dispatch, order: index}
		tdzDNSWorkersMu.Lock()
		tdzDNSWorkers[worker] = struct{}{}
		tdzDNSWorkersMu.Unlock()
		return func() { tdzDNSWorkersMu.Lock(); delete(tdzDNSWorkers, worker); tdzDNSWorkersMu.Unlock() }
	}
	return func() {}
}

type tdzDNSReplayConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *tdzDNSReplayConn) Read(data []byte) (int, error) { return c.reader.Read(data) }

// Transport framing has already been removed by this point. Route the native
// DNS stream by its authenticated credential without changing direct listeners
// or requiring an i<ID>. prefix on the shared NS zone.
func TDZRouteSlowDNSWorker(conn stat.Connection, current TDZSlowDNSMatcher) (stat.Connection, bool) {
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil || !TDZIsSlowDNSSource(strings.Trim(host, "[]")) {
		return conn, false
	}
	tdzDNSWorkersMu.RLock()
	workers := make([]*tdzDNSWorker, 0, len(tdzDNSWorkers))
	for worker := range tdzDNSWorkers {
		workers = append(workers, worker)
	}
	tdzDNSWorkersMu.RUnlock()
	if len(workers) < 2 {
		return conn, false
	}
	sort.Slice(workers, func(i, j int) bool {
		a, b := workers[i], workers[j]
		if a.matcher.TDZSlowDNSHeaderSize() != b.matcher.TDZSlowDNSHeaderSize() {
			return a.matcher.TDZSlowDNSHeaderSize() < b.matcher.TDZSlowDNSHeaderSize()
		}
		return a.order < b.order
	})
	reader := bufio.NewReaderSize(conn, 4096)
	wrapped := &tdzDNSReplayConn{Conn: conn, reader: reader}
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	for _, worker := range workers {
		size := worker.matcher.TDZSlowDNSHeaderSize()
		if size <= 0 || size > 4096 {
			continue
		}
		header, err := reader.Peek(size)
		if err != nil {
			break
		}
		if !worker.matcher.TDZSlowDNSMatch(header) {
			continue
		}
		conn.SetReadDeadline(time.Time{})
		if worker.matcher == current {
			return wrapped, false
		}
		worker.dispatch(wrapped)
		return nil, true
	}
	conn.SetReadDeadline(time.Time{})
	return wrapped, false
}
