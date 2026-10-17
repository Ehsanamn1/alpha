# Changelog

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
