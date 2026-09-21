package redisstore

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	session "github.com/soulteary/session-kit/v3"
)

// --- Codex review follow-ups (PR #4) ---

// TestEmptyValueSetDeletesInEveryBackend is the regression test for the
// delete-on-empty-value semantics being applied to MemoryStorage only.
// RedisStorage still ignored an empty value, so code tested against memory
// appeared to clear old authenticated data and left it readable in production.
func TestEmptyValueSetDeletesInEveryBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	backends := map[string]session.Storage{
		"memory": session.NewMemoryStorage("consistency:", 0),
		"redis":  New(client, "consistency:"),
	}

	for name, store := range backends {
		t.Run(name, func(t *testing.T) {
			defer func() { _ = store.Close() }()

			if err := store.Set("sid", []byte("authenticated-payload"), time.Minute); err != nil {
				t.Fatalf("Set() error = %v", err)
			}
			got, err := store.Get("sid")
			if err != nil || string(got) != "authenticated-payload" {
				t.Fatalf("Get() = (%q, %v), want the stored payload", got, err)
			}

			// Overwrite with empty data: the old payload must not survive.
			if err := store.Set("sid", nil, time.Minute); err != nil {
				t.Fatalf("Set(empty) error = %v", err)
			}
			got, err = store.Get("sid")
			if err != nil {
				t.Fatalf("Get() after empty Set error = %v", err)
			}
			if len(got) != 0 {
				t.Errorf("Get() = %q after an empty Set; the old authenticated payload is still readable", got)
			}

			// An empty KEY stays a silent no-op, matching fiber.Storage.
			if err := store.Set("", []byte("x"), time.Minute); err != nil {
				t.Errorf("Set(empty key) error = %v, want nil", err)
			}
		})
	}
}
