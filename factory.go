package session

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// StorageType represents the type of storage backend.
type StorageType string

const (
	// StorageTypeMemory uses in-memory storage.
	StorageTypeMemory StorageType = "memory"
	// StorageTypeRedis uses Redis storage.
	StorageTypeRedis StorageType = "redis"
)

// StorageConfig represents configuration for creating a storage backend.
type StorageConfig struct {
	// Type is the storage backend type.
	Type StorageType

	// KeyPrefix is the prefix for session keys.
	KeyPrefix string

	// RedisAddr is the Redis server address (for Redis storage).
	RedisAddr string

	// RedisPassword is the Redis password (for Redis storage).
	RedisPassword string

	// RedisDB is the Redis database number (for Redis storage).
	RedisDB int

	// RedisClient is an existing Redis client (for Redis storage).
	// If provided, RedisAddr, RedisPassword, and RedisDB are ignored.
	RedisClient *redis.Client

	// MemoryGCInterval is the garbage collection interval for memory storage.
	// Default: 10 minutes. Set to 0 to disable GC.
	MemoryGCInterval time.Duration
}

// DefaultStorageConfig returns a StorageConfig with default values.
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		Type:             StorageTypeMemory,
		KeyPrefix:        "session:",
		RedisAddr:        "localhost:6379",
		RedisPassword:    "",
		RedisDB:          0,
		MemoryGCInterval: 10 * time.Minute,
	}
}

// WithType sets the storage type.
func (c StorageConfig) WithType(t StorageType) StorageConfig {
	c.Type = t
	return c
}

// WithKeyPrefix sets the key prefix.
func (c StorageConfig) WithKeyPrefix(prefix string) StorageConfig {
	c.KeyPrefix = prefix
	return c
}

// WithRedisAddr sets the Redis address.
func (c StorageConfig) WithRedisAddr(addr string) StorageConfig {
	c.RedisAddr = addr
	return c
}

// WithRedisPassword sets the Redis password.
func (c StorageConfig) WithRedisPassword(password string) StorageConfig {
	c.RedisPassword = password
	return c
}

// WithRedisDB sets the Redis database number.
func (c StorageConfig) WithRedisDB(db int) StorageConfig {
	c.RedisDB = db
	return c
}

// WithRedisClient sets an existing Redis client.
func (c StorageConfig) WithRedisClient(client *redis.Client) StorageConfig {
	c.RedisClient = client
	return c
}

// WithMemoryGCInterval sets the memory storage garbage collection interval.
func (c StorageConfig) WithMemoryGCInterval(interval time.Duration) StorageConfig {
	c.MemoryGCInterval = interval
	return c
}

// NewStorage creates a new Storage instance based on the configuration.
// It automatically selects the appropriate storage backend based on the Type field.
func NewStorage(cfg StorageConfig) (Storage, error) {
	switch cfg.Type {
	case StorageTypeMemory:
		return NewMemoryStorage(cfg.KeyPrefix, cfg.MemoryGCInterval), nil

	case StorageTypeRedis:
		if cfg.RedisClient != nil {
			return NewRedisStorage(cfg.RedisClient, cfg.KeyPrefix), nil
		}
		return NewRedisStorageFromConfig(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.KeyPrefix)

	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
	}
}

// NewStorageFromEnv creates a Storage based on environment-like configuration.
// If redisEnabled is true, it creates a Redis storage; otherwise, it creates a memory storage.
// This is a convenience function for common use cases.
func NewStorageFromEnv(redisEnabled bool, redisAddr, redisPassword string, redisDB int, keyPrefix string) (Storage, error) {
	if redisEnabled {
		cfg := DefaultStorageConfig().
			WithType(StorageTypeRedis).
			WithRedisAddr(redisAddr).
			WithRedisPassword(redisPassword).
			WithRedisDB(redisDB).
			WithKeyPrefix(keyPrefix)
		return NewStorage(cfg)
	}

	cfg := DefaultStorageConfig().
		WithType(StorageTypeMemory).
		WithKeyPrefix(keyPrefix)
	return NewStorage(cfg)
}

// MustNewStorage creates a new Storage instance or panics if an error occurs.
// This is useful for initialization in main() where errors should be fatal.
func MustNewStorage(cfg StorageConfig) Storage {
	storage, err := NewStorage(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create session storage: %v", err))
	}
	return storage
}
