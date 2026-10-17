package subscriptions

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Node is a parsed, imported share link. RawURI contains credentials and must
// never be exposed by the dashboard API or included in diagnostics.
type Node struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Protocol  string   `json:"protocol"`
	Server    string   `json:"server"`
	Port      int      `json:"port"`
	SourceID  string   `json:"source_id,omitempty"`
	SourceIDs []string `json:"source_ids,omitempty"`
	Source    string   `json:"source,omitempty"`
	Manual    bool     `json:"manual,omitempty"`
	UpdatedAt string   `json:"updated_at"`
	RawURI    string   `json:"-"`
}

type outbound struct {
	Type           string         `json:"type"`
	Tag            string         `json:"tag"`
	Server         string         `json:"server,omitempty"`
	ServerPort     int            `json:"server_port,omitempty"`
	UUID           string         `json:"uuid,omitempty"`
	Password       string         `json:"password,omitempty"`
	Method         string         `json:"method,omitempty"`
	AlterID        int            `json:"alter_id,omitempty"`
	Security       string         `json:"security,omitempty"`
	Flow           string         `json:"flow,omitempty"`
	TLS            map[string]any `json:"tls,omitempty"`
	Transport      map[string]any `json:"transport,omitempty"`
	Network        string         `json:"network,omitempty"`
	Username       string         `json:"username,omitempty"`
	Version        string         `json:"version,omitempty"`
	CongestionCtrl string         `json:"congestion_control,omitempty"`
	Obfs           map[string]any `json:"obfs,omitempty"`
	Plugin         string         `json:"plugin,omitempty"`
	PluginOpts     string         `json:"plugin_opts,omitempty"`
}

// ParseLinks extracts common share links from plain text or base64 subscriptions.
// It intentionally ignores arbitrary URLs and only accepts recognized proxy schemes.
func ParseLinks(body string) []string {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	body = strings.ReplaceAll(body, "&amp;", "&")
	if !hasProxyScheme(body) {
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			decoded, err := enc.DecodeString(strings.TrimSpace(body))
			if err == nil && hasProxyScheme(string(decoded)) {
				body = string(decoded)
				break
			}
		}
	}
	var links []string
	seen := map[string]bool{}
	for _, line := range strings.Fields(body) {
		line = strings.TrimSpace(line)
		// HTML pages and chat exports can place punctuation immediately after a URI.
		line = strings.TrimRight(line, ",;，。.)]}'\"`>")
		lower := strings.ToLower(line)
		if !recognizedShareLink(line, lower) {
			continue
		}
		if !seen[line] {
			seen[line] = true
			links = append(links, line)
		}
	}
	return links
}

func hasProxyScheme(s string) bool {
	for _, part := range strings.Fields(s) {
		if recognizedShareLink(part, strings.ToLower(part)) {
			return true
		}
	}
	return false
}

func recognizedShareLink(raw, lower string) bool {
	for _, scheme := range []string{"vless://", "vmess://", "trojan://", "ss://", "hysteria2://", "hy2://", "tuic://", "anytls://", "socks5://", "socks://"} {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	// Bare HTTP(S) URLs are common in Telegram posts and must never be
	// mistaken for proxy endpoints. Accept them as proxy links only when they
	// explicitly contain a username/password auth section.
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, err := url.Parse(raw)
		return err == nil && u.User != nil && u.Hostname() != "" && u.Port() != ""
	}
	return false
}

// ParseNode validates a share URL and returns its public metadata without
// exposing credentials. Only formats that Alpha can translate to sing-box
// are accepted.
func ParseNode(raw, sourceID, sourceName string) (Node, map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 32*1024 {
		return Node{}, nil, errors.New("empty or excessively long share link")
	}
	var proto, name, server string
	var port int
	var out map[string]any
	var err error
	scheme := strings.ToLower(strings.SplitN(raw, "://", 2)[0])
	switch scheme {
	case "vmess":
		proto, name, server, port, out, err = parseVMess(raw)
	case "ss":
		proto, name, server, port, out, err = parseShadowsocks(raw)
	case "vless", "trojan", "hysteria2", "hy2", "tuic", "anytls", "socks5", "socks", "http", "https":
		proto, name, server, port, out, err = parseURLProxy(raw, scheme)
	default:
		err = fmt.Errorf("unsupported share-link protocol %q", scheme)
	}
	if err != nil {
		return Node{}, nil, err
	}
	if name == "" {
		name = proto + " · " + server + ":" + strconv.Itoa(port)
	}
	if len(name) > 120 {
		name = name[:120]
	}
	sum := sha256.Sum256([]byte(raw))
	id := hex.EncodeToString(sum[:12])
	out["tag"] = "alpha-" + id
	return Node{ID: id, Name: name, Protocol: proto, Server: server, Port: port, SourceID: sourceID, Source: sourceName, RawURI: raw}, out, nil
}

