# TDZ X-UI

A 3X-UI distribution with native IP limits, independent reseller accounts, scheduled client traffic resets, and TDZ payload acceptance on the inbound's own port.

[![Latest release](https://img.shields.io/github/v/release/yeasinulhoquetuhin/x-ui?label=release&color=008771)](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest)
[![License](https://img.shields.io/github/license/yeasinulhoquetuhin/x-ui)](LICENSE)
[![Build](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml/badge.svg)](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml)

**Maintainer:** [Yeasinul Hoque Tuhin](https://github.com/yeasinulhoquetuhin)

**Current release:** [`v2.9.3-patched.4`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.4) · **Base panel:** 3X-UI 2.9.3 · **Bundled Xray:** 26.4.25, `TDZ-IPLimit/2+Payload/3`

## What's new in V4

| Feature | Behavior |
| --- | --- |
| **Accept Payloads** | Original TDZ payload handshake on one inbound port; default off, above Proxy Protocol, with saved state across supported transport changes |
| **Reseller management** | Admin account cards, search/status filters, inbound assignments, permission controls and a searchable activity log |
| **Edit Password** | Per-reseller permission, default off; permitted resellers change their own password from the sidebar |
| **Independent reseller URLs** | `/reseller/<path>/` works independently of the admin panel's base path |
| **Client config labels** | Subscription, Telegram, copy/export and QR labels omit the inbound-name prefix |
| **CLI fixes** | Options renumbered through 25; IP SSL setup from View Current Settings no longer opens a hidden nested menu |

See the [V4 release notes](RELEASE_NOTES.md) and [changelog](CHANGELOG.md) for changes and upgrade details.

[Install](#install-or-update) · [Downloads](#downloads) · [Accept Payloads](#accept-payloads) · [Resellers](#inbound-reseller-access) · [IP limits](#native-ip-limits) · [Traffic resets](#client-traffic-resets) · [Build](#building-from-source)

## Install or update

Run as root on the server:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

Without an argument, the installer selects the latest published package for your architecture. An in-place update preserves the database, panel credentials, base path, port, SSL configuration, service file, and local certificates. The application and database are backed up before migration; a failed migration restores the previous setup.

### Choose a version

Append `v4` to install this release explicitly. Historical `v0`–`v3` shortcuts remain available:

```bash
# V4: single-port payloads and reseller management improvements
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v4

# July 2026 archive
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v0

# Previous native-IP-limit release
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v1

# V2 per-inbound reseller release
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v2

# V3 independent reseller accounts
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v3
```

| Shortcut | Release | Contents |
| --- | --- | --- |
| `v0` | [`v2.9.3-patched.0`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.0) | Original July release; archived packages and source |
| `v1` | [`v2.9.3-patched.1`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.1) | Former patched.3 release; preserved packages and source |
| `v2` | [`v2.9.3-patched.2`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.2) | Former patched.5 feature set plus the final UI refinements |
| `v3` | [`v2.9.3-patched.3`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.3) | Independent reseller accounts, multiple/shared inbound assignments and optional inbound editing |
| `v4` | [`v2.9.3-patched.4`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.4) | Single-port TDZ payloads, reseller admin redesign/password permission, independent reseller paths, client labels and CLI fixes |

Full release tags also work. If a version or architecture package is unavailable, the installer lists available releases before changing the installation. Enter `y` for the latest release, another version such as `v1`, or `n` to cancel.

Archived packages retain their original embedded build versions. The release names above are the public version-selection scheme.

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

## Inbound reseller access

Create and manage independent accounts from **Reseller** in the administrator sidebar, directly below Inbounds and above Xray Configs:

1. Select **Create Reseller** and choose a **Path ID**, username, and password.
2. Select any number of inbounds in the searchable **Assigned Inbounds** picker, or leave it empty.
3. Set **Edit Inbounds** and **Edit Password** permissions for this account. Both default to off.
4. Save, then copy the account's login URL using the link button. Successful copies show a confirmation.

For a Path ID of `shop-one`, the login address is:

```text
https://panel.example.com:2053/reseller/shop-one/
```

The reseller can create, edit, enable, disable, delete, and reset clients within assigned multi-client inbounds. Bulk creation, usage statistics, connection links, QR codes, subscriptions, Traffic Reset, and subscription IP-limit visibility are included. There is no reseller client-count cap.

Each account can have multiple inbounds, and the same inbound can belong to multiple resellers. Sharing an inbound shares its full client list and usage: a client change by one reseller is visible to the others assigned to it. The dashboard's **Total Inbounds** counts only assigned inbounds. An enabled account with no assignments can log in and sees **No inbounds assigned yet.** Assignment changes appear in the open dashboard automatically.

**Edit Inbounds** defaults to off. When enabled, the reseller can edit configuration and enable/disable assigned inbounds, including access to configuration secrets needed for editing. Inbound edits preserve the latest clients and traffic counters, so a stale editor does not overwrite another reseller's client changes. Inbound creation/deletion, unassigned inbounds, panel settings, server controls and backups remain administrator-only. API checks and separate live-update streams enforce these boundaries.

**Edit Password** also defaults to off. Enable it per account to show Change Password in the reseller sidebar. Password changes verify the current password, revoke existing sessions and appear in the administrator's activity log. Administrators manage accounts with responsive cards, search, status filters and assignment/permission controls. Reseller portals are at `/reseller/<path>/`, independent of the panel base path; their original dashboard layout is retained and the panel TLS warning is hidden.

Client config labels omit the inbound-name prefix in subscription URI/JSON/Clash output, Telegram-delivered links, and panel copy/QR/export actions, including when an existing installation still has the old remark pattern.

Changing the reseller username, password, or Path ID, disabling the account, or deleting it invalidates previous sessions, including open live-update connections. Permission/assignment changes take effect without requiring a new login. Deleting an inbound removes its assignments and keeps reseller accounts available. Recent actions, including the affected inbound number, are available from the account's activity button.

Existing V2 per-inbound accounts migrate automatically: Path IDs, usernames, password hashes and activity history are preserved, the original inbound becomes an assignment, and inbound editing stays off. Migration runs once and does not recreate deleted accounts or removed assignments on subsequent restarts. On V4, distribute the new `/reseller/<path>/` login URL and sign in again; the previous panel-prefixed reseller URL is replaced.

Path IDs and credentials have no arbitrary minimum-length rule. The path must remain a valid, non-empty URL segment and must not conflict with reserved panel routes. Passwords are hashed; an empty password field when editing keeps the existing password.

Client authentication is inbound-scoped: separate inbounds may use the same protocol UUID/password. Client emails/names stay unique for separate traffic accounting. A subscription shared across multiple inbounds requires all those inbounds to be assigned to the account; new clients cannot join another client's subscription. Conflicts identify the inbound when saving assignments.

## Native IP limits

IP admission is enforced inside the bundled Xray core before an authenticated stream is routed:

- Streams from one canonical source IP share a slot.
- Once the limit is full, a new source IP is rejected while the first admitted IP continues working.
- Closing the last stream from an IP releases its slot, including MUX sessions.
- Client IP details show the core's current connection snapshot. Open the details or use **Refresh** to update it; no automatic IP-log polling is enabled.
- **Clear** removes the stored/displayed IP records without fetching another snapshot. Connected IPs appear again when you explicitly refresh or reopen the details.

IP limits count source addresses, not physical devices. Devices sharing one public IP occupy one IP slot. Limited accounts reject unresolved local/private relay addresses rather than silently treating all proxied clients as one IP.

The bundled payload proxy preserves real IPs through a private, live-socket-validated channel without changing payload or relay bytes. Other reverse proxies must forward the actual client address through configured trusted headers or PROXY protocol. For HTTP proxy headers, configure exact trusted peer IPs/CIDRs with `TDZ_XRAY_TRUSTED_PROXIES` in the panel service environment.

The default access-log path is `./access.log`. Native admission and live IP reporting do not depend on access logging or Fail2ban.

### Show IP usage on subscriptions

Enable **Show IP Limit on Subscription Page** in the admin or reseller client editor. The client's subscription web page displays the current count directly after Total Quota:

| Example | Meaning |
| --- | --- |
| `0/2` or `1/2` — green | Slots available |
| `1/1` or `2/2` — red | Limit full |
| `Unavailable/2` — orange | Live statistics could not be read |

The setting is per client and defaults to off. Actual IP addresses are not exposed on the subscription page. Reload the page to request a fresh count.

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

With the switch **ON**, the initial payload is required. GET, POST, CONNECT and custom header content use the same response; this is the original TDZ relay behavior. For a WS inbound, the real WS handshake follows that response. For TLS, the payload comes before TLS. UDP-based KCP, Hysteria and QUIC/HTTP3 do not support this option. Client apps must expand payload macros and use the matching transport and authentication.

Existing two-port proxies are still managed with `x-ui-proxy`. The main CLI's Payload / Proxy entry has been removed, and the remaining choices now run from 0 to 25. **9. View Current Settings** can request an IP certificate without opening a hidden nested menu; failures or cancellation restore a previously running panel.

Menu **1. Install** and **2. Update** use this repository's installer. **3. Update Menu** updates only the CLI menu scripts.

## Building from source

The release source archive includes the panel, tests, Xray patch set, pinned original Xray source, and license files. Use the Go version declared in `go.mod`, Python 3 for the core patcher, and a C compiler for SQLite/CGO.

```bash
go build -o x-ui main.go
bash build_xray.sh ./bin/xray-linux-amd64
```

The core is pinned to Xray `v26.4.25`, revision `b4f08981becb71eaa995fa98ed2098ade92566bb`, and identifies itself as `TDZ-IPLimit/2+Payload/3`. `build_xray.sh` uses the bundled, checksum-verified source archive. The original Go module namespace is retained for local source compatibility.

For cross-compilation, set `GOOS`, `GOARCH`, `GOARM` where applicable, and an appropriate CGO `CC`. ARM panel builds set `xray.armVariant` to `armv5`, `armv6`, or `armv7` for architecture-correct core updates. See [the release workflow](.github/workflows/release.yml) for platform configuration.

For Docker, the release workflow publishes `linux/amd64` and `linux/arm64` images to `ghcr.io/yeasinulhoquetuhin/x-ui`.

## Verification

- Go unit/integration tests, race detection, `go vet`, and static analysis.
- Installer data preservation, migration rollback, version selection, and menu-only updates.
- Browser checks for reseller login, scoped client management, copy feedback, session revocation, Traffic Reset details, and subscription IP tags.
- Real multi-source connection tests across supported VLESS, VMess, Trojan, and Shadowsocks transport cases, including MUX and the local payload proxy.
- Byte-for-byte comparison with the original TDZ Python proxy, including split/coalesced headers and unchanged post-header data; real payload traffic with supported transports, TLS and live API updates.
- Architecture build/version checks and published-asset checksums.

See [RELEASE_NOTES.md](RELEASE_NOTES.md) for release-specific changes and [resources/README.md](resources/README.md) for bundled-data provenance.

## Credits and license

Maintained by **Yeasinul Hoque Tuhin**. Built on **3X-UI** and **Xray-core**; original copyright notices and licenses are retained. GeoIP/GeoSite dataset provenance is documented with the release resources.

[Project page](https://tuhinbro.com/project/x-ui-mod) · [GitHub releases](https://github.com/yeasinulhoquetuhin/x-ui/releases) · [License](LICENSE)
