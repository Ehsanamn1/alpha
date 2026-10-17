# Alpha — Network Resilience & Route Control

**Version: 0.3.0-alpha. Experimental; not a production-ready or guaranteed anti-censorship VPN.**

Alpha is a local-first controller for importing compatible proxy share links, refreshing public HTTPS / Telegram-preview sources, generating sing-box configuration, probing local routes and keeping a managed sing-box process alive. Alpha does not reimplement the VLESS/VMess/Trojan protocols; it uses the external sing-box engine for imported nodes.

## 0.3.0-alpha capabilities

- Persian RTL dashboard with desktop navigation and mobile bottom navigation for Overview, Connect, Nodes, Sources, Routes and Diagnostics.
- Import supported `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`, `tuic://`, `anytls://`, SOCKS and HTTP share links, plus Base64 subscription bodies.
- Persistent local catalog with link deduplication; raw credential-bearing URIs are not returned by the read-only node-list API or diagnostics.
- HTTPS source URLs and public Telegram previews (`t.me/s/<channel>`); source sync runs every 15 minutes while the local dashboard is active.
- Generated sing-box URL-test group for automatic node selection, configuration validation before launch, and supervised restart after an unexpected process exit.
- Local Alpha SOCKS5 entry at `127.0.0.1:1088`, route health checks, session token, CSP, same-origin mutation checks, request size limits, public-IP-pinned HTTPS source dialing and private local config files. The session-bearing root HTML is never cached by the service worker.

## Quick start

Requirements: Go 1.23+ for building, plus a compatible `sing-box` executable available in `PATH` or `engines/` next to Alpha when using imported nodes.

```bash
cp config.example.json config.json
go test -race ./...
go vet ./...
go run ./cmd/resilience engines
go run ./cmd/resilience ui --config config.json
```

On first Windows launch, `Start-Alpha.bat` creates `config.json` from `config.example.json`; review it and launch again. In the local dashboard, add raw share links or a supported HTTPS subscription / public Telegram channel, then explicitly click **Connect**. Apps must be configured to use Alpha's local SOCKS5 endpoint `127.0.0.1:1088`.

Install sing-box from its [official releases](https://github.com/SagerNet/sing-box/releases). Alpha validates its generated config with `sing-box check` before launching it. Exact compatibility can still vary by sing-box version and node-provider conventions.

## Sources and limits

- HTTPS sources must return text/Base64 subscriptions with supported share links.
- Only public Telegram channel preview pages are fetched; private invite links are rejected. Source reachability depends on the current network.
- The UI offers example sources `@thefeedconfig` and `@networkti`; these are untrusted community sources, not guarantees of working/secure configurations.
- Explicit activation is required before running imported nodes. Public nodes can observe destinations and unencrypted traffic.
- Current operational caps: 1,000 sources, 50,000 stored nodes, 8 MiB response per source. “Unlimited” cannot mean infinite resource consumption.
- Google search scraping, TheFeed's DNS-based message protocol, iodine, Spoof Tunnel, and native Android/iOS VPN services are not integrated in this release.
- Diban, Psiphon, Xray, Tor and other engines are not shipped or silently installed; Alpha can route through them only if their local endpoint / executable and configuration are set up correctly.

## Security / limitations

The dashboard binds to loopback; mutation endpoints use a per-session token and cross-origin checks. Raw import strings and generated configs are stored locally with restrictive file modes, but are still sensitive and should not be shared. Insecure-TLS flags in imported links are intentionally ignored so certificate validation is not silently disabled. An unknown node operator must not be trusted by default.

No software can create a path to the global Internet when every external route is physically unavailable. This release has not been field-tested on Iranian ISP networks, and no speed, anti-censorship or always-connected guarantee is made.

See [Architecture](docs/ARCHITECTURE.md), [platform status](docs/PLATFORM_STATUS_FA.md), [roadmap](docs/ROADMAP.md), [security policy](SECURITY.md) and the [changelog](CHANGELOG.md).
