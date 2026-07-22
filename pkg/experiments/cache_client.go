package experiments

import (
	"context"
	"time"

	"k8s.io/klog/v2"
)

type Client interface {
	BoolFlagValueOrFailsafe(ctx context.Context, flagName string, failSafeValue bool) bool
}

type cacheEntry struct {
	value  bool
	expiry time.Time
}

type CacheClient struct {
	client Client
	values map[string]cacheEntry
	ttl    time.Duration
	logger klog.Logger
}

func NewCacheClient(client Client, logger klog.Logger, ttl time.Duration) Client {
	return &CacheClient{
		client: client,
		values: make(map[string]cacheEntry),
		ttl:    ttl,
		logger: logger,
	}
}

func (c *CacheClient) BoolFlagValueOrFailsafe(ctx context.Context, flagName string, failSafeValue bool) bool {
	entry, ok := c.values[flagName]
	c.logger.V(5).Info("Check cache entry", "flagName", flagName, "cache", c.values)

	needsClientCall := !ok || entry.expiry.Before(time.Now())
	c.logger.V(4).Info("Check cache", "flagName", flagName, "needsClientCall", needsClientCall)
	if needsClientCall {
		value := c.client.BoolFlagValueOrFailsafe(ctx, flagName, failSafeValue)
		c.values[flagName] = cacheEntry{
			value:  value,
			expiry: time.Now().Add(c.ttl),
		}
	}

	return c.values[flagName].value
}
