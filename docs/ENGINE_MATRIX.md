# Alpha engine integration matrix — 0.3.0-alpha

| Engine/project | Actual status | Integration approach | Important limitation |
|---|---|---|---|
| Imported VLESS / VMess / Trojan / Shadowsocks / Hysteria2 / TUIC / AnyTLS | Parser + sing-box config generation | User supplies share links or a text/Base64 subscription; sing-box executes | Not validated against every provider variant or a live endpoint yet |
| SOCKS5-compatible local proxy | Implemented | External local endpoint monitored by Alpha | The local endpoint must independently reach the target |
| Diban | External endpoint template | Configure its local SOCKS5 endpoint if the installed client exposes one | Diban client is not bundled or automatically installed |
| sing-box | Managed process hook for imported nodes | Generated local SOCKS inbound + URL-test group | Official binary/config validation has not been exercised in this build environment |
| Xray-core | Example endpoint/config template | External process with compatible local SOCKS endpoint | Not bundled; config and binary are user-provided |
| Tor | Example profile and torrc template | Tor process exposes SOCKS5 `127.0.0.1:9050` | Bridge/transports may be required under blocking; not guaranteed |
| Psiphon | External endpoint only | Use client build that exposes a local proxy | No dedicated lifecycle adapter for a particular build |
| MasterHttpRelayVPN / Zyrln / LTSC | Not merged as full engines | External endpoint if a compatible local interface exists | License, security, protocol and endpoint need review |
| TheFeed public config channel | Optional public Telegram preview source | `@thefeedconfig` text is read when reachable | Not integration of TheFeed's DNS/feed messaging protocol |
| TheFeed DNS/feed protocol | Not integrated | Requires actual client/server protocol and acceptance test | Messaging/feed is not automatically a full-device tunnel |
| iodine | Research only | Needs DNS transport plus appropriate TUN/IP adapter | DNS tunnel alone is not a full encrypted VPN |
| Spoof Tunnel | Research only | Needs a separately secured/authenticated TUN/IP adapter | Not a SOCKS5 engine; requires controlled two-node testing |
| Google search | Not integrated | Future permitted search/API evaluation | No automated crawling or Google Search API in this version |
| Snowflake / WebTunnel / obfs4 | Through Tor only if independently configured | Tor transport configuration | Needs reachable bridge/transport infrastructure |
| GoodbyeDPI / zapret | Not integrated in SOCKS router | Separate platform-level module would be required | Only addresses selected DPI cases |
| Ceno | Separate browser project | Retrieve/share selected content | Does not guarantee global Internet during complete shutdown |

Alpha deliberately does not label research-only or untested projects as a working engine. A listed external endpoint profile is not the same as bundling or validating that project's client.
