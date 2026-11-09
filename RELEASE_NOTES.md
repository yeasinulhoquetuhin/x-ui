# TDZ X-UI v2.9.3-patched.5

- Fix reseller-access Save/Copy URL/Activity buttons losing their inbound context and sending `undefined` IDs (HTTP 400). Verified using actual browser button clicks.
- Remove arbitrary path, username and password minimum/maximum length rules. One-character credentials work; long passwords use a versioned prehash before bcrypt, with existing bcrypt hashes remaining valid.
- Subscription IP-limit counts now use bordered tags: green below the limit, red when full, orange if a live snapshot is unavailable.
- Refresh the Payload / Proxy CLI to match x-ui's green numbered boxed menu. Remove whole-manager uninstall/99. Panel installation installs the manager only and creates/imports no proxy instances automatically.
- Installer without arguments installs the latest published architecture package. Shortcuts `v1`, `v2`, `v3` select the corresponding patched releases. Missing versions list available releases and offer the latest, another version, or cancellation before installation starts.

- Inbound three-dot menu: **Reseller Access** and **Copy Inbound Login URL**. Configure a unique custom Path ID, username and password for each multi-client inbound.
- Separate reseller login/session grants client management for the assigned inbound only. Other inbounds, inbound editing, server controls, panel settings, backups and admin live broadcasts are inaccessible. Admins retain all inbound/client controls; no maximum-client cap is added.
- Passwords are hashed. Editing access credentials/path, disabling access or deleting the inbound revokes existing reseller sessions and live updates. Recent reseller client actions are visible to the admin.
- **Show IP Limit on Subscription Page** is a per-client switch in both admin and reseller editors, including bulk creation. The opt-in row appears directly after Total Quota and shows current slots such as `0/2` or `1/2`. It defaults to off and does not expose client IP addresses.
- **X-UI Payload Proxy** is built into **x-ui menu 4**, replacing Legacy Version. Create/list/edit/start/stop/restart/delete individual proxies and view logs. Existing proxy settings remain preserved; importing old legacy instances requires an explicit migration action.
- Menu Install and Update use this repository's `install.sh`; Update Menu updates only the menu scripts. IP details use the existing manual Refresh button.

- Payload-proxy client IPs use a private, live-socket-validated IP channel. Payload handshake, relay bytes and proxy ports are preserved.
- Limited accounts reject unresolved local/private proxy sources instead of treating all proxied devices as one IP. New IPs are rejected while slots are full; the first admitted IP remains connected.
- HTTP forwarded-IP headers are trusted only from explicitly configured proxy peers; direct connections and unconfigured payload relays cannot choose their counted IP through client-supplied headers.
- Client details load live IPs/counts once when opened; use the Refresh button for subsequent updates. Failed tracking is shown as unavailable, and clearing the display cannot conceal an active IP.
- Add **Hourly** and **12 Hourly** Traffic Reset schedules, anchored to activation and catching up after downtime.

- Hotfix: client Telegram IDs accept empty strings, numeric strings, JSON numbers, null and omitted fields. Existing client records and client forms work without a database reset.

- Native per-client IP admission limits in the bundled Xray core, independent of access-log timing and Fail2ban.
- Live connection IPs from the Xray stats API; offline IPs are removed on refresh.
- Atomic slot reservation across simultaneous connections, shared-IP streams and MUX. Releasing the final stream immediately frees the IP slot.
- Default access log is `./access.log`; existing `/var/log/xray/access.log` defaults migrate automatically.
- Changing panel credentials invalidates existing HTTP sessions and closes authenticated WebSocket connections.
- Re-running the installer updates the application while preserving the database, SSL, credentials, panel path, port, service file and local certificates. Failed migrations restore the previous application and database backup.
- Per-client Traffic Reset: hourly, 12-hourly, daily, weekly, monthly or a custom number of days. Only current-cycle usage resets; account expiry, quota and all-time usage are preserved.
- Install/update scripts, core updates and bundled GeoIP/GeoSite downloads use this repository's releases.

## Six-month account with a monthly allowance

Set the client's expiry six months ahead, Total Traffic to 100 GB, and Traffic Reset to Monthly. Leave the existing account-renewal interval disabled if the account must stop at its six-month expiry. Monthly traffic cycles are anchored to activation time and catch up after panel downtime.

## Installation / in-place update

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

Both `x-ui-linux-amd64.tar.gz` and `x-ui-linux-arm64.tar.gz` contain the matching patched core. The source archive includes the pinned original Xray source and TDZ patch, tests and build scripts. SHA-256 checksums accompany the release.

## Verified

- Isolated reseller API authorization and browser workflow: admin access settings, custom-path login, scoped client create/edit/bulk/delete/reset, protected inbound fields, separate sessions, live-update isolation and credential revocation.
- Subscription web-page opt-in/opt-out and live count `0/2` → `1/2` using a real authenticated stream.
- Built-in proxy migration preserves ports; failed startup restores the original service. Menu-only updates preserve the panel executable and do not invoke the installer.
- Panel unit/integration tests, shuffled race tests and `go vet`.
- Installer preservation, invalid-archive handling and failed-migration rollback.
- Client UI model round trips and empty/error IP rendering.
- Core atomic-admission, canonical IPv4/IPv6, reference-count release, live limit edits and missing/malformed policy tests.
- Real two-source-IP echo connections: VLESS TCP, TCP+MUX, WebSocket, gRPC, XHTTP, HTTPUpgrade and mKCP; VMess TCP and WebSocket+MUX; Trojan TCP; Shadowsocks TCP.

IP limits count real source addresses, not physical devices. The bundled TDZP helper publishes the socket peer without changing payload protocols. Other reverse proxies/CDNs must forward the actual client address through trusted headers or PROXY protocol. A shared public IP occupies one slot.
