# Changelog

## 0.4.1-alpha — 2026-10-17

- Fixed standard `trojan://password@host:port` parsing; passwords without a colon are now read from the URI username component.
- Added CI verification against the official sing-box v1.14.2 executable, pinned by SHA-256.
- Added per-protocol generated-config checks for VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, AnyTLS and SOCKS5.
- Confirmed the complete CI pipeline (race tests, vet, build, browser asset validation) on the parser-fix commit before the release/version bump.

## 0.4.0-alpha — 2026-10-17

- Added safe HTTPS source-fetch fallback through Alpha's local SOCKS5 router when direct access fails; targets remain public-IP-pinned and TLS verification stays enabled.
- Added a regression test proving SOCKS5 CONNECT requests use the validated public IP rather than a remote hostname lookup.
- Fixed background catalog refresh so active sing-box config is reapplied whenever node membership changes, including removals and replacements.
- Published an installable static website/PWA shell and mobile quick-navigation; it does not expose the local proxy/controller.
- Updated CI to validate local and public browser assets and the website manifest.
- Updated versioned release workflow to package source, cross-platform builds, checksums and an all-in-one bundle.

## 0.3.0-alpha — 2026-10-17

- Added persistent local catalog of HTTPS subscription sources and public Telegram preview channels.
- Added de-duplicated import for VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, AnyTLS, SOCKS and authenticated HTTP proxy share links, including Base64 subscription bodies.
- Added generated sing-box URL-test configuration, explicit connect/disconnect controls, pre-launch config validation and a supervised restart loop.
- Added automatic source refresh every 15 minutes while Alpha's local dashboard is running. Refresh depends on source reachability from the current network.
- Added a refreshed Persian RTL app-like dashboard, mobile bottom navigation, source/node libraries and a custom Alpha SVG mark.
- Added source redirect/DNS checks with public-IP-pinned HTTPS dialing, rejection of private/local proxy endpoints, request size bounds and credential-aware API behavior.
- Clarified the unfinished state of Google search discovery, TheFeed DNS/feed protocol, iodine, Spoof Tunnel and native Android/iOS VPN.

## 0.2.0-alpha

- Initial Go SOCKS5 router, health probes, opt-in generic process supervisor, local dashboard, PWA shell and packaging workflows.
