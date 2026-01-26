package session

import (
	"testing"
	"time"
)

func TestNewSessionData(t *testing.T) {
	id := "test-session-123"
	exp := 1 * time.Hour

	session := NewSessionData(id, exp)

	if session.ID != id {
		t.Errorf("expected ID to be %s, got %s", id, session.ID)
	}
	if session.Authenticated {
		t.Error("expected Authenticated to be false")
	}
	if session.Data == nil {
		t.Error("expected Data to be initialized")
	}
	if session.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if session.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}
	if session.LastAccessedAt.IsZero() {
		t.Error("expected LastAccessedAt to be set")
	}

	// Check expiration is approximately 1 hour from now
	expectedExpiry := time.Now().Add(exp)
	if session.ExpiresAt.Sub(expectedExpiry) > time.Second {
		t.Errorf("expected ExpiresAt to be around %v, got %v", expectedExpiry, session.ExpiresAt)
	}
}

func TestSessionDataIsExpired(t *testing.T) {
	// Create a session that expires in 1 hour
	session := NewSessionData("test", 1*time.Hour)
	if session.IsExpired() {
		t.Error("expected session to not be expired")
	}

	// Create an already expired session
	session.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if !session.IsExpired() {
		t.Error("expected session to be expired")
	}
}

func TestSessionDataIsAuthenticated(t *testing.T) {
	session := NewSessionData("test", 1*time.Hour)

	// Initially not authenticated
	if session.IsAuthenticated() {
		t.Error("expected session to not be authenticated initially")
	}

	// Set authenticated
	session.Authenticated = true
	if !session.IsAuthenticated() {
		t.Error("expected session to be authenticated")
	}

	// Expire the session
	session.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if session.IsAuthenticated() {
		t.Error("expected expired session to not be authenticated")
	}
}

func TestSessionDataTouch(t *testing.T) {
	session := NewSessionData("test", 1*time.Hour)
	originalTime := session.LastAccessedAt

	time.Sleep(10 * time.Millisecond)
	session.Touch()

	if !session.LastAccessedAt.After(originalTime) {
		t.Error("expected LastAccessedAt to be updated")
	}
}

func TestSessionDataValues(t *testing.T) {
	session := NewSessionData("test", 1*time.Hour)

	// Set and get value
	session.SetValue("key1", "value1")
	val, ok := session.GetValue("key1")
	if !ok {
		t.Error("expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}

	// Get non-existent key
	_, ok = session.GetValue("nonexistent")
	if ok {
		t.Error("expected nonexistent key to not exist")
	}

	// Delete value
	session.DeleteValue("key1")
	_, ok = session.GetValue("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestSessionDataAMR(t *testing.T) {
	session := NewSessionData("test", 1*time.Hour)

	// Initially no AMR
	if session.HasAMR("pwd") {
		t.Error("expected no pwd AMR initially")
	}

	// Add AMR
	session.AddAMR("pwd")
	if !session.HasAMR("pwd") {
		t.Error("expected pwd AMR to be added")
	}

	// Add duplicate AMR (should not duplicate)
	session.AddAMR("pwd")
	if len(session.AMR) != 1 {
		t.Errorf("expected 1 AMR, got %d", len(session.AMR))
	}

	// Add another AMR
	session.AddAMR("otp")
	if !session.HasAMR("otp") {
		t.Error("expected otp AMR to be added")
	}
	if len(session.AMR) != 2 {
		t.Errorf("expected 2 AMRs, got %d", len(session.AMR))
	}
}

func TestSessionDataScopes(t *testing.T) {
	session := NewSessionData("test", 1*time.Hour)

	// Initially no scopes
	if session.HasScope("read") {
		t.Error("expected no read scope initially")
	}

	// Add scope
	session.AddScope("read")
	if !session.HasScope("read") {
		t.Error("expected read scope to be added")
	}

	// Add duplicate scope (should not duplicate)
	session.AddScope("read")
	if len(session.Scopes) != 1 {
		t.Errorf("expected 1 scope, got %d", len(session.Scopes))
	}

	// Add another scope
	session.AddScope("write")
	if !session.HasScope("write") {
		t.Error("expected write scope to be added")
	}
	if len(session.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(session.Scopes))
	}
}

func TestSessionDataSetValueNilData(t *testing.T) {
	session := &SessionData{
		ID:   "test",
		Data: nil, // nil data map
	}

	// SetValue should initialize the map
	session.SetValue("key", "value")

	if session.Data == nil {
		t.Error("expected Data map to be initialized")
	}

	val, ok := session.GetValue("key")
	if !ok {
		t.Error("expected key to exist")
	}
	if val != "value" {
		t.Errorf("expected 'value', got %v", val)
	}
}

func TestSessionDataGetValueNilData(t *testing.T) {
	session := &SessionData{
		ID:   "test",
		Data: nil, // nil data map
	}

	// GetValue should return false for nil data map
	_, ok := session.GetValue("key")
	if ok {
		t.Error("expected false for nil data map")
	}
}

func TestSessionDataDeleteValueNilData(t *testing.T) {
	session := &SessionData{
		ID:   "test",
		Data: nil, // nil data map
	}

	// DeleteValue should not panic with nil data map
	session.DeleteValue("key") // Should not panic
}
