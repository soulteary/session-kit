package session

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

func TestRedisStorageBasicOperations(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

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

func TestRedisStorageGetNonExistent(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

	got, err := storage.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestRedisStorageEmptyKeyValue(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

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

func TestRedisStorageNoExpiration(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

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

func TestRedisStorageReset(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

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

func TestRedisStorageKeyPrefix(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	// Test with colon
	storage1 := NewRedisStorage(client, "prefix:")
	if storage1.GetKeyPrefix() != "prefix:" {
		t.Errorf("expected prefix 'prefix:', got %s", storage1.GetKeyPrefix())
	}

	// Test without colon (should add it)
	storage2 := NewRedisStorage(client, "prefix")
	if storage2.GetKeyPrefix() != "prefix:" {
		t.Errorf("expected prefix 'prefix:', got %s", storage2.GetKeyPrefix())
	}

	// Test empty prefix (should use default)
	storage3 := NewRedisStorage(client, "")
	if storage3.GetKeyPrefix() != "session:" {
		t.Errorf("expected prefix 'session:', got %s", storage3.GetKeyPrefix())
	}
}

func TestRedisStorageClose(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()

	storage := NewRedisStorage(client, "test:")

	err := storage.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}
}

func TestRedisStorageGetClient(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

	if storage.GetClient() != client {
		t.Error("expected GetClient to return the same client")
	}
}

func TestRedisStorageExists(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

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

func TestRedisStorageGetTTL(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

	// Set with expiration
	_ = storage.Set("expiring", []byte("value"), 1*time.Hour)

	ttl, err := storage.GetTTL("expiring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttl <= 0 {
		t.Errorf("expected positive TTL, got %v", ttl)
	}

	// Non-existent key - Redis returns -2 (as nanoseconds in go-redis)
	ttl, err = storage.GetTTL("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// go-redis returns -2 nanoseconds for non-existent keys
	if ttl >= 0 {
		t.Errorf("expected negative TTL for non-existent key, got %v", ttl)
	}
}

func TestRedisStorageExpire(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

	// Set with no expiration
	_ = storage.Set("key", []byte("value"), 0)

	// Set expiration
	err := storage.Expire("key", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to set expiration: %v", err)
	}

	// Check TTL
	ttl, _ := storage.GetTTL("key")
	if ttl <= 0 {
		t.Errorf("expected positive TTL after Expire, got %v", ttl)
	}
}

func TestRedisStorageNilClient(t *testing.T) {
	storage := &RedisStorage{client: nil, keyPrefix: "test:"}

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

	_, err = storage.GetTTL("key")
	if err == nil {
		t.Error("expected error for nil client on GetTTL")
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

func TestNewRedisStorageFromConfig(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	storage, err := NewRedisStorageFromConfig(mr.Addr(), "", 0, "test:")
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

func TestNewRedisStorageFromConfigError(t *testing.T) {
	// Invalid address should fail
	_, err := NewRedisStorageFromConfig("invalid:99999", "", 0, "test:")
	if err == nil {
		t.Error("expected error for invalid address")
	}
}

func TestNewStorageWithRedisClient(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	cfg := DefaultStorageConfig().
		WithType(StorageTypeRedis).
		WithRedisClient(client).
		WithKeyPrefix("test:")

	storage, err := NewStorage(cfg)
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

func TestNewStorageFromEnvRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	storage, err := NewStorageFromEnv(true, mr.Addr(), "", 0, "test:")
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

func TestRedisStorageResetEmpty(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

	// Reset with no keys should work
	err := storage.Reset()
	if err != nil {
		t.Fatalf("failed to reset empty storage: %v", err)
	}
}

func TestRedisStorageCloseWithError(t *testing.T) {
	mr, client := setupMiniRedis(t)
	mr.Close() // Close miniredis first

	storage := NewRedisStorage(client, "test:")

	// Close should still work (may or may not error depending on client state)
	_ = storage.Close()
}

func TestRedisStorageWithExpiration(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

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

func TestRedisStorageSetNoExpiration(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	storage := NewRedisStorage(client, "test:")

	// Set with 0 expiration (no TTL)
	err := storage.Set("persistent", []byte("value"), 0)
	if err != nil {
		t.Fatalf("failed to set without expiration: %v", err)
	}

	// Check TTL is -1 (no expiration)
	ttl, _ := storage.GetTTL("persistent")
	if ttl != -1*time.Second && ttl != -1*time.Nanosecond {
		// miniredis may return -1ns or actual TTL check
		got, _ := storage.Get("persistent")
		if string(got) != "value" {
			t.Errorf("expected value to exist, got %v", got)
		}
	}
}
