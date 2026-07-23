package api

// cache.go — in-memory resource cache for the API handlers.
//
// A single Collect() sweeps 21 Kubernetes APIs and is the dominant latency
// source. With a 30-second TTL cache, all handlers within a refresh window
// share one sweep instead of each triggering a fresh one.
//
// Key: kubeconfig context name. Namespace filtering happens in-memory after
// the cache hit, so one cache entry covers all namespace views.

import (
	"sync"
	"time"

	"github.com/anishetty/kgraph/internal/graph"
)

const cacheTTL = 30 * time.Second

type cacheEntry struct {
	res   graph.Resources
	built time.Time
}

type resourceCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func newResourceCache() *resourceCache {
	return &resourceCache{entries: make(map[string]cacheEntry)}
}

func (c *resourceCache) get(ctxName string) (*graph.Resources, bool) {
	c.mu.RLock()
	e, ok := c.entries[ctxName]
	c.mu.RUnlock()
	if !ok || time.Since(e.built) > cacheTTL {
		return nil, false
	}
	return &e.res, true
}

func (c *resourceCache) set(ctxName string, res graph.Resources) {
	c.mu.Lock()
	c.entries[ctxName] = cacheEntry{res: res, built: time.Now()}
	c.mu.Unlock()
}

func (c *resourceCache) invalidate(ctxName string) {
	c.mu.Lock()
	delete(c.entries, ctxName)
	c.mu.Unlock()
}
