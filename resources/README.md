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
