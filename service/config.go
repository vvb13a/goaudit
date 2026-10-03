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

	// LinkCacheTTLMin is the lifetime of a stored link validation result: a
	// target is only re-fetched once its stored result is older than this.
	// Defaults to a day so validation does not spam the internet.
	LinkCacheTTLMin int `json:"link_cache_ttl_min"`

	// Asset pacing. Static assets (stylesheets, scripts, images, fonts,
	// media, data files) are normally served by a CDN or a static web server
	// that can absorb far more parallel requests than an application server.
	// During link validation these targets use their own delay and
	// concurrency instead of the document-oriented RequestDelayMs and
	// MaxConcurrency, so validation of a large asset graph is not throttled to
	// document speed. Document-like targets (html and unknown/extensionless
	// URLs) keep the document settings.
	AssetRequestDelayMs int `json:"asset_request_delay_ms"`
	AssetMaxConcurrency int `json:"asset_max_concurrency"`

	// Workflow toggles. A run performs the enabled workflows; at least one
	// must be on. Checks and the link graph are independent.
	EnableChecks bool `json:"enable_checks"`
	EnableGraph  bool `json:"enable_graph"`

	// EnableLinkValidation turns on the bulk validation of the graph's target
	// nodes (assets, external and internal link targets) after a run. It is
	// only useful with the graph workflow and is off by default to avoid
	// unnecessary outbound requests.
	EnableLinkValidation bool `json:"enable_link_validation"`

	// Notification delivery. The webhook is app-level; the trigger toggles
	// decide whether a finished run is announced for direct (interface) runs
	// and/or scheduled runs.
	NotificationsEnabled bool   `json:"notifications_enabled"`
	SlackWebhookURL      string `json:"slack_webhook_url"`
	NotifyOnDirect       bool   `json:"notify_on_direct"`
	NotifyOnSchedule     bool   `json:"notify_on_schedule"`
}

func DefaultConfig() *Config {
	return &Config{
		MaxConcurrency:  5,
		RequestDelayMs:  100,
		HTTPTimeoutSec:  15,
		UserAgent:       "GoAuditEngine/1.0 (AuditBot; +https://example.com/bot)",
		MaxSitemapDepth: 3,
		LinkCacheTTLMin: 1440,

		AssetRequestDelayMs: 0,
		AssetMaxConcurrency: 20,

		EnableChecks:         true,
		EnableGraph:          false,
		EnableLinkValidation: false,

		NotificationsEnabled: false,
		SlackWebhookURL:      "",
		NotifyOnDirect:       true,
		NotifyOnSchedule:     true,
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
	// These trigger toggles are new: configs written before they existed lack
	// the keys, and the zero value (false) would silently disable them.
	// Default them to on when absent.
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err == nil {
		if _, ok := keys["notify_on_direct"]; !ok {
			cfg.NotifyOnDirect = true
		}
		if _, ok := keys["notify_on_schedule"]; !ok {
			cfg.NotifyOnSchedule = true
		}
		if _, ok := keys["enable_checks"]; !ok {
			cfg.EnableChecks = true
		}
		if _, ok := keys["asset_request_delay_ms"]; !ok {
			cfg.AssetRequestDelayMs = DefaultConfig().AssetRequestDelayMs
		}
		if _, ok := keys["asset_max_concurrency"]; !ok {
			cfg.AssetMaxConcurrency = DefaultConfig().AssetMaxConcurrency
		}
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

func (c *Config) AssetRequestDelay() time.Duration {
	return time.Duration(c.AssetRequestDelayMs) * time.Millisecond
}

// RequestDelayFor returns the delay to apply before fetching a target of the
// given filetype: assets use the asset setting, everything else the document
// setting.
func (c *Config) RequestDelayFor(filetype string) time.Duration {
	if isStaticAssetFiletype(filetype) {
		return c.AssetRequestDelay()
	}
	return c.RequestDelay()
}

// staticAssetFiletypes are the coarse filetypes served by a static host or CDN
// and therefore safe to fetch with the faster asset pacing. It holds both the
// canonical filetypes derived from URL extensions and the hint tokens the
// extractor attaches to extensionless assets (image, media, vtt). html and
// other (unknown or extensionless, often dynamic) keep the document pacing.
var staticAssetFiletypes = map[string]struct{}{
	"css":      {},
	"js":       {},
	"json":     {},
	"xml":      {},
	"txt":      {},
	"pdf":      {},
	"manifest": {},
	"jpg":      {},
	"png":      {},
	"webp":     {},
	"gif":      {},
	"svg":      {},
	"ico":      {},
	"avif":     {},
	"bmp":      {},
	"font":     {},
	"video":    {},
	"audio":    {},
	"image":    {},
	"media":    {},
	"vtt":      {},
}

func isStaticAssetFiletype(filetype string) bool {
	_, ok := staticAssetFiletypes[strings.ToLower(filetype)]
	return ok
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
	if v, ok := obj["asset_request_delay_ms"]; ok {
		out.AssetRequestDelayMs = mergeInt(base.AssetRequestDelayMs, v, false)
	}
	if v, ok := obj["asset_max_concurrency"]; ok {
		out.AssetMaxConcurrency = mergeInt(base.AssetMaxConcurrency, v, true)
	}
	if v, ok := obj["enable_checks"]; ok {
		if b, ok := v.(bool); ok {
			out.EnableChecks = b
		}
	}
	if v, ok := obj["enable_graph"]; ok {
		if b, ok := v.(bool); ok {
			out.EnableGraph = b
		}
	}
	if v, ok := obj["enable_link_validation"]; ok {
		if b, ok := v.(bool); ok {
			out.EnableLinkValidation = b
		}
	}
	if v, ok := obj["notifications_enabled"]; ok {
		if b, ok := v.(bool); ok {
			out.NotificationsEnabled = b
		}
	}
	if v, ok := obj["slack_webhook_url"]; ok {
		if s, ok := v.(string); ok {
			out.SlackWebhookURL = strings.TrimSpace(s)
		}
	}
	if v, ok := obj["notify_on_direct"]; ok {
		if b, ok := v.(bool); ok {
			out.NotifyOnDirect = b
		}
	}
	if v, ok := obj["notify_on_schedule"]; ok {
		if b, ok := v.(bool); ok {
			out.NotifyOnSchedule = b
		}
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
