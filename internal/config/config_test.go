package config

import (
	"os"
	"testing"
)

func validConfig() Config {
	return Config{
		Listen:                 "127.0.0.1:1088",
		ProbeURL:               "https://example.com/",
		ProbeIntervalSeconds:   15,
		ProbeTimeoutSeconds:    8,
		EngineStartTimeoutSecs: 10,
		FailureThreshold:       2,
		Engines:                []EngineConfig{{ID: "one", Name: "local", Adapter: "external-socks5", Enabled: true, SOCKSAddr: "127.0.0.1:1080"}},
	}
}

func TestValidateAcceptsLoopbackConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateRejectsPublicRouter(t *testing.T) {
	c := validConfig()
	c.Listen = "0.0.0.0:1088"
	if err := c.Validate(); err == nil {
		t.Fatal("expected non-loopback listener to be rejected")
	}
}

func TestValidateRejectsDuplicateEngineIDs(t *testing.T) {
	c := validConfig()
	c.Engines = append(c.Engines, c.Engines[0])
	if err := c.Validate(); err == nil {
		t.Fatal("expected duplicate engine id to be rejected")
	}
}

func TestValidateRequiresCommandOptInForManagedProcess(t *testing.T) {
	c := validConfig()
	c.Engines[0].Adapter = "managed-socks5"
	c.Engines[0].StartProcess = true
	if err := c.Validate(); err == nil {
		t.Fatal("expected configured process to require command")
	}
	c.Engines[0].Command = "/usr/local/bin/socks-engine"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected process config to validate, got %v", err)
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(`{"listen":"127.0.0.1:1088"} {"unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
}

func TestValidateRejectsResearchOnlyAdapters(t *testing.T) {
	c := validConfig()
	c.Engines[0].Adapter = "spoof-tunnel"
	if err := c.Validate(); err == nil {
		t.Fatal("research-only adapter must not be presented as a routable SOCKS5 engine")
	}
}
