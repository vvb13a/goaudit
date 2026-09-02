package service

import (
	"strings"
	"sync"
	"time"
)

type LinkResult struct {
	Passed       bool          `json:"passed"`
	StatusCode   int           `json:"status_code"`
	ErrorMessage string        `json:"error_message,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
	ExpiresAt    time.Time     `json:"expires_at"`
}

type LinkCache struct {
	mu    sync.RWMutex
	items map[string]LinkResult
	ttl   time.Duration
}

func NewLinkCache(ttl time.Duration) *LinkCache {
	return &LinkCache{
		items: make(map[string]LinkResult),
		ttl:   ttl,
	}
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "#"); idx != -1 {
		raw = raw[:idx]
	}
	return raw
}

func (c *LinkCache) Get(urlOrPath string) (LinkResult, bool) {
	key := normalizeURL(urlOrPath)

	c.mu.RLock()
	item, found := c.items[key]
	c.mu.RUnlock()

	if !found {
		return LinkResult{}, false
	}

	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		if current, exists := c.items[key]; exists && time.Now().After(current.ExpiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return LinkResult{}, false
	}

	return item, true
}

func (c *LinkCache) Set(urlOrPath string, passed bool, statusCode int, errMsg string) {
	key := normalizeURL(urlOrPath)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = LinkResult{
		Passed:       passed,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		ExpiresAt:    time.Now().Add(c.ttl),
	}
}

func (c *LinkCache) SetResult(urlOrPath string, res LinkResult) {
	key := normalizeURL(urlOrPath)
	if res.ExpiresAt.IsZero() {
		res.ExpiresAt = time.Now().Add(c.ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = res
}

func (c *LinkCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *LinkCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]LinkResult)
}
