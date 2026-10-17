package subscriptions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	alphasocks "github.com/Ehsanamn1/alpha/internal/socks5"
)

const maxSourceBytes = 8 << 20
const maxNodes = 50000

type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Host      string `json:"host"`
	Enabled   bool   `json:"enabled"`
	LastFetch string `json:"last_fetch,omitempty"`
	LastError string `json:"last_error,omitempty"`
	Imported  int    `json:"imported"`
	URL       string `json:"-"`
}

type Catalog struct {
	Sources   []Source `json:"sources"`
	Nodes     []Node   `json:"nodes"`
	UpdatedAt string   `json:"updated_at"`
}

type Store struct {
	mu                sync.RWMutex
	refreshMu         sync.Mutex
	path              string
	data              Catalog
	client            *http.Client
	fallbackProxyAddr string
}

func NewStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("find user config directory: %w", err)
	}
	return NewStoreAt(filepath.Join(base, "Alpha", "catalog.json"))
}

func NewStoreAt(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		path:              path,
		data:              Catalog{Sources: []Source{}, Nodes: []Node{}},
		client:            &http.Client{Timeout: 12 * time.Second},
		fallbackProxyAddr: "127.0.0.1:1088",
	}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("read local Alpha catalog: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if s.data.Sources == nil {
		s.data.Sources = []Source{}
	}
	if s.data.Nodes == nil {
		s.data.Nodes = []Node{}
	}
	// Migrate catalogs written before multi-source tracking was added.
	for i := range s.data.Nodes {
		n := &s.data.Nodes[i]
		if n.SourceID != "" && len(n.SourceIDs) == 0 {
			n.SourceIDs = []string{n.SourceID}
		}
		if n.SourceID == "" && len(n.SourceIDs) == 0 && n.Source != "" && n.Source != "Imported/manual" {
			n.Manual = true
		}
	}
	return s, nil
}

