package redisstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	session "github.com/soulteary/session-kit/v3"
)

func TestStore_CreateGetSetDeleteExists(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewStore(client, "kv:")
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

func TestStore_NilClient(t *testing.T) {
	ctx := context.Background()
	store := NewStore(nil, "kv:")

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

func TestStore_GetInvalidJSON(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewStore(client, "bad:")
	// key = keyPrefix + id => "bad:" + "sess_invalid"
	if err := client.Set(ctx, "bad:sess_invalid", []byte("not json"), time.Minute).Err(); err != nil {
		t.Fatalf("set raw value: %v", err)
	}

	_, err := store.Get(ctx, "sess_invalid")
	if err == nil {
		t.Error("expected error for invalid JSON in Get")
	}
}

func TestStore_GetExpiredRecord(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewStore(client, "exp:")
	id := "sess_expired"
	rec := &session.KVSessionRecord{
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

func TestStore_KeyPrefixWithoutColon(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	// Prefix without trailing colon: NewRedisStore should append ":"
	store := NewStore(client, "myprefix")
	id, err := store.Create(ctx, map[string]interface{}{"a": "b"}, 10*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec, err := store.Get(ctx, id)
	if err != nil || rec == nil || rec.Data["a"] != "b" {
		t.Errorf("Get after Create: err=%v rec=%v", err, rec)
	}
}

func TestStore_EmptyKeyPrefix(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewStore(client, "") // empty prefix: key(id) = id

	id, err := store.Create(ctx, map[string]interface{}{"k": "v"}, 10*time.Minute)
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
	if rec == nil || rec.Data["k"] != "v" {
		t.Errorf("Get: rec=%v", rec)
	}

	ok, err := store.Exists(ctx, id)
	if err != nil || !ok {
		t.Errorf("Exists: err=%v ok=%v", err, ok)
	}
}
