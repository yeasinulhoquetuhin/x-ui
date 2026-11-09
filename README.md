# TDZ X-UI

An actively maintained 3X-UI distribution with native IP admission limits, scoped reseller access, per-client traffic resets, and a built-in payload proxy manager.

[![Latest release](https://img.shields.io/github/v/release/yeasinulhoquetuhin/x-ui?label=release&color=008771)](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest)
[![License](https://img.shields.io/github/license/yeasinulhoquetuhin/x-ui)](LICENSE)
[![Build](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml/badge.svg)](https://github.com/yeasinulhoquetuhin/x-ui/actions/workflows/release.yml)

**Maintainer:** [Yeasinul Hoque Tuhin](https://github.com/yeasinulhoquetuhin)

**Current release:** `v2.9.3-patched.2` · **Base panel:** 3X-UI 2.9.3

[Install](#install-or-update) · [Downloads](#downloads) · [Resellers](#inbound-reseller-access) · [IP limits](#native-ip-limits) · [Traffic resets](#client-traffic-resets) · [Build](#building-from-source)

## Install or update

Run as root on the server:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

Without an argument, the installer selects the latest published package for your architecture. An in-place update preserves the database, panel credentials, base path, port, SSL configuration, service file, and local certificates. The application and database are backed up before migration; a failed migration restores the previous setup.

### Choose a version

Append `v0`, `v1`, or `v2` to the same command:

```bash
# July 2026 archive
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v0

# Previous native-IP-limit release
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v1

# Current release
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v2
```

| Shortcut | Release | Contents |
| --- | --- | --- |
| `v0` | [`v2.9.3-patched.0`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.0) | Original July release; archived packages and source |
| `v1` | [`v2.9.3-patched.1`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.1) | Former patched.3 release; preserved packages and source |
| `v2` | [`v2.9.3-patched.2`](https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched.2) | Former patched.5 feature set plus the final UI refinements |

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

Assign client management without handing over the administrator account:

1. Open an inbound's three-dot menu and select **Reseller Access**.
2. Enable access and choose a **Path ID**, username, and password.
3. Save, then use **Copy Inbound Login URL**. A confirmation appears when copying succeeds.

For a Path ID of `shop-one`, the login address is:

```text
https://panel.example.com:2053/your-panel-path/shop-one/
```

The reseller can create, edit, enable, disable, delete, and reset clients within the assigned inbound. Bulk creation, usage statistics, connection links, QR codes, subscriptions, Traffic Reset, and subscription IP-limit visibility are included. There is no reseller client-count cap.

The administrator retains full control. Inbound configuration, other inbounds, panel settings, server controls, backups, and global live updates remain admin-only. Restrictions are enforced by server-side API checks and separate reseller sessions.

Changing the reseller username, password, or Path ID, disabling access, or deleting the inbound invalidates previous sessions, including open live-update connections. Recent reseller actions are available in the admin access modal.

Path IDs and credentials have no arbitrary minimum-length rule. The path must remain a valid, non-empty URL segment and must not conflict with reserved panel routes. Passwords are hashed; an empty password field when editing keeps the existing password.

## Native IP limits

IP admission is enforced inside the bundled Xray core before an authenticated stream is routed:

- Streams from one canonical source IP share a slot.
- Once the limit is full, a new source IP is rejected while the first admitted IP continues working.
- Closing the last stream from an IP releases its slot, including MUX sessions.
- Client IP details show the core's current connection snapshot. Open the details or use **Refresh** to update it; no automatic IP-log polling is enabled.

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
