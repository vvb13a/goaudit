package engine

import (
	"sync"
	"time"
)

type LinkResult struct {
	Passed       bool
	StatusCode   int
	ErrorMessage string
	ExpiresAt    time.Time
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

func (c *LinkCache) Get(urlOrPath string) (LinkResult, bool) {
	c.mu.RLock()
	item, found := c.items[urlOrPath]
	c.mu.RUnlock()

	if !found {
		return LinkResult{}, false
	}

	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, urlOrPath)
		c.mu.Unlock()
		return LinkResult{}, false
	}

	return item, true
}

func (c *LinkCache) Set(urlOrPath string, passed bool, statusCode int, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[urlOrPath] = LinkResult{
		Passed:       passed,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		ExpiresAt:    time.Now().Add(c.ttl),
	}
}
