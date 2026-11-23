# TDZ X-UI V5 — Always Best

**Release:** `v2.9.3-patched.5` · **Upgrade from:** V4 (`v2.9.3-patched.4`)

V5 adds native encrypted SlowDNS, persistent DNS keys, a shared client NameServer, TDZ SSH coexistence and a clearer subscription experience. Ordinary real-IP admission remains enforced; **native DNSTT is explicitly exempt from IP-limit counts, blocking and IP Log**.

## Fixed

- SlowDNS limited accounts no longer fail because of localhost TCP forwarding.
- DNSTT does not occupy IP slots or appear in IP Log/subscription IP counts. A configured limit of 1 still permits multiple authenticated DNS tunnels.
- Ordinary connections to a DNS-enabled inbound still count their real source IP and enforce the configured limit. Blocking a second ordinary IP leaves admitted ordinary/DNS traffic working.
- DNS-activity leases clean up abandoned resources and cancel streams while retaining healthy idle sessions; a stale DNS tunnel cannot hold an IP-limit slot during reconnect.
- Distinct client credentials use the same main NS domain across supported inbounds without an inbound prefix.
- Subscription pages scroll natively on mobile and desktop without needing to open settings first.
- Inbound database changes and native DNS routing are published before live inbound creation/update.
- Managed core rebuilds recognize gofmt-aligned payload fields and remove duplicate cached declarations.
- IP-reporting APIs preserve typed records and exclude legacy DNS metadata from ordinary IP counts/logs.

## Added

- **23. SlowDNS Management:** install, edit/reconfigure, status, start/stop, uninstall and explicit public-key revocation/regeneration.
- Default-off SlowDNS on NTLS VLESS, VMess, Trojan and Shadowsocks over TCP/RAW, WS, HTTPUpgrade, gRPC and TCP XHTTP.
- X25519 private/public keys stored in the database; SlowDNS reinstall and domain/MTU edits retain the public key.
- Bottom More Information card and per-client opt-in subscription hints below configs: clickable NameServer and Public Key only.
- Separate-zone TDZ SSH SlowDNS coexistence through domain-specific early-priority DNS routing.
- Public SlowDNS metadata endpoint: `SUBSCRIPTION_URL?format=slowdns`, subject to each client's opt-in setting.
- Linux/Docker routing dependencies and NET_ADMIN support for domain-scoped public-53/internal-5301 DNS routing.

## Improved

- One shared UDP socket and native DNSTT execution with no extra server daemon/loopback TCP hop.
- 100ms KCP update scheduling, bounded 64 KiB per-stream / 512 KiB receive buffers and event-driven idle waits.
- Readable protocol/transport/security/ALPN documentation and the [SlowDNS guide](SLOWDNS.md).
- CLI **24 BBR**, **25 Geo Files**, **26 Speedtest** after adding SlowDNS at 23.
- Simple menu 23 flow; option 3 shows a dedicated details page until 0 returns to the main menu.
- Plain CLI status output, retaining the functional UDP listener without decorative separator blocks.
- Expanded README covering the complete feature set, setup, protocols/transports/security/ALPN, DNS policy, upgrades and reproducible builds.

## Final connection-limit behavior

| Connection | IP count/log | IP-limit admission |
| --- | --- | --- |
| Ordinary authenticated client | Its canonical real source IP | Enforced atomically inside Xray |
| Additional streams from the same ordinary IP | Share that IP slot | Allowed while that IP remains admitted |
| Another ordinary IP at limit 1 | Rejected | First admitted IP continues working |
| Native DNSTT, including multiple app-created tunnels | Excluded | Unlimited by the IP-limit setting |

DNSTT retains protocol authentication, traffic accounting and account-disable/removal cancellation. Its exemption is granted only to registered in-process native DNS sources; an arbitrary private/loopback address does not bypass ordinary admission.

Recursive DNS does not expose a reliable phone identity, and some apps open many independent DNSTT tunnels during one VPN connection. V5 therefore does not present tunnel counts as device/IP counts.

## SlowDNS setup and keys

1. Upgrade, open **23 → 1**, and save the nameserver A/AAAA host, delegated NS zone, internal listener and MTU.
2. Create the DNS records shown in **23 → 3**. Linux clients use public UDP 53; X-UI's zone is routed to internal UDP 5301.
3. Enable SlowDNS on supported NTLS inbounds. VLESS, VMess, Trojan and classic Shadowsocks AEAD traffic were exercised over TCP/RAW, WS, HTTPUpgrade, gRPC and TCP XHTTP.
4. Use the common **NameServer** and **Public Key** with existing client credentials and transport settings. Google/Cloudflare resolvers are accepted.
5. Enable **Show SlowDNS on Subscription Page** only for clients that should publish the two public fields.

Domain/listener/MTU edits and SlowDNS uninstall/reinstall preserve the database X25519 keypair. Uninstall disables inbound opt-ins and removes X-UI routing rules. **Regenerate / Revoke Public Key** explicitly replaces the keypair; clients then need the new public key. Back up the panel database before replacing the complete database.

TDZ SSH and X-UI can share the server/nameserver host using **separate sibling tunnel zones**. X-UI's early-priority domain route coexists with TDZ SSH's normal public-53-to-internal-54 redirect.

## Verification

- Actual unmodified DNSTT-client traffic passed all 20 protocol/transport combinations, including distinct clients/inbounds on the common NS domain.
- Multiple independent DNS tunnels work at IP limit 1 and remain absent from counts/logs; mixed ordinary/DNS traffic retains first-IP admission and second-IP rejection.
- Key retention, core race checks, installer/menu tests and TDZ SSH shared-port routing/reinstall checks passed.
- Mobile/desktop fresh reloads, settings/theme interactions and copy actions were checked with native subscription scrolling.
- Packages cover Linux amd64/arm64/386/armv5/armv6/armv7/s390x and Windows amd64, with matching sources, documentation and SHA256 checksums.

## Upgrade

Select V5 using:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v5
```

Use **menu 23** to configure the nameserver host, delegated tunnel zone and MTU; then opt in only the required NTLS inbounds. Public DNS delegation and a DNSTT-compatible client are required.

The in-place installer preserves the database, credentials, clients, panel port/base path and certificates with rollback backups. Reload browser tabs after upgrading to pick up UI/subscription changes.

---

## V4 features retained

V4 brings the original TDZ payload handshake into the inbound's own listener, improves administrator reseller management, and adds controlled reseller password changes.

## Accept Payloads on one port

- Default-disabled **Accept Payloads** switch above Proxy Protocol; its value survives supported transport changes, save and reopen.
- Available for TCP/RAW, WebSocket, HTTPUpgrade, gRPC and XHTTP over TCP (HTTP/1.1 and HTTP/2).
- Matches the original TDZ proxy: consume exactly one header ending in CRLFCRLF, send the fixed TDZ `HTTP/1.1 101` response, then forward all remaining bytes unchanged. GET, POST, CONNECT and custom header contents use the same response.
- An injected payload comes before the native transport/TLS handshake. Matching native handshakes are also detected and replayed intact; OFF uses ordinary inbound behavior. No extra backend TCP port is allocated.
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

- Inbound edits commit database changes before updating Xray, preventing save failures from leaving a different live listener and reducing database-lock conflicts with background statistics.
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
