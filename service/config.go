package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	MaxConcurrency  int    `json:"max_concurrency"`
	RequestDelayMs  int    `json:"request_delay_ms"`
	HTTPTimeoutSec  int    `json:"http_timeout_sec"`
	UserAgent       string `json:"user_agent"`
	MaxSitemapDepth int    `json:"max_sitemap_depth"`
	LinkCacheTTLMin int    `json:"link_cache_ttl_min"`
}

func DefaultConfig() *Config {
	return &Config{
		MaxConcurrency:  5,
		RequestDelayMs:  100,
		HTTPTimeoutSec:  15,
		UserAgent:       "GoAuditEngine/1.0 (AuditBot; +https://example.com/bot)",
		MaxSitemapDepth: 3,
		LinkCacheTTLMin: 15,
	}
}

type Manager struct {
	mu       sync.RWMutex
	filePath string
	cfg      *Config
}

func NewManager(filePath string) *Manager {
	m := &Manager{
		filePath: filePath,
		cfg:      DefaultConfig(),
	}
	_ = m.Load()
	return m
}

func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.cfg
}

func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return m.saveInternal(m.cfg)
		}
		return err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	m.cfg = &cfg
	return nil
}

func (m *Manager) Update(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = &cfg
	return m.saveInternal(&cfg)
}

func (m *Manager) saveInternal(cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(m.filePath), 0755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, bytes, 0644)
}

func (c *Config) RequestDelay() time.Duration {
	return time.Duration(c.RequestDelayMs) * time.Millisecond
}

func (c *Config) HTTPTimeout() time.Duration {
	return time.Duration(c.HTTPTimeoutSec) * time.Second
}

func (c *Config) LinkCacheTTL() time.Duration {
	return time.Duration(c.LinkCacheTTLMin) * time.Minute
}

// MergeConfig builds the effective engine configuration for an audit run. raw
// is the JSON config stored on the audit record; base carries the fallback
// values (typically the app-level config). The stored JSON may be partial,
// malformed, or contain wrong value types: each recognized key is validated
// individually and any missing or invalid value falls back to base, so a bad
// audit config can never produce a broken run.
func MergeConfig(base Config, raw json.RawMessage) Config {
	out := base

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return out
	}

	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return out
	}

	if v, ok := obj["max_concurrency"]; ok {
		out.MaxConcurrency = mergeInt(base.MaxConcurrency, v, true)
	}
	if v, ok := obj["request_delay_ms"]; ok {
		out.RequestDelayMs = mergeInt(base.RequestDelayMs, v, false)
	}
	if v, ok := obj["http_timeout_sec"]; ok {
		out.HTTPTimeoutSec = mergeInt(base.HTTPTimeoutSec, v, true)
	}
	if v, ok := obj["user_agent"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out.UserAgent = s
		}
	}
	if v, ok := obj["max_sitemap_depth"]; ok {
		out.MaxSitemapDepth = mergeInt(base.MaxSitemapDepth, v, true)
	}
	if v, ok := obj["link_cache_ttl_min"]; ok {
		out.LinkCacheTTLMin = mergeInt(base.LinkCacheTTLMin, v, true)
	}
	return out
}

// mergeInt applies a config integer value when it is actually a JSON number
// within the allowed range; otherwise base is kept.
func mergeInt(base int, v any, positive bool) int {
	f, ok := v.(float64)
	if !ok {
		return base
	}
	n := int(f)
	if positive && n <= 0 {
		return base
	}
	if !positive && n < 0 {
		return base
	}
	return n
}
