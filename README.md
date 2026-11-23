# TDZ X-UI - And This is Best of X-UI 

**A complete X-UI / 3X-UI control panel for Xray servers:** real-IP admission for ordinary connections, native encrypted SlowDNS, multi-inbound reseller management, scheduled traffic resets, and single-port TDZ payload acceptance.

[![Latest release](https://img.shields.io/github/v/release/yeasinulhoquetuhin/x-ui?label=release&color=008771)](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest)
[![License](https://img.shields.io/github/license/yeasinulhoquetuhin/x-ui)](LICENSE)
[![Build](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml/badge.svg)](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml)

**Maintainer:** [Yeasinul Hoque Tuhin](https://github.com/yeasinulhoquetuhin)

**V5:** [`2.9.3-patched.5`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.5) · **Base:** 3X-UI 2.9.3 · **Xray:** 26.4.25

## Features

| Capability | What you get |
| --- | --- |
| **Real connection admission** | Ordinary authenticated connections are limited inside Xray, with atomic admission, live counts and stream-close cleanup |
| **Native SlowDNS** | Stock-DNSTT-compatible encrypted streams inside Xray, using one shared UDP listener and the same inbound authentication/routing |
| **Clear DNSTT policy** | DNSTT is unlimited by the IP-limit setting and excluded from IP counts/logs; ordinary connections to the same inbound still enforce their IP limit |
| **One-port payload support** | TCP/RAW, WS, HTTPUpgrade, gRPC and TCP XHTTP can accept the original TDZ payload preface on their existing port |
| **Independent resellers** | Multiple/shared inbound assignments, per-account edit/password permissions, scoped APIs and live session revocation |
| **Flexible account cycles** | Traffic Reset schedules, total quota, expiry, delayed activation and all-time usage accounting |
| **Client-ready sharing** | URI, JSON and Clash/Mihomo subscriptions, QR/copy actions, clean client labels and opt-in public connection hints |
| **Persistent DNS keys** | Database-backed X25519 keys survive domain/MTU edits and SlowDNS uninstall/reinstall |
| **SSH coexistence** | Separate-zone DNS routing allows TDZ SSH and X-UI to share public UDP 53 with independent internal listeners |
| **Reproducible releases** | Pinned core/DNSTT sources, Linux/Windows packages, source archive, SHA256 checksums and multi-architecture Docker images |

[V4 → V5 changes](RELEASE_NOTES.md) · [Changelog](CHANGELOG.md) · [SlowDNS guide](SLOWDNS.md)

[Install](#install-or-update) · [Support](#protocols-transports-and-security) · [SlowDNS](#slowdns-management) · [Downloads](#downloads) · [Accept Payloads](#accept-payloads) · [Resellers](#inbound-reseller-access) · [IP limits](#native-ip-limits) · [Build](#building-from-source)

## Install or update

Run as root on the server:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

Without an argument, the installer selects the latest published package for your architecture. An in-place update preserves the database, panel credentials, base path, port, SSL configuration, service file, and local certificates. The application and database are backed up before migration; a failed migration restores the previous setup.

To select V5 explicitly:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v5
```

Historical shortcuts `v0`–`v4` and full release tags also work. See [all releases](https://github.com/yeasinulhoquetuhin/x-ui/releases).

## Downloads

Each package contains the panel, matching patched Xray core, GeoIP/GeoSite data and release documentation. Linux packages include service/CLI scripts; Windows packages include the Windows resources.

| Platform | Release asset |
| --- | --- |
| Linux x86-64 | [`x-ui-linux-amd64.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-amd64.tar.gz) |
| Linux ARM64 | [`x-ui-linux-arm64.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-arm64.tar.gz) |
| Linux x86, 32-bit | [`x-ui-linux-386.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-386.tar.gz) |
| Linux ARMv5 | [`x-ui-linux-armv5.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-armv5.tar.gz) |
| Linux ARMv6 | [`x-ui-linux-armv6.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-armv6.tar.gz) |
| Linux ARMv7 | [`x-ui-linux-armv7.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-armv7.tar.gz) |
| Linux s390x | [`x-ui-linux-s390x.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-s390x.tar.gz) |
| Windows x86-64 | [`x-ui-windows-amd64.zip`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-windows-amd64.zip) |
| Matching source | [`x-ui-mod-source.tar.gz`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-mod-source.tar.gz) |

Verify downloads against [`SHA256SUMS`](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/SHA256SUMS). Historical releases contain the architectures available when those packages were built.

## Protocols, transports and security

| Inbound protocol / mode | Panel support | Native IP limits | SlowDNS option |
| --- | --- | --- | --- |
| **VLESS** | UUID, optional VLESS Encryption, Vision/fallback configuration | Authenticated client emails | NTLS on the five TCP transports below |
| **VMess** | UUID and supported VMess security modes | Authenticated client emails | NTLS on the five TCP transports below |
| **Trojan** | Password clients and fallback configuration | Authenticated client emails | NTLS on the five TCP transports below |
| **Shadowsocks** | ChaCha20/XChaCha20 and Shadowsocks 2022 methods exposed by the panel | Email-accounted clients | TCP streams on the five transports below |
| **Hysteria v1 / v2** | QUIC/UDP configuration and client authentication | Email-accounted clients with the bundled core | — |
| **WireGuard** | WireGuard peers and connection configuration | Peer keys; no email-based IP-admission editor | — |
| **Mixed / SOCKS** | SOCKS-compatible proxy mode | No panel multi-client IP-admission editor | — |
| **HTTP proxy** | HTTP proxy configuration | No panel multi-client IP-admission editor | — |
| **Tunnel / Dokodemo-door** | TCP/UDP forwarding and redirect configuration | No authenticated client identity | — |
| **TUN** | TUN interface configuration | Interface traffic, not per-client proxy accounts | — |

| Transport | Ordinary inbound use | Accept Payloads | SlowDNS |
| --- | --- | --- | --- |
| **TCP / RAW** | Native TCP stream; optional HTTP camouflage | Yes | NTLS |
| **WebSocket** | Matching path/Host and WS handshake | Yes | NTLS |
| **HTTPUpgrade** | Matching HTTP upgrade configuration | Yes | NTLS |
| **gRPC** | HTTP/2, matching service name/authority | Yes | NTLS |
| **XHTTP** | Supported TCP modes: auto, packet-up, stream-up, stream-one | TCP only | NTLS over TCP |
| **mKCP** | UDP-based transport | — | — |
| **Hysteria / QUIC** | UDP/QUIC with its required transport security | — | — |
| **XHTTP over HTTP/3** | QUIC-capable client and configuration | — | — |

Transport choices depend on the selected protocol. The panel controls the supported combinations; clients need matching protocol, transport and security settings.

### TLS, REALITY, Vision and ALPN

- **TLS:** server certificates, SNI, supported TLS-version/cipher controls, fingerprints, certificate pinning and exposed ECH options.
- **REALITY:** exposed for compatible VLESS/Trojan configurations on TCP, gRPC or XHTTP; matching target/server name, public key and short ID are required.
- **XTLS Vision:** VLESS on compatible TLS/REALITY TCP configuration. SlowDNS uses NTLS and does not enable a Vision flow.
- **NTLS / `security: none`:** no inner TLS. SlowDNS still encrypts/authenticates its outer DNSTT tunnel using Noise/X25519.

| ALPN | Purpose | Typical configuration |
| --- | --- | --- |
| `http/1.1` | HTTP/1.1 negotiation | TLS WebSocket / HTTPUpgrade |
| `h2` | HTTP/2 negotiation | TLS gRPC and TCP-based XHTTP |
| `h3` | HTTP/3 over QUIC | Hysteria / QUIC-capable XHTTP |

ALPN is TLS application-protocol negotiation. NTLS SlowDNS has no inner TLS ALPN, and choosing `h3` does not turn a TCP transport into QUIC. See the [official TLS reference](https://xtls.github.io/en/config/transports/tls.html).

### Outbounds and routing

The outbound editor exposes **Freedom/direct, Blackhole/block, DNS, VLESS, VMess, Trojan, Shadowsocks, SOCKS, HTTP, WireGuard and Hysteria**. Routing includes domain/IP/GeoIP/GeoSite rules, sniffing, DNS/FakeDNS, outbound chaining and supported balancer controls. Panel subscriptions support URI, JSON and Clash/Mihomo output with client-app compatibility requirements.

## SlowDNS Management

Open **`x-ui → 23. SlowDNS Management`** or run `x-ui slowdns`:

1. Choose **Install / Set up**.
2. Enter a **Host domain** for the nameserver, e.g. `ns.example.com`, with an A/AAAA record pointing to the listening VPS.
3. Enter the delegated **NS domain / tunnel zone**, e.g. `xui.example.com`; create `NS xui.example.com → ns.example.com`.
4. Press Enter for MTU **1232**, the upstream DNSTT default selected to avoid common IPv6 fragmentation. A custom **512–4096** value is accepted; `512` helps resolvers with smaller DNS response limits.
5. Enable **SlowDNS** on a supported NTLS inbound. The option starts off, and TLS/REALITY hides and disables it.

Every DNS-enabled inbound uses the same client NS domain: **`xui.example.com`**. Transport framing and distinct protocol credentials select the inbound without an `i<inbound-id>.` prefix. Several inbounds share one public UDP **53** entry. Linux uses an internal **5301/UDP** listener by default, separate from TDZ SSH's **54/UDP** listener.

**More Information** shows a bottom SlowDNS card containing **NameServer** and **Public Key**, both clickable to copy. Enable **Show SlowDNS on Subscription Page** per client to display the same fields **below client configs**. It defaults to off. Google and Cloudflare resolvers are accepted without displaying resolver fields. Standard URI/JSON/Clash profiles keep their ordinary formats; a DNSTT-compatible client uses these hints and the existing protocol credentials. CLI **23 → 3** keeps its details page open until **0** returns to the main menu.

**DNSTT is unlimited by the IP-limit setting and excluded from IP Log/subscription IP counts.** Ordinary connections to the same inbound still enforce their real-IP limit. Protocol authentication, quota and account policies apply to DNS traffic.

### Stable keys and TDZ SSH coexistence

- Domains, MTU and listener edits keep the database keypair. SlowDNS uninstall disables inbound opt-ins and retains keys; reinstall keeps the public key.
- Only **Regenerate / Revoke public key** explicitly creates a replacement keypair. Back up the panel database to preserve keys across a complete panel/database reinstall.
- Give TDZ SSH and X-UI **separate sibling tunnel zones**, such as `ssh.example.com` and `xui.example.com`. The nameserver host can be shared.
- Domain-specific, earlier-priority DNS routing selects X-UI while TDZ SSH keeps its own default DNS forwarding. Starting/stopping/reinstalling the SSH forwarding component does not override the X-UI zone.

See [SLOWDNS.md](SLOWDNS.md) for command-line examples, DNS records, client setup and coexistence details.

## Inbound reseller access

Create accounts from **Reseller** in the administrator sidebar:

1. Select **Create Reseller** and choose a **Path ID**, username, and password.
2. Select any number of inbounds in the searchable **Assigned Inbounds** picker, or leave it empty.
3. Set **Edit Inbounds** and **Edit Password** permissions for this account. Both default to off.
4. Save, then copy the account's login URL using the link button. Successful copies show a confirmation.

For a Path ID of `shop-one`, the login address is:

```text
https://panel.example.com:2053/reseller/shop-one/
```

The reseller can create, edit, enable, disable, delete, and reset clients within assigned multi-client inbounds. Bulk creation, usage statistics, connection links, QR codes, subscriptions, Traffic Reset, and subscription IP-limit visibility are included. There is no reseller client-count cap.

- Multiple/shared inbound assignments are supported. Sharing an inbound shares its client list and traffic visibility.
- **Edit Inbounds** enables editing assigned inbound configurations; panel/server controls stay administrator-only.
- **Edit Password** exposes the reseller password-change page. Credential changes, disabling or deletion revoke existing sessions.
- Client labels omit the inbound-name prefix in subscriptions, Telegram links, copy/QR and exports.

## Native IP limits

IP admission is enforced inside the bundled Xray core before an authenticated stream is routed:

- Streams from one canonical source IP share a slot.
- Once the limit is full, a new source IP is rejected while the first admitted IP continues working.
- Closing the last stream from an IP releases its slot, including MUX sessions.
- Client IP details show the core's current connection snapshot. Open the details or use **Refresh** to update it; no automatic IP-log polling is enabled.
- **Clear** removes the stored/displayed IP records without fetching another snapshot. Connected IPs appear again when you explicitly refresh or reopen the details.

Direct IP limits count source addresses, not physical devices. Devices sharing one public IP occupy one IP slot. Native DNSTT is excluded from IP admission/statistics. Unresolved local/private addresses from ordinary relays are rejected for limited accounts.

Reverse proxies must forward the actual client address through trusted headers or PROXY protocol. For HTTP headers, configure trusted peer IPs/CIDRs with `TDZ_XRAY_TRUSTED_PROXIES`. Native IP enforcement does not depend on access-log scanning or Fail2ban.

### Show IP usage on subscriptions

Enable **Show IP Limit on Subscription Page** in the admin or reseller client editor. The client's subscription web page displays the current count directly after Total Quota:

| Example | Meaning |
| --- | --- |
| `0/2` or `1/2` — green | Slots available |
| `1/1` or `2/2` — red | Limit full |
| `Unavailable/2` — orange | Live statistics could not be read |

The setting is per client and defaults to off. Actual IP addresses are not exposed on the subscription page. Reload the page to request a fresh count. DNSTT-only traffic shows **0** ordinary IP slots, even when the account's limit is 1.

## Client traffic resets

Choose **Traffic Reset** when creating or editing a client:

**Disabled · Hourly · Every 12 hours · Daily · Weekly · Monthly · Custom days**

Client details show the selected cycle and next reset time. Delayed-start accounts show **Starts after first use** until activation. Only current-cycle upload/download usage resets; quota, all-time usage, account expiry, and manual enable state are preserved. Monthly schedules stay anchored across shorter months, and missed cycles catch up after downtime.

**Example:** for a six-month account with a 100 GB monthly allowance, set Total Traffic to 100 GB, expiry six months ahead, and Traffic Reset to Monthly. Leave account auto-renewal disabled if access must stop at the fixed expiry.

## Accept Payloads

Enable **Accept Payloads** for TCP/RAW, WS, HTTPUpgrade, gRPC or XHTTP over TCP to use the original TDZ proxy handshake on that same port: consume the first CRLFCRLF-terminated payload, reply with the fixed 101 response, and pass the remaining stream unchanged. The client sends the payload before its normal transport/TLS handshake. The option defaults to off, survives switching supported transports, and is mutually exclusive with HTTP Obfuscation. See [ACCEPT_PAYLOADS.md](ACCEPT_PAYLOADS.md) for details.

```text
Client → inbound port → payload → fixed TDZ 101 → configured transport → Xray authentication
```

With the switch **ON**, payload-aware clients can send the initial payload; GET, POST, CONNECT and custom prefaces use the same fixed TDZ response. Native binary/TLS, HTTP/2 and matching HTTP transport handshakes are detected and replayed intact. For a payload-prefaced WS connection, the real WS handshake follows the TDZ response; a payload-prefaced TLS connection sends the payload before TLS. UDP-based KCP, Hysteria and QUIC/HTTP3 do not support this option. Client apps must expand payload macros and use the matching transport and authentication.

Existing two-port proxies are still managed with `x-ui-proxy`. Main CLI choices run from **0 to 26**, with **23 SlowDNS**, **24 BBR**, **25 Geo Files** and **26 Speedtest**. **9. View Current Settings** can request an IP certificate without opening a hidden nested menu; failures or cancellation restore a previously running panel.

Menu **1. Install** and **2. Update** use this repository's installer. **3. Update Menu** updates only the CLI menu scripts.

## Building from source

The release source archive includes the panel, tests, Xray patch set, pinned original Xray source, and license files. Use the Go version declared in `go.mod`, Python 3 for the core patcher, and a C compiler for SQLite/CGO.

```bash
go build -o x-ui main.go
bash build_xray.sh ./bin/xray-linux-amd64
```

See [the release workflow](.github/workflows/release.yml) for cross-compilation and [resources/README.md](resources/README.md) for bundled-data provenance. Docker images support `linux/amd64` and `linux/arm64` at `ghcr.io/yeasinulhoquetuhin/x-ui`.

## Credits and license

Maintained by **Yeasinul Hoque Tuhin**. Built on **3X-UI** and **Xray-core**; original copyright notices and licenses are retained. GeoIP/GeoSite dataset provenance is documented with the release resources.

[Project page](https://tuhinbro.com/project/x-ui-mod) · [GitHub releases](https://github.com/yeasinulhoquetuhin/x-ui/releases) · [License](LICENSE)
