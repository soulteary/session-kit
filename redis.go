package session

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	rediskitclient "github.com/soulteary/redis-kit/client"
)

// RedisStorage implements Storage interface using Redis.
// This is suitable for production with multiple server instances
// as sessions are shared via Redis.
type RedisStorage struct {
	client    *redis.Client
	keyPrefix string
}

// NewRedisStorage creates a new Redis storage for sessions.
// The client parameter should be a valid Redis client.
// The keyPrefix is prepended to all session keys.
func NewRedisStorage(client *redis.Client, keyPrefix string) *RedisStorage {
	if keyPrefix == "" {
		keyPrefix = "session:"
	} else if len(keyPrefix) > 0 && keyPrefix[len(keyPrefix)-1] != ':' {
		keyPrefix += ":"
	}

	return &RedisStorage{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

// NewRedisStorageFromConfig creates a new Redis storage using configuration.
// This is a convenience function that creates both the Redis client and storage.
func NewRedisStorageFromConfig(addr, password string, db int, keyPrefix string) (*RedisStorage, error) {
	cfg := rediskitclient.DefaultConfig().
		WithAddr(addr).
		WithPassword(password).
		WithDB(db)

	client, err := rediskitclient.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rediskitclient.Ping(ctx, client); err != nil {
		_ = rediskitclient.Close(client)
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return NewRedisStorage(client, keyPrefix), nil
}

// buildKey constructs the full key with prefix.
func (s *RedisStorage) buildKey(key string) string {
	return s.keyPrefix + key
}

// Get retrieves the value for the given key.
// Returns nil, nil if the key does not exist.
func (s *RedisStorage) Get(key string) ([]byte, error) {
	if s.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	fullKey := s.buildKey(key)
	ctx := context.Background()

	data, err := s.client.Get(ctx, fullKey).Bytes()
	if err == redis.Nil {
		return nil, nil // Key does not exist, return nil, nil as per interface
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get from redis: %w", err)
	}

	return data, nil
}

// Set stores the given value for the given key along with an expiration value.
// If expiration is 0, the value never expires.
// Empty key or value will be ignored without an error.
func (s *RedisStorage) Set(key string, val []byte, exp time.Duration) error {
	if s.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	if key == "" || len(val) == 0 {
		return nil // Ignore empty key or value as per interface
	}

	fullKey := s.buildKey(key)
	ctx := context.Background()

	err := s.client.Set(ctx, fullKey, val, exp).Err()
	if err != nil {
		return fmt.Errorf("failed to set in redis: %w", err)
	}

	return nil
}

// Delete removes the value for the given key.
// It returns no error if the storage does not contain the key.
func (s *RedisStorage) Delete(key string) error {
	if s.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	fullKey := s.buildKey(key)
	ctx := context.Background()

	err := s.client.Del(ctx, fullKey).Err()
	if err != nil {
		return fmt.Errorf("failed to delete from redis: %w", err)
	}

	return nil
}

// Reset removes all keys with the configured prefix.
func (s *RedisStorage) Reset() error {
	if s.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	ctx := context.Background()

	// Get all keys matching the prefix
	pattern := s.keyPrefix + "*"
	iter := s.client.Scan(ctx, 0, pattern, 0).Iterator()

	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan keys: %w", err)
	}

	// Delete all keys
	if len(keys) > 0 {
		err := s.client.Del(ctx, keys...).Err()
		if err != nil {
			return fmt.Errorf("failed to delete keys: %w", err)
		}
	}

	return nil
}

// Close closes the Redis client connection.
func (s *RedisStorage) Close() error {
	if s.client == nil {
		return nil
	}

	err := rediskitclient.Close(s.client)
	if err != nil {
		return fmt.Errorf("failed to close redis client: %w", err)
	}

	return nil
}

// GetClient returns the underlying Redis client.
// This can be useful for advanced operations not covered by the Storage interface.
func (s *RedisStorage) GetClient() *redis.Client {
	return s.client
}

// GetKeyPrefix returns the key prefix used by this storage.
func (s *RedisStorage) GetKeyPrefix() string {
	return s.keyPrefix
}

// Exists checks if a key exists in Redis.
func (s *RedisStorage) Exists(key string) (bool, error) {
	if s.client == nil {
		return false, fmt.Errorf("redis client is nil")
	}

	fullKey := s.buildKey(key)
	ctx := context.Background()

	count, err := s.client.Exists(ctx, fullKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check existence in redis: %w", err)
	}

	return count > 0, nil
}

// GetTTL returns the remaining TTL for a key.
// Returns -2 if the key does not exist, -1 if the key has no expiration.
func (s *RedisStorage) GetTTL(key string) (time.Duration, error) {
	if s.client == nil {
		return 0, fmt.Errorf("redis client is nil")
	}

	fullKey := s.buildKey(key)
	ctx := context.Background()

	ttl, err := s.client.TTL(ctx, fullKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get TTL from redis: %w", err)
	}

	return ttl, nil
}

// Expire sets a new expiration on a key.
func (s *RedisStorage) Expire(key string, exp time.Duration) error {
	if s.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	fullKey := s.buildKey(key)
	ctx := context.Background()

	err := s.client.Expire(ctx, fullKey, exp).Err()
	if err != nil {
		return fmt.Errorf("failed to set expiration in redis: %w", err)
	}

	return nil
}
