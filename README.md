<p align="center">
  <img src="https://github.com/yeasinulhoquetuhin/x-ui/raw/master/media/3x-ui-light.png" alt="3X-UI Logo" width="200"/>
</p>

<h1 align="center">TDZ X-UI v2.9.3 <sup>Patched • Beta</sup></h1>

<p align="center">
  <a href="https://github.com/yeasinulhoquetuhin/x-ui/releases/tag/v2.9.3-patched">
    <img src="https://img.shields.io/github/v/release/yeasinulhoquetuhin/x-ui?color=blue&label=release&style=for-the-badge" alt="Release">
  </a>
  <a href="https://github.com/yeasinulhoquetuhin/x-ui/blob/master/LICENSE">
    <img src="https://img.shields.io/github/license/yeasinulhoquetuhin/x-ui?color=green&style=for-the-badge" alt="License">
  </a>
  <a href="https://github.com/yeasinulhoquetuhin/x-ui/commits/master">
    <img src="https://img.shields.io/github/last-commit/yeasinulhoquetuhin/x-ui?style=for-the-badge" alt="Last Commit">
  </a>
  <img src="https://img.shields.io/badge/arch-6_supported-orange?style=for-the-badge" alt="Architectures">
</p>

---

## Quick Install

```bash
bash <(curl -Ls https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh)
```

The installer auto-detects your architecture and handles IP limit config, SSL skip option, fail2ban setup, and more.

---

## Architecture Support

| Architecture | Binary |
|---|---|
| x86_64 / amd64 | [Download](https://github.com/yeasinulhoquetuhin/x-ui/releases/download/v2.9.3-patched/x-ui-linux-amd64.tar.gz) |
| ARM64 / aarch64 | [Download](https://github.com/yeasinulhoquetuhin/x-ui/releases/download/v2.9.3-patched/x-ui-linux-arm64.tar.gz) |
| ARMv7 | [Download](https://github.com/yeasinulhoquetuhin/x-ui/releases/download/v2.9.3-patched/x-ui-linux-armv7.tar.gz) |
| ARMv6 | [Download](https://github.com/yeasinulhoquetuhin/x-ui/releases/download/v2.9.3-patched/x-ui-linux-armv6.tar.gz) |
| ARMv5 | [Download](https://github.com/yeasinulhoquetuhin/x-ui/releases/download/v2.9.3-patched/x-ui-linux-armv5.tar.gz) |
| IBM S390x | [Download](https://github.com/yeasinulhoquetuhin/x-ui/releases/download/v2.9.3-patched/x-ui-linux-s390x.tar.gz) |

> No compilation needed — every release ships pre-built binaries for all 6 architectures.

---

## Features

| # | Feature | Status |
|---|---|---|
| 1 | IP Limit via access log — auto-configured on install/update | Done |
| 2 | SSL Skip option (option 4) during install & update | Done |
| 3 | Default theme changed from black to light | Done |
| 4 | Fail2ban auto-setup with 3x-ipl jail | Done |
| 5 | Telegram Bot — full support, no webhook errors | Done |
| 6 | SSL works with domain & IP certificates — Let's Encrypt auto-renew | Done |
| 7 | WebBasePath preserved during updates — no unwanted changes | Done |
| 8 | Rate-limit safe downloads — SHA-based URLs, no GitHub API dependency | Done |
| 9 | Pre-built for all architectures — zero compilation needed | Done |
| 10 | 0/1 IP count flicker fixed — historical IPs retained between scans | Done |

---

## IP Limit

The IP limit feature uses a persistent access log that:
1. Accumulates client IPs across scan intervals
2. Merges historical IPs with a 30-minute stale cutoff — no flickering
3. Falls back gracefully when gRPC GetOnlineUsers is unavailable

No manual config needed — the installer sets everything up automatically.

---

## Changelog (v2.9.3-patched)

- IP Limit — Fully functional with access log enabled; persistent log prevents 0/1 flicker
- SSL Skip — Bypass SSL setup during install or update with option 4
- Light Theme — Default panel theme switched to light mode
- Fail2ban — Automatically configured with optimized 3x-ipl jail rules
- Telegram Bot — Stable webhook integration, zero errors
- SSL Auto-Renew — Let's Encrypt certificates renew for both domain and IP-based setups
- WebBasePath — Unchanged during updates, preserving your custom path
- Rate-Limiting — Downloads use SHA-based asset URLs, avoiding GitHub API rate limits
- Multi-Arch — 6 architectures pre-built, no server-side compilation required
- IP Flicker Fix — Historical IPs are retained between scans, eliminating 0/1 count oscillation

---

## Credits

```
Thanks to: Yeasinul Hoque Tuhin
```

Project page: [https://tuhinbro.com/project/x-ui-mod](https://tuhinbro.com/project/x-ui-mod)

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
