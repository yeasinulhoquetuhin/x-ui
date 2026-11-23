#!/usr/bin/env python3
"""Add stock-DNSTT-compatible virtual listeners to the pinned TCP transports."""
import hashlib
import json
import re
from pathlib import Path
import shutil
import subprocess
import sys

root = Path(sys.argv[1])
assets = Path(__file__).parent / 'testdata'
module = 'www.bamsoftware.com/git/dnstt.git@v0.0.0-20260501011358-0c5c52a57d89'
subprocess.run(['go', 'mod', 'edit', '-require=' + module], cwd=root, check=True)
download = json.loads(subprocess.check_output(['go', 'mod', 'download', '-json', module], cwd=root))
server = Path(download['Dir']) / 'dnstt-server/main.go'
assert hashlib.sha256(server.read_bytes()).hexdigest() == '778a4e16747586af33a05e177034294a91813414e3a71bcbd8e634aca27a84e8'
source = server.read_text()
helpers = source[source.index('// nextPacket '):source.index('// record represents')]
helpers += source[source.index('// computeMaxEncodedPayload '):source.index('func run(')]
helpers = helpers.replace('domain dns.Name) (*dns.Message, []byte)', 'domain dns.Name, maxUDPPayload int) (*dns.Message, []byte)')
helpers = helpers.replace('responseFor(query, dns.Name([][]byte{}))', 'responseFor(query, dns.Name([][]byte{}), limit)')
for old, new in [('nextPacket', 'tdzDNSNextPacket'), ('responseFor', 'tdzDNSResponseFor'), ('computeMaxEncodedPayload', 'tdzDNSComputeMaxPayload'), ('base32Encoding', 'tdzDNSBase32Encoding'), ('ioutil.Discard', 'io.Discard')]:
    helpers = helpers.replace(old, new)
header = '''// DNSTT DNS framing, adapted from the pinned public-domain upstream.
// Source: 0c5c52a57d899c05428c116898941761a2ed83c2 (2026-05-01).
package internet

import (
    "bytes"
    "encoding/base32"
    "fmt"
    "io"
    "log"
    "www.bamsoftware.com/git/dnstt.git/dns"
)

const responseTTL = 60
var tdzDNSBase32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

'''
(root / 'transport/internet/tdz_slowdns_dns.go').write_text(header + helpers)
for name in ('tdz_slowdns.go', 'tdz_slowdns_test.go', 'tdz_slowdns_worker.go'):
    if (assets / name).exists():
        shutil.copyfile(assets / name, root / 'transport/internet' / name)


# Remove the prototype per-transport hooks from an existing managed build cache.
# The system listener hook below is shared by every TCP transport and precedes
# TCP masks, payload parsing and TLS, including gRPC's asynchronous listener.
for transport, variable in [('tcp', 'listener'), ('websocket', 'listener'), ('httpupgrade', 'listener'), ('grpc', 'streamListener'), ('splithttp', 'l.listener')]:
    path = root / f'transport/internet/{transport}/hub.go'
    text = path.read_text()
    if 'internet.TDZSlowDNSListener(' not in text:
        continue
    pattern = (r'\s*tdzDNSListener, tdzDNSErr := internet\.TDZSlowDNSListener\(' + re.escape(variable) + ', "' + transport + r'"\)\s*'
               r'if tdzDNSErr != nil \{\s*' + re.escape(variable) + r'\.Close\(\)\s*return nil, tdzDNSErr\s*\}\s*'
               + re.escape(variable) + r' = tdzDNSListener\s*')
    text, count = re.subn(pattern, '\n\t', text)
    if count != 1:
        raise SystemExit(f'Unexpected cached SlowDNS prototype: {path}')
    path.write_text(text)

