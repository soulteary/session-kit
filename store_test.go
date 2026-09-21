package session

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"
)

// memoryStore is an in-memory Store, standing in for a real backend. The
// Redis implementation of the same contract is tested in the redisstore
// package, against miniredis; what is under test here is KVManager, which
// knows nothing about Redis.
type memoryStore struct {
	mu      sync.Mutex
	records map[string]*KVSessionRecord
	nextID  int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: make(map[string]*KVSessionRecord)}
}

func (s *memoryStore) Create(ctx context.Context, data map[string]interface{}, ttl time.Duration) (string, error) {
	s.mu.Lock()
	s.nextID++
	id := "sess_" + string(rune('a'+s.nextID%26)) + string(rune('0'+s.nextID%10))
	s.mu.Unlock()

	if err := s.Set(ctx, id, data, ttl); err != nil {
		return "", err
	}
	return id, nil
}

func (s *memoryStore) Get(_ context.Context, id string) (*KVSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[id]
	if !ok {
		return nil, nil
	}
	if time.Now().After(rec.ExpiresAt) {
		delete(s.records, id)
		return nil, nil
	}

	clone := *rec
	clone.Data = maps.Clone(rec.Data)
	return &clone, nil
}

func (s *memoryStore) Set(_ context.Context, id string, data map[string]interface{}, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	createdAt := now
	if existing, ok := s.records[id]; ok {
		createdAt = existing.CreatedAt
	}
	s.records[id] = &KVSessionRecord{
		ID:        id,
		Data:      maps.Clone(data),
		CreatedAt: createdAt,
		ExpiresAt: now.Add(ttl),
	}
	return nil
}

func (s *memoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.records, id)
	return nil
}

func (s *memoryStore) Exists(ctx context.Context, id string) (bool, error) {
	rec, err := s.Get(ctx, id)
	return rec != nil, err
}

var _ Store = (*memoryStore)(nil)

func TestKVManager_CreateGetRefresh(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	mgr := NewKVManager(store, 5*time.Minute)

	id, err := mgr.Create(ctx, map[string]interface{}{"x": "y"}, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Error("Create returned empty id")
	}

	rec, err := mgr.Get(ctx, id)
	if err != nil || rec == nil || rec.Data["x"] != "y" {
		t.Errorf("Get: err=%v rec=%v", err, rec)
	}

	if err := mgr.Refresh(ctx, id, 10*time.Minute); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	rec2, err := mgr.Get(ctx, id)
	if err != nil || rec2 == nil {
		t.Errorf("after Refresh Get: err=%v rec=%v", err, rec2)
	}
}

func TestKVManager_Set(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	mgr := NewKVManager(store, 5*time.Minute)

	id, err := mgr.Create(ctx, map[string]interface{}{"a": "1"}, 10*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Set with explicit TTL
	err = mgr.Set(ctx, id, map[string]interface{}{"a": "2"}, 15*time.Minute)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	rec, err := mgr.Get(ctx, id)
	if err != nil || rec == nil || rec.Data["a"] != "2" {
		t.Errorf("after Set: err=%v rec=%v", err, rec)
	}

	// Set with 0 TTL (uses defaultTTL)
	err = mgr.Set(ctx, id, map[string]interface{}{"a": "3"}, 0)
	if err != nil {
		t.Fatalf("Set with 0 ttl: %v", err)
	}
	rec2, err := mgr.Get(ctx, id)
	if err != nil || rec2 == nil || rec2.Data["a"] != "3" {
		t.Errorf("after Set(0 ttl): err=%v rec=%v", err, rec2)
	}
}

func TestKVManager_Delete(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	mgr := NewKVManager(store, 5*time.Minute)

	id, err := mgr.Create(ctx, map[string]interface{}{"k": "v"}, 10*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = mgr.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rec, err := mgr.Get(ctx, id)
	if err != nil || rec != nil {
		t.Errorf("after Delete: err=%v rec=%v", err, rec)
	}
	ok, _ := mgr.Exists(ctx, id)
	if ok {
		t.Error("Exists after Delete should be false")
	}
}

func TestKVManager_Exists(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	mgr := NewKVManager(store, 5*time.Minute)

	ok, err := mgr.Exists(ctx, "nonexistent")
	if err != nil || ok {
		t.Errorf("Exists(nonexistent): err=%v ok=%v", err, ok)
	}

	id, err := mgr.Create(ctx, map[string]interface{}{"x": "y"}, 10*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ok, err = mgr.Exists(ctx, id)
	if err != nil || !ok {
		t.Errorf("Exists(created): err=%v ok=%v", err, ok)
	}
}

func TestKVManager_RefreshNotFound(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	mgr := NewKVManager(store, 5*time.Minute)

	// Refresh on non-existent id: Get returns (nil, nil), so Refresh returns nil (no error)
	err := mgr.Refresh(ctx, "nonexistent-id", 10*time.Minute)
	if err != nil {
		t.Errorf("Refresh on non-existent id returns nil in current impl: %v", err)
	}
}

func TestKVManager_RefreshWithZeroTTL(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	mgr := NewKVManager(store, 5*time.Minute)

	id, err := mgr.Create(ctx, map[string]interface{}{"k": "v"}, 10*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	err = mgr.Refresh(ctx, id, 0)
	if err != nil {
		t.Fatalf("Refresh with 0 ttl: %v", err)
	}
	rec, err := mgr.Get(ctx, id)
	if err != nil || rec == nil {
		t.Errorf("after Refresh(0): err=%v rec=%v", err, rec)
	}
}

// failingStore implements Store and returns configurable errors for testing.
type failingStore struct {
	getErr error
	Store  Store
}

func (f *failingStore) Create(ctx context.Context, data map[string]interface{}, ttl time.Duration) (string, error) {
	return f.Store.Create(ctx, data, ttl)
}

func (f *failingStore) Get(ctx context.Context, id string) (*KVSessionRecord, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.Store.Get(ctx, id)
}

func (f *failingStore) Set(ctx context.Context, id string, data map[string]interface{}, ttl time.Duration) error {
	return f.Store.Set(ctx, id, data, ttl)
}

func (f *failingStore) Delete(ctx context.Context, id string) error {
	return f.Store.Delete(ctx, id)
}

func (f *failingStore) Exists(ctx context.Context, id string) (bool, error) {
	return f.Store.Exists(ctx, id)
}

func TestKVManager_RefreshGetError(t *testing.T) {
	ctx := context.Background()
	base := newMemoryStore()
	wrapped := &failingStore{Store: base, getErr: errors.New("get failed")}
	mgr := NewKVManager(wrapped, 5*time.Minute)

	// Refresh when Get returns error should propagate error
	err := mgr.Refresh(ctx, "any-id", 10*time.Minute)
	if err == nil {
		t.Error("expected error when Get fails in Refresh")
	}
}
