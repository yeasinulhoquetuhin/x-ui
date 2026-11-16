# TDZ X-UI v2.9.3-patched.4

V4 brings the original TDZ payload handshake into the inbound's own listener, improves administrator reseller management, and adds controlled reseller password changes.

## Accept Payloads on one port

- Default-disabled **Accept Payloads** switch above Proxy Protocol; its value survives supported transport changes, save and reopen.
- Available for TCP/RAW, WebSocket, HTTPUpgrade, gRPC and XHTTP over TCP (HTTP/1.1 and HTTP/2).
- Matches the original TDZ proxy: consume exactly one header ending in CRLFCRLF, send the fixed `101 Switching Protocols` response, then forward all remaining bytes unchanged. GET, POST, CONNECT and custom header contents use the same response.
- The payload comes before the native transport/TLS handshake. **ON requires an initial payload**; OFF uses ordinary inbound behavior. No extra backend TCP port is allocated.
- Original source IP, authentication, routing and IP limits remain integrated with Xray. Live API updates retain the payload setting.
- Bundled core marker: `TDZ-IPLimit/2+Payload/3`, based on Xray 26.4.25. UDP/KCP/Hysteria/QUIC do not use this TCP payload option.

See [ACCEPT_PAYLOADS.md](ACCEPT_PAYLOADS.md) for the exact response, limits and client handshake order.

## Reseller management

- The administrator's Reseller page has responsive account cards, search/status filters, clearer inbound assignments and permission controls.
- Activity logs show readable actions, timestamps, affected clients/inbounds and source IPs, with search and category filters.
- New **Edit Password** permission defaults to off. When enabled, **Change Password** appears in the reseller sidebar. The current password is verified; successful changes revoke existing sessions and create an audit event.
- Resellers keep the original inbound dashboard layout and no longer see the administrator TLS warning.
- Login, scoped cookies, assets, APIs and live updates use **`/reseller/<path>/`**, independently of the administrator base path.

## Client links and CLI fixes

- Removed inbound-name prefixes from client subscription URI/JSON/Clash labels, Telegram-delivered config links and panel copy/export/QR actions.
- Removed the main CLI Payload / Proxy entry and renumbered choices through **25**. Legacy instances remain accessible through `x-ui-proxy`.
- Fixed the hidden nested menu that stalled IP SSL setup from **9. View Current Settings**. Accepts `y`, `yes` and `-y`; busy-port detection, visible dependency progress, operation timeouts and service recovery on failure/cancellation are included.

## Upgrade

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v4
```

No argument selects the latest published package for the server's architecture. Existing database, credentials, panel port/base path, certificates and service settings are preserved with rollback backups.

After upgrading, hard-refresh the browser, copy/distribute reseller URLs in the new `/reseller/<path>/` form, and sign in again. Existing Path IDs/passwords/assignments are retained; password-edit permission defaults to off. Existing inbounds do not have Accept Payloads enabled automatically.

## Packages and validation

- Linux: **amd64, arm64, 386, armv5, armv6, armv7, s390x**. Windows: **amd64**.
- Matching patched cores, panel source archive, pinned original Xray source, release documentation and **SHA256SUMS** accompany the packages.
- Verification includes Go race tests/vet/static analysis, installer and client-model tests, browser permission/path/mobile checks, legacy TDZ byte comparisons, actual transport/TLS/MUX/IP-limit traffic, live API updates and migration comparisons.
- The test build was installed on the supplied VPS and the user confirmed that payload connections work. CLI SSL recovery was regression-tested; live certificate issuance is not part of that test.

Full history: [CHANGELOG.md](CHANGELOG.md).