func normalizeSourceURL(raw string) (string, string, string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "@") {
		raw = "https://t.me/s/" + strings.TrimPrefix(raw, "@")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", "", "", errors.New("منبع باید نشانی HTTPS عمومی یا شناسهٔ کانال عمومی مثل @channel باشد")
	}
	if u.Port() != "" && u.Port() != "443" {
		return "", "", "", errors.New("فقط درگاه HTTPS استاندارد 443 پذیرفته می‌شود")
	}
	host := strings.ToLower(u.Hostname())
	if host == "t.me" || host == "telegram.me" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			return "", "", "", errors.New("شناسهٔ کانال تلگرام مشخص نیست")
		}
		channel := parts[0]
		if channel == "s" && len(parts) > 1 {
			channel = parts[1]
		}
		if channel == "joinchat" || strings.HasPrefix(channel, "+") || strings.Contains(channel, "+") {
			return "", "", "", errors.New("لینک دعوت خصوصی تلگرام پشتیبانی نمی‌شود؛ از کانال عمومی استفاده کن")
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_]{4,64}$`).MatchString(channel) {
			return "", "", "", errors.New("شناسهٔ کانال عمومی معتبر نیست")
		}
		return "https://t.me/s/" + channel, "telegram", channel, nil
	}
	if !publicHostname(host) {
		return "", "", "", errors.New("منبع به میزبان عمومی اینترنت اشاره نمی‌کند")
	}
	u.Host = host
	return u.String(), "https", host, nil
}

func publicHostname(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
	}
	return strings.Contains(host, ".") && !strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".localhost")
}

func resolvePublicIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, errors.New("private or local source addresses are rejected")
		}
		return []net.IP{ip}, nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve source host: %w", err)
	}
	if len(ips) == 0 {
		return nil, errors.New("source host did not resolve to an address")
	}
	result := make([]net.IP, 0, len(ips))
	for _, item := range ips {
		if !isPublicIP(item.IP) {
			return nil, errors.New("source host resolves to a private or local address")
		}
		result = append(result, item.IP)
	}
	return result, nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func ensurePublicDestination(ctx context.Context, host string) error {
	_, err := resolvePublicIPs(ctx, host)
	return err
}

// publicSourceDialContext pins each HTTPS source connection to an IP address
// resolved and validated at dial time. It prevents a DNS-rebinding source from
// passing validation and then resolving to loopback/private infrastructure.
func publicSourceDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("only public HTTPS on port 443 is allowed")
	}
	ips, err := resolvePublicIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, fmt.Errorf("could not connect to validated public source address: %w", lastErr)
}

func (s *Store) AddSource(name, rawURL string) (Source, error) {
	normalized, kind, host, err := normalizeSourceURL(rawURL)
	if err != nil {
		return Source{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.data.Sources {
		if existing.URL == normalized {
			return Source{}, errors.New("این منبع از قبل اضافه شده است")
		}
	}
	if len(s.data.Sources) >= 1000 {
		return Source{}, errors.New("حد عملیاتی منابع (۱۰۰۰) رسیده است؛ منابع غیرضروری را حذف کن")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		if kind == "telegram" {
			name = "Telegram · @" + host
		} else {
			name = host
		}
	}
	if len(name) > 100 {
		name = name[:100]
	}
	hash := sha256.Sum256([]byte(normalized))
	source := Source{ID: hex.EncodeToString(hash[:8]), Name: name, Kind: kind, Host: host, Enabled: true, URL: normalized}
	s.data.Sources = append(s.data.Sources, source)
	if err := s.saveLocked(); err != nil {
		s.data.Sources = s.data.Sources[:len(s.data.Sources)-1]
		return Source{}, err
	}
	return source, nil
}

func (s *Store) DeleteSource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	sources := make([]Source, 0, len(s.data.Sources))
	for _, src := range s.data.Sources {
		if src.ID == id {
			found = true
			continue
		}
		sources = append(sources, src)
	}
	if !found {
		return os.ErrNotExist
	}
	remainingNames := make(map[string]string, len(sources))
	for _, src := range sources {
		remainingNames[src.ID] = src.Name
	}
	for i := range s.data.Nodes {
		n := &s.data.Nodes[i]
		ids := sourceIDsFor(*n)
		ids = removeString(ids, id)
		n.SourceIDs = ids
		if n.SourceID == id || (n.SourceID == "" && len(ids) > 0) {
			if len(ids) > 0 {
				n.SourceID = ids[0]
				n.Source = remainingNames[ids[0]]
			} else if n.Manual {
				n.SourceID, n.Source = "", "واردسازی دستی"
			} else {
				n.SourceID, n.Source = "", "Imported/manual"
			}
		}
	}
	s.data.Sources = sources
	return s.saveLocked()
}

func (s *Store) Snapshot() Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Catalog{UpdatedAt: s.data.UpdatedAt, Sources: append([]Source{}, s.data.Sources...), Nodes: append([]Node{}, s.data.Nodes...)}
	for i := range out.Sources {
		out.Sources[i].URL = ""
	}
	for i := range out.Nodes {
		out.Nodes[i].RawURI = ""
	}
	return out
}

func (s *Store) NodesForConnect() []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Node{}, s.data.Nodes...)
}

func (s *Store) ImportText(body, sourceID, sourceName string) (int, int, error) {
	links := ParseLinks(body)
	if len(links) == 0 {
		return 0, 0, errors.New("در متن لینک اشتراک قابل پشتیبانی پیدا نشد")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	parsed := make([]Node, 0, len(links))
	skipped := 0
	for _, link := range links {
		n, _, err := ParseNode(link, sourceID, sourceName)
		if err != nil {
			skipped++
			continue
		}
		n.UpdatedAt = now
		n.Manual = sourceID == ""
		if sourceID != "" {
			n.SourceIDs = []string{sourceID}
		}
		parsed = append(parsed, n)
	}
	if len(parsed) == 0 {
		return 0, skipped, errors.New("لینک پیدا شد اما هیچ‌کدام از قالب‌های پشتیبانی‌شده معتبر نبودند")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := make(map[string]int, len(s.data.Nodes))
	for i, n := range s.data.Nodes {
		index[n.ID] = i
	}
	newCount := 0
	for _, n := range parsed {
		if i, ok := index[n.ID]; ok {
			s.data.Nodes[i] = mergeNodeSources(s.data.Nodes[i], n)
			continue
		}
		if len(s.data.Nodes) >= maxNodes {
			skipped++
			continue
		}
		index[n.ID] = len(s.data.Nodes)
		s.data.Nodes = append(s.data.Nodes, n)
		newCount++
	}
	if err := s.saveLocked(); err != nil {
		return 0, skipped, err
	}
	return newCount, skipped, nil
}

func sourceIDsFor(n Node) []string {
	ids := append([]string{}, n.SourceIDs...)
	if n.SourceID != "" {
		found := false
		for _, id := range ids {
			if id == n.SourceID {
				found = true
				break
			}
		}
		if !found {
			ids = append(ids, n.SourceID)
		}
	}
	return ids
}

func removeString(items []string, target string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != target {
			out = append(out, item)
		}
	}
	return out
}

func mergeNodeSources(existing, incoming Node) Node {
	merged := existing
	merged.UpdatedAt = incoming.UpdatedAt
	merged.RawURI = incoming.RawURI
	merged.Manual = existing.Manual || incoming.Manual
	ids := sourceIDsFor(existing)
	for _, id := range sourceIDsFor(incoming) {
		found := false
		for _, old := range ids {
			if old == id {
				found = true
				break
			}
		}
		if !found {
			ids = append(ids, id)
		}
	}
	merged.SourceIDs = ids
	if merged.SourceID == "" && incoming.SourceID != "" {
		merged.SourceID, merged.Source = incoming.SourceID, incoming.Source
	}
	if merged.Manual && merged.SourceID == "" {
		merged.Source = "واردسازی دستی"
	}
	return merged
}

// ReplaceSourceNodes refreshes one source transactionally: stale nodes lose
// that source association, nodes still supplied by other sources remain, and
// manually imported nodes are never deleted by a feed refresh.
func (s *Store) ReplaceSourceNodes(body, sourceID, sourceName string) (added, skipped, removed int, err error) {
	links := ParseLinks(body)
	if len(links) == 0 {
		return 0, 0, 0, errors.New("منبع هیچ لینک کانفیگ شناخته‌شده‌ای برنگرداند")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	parsed := make([]Node, 0, len(links))
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		n, _, parseErr := ParseNode(link, sourceID, sourceName)
		if parseErr != nil {
			skipped++
			continue
		}
		n.Manual = false
		n.SourceIDs = []string{sourceID}
		n.UpdatedAt = now
		if seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		parsed = append(parsed, n)
	}
	if len(parsed) == 0 {
		return 0, skipped, 0, errors.New("منبع لینک داشت، اما هیچ لینک سازگار و معتبری پیدا نشد")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	incomingIDs := make(map[string]bool, len(parsed))
	for _, n := range parsed {
		incomingIDs[n.ID] = true
	}
	names := make(map[string]string, len(s.data.Sources))
	for _, src := range s.data.Sources {
		names[src.ID] = src.Name
	}
	// Reconcile only when every discovered share link was understood. If a
	// provider introduces a new/unsupported link scheme, retain its old nodes
	// rather than treating partial parsing as proof that they were removed.
	reconcile := skipped == 0
	// Drop this source's claim from stale nodes, deleting only nodes which
	// were feed-owned and are no longer supplied by any other source.
	next := make([]Node, 0, len(s.data.Nodes)+len(parsed))
	index := make(map[string]int, len(s.data.Nodes)+len(parsed))
	for _, old := range s.data.Nodes {
		ids := sourceIDsFor(old)
		if reconcile && containsString(ids, sourceID) && !incomingIDs[old.ID] {
			ids = removeString(ids, sourceID)
			old.SourceIDs = ids
			if len(ids) == 0 && !old.Manual && old.SourceID != "" {
				removed++
				continue
			}
			if old.SourceID == sourceID {
				if len(ids) > 0 {
					old.SourceID = ids[0]
					old.Source = names[ids[0]]
				} else {
					old.SourceID, old.Source = "", "واردسازی دستی"
				}
			}
		}
		index[old.ID] = len(next)
		next = append(next, old)
	}
	for _, n := range parsed {
		if i, ok := index[n.ID]; ok {
			next[i] = mergeNodeSources(next[i], n)
			continue
		}
		if len(next) >= maxNodes {
			skipped++
			continue
		}
		index[n.ID] = len(next)
		next = append(next, n)
		added++
	}
	s.data.Nodes = next
	if saveErr := s.saveLocked(); saveErr != nil {
		return 0, skipped, 0, saveErr
	}
	return added, skipped, removed, nil
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (s *Store) DeleteNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, n := range s.data.Nodes {
		if n.ID == id {
			s.data.Nodes = append(s.data.Nodes[:i], s.data.Nodes[i+1:]...)
			return s.saveLocked()
		}
	}
	return os.ErrNotExist
}

func (s *Store) Refresh(ctx context.Context) (added, skipped, failures int) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.mu.RLock()
	sources := append([]Source(nil), s.data.Sources...)
	s.mu.RUnlock()
	active := make([]Source, 0, len(sources))
	for _, src := range sources {
		if src.Enabled {
			active = append(active, src)
		}
	}
	if len(active) == 0 {
		return 0, 0, 0
	}
	type result struct{ added, skipped, failures int }
	jobs := make(chan Source)
	results := make(chan result, len(active))
	workers := 4
	if len(active) < workers {
		workers = len(active)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for src := range jobs {
				if ctx.Err() != nil {
					results <- result{failures: 1}
					continue
				}
				body, err := s.fetch(ctx, src.URL)
				now := time.Now().UTC().Format(time.RFC3339)
				if err != nil {
					s.updateSource(src.ID, now, err.Error(), 0)
					results <- result{failures: 1}
					continue
				}
				if src.Kind == "telegram" {
					body = extractVisibleText(body)
				}
				a, sk, _, importErr := s.ReplaceSourceNodes(body, src.ID, src.Name)
				if importErr != nil {
					s.updateSource(src.ID, now, importErr.Error(), 0)
					results <- result{failures: 1}
					continue
				}
				s.updateSource(src.ID, now, "", a)
				results <- result{added: a, skipped: sk}
			}
		}()
	}
	go func() {
		for _, src := range active {
			jobs <- src
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for result := range results {
		added += result.added
		skipped += result.skipped
		failures += result.failures
	}
	return added, skipped, failures
}

func (s *Store) fetch(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || !publicHostname(u.Hostname()) {
		return "", errors.New("منبع HTTPS عمومی معتبر نیست")
	}
	if err := ensurePublicDestination(ctx, u.Hostname()); err != nil {
		return "", err
	}

	// Prefer direct, DNS-pinned HTTPS. Retry through Alpha's loopback SOCKS5
	// router if the current network blocks the source. The fallback pins the
	// CONNECT target to a validated public IP and keeps standard TLS checks.
	resp, directErr := s.doSourceRequest(ctx, rawURL, false)
	directProblem := ""
	if directErr != nil {
		directProblem = directErr.Error()
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		directProblem = fmt.Sprintf("منبع پاسخ HTTP %d داد", resp.StatusCode)
		_ = resp.Body.Close()
		resp = nil
	}
	if resp == nil {
		resp, err = s.doSourceRequest(ctx, rawURL, true)
		if err != nil {
			if directProblem == "" {
				directProblem = "دریافت مستقیم ناموفق بود"
			}
			return "", fmt.Errorf("دریافت مستقیم منبع ناموفق بود (%s)؛ تلاش از مسیر پراکسی محلی آلفا هم ناموفق بود: %w", directProblem, err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			status := resp.StatusCode
			_ = resp.Body.Close()
			return "", fmt.Errorf("دریافت مستقیم منبع ناموفق بود (%s)؛ مسیر جایگزین آلفا هم HTTP %d برگرداند", directProblem, status)
		}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxSourceBytes {
		return "", errors.New("حجم منبع از ۸ مگابایت بیشتر است")
	}
	return string(body), nil
}

func (s *Store) doSourceRequest(ctx context.Context, rawURL string, viaAlpha bool) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || !publicHostname(u.Hostname()) {
		return nil, errors.New("منبع HTTPS عمومی معتبر نیست")
	}
	if err := ensurePublicDestination(ctx, u.Hostname()); err != nil {
		return nil, err
	}

	transport := &http.Transport{
		DialContext:           publicSourceDialContext,
		ForceAttemptHTTP2:     !viaAlpha,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	// Preserve injected TLS roots for deterministic end-to-end tests; production
	// leaves this nil and uses the platform trust store.
	if baseTransport, ok := s.client.Transport.(*http.Transport); ok && baseTransport.TLSClientConfig != nil {
		transport.TLSClientConfig = baseTransport.TLSClientConfig.Clone()
	}
	if viaAlpha {
		proxyAddress := s.fallbackProxyAddr
		if strings.TrimSpace(proxyAddress) == "" {
			proxyAddress = "127.0.0.1:1088"
		}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialPublicSourceViaSOCKS5(ctx, proxyAddress, network, address)
		}
	}
	client := *s.client
	client.Transport = transport
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("redirect limit exceeded")
		}
		if req.URL.Scheme != "https" || !publicHostname(req.URL.Hostname()) {
			return errors.New("redirect to non-public host rejected")
		}
		return ensurePublicDestination(req.Context(), req.URL.Hostname())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Alpha-Network-Resilience/0.4 (+https://github.com/Ehsanamn1/alpha)")
	req.Header.Set("Accept", "text/html,text/plain,application/json,*/*;q=0.5")
	return client.Do(req)
}

// dialPublicSourceViaSOCKS5 tunnels a public HTTPS request through Alpha's
// local router while preserving IP pinning. It never asks a remote proxy to
// resolve arbitrary hostnames, and only permits the standard HTTPS port.
func dialPublicSourceViaSOCKS5(ctx context.Context, proxyAddr, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("unsupported source network %q", network)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil || portText != "443" {
		return nil, errors.New("only public HTTPS on port 443 is allowed")
	}
	ips, err := resolvePublicIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := (&net.Dialer{Timeout: 4 * time.Second, KeepAlive: 20 * time.Second}).DialContext(ctx, "tcp", proxyAddr)
		if dialErr != nil {
			lastErr = dialErr
			continue
		}
		deadline := time.Now().Add(8 * time.Second)
		if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
			deadline = contextDeadline
		}
		_ = conn.SetDeadline(deadline)
		if err := alphasocks.Negotiate(conn); err != nil {
			_ = conn.Close()
			lastErr = fmt.Errorf("negotiate Alpha SOCKS5: %w", err)
			continue
		}
		request, err := alphasocks.BuildConnectRequest(ip.String(), 443)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if err := writeSOCKSRequest(conn, request); err != nil {
			_ = conn.Close()
			lastErr = fmt.Errorf("write SOCKS5 connect request: %w", err)
			continue
		}
		_, replyCode, err := alphasocks.ReadReply(conn)
		if err != nil || replyCode != 0 {
			_ = conn.Close()
			if err != nil {
				lastErr = fmt.Errorf("read SOCKS5 connect reply: %w", err)
			} else {
				lastErr = fmt.Errorf("Alpha SOCKS5 refused public source IP %s (reply %d)", ip, replyCode)
			}
			continue
		}
		_ = conn.SetDeadline(time.Time{})
		return conn, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no public source address was available")
	}
	return nil, fmt.Errorf("source fetch through Alpha SOCKS5 failed: %w", lastErr)
}

func writeSOCKSRequest(conn net.Conn, request []byte) error {
	for len(request) > 0 {
		n, err := conn.Write(request)
		if n > 0 {
			request = request[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func extractVisibleText(document string) string {
	decoded := html.UnescapeString(document)
	// Some subscription pages put share URIs in anchor attributes rather than
	// visible text. Preserve only recognized protocol URLs from href values.
	hrefPattern := regexp.MustCompile(`(?is)href\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	var hrefs []string
	for _, match := range hrefPattern.FindAllStringSubmatch(decoded, -1) {
		if len(match) > 2 {
			value := match[1]
			if value == "" {
				value = match[2]
			}
			if strings.Contains(value, "://") {
				hrefs = append(hrefs, html.UnescapeString(value))
			}
		}
	}
	// Remove scripts/styles/comments before finding configuration links.
	for _, pattern := range []string{
		`(?is)<script\b[^>]*>.*?</script\s*>`,
		`(?is)<style\b[^>]*>.*?</style\s*>`,
		`(?is)<svg\b[^>]*>.*?</svg\s*>`,
		`(?is)<noscript\b[^>]*>.*?</noscript\s*>`,
	} {
		decoded = regexp.MustCompile(pattern).ReplaceAllString(decoded, " ")
	}
	tags := regexp.MustCompile(`(?s)<!--.*?-->|<[^>]*>`)
	decoded = tags.ReplaceAllString(decoded, " ")
	return strings.Join(append(strings.Fields(decoded), hrefs...), " ")
}

func (s *Store) updateSource(id, when, problem string, imported int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Sources {
		if s.data.Sources[i].ID == id {
			s.data.Sources[i].LastFetch = when
			s.data.Sources[i].LastError = problem
			s.data.Sources[i].Imported = imported
			break
		}
	}
	_ = s.saveLocked()
}

func (s *Store) saveLocked() error {
	s.data.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
