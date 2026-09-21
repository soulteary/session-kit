package session

import (
	"fmt"
	"sync"
	"time"
)

// StorageType represents the type of storage backend.
type StorageType string

const (
	// StorageTypeMemory uses in-memory storage.
	StorageTypeMemory StorageType = "memory"
	// StorageTypeRedis uses Redis storage. It is registered by importing
	// github.com/soulteary/session-kit/v3/redisstore; see [NewStorage].
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

	// RedisAddrs is a seed list of host:port addresses: the nodes of a Redis
	// Cluster, or the Sentinel nodes of a failover setup. It is what lets a
	// configuration-driven deployment be something other than standalone --
	// RedisAddr alone can only name one server. When it is empty, RedisAddr
	// is used; two or more entries with RedisMasterName empty select a
	// cluster client.
	RedisAddrs []string

	// RedisMasterName is the Sentinel master name. Setting it selects a
	// Sentinel-backed failover client, and RedisAddrs (or RedisAddr) is then
	// read as the Sentinel addresses rather than as Redis servers.
	RedisMasterName string

	// RedisPassword is the Redis password (empty if no password). It
	// authenticates to the Redis server itself -- under Sentinel, that is the
	// master Sentinel points at, not the Sentinel nodes.
	RedisPassword string

	// RedisSentinelUsername and RedisSentinelPassword authenticate to the
	// Sentinel nodes themselves. They are separate from RedisPassword because
	// the two credentials frequently differ, and a Sentinel deployment with
	// ACLs cannot be reached without them.
	RedisSentinelUsername string
	RedisSentinelPassword string

	// RedisDB is the Redis database number (for Redis storage). Redis Cluster
	// supports only database 0, so this is ignored there.
	RedisDB int

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

// WithRedisAddrs sets the seed list of cluster or Sentinel addresses. Passing
// none clears it, which puts the configuration back on RedisAddr.
func (c StorageConfig) WithRedisAddrs(addrs ...string) StorageConfig {
	c.RedisAddrs = addrs
	return c
}

// WithRedisMasterName sets the Sentinel master name, selecting a
// Sentinel-backed failover client.
func (c StorageConfig) WithRedisMasterName(name string) StorageConfig {
	c.RedisMasterName = name
	return c
}

// WithRedisSentinelAuth sets the credentials for the Sentinel nodes
// themselves, which are not the ones [StorageConfig.WithRedisPassword] sets.
func (c StorageConfig) WithRedisSentinelAuth(username, password string) StorageConfig {
	c.RedisSentinelUsername = username
	c.RedisSentinelPassword = password
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

// WithMemoryGCInterval sets the memory storage garbage collection interval.
func (c StorageConfig) WithMemoryGCInterval(interval time.Duration) StorageConfig {
	c.MemoryGCInterval = interval
	return c
}

// StorageBuilder creates a Storage from a StorageConfig. It is what a backend
// living outside this package registers with [RegisterStorage].
type StorageBuilder func(StorageConfig) (Storage, error)

var (
	buildersMu sync.RWMutex
	builders   = map[StorageType]StorageBuilder{}
)

// RegisterStorage makes a storage backend available to [NewStorage] under the
// given type, in the manner of database/sql drivers. It panics on an empty
// type or a duplicate registration, both of which are programming errors that
// would otherwise surface as the wrong backend at runtime.
//
// Backends that need a third-party client register themselves from an init
// function, so that importing them is what links the client in:
//
//	import _ "github.com/soulteary/session-kit/v3/redisstore"
//
// Nothing here is needed to use a backend directly -- redisstore.New returns
// a Storage that NewManager accepts as it is. Registration exists so that
// [NewStorage] and [NewStorageFromEnv], which pick a backend from
// configuration rather than from code, can reach one this package does not
// import. The same hook takes a Memcached, DynamoDB or in-house backend.
func RegisterStorage(t StorageType, build StorageBuilder) {
	if t == "" {
		panic("session: RegisterStorage called with an empty storage type")
	}
	if build == nil {
		panic("session: RegisterStorage called with a nil builder")
	}

	buildersMu.Lock()
	defer buildersMu.Unlock()

	if _, dup := builders[t]; dup {
		panic(fmt.Sprintf("session: storage type %q registered twice", t))
	}
	builders[t] = build
}

func init() {
	RegisterStorage(StorageTypeMemory, func(cfg StorageConfig) (Storage, error) {
		return NewMemoryStorage(cfg.KeyPrefix, cfg.MemoryGCInterval), nil
	})
}

// NewStorage creates a new Storage instance based on the configuration.
// It automatically selects the appropriate storage backend based on the Type field.
//
// Only StorageTypeMemory is built in. Every other type must have been
// registered, which for Redis means importing the redisstore subpackage:
//
//	import _ "github.com/soulteary/session-kit/v3/redisstore"
//
// That import is what links go-redis into the binary, so a service on
// in-memory sessions does not carry it. Code that already holds a Redis
// client should skip this factory and call redisstore.New directly.
func NewStorage(cfg StorageConfig) (Storage, error) {
	buildersMu.RLock()
	build, ok := builders[cfg.Type]
	buildersMu.RUnlock()

	if ok {
		return build(cfg)
	}

	if cfg.Type == StorageTypeRedis {
		return nil, fmt.Errorf("storage type %q is not registered: add `import _ \"github.com/soulteary/session-kit/v3/redisstore\"`, or build the storage with redisstore.New", cfg.Type)
	}
	return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
}

// NewStorageFromEnv creates a Storage based on environment-like configuration.
// If redisEnabled is true, it creates a Redis storage; otherwise, it creates a memory storage.
// This is a convenience function for common use cases.
//
// Redis requires the redisstore subpackage to have been imported; see
// [NewStorage].
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
