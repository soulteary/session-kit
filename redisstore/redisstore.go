// Package redisstore backs session-kit with Redis.
//
// It lives in its own package so that importing the root package does not drag
// go-redis -- and with it cespare/xxhash and go.uber.org/atomic -- into
// binaries that keep sessions in memory. A single-instance service, a test
// binary or a CLI pays nothing for Redis support existing; only importing this
// package links it in.
//
// It provides both of session-kit's storage shapes:
//
//   - [Storage] implements session.Storage, the byte-blob backend behind
//     session.Manager and the Fiber session middleware.
//   - [Store] implements session.Store, the generic key-value session store
//     behind session.KVManager.
//
// Constructing either one directly is the usual path:
//
//	storage := redisstore.New(redisClient, "session:")
//	manager := session.NewManager(storage, session.DefaultConfig())
//
// [New] accepts any go-redis client shape -- standalone, cluster, ring or a
// Sentinel-backed failover client. To have the package open the connection
// instead, [NewFromConfig] takes a single server address and
// [NewFromStorageConfig] takes a full [session.StorageConfig], which can also
// describe a cluster or a Sentinel deployment.
//
// Importing this package also registers session.StorageTypeRedis with
// session.NewStorage, for code that picks its backend from configuration:
//
//	import _ "github.com/soulteary/session-kit/v3/redisstore"
//
//	storage, err := session.NewStorageFromEnv(useRedis, addr, pass, db, "session:")
package redisstore

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/redis/go-redis/v9"
	rediskitclient "github.com/soulteary/redis-kit/client"

	session "github.com/soulteary/session-kit/v3"
)

// DefaultKeyPrefix is the key prefix [New] applies when given an empty one.
const DefaultKeyPrefix = "session:"

// dialTimeout bounds the connectivity check [NewFromConfig] makes before it
// hands back a storage.
const dialTimeout = 5 * time.Second

// Client is the part of a go-redis client this package uses. *redis.Client,
// *redis.ClusterClient, *redis.Ring and redis.UniversalClient all satisfy it,
// so a store written against it works the same on a standalone server, a
// cluster and a Sentinel failover setup -- where the previous *redis.Client
// parameter accepted only the first.
//
// Note that Reset scans for keys, which on a *redis.ClusterClient walks every
// master node; that is Redis's behaviour, not this package's.
type Client interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	TTL(ctx context.Context, key string) *redis.DurationCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd
}

func init() {
	session.RegisterStorage(session.StorageTypeRedis, func(cfg session.StorageConfig) (session.Storage, error) {
		// The whole config, not four of its fields: that is what lets
		// session.NewStorage reach a cluster or a Sentinel deployment.
		return NewFromStorageConfig(cfg)
	})
}

// Storage implements session.Storage using Redis.
// This is suitable for production with multiple server instances
// as sessions are shared via Redis.
type Storage struct {
	client    Client
	keyPrefix string
}

// Compile-time proof that a Storage is usable wherever session.Storage is.
var _ session.Storage = (*Storage)(nil)

// New creates a new Redis storage for sessions.
// The client parameter should be a valid Redis client.
// The keyPrefix is prepended to all session keys; an empty one becomes
// [DefaultKeyPrefix], and one not ending in ':' gets one appended.
func New(client Client, keyPrefix string) *Storage {
	return &Storage{
		client:    client,
		keyPrefix: normalizePrefix(keyPrefix, DefaultKeyPrefix),
	}
}

// NewFromConfig creates a new Redis storage for a single standalone server.
// This is a convenience function that creates both the Redis client and
// storage, and verifies connectivity before returning.
//
// Its parameters can only describe one server. For a cluster or a Sentinel
// deployment use [NewFromStorageConfig], or build the client yourself and
// pass it to [New] -- which has always accepted any client shape.
func NewFromConfig(addr, password string, db int, keyPrefix string) (*Storage, error) {
	return NewFromStorageConfig(session.StorageConfig{
		RedisAddr:     addr,
		RedisPassword: password,
		RedisDB:       db,
		KeyPrefix:     keyPrefix,
	})
}