func parseVMess(raw string) (string, string, string, int, map[string]any, error) {
	encoded := strings.TrimPrefix(strings.TrimPrefix(raw, "vmess://"), "VMESS://")
	data, err := decodeAnyBase64(encoded)
	if err != nil {
		return "", "", "", 0, nil, fmt.Errorf("decode VMess link: %w", err)
	}
	var v struct {
		Name string      `json:"ps"`
		Add  string      `json:"add"`
		Port json.Number `json:"port"`
		ID   string      `json:"id"`
		AID  json.Number `json:"aid"`
		Scy  string      `json:"scy"`
		Net  string      `json:"net"`
		Host string      `json:"host"`
		Path string      `json:"path"`
		TLS  string      `json:"tls"`
		SNI  string      `json:"sni"`
		ALPN string      `json:"alpn"`
		FP   string      `json:"fp"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", "", "", 0, nil, fmt.Errorf("parse VMess JSON: %w", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(v.Port.String()))
	if err != nil || port < 1 || port > 65535 || strings.TrimSpace(v.Add) == "" || strings.TrimSpace(v.ID) == "" {
		return "", "", "", 0, nil, errors.New("VMess link has invalid server, port or UUID")
	}
	security := v.Scy
	if security == "" {
		security = "auto"
	}
	out := map[string]any{"type": "vmess", "server": v.Add, "server_port": port, "uuid": v.ID, "security": security}
	if aid, e := strconv.Atoi(v.AID.String()); e == nil && aid >= 0 {
		out["alter_id"] = aid
	}
	if strings.EqualFold(v.TLS, "tls") {
		tls := tlsSettings(v.SNI, v.ALPN, v.FP, false)
		out["tls"] = tls
	}
	if tr := transportSettings(v.Net, v.Path, v.Host, ""); tr != nil {
		out["transport"] = tr
	}
	if v.Name == "" {
		v.Name = "VMess · " + v.Add
	}
	return "vmess", v.Name, v.Add, port, out, nil
}

func parseShadowsocks(raw string) (string, string, string, int, map[string]any, error) {
	withoutScheme := strings.TrimPrefix(raw, "ss://")
	withoutScheme = strings.TrimPrefix(withoutScheme, "SS://")
	withoutScheme = strings.SplitN(withoutScheme, "#", 2)[0]
	parsed, err := url.Parse("ss://" + withoutScheme)
	if err != nil {
		return "", "", "", 0, nil, err
	}
	var method, password, server string
	var port int
	if parsed.User != nil && parsed.Hostname() != "" {
		server = parsed.Hostname()
		port, err = strconv.Atoi(parsed.Port())
		user := parsed.User.String()
		if decoded, e := decodeAnyBase64(user); e == nil && strings.Contains(string(decoded), ":") {
			user = string(decoded)
		}
		method, password, _ = strings.Cut(user, ":")
	} else {
		encoded := strings.SplitN(withoutScheme, "?", 2)[0]
		decoded, e := decodeAnyBase64(encoded)
		if e != nil {
			return "", "", "", 0, nil, errors.New("unsupported legacy Shadowsocks share URL")
		}
		parsed, err = url.Parse("ss://" + string(decoded))
		if err != nil || parsed.User == nil {
			return "", "", "", 0, nil, errors.New("invalid Shadowsocks URL")
		}
		server = parsed.Hostname()
		port, err = strconv.Atoi(parsed.Port())
		method, password, _ = strings.Cut(parsed.User.String(), ":")
	}
	if err != nil || !validServerPort(server, port) || method == "" || password == "" {
		return "", "", "", 0, nil, errors.New("Shadowsocks link has invalid server credentials")
	}
	out := map[string]any{"type": "shadowsocks", "server": server, "server_port": port, "method": method, "password": password}
	if parsed.Query().Get("plugin") != "" {
		return "", "", "", 0, nil, errors.New("Shadowsocks plugin links are not supported in this build")
	}
	name := parsed.Fragment
	if name == "" {
		name = "Shadowsocks · " + server
	}
	return "shadowsocks", name, server, port, out, nil
}

func parseURLProxy(raw, scheme string) (string, string, string, int, map[string]any, error) {
	p, err := url.Parse(raw)
	if err != nil {
		return "", "", "", 0, nil, fmt.Errorf("parse share URL: %w", err)
	}
	server := p.Hostname()
	port, err := strconv.Atoi(p.Port())
	if err != nil || !validServerPort(server, port) {
		return "", "", "", 0, nil, errors.New("share link needs a valid public server and port")
	}
	q := p.Query()
	name := p.Fragment
	if decoded, e := url.QueryUnescape(name); e == nil {
		name = decoded
	}
	var out map[string]any
	proto := scheme
	switch scheme {
	case "vless":
		uuid := ""
		if p.User != nil {
			uuid = p.User.Username()
		}
		if uuid == "" {
			return "", "", "", 0, nil, errors.New("VLESS link has no UUID")
		}
		out = map[string]any{"type": "vless", "server": server, "server_port": port, "uuid": uuid}
		if flow := q.Get("flow"); flow != "" {
			out["flow"] = flow
		}
		sec := strings.ToLower(q.Get("security"))
		if sec == "tls" || sec == "reality" {
			tls := tlsSettings(firstNonEmpty(q.Get("sni"), q.Get("servername")), q.Get("alpn"), q.Get("fp"), false)
			if sec == "reality" {
				reality := map[string]any{"enabled": true}
				if pk := q.Get("pbk"); pk != "" {
					reality["public_key"] = pk
				}
				if sid := q.Get("sid"); sid != "" {
					reality["short_id"] = sid
				}
				tls["reality"] = reality
			}
			out["tls"] = tls
		}
		if tr := transportSettings(q.Get("type"), q.Get("path"), firstNonEmpty(q.Get("host"), q.Get("authority")), q.Get("serviceName")); tr != nil {
			out["transport"] = tr
		}
	case "trojan":
		password := ""
		if p.User != nil {
			password, _ = p.User.Password()
			// Most trojan:// share links encode the credential as a single
			// password before "@", not as user:password.
			if password == "" {
				password = p.User.Username()
			}
		}
		if password == "" {
			return "", "", "", 0, nil, errors.New("Trojan link has no password")
		}
		out = map[string]any{"type": "trojan", "server": server, "server_port": port, "password": password}
		out["tls"] = tlsSettings(firstNonEmpty(q.Get("sni"), q.Get("peer")), q.Get("alpn"), q.Get("fp"), false)
		if tr := transportSettings(q.Get("type"), q.Get("path"), firstNonEmpty(q.Get("host"), q.Get("host")), q.Get("serviceName")); tr != nil {
			out["transport"] = tr
		}
	case "hysteria2", "hy2":
		password := ""
		if p.User != nil {
			password, _ = p.User.Password()
			if password == "" {
				password = p.User.Username()
			}
		}
		if password == "" {
			return "", "", "", 0, nil, errors.New("Hysteria2 link has no password")
		}
		proto = "hysteria2"
		out = map[string]any{"type": "hysteria2", "server": server, "server_port": port, "password": password, "tls": tlsSettings(firstNonEmpty(q.Get("sni"), q.Get("peer")), q.Get("alpn"), q.Get("fp"), false)}
		if obfs := q.Get("obfs"); obfs != "" {
			out["obfs"] = map[string]any{"type": obfs, "password": firstNonEmpty(q.Get("obfs-password"), q.Get("obfsPassword"))}
		}
	case "tuic":
		user := ""
		pass := ""
		if p.User != nil {
			user = p.User.Username()
			pass, _ = p.User.Password()
		}
		if user == "" || pass == "" {
			return "", "", "", 0, nil, errors.New("TUIC link needs UUID and password")
		}
		proto = "tuic"
		out = map[string]any{"type": "tuic", "server": server, "server_port": port, "uuid": user, "password": pass, "congestion_control": firstNonEmpty(q.Get("congestion_control"), "bbr"), "tls": tlsSettings(firstNonEmpty(q.Get("sni"), q.Get("peer")), q.Get("alpn"), q.Get("fp"), false)}
	case "anytls":
		pass := ""
		if p.User != nil {
			pass = p.User.Username()
		}
		if pass == "" {
			return "", "", "", 0, nil, errors.New("AnyTLS link has no password")
		}
		proto = "anytls"
		out = map[string]any{"type": "anytls", "server": server, "server_port": port, "password": pass, "tls": tlsSettings(firstNonEmpty(q.Get("sni"), q.Get("peer")), q.Get("alpn"), q.Get("fp"), false)}
	case "socks5", "socks", "http", "https":
		if scheme == "https" {
			proto = "https"
			out = map[string]any{"type": "http", "server": server, "server_port": port, "tls": map[string]any{"enabled": true, "server_name": firstNonEmpty(q.Get("sni"), server)}}
		} else if scheme == "http" {
			out = map[string]any{"type": "http", "server": server, "server_port": port}
		} else {
			proto = "socks5"
			out = map[string]any{"type": "socks", "server": server, "server_port": port}
		}
		if p.User != nil {
			u := p.User.Username()
			pass, _ := p.User.Password()
			if u != "" {
				out["username"] = u
			}
			if pass != "" {
				out["password"] = pass
			}
		}
	default:
		return "", "", "", 0, nil, errors.New("unsupported protocol")
	}
	if name == "" {
		name = proto + " · " + server
	}
	return proto, name, server, port, out, nil
}

func tlsSettings(sni, alpn, fp string, insecure bool) map[string]any {
	tls := map[string]any{"enabled": true}
	if sni != "" {
		tls["server_name"] = sni
	}
	if insecure {
		tls["insecure"] = true
	}
	if alpn != "" {
		parts := strings.Split(alpn, ",")
		tls["alpn"] = parts
	}
	if fp != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	return tls
}

func transportSettings(network, path, host, service string) map[string]any {
	switch strings.ToLower(network) {
	case "ws", "websocket":
		tr := map[string]any{"type": "ws"}
		if path == "" {
			path = "/"
		}
		tr["path"] = path
		if host != "" {
			tr["headers"] = map[string]any{"Host": host}
		}
		return tr
	case "grpc":
		tr := map[string]any{"type": "grpc"}
		if service != "" {
			tr["service_name"] = service
		} else if path != "" {
			tr["service_name"] = strings.TrimPrefix(path, "/")
		}
		return tr
	case "httpupgrade":
		tr := map[string]any{"type": "httpupgrade"}
		if path != "" {
			tr["path"] = path
		}
		if host != "" {
			tr["host"] = host
		}
		return tr
	default:
		return nil
	}
}

func decodeAnyBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "//")
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("not valid base64")
}

func validServerPort(server string, port int) bool {
	server = strings.TrimSpace(strings.TrimSuffix(server, "."))
	if server == "" || strings.ContainsAny(server, " /\t\r\n") || port < 1 || port > 65535 {
		return false
	}
	if ip := net.ParseIP(server); ip != nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !ip.IsMulticast()
	}
	if strings.EqualFold(server, "localhost") || strings.HasSuffix(strings.ToLower(server), ".localhost") || strings.HasSuffix(strings.ToLower(server), ".local") || strings.HasSuffix(strings.ToLower(server), ".internal") {
		return false
	}
	if len(server) > 253 || !strings.Contains(server, ".") {
		return false
	}
	for _, label := range strings.Split(server, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
				return false
			}
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// SingBoxConfig creates a local-only SOCKS inbound and a URL-test group which
// continuously chooses among imported outbounds. The caller still must ask the
// user before launching the external sing-box process.
func SingBoxConfig(nodes []Node) ([]byte, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no supported nodes have been imported")
	}
	outbounds := make([]map[string]any, 0, len(nodes)+2)
	tags := make([]string, 0, len(nodes))
	seen := map[string]bool{}
	for _, n := range nodes {
		if seen[n.ID] {
			continue
		}
		_, ob, err := ParseNode(n.RawURI, n.SourceID, n.Source)
		if err != nil {
			continue
		}
		tag := "alpha-" + n.ID
		ob["tag"] = tag
		outbounds = append(outbounds, ob)
		tags = append(tags, tag)
		seen[n.ID] = true
	}
	if len(tags) == 0 {
		return nil, errors.New("no supported nodes could be translated to sing-box")
	}
	outbounds = append(outbounds, map[string]any{"type": "urltest", "tag": "alpha-auto", "outbounds": tags, "url": "https://www.gstatic.com/generate_204", "interval": "3m", "tolerance": 50})
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "alpha-direct"})
	config := map[string]any{
		"log":       map[string]any{"level": "warn", "timestamp": true},
		"inbounds":  []any{map[string]any{"type": "socks", "tag": "alpha-socks-in", "listen": "127.0.0.1", "listen_port": 2089}},
		"outbounds": outbounds,
		"route":     map[string]any{"final": "alpha-auto"},
	}
	return json.MarshalIndent(config, "", "  ")
}
