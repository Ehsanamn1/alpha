# Alpha architecture

## Current components

1. **External engine**: a separately installed process that exposes a local SOCKS5 endpoint.
2. **Health probe**: sends a CONNECT through SOCKS5, validates TLS, and checks an HTTP(S) response.
3. **Orchestrator**: tracks health, priority, latency and failure reasons.
4. **Local SOCKS5 router**: provides a fixed loopback endpoint and sends new connections through a healthy engine.
5. **Process supervisor**: launches user-configured executables only when explicitly allowed.
6. **Local UI**: embedded responsive Persian dashboard, protected local API, manual probe action and a PWA shell.

## Why a SOCKS5 integration contract?

This lets independently maintained engines be tested without blindly combining their protocol internals. Only `external-socks5` and `managed-socks5` are supported by the main router in this version. Protocols which need TUN/IP integration or provide only feeds/messages are not presented as compatible SOCKS engines.

## Failover

- Health is measured via a real HTTP(S) request through the engine.
- Healthy routes are ranked by configured priority, then latency.
- New connections try the next route when local connection/handshake fails.
- Existing TCP connections are not guaranteed to migrate after a tunnel drops.

## Security

The router and UI bind to loopback only. Dashboard POSTs require a per-process token; cross-origin requests are rejected, no CORS is enabled, and a restrictive CSP is sent. The service worker does not cache API endpoints. Alpha does not install a root CA or intercept HTTPS.

## Deliberately separate research tracks

- Diban, MasterHttpRelayVPN, Zyrln, LTSC: relay-style paths that still need access to their relay infrastructure.
- sing-box, Xray, Tor, compatible Psiphon client: usable through SOCKS5 when a local endpoint exists.
- Spoof Tunnel: needs a carefully reviewed secure adapter and an authorized two-node test; not directly SOCKS5.
- iodine: requires a DNS-tunnel/TUN or compatible proxy adapter.
- TheFeed: emergency feed and messaging, not a full-tunnel proxy.
- GoodbyeDPI/zapret: platform-specific methods for some DPI cases, not SOCKS5 engines.

The methods are not necessarily composable: some are alternatives, some can be chained, and some only support low-bandwidth messaging. Each adapter requires its own threat model and acceptance tests.
