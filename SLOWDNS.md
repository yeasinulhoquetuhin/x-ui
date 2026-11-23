# TDZ X-UI SlowDNS

V5 adds a stock-DNSTT-compatible encrypted DNS tunnel directly to the bundled Xray core. DNS streams enter the selected inbound's transport framing and protocol authentication without a localhost TCP relay.

## First setup

Run `x-ui`, select **23. SlowDNS Management**, then **1. Install / Set up**.

| Input | Example | Purpose |
| --- | --- | --- |
| Host domain / NS Host | `ns.example.com` | Nameserver hostname; its A/AAAA record points to the VPS |
| NS domain / tunnel zone | `xui.example.com` | Zone delegated to the nameserver |
| Internal UDP listener | `[::]:5301` | Linux default; public clients use UDP 53 |
| MTU | `1232` | Press Enter for the initial upstream default, or enter 512–4096 |

Create these DNS records:

```text
A     ns.example.com   -> VPS IPv4
AAAA  ns.example.com   -> VPS IPv6 (when the listener/server supports IPv6)
NS    xui.example.com  -> ns.example.com
```

Keep the nameserver host outside the delegated tunnel zone. Use a reasonably short tunnel domain so DNS names have enough room for the encoded data.

The Linux management menu installs `iptables`, `nftables` and `util-linux` when needed. The core and DNSTT implementation are already bundled; runtime setup requires no Go compiler or separate DNSTT server download.

## Select an inbound

1. Configure **VLESS, VMess, Trojan or Shadowsocks** on TCP/RAW, WebSocket, HTTPUpgrade, gRPC or TCP-based XHTTP.
2. Set transport security to **none / NTLS**.
3. Enable the inbound's **SlowDNS** switch. It defaults off and is hidden before setup or with TLS/REALITY.
4. Open a client's **More Information** and copy **NameServer** and **Public Key** from the SlowDNS card at the bottom. Click a value to copy it.

Every enabled inbound uses the same client NS domain, **`xui.example.com`**, and the same public key. Transport framing and protocol credentials select the handler; clients do not need an `i<ID>.` prefix. Use distinct protocol credentials across DNS-enabled inbounds when they need independent account routing.

TLS/REALITY, Vision flows, mKCP, Hysteria, native UDP listeners and QUIC/HTTP3 are not SlowDNS modes. DNS carries reliable TCP streams; UDP carried inside a supported proxy protocol depends on the client and its encapsulation.

## Client example

Use a DNSTT-compatible client and the matching Xray protocol/transport. The standalone upstream client can create a local TCP endpoint:

```bash
dnstt-client -udp 8.8.8.8:53 -pubkey PUBLIC_KEY xui.example.com 127.0.0.1:7000
```

Google (`8.8.8.8:53`, `8.8.4.4:53`) and Cloudflare (`1.1.1.1:53`, `1.0.0.1:53`) resolvers are accepted. Compatible upstream DNSTT clients also support DoH/DoT resolvers.

Point the Xray client profile to the local endpoint `127.0.0.1:7000`, retaining its UUID/password and the selected transport's path/service-name/Host configuration. Apps with built-in SlowDNS can use the displayed DNS fields directly. Standard URI/JSON/Clash subscriptions do not automatically install or start a DNSTT frontend.

The resolver forwards authoritative queries to the VPS on **UDP 53**. A successful direct test against the VPS does not replace the public NS/A/AAAA delegation needed when using a recursive resolver.

## Subscription details

In the client editor, enable **Show SlowDNS on Subscription Page**. It defaults off independently for every client.

Below the client configs, the subscription page displays:

- **NameServer:** the shared tunnel domain.
- **Public Key:** the server's public DNSTT key.

Click either value to copy it. Resolver addresses and MTU are not displayed in the client card or subscription page.

The same opted-in hints are available at `SUBSCRIPTION_URL?format=slowdns`. Hidden/disabled clients and uninstalled SlowDNS produce no hints. The private key is never public subscription metadata.

