package redisstore

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	rediskitclient "github.com/soulteary/redis-kit/client"
	session "github.com/soulteary/session-kit/v3"
)

// NewFromStorageConfig builds whichever client the configuration describes.
// Before it existed, the config path could only ever produce a standalone
// client, so a cluster or Sentinel deployment was reachable only by callers
// who already held a client and called New directly.
func TestNewFromStorageConfigSelectsTheClientShape(t *testing.T) {
	t.Run("single address builds a standalone client and works", func(t *testing.T) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("failed to start miniredis: %v", err)
		}
		defer mr.Close()

		storage, err := NewFromStorageConfig(session.DefaultStorageConfig().
			WithType(session.StorageTypeRedis).
			WithRedisAddr(mr.Addr()).
			WithKeyPrefix("test:"))
		if err != nil {
			t.Fatalf("NewFromStorageConfig() error = %v, want nil", err)
		}
		defer func() { _ = storage.Close() }()

		if _, ok := storage.Client().(*redis.Client); !ok {
			t.Errorf("Client() = %T, want *redis.Client for a single address", storage.Client())
		}
		if err := storage.Set("k", []byte("v"), 0); err != nil {
			t.Fatalf("Set() error = %v, want nil", err)
		}
		got, err := storage.Get("k")
		if err != nil || string(got) != "v" {
			t.Errorf("Get() = %q, %v; want \"v\", nil", got, err)
		}
	})

	t.Run("RedisAddrs takes precedence over RedisAddr", func(t *testing.T) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("failed to start miniredis: %v", err)
		}
		defer mr.Close()

		// RedisAddr points somewhere unusable; only RedisAddrs winning makes
		// this connect at all.
		storage, err := NewFromStorageConfig(session.DefaultStorageConfig().
			WithRedisAddr("127.0.0.1:1").
			WithRedisAddrs(mr.Addr()).
			WithKeyPrefix("test:"))
		if err != nil {
			t.Fatalf("NewFromStorageConfig() error = %v, want nil", err)
		}
		defer func() { _ = storage.Close() }()

		if err := storage.Set("k", []byte("v"), 0); err != nil {
			t.Errorf("Set() error = %v, want nil", err)
		}
	})

	t.Run("no address at all is rejected", func(t *testing.T) {
		_, err := NewFromStorageConfig(session.StorageConfig{KeyPrefix: "test:"})
		if err == nil {
			t.Fatal("NewFromStorageConfig() with no address should return an error")
		}
	})
}

// NewFromConfig keeps its signature and its behaviour; it is now the
// single-server shorthand for NewFromStorageConfig.
func TestNewFromConfigStillBuildsAStandaloneClient(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	storage, err := NewFromConfig(mr.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v, want nil", err)
	}
	defer func() { _ = storage.Close() }()

	if _, ok := storage.Client().(*redis.Client); !ok {
		t.Errorf("Client() = %T, want *redis.Client", storage.Client())
	}
	if storage.KeyPrefix() != "test:" {
		t.Errorf("KeyPrefix() = %q, want %q", storage.KeyPrefix(), "test:")
	}
}

// The registry path reaches the same constructor, so picking a backend from
// configuration reaches cluster and Sentinel too.
func TestNewStorageCarriesTheWholeConfigToRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	// RedisAddr is unusable, so this only succeeds if session.NewStorage
	// passed RedisAddrs through rather than the four fields it used to.
	storage, err := session.NewStorage(session.DefaultStorageConfig().
		WithType(session.StorageTypeRedis).
		WithRedisAddr("127.0.0.1:1").
		WithRedisAddrs(mr.Addr()).
		WithKeyPrefix("test:"))
	if err != nil {
		t.Fatalf("session.NewStorage() error = %v, want nil", err)
	}
	defer func() { _ = storage.Close() }()

	if err := storage.Set("k", []byte("v"), 0); err != nil {
		t.Errorf("Set() error = %v, want nil", err)
	}
}

// The Sentinel and cluster fields have to reach the client config, which is
// what decides the client shape. Asserted on the mapping rather than by
// dialling: proving it end to end means waiting out go-redis's Sentinel
// discovery retries, which cost 8.6s for one field copy. Which shape each
// config produces is redis-kit's contract, and is tested there.
func TestClientConfigMapping(t *testing.T) {
	t.Run("sentinel fields are carried over", func(t *testing.T) {
		got := clientConfig(session.DefaultStorageConfig().
			WithRedisAddrs("127.0.0.1:26379", "127.0.0.2:26379").
			WithRedisMasterName("mymaster").
			WithRedisSentinelAuth("sentinel-user", "sentinel-pass").
			WithRedisPassword("server-pass"))

		if got.MasterName != "mymaster" {
			t.Errorf("MasterName = %q, want %q", got.MasterName, "mymaster")
		}
		if len(got.Addrs) != 2 {
			t.Errorf("Addrs = %v, want 2 entries", got.Addrs)
		}
		if got.SentinelUsername != "sentinel-user" || got.SentinelPassword != "sentinel-pass" {
			t.Errorf("sentinel auth = %q/%q, want %q/%q",
				got.SentinelUsername, got.SentinelPassword, "sentinel-user", "sentinel-pass")
		}
		// The Sentinel credentials must not be confused with the ones used
		// against the Redis server behind it.
		if got.Password != "server-pass" {
			t.Errorf("Password = %q, want %q", got.Password, "server-pass")
		}
	})

	t.Run("a lone Addr survives and stays the one redis-kit will use", func(t *testing.T) {
		got := clientConfig(session.DefaultStorageConfig().WithRedisAddr("10.0.0.1:6379"))
		if got.Addr != "10.0.0.1:6379" {
			t.Errorf("Addr = %q, want %q", got.Addr, "10.0.0.1:6379")
		}
		// len 0, not nil: redis-kit falls back to Addr on the length, which is
		// why clientConfig needs no branch for the empty case.
		if len(got.Addrs) != 0 {
			t.Errorf("Addrs = %v, want empty so Addr is used", got.Addrs)
		}
	})

	t.Run("defaults survive the mapping", func(t *testing.T) {
		got := clientConfig(session.DefaultStorageConfig())
		want := rediskitclient.DefaultConfig()
		if got.PoolSize != want.PoolSize || got.DialTimeout != want.DialTimeout {
			t.Errorf("pool/timeout defaults lost: PoolSize=%d DialTimeout=%v, want %d/%v",
				got.PoolSize, got.DialTimeout, want.PoolSize, want.DialTimeout)
		}
	})
}