path = root / 'transport/internet/system_listener.go'
text = path.read_text()
if 'TDZSlowDNSListener(l, "")' not in text:
    before = '\treturn l, err\n}\n\nfunc (dl *DefaultListener) ListenPacket'
    after = '''\t// Native DNS streams pass through the same transport framing as TCP.
    if err == nil && (network == "tcp" || network == "tcp4" || network == "tcp6") {
        wrapped, dnsErr := TDZSlowDNSListener(l, "")
        if dnsErr != nil { l.Close(); return nil, dnsErr }
        l = wrapped
    }
    return l, err
}

func (dl *DefaultListener) ListenPacket'''
    if text.count(before) != 1:
        raise SystemExit('Pinned SlowDNS system listener mismatch')
    path.write_text(text.replace(before, after))

path = root / 'app/proxyman/inbound/worker.go'
text = path.read_text()
if 'TDZRouteSlowDNSWorker' not in text:
    before = 'func (w *tcpWorker) callback(conn stat.Connection) {\n'
    after = before + '''	if matcher, ok := w.proxy.(internet.TDZSlowDNSMatcher); ok {
        routed, redirected := internet.TDZRouteSlowDNSWorker(conn, matcher)
        if redirected { return }
        conn = routed
    }
'''
    if text.count(before) != 1:
        raise SystemExit('Pinned SlowDNS worker callback mismatch')
    text = text.replace(before, after)
    text, count = re.subn(r'(type tcpWorker struct \{.*?\n\s*hub\s+internet\.Listener\n)',
                          r'\1\ttdzDNSUnregister func()\n', text, count=1, flags=re.S)
    if count != 1:
        raise SystemExit('Pinned SlowDNS worker lifetime mismatch')
    before = 'func (w *tcpWorker) Start() error {\n'
    text = text.replace(before, before + '''	if matcher, ok := w.proxy.(internet.TDZSlowDNSMatcher); ok {
        w.tdzDNSUnregister = internet.TDZRegisterSlowDNSWorker(w.address.String(), int(w.port), matcher, w.callback)
    }
''')
    before = 'func (w *tcpWorker) Close() error {\n'
    text = text.replace(before, before + '\tif w.tdzDNSUnregister != nil { w.tdzDNSUnregister() }\n')
    path.write_text(text)

# A failed bind must not leave a selectable worker in the shared DNS namespace.
text = path.read_text()
before = '\tif err != nil {\n\t\treturn errors.New("failed to listen TCP on ", w.port).AtWarning().Base(err)\n\t}'
after = '\tif err != nil {\n\t\tif w.tdzDNSUnregister != nil { w.tdzDNSUnregister() }\n\t\treturn errors.New("failed to listen TCP on ", w.port).AtWarning().Base(err)\n\t}'
if before in text:
    path.write_text(text.replace(before, after, 1))

