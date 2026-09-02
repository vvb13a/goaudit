package service

import (
	"encoding/json"
	"os"
	"path/filepath"
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
