#!/usr/bin/env python3
"""Apply payload acceptance to the pinned TCP-based transport listeners."""
import pathlib
import shutil
import sys
import tarfile
import hashlib
import subprocess

root = pathlib.Path(sys.argv[1])
assets = pathlib.Path(__file__).parent / 'testdata'

# Remove test.1/test.2 hooks before applying the exact TDZ one-header relay.
# Reapply trusted-address changes after restoring HTTP transport source files.
old_tcp = (root / 'transport/internet/tcp/hub.go').read_text()
if 'tdzAcceptPayload(conn)' in old_tcp or 'EnableTDZPayload()' in old_tcp:
    archive = assets.parent.parent / 'third_party/xray-core-v26.4.25-source.tar.gz'
    if hashlib.sha256(archive.read_bytes()).hexdigest() != 'a2ed4c9b7022c9dd5eed6863d360c70495d3ca5174a497d99d7e082b087c16fd':
        raise SystemExit('Pinned source checksum mismatch')
    restore = {
        'transport/internet/tcp/hub.go', 'transport/internet/tcp/config.proto',
        'transport/internet/tcp/config.pb.go', 'infra/conf/transport_internet.go',
        'transport/internet/tls/tls.go', 'transport/internet/websocket/hub.go',
        'transport/internet/httpupgrade/hub.go', 'transport/internet/grpc/hub.go',
        'transport/internet/splithttp/hub.go',
    }
    with tarfile.open(archive) as source:
        for member in source:
            name = member.name.split('/', 1)[-1]
            if name in restore and member.isfile():
                (root / name).write_bytes(source.extractfile(member).read())
    subprocess.run([sys.executable, str(assets.parent / 'apply_client_addr.py'), str(root)], check=True)


def replace(name, before, after):
    path = root / name
    text = path.read_text()
    if after in text:
        return
    if text.count(before) != 1:
        raise SystemExit(f'Pinned payload patch mismatch: {name}: {before[:70]}')
    path.write_text(text.replace(before, after))


# Older test-build generated files must not remain in the TCP package.
for name in ('tdz_payload.go', 'tdz_payload_test.go'):
    (root / 'transport/internet/tcp' / name).unlink(missing_ok=True)

replace('transport/internet/config.proto', '  SocketConfig socket_settings = 6;',
        '  SocketConfig socket_settings = 6;\n  bool accept_payloads = 13;')
replace('infra/conf/transport_internet.go', 'type TCPConfig struct {',
        'type TCPConfig struct {\n\tInjectPayload bool `json:"injectPayload"`')
replace('infra/conf/transport_internet.go', 'type StreamConfig struct {',
        'type StreamConfig struct {\n\tAcceptPayloads *bool `json:"acceptPayloads"`')
replace('infra/conf/transport_internet.go', '\tif c.Address != nil {\n\t\tconfig.Address',
        '''\tlegacyPayload := c.TCPSettings != nil && c.TCPSettings.InjectPayload || c.RAWSettings != nil && c.RAWSettings.InjectPayload
\tconfig.AcceptPayloads = legacyPayload
\tif c.AcceptPayloads != nil { config.AcceptPayloads = *c.AcceptPayloads }
\tif c.Address != nil {
\t\tconfig.Address''')
replace('infra/conf/transport_internet.go', '\tswitch strings.ToLower(c.Security) {',
        '''\tif config.AcceptPayloads {
\t\tswitch config.ProtocolName { case "tcp", "websocket", "httpupgrade", "grpc", "splithttp": default: return nil, errors.New("Accept Payloads requires a TCP-based transport") }
\t}
\tswitch strings.ToLower(c.Security) {''')
replace('transport/internet/memory_settings.go', 'type MemoryStreamConfig struct {',
        'type MemoryStreamConfig struct {\n\tAcceptPayloads bool')
replace('transport/internet/memory_settings.go', '\t\tmss.SocketSettings = s.SocketSettings',
        '\t\tmss.AcceptPayloads = s.AcceptPayloads\n\t\tmss.SocketSettings = s.SocketSettings')

replace('transport/internet/tcp/hub.go', '\tl.listener = listener',
        '\tlistener = internet.TDZPayloadListener(listener, streamSettings.AcceptPayloads)\n\tl.listener = listener')
replace('transport/internet/websocket/hub.go', '\tif config := v2tls.ConfigFromStreamSettings(streamSettings);',
        '\tlistener = internet.TDZPayloadListener(listener, streamSettings.AcceptPayloads)\n\tif config := v2tls.ConfigFromStreamSettings(streamSettings);')
replace('transport/internet/httpupgrade/hub.go', '\tif config := v2tls.ConfigFromStreamSettings(streamSettings);',
        '\tlistener = internet.TDZPayloadListener(listener, streamSettings.AcceptPayloads)\n\tif config := v2tls.ConfigFromStreamSettings(streamSettings);')
replace('transport/internet/grpc/hub.go', '\t\tif config := reality.ConfigFromStreamSettings(settings);',
        '\t\tstreamListener = internet.TDZPayloadListener(streamListener, settings.AcceptPayloads)\n\t\tif config := reality.ConfigFromStreamSettings(settings);')
replace('transport/internet/splithttp/hub.go', '\tif l.listener != nil {\n\t\tif config := tls.',
        '\tif l.listener != nil {\n\t\tl.listener = internet.TDZPayloadListener(l.listener, streamSettings.AcceptPayloads)\n\t\tif config := tls.')
replace('transport/internet/splithttp/hub.go', '\tvar err error\n\tif port ==',
        '\tif l.isH3 && streamSettings.AcceptPayloads { return nil, errors.New("Accept Payloads does not support XHTTP over QUIC") }\n\tvar err error\n\tif port ==')

for name in ('tdz_payload.go', 'tdz_payload_test.go'):
    shutil.copyfile(assets / name, root / 'transport/internet' / name)
shutil.copyfile(assets / 'tdz_stream_config.pb.go', root / 'transport/internet/config.pb.go')
