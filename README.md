# Alpha — Network Resilience & Route Control

[![CI](https://github.com/Ehsanamn1/alpha/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Ehsanamn1/alpha/actions/workflows/ci.yml)
[![GitHub Pages](https://github.com/Ehsanamn1/alpha/actions/workflows/pages.yml/badge.svg?branch=main)](https://github.com/Ehsanamn1/alpha/actions/workflows/pages.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Website:** https://ehsanamn1.github.io/alpha/ · **Releases:** https://github.com/Ehsanamn1/alpha/releases · **Source:** https://github.com/Ehsanamn1/alpha

**Version: 0.4.2-alpha. Experimental; not a production-ready or guaranteed anti-censorship VPN.**

Alpha is a local-first controller for importing compatible proxy share links, refreshing public HTTPS / Telegram-preview sources, generating sing-box configuration, probing local routes and keeping a managed sing-box process alive. Alpha does not reimplement the VLESS/VMess/Trojan protocols; it uses the external sing-box engine for imported nodes.

## Download the alpha build

- [All release assets](https://github.com/Ehsanamn1/alpha/releases/tag/v0.4.2-alpha)
- [Full bundle (source + binaries)](https://github.com/Ehsanamn1/alpha/releases/download/v0.4.2-alpha/Alpha-0.4.2-alpha-bundle.zip)
- [Source archive](https://github.com/Ehsanamn1/alpha/releases/download/v0.4.2-alpha/Alpha-0.4.2-alpha-source.zip)
- [SHA-256 checksums](https://github.com/Ehsanamn1/alpha/releases/download/v0.4.2-alpha/SHA256SUMS.txt)

The public product/download website is published at [ehsanamn1.github.io/alpha](https://ehsanamn1.github.io/alpha/). It is a static, installable information/download portal; the controller and proxy run locally on the user's device and are not exposed as a public service.

## 0.4.2-alpha capabilities

- Persian RTL dashboard with desktop navigation and mobile bottom navigation for Overview, Connect, Nodes, Sources, Routes and Diagnostics.
- Import supported `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`, `tuic://`, `anytls://`, SOCKS and HTTP share links, plus Base64 subscription bodies.
- Persistent local catalog with link deduplication; raw credential-bearing URIs are not returned by the read-only node-list API or diagnostics.
- HTTPS source URLs and public Telegram previews (`t.me/s/<channel>`); source sync runs every 15 minutes while the local dashboard is active. If direct fetching fails, the controller retries through Alpha's local SOCKS5 router using validated public-IP pinning and normal TLS verification.
- End-to-end source-fetch recovery is tested with the official sing-box 1.14.2 binary, Alpha's actual SOCKS5 router, a TLS test source and a mapped SOCKS upstream on both Linux and Windows CI. The test verifies the fetched link remains importable; it is not a live ISP or public-node availability test.
- Generated sing-box URL-test group for automatic node selection, configuration validation before launch, and supervised restart after an unexpected process exit.
- CI validates eight representative protocol configurations against the official sing-box v1.14.2 executable after verifying its SHA-256 digest; this is config compatibility testing, not a live service uptime guarantee.
- Local Alpha SOCKS5 entry at `127.0.0.1:1088`, route health checks, session token, CSP, same-origin mutation checks, request size limits, public-IP-pinned HTTPS source dialing and private local config files. The session-bearing root HTML is never cached by the local dashboard service worker. The separate public website has an offline-capable information PWA but no access to the local proxy process.

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
- The UI offers example sources `@thefeedconfig` and `@networkti`; these are untrusted community sources, not guarantees of working/secure configurations. Source retrieval tries direct HTTPS first, then the local Alpha route if direct access fails.
- Explicit activation is required before running imported nodes. Public nodes can observe destinations and unencrypted traffic.
- Current operational caps: 1,000 sources, 50,000 stored nodes, 8 MiB response per source. “Unlimited” cannot mean infinite resource consumption.
- Google search scraping, TheFeed's DNS-based message protocol, iodine, Spoof Tunnel, and native Android/iOS VPN services are not integrated in this release.
- Diban, Psiphon, Xray, Tor and other engines are not shipped or silently installed; Alpha can route through them only if their local endpoint / executable and configuration are set up correctly.

## Security / limitations

The dashboard binds to loopback; mutation endpoints use a per-session token and cross-origin checks. Raw import strings and generated configs are stored locally with restrictive file modes, but are still sensitive and should not be shared. Insecure-TLS flags in imported links are intentionally ignored so certificate validation is not silently disabled. An unknown node operator must not be trusted by default.

No software can create a path to the global Internet when every external route is physically unavailable. This release has not been field-tested on Iranian ISP networks, and no speed, anti-censorship or always-connected guarantee is made.

See [Architecture](docs/ARCHITECTURE.md), [platform status](docs/PLATFORM_STATUS_FA.md), [roadmap](docs/ROADMAP.md), [security policy](SECURITY.md) and the [changelog](CHANGELOG.md).
