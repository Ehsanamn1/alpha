package subscriptions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDefaultSourcesPersistAndSeedOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	store, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	added, err := store.EnsureDefaultSources()
	if err != nil {
		t.Fatal(err)
	}
	if added != 4 {
		t.Fatalf("expected 4 built-in feeds, got %d", added)
	}
	if again, err := store.EnsureDefaultSources(); err != nil || again != 0 {
		t.Fatalf("second seed should be no-op: added=%d err=%v", again, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"https://t.me/s/v2ray_free_conf",
		"https://t.me/s/configv2rayng",
		"https://t.me/s/VIProxys",
		"https://t.me/s/Vless_config",
	} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("local catalog did not persist %q", want)
		}
	}
	for _, source := range store.Snapshot().Sources {
		if source.URL != "" {
			t.Fatal("Snapshot must redact source URLs")
		}
	}
	reopened, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.data.Sources) != 4 {
		t.Fatalf("sources lost after restart: %d", len(reopened.data.Sources))
	}
	for _, source := range reopened.data.Sources {
		if source.URL == "" {
			t.Fatalf("URL not persisted: %+v", source)
		}
	}
	if err := reopened.DeleteSource(reopened.data.Sources[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.EnsureDefaultSources(); err != nil {
		t.Fatal(err)
	}
	if got := len(reopened.Snapshot().Sources); got != 3 {
		t.Fatalf("intentionally deleted feed reappeared: %d", got)
	}
}

func TestLegacyTelegramSourceURLCanBeMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	legacy := "{\"sources\":[{\"id\":\"abc\",\"name\":\"old Telegram\",\"kind\":\"telegram\",\"host\":\"networkti\",\"enabled\":true}],\"nodes\":[]} "
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.data.Sources[0].URL; got != "https://t.me/s/networkti" {
		t.Fatalf("migration returned %q", got)
	}
}
