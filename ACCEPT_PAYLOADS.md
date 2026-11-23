# Accept Payloads

**Accept Payloads** uses the original bundled TDZ payload proxy's handshake on the inbound's existing TCP listener. It is available for TCP/RAW, WebSocket, HTTPUpgrade, gRPC and XHTTP over TCP (HTTP/1.1 or HTTP/2). The switch is above Proxy Protocol, defaults to off and survives switching supported transports and saving/reopening the inbound.

With the switch enabled, a native binary/TLS, HTTP/2 or matching HTTP transport handshake is detected and replayed intact. An injected opaque preface follows the original `resources/payload-proxy.sh` behavior:

1. Read opaque bytes until the first `\r\n\r\n` terminator. Request method, Host, path, CONNECT target and WebSocket headers are not interpreted or validated.
2. Remove only that first header and send exactly:

   ```text
    HTTP/1.1 101 <b><font color="red" size="7">Script By:</font> <font color="#0057B7" size="7">tuhinbro.com</font></b>\r\n
   Upgrade: websocket\r\n
   Connection: Upgrade\r\n
   \r\n
   ```

3. Pass every remaining byte to the configured inbound transport, including bytes received in the same read as the header terminator. Subsequent headers, WebSocket frames and TLS records are unchanged.

CONNECT receives the same 101 response as GET. The handler does not generate `Sec-WebSocket-Accept`, decode frames, remove a second payload, or wait for tunnel data after replying. Header reads use the legacy 4096-byte chunks, 65536-byte pre-read size check and eight-second idle timeout per read.

**For payload-aware clients**, send the initial payload before the configured transport handshake. For TLS, the order is payload → 101 → TLS → transport/protocol. For WS, the order is payload → 101 → native WS handshake → WS frames. The client must use the inbound's configured transport and authentication. Matching native handshakes can use the same enabled listener without an extra preface.

The payload terminator must be actual CRLFCRLF bytes; client macros such as `[crlf]` and `[split]` must be expanded by the client app. UDP-based KCP, Hysteria and QUIC/HTTP3 do not use this TCP preface. HTTP Obfuscation is mutually exclusive. If Proxy Protocol is enabled, its required header precedes the TDZ payload.

No additional TCP backend port is allocated. The original socket/source address is retained for ordinary IP limits; V5 native DNSTT is exempt from IP admission/statistics. The JSON setting is `streamSettings.acceptPayloads`; the earlier `tcpSettings.injectPayload` remains a fallback. These server-only options are omitted from client JSON. The V5 core has the `TDZ-IPLimit/2+Payload/3+Auto/1+SlowDNS/1` marker, including support for live API updates.

Legacy standalone instances remain available through `x-ui-proxy`. The main CLI has options 0–26, with SlowDNS at 23 and BBR/Geo Files/Speedtest at 24/25/26.
