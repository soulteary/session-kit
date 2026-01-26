package session

import (
	"testing"
	"time"
)

func TestDefaultStorageConfig(t *testing.T) {
	cfg := DefaultStorageConfig()

	if cfg.Type != StorageTypeMemory {
		t.Errorf("expected Type to be memory, got %s", cfg.Type)
	}
	if cfg.KeyPrefix != "session:" {
		t.Errorf("expected KeyPrefix to be 'session:', got %s", cfg.KeyPrefix)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("expected RedisAddr to be 'localhost:6379', got %s", cfg.RedisAddr)
	}
	if cfg.RedisPassword != "" {
		t.Errorf("expected RedisPassword to be empty, got %s", cfg.RedisPassword)
	}
	if cfg.RedisDB != 0 {
		t.Errorf("expected RedisDB to be 0, got %d", cfg.RedisDB)
	}
	if cfg.MemoryGCInterval != 10*time.Minute {
		t.Errorf("expected MemoryGCInterval to be 10m, got %v", cfg.MemoryGCInterval)
	}
}

func TestStorageConfigWithMethods(t *testing.T) {
	cfg := DefaultStorageConfig().
		WithType(StorageTypeRedis).
		WithKeyPrefix("myapp:").
		WithRedisAddr("redis.example.com:6379").
		WithRedisPassword("secret").
		WithRedisDB(5).
		WithMemoryGCInterval(5 * time.Minute)

	if cfg.Type != StorageTypeRedis {
		t.Errorf("expected Type to be redis, got %s", cfg.Type)
	}
	if cfg.KeyPrefix != "myapp:" {
		t.Errorf("expected KeyPrefix to be 'myapp:', got %s", cfg.KeyPrefix)
	}
	if cfg.RedisAddr != "redis.example.com:6379" {
		t.Errorf("expected RedisAddr to be 'redis.example.com:6379', got %s", cfg.RedisAddr)
	}
	if cfg.RedisPassword != "secret" {
		t.Errorf("expected RedisPassword to be 'secret', got %s", cfg.RedisPassword)
	}
	if cfg.RedisDB != 5 {
		t.Errorf("expected RedisDB to be 5, got %d", cfg.RedisDB)
	}
	if cfg.MemoryGCInterval != 5*time.Minute {
		t.Errorf("expected MemoryGCInterval to be 5m, got %v", cfg.MemoryGCInterval)
	}
}

func TestNewStorageMemory(t *testing.T) {
	cfg := DefaultStorageConfig().WithType(StorageTypeMemory)

	storage, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to create memory storage: %v", err)
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

func TestNewStorageUnknownType(t *testing.T) {
	cfg := StorageConfig{Type: "unknown"}

	_, err := NewStorage(cfg)
	if err == nil {
		t.Error("expected error for unknown storage type")
	}
}

func TestNewStorageFromEnvMemory(t *testing.T) {
	storage, err := NewStorageFromEnv(false, "", "", 0, "test:")
	if err != nil {
		t.Fatalf("failed to create memory storage: %v", err)
	}
	defer func() { _ = storage.Close() }()

	// Verify it works
	err = storage.Set("test", []byte("value"), time.Hour)
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}
}

func TestMustNewStoragePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid storage config")
		}
	}()

	// This should panic because the storage type is unknown
	cfg := StorageConfig{Type: "invalid"}
	_ = MustNewStorage(cfg)
}

func TestMustNewStorageSuccess(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	cfg := DefaultStorageConfig().WithType(StorageTypeMemory)
	storage := MustNewStorage(cfg)
	defer func() { _ = storage.Close() }()
}
