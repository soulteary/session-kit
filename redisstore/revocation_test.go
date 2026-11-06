package redisstore

import (
	"errors"
	"testing"
	"time"

	session "github.com/soulteary/session-kit/v3"
)

func TestRedisAtomicRevocationAndDeadline(t *testing.T) {
	server, client := setupMiniRedis(t)
	defer server.Close()
	defer func() { _ = client.Close() }()
	s := New(client, "revocable:")
	id := session.NewRevocableSessionID(time.Hour)
	if err := s.SetSessionIfActive(id, []byte("authenticated"), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	payload, err := s.GetSession(id)
	if err != nil || string(payload) != "authenticated" {
		t.Fatalf("read: %q %v", payload, err)
	}
	if ttl := server.TTL(s.buildKey(id)); ttl > time.Hour || ttl <= 0 {
		t.Fatalf("invalid bounded TTL %s", ttl)
	}
	other := New(client, "revocable:")
	if err := other.RevokeSession(id); err != nil {
		t.Fatal(err)
	}
	if payload, err := s.GetSession(id); err != nil || payload != nil {
		t.Fatal("revoked session remained readable")
	}
	if err := s.SetSessionIfActive(id, []byte("stale authenticated snapshot"), time.Hour); !errors.Is(err, session.ErrSessionRevoked) {
		t.Fatalf("revived session: %v", err)
	}
	future := session.NewRevocableSessionID(time.Hour)
	server.SetTime(time.Now().Add(2 * time.Hour))
	if err := s.SetSessionIfActive(future, []byte("late snapshot"), time.Hour); !errors.Is(err, session.ErrSessionExpired) {
		t.Fatalf("Redis server deadline was ignored: %v", err)
	}
	if err := s.RevokeSession(future); err != nil {
		t.Fatal(err)
	}
	server.SetTime(time.Now())
	expired := session.NewRevocableSessionID(-time.Second)
	if err := s.SetSessionIfActive(expired, []byte("late"), time.Hour); !errors.Is(err, session.ErrSessionExpired) {
		t.Fatal(err)
	}
	if payload, err := s.GetSession(expired); err != nil || payload != nil {
		t.Fatal(err)
	}
	if payload, err := s.GetSession("legacy"); err != nil || payload != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionIfActive("legacy", []byte("x"), time.Hour); !errors.Is(err, session.ErrSessionEncoding) {
		t.Fatal(err)
	}
	if err := s.RevokeSession("legacy"); !errors.Is(err, session.ErrSessionEncoding) {
		t.Fatal(err)
	}
	fresh := session.NewRevocableSessionID(time.Hour)
	if err := s.SetSessionIfActive(fresh, []byte("x"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if ttl := server.TTL(s.buildKey(fresh)); ttl > time.Minute || ttl <= 0 {
		t.Fatal(ttl)
	}
	if err := s.SetSessionIfActive(fresh, nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	bad := session.NewRevocableSessionID(time.Hour)
	if err := s.Set(bad, []byte("legacy payload"), time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(bad); !errors.Is(err, session.ErrSessionEncoding) {
		t.Fatal(err)
	}
	if err := s.SetSessionIfActive(bad, []byte("x"), time.Hour); !errors.Is(err, session.ErrSessionEncoding) {
		t.Fatal(err)
	}
	missing := New(nil, "missing:")
	if err := missing.SetSessionIfActive(id, []byte("x"), time.Hour); err == nil {
		t.Fatal("nil client accepted")
	}
	if err := missing.RevokeSession(id); err == nil {
		t.Fatal("nil client accepted")
	}
	server.Close()
	if _, err := s.GetSession(id); err == nil {
		t.Fatal("backend outage accepted")
	}
	if err := s.SetSessionIfActive(id, []byte("x"), time.Hour); err == nil {
		t.Fatal("backend outage accepted")
	}
	if err := s.RevokeSession(id); err == nil {
		t.Fatal("backend outage accepted")
	}
}
