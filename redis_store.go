package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements Store using Redis. Keys are prefixed with keyPrefix.
type RedisStore struct {
	client    *redis.Client
	keyPrefix string
}

// NewRedisStore creates a Redis-backed Store. keyPrefix is prepended to all keys (e.g. "otp:session:").
func NewRedisStore(client *redis.Client, keyPrefix string) *RedisStore {
	if keyPrefix != "" && keyPrefix[len(keyPrefix)-1] != ':' {
		keyPrefix += ":"
	}
	return &RedisStore{client: client, keyPrefix: keyPrefix}
}

func (s *RedisStore) key(id string) string {
	return s.keyPrefix + id
}

func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sess_" + base64.URLEncoding.EncodeToString(b)[:22], nil
}

// Create creates a new session and returns its ID.
func (s *RedisStore) Create(ctx context.Context, data map[string]interface{}, ttl time.Duration) (string, error) {
	id, err := generateSessionID()
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	if err := s.Set(ctx, id, data, ttl); err != nil {
		return "", err
	}
	return id, nil
}

// Get returns the session for the given ID, or nil and error if not found/expired.
func (s *RedisStore) Get(ctx context.Context, id string) (*KVSessionRecord, error) {
	if s.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}
	data, err := s.client.Get(ctx, s.key(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var rec KVSessionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	if time.Now().After(rec.ExpiresAt) {
		_ = s.client.Del(ctx, s.key(id))
		return nil, nil
	}
	return &rec, nil
}

// Set stores or updates the session for the given ID with the given ttl.
// When updating an existing session, CreatedAt is preserved.
func (s *RedisStore) Set(ctx context.Context, id string, data map[string]interface{}, ttl time.Duration) error {
	if s.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	now := time.Now()
	createdAt := now
	if existing, _ := s.Get(ctx, id); existing != nil {
		createdAt = existing.CreatedAt
	}
	rec := &KVSessionRecord{
		ID:        id,
		Data:      data,
		CreatedAt: createdAt,
		ExpiresAt: now.Add(ttl),
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := s.client.Set(ctx, s.key(id), body, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Delete removes the session for the given ID.
func (s *RedisStore) Delete(ctx context.Context, id string) error {
	if s.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	if err := s.client.Del(ctx, s.key(id)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

// Exists reports whether a session exists for the given ID.
func (s *RedisStore) Exists(ctx context.Context, id string) (bool, error) {
	if s.client == nil {
		return false, fmt.Errorf("redis client is nil")
	}
	n, err := s.client.Exists(ctx, s.key(id)).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists: %w", err)
	}
	return n > 0, nil
}
