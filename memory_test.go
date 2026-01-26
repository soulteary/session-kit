package session

import (
	"testing"
	"time"
)

func TestMemoryStorageBasicOperations(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

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

func TestMemoryStorageGetNonExistent(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	got, err := storage.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestMemoryStorageEmptyKeyValue(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

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

func TestMemoryStorageExpiration(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	key := "expiring"
	value := []byte("test")

	// Set with very short expiration
	err := storage.Set(key, value, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	// Should exist immediately
	got, err := storage.Get(key)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if got == nil {
		t.Error("expected value to exist")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired now
	got, err = storage.Get(key)
	if err != nil {
		t.Fatalf("failed to get after expiration: %v", err)
	}
	if got != nil {
		t.Error("expected value to be expired")
	}
}

func TestMemoryStorageNoExpiration(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

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

func TestMemoryStorageReset(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	// Add some data
	_ = storage.Set("key1", []byte("value1"), time.Hour)
	_ = storage.Set("key2", []byte("value2"), time.Hour)

	if storage.Len() != 2 {
		t.Errorf("expected 2 entries, got %d", storage.Len())
	}

	// Reset
	err := storage.Reset()
	if err != nil {
		t.Fatalf("failed to reset: %v", err)
	}

	if storage.Len() != 0 {
		t.Errorf("expected 0 entries after reset, got %d", storage.Len())
	}
}

func TestMemoryStorageKeyPrefix(t *testing.T) {
	// Test with colon
	storage1 := NewMemoryStorage("prefix:", 0)
	defer func() { _ = storage1.Close() }()

	// Test without colon (should add it)
	storage2 := NewMemoryStorage("prefix", 0)
	defer func() { _ = storage2.Close() }()

	// Test empty prefix (should use default)
	storage3 := NewMemoryStorage("", 0)
	defer func() { _ = storage3.Close() }()

	// Set a value and verify the key is prefixed correctly
	_ = storage1.Set("key", []byte("value"), time.Hour)
	if storage1.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", storage1.Len())
	}
}

func TestMemoryStorageGC(t *testing.T) {
	// Create storage with short GC interval
	storage := NewMemoryStorage("test:", 50*time.Millisecond)
	defer func() { _ = storage.Close() }()

	// Add an entry that expires quickly
	_ = storage.Set("expiring", []byte("value"), 25*time.Millisecond)

	// Wait for GC to run
	time.Sleep(100 * time.Millisecond)

	// The entry should be cleaned up
	if storage.Len() != 0 {
		t.Errorf("expected 0 entries after GC, got %d", storage.Len())
	}
}

func TestMemoryStorageDataIsolation(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	// Set original value
	original := []byte("original")
	_ = storage.Set("key", original, time.Hour)

	// Modify the original slice
	original[0] = 'X'

	// Get should return the original value, not the modified one
	got, _ := storage.Get("key")
	if got[0] == 'X' {
		t.Error("expected data to be isolated from original slice")
	}
}
