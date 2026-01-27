package session

import (
	"context"
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
