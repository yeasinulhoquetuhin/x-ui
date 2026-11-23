package internet

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
	"www.bamsoftware.com/git/dnstt.git/dns"
	"www.bamsoftware.com/git/dnstt.git/noise"
	"www.bamsoftware.com/git/dnstt.git/turbotunnel"
)

const tdzDNSIdleTimeout = 2 * time.Minute

// The stock client backs idle DNS polling off to ten seconds. A two-second
// server keepalive prevents that backoff during a live session; this lease
// tolerates several lost polls without keeping a stopped app online for minutes.
const tdzDNSLeaseTimeout = 15 * time.Second

type tdzDNSRoute struct {
	Domain, Listen, Network string
	Port                    int
	Path, Host, ServiceName string
	AcceptPayloads          bool
}

type tdzDNSConfig struct {
	Enabled                                  bool
	Listen, HostDomain, NSDomain, PrivateKey string
	MTU                                      int
	Routes                                   []tdzDNSRoute
}

var tdzDNSHubMu sync.Mutex
var tdzDNSHubInstance *tdzDNSHub
var tdzDNSSources sync.Map

// Native session identities occupy fd53::/16. They are not public IP addresses.
// Only a currently registered, in-process DNSTT session can admit such a source.
func TDZIsSlowDNSSource(source string) bool {
	value, ok := tdzDNSSources.Load(source)
	return ok && value.(*tdzDNSSession).ctx.Err() == nil
}

// Online-map keys retain a separate identity per authenticated tunnel. They are
// not public IP addresses and are presented as numbered sessions by the panel.
func TDZSlowDNSSourceInfo(source string) (context.Context, string, bool) {
	value, ok := tdzDNSSources.Load(source)
	if !ok {
		return nil, "", false
	}
	session := value.(*tdzDNSSession)
	if session.ctx.Err() != nil {
		return nil, "", false
	}
	return session.ctx, "slowdns|" + source, true
}

type tdzDNSActivity struct {
	lastQuery atomic.Int64
	resolver  string
}

type tdzDNSSession struct {
	ctx      context.Context
	cancel   context.CancelFunc
	activity *tdzDNSActivity
}

type tdzDNSHub struct {
	conn          net.PacketConn
	config        tdzDNSConfig
	mu            sync.RWMutex
	endpoints     map[string]*tdzDNSListener
	zone          dns.Name
	labels        int
	primaryDomain string
}

type tdzDNSRecord struct {
	response *dns.Message
	address  net.Addr
	clientID turbotunnel.ClientID
}

type tdzDNSListener struct {
	net.Listener
	hub         *tdzDNSHub
	route       tdzDNSRoute
	domain      dns.Name
	domainKey   string
	queue       *turbotunnel.QueuePacketConn
	kcpListener *kcp.Listener
	connections chan net.Conn
	errors      chan error
	records     chan *tdzDNSRecord
	done        chan struct{}
	closeOnce   sync.Once
	mu          sync.Mutex
	sessions    map[*kcp.UDPSession]*tdzDNSSession
	activities  map[turbotunnel.ClientID]*tdzDNSActivity
	closed      bool
}

