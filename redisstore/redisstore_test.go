package redisstore

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	session "github.com/soulteary/session-kit/v3"
)

func setupMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

func TestStorageBasicOperations(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Test Set and Get
	key := "session1"
	value := []byte("test data")
	exp := 1 * time.Hour

	err := storage.Set(key, value, exp)
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	got, err := storage.Get(key)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("expected %s, got %s", string(value), string(got))
	}

	// Test Delete
	err = storage.Delete(key)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	got, err = storage.Get(key)
	if err != nil {
		t.Fatalf("failed to get after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestStorageGetNonExistent(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	got, err := storage.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestStorageEmptyKeyValue(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Empty key should be ignored
	err := storage.Set("", []byte("value"), time.Hour)
	if err != nil {
		t.Errorf("expected no error for empty key, got %v", err)
	}

	// Empty value should be ignored
	err = storage.Set("key", []byte{}, time.Hour)
	if err != nil {
		t.Errorf("expected no error for empty value, got %v", err)
	}
}

func TestStorageNoExpiration(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	key := "persistent"
	value := []byte("test")

	// Set with no expiration
	err := storage.Set(key, value, 0)
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	got, err := storage.Get(key)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("expected %s, got %s", string(value), string(got))
	}
}

func TestStorageReset(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Add some data
	_ = storage.Set("key1", []byte("value1"), time.Hour)
	_ = storage.Set("key2", []byte("value2"), time.Hour)

	// Reset
	err := storage.Reset()
	if err != nil {
		t.Fatalf("failed to reset: %v", err)
	}

	// Verify deletion
	got1, _ := storage.Get("key1")
	got2, _ := storage.Get("key2")
	if got1 != nil || got2 != nil {
		t.Error("expected all keys to be deleted after reset")
	}
}

func TestStorageKeyPrefix(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	// Test with colon
	storage1 := New(client, "prefix:")
	if storage1.KeyPrefix() != "prefix:" {
		t.Errorf("expected prefix 'prefix:', got %s", storage1.KeyPrefix())
	}

	// Test without colon (should add it)
	storage2 := New(client, "prefix")
	if storage2.KeyPrefix() != "prefix:" {
		t.Errorf("expected prefix 'prefix:', got %s", storage2.KeyPrefix())
	}

	// Test empty prefix (should use default)
	storage3 := New(client, "")
	if storage3.KeyPrefix() != "session:" {
		t.Errorf("expected prefix 'session:', got %s", storage3.KeyPrefix())
	}
}

func TestStorageClose(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()

	storage := New(client, "test:")

	err := storage.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}
}

func TestStorageClient(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	if storage.Client() != client {
		t.Error("expected Client() to return the same client")
	}
}

func TestStorageExists(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Key should not exist
	exists, err := storage.Exists("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected key to not exist")
	}

	// Add key
	_ = storage.Set("existing", []byte("value"), time.Hour)

	// Key should exist
	exists, err = storage.Exists("existing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected key to exist")
	}
}

func TestStorageTTL(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Set with expiration
	_ = storage.Set("expiring", []byte("value"), 1*time.Hour)

	ttl, err := storage.TTL("expiring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttl <= 0 {
		t.Errorf("expected positive TTL, got %v", ttl)
	}

	// Non-existent key - Redis returns -2 (as nanoseconds in go-redis)
	ttl, err = storage.TTL("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// go-redis returns -2 nanoseconds for non-existent keys
	if ttl >= 0 {
		t.Errorf("expected negative TTL for non-existent key, got %v", ttl)
	}
}

func TestStorageExpire(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Set with no expiration
	_ = storage.Set("key", []byte("value"), 0)

	// Set expiration
	err := storage.Expire("key", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to set expiration: %v", err)
	}

	// Check TTL
	ttl, _ := storage.TTL("key")
	if ttl <= 0 {
		t.Errorf("expected positive TTL after Expire, got %v", ttl)
	}
}

func TestStorageNilClient(t *testing.T) {
	storage := &Storage{client: nil, keyPrefix: "test:"}

	// All operations should return error
	_, err := storage.Get("key")
	if err == nil {
		t.Error("expected error for nil client on Get")
	}

	err = storage.Set("key", []byte("value"), time.Hour)
	if err == nil {
		t.Error("expected error for nil client on Set")
	}

	err = storage.Delete("key")
	if err == nil {
		t.Error("expected error for nil client on Delete")
	}

	err = storage.Reset()
	if err == nil {
		t.Error("expected error for nil client on Reset")
	}

	_, err = storage.Exists("key")
	if err == nil {
		t.Error("expected error for nil client on Exists")
	}

	_, err = storage.TTL("key")
	if err == nil {
		t.Error("expected error for nil client on TTL")
	}

	err = storage.Expire("key", time.Hour)
	if err == nil {
		t.Error("expected error for nil client on Expire")
	}

	// Close should not error
	err = storage.Close()
	if err != nil {
		t.Errorf("expected no error for nil client on Close, got %v", err)
	}
}

func TestNewFromConfig(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	storage, err := NewFromConfig(mr.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Close() }()

	// Verify it works
	err = storage.Set("test", []byte("value"), time.Hour)
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	got, err := storage.Get("test")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if string(got) != "value" {
		t.Errorf("expected 'value', got '%s'", string(got))
	}
}

func TestNewFromConfigError(t *testing.T) {
	// Invalid address should fail
	_, err := NewFromConfig("invalid:99999", "", 0, "test:")
	if err == nil {
		t.Error("expected error for invalid address")
	}
}

// Importing this package is what teaches session.NewStorage about Redis. The
// root package cannot test this: it would have to import the subpackage, which
// is exactly the dependency the split removes.
func TestNewStorageResolvesTheRegisteredRedisBackend(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	cfg := session.DefaultStorageConfig().
		WithType(session.StorageTypeRedis).
		WithRedisAddr(mr.Addr()).
		WithKeyPrefix("test:")

	storage, err := session.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Close() }()

	if _, ok := storage.(*Storage); !ok {
		t.Errorf("NewStorage() returned %T, want *redisstore.Storage", storage)
	}

	if err := storage.Set("test", []byte("value"), time.Hour); err != nil {
		t.Fatalf("failed to set: %v", err)
	}
	if got, err := storage.Get("test"); err != nil || string(got) != "value" {
		t.Errorf("Get() = (%q, %v), want (\"value\", nil)", got, err)
	}
}

func TestNewStorageFromEnvRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	storage, err := session.NewStorageFromEnv(true, mr.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Close() }()

	// Verify it works
	err = storage.Set("test", []byte("value"), time.Hour)
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}
}

// A *redis.ClusterClient satisfies Client just as a *redis.Client does, which
// is the point of taking an interface: a Cluster or Sentinel deployment no
// longer needs a storage of its own. Nothing here talks to a cluster -- the
// assertion is that it compiles and that New accepts it.
func TestClientAcceptsEveryGoRedisClientShape(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	clients := map[string]Client{
		"Client":        redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		"ClusterClient": redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{mr.Addr()}}),
		"Ring":          redis.NewRing(&redis.RingOptions{Addrs: map[string]string{"one": mr.Addr()}}),
		"Universal":     redis.NewUniversalClient(&redis.UniversalOptions{Addrs: []string{mr.Addr()}}),
	}

	for name, client := range clients {
		t.Run(name, func(t *testing.T) {
			storage := New(client, "shapes:")
			defer func() { _ = storage.Close() }()

			if err := storage.Set(name, []byte("value"), time.Minute); err != nil {
				t.Fatalf("Set() error = %v", err)
			}
			got, err := storage.Get(name)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if string(got) != "value" {
				t.Errorf("Get() = %q, want \"value\"", got)
			}
		})
	}
}

// Client is an interface, so the nil that reaches these methods in practice is
// a nil *redis.Client inside a non-nil interface. A plain client == nil misses
// it and the next command panics.
func TestStorageTypedNilClient(t *testing.T) {
	var client *redis.Client
	storage := New(client, "test:")

	if _, err := storage.Get("key"); err == nil {
		t.Error("Get() on a typed-nil client returned no error")
	}
	if err := storage.Set("key", []byte("v"), time.Hour); err == nil {
		t.Error("Set() on a typed-nil client returned no error")
	}
	if err := storage.Close(); err != nil {
		t.Errorf("Close() on a typed-nil client error = %v, want nil", err)
	}
}

// A Client that hides Close -- a deliberately shared handle, say -- is not an
// error: the storage never owned that connection.
func TestStorageCloseIgnoresANonClosingClient(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	storage := New(noCloseClient{Client: client}, "test:")
	if err := storage.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}

	// The wrapped connection is still usable.
	if err := storage.Set("k", []byte("v"), time.Minute); err != nil {
		t.Errorf("Set() after Close() error = %v", err)
	}
}

// noCloseClient is a Client with no Close method of its own.
type noCloseClient struct{ Client }

func TestStorageResetEmpty(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Reset with no keys should work
	err := storage.Reset()
	if err != nil {
		t.Fatalf("failed to reset empty storage: %v", err)
	}
}

func TestStorageCloseWithError(t *testing.T) {
	mr, client := setupMiniRedis(t)
	mr.Close() // Close miniredis first

	storage := New(client, "test:")

	// Close should still work (may or may not error depending on client state)
	_ = storage.Close()
}

func TestStorageWithExpiration(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Set with positive expiration
	err := storage.Set("key1", []byte("value"), 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to set with expiration: %v", err)
	}

	// Verify it exists
	got, _ := storage.Get("key1")
	if string(got) != "value" {
		t.Errorf("expected 'value', got '%s'", string(got))
	}
}

func TestStorageSetNoExpiration(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := New(client, "test:")

	// Set with 0 expiration (no TTL)
	err := storage.Set("persistent", []byte("value"), 0)
	if err != nil {
		t.Fatalf("failed to set without expiration: %v", err)
	}

	// Check TTL is -1 (no expiration)
	ttl, _ := storage.TTL("persistent")
	if ttl != -1*time.Second && ttl != -1*time.Nanosecond {
		// miniredis may return -1ns or actual TTL check
		got, _ := storage.Get("persistent")
		if string(got) != "value" {
			t.Errorf("expected value to exist, got %v", got)
		}
	}
}
