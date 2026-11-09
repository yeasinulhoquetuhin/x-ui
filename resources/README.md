# Release resources

All panel installation/update links point to `yeasinulhoquetuhin/x-ui`.
The panel's Go module namespace is retained for source compatibility; imports
within this module resolve to the local source tree, not a downloaded panel.

Release assets include:

- `x-ui-linux-amd64.tar.gz` and `x-ui-linux-arm64.tar.gz`
- `x-ui-mod-source.tar.gz` (panel source, patches, tests and core source archive)
- `xray-core-v26.4.25-source.tar.gz` (pinned source, original LICENSE retained)
- The six bundled GeoIP/GeoSite datasets and `SHA256SUMS`

The pinned core revision is `b4f08981becb71eaa995fa98ed2098ade92566bb`.
`build_xray.sh` prefers the source archive shipped in `third_party/` and applies
the included TDZ admission patch before compilation. It can also use an explicit
`TDZ_XRAY_SOURCE` checkout at this exact revision.

Dataset provenance:

| Assets | Original project |
| --- | --- |
| geoip.dat / geosite.dat | Loyalsoldier/v2ray-rules-dat |
| geoip_IR.dat / geosite_IR.dat | chocolate4u/Iran-v2ray-rules |
| geoip_RU.dat / geosite_RU.dat | runetfreedom/russia-v2ray-rules-dat |

Keep original license and attribution files when redistributing these resources.

`payload-proxy.sh` is the built-in **X-UI Payload Proxy** manager. The installer
runs `setup-payload-proxy.py` to install `/usr/local/bin/x-ui-proxy` and safely
migrate existing instances. Access it with **x-ui → 4. Payload / Proxy**.

`tdzp-client-ip.py` remains a compatibility helper for the legacy 1.2 relay with a private
per-socket real-IP channel for `TDZ-IPLimit/2`. It preserves payload handshakes
and every relayed byte. The panel installer applies it to an installed TDZP
script, saves a rollback copy, and restarts active proxy instances. Live records
are stored under `/run/tdzp-client-ips` (0700), checked against `/proc/PID/fd/FD`,
and removed when the backend socket closes. No client configuration change is
required. Use the same `TDZ_XRAY_PROXY_IP_DIR` for both processes when overriding
the default path.