// Register the DNS stream alongside the ordinary TCP listener. There is one
// shared UDP socket for every enabled inbound, and no loopback forwarding hop.
func TDZSlowDNSListener(listener net.Listener, network string) (net.Listener, error) {
	path := os.Getenv("TDZ_XRAY_SLOWDNS")
	if path == "" {
		return listener, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return listener, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return nil, errors.New("invalid SlowDNS runtime configuration file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg tdzDNSConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return listener, nil
	}
	host, portString, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return listener, nil // Unix socket transports are not eligible.
	}
	port, _ := strconv.Atoi(portString)
	var route *tdzDNSRoute
	for i := range cfg.Routes {
		candidate := &cfg.Routes[i]
		if candidate.Port != port || network != "" && candidate.Network != network {
			continue
		}
		switch candidate.Network {
		case "tcp", "websocket", "httpupgrade", "grpc", "splithttp":
		default:
			continue
		}
		if candidate.Listen != "" && candidate.Listen != "0.0.0.0" && candidate.Listen != "::" && candidate.Listen != "::0" && candidate.Listen != host {
			continue
		}
		route = candidate
		break
	}
	if route == nil {
		return listener, nil
	}
	if cfg.MTU < 512 || cfg.MTU > 4096 {
		return nil, errors.New("SlowDNS MTU must be between 512 and 4096")
	}
	if _, err := noise.DecodeKey(cfg.PrivateKey); err != nil {
		return nil, fmt.Errorf("SlowDNS private key: %w", err)
	}
	domain, err := dns.ParseName(route.Domain)
	if err != nil {
		return nil, err
	}
	zone, err := dns.ParseName(cfg.NSDomain)
	if err != nil || len(zone) == 0 || len(domain) != len(zone)+1 {
		return nil, errors.New("SlowDNS route must be an inbound label under the configured NS domain")
	}
	maxPayload := tdzDNSComputeMaxPayload(cfg.MTU)
	if maxPayload-2 < 80 {
		return nil, errors.New("SlowDNS MTU is too small for DNSTT framing")
	}
	wrapped := &tdzDNSListener{Listener: listener, route: *route, domain: domain, domainKey: strings.ToLower(route.Domain), connections: make(chan net.Conn, 64), errors: make(chan error, 1), records: make(chan *tdzDNSRecord, 100), done: make(chan struct{}), sessions: make(map[*kcp.UDPSession]*tdzDNSSession), activities: make(map[turbotunnel.ClientID]*tdzDNSActivity)}
	wrapped.queue = turbotunnel.NewQueuePacketConn(turbotunnel.DummyAddr{}, tdzDNSIdleTimeout*2)
	wrapped.kcpListener, err = kcp.ServeConn(nil, 0, 0, wrapped.queue)
	if err != nil {
		wrapped.queue.Close()
		return nil, err
	}
	tdzDNSHubMu.Lock()
	hub := tdzDNSHubInstance
	if hub == nil {
		conn, err := net.ListenPacket("udp", cfg.Listen)
		if err != nil {
			tdzDNSHubMu.Unlock()
			wrapped.kcpListener.Close()
			wrapped.queue.Close()
			return nil, fmt.Errorf("SlowDNS UDP listener: %w", err)
		}
		hub = &tdzDNSHub{conn: conn, config: cfg, endpoints: make(map[string]*tdzDNSListener), zone: zone, labels: len(domain), primaryDomain: strings.ToLower(cfg.Routes[0].Domain)}
		tdzDNSHubInstance = hub
		go hub.readLoop()
		go hub.leaseLoop()
	} else if hub.config.Listen != cfg.Listen || hub.config.PrivateKey != cfg.PrivateKey || hub.config.MTU != cfg.MTU || hub.config.NSDomain != cfg.NSDomain || hub.config.HostDomain != cfg.HostDomain {
		tdzDNSHubMu.Unlock()
		wrapped.kcpListener.Close()
		wrapped.queue.Close()
		return nil, errors.New("restart Xray to apply the changed SlowDNS configuration")
	}
	hub.mu.Lock()
	if _, exists := hub.endpoints[wrapped.domainKey]; exists {
		hub.mu.Unlock()
		tdzDNSHubMu.Unlock()
		wrapped.kcpListener.Close()
		wrapped.queue.Close()
		return nil, errors.New("duplicate SlowDNS inbound domain")
	}
	wrapped.hub = hub
	hub.endpoints[wrapped.domainKey] = wrapped
	hub.mu.Unlock()
	tdzDNSHubMu.Unlock()
	go wrapped.acceptTCP()
	go wrapped.acceptDNS(maxPayload - 2)
	go wrapped.sendLoop(maxPayload)
	return wrapped, nil
}

func (l *tdzDNSListener) offer(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		conn.Close()
		return
	}
	select {
	case l.connections <- conn:
	default:
		conn.Close()
	}
}

func (l *tdzDNSListener) acceptTCP() {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			select {
			case l.errors <- err:
			default:
			}
			return
		}
		l.offer(conn)
	}
}

func (l *tdzDNSListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	case conn := <-l.connections:
		return conn, nil
	case err := <-l.errors:
		return nil, err
	}
}

func (l *tdzDNSListener) Close() error {
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		close(l.done)
		for conn, session := range l.sessions {
			session.cancel()
			conn.Close()
		}
		for {
			select {
			case conn := <-l.connections:
				conn.Close()
			default:
				goto drained
			}
		}
	drained:
		l.mu.Unlock()
		l.Listener.Close()
		l.kcpListener.Close()
		l.queue.Close()
		tdzDNSHubMu.Lock()
		l.hub.mu.Lock()
		delete(l.hub.endpoints, l.domainKey)
		if len(l.hub.endpoints) == 0 {
			l.hub.conn.Close()
			if tdzDNSHubInstance == l.hub {
				tdzDNSHubInstance = nil
			}
		}
		l.hub.mu.Unlock()
		tdzDNSHubMu.Unlock()
	})
	return nil
}

