package session

import (
	"context"
	"time"
)

// KVSessionRecord is a generic key-value session record for server-side sessions (e.g. Herald).
// It is not tied to Fiber; use SessionData and Storage for Fiber session backends.
type KVSessionRecord struct {
	ID        string                 `json:"id"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt time.Time              `json:"expires_at"`
}

// Store is a generic KV session store for server-side sessions.
// Used by services like Herald that need Create/Get/Set/Delete/Exists with TTL.
type Store interface {
	Create(ctx context.Context, data map[string]interface{}, ttl time.Duration) (id string, err error)
	Get(ctx context.Context, id string) (*KVSessionRecord, error)
	Set(ctx context.Context, id string, data map[string]interface{}, ttl time.Duration) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// KVManager wraps a Store and provides default TTL and a high-level API.
// Use NewKVManager(store, defaultTTL) then Create/Get/Set/Delete/Exists/Refresh.
type KVManager struct {
	store      Store
	defaultTTL time.Duration
}

// NewKVManager returns a KVManager that uses the given store and default TTL.
func NewKVManager(store Store, defaultTTL time.Duration) *KVManager {
	return &KVManager{
		store:      store,
		defaultTTL: defaultTTL,
	}
}

// Create creates a new session and returns its ID.
func (m *KVManager) Create(ctx context.Context, data map[string]interface{}, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = m.defaultTTL
	}
	return m.store.Create(ctx, data, ttl)
}

// Get returns the session for the given ID, or an error if not found/expired.
func (m *KVManager) Get(ctx context.Context, id string) (*KVSessionRecord, error) {
	return m.store.Get(ctx, id)
}

// Set updates the session for the given ID. If ttl is 0, the implementation may keep existing TTL or use default.
func (m *KVManager) Set(ctx context.Context, id string, data map[string]interface{}, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = m.defaultTTL
	}
	return m.store.Set(ctx, id, data, ttl)
}

// Delete removes the session for the given ID.
func (m *KVManager) Delete(ctx context.Context, id string) error {
	return m.store.Delete(ctx, id)
}

// Exists reports whether a session exists for the given ID.
func (m *KVManager) Exists(ctx context.Context, id string) (bool, error) {
	return m.store.Exists(ctx, id)
}

// Refresh extends the expiration of the session by setting it again with the given ttl.
func (m *KVManager) Refresh(ctx context.Context, id string, ttl time.Duration) error {
	rec, err := m.store.Get(ctx, id)
	if err != nil || rec == nil {
		return err
	}
	if ttl <= 0 {
		ttl = m.defaultTTL
	}
	return m.store.Set(ctx, id, rec.Data, ttl)
}
