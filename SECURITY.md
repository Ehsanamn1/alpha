# Security model and reporting

## Scope

Alpha routes traffic through separately installed local proxy engines. Its current core does not implement those engine protocols internally, does not decrypt HTTPS, and does not install a local root CA. Each external engine's security properties remain separate.

## Current controls

- Router and dashboard listeners are restricted to loopback by default.
- Dashboard POST actions require a random per-process session token, reject cross-origin requests, emit no CORS headers, and use a restrictive Content Security Policy.
- API responses and the root HTML (which contains a per-process session token) are never cached by the service worker; only static assets are cached.
- HTTPS source fetches are limited to port 443 and dial a public IP address resolved and checked at connection time; redirects are checked as well.
- Literal private/loopback/link-local proxy endpoints are rejected when importing share links, and imported `allowInsecure` flags do not disable TLS verification.
- Process launch is explicit via `--allow-process-launch`; configuration commands must be reviewed.
- Alpha never silently downloads and executes third-party proxy engines.

## Important risks

- The local SOCKS5 router in this prototype does not add user/password authentication. Do not expose it to the LAN or public Internet.
- Process supervision can execute locally configured programs. Use trusted binaries and paths only.
- Probe targets receive requests through every enabled engine. Use benign endpoints you control where possible.
- A malicious or compromised external engine can inspect/alter traffic according to its capabilities. Alpha does not make an untrusted engine trustworthy.
- Diban's local MITM CA approach is not adopted by Alpha.
- Spoof Tunnel's newer UDP-pipe design needs a separately reviewed authenticated encrypted layer before carrying user traffic.
- Unsigned builds are not authenticated by checksum alone.

## Responsible testing

Only run spoofing or network tests against systems and address ranges you own or are explicitly authorized to test. Do not scan unrelated third-party ranges.

## Reporting

Until a dedicated security contact is published, use the repository's private vulnerability reporting feature if available. Do not post secrets, active bridge information, server credentials, private keys, or sensitive destination logs in public issues.