## IP-limit behavior

An authoritative DNS server sees the recursive resolver, not the device's original public IP. Counting that resolver would merge unrelated users; counting `127.0.0.1` would merge localhost relays and cause limited accounts to fail.

**Native DNSTT is exempt from IP limits.** DNS tunnels never consume IP slots and never appear in IP Log or subscription IP counts. A configured IP limit of **1** still permits multiple authenticated DNSTT tunnels. Ordinary traffic to the same inbound continues to use its real source IP: one IP works, another is blocked when the limit is full, and DNS traffic does not alter that count.

Apps such as Dark Tunnel can open many independent DNSTT IDs during one VPN connection. Those IDs are not reliable device identities, so counting them would incorrectly block a single device's own traffic. The exemption uses the core's active native-DNSTT registry, not an inbound flag or a private-IP prefix. Protocol authentication, traffic accounting and account-disable/removal cancellation still apply.

A two-second server keepalive and 15-second DNS-activity lease remove abandoned tunnel resources and cancel their streams. Healthy idle tunnels continue polling. Resource cleanup does not reserve an IP-limit slot or block a new DNS connection.

## Keys and reconfiguration

**Edit / Reconfigure** changes host/NS domains, listener or MTU and requests an Xray restart. Domain/MTU edits keep the public key; clients must update their tunnel domain if it changes.

SlowDNS uninstall disables inbound opt-ins and removes only X-UI's domain-routing rules. Its database keypair is retained for reinstall. **Regenerate / Revoke public key** explicitly replaces it after confirmation. Save a panel database backup before replacing the complete database or reinstalling the entire panel.

## TDZ SSH SlowDNS coexistence

Use separate sibling tunnel zones, for example:

```text
SSH:  ssh.example.com -> ns.example.com
X-UI: xui.example.com -> ns.example.com
```

The nameserver host/VPS can be shared. X-UI detects the saved TDZ SSH zone and rejects overlapping zones during setup.

TDZ SSH normally listens on **54/UDP**, with public **53/UDP** redirected to it. X-UI uses **5301/UDP** internally and selects only its own DNS zone through an earlier-priority nft NAT hook. TDZ SSH's default redirect can be started, stopped or reinstalled in either order. Its services, key files, SSH port and domain remain independently managed.

Direct UDP-53 listening is available through an explicit listener choice where appropriate. Managed shared-port routing on Linux requires NET_ADMIN and the installed networking dependencies; Docker Compose grants NET_ADMIN for this component.

## Resource use

- One shared UDP listener for all enabled inbounds; no external DNSTT server daemon.
- A 100ms server KCP update interval instead of frequent 10ms per-session wakeups.
- Bounded 64 KiB per-stream and 512 KiB per-tunnel receive buffers.
- Event-driven queues and delayed empty DNS replies rather than busy polling.
- Idle timeout and stream/session cleanup; no active SlowDNS listener before an inbound opts in.

Resource use depends on traffic and concurrency. The coexistence smoke check measured the complete native-DNS Xray process around 29 MiB RSS and 0.2% CPU under low traffic.

## Command-line management

```bash
x-ui slowdns
x-ui slowdns status
x-ui slowdns status --details
x-ui slowdns install --host-domain ns.example.com --ns-domain xui.example.com --mtu 1232
x-ui slowdns configure --ns-domain dns.example.com --mtu 512
x-ui slowdns stop
x-ui slowdns start
x-ui slowdns uninstall
x-ui slowdns regenerate-key --yes
x-ui restart-xray
```

The interactive menu requests the restart automatically. Option **23 → 3** opens a dedicated details page and stays there until **0**, which returns to the main menu. Direct binary-style commands save configuration and report the restart requirement.

## References

- [Official DNSTT setup/protocol](https://www.bamsoftware.com/software/dnstt/)
- [Project X transport reference](https://xtls.github.io/en/config/transport.html)
- [Official 3X-UI base source](https://github.com/MHSanaei/3x-ui/tree/v2.9.3)
