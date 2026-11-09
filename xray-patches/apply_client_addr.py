#!/usr/bin/env python3
"""Upgrade the pinned core's HTTP transports to trusted source-IP resolution."""
import pathlib
import shutil
import sys

root = pathlib.Path(sys.argv[1])
assets = pathlib.Path(__file__).parent / 'testdata'


def replace(path, before, after):
    text = path.read_text()
    if after in text:
        return
    if text.count(before) != 1:
        raise SystemExit(f'Pinned source-address patch mismatch: {path}')
    path.write_text(text.replace(before, after))


for transport, prefix, request, tail in (
    ('websocket', 'h', 'request', '\n\th.ln.addConn('),
    ('httpupgrade', 's', 'req', '\n\treturn stat.Connection('),
    ('splithttp', 'h', 'request', '\n\tvar currentSession'),
):
    path = root / f'transport/internet/{transport}/hub.go'
    text = path.read_text()
    if 'internet.TDZClientAddr(' in text:
        continue
    start = text.index('\tvar forwardedAddrs []net.Address')
    end = text.index(tail, start)
    block = text[start:end]
    if transport == 'splithttp':
        # Keep the existing TCP/HTTP3 address construction intact.
        remote_start = block.index('\tvar remoteAddr net.Addr')
        remote_end = block.index('\tif len(forwardedAddrs) > 0')
        address = block[remote_start:remote_end]
    else:
        address = '\tremoteAddr := conn.RemoteAddr()\n'
    local = 'conn.LocalAddr()' if transport != 'splithttp' else 'tdzLocalAddr'
    if transport == 'splithttp':
        address += '\ttdzLocalAddr, _ := request.Context().Value(http.LocalAddrContextKey).(net.Addr)\n'
    replacement = f'''{address}
	var tdzHeaderNames []string
	if {prefix}.socketSettings != nil {{
		tdzHeaderNames = {prefix}.socketSettings.TrustedXForwardedFor
	}}
	remoteAddr = internet.TDZClientAddr(remoteAddr, {local}, {request}.Header, tdzHeaderNames)
'''
    text = text[:start] + replacement + text[end:]
    text = text.replace('\thttp_proto "github.com/xtls/xray-core/common/protocol/http"\n', '')
    path.write_text(text)

for name in ('hunkconn.go', 'multiconn.go'):
    path = root / 'transport/internet/grpc/encoding' / name
    text = path.read_text()
    if '\trAddr := tdzGRPCClientAddr(hc.Context())' in text:
        continue
    start = text.index('\tvar rAddr net.Addr')
    end = text.index('\twrc :=', start)
    text = text[:start] + '\trAddr := tdzGRPCClientAddr(hc.Context())\n' + text[end:]
    for unused in (
        '\t"google.golang.org/grpc/metadata"\n',
        '\t"google.golang.org/grpc/peer"\n',
        '\txnet "github.com/xtls/xray-core/common/net"\n',
    ):
        text = text.replace(unused, '')
    path.write_text(text)

for name in ('tdz_client_addr.go', 'tdz_client_addr_test.go'):
    shutil.copyfile(assets / name, root / 'transport/internet' / name)
shutil.copyfile(assets / 'tdz_grpc_addr.go', root / 'transport/internet/grpc/encoding/tdz_client_addr.go')