func (l *tdzDNSListener) acceptDNS(mtu int) {
	for {
		conn, err := l.kcpListener.AcceptKCP()
		if err != nil {
			return
		}
		conn.SetStreamMode(true)
		// DNS RTT dominates; a 100ms KCP clock avoids a 10ms wakeup per session.
		conn.SetNoDelay(0, 100, 0, 1)
		conn.SetWindowSize(128, 128)
		if !conn.SetMtu(mtu) {
			conn.Close()
			continue
		}
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			conn.Close()
			return
		}
		clientID, ok := conn.RemoteAddr().(turbotunnel.ClientID)
		if !ok {
			l.mu.Unlock()
			conn.Close()
			continue
		}
		activity := l.activities[clientID]
		if activity == nil {
			l.mu.Unlock()
			conn.Close()
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		session := &tdzDNSSession{ctx: ctx, cancel: cancel, activity: activity}
		l.sessions[conn] = session
		l.mu.Unlock()
		go l.acceptStreams(conn, clientID, session)
	}
}

func (l *tdzDNSListener) acceptStreams(conn *kcp.UDPSession, clientID turbotunnel.ClientID, session *tdzDNSSession) {
	defer func() {
		session.cancel()
		conn.Close()
		l.mu.Lock()
		delete(l.sessions, conn)
		inUse := false
		for _, other := range l.sessions {
			if other.activity == session.activity {
				inUse = true
				break
			}
		}
		if !inUse && l.activities[clientID] == session.activity {
			delete(l.activities, clientID)
		}
		l.mu.Unlock()
	}()
	privateKey, _ := noise.DecodeKey(l.hub.config.PrivateKey)
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	encrypted, err := noise.NewServer(conn, privateKey)
	if err != nil {
		return
	}
	conn.SetDeadline(time.Time{})
	cfg := smux.DefaultConfig()
	cfg.Version, cfg.KeepAliveTimeout = 2, tdzDNSIdleTimeout
	cfg.KeepAliveInterval = 2 * time.Second
	cfg.MaxStreamBuffer, cfg.MaxReceiveBuffer = 64*1024, 512*1024
	mux, err := smux.Server(encrypted, cfg)
	if err != nil {
		encrypted.Close()
		return
	}
	defer mux.Close()
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%08x", l.domainKey, conn.RemoteAddr().String(), conn.GetConv())))
	ip := make(net.IP, 16)
	ip[0], ip[1] = 0xfd, 0x53
	copy(ip[2:], digest[:14])
	identity := ip.String()
	tdzDNSSources.Store(identity, session)
	defer tdzDNSSources.CompareAndDelete(identity, session)
	for {
		stream, err := mux.AcceptStream()
		if err != nil {
			return
		}
		go l.hub.offerStream(&tdzDNSConn{Conn: stream, local: l.Listener.Addr(), remote: &net.TCPAddr{IP: ip, Port: int(stream.ID()%65535) + 1}}, l)
	}
}

// Select only the outer transport here; credential routing happens after the
// transport decodes its stream. Every inbound can therefore share the base NS.
func (h *tdzDNSHub) offerStream(conn net.Conn, fallback *tdzDNSListener) {
	h.mu.RLock()
	endpoints := make([]*tdzDNSListener, 0, len(h.endpoints))
	for _, route := range h.config.Routes {
		if endpoint := h.endpoints[strings.ToLower(route.Domain)]; endpoint != nil {
			endpoints = append(endpoints, endpoint)
		}
	}
	h.mu.RUnlock()
	if len(endpoints) < 2 {
		fallback.offer(conn)
		return
	}
	for attempts := 0; attempts < 2; attempts++ {
		reader := bufio.NewReaderSize(conn, 8192)
		wrapped := &tdzDNSReplayConn{Conn: conn, reader: reader}
		conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		prefix, err := reader.Peek(4)
		if err != nil {
			conn.Close()
			return
		}
		network := "tcp"
		var request *http.Request
		if string(prefix) == "PRI " {
			network = "grpc"
		} else if string(prefix) == "GET " || string(prefix) == "POST" || string(prefix) == "CONN" || string(prefix) == "HEAD" || string(prefix) == "PUT " || string(prefix) == "OPTI" {
			for size := 4; size <= 8192; size++ {
				header, peekErr := reader.Peek(size)
				if peekErr != nil {
					conn.Close()
					return
				}
				if !bytes.HasSuffix(header, []byte("\r\n\r\n")) {
					continue
				}
				request, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(header)))
				if err != nil {
					conn.Close()
					return
				}
				break
			}
			if request == nil {
				conn.Close()
				return
			}
			if request.Header.Get("X-TDZ-Payload") != "1" && strings.EqualFold(request.Header.Get("Upgrade"), "websocket") {
				network = "httpupgrade"
				if request.Header.Get("Sec-WebSocket-Key") != "" {
					network = "websocket"
				}
			} else {
				for _, endpoint := range endpoints {
					path := endpoint.route.Path
					if path == "" {
						path = "/"
					}
					if endpoint.route.Network == "splithttp" && strings.HasPrefix(request.URL.Path, path) && request.Method != "CONNECT" {
						network = "splithttp"
						break
					}
				}
				if network == "tcp" && attempts == 0 {
					for _, endpoint := range endpoints {
						if !endpoint.route.AcceptPayloads {
							continue
						}
						conn, err = tdzAcceptAutoPayload(wrapped, "", "", tdzPayloadTimeout)
						if err != nil {
							wrapped.Close()
							return
						}
						break
					}
					if conn != wrapped.Conn {
						continue
					}
				}
			}
		}
		conn.SetReadDeadline(time.Time{})
		for _, endpoint := range endpoints {
			if endpoint.route.Network != network {
				continue
			}
			if request != nil && (network == "websocket" || network == "httpupgrade" || network == "splithttp") {
				path := endpoint.route.Path
				if path == "" {
					path = "/"
				}
				if network == "splithttp" {
					if !strings.HasPrefix(request.URL.Path, path) {
						continue
					}
				} else if request.URL.Path != path {
					continue
				}
				if endpoint.route.Host != "" && !IsValidHTTPHost(request.Host, endpoint.route.Host) {
					continue
				}
			}
			endpoint.offer(wrapped)
			return
		}
		fallback.offer(wrapped)
		return
	}
	conn.Close()
}

