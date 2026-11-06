package session

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRevocableIdentifiersAndEncoding(t *testing.T) {
	id := NewRevocableSessionID(time.Hour)
	deadline, err := RevocableSessionDeadline(id)
	if err != nil || !deadline.After(time.Now()) || deadline.After(time.Now().Add(time.Hour)) {
		t.Fatalf("invalid generated deadline: %v %v", deadline, err)
	}
	for _, id := range []string{"", "r1.a.x", "r2.a." + strings.Repeat("A", 26), "r1.bad!." + strings.Repeat("A", 26), "r1.0." + strings.Repeat("A", 26), "r1.a." + strings.Repeat("8", 26), "r1.a." + strings.Repeat("A", 129)} {
		if _, err := RevocableSessionDeadline(id); !errors.Is(err, ErrSessionEncoding) {
			t.Fatalf("accepted identifier %q", id)
		}
	}
	for _, value := range [][]byte{nil, []byte(RevokedSessionValue)} {
		got, err := DecodeActiveSession(value)
		if err != nil || got != nil {
			t.Fatalf("decoded absent/revoked value: %q %v", got, err)
		}
	}
	if _, err := DecodeActiveSession([]byte("legacy")); !errors.Is(err, ErrSessionEncoding) {
		t.Fatal(err)
	}
	input := []byte(ActiveSessionPrefix + "payload")
	got, err := DecodeActiveSession(input)
	if err != nil || string(got) != "payload" {
		t.Fatal(err)
	}
	got[0] = 'X'
	if string(input) != ActiveSessionPrefix+"payload" {
		t.Fatal("aliased payload")
	}
}

func TestMemoryRevocationSurvivesDelayedSave(t *testing.T) {
	s := NewMemoryStorage("revocable:", 0)
	defer func() { _ = s.Close() }()
	id := NewRevocableSessionID(time.Hour)
	if err := s.SetSessionIfActive(id, []byte("authenticated"), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	payload, err := s.GetSession(id)
	if err != nil || string(payload) != "authenticated" {
		t.Fatalf("read: %q %v", payload, err)
	}
	deadline, _ := RevocableSessionDeadline(id)
	if !s.data[s.buildKey(id)].expiresAt.Equal(deadline) {
		t.Fatal("storage outlived immutable identifier deadline")
	}
	if err := s.RevokeSession(id); err != nil {
		t.Fatal(err)
	}
	if payload, err := s.GetSession(id); err != nil || payload != nil {
		t.Fatal("revoked session remained readable")
	}
	if err := s.SetSessionIfActive(id, []byte("stale authenticated snapshot"), time.Hour); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("revived session: %v", err)
	}
	if !s.data[s.buildKey(id)].expiresAt.Equal(deadline) {
		t.Fatal("revocation retention changed")
	}
	expired := NewRevocableSessionID(-time.Second)
	if err := s.SetSessionIfActive(expired, []byte("late snapshot"), time.Hour); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired snapshot written: %v", err)
	}
	if err := s.RevokeSession(expired); err != nil {
		t.Fatal(err)
	}
	if payload, err := s.GetSession(expired); err != nil || payload != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession("legacy"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionIfActive("legacy", []byte("x"), time.Hour); !errors.Is(err, ErrSessionEncoding) {
		t.Fatal(err)
	}
	if err := s.RevokeSession("legacy"); !errors.Is(err, ErrSessionEncoding) {
		t.Fatal(err)
	}
	fresh := NewRevocableSessionID(time.Hour)
	if err := s.SetSessionIfActive(fresh, []byte("x"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if !s.data[s.buildKey(fresh)].expiresAt.Before(deadline) {
		t.Fatal("idle TTL was not honored")
	}
	if err := s.SetSessionIfActive(fresh, nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	bad := NewRevocableSessionID(time.Hour)
	if err := s.Set(bad, []byte("legacy data"), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionIfActive(bad, []byte("x"), time.Hour); !errors.Is(err, ErrSessionEncoding) {
		t.Fatal(err)
	}
	if _, err := s.GetSession(bad); !errors.Is(err, ErrSessionEncoding) {
		t.Fatal(err)
	}
}