// NewFromStorageConfig creates a new Redis storage from a full
// [session.StorageConfig], and verifies connectivity before returning.
//
// It builds whichever client the configuration describes: a Sentinel-backed
// failover client when RedisMasterName is set, a cluster client when
// RedisAddrs holds more than one address, and a single-node client otherwise.
// [New] has always accepted any of those; this is the path that can now
// *construct* one, so a cluster or Sentinel deployment is served end to end
// rather than only by callers who already hold a client.
//
// This is also what session.NewStorage calls for StorageTypeRedis, so
// selecting a backend from configuration reaches the same client shapes.
func NewFromStorageConfig(cfg session.StorageConfig) (*Storage, error) {
	client, err := rediskitclient.NewUniversalClient(clientConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := rediskitclient.Ping(ctx, client); err != nil {
		_ = rediskitclient.Close(client)
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return New(client, cfg.KeyPrefix), nil
}

// clientConfig maps a session.StorageConfig onto the redis-kit client config
// that decides which client shape gets built.
//
// Separate from NewFromStorageConfig so the mapping can be asserted without a
// network: the alternative is a test that waits out go-redis's Sentinel
// discovery retries to prove a field was copied.
func clientConfig(cfg session.StorageConfig) rediskitclient.Config {
	// Addrs is copied unconditionally. An empty one needs no guard: redis-kit
	// falls back to Addr on len(Addrs) == 0, so a branch here would be dead
	// code -- removing it changed no test, which is how it was found.
	return rediskitclient.DefaultConfig().
		WithAddr(cfg.RedisAddr).
		WithAddrs(cfg.RedisAddrs...).
		WithPassword(cfg.RedisPassword).
		WithDB(cfg.RedisDB).
		WithMasterName(cfg.RedisMasterName).
		WithSentinelAuth(cfg.RedisSentinelUsername, cfg.RedisSentinelPassword)
}

// normalizePrefix applies fallback when prefix is empty and makes sure the
// result ends in ':', so that "otp" and "otp:" name the same keyspace.
func normalizePrefix(prefix, fallback string) string {
	if prefix == "" {
		return fallback
	}
	if prefix[len(prefix)-1] != ':' {
		return prefix + ":"
	}
	return prefix
}

// isNil reports whether there is no client to talk to.
//
// Client is an interface, so a plain client == nil misses the case that
// actually reaches here -- a nil *redis.Client stored in it, which a caller
// gets from an unassigned field or a constructor that returned early. Calling
// a command on that panics, and a session lookup that takes the process down
// is worse than the storage error it was meant to report.
func isNil(client Client) bool {
	if client == nil {
		return true
	}
	v := reflect.ValueOf(client)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// buildKey constructs the full key with prefix.
func (s *Storage) buildKey(key string) string {
	return s.keyPrefix + key
}

// Get retrieves the value for the given key.
// Returns nil, nil if the key does not exist.
func (s *Storage) Get(key string) ([]byte, error) {
	if isNil(s.client) {
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
//
// An empty key is ignored without an error, matching fiber.Storage. An empty
// value DELETES the key, as the session.Storage contract requires: ignoring it
// left the previous value in place, so overwriting a session with empty data
// kept the old, still-authenticated payload readable -- and code tested
// against MemoryStorage, which does delete, behaved differently here in
// production.
func (s *Storage) Set(key string, val []byte, exp time.Duration) error {
	if isNil(s.client) {
		return fmt.Errorf("redis client is nil")
	}

	if key == "" {
		return nil
	}
	if len(val) == 0 {
		return s.Delete(key)
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
func (s *Storage) Delete(key string) error {
	if isNil(s.client) {
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
func (s *Storage) Reset() error {
	if isNil(s.client) {
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
//
// A Client that does not close -- a shared handle deliberately wrapped to
// hide Close, say -- makes this a no-op rather than an error: the storage
// never owned the connection in that case.
func (s *Storage) Close() error {
	if isNil(s.client) {
		return nil
	}

	closer, ok := s.client.(interface{ Close() error })
	if !ok {
		return nil
	}

	if err := closer.Close(); err != nil {
		return fmt.Errorf("failed to close redis client: %w", err)
	}

	return nil
}

// Client returns the underlying Redis client.
// This can be useful for advanced operations not covered by session.Storage.
func (s *Storage) Client() Client {
	return s.client
}

// KeyPrefix returns the key prefix used by this storage.
func (s *Storage) KeyPrefix() string {
	return s.keyPrefix
}

// Exists checks if a key exists in Redis.
func (s *Storage) Exists(key string) (bool, error) {
	if isNil(s.client) {
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

// TTL returns the remaining TTL for a key.
// Returns -2 if the key does not exist, -1 if the key has no expiration.
func (s *Storage) TTL(key string) (time.Duration, error) {
	if isNil(s.client) {
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
func (s *Storage) Expire(key string, exp time.Duration) error {
	if isNil(s.client) {
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
