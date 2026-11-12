package admissions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// IdempotencyStore abstracts persisted/cached idempotency lookup & storage across retries.
type IdempotencyStore interface {
	Lookup(ctx context.Context, tenantID, userID uuid.UUID, op, key string, dest any) (bool, error)
	Save(ctx context.Context, tenantID, userID uuid.UUID, op, key string, value any) error
}

type idempotencyEntry struct {
	expiresAt time.Time
	value     any
}

// IdempotencyCache prevents duplicate mutations across retried mobile/slow network requests.
type IdempotencyCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]idempotencyEntry
}

// NewIdempotencyCache creates an in-process scoped idempotency store.
func NewIdempotencyCache(ttl time.Duration) *IdempotencyCache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &IdempotencyCache{
		ttl:     ttl,
		entries: make(map[string]idempotencyEntry),
	}
}

func (c *IdempotencyCache) buildKey(tenantID, userID uuid.UUID, op, key string) string {
	return fmt.Sprintf("%s:%s:%s:%s", tenantID.String(), userID.String(), op, key)
}

// Get returns a cached response for a repeated Idempotency-Key if not expired.
func (c *IdempotencyCache) Get(tenantID, userID uuid.UUID, op, key string) (any, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	now := time.Now().UTC()
	fullKey := c.buildKey(tenantID, userID, op, key)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[fullKey]
	if !ok {
		return nil, false
	}
	if !entry.expiresAt.After(now) {
		delete(c.entries, fullKey)
		return nil, false
	}
	return entry.value, true
}

// Set caches a mutation result under a scoped Idempotency-Key.
func (c *IdempotencyCache) Set(tenantID, userID uuid.UUID, op, key string, value any) {
	if c == nil || key == "" {
		return
	}
	now := time.Now().UTC()
	fullKey := c.buildKey(tenantID, userID, op, key)

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) > 4096 {
		for k, v := range c.entries {
			if !v.expiresAt.After(now) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[fullKey] = idempotencyEntry{
		expiresAt: now.Add(c.ttl),
		value:     value,
	}
}

// Lookup satisfies IdempotencyStore by unmarshaling the JSON snapshot into dest when present.
func (c *IdempotencyCache) Lookup(_ context.Context, tenantID, userID uuid.UUID, op, key string, dest any) (bool, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || c == nil {
		return false, nil
	}
	rawVal, ok := c.Get(tenantID, userID, op, trimmed)
	if !ok {
		return false, nil
	}
	if b, isBytes := rawVal.([]byte); isBytes {
		if err := json.Unmarshal(b, dest); err != nil {
			return false, err
		}
		return true, nil
	}
	encoded, err := json.Marshal(rawVal)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(encoded, dest); err != nil {
		return false, err
	}
	return true, nil
}

// Save satisfies IdempotencyStore by storing a JSON snapshot so callers never alias pointers across retries.
func (c *IdempotencyCache) Save(_ context.Context, tenantID, userID uuid.UUID, op, key string, value any) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || c == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.Set(tenantID, userID, op, trimmed, encoded)
	return nil
}

