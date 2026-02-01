package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestRedisStore_CreateGetSetDeleteExists(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "kv:")
	ttl := 10 * time.Minute

	id, err := store.Create(ctx, map[string]interface{}{"k": "v1"}, ttl)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" || id[:5] != "sess_" {
		t.Errorf("expected sess_ prefix, got %q", id)
	}

	rec, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec == nil {
		t.Fatal("Get returned nil")
	}
	if rec.ID != id || rec.Data["k"] != "v1" {
		t.Errorf("Get: id=%q data[k]=%v", rec.ID, rec.Data["k"])
	}

	ok, err := store.Exists(ctx, id)
	if err != nil || !ok {
		t.Errorf("Exists: err=%v ok=%v", err, ok)
	}

	if err := store.Set(ctx, id, map[string]interface{}{"k": "v2"}, ttl); err != nil {
		t.Fatalf("Set: %v", err)
	}
	rec2, err := store.Get(ctx, id)
	if err != nil || rec2 == nil || rec2.Data["k"] != "v2" {
		t.Errorf("after Set: err=%v rec=%v", err, rec2)
	}

	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rec3, err := store.Get(ctx, id)
	if err != nil || rec3 != nil {
		t.Errorf("after Delete: err=%v rec=%v", err, rec3)
	}
	ok2, _ := store.Exists(ctx, id)
	if ok2 {
		t.Error("Exists after Delete should be false")
	}
}

func TestKVManager_CreateGetRefresh(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "mgr:")
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
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "set:")
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
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "del:")
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
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "exists:")
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
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "refresh:")
	mgr := NewKVManager(store, 5*time.Minute)

	// Refresh on non-existent id: Get returns (nil, nil), so Refresh returns nil (no error)
	err := mgr.Refresh(ctx, "nonexistent-id", 10*time.Minute)
	if err != nil {
		t.Errorf("Refresh on non-existent id returns nil in current impl: %v", err)
	}
}

func TestKVManager_RefreshWithZeroTTL(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "refresh0:")
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

func TestRedisStore_NilClient(t *testing.T) {
	ctx := context.Background()
	store := NewRedisStore(nil, "kv:")

	_, err := store.Create(ctx, map[string]interface{}{"k": "v"}, time.Minute)
	if err == nil {
		t.Error("expected error for nil client on Create")
	}

	_, err = store.Get(ctx, "sess_abc")
	if err == nil {
		t.Error("expected error for nil client on Get")
	}

	err = store.Set(ctx, "sess_abc", map[string]interface{}{"k": "v"}, time.Minute)
	if err == nil {
		t.Error("expected error for nil client on Set")
	}

	err = store.Delete(ctx, "sess_abc")
	if err == nil {
		t.Error("expected error for nil client on Delete")
	}

	_, err = store.Exists(ctx, "sess_abc")
	if err == nil {
		t.Error("expected error for nil client on Exists")
	}
}

func TestRedisStore_GetInvalidJSON(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "bad:")
	// key = keyPrefix + id => "bad:" + "sess_invalid"
	if err := client.Set(ctx, "bad:sess_invalid", []byte("not json"), time.Minute).Err(); err != nil {
		t.Fatalf("set raw value: %v", err)
	}

	_, err := store.Get(ctx, "sess_invalid")
	if err == nil {
		t.Error("expected error for invalid JSON in Get")
	}
}

func TestRedisStore_GetExpiredRecord(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisStore(client, "exp:")
	id := "sess_expired"
	rec := &KVSessionRecord{
		ID:        id,
		Data:      map[string]interface{}{"k": "v"},
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// key = "exp:" + id
	if err := client.Set(ctx, "exp:"+id, data, time.Minute).Err(); err != nil {
		t.Fatalf("set expired record: %v", err)
	}

	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Error("expected nil for expired record")
	}
}
