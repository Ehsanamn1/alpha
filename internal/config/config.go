package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Listen                 string         `json:"listen"`
	ProbeURL               string         `json:"probe_url,omitempty"` // Legacy single target
	ProbeURLs              []string       `json:"probe_urls,omitempty"`
	ProbeIntervalSeconds   int            `json:"probe_interval_seconds"`
	ProbeTimeoutSeconds    int            `json:"probe_timeout_seconds"`
	EngineStartTimeoutSecs int            `json:"engine_start_timeout_seconds"`
	FailureThreshold       int            `json:"failure_threshold"`
	Engines                []EngineConfig `json:"engines"`
}

type EngineConfig struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Adapter      string            `json:"adapter"`
	Enabled      bool              `json:"enabled"`
	Priority     int               `json:"priority"`
	SOCKSAddr    string            `json:"socks_addr"`
	StartProcess bool              `json:"start_process,omitempty"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	WorkDir      string            `json:"work_dir,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err == nil {
			return Config{}, errors.New("parse config: unexpected trailing JSON value")
		}
		return Config{}, fmt.Errorf("parse config trailing data: %w", err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	listenHost, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen must be host:port: %w", err)
	}
	if !isLoopbackHost(listenHost) {
		return errors.New("listen must bind to localhost/loopback in this alpha; public/LAN exposure is not supported")
	}
	targets := c.ProbeTargets()
	if len(targets) == 0 || len(targets) > 5 {
		return errors.New("configure between 1 and 5 probe_urls (or legacy probe_url)")
	}
	for _, target := range targets {
		u, parseErr := url.ParseRequestURI(target)
		if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return fmt.Errorf("invalid probe URL %q: expected http/https URL without embedded credentials", target)
		}
	}
	if c.ProbeIntervalSeconds < 3 || c.ProbeIntervalSeconds > 3600 {
		return errors.New("probe_interval_seconds must be between 3 and 3600")
	}
	if c.ProbeTimeoutSeconds < 2 || c.ProbeTimeoutSeconds > 120 {
		return errors.New("probe_timeout_seconds must be between 2 and 120")
	}
	if c.EngineStartTimeoutSecs < 1 || c.EngineStartTimeoutSecs > 120 {
		return errors.New("engine_start_timeout_seconds must be between 1 and 120")
	}
	if c.FailureThreshold < 1 || c.FailureThreshold > 10 {
		return errors.New("failure_threshold must be between 1 and 10")
	}
	seen := map[string]bool{}
	for i, e := range c.Engines {
		if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("engine %d must have id and name", i)
		}
		if seen[e.ID] {
			return fmt.Errorf("duplicate engine id %q", e.ID)
		}
		seen[e.ID] = true
		engineHost, _, err := net.SplitHostPort(e.SOCKSAddr)
		if err != nil {
			return fmt.Errorf("engine %q socks_addr must be host:port: %w", e.ID, err)
		}
		if !isLoopbackHost(engineHost) {
			return fmt.Errorf("engine %q socks_addr must point to localhost/loopback in this alpha", e.ID)
		}
		switch e.Adapter {
		case "external-socks5", "managed-socks5":
		default:
			return fmt.Errorf("engine %q uses adapter %q, which is research-only and cannot yet be routed by the Alpha SOCKS5 core", e.ID, e.Adapter)
		}
		if e.StartProcess {
			if e.Adapter != "managed-socks5" {
				return fmt.Errorf("engine %q: automatic process launch currently requires adapter=managed-socks5; research engines need a separately reviewed wrapper", e.ID)
			}
			if strings.TrimSpace(e.Command) == "" {
				return fmt.Errorf("engine %q: command is required when start_process=true", e.ID)
			}
		}
		for k := range e.Env {
			if strings.ContainsAny(k, "=\x00") || strings.TrimSpace(k) == "" {
				return fmt.Errorf("engine %q has invalid environment variable name", e.ID)
			}
		}
	}
	return nil
}

func (c Config) ProbeInterval() time.Duration {
	return time.Duration(c.ProbeIntervalSeconds) * time.Second
}
func (c Config) ProbeTimeout() time.Duration {
	return time.Duration(c.ProbeTimeoutSeconds) * time.Second
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) ProbeTargets() []string {
	if len(c.ProbeURLs) > 0 {
		return append([]string(nil), c.ProbeURLs...)
	}
	if strings.TrimSpace(c.ProbeURL) != "" {
		return []string{c.ProbeURL}
	}
	return nil
}
