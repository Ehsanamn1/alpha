package subscriptions

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVLESSAndBuildSingBoxConfig(t *testing.T) {
	uri := "vless://01234567-89ab-cdef-0123-456789abcdef@example.net:443?encryption=none&security=tls&sni=example.net&type=ws&path=%2Fedge#Primary"
	n, _, err := ParseNode(uri, "test", "Test")
	if err != nil {
		t.Fatal(err)
	}
	if n.Protocol != "vless" || n.Server != "example.net" || n.Port != 443 || n.Name != "Primary" {
		t.Fatalf("unexpected metadata: %+v", n)
	}
	cfg, err := SingBoxConfig([]Node{n})
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed["inbounds"].([]any)) != 1 {
		t.Fatal("expected SOCKS inbound")
	}
	outbounds := parsed["outbounds"].([]any)
	if len(outbounds) != 3 {
		t.Fatalf("expected node, urltest and direct outbounds; got %d", len(outbounds))
	}
	if strings.Contains(string(cfg), "01234567-89ab-cdef-0123-456789abcdef") == false {
		t.Fatal("generated config missing node credential")
	}
}

func TestParseVMessBase64(t *testing.T) {
	payload := `{"v":"2","ps":"Test VMess","add":"edge.example.net","port":"443","id":"01234567-89ab-cdef-0123-456789abcdef","aid":0,"scy":"auto","net":"ws","host":"cdn.example.net","path":"/ws","tls":"tls","sni":"edge.example.net"}`
	uri := "vmess://" + base64.StdEncoding.EncodeToString([]byte(payload))
	n, out, err := ParseNode(uri, "x", "x")
	if err != nil {
		t.Fatal(err)
	}
	if n.Protocol != "vmess" || n.Port != 443 || out["type"] != "vmess" {
		t.Fatalf("unexpected: %+v %#v", n, out)
	}
}

