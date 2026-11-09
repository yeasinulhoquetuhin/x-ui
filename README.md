<p align="center">
  <img src="https://github.com/yeasinulhoquetuhin/x-ui/raw/master/media/3x-ui-light.png" alt="TDZ X-UI Logo" width="200">
</p>

<h1 align="center">TDZ X-UI</h1>
<p align="center"><b>3X-UI v2.9.3-patched.5</b></p>

<p align="center">
  <a href="https://github.com/yeasinulhoquetuhin/x-ui/releases/latest">
    <img src="https://img.shields.io/github/v/release/yeasinulhoquetuhin/x-ui?color=blue&label=release&style=for-the-badge" alt="Release">
  </a>
  <a href="https://github.com/yeasinulhoquetuhin/x-ui/blob/master/LICENSE">
    <img src="https://img.shields.io/github/license/yeasinulhoquetuhin/x-ui?color=green&style=for-the-badge" alt="License">
  </a>
  <a href="https://github.com/yeasinulhoquetuhin/x-ui/commits/master">
    <img src="https://img.shields.io/github/last-commit/yeasinulhoquetuhin/x-ui?style=for-the-badge" alt="Last Commit">
  </a>
  <img src="https://img.shields.io/badge/arch-amd64%20%7C%20arm64-orange?style=for-the-badge" alt="Architectures">
</p>

<p align="center">
  A modded version of <b>3x-ui v2.9.3</b> with native per-client IP limits, live IP reporting, automatic client traffic resets, settings-preserving updates, and a refreshed light theme.
</p>

---

## ◈ Quick Install

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

The installer auto-detects your architecture. Re-running it updates the application while preserving existing SSL, credentials, panel path, port, database and local certificates. Native IP limits use the patched core included in the release.

No version argument installs the latest published package. Select an older patched version with a shortcut:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v1
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v2
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v3
```

`v1` maps to `v2.9.3-patched.1`, and so on. Full release tags also work. If the requested version has no package for this architecture, the installer lists available releases and lets you accept the latest with `y`, select another version, or cancel before changing the installation.

---

## ◈ Architecture Support

| Architecture | File | Download |
|---|---|---|
| **x86_64 / amd64** | `x-ui-linux-amd64.tar.gz` | [**Download**](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-amd64.tar.gz) |
| **ARM64 / aarch64** | `x-ui-linux-arm64.tar.gz` | [**Download**](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-linux-arm64.tar.gz) |
| **Source** | `x-ui-mod-source.tar.gz` | [**Download**](https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/x-ui-mod-source.tar.gz) |

> Both Linux packages contain the matching patched Xray core. Check `SHA256SUMS` from the same release.

---

## ◈ What's Modded?

This isn't just a re-upload — it's a **patched fork** with real fixes and new features on top of the original 3x-ui:

| # | Feature | Status |
|---|---|---|
| 1 | Native per-client IP admission limits, including concurrent streams and MUX | ✅ Done |
| 2 | SSL Skip option (option 4) for fresh installs; existing SSL preserved | ✅ Done |
| 3 | Default theme changed from black to light | ✅ Done |
| 4 | Live IP reporting from core connection statistics | ✅ Done |
| 5 | Telegram Bot — full support, no webhook errors | ✅ Done |
| 6 | SSL works with domain & IP certificates — Let's Encrypt auto-renew | ✅ Done |
| 7 | WebBasePath preserved during updates — no unwanted changes | ✅ Done |
| 8 | Credentials changes invalidate existing panel sessions and WebSockets | ✅ Done |
| 9 | Pre-built amd64 and arm64 packages with saved matching source | ✅ Done |
| 10 | Per-client daily, weekly, monthly and custom-day auto traffic reset | ✅ Done |

---

## ◈ IP Limit — How It Works

The bundled core reserves an IP slot **before routing a client's authenticated stream**:

1. Concurrent connections from one source IP share one slot.
2. A new source IP is rejected while all configured slots are occupied.
3. Closing the final stream releases its slot, including long-lived and MUX streams.
4. Refreshing the IP list queries open connections through the core stats API; disconnected historical IPs are removed.

Install/update packages and core/GeoIP/GeoSite downloads use this repository's releases. The original Go module namespace is kept for local import compatibility.

The default access-log path is `./access.log`. Native IP limiting and live reporting also work when access logging is disabled. Install the matching TDZ core; a stock core does not contain the admission limiter.

IP limits reserve slots for real source addresses: the first admitted IP stays connected and new IPs are rejected while the limit is full. A shared public IP consumes one slot. Limited accounts reject unresolved loopback/private relay sources instead of silently putting every device into one proxy slot.

**X-UI Payload Proxy** is bundled under **x-ui → 4. Payload / Proxy**. Create, list, edit ports, start/stop, restart, view logs or delete individual proxies using the matching green CLI. Panel installation installs the manager only; it never creates or imports proxy instances automatically. Existing managed proxies are preserved. Whole-manager uninstall has been removed from the proxy menu. Real source-IP admission uses the same private, live-socket-validated channel. No client payload change is needed.

```bash
x-ui
# Choose 4. Payload / Proxy
```

The local payload-proxy channel works automatically. For other reverse proxies/CDNs, HTTP transport IP headers are accepted only from explicitly trusted proxy addresses: set `TDZ_XRAY_TRUSTED_PROXIES` to a comma-separated list of exact proxy IPs/CIDRs in the panel service environment (include `127.0.0.1,::1` for a local HTTP reverse proxy). The proxy must overwrite or append the actual peer address. PROXY protocol remains available for correctly configured trusted listeners.

## ◈ Inbound Reseller Access

Open an inbound's three-dot menu → **Reseller Access**. Enable access and set a unique **Path ID**, username and password, then save. **Copy Inbound Login URL** shares a URL such as `https://host:port/panelpath/shop-one/`.