(root / 'proxy/vless/inbound/tdz_slowdns.go').write_text('''package inbound
import "github.com/xtls/xray-core/common/uuid"
func (h *Handler) TDZSlowDNSHeaderSize() int { return 17 }
func (h *Handler) TDZSlowDNSMatch(data []byte) bool {
    if h.decryption != nil || len(data) < 17 || data[0] != 0 { return false }
    var id uuid.UUID
    copy(id[:], data[1:17])
    return h.validator.Get(id) != nil
}
''')
(root / 'proxy/vmess/inbound/tdz_slowdns.go').write_text('''package inbound
import (
    "hash/crc32"
    "time"
    "github.com/xtls/xray-core/proxy/vmess"
    "github.com/xtls/xray-core/proxy/vmess/aead"
)
func (h *Handler) TDZSlowDNSHeaderSize() int { return 16 }
func (h *Handler) TDZSlowDNSMatch(data []byte) bool {
    if len(data) < 16 { return false }
    var auth [16]byte
    copy(auth[:], data[:16])
    now := time.Now().Unix()
    for _, user := range h.clients.GetUsers() {
        account, ok := user.Account.(*vmess.MemoryAccount)
        if !ok { continue }
        stamp, checksum, _, plain := aead.NewAuthIDDecoder(account.ID.CmdKey()).Decode(auth)
        if checksum == crc32.ChecksumIEEE(plain[:12]) && stamp >= now-120 && stamp <= now+120 { return true }
    }
    return false
}
''')
(root / 'proxy/trojan/tdz_slowdns.go').write_text('''package trojan
func (s *Server) TDZSlowDNSHeaderSize() int { return 58 }
func (s *Server) TDZSlowDNSMatch(data []byte) bool {
    return len(data) >= 58 && data[56] == '\\r' && data[57] == '\\n' && s.validator.Get(hexString(data[:56])) != nil
}
''')
(root / 'proxy/shadowsocks/tdz_slowdns.go').write_text('''package shadowsocks
import "github.com/xtls/xray-core/common/protocol"
func (s *Server) TDZSlowDNSHeaderSize() int {
    maximum := 0
    for _, user := range s.validator.GetAll() {
        account, ok := user.Account.(*MemoryAccount)
        if !ok || !account.Cipher.IsAEAD() { continue }
        size := int(account.Cipher.IVSize()) + 18
        if size > maximum { maximum = size }
    }
    return maximum
}
func (s *Server) TDZSlowDNSMatch(data []byte) bool {
    if s.TDZSlowDNSHeaderSize() == 0 || len(data) < s.TDZSlowDNSHeaderSize() { return false }
    user, _, _, _, err := s.validator.Get(data, protocol.RequestCommandTCP)
    return err == nil && user != nil
}
''')

(root / 'transport/internet/grpc/tdz_slowdns.go').write_text('''package grpc
import (
    "context"
    "net"
    "github.com/xtls/xray-core/transport/internet"
    "github.com/xtls/xray-core/transport/internet/grpc/encoding"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/peer"
    "google.golang.org/grpc/status"
)
type tdzDNSService struct { *Listener }
func tdzDNSPeer(ctx context.Context) bool {
    p, ok := peer.FromContext(ctx)
    if !ok || p.Addr == nil { return false }
    host, _, err := net.SplitHostPort(p.Addr.String())
    return err == nil && internet.TDZIsSlowDNSSource(host)
}
func (s *tdzDNSService) Tun(stream encoding.GRPCService_TunServer) error {
    if !tdzDNSPeer(stream.Context()) { return status.Error(codes.Unimplemented, "unknown service") }
    return s.Listener.Tun(stream)
}
func (s *tdzDNSService) TunMulti(stream encoding.GRPCService_TunMultiServer) error {
    if !tdzDNSPeer(stream.Context()) { return status.Error(codes.Unimplemented, "unknown service") }
    return s.Listener.TunMulti(stream)
}
''')
path = root / 'transport/internet/grpc/hub.go'
text = path.read_text()
if 'TDZSlowDNSGRPCServices' not in text:
    before = '\t\tencoding.RegisterGRPCServiceServerX(s, listener, grpcSettings.getServiceName(), grpcSettings.getTunStreamName(), grpcSettings.getTunMultiStreamName())'
    after = before + '''
        tdzServices := map[string]bool{grpcSettings.getServiceName(): true}
        for _, name := range internet.TDZSlowDNSGRPCServices() {
            alias := *grpcSettings
            alias.ServiceName = name
            if tdzServices[alias.getServiceName()] { continue }
            tdzServices[alias.getServiceName()] = true
            encoding.RegisterGRPCServiceServerX(s, &tdzDNSService{listener}, alias.getServiceName(), alias.getTunStreamName(), alias.getTunMultiStreamName())
        }
'''
    if text.count(before) != 1:
        raise SystemExit('Pinned SlowDNS gRPC service routing mismatch')
    path.write_text(text.replace(before, after))
subprocess.run(['go', 'mod', 'tidy'], cwd=root, check=True)
subprocess.run(['gofmt', '-w', str(root / 'transport/internet'), str(root / 'app/proxyman/inbound/worker.go'),
                *[str(path) for path in (root / 'proxy').glob('**/tdz_slowdns.go')]], check=True)