func (l *tdzDNSListener) touch(clientID turbotunnel.ClientID, address net.Addr, create bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	activity := l.activities[clientID]
	if activity == nil && create && !l.closed {
		resolver := ""
		if address, ok := address.(*net.UDPAddr); ok {
			resolver = address.IP.String()
		}
		activity = &tdzDNSActivity{resolver: resolver}
		l.activities[clientID] = activity
	}
	if activity != nil {
		activity.lastQuery.Store(time.Now().UnixNano())
	}
}

func (h *tdzDNSHub) leaseLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		var expired []*kcp.UDPSession
		cutoff := time.Now().Add(-tdzDNSLeaseTimeout).UnixNano()
		h.mu.RLock()
		if len(h.endpoints) == 0 {
			h.mu.RUnlock()
			return
		}
		for _, l := range h.endpoints {
			l.mu.Lock()
			for conn, session := range l.sessions {
				if session.activity.lastQuery.Load() < cutoff {
					session.cancel()
					expired = append(expired, conn)
				}
			}
			for id, activity := range l.activities {
				if activity.lastQuery.Load() < cutoff {
					delete(l.activities, id)
				}
			}
			l.mu.Unlock()
		}
		h.mu.RUnlock()
		for _, conn := range expired {
			conn.Close()
		}
	}
}

type tdzDNSConn struct {
	net.Conn
	local, remote net.Addr
}

func (c *tdzDNSConn) LocalAddr() net.Addr  { return c.local }
func (c *tdzDNSConn) RemoteAddr() net.Addr { return c.remote }

func (h *tdzDNSHub) readLoop() {
	var buffer [4096]byte
	for {
		n, address, err := h.conn.ReadFrom(buffer[:])
		if err != nil {
			return
		}
		query, err := dns.MessageFromWireFormat(buffer[:n])
		if err != nil || query.Flags&0x8000 != 0 {
			continue
		}
		if h.metadata(&query, address) {
			continue
		}
		var endpoint *tdzDNSListener
		responseDomain := h.zone
		if len(query.Question) == 1 && len(query.Question[0].Name) >= h.labels {
			name := query.Question[0].Name
			key := strings.ToLower(string(bytes.Join(name[len(name)-h.labels:], []byte("."))))
			h.mu.RLock()
			endpoint = h.endpoints[key]
			h.mu.RUnlock()
			if endpoint != nil {
				responseDomain = endpoint.domain
			}
		}
		if endpoint == nil && len(query.Question) == 1 {
			if _, ok := query.Question[0].Name.TrimSuffix(h.zone); ok {
				h.mu.RLock()
				endpoint = h.endpoints[h.primaryDomain]
				h.mu.RUnlock()
			}
		}
		if endpoint == nil {
			response := &dns.Message{ID: query.ID, Flags: 0x8000 | dns.RcodeNameError, Question: query.Question}
			h.write(response, address)
			continue
		}
		response, payload := tdzDNSResponseFor(&query, responseDomain, h.config.MTU)
		var clientID turbotunnel.ClientID
		copied := copy(clientID[:], payload)
		if copied == len(clientID) {
			endpoint.touch(clientID, address, false)
			reader := bytes.NewReader(payload[copied:])
			for {
				packet, err := tdzDNSNextPacket(reader)
				if err != nil {
					break
				}
				if len(packet) < 24 {
					continue
				}
				endpoint.touch(clientID, address, true)
				endpoint.queue.QueueIncoming(packet, clientID)
			}
		} else if response != nil && response.Rcode() == dns.RcodeNoError {
			response.Flags |= dns.RcodeNameError
		}
		if response != nil {
			select {
			case <-endpoint.done:
			case endpoint.records <- &tdzDNSRecord{response, address, clientID}:
			default:
			}
		}
	}
}

