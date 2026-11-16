# Changelog

## v2.9.3-patched.4 — 2026-11-16

### Added

- Single-port **Accept Payloads** for TCP/RAW, WS, HTTPUpgrade, gRPC and XHTTP over TCP, with the original TDZ one-header/fixed-101/unchanged-stream behavior.
- Common payload preference that survives supported transport switches and live API updates.
- Per-account **Edit Password** permission and reseller sidebar password changes, with current-password verification, session revocation and audit records.
- Administrator reseller account cards, search/status filters and an improved searchable activity log.

### Changed

- Reseller portals now use `/reseller/<path>/` independently of the admin base path. Existing Path IDs and credentials remain valid at the new URL.
- Reseller dashboards retain the standard inbound layout; the administrator TLS warning is hidden for resellers.
- Client config labels omit inbound-name prefixes in subscriptions, Telegram config links and panel copy/export/QR output.
- Main CLI entries are numbered 0–25 after removing Payload / Proxy. Legacy proxy management is available through `x-ui-proxy`.
- Bundled core marker is `TDZ-IPLimit/2+Payload/3` on pinned Xray 26.4.25.

### Fixed

- Inbound edits commit their database changes before updating the live Xray listener, avoiding lock errors during background traffic updates and preventing failed saves from changing the active listener.
- IP SSL issuance from View Current Settings no longer enters a hidden nested menu; recovery restores a previously running panel on failure/cancellation.
- Busy-port detection and progress/time limits for certificate dependencies and ACME commands.
- Native payload compatibility with the original TDZ proxy, including CONNECT's fixed 101 response, fragmented/coalesced headers and preservation of every byte after the first CRLFCRLF.

## v2.9.3-patched.3

- Independent reseller accounts with zero, multiple or shared inbound assignments.
- Administrator Reseller page, scoped activity records and optional inbound-editing permission.
- One-time migration from per-inbound reseller records, preserving account identity, password hashes and activity.
- Live assignment/permission updates, scoped client/traffic access and session revocation after credential/access changes.

## v2.9.3-patched.2

- Inbound-scoped client authentication identities with global email/accounting isolation.
- Reseller control/save/copy fixes and client reset details.
- Separate client IP Clear/Refresh actions with protection against stale responses.
- Bundled CLI payload proxy management and version-selection improvements.

## v2.9.3-patched.1

- Native source-IP admission limits with real-IP tracking through the TDZ payload proxy.
- Scheduled per-client traffic resets, subscription IP-limit display and settings-preserving installation updates.
- Client-field and Telegram-ID compatibility fixes.

## v2.9.3-patched.0

- Archived original July 2026 distribution. Historical packages retain their original embedded build versions.
