# TDZ X-UI

An actively maintained 3X-UI distribution with native IP admission limits, scoped reseller access, per-client traffic resets, and a built-in payload proxy manager.

[![Latest release](https://img.shields.io/github/v/release/yeasinulhoquetuhin/x-ui?label=release&color=008771)](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest)
[![License](https://img.shields.io/github/license/yeasinulhoquetuhin/x-ui)](LICENSE)
[![Build](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml/badge.svg)](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml)

**Maintainer:** [Yeasinul Hoque Tuhin](https://github.com/yeasinulhoquetuhin)

**Current release:** `v2.9.3-patched.3` · **Base panel:** 3X-UI 2.9.3

[Install](#install-or-update) · [Downloads](#downloads) · [Resellers](#inbound-reseller-access) · [IP limits](#native-ip-limits) · [Traffic resets](#client-traffic-resets) · [Build](#building-from-source)

## Install or update

Run as root on the server:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

Without an argument, the installer selects the latest published package for your architecture. An in-place update preserves the database, panel credentials, base path, port, SSL configuration, service file, and local certificates. The application and database are backed up before migration; a failed migration restores the previous setup.

### Choose a version

Append `v0`, `v1`, `v2`, or `v3` to the same command:

```bash
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

Full release tags also work. If a version or architecture package is unavailable, the installer lists available releases before changing the installation. Enter `y` for the latest release, another version such as `v1`, or `n` to cancel.

Archived packages retain their original embedded build versions. The release names above are the public version-selection scheme.

## Downloads

Each current package contains the panel, a matching patched Xray core, service scripts, and GeoIP/GeoSite data.

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
3. Leave **Allow Inbound Editing** off for client-only management, or enable it for this account.
4. Save, then copy the account's login URL using the link button. Successful copies show a confirmation.

For a Path ID of `shop-one`, the login address is:

```text
https://panel.example.com:2053/your-panel-path/shop-one/
```

The reseller can create, edit, enable, disable, delete, and reset clients within assigned multi-client inbounds. Bulk creation, usage statistics, connection links, QR codes, subscriptions, Traffic Reset, and subscription IP-limit visibility are included. There is no reseller client-count cap.

Each account can have multiple inbounds, and the same inbound can belong to multiple resellers. Sharing an inbound shares its full client list and usage: a client change by one reseller is visible to the others assigned to it. The dashboard's **Total Inbounds** counts only assigned inbounds. An enabled account with no assignments can log in and sees **No inbounds assigned yet.** Assignment changes appear in the open dashboard automatically.

**Allow Inbound Editing** defaults to off. When enabled, the reseller can edit configuration and enable/disable assigned inbounds, including access to configuration secrets needed for editing. Inbound edits preserve the latest clients and traffic counters, so a stale editor does not overwrite another reseller's client changes. Inbound creation/deletion, unassigned inbounds, panel settings, server controls and backups remain administrator-only. API checks and separate live-update streams enforce these boundaries.

Changing the reseller username, password, or Path ID, disabling the account, or deleting it invalidates previous sessions, including open live-update connections. Permission/assignment changes take effect without requiring a new login. Deleting an inbound removes its assignments and keeps reseller accounts available. Recent actions, including the affected inbound number, are available from the account's activity button.

Existing V2 per-inbound accounts migrate automatically: login URLs, usernames, password hashes and activity history are preserved, the original inbound becomes an assignment, and inbound editing stays off. Migration runs once and does not recreate deleted accounts or removed assignments on subsequent restarts.

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

## Payload / Proxy

Run `x-ui`, then select **4. Payload / Proxy** to open **X-UI Payload Proxy**.

- Create a public-port → backend-port proxy.
- List and manage proxies; change ports, start/stop, restart, inspect logs, or delete an individual instance.
- Panel installation installs only the manager. It creates/imports no proxy instances automatically.
- The proxy submenu has no whole-manager uninstall option.

Menu **1. Install** and **2. Update** use this repository's installer. **3. Update Menu** updates only the CLI menu scripts.

## Building from source

The release source archive includes the panel, tests, Xray patch set, pinned original Xray source, and license files. Use the Go version declared in `go.mod`, Python 3 for the core patcher, and a C compiler for SQLite/CGO.

```bash
go build -o x-ui main.go
bash build_xray.sh ./bin/xray-linux-amd64
```

The core is pinned to Xray `v26.4.25`, revision `b4f08981becb71eaa995fa98ed2098ade92566bb`, and identifies itself as `TDZ-IPLimit/2`. `build_xray.sh` uses the bundled, checksum-verified source archive. The original Go module namespace is retained for local source compatibility.

For cross-compilation, set `GOOS`, `GOARCH`, `GOARM` where applicable, and an appropriate CGO `CC`. ARM panel builds set `xray.armVariant` to `armv5`, `armv6`, or `armv7` for architecture-correct core updates. See [the release workflow](.github/workflows/release.yml) for platform configuration.

## Verification

- Go unit/integration tests, race detection, `go vet`, and static analysis.
- Installer data preservation, migration rollback, version selection, and menu-only updates.
- Browser checks for reseller login, scoped client management, copy feedback, session revocation, Traffic Reset details, and subscription IP tags.
- Real multi-source connection tests across supported VLESS, VMess, Trojan, and Shadowsocks transport cases, including MUX and the local payload proxy.
- Architecture build/version checks and published-asset checksums.

See [RELEASE_NOTES.md](RELEASE_NOTES.md) for release-specific changes and [resources/README.md](resources/README.md) for bundled-data provenance.

## Credits and license

Maintained by **Yeasinul Hoque Tuhin**. Built on **3X-UI** and **Xray-core**; original copyright notices and licenses are retained. GeoIP/GeoSite dataset provenance is documented with the release resources.

[Project page](https://tuhinbro.com/project/x-ui-mod) · [GitHub releases](https://github.com/yeasinulhoquetuhin/x-ui/releases) · [License](LICENSE)