func tdzDNSNameWire(name dns.Name) []byte {
	var wire []byte
	for _, label := range name {
		wire = append(wire, byte(len(label)))
		wire = append(wire, label...)
	}
	return append(wire, 0)
}

// Answer QNAME-minimization NS probes without claiming the delegated zone is
// nonexistent. Per-inbound subdomains use the parent's existing NS delegation.
func (h *tdzDNSHub) metadata(query *dns.Message, address net.Addr) bool {
	if len(query.Question) != 1 {
		return false
	}
	question := query.Question[0]
	if _, ok := question.Name.TrimSuffix(h.zone); !ok || question.Type != 2 && question.Type != 6 {
		return false
	}
	response := &dns.Message{ID: query.ID, Flags: 0x8400, Question: query.Question}
	host, err := dns.ParseName(h.config.HostDomain)
	if err != nil {
		return false
	}
	mailbox, _ := dns.ParseName("hostmaster." + h.config.NSDomain)
	soa := append(tdzDNSNameWire(host), tdzDNSNameWire(mailbox)...)
	for _, value := range []uint32{1, 3600, 600, 86400, 60} {
		soa = binary.BigEndian.AppendUint32(soa, value)
	}
	if question.Type == 6 {
		response.Answer = []dns.RR{{Name: h.zone, Type: 6, Class: 1, TTL: 60, Data: soa}}
	} else if len(question.Name) == len(h.zone) {
		response.Answer = []dns.RR{{Name: h.zone, Type: 2, Class: 1, TTL: 60, Data: tdzDNSNameWire(host)}}
	} else {
		response.Authority = []dns.RR{{Name: h.zone, Type: 6, Class: 1, TTL: 60, Data: soa}}
	}
	h.write(response, address)
	return true
}

func (h *tdzDNSHub) write(response *dns.Message, address net.Addr) {
	data, err := response.WireFormat()
	if err != nil {
		return
	}
	if len(data) > h.config.MTU {
		data = data[:h.config.MTU]
		data[2] |= 0x02
	}
	h.conn.WriteTo(data, address)
}

func (l *tdzDNSListener) fillResponse(record *tdzDNSRecord, maximum int) *tdzDNSRecord {
	question := record.response.Question[0]
	var payload bytes.Buffer
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	var next *tdzDNSRecord
	for {
		var packet []byte
		stash, outgoing := l.queue.Unstash(record.clientID), l.queue.OutgoingQueue(record.clientID)
		select {
		case packet = <-stash:
		default:
			select {
			case packet = <-stash:
			case packet = <-outgoing:
			default:
				if payload.Len() != 0 {
					goto complete
				}
				select {
				case packet = <-stash:
				case packet = <-outgoing:
				case <-timer.C:
					goto complete
				case next = <-l.records:
					goto complete
				case <-l.done:
					goto complete
				}
			}
		}
		if len(packet) == 0 {
			goto complete
		}
		if payload.Len()+2+len(packet) > maximum {
			l.queue.Stash(packet, record.clientID)
			goto complete
		}
		binary.Write(&payload, binary.BigEndian, uint16(len(packet)))
		payload.Write(packet)
	}
complete:
	record.response.Answer = []dns.RR{{Name: question.Name, Type: question.Type, Class: question.Class, TTL: responseTTL, Data: dns.EncodeRDataTXT(payload.Bytes())}}
	return next
}

func (l *tdzDNSListener) sendLoop(maximum int) {
	var next *tdzDNSRecord
	for {
		record := next
		next = nil
		if record == nil {
			select {
			case <-l.done:
				return
			case record = <-l.records:
			}
		}
		if record.response.Rcode() == dns.RcodeNoError && len(record.response.Question) == 1 && record.response.Question[0].Type == dns.RRTypeTXT {
			next = l.fillResponse(record, maximum)
		}
		l.hub.write(record.response, record.address)
	}
}
