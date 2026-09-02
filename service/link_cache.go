package service

import (
	"strings"
	"sync"
	"time"
)

// LinkResult represents the cached status of a URL verification.
type LinkResult struct {
	Passed       bool          `json:"passed"`
	StatusCode   int           `json:"status_code"`
	ErrorMessage string        `json:"error_message,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
	ExpiresAt    time.Time     `json:"expires_at"`
}

// LinkCache is an in-memory, thread-safe cache for link status checks.
type LinkCache struct {
	mu    sync.RWMutex
	items map[string]LinkResult
	ttl   time.Duration
}

// NewLinkCache creates a cache with a specific time-to-live per entry.
func NewLinkCache(ttl time.Duration) *LinkCache {
	return &LinkCache{
		items: make(map[string]LinkResult),
		ttl:   ttl,
	}
}

// normalizeURL trims whitespace and removes anchor fragments (#heading).
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "#"); idx != -1 {
		raw = raw[:idx]
	}
	return raw
}

// Get looks up a cached link result, removing expired entries safely.
func (c *LinkCache) Get(urlOrPath string) (LinkResult, bool) {
	key := normalizeURL(urlOrPath)

	c.mu.RLock()
	item, found := c.items[key]
	c.mu.RUnlock()

	if !found {
		return LinkResult{}, false
	}

	// Check expiration
	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		// Double-check expiration under write lock to avoid deleting a refreshed entry
		if current, exists := c.items[key]; exists && time.Now().After(current.ExpiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return LinkResult{}, false
	}

	return item, true
}

// Set stores a link verification outcome.
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

// SetResult stores a full LinkResult directly.
func (c *LinkCache) SetResult(urlOrPath string, res LinkResult) {
	key := normalizeURL(urlOrPath)
	if res.ExpiresAt.IsZero() {
		res.ExpiresAt = time.Now().Add(c.ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = res
}

// Len returns the total number of cached entries.
func (c *LinkCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Clear flushes all cached entries (useful between plan runs).
func (c *LinkCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]LinkResult)
}
