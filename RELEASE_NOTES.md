# TDZ X-UI v2.9.3-patched.2

- Hotfix: client Telegram IDs accept empty strings, numeric strings, JSON numbers, null and omitted fields. Existing client records and client forms work without a database reset.

- Native per-client IP admission limits in the bundled Xray core, independent of access-log timing and Fail2ban.
- Live connection IPs from the Xray stats API; offline IPs are removed on refresh.
- Atomic slot reservation across simultaneous connections, shared-IP streams and MUX. Releasing the final stream immediately frees the IP slot.
- Default access log is `./access.log`; existing `/var/log/xray/access.log` defaults migrate automatically.
- Changing panel credentials invalidates existing HTTP sessions and closes authenticated WebSocket connections.
- Re-running the installer updates the application while preserving the database, SSL, credentials, panel path, port, service file and local certificates. Failed migrations restore the previous application and database backup.
- Per-client Traffic Reset: daily, weekly, monthly or a custom number of days. Only current-cycle usage resets; account expiry, quota and all-time usage are preserved.
- Install/update scripts, core updates and bundled GeoIP/GeoSite downloads use this repository's releases.

## Six-month account with a monthly allowance

Set the client's expiry six months ahead, Total Traffic to 100 GB, and Traffic Reset to Monthly. Leave the existing account-renewal interval disabled if the account must stop at its six-month expiry. Monthly traffic cycles are anchored to activation time and catch up after panel downtime.

## Installation / in-place update

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

Both `x-ui-linux-amd64.tar.gz` and `x-ui-linux-arm64.tar.gz` contain the matching patched core. The source archive includes the pinned original Xray source and TDZ patch, tests and build scripts. SHA-256 checksums accompany the release.

## Verified

- Panel unit/integration tests, shuffled race tests and `go vet`.
- Installer preservation, invalid-archive handling and failed-migration rollback.
- Client UI model round trips and empty/error IP rendering.
- Core atomic-admission, canonical IPv4/IPv6, reference-count release, live limit edits and missing/malformed policy tests.
- Real two-source-IP echo connections: VLESS TCP, TCP+MUX, WebSocket, gRPC, XHTTP, HTTPUpgrade and mKCP; VMess TCP and WebSocket+MUX; Trojan TCP; Shadowsocks TCP.

IP limits count source addresses visible to the core, not physical devices. Reverse proxies/CDNs must forward the actual client address through a correctly configured trusted transport or PROXY protocol. A shared public IP occupies one slot.
