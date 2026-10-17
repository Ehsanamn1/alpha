# Alpha roadmap — 0.3.0-alpha

## Completed in this alpha

- [x] Go local SOCKS5 router, health monitoring and failover for new connections.
- [x] Persian RTL control dashboard with mobile navigation, static PWA shell, routes and diagnostics.
- [x] Manual share-link and Base64 subscription import with deduplication.
- [x] Parsing/generation for VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, AnyTLS, SOCKS and authenticated HTTP proxy links.
- [x] sing-box config generation with URL-test selection, config validation before launch, and child-process supervision/restart.
- [x] Local persistent catalog of HTTPS and public Telegram preview sources with periodic refresh and bounded downloads.
- [x] Local-only API, session token, CSP, same-origin checks, request limits and reduced credential exposure.
- [x] CI and cross-compilation/release workflows for Windows, Linux and macOS.

## Phase 1 — Real connectivity and safety

- [ ] End-to-end acceptance test with an official sing-box binary and real configs on Windows/Linux/macOS.
- [ ] Verify each supported protocol variant against a pinned sing-box release; share-link conventions differ by provider.
- [ ] Import validated Clash YAML and sing-box JSON in addition to links/Base64.
- [ ] Refresh subscription sources through a working proxy when direct Telegram access is blocked.
- [ ] Track a node across multiple sources and expire stale nodes after successful updates.
- [ ] Add backoff, circuit breaking and real-network reconnection tests.
- [ ] Independent security review and deeper SSRF/DNS/source threat review.

## Phase 2 — Distinct sources and transports

- [ ] Integrate and test TheFeed's actual DNS/feed protocol; `@thefeedconfig` is only a public config channel, not TheFeed protocol integration.
- [ ] Evaluate permitted APIs for discovering public sources; Google search scraping is not enabled.
- [ ] Review and test Spoof Tunnel with an authenticated TUN/IP adapter and two controlled nodes.
- [ ] Evaluate iodine only with an appropriate TUN/IP design and threat model; DNS tunneling alone is not a full encrypted VPN.
- [ ] Dedicated lifecycle integration for Psiphon, Tor bridges/transports, Xray and other external engines.
- [ ] Review license/security/operation of Diban, MasterHttpRelayVPN, Zyrln and LTSC before describing them as supported engines.

## Phase 3 — System apps

- [ ] Native desktop installer/uninstaller, optional autostart and secure updater.
- [ ] Android `VpnService` / TUN app.
- [ ] iOS `NetworkExtension` app and required entitlements/distribution.
- [ ] Signed artifacts, SBOM and auditable release chain.

## Stable release gates

- [ ] Public-repository CI and distributable release verified green.
- [ ] Real-device/network field testing with user consent and data minimization.
- [ ] Independent security review and published threat model.
- [ ] Documented limitations, artifact checksums and signatures.

This roadmap distinguishes implemented code from unverified work. Software alone cannot create a route to the global Internet when every external route is unavailable.