func TestParseTrojanAndHysteria2(t *testing.T) {
	cases := []struct{ uri, proto string }{
		{"trojan://user:p%40ss@example.net:443?sni=example.net#T", "trojan"},
		{"hysteria2://pass@example.net:443?sni=example.net&obfs=salamander&obfs-password=hide#H", "hysteria2"},
		{"tuic://01234567-89ab-cdef-0123-456789abcdef:secret@example.net:443?sni=example.net#Q", "tuic"},
	}
	for _, tc := range cases {
		t.Run(tc.proto, func(t *testing.T) {
			n, _, err := ParseNode(tc.uri, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if n.Protocol != tc.proto {
				t.Fatalf("protocol=%q", n.Protocol)
			}
		})
	}
}

func TestParseLinksPlainAndBase64Subscription(t *testing.T) {
	content := "vless://01234567-89ab-cdef-0123-456789abcdef@example.net:443?security=tls#One\n" + "trojan://secret@example.org:443#Two"
	links := ParseLinks(content)
	if len(links) != 2 {
		t.Fatalf("got %d links: %#v", len(links), links)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	if links2 := ParseLinks(encoded); len(links2) != 2 {
		t.Fatalf("base64 decoded %d links", len(links2))
	}
}

func TestStoreSourceNormalizeAndImportDeduplicate(t *testing.T) {
	store, err := NewStoreAt(t.TempDir() + "/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSource("Test", "@abc"); err == nil {
		t.Fatal("invalid short Telegram channel name should be rejected")
	}
	source, err := store.AddSource("Test", "https://example.com/sub.txt")
	if err != nil {
		t.Fatal(err)
	}
	uri := "vless://01234567-89ab-cdef-0123-456789abcdef@example.net:443?security=tls#One"
	added, skipped, err := store.ImportText(uri+"\n"+uri, source.ID, source.Name)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || skipped != 0 {
		t.Fatalf("added=%d skipped=%d", added, skipped)
	}
	if got := len(store.NodesForConnect()); got != 1 {
		t.Fatalf("expected dedupe to 1, got %d", got)
	}
}

func TestParseLinksIgnoresOrdinaryHTTPSLinks(t *testing.T) {
	text := "https://example.com/readme vless://01234567-89ab-cdef-0123-456789abcdef@example.net:443?security=tls#One https://proxy.example.net:8443"
	links := ParseLinks(text)
	if len(links) != 1 || !strings.HasPrefix(links[0], "vless://") {
		t.Fatalf("ordinary web links must not become proxy nodes: %#v", links)
	}
	authProxy := "https://user:password@proxy.example.net:8443"
	if got := ParseLinks(authProxy); len(got) != 1 || got[0] != authProxy {
		t.Fatalf("authenticated HTTPS proxy was not retained: %#v", got)
	}
}

func TestRejectsPrivateAndLocalProxyServers(t *testing.T) {
	bad := []string{
		"vless://01234567-89ab-cdef-0123-456789abcdef@127.0.0.1:443?security=tls",
		"vless://01234567-89ab-cdef-0123-456789abcdef@10.2.3.4:443?security=tls",
		"vless://01234567-89ab-cdef-0123-456789abcdef@[::1]:443?security=tls",
		"trojan://secret@router.local:443#local",
		"trojan://secret@localhost:443#local",
	}
	for _, uri := range bad {
		t.Run(uri, func(t *testing.T) {
			if _, _, err := ParseNode(uri, "", ""); err == nil {
				t.Fatalf("expected local/private server to be rejected: %s", uri)
			}
		})
	}
}

func TestExtractVisibleTextPreservesProxyURIsFromAnchors(t *testing.T) {
	doc := `<html><body><a href="vless://01234567-89ab-cdef-0123-456789abcdef@example.net:443?security=tls#One">Connect</a></body></html>`
	got := extractVisibleText(doc)
	links := ParseLinks(got)
	if len(links) != 1 || !strings.HasPrefix(links[0], "vless://") {
		t.Fatalf("anchor proxy URI not extracted: %q / %#v", got, links)
	}
}

func TestSourceRefreshReplacesStaleNodesAndKeepsSharedNodes(t *testing.T) {
	store, err := NewStoreAt(t.TempDir() + "/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	uriOne := "vless://01234567-89ab-cdef-0123-456789abcdef@one.example.net:443?security=tls#One"
	uriTwo := "vless://01234567-89ab-cdef-0123-456789abcdef@two.example.net:443?security=tls#Two"
	uriThree := "vless://01234567-89ab-cdef-0123-456789abcdef@three.example.net:443?security=tls#Three"
	if added, _, _, err := store.ReplaceSourceNodes(uriOne+"\n"+uriTwo, "source-a", "A"); err != nil || added != 2 {
		t.Fatalf("initial source import: added=%d err=%v", added, err)
	}
	if added, _, _, err := store.ReplaceSourceNodes(uriOne, "source-b", "B"); err != nil || added != 0 {
		t.Fatalf("shared import: added=%d err=%v", added, err)
	}
	added, skipped, removed, err := store.ReplaceSourceNodes(uriTwo+"\n"+uriThree, "source-a", "A")
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || skipped != 0 || removed != 0 {
		t.Fatalf("replace shared source: added=%d skipped=%d removed=%d", added, skipped, removed)
	}
	nodes := store.NodesForConnect()
	if len(nodes) != 3 {
		t.Fatalf("shared node should survive the source A refresh, got %d", len(nodes))
	}
	var shared *Node
	for i := range nodes {
		if nodes[i].RawURI == uriOne {
			shared = &nodes[i]
			break
		}
	}
	if shared == nil || !containsString(shared.SourceIDs, "source-b") || containsString(shared.SourceIDs, "source-a") {
		t.Fatalf("source membership was not reconciled: %#v", shared)
	}
}

func TestSourceRefreshDoesNotDeleteNodesOnPartialParseFailure(t *testing.T) {
	store, err := NewStoreAt(t.TempDir() + "/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	oldURI := "vless://01234567-89ab-cdef-0123-456789abcdef@old.example.net:443?security=tls#Old"
	newURI := "vless://01234567-89ab-cdef-0123-456789abcdef@new.example.net:443?security=tls#New"
	if _, _, _, err := store.ReplaceSourceNodes(oldURI, "source-a", "A"); err != nil {
		t.Fatal(err)
	}
	if _, skipped, removed, err := store.ReplaceSourceNodes(newURI+"\ntrojan://broken", "source-a", "A"); err != nil {
		t.Fatal(err)
	} else if skipped != 1 || removed != 0 {
		t.Fatalf("partial parse should not prune stale nodes: skipped=%d removed=%d", skipped, removed)
	}
	if len(store.NodesForConnect()) != 2 {
		t.Fatalf("expected both nodes retained after partial parse, got %d", len(store.NodesForConnect()))
	}
}

func TestShareLinkCannotSilentlyDisableTLSVerification(t *testing.T) {
	uri := "vless://01234567-89ab-cdef-0123-456789abcdef@example.net:443?security=tls&sni=example.net&allowInsecure=1#NoInsecure"
	_, outbound, err := ParseNode(uri, "", "")
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, ok := outbound["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected TLS config")
	}
	if tlsConfig["insecure"] == true {
		t.Fatal("imported link must not silently disable TLS verification")
	}
}
