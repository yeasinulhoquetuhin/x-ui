# TDZ X-UI v2.9.3-patched.2

## Final UI refinements

- Remove the extra reseller-name/dashboard badge and Total Inbounds statistic from the reseller view. Administrator overview statistics stay available.
- Confirm successful inbound-login-URL copies with a toast, inline text, and a temporary check icon. Copy failures are reported.
- Show each client's Traffic Reset cycle and next reset time in client details, including the first-use state for delayed-start accounts.

## Features included

- Per-inbound reseller login with editable Path ID/credentials, server-side client scope checks, separate sessions, revocation, and admin-visible activity. No client-count cap.
- Native per-client IP admission in the bundled Xray core. First admitted IPs retain their slots; excess IPs are rejected. Live IP details refresh manually.
- Opt-in subscription-page IP-limit tags: green below capacity, red when full, orange if statistics are unavailable.
- Hourly, 12-hourly, daily, weekly, monthly, and custom-day Traffic Reset schedules. Quota, all-time usage, and fixed expiry remain unchanged.
- X-UI Payload Proxy in CLI menu 4. Installation creates/imports no proxy instances; individual proxy management remains available.
- Settings-preserving updates, rollback backups, Telegram-ID compatibility, and panel/reseller session invalidation when credentials change.

## Version layout

| Shortcut | Release | History |
| --- | --- | --- |
| `v0` | `v2.9.3-patched.0` | Original July 2026 release |
| `v1` | `v2.9.3-patched.1` | Former patched.3 packages/source |
| `v2` | `v2.9.3-patched.2` | Former patched.5 plus these final refinements |

These are the three published versions. Historical package contents are preserved and may report their original embedded build number.

## Install or update

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

Append `v0`, `v1`, or `v2` to select a version explicitly. No argument installs the latest available architecture package. An unavailable version presents the release list and lets you choose another version, install the latest, or cancel.

## Packages and source

The current release provides Linux amd64, arm64, 386, ARMv5/v6/v7, and s390x packages, plus Windows amd64. Each platform includes its matching patched core. `x-ui-mod-source.tar.gz` contains the matching panel source, original pinned core source, patcher, and tests. Verify assets against `SHA256SUMS`.

## Verification

- Go tests with race detection, vet, and static analysis.
- Installer preservation/rollback, version shortcuts, cancellation, and menu-only updates.
- Actual browser controls for reseller access, copied-URL feedback, scoped clients, Traffic Reset details, and credential revocation.
- Real authenticated connection checks for subscription counts and native IP admission, including MUX and payload-proxy cases.
- Cross-platform builds, version smoke checks, archive contents, and published download digests.

IP limits count real source addresses, not physical devices. One shared public IP occupies one slot. Remote proxies require correctly configured real-IP forwarding.
