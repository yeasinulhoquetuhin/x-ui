#!/usr/bin/env python3
"""Apply the admission hook to the pinned Xray v26.4.25 source."""
import pathlib
import shutil
import sys

root = pathlib.Path(sys.argv[1])
source = root / "app/dispatcher/default.go"
text = source.read_text()
replacements = [
    ("type DefaultDispatcher struct {", "type DefaultDispatcher struct {\n\ttdzIPs tdzIPLimiter\n\ttdzIPStop chan struct{}"),
    ("func (*DefaultDispatcher) Start() error {", "func (d *DefaultDispatcher) Start() error {\n\td.startTDZIPLimits()"),
    ("func (*DefaultDispatcher) Close() error { return nil }", "func (d *DefaultDispatcher) Close() error {\n\tif d.tdzIPStop != nil { close(d.tdzIPStop) }\n\treturn nil\n}"),
    ('\t\tif p.Stats.UserOnline {\n\t\t\ttrackOnlineIP(ctx, d.stats, user.Email, sessionInbound.Source.Address.String())\n\t\t}', ''),
    ('\t\tif p.Stats.UserOnline {\n\t\t\ttrackOnlineIP(ctx, statsManager, user.Email, sessionInbound.Source.Address.String())\n\t\t}', ''),
    ('func (d *DefaultDispatcher) routedDispatch(ctx context.Context, link *transport.Link, destination net.Destination) {', '''func (d *DefaultDispatcher) routedDispatch(ctx context.Context, link *transport.Link, destination net.Destination) {
	clientCtx, release, err := d.admitTDZClient(ctx)
	if err != nil {
		errors.LogWarning(ctx, err)
		common.Close(link.Writer)
		common.Interrupt(link.Reader)
		return
	}
	ctx = clientCtx
	defer release()
	stop := context.AfterFunc(ctx, func() { common.Interrupt(link.Reader); common.Interrupt(link.Writer) })
	defer stop()'''),
]
for before, after in replacements:
    if text.count(before) != 1:
        raise SystemExit(f"Pinned core patch mismatch: {before[:80]}")
    text = text.replace(before, after)
source.write_text(text)
start = text.find('func trackOnlineIP(')
if start >= 0:
    end = text.find('\nfunc ', start + 1)
    text = text[:start] + text[end + 1:]
    source.write_text(text)
source = root / "common/mux/client.go"
text = source.read_text()
before = 'if _, ok := link.Reader.(*pipe.Reader); !ok {'
if text.count(before) != 1:
    raise SystemExit("Pinned core MUX patch mismatch")
text = text.replace(before, 'if _, ok := link.Reader.(*pipe.Reader); !ok || ctx.Value(session.TDZWaitForMuxKey{}) == true {')
source.write_text(text)
source = root / "app/stats/online_map.go"
text = source.read_text()
before = '\tif ip == localhostIPv4 || ip == localhostIPv6 {\n\t\treturn\n\t}\n'
if text.count(before) != 1:
    raise SystemExit("Pinned core online-map patch mismatch")
# Show the source actually observed by the core, including local proxies. A
# proxy hiding the client IP must not be disguised as an empty client list.
source.write_text(text.replace(before, ""))
shutil.copyfile(pathlib.Path(__file__).parent / "testdata/tdz_session.go", root / "common/session/tdz_session.go")
for name in ("tdz_ip_limit.go", "tdz_ip_limit_test.go"):
    src = pathlib.Path(__file__).parent / "testdata" / name
    if src.exists():
        shutil.copyfile(src, root / "app/dispatcher" / name)
