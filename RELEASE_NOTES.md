# TDZ X-UI v2.9.3-patched.3

## Independent reseller accounts

- Manage accounts from **Reseller** in the administrator sidebar, below Inbounds and above Xray Configs. Create, edit, enable/disable, delete, copy login URLs and inspect account activity from one page.
- Assign any number of inbounds through a searchable multi-select. Multiple resellers can share an inbound and its client list/usage.
- Enabled accounts with no assignments can log in and see **Total Inbounds: 0** and **No inbounds assigned yet.** Assignments and counts update in open dashboards.
- **Allow Inbound Editing** defaults to off. Enable it per account to allow configuration edits and inbound enable/disable for every assigned inbound. Inbound creation/deletion and panel/server administration remain admin-only.
- Shared inbound configuration edits preserve the latest clients and traffic counters. Permission and assignment changes are checked server-side and reflected in scoped live updates.

## V2 migration

Existing per-inbound reseller accounts migrate automatically, preserving their URL path, username, password hash, authentication version and activity. Their original inbound becomes an assignment and inbound editing defaults to off. Migration is idempotent: removed assignments/deleted accounts are not recreated on restart. Removing an inbound keeps its reseller accounts available.

Credentials and account enable/disable changes revoke existing HTTP/WebSocket sessions. Assignment and editing-permission changes apply without a new login. Activity is recorded by reseller account with the affected inbound ID.

## Features retained

- Native per-client source-IP admission in the bundled Xray `26.4.25`, identified as `TDZ-IPLimit/2`, including real-IP handling through the bundled payload proxy and live IP reporting.
- Client IP **Clear** and **Refresh** remain independent; older in-flight refreshes cannot repopulate a cleared details view.
- Per-client subscription IP-limit tags and hourly/12-hourly/daily/weekly/monthly/custom Traffic Reset schedules.
- Settings-preserving updates, rollback backups, Telegram-ID compatibility and CLI menu 4's X-UI Payload Proxy.

## Install or update

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh) v3
```

No argument selects the latest available architecture package. `v0`, `v1`, `v2`, `v3` and full release tags are supported. Historical V0/V1/V2 releases remain available.

## Packages and verification

Linux amd64, arm64, 386, ARMv5/v6/v7 and s390x, plus Windows amd64, include matching patched cores. `x-ui-mod-source.tar.gz` contains matching panel source, tests, pinned original core source and patches. Verify assets against `SHA256SUMS`.

Verification covers V2 database migration/restart behavior; Go race tests, vet/staticcheck; installer/client models; real browser management and live permission changes; multi/shared/zero-inbound access; scoped client/traffic operations; and platform package/version/digest checks.

IP limits count source addresses, not physical devices. Devices sharing a public IP occupy one slot.