Path IDs, usernames and passwords have no arbitrary minimum length or form length cap: one-character values work. A path must still be a non-empty URL segment and cannot conflict with a reserved panel route. Existing password hashes remain supported. Save, Copy URL and Activity buttons retain the inbound context when clicked.

The reseller signs in with separate credentials and sees only the assigned inbound's clients, usage and online status. Client creation, editing, bulk creation, deletion, enable/disable, traffic resets, QR codes and subscription links are available. Inbound configuration, other inbounds, server controls, panel settings and backups remain admin-only. Admins retain full control of every client. No maximum-client cap is imposed.

Changing the username, password or Path ID, disabling access, or deleting the inbound revokes the reseller's sessions, including an open live-update connection. The admin modal shows recent reseller client actions. Supported multi-client protocols are VLESS, VMess, Trojan, multi-user Shadowsocks and Hysteria.

## ◈ Subscription IP Limit Display

In the admin or reseller client editor, enable **Show IP Limit on Subscription Page** for an individual client. Its subscription web page then displays **IP Limit 0/2**, **1/2**, etc. immediately after Total Quota. The count uses the same bordered tag style as client details: green below the limit, red when full (for example `1/1`), orange when unavailable. The count is a current core snapshot, loaded when the page is opened or reloaded. Turning the switch off hides the row; existing clients default to off. Unlimited IP policies display `∞`.

Menu **1. Install** and **2. Update** use this repository's `install.sh`; **3. Update Menu** downloads only `x-ui.sh`, validates it and replaces the CLI menu.

## ◈ Client Traffic Reset

Choose **Traffic Reset → Hourly / 12 Hourly / Daily / Weekly / Monthly / Custom days** in the client editor. The schedule resets used traffic while preserving the total quota, expiry and all-time usage. Hourly intervals are measured from activation, missed cycles catch up after downtime, and delayed-start clients begin their cycle on first use. Monthly cycles stay anchored to activation time.

For a six-month account with a 100 GB monthly allowance: set expiry six months ahead, Total Traffic to 100 GB and Traffic Reset to Monthly. Leave account auto-renewal disabled if access must end after those six months.

---

## ◈ Changelog (v2.9.3-patched.5)

- **Reseller buttons** — Fix undefined inbound IDs on actual Save/Copy/Activity button clicks
- **Flexible credentials** — One-character paths/usernames/passwords and long passwords supported
- **Version selection** — Latest by default, `v1`/`v2`/`v3` shortcuts and missing-version selection prompt
- **Proxy manager** — Updated green CLI, no manager-uninstall option, no automatic proxy creation/import
- **IP tags** — Bordered green/red subscription IP-limit indicators

- **Reseller Access** — Per-inbound login, editable custom path, isolated client management and revocable live sessions
- **Subscription IP Limit** — Per-client opt-in active/maximum IP display in admin and reseller editors
- **Payload / Proxy** — Built-in X-UI proxy manager in menu 4, preserving existing proxy instances
- **Manual IP Details** — Load once on opening; refresh using the existing button

- **Client compatibility** — Empty and numeric-string Telegram IDs decode correctly alongside JSON numbers; no database reset required

- **IP Limit** — Atomic admission checks inside the bundled Xray core
- **SSL Skip** — Bypass SSL setup during install or update with option 4
- **Light Theme** — Default panel theme switched to light mode
- **Live IPs** — Core connection statistics replace access-log history
- **Telegram Bot** — Stable webhook integration, zero errors
- **SSL Auto-Renew** — Let's Encrypt certificates renew for both domain and IP-based setups
- **WebBasePath** — Unchanged during updates, preserving your custom path
- **Sessions** — Credential changes log out existing panel sessions
- **Multi-Arch** — amd64 and arm64 packages with bundled core source, patches and tests
- **Traffic Reset** — Per-client hourly, 12-hourly, daily, weekly, monthly or custom-day schedules
- **Reinstall** — Existing settings preserved with migration rollback backup

See [RELEASE_NOTES.md](RELEASE_NOTES.md) for verification coverage and usage details.

---

## ◈ Credits

> *Patched and maintained by **Yeasinul Hoque Tuhin***

**Project Page:** [tuhinbro.com/project/x-ui-mod](https://tuhinbro.com/project/x-ui-mod)

---

<p align="center">
  <a href="https://github.com/yeasinulhoquetuhin/x-ui">
    <img src="https://img.shields.io/github/stars/yeasinulhoquetuhin/x-ui?style=social" alt="Stars">
  </a>
  <a href="https://github.com/yeasinulhoquetuhin/x-ui/fork">
    <img src="https://img.shields.io/github/forks/yeasinulhoquetuhin/x-ui?style=social" alt="Forks">
  </a>
</p>

<p align="center">
  <sub>Built for the community</sub>
</p>
