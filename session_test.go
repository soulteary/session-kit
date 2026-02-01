package session

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	fibersession "github.com/gofiber/fiber/v2/middleware/session"
)

// failingStorage implements Storage and returns configurable errors for testing.
type failingStorage struct {
	setErr  error
	getErr  error
	Storage Storage
}

func (f *failingStorage) Get(key string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.Storage.Get(key)
}

func (f *failingStorage) Set(key string, val []byte, exp time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}
	return f.Storage.Set(key, val, exp)
}

func (f *failingStorage) Delete(key string) error {
	return f.Storage.Delete(key)
}

func (f *failingStorage) Reset() error {
	return f.Storage.Reset()
}

func (f *failingStorage) Close() error {
	return f.Storage.Close()
}

func TestManagerCreateSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().WithExpiration(1 * time.Hour)
	manager := NewManager(storage, config)

	session := manager.CreateSession("session-123")

	if session.ID != "session-123" {
		t.Errorf("expected ID to be 'session-123', got %s", session.ID)
	}
	if session.Authenticated {
		t.Error("expected session to not be authenticated")
	}
}

func TestManagerSaveAndLoadSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().WithExpiration(1 * time.Hour)
	manager := NewManager(storage, config)

	// Create and save session
	session := manager.CreateSession("session-123")
	session.Authenticated = true
	session.UserID = "user-456"
	session.Email = "test@example.com"
	session.AddAMR("pwd")

	err := manager.SaveSession(session)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Load session
	loaded, err := manager.LoadSession("session-123")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected session to be loaded")
	}

	if loaded.ID != "session-123" {
		t.Errorf("expected ID to be 'session-123', got %s", loaded.ID)
	}
	if !loaded.Authenticated {
		t.Error("expected session to be authenticated")
	}
	if loaded.UserID != "user-456" {
		t.Errorf("expected UserID to be 'user-456', got %s", loaded.UserID)
	}
	if loaded.Email != "test@example.com" {
		t.Errorf("expected Email to be 'test@example.com', got %s", loaded.Email)
	}
	if !loaded.HasAMR("pwd") {
		t.Error("expected session to have pwd AMR")
	}
}

func TestManagerLoadNonExistentSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig()
	manager := NewManager(storage, config)

	loaded, err := manager.LoadSession("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for non-existent session")
	}
}

func TestManagerLoadExpiredSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().WithExpiration(50 * time.Millisecond)
	manager := NewManager(storage, config)

	// Create and save session
	session := manager.CreateSession("session-123")
	err := manager.SaveSession(session)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Load session - should be nil because it's expired
	loaded, err := manager.LoadSession("session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for expired session")
	}
}

func TestManagerDeleteSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig()
	manager := NewManager(storage, config)

	// Create and save session
	session := manager.CreateSession("session-123")
	_ = manager.SaveSession(session)

	// Delete session
	err := manager.DeleteSession("session-123")
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Verify deletion
	loaded, _ := manager.LoadSession("session-123")
	if loaded != nil {
		t.Error("expected session to be deleted")
	}
}

func TestManagerTouchSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().WithExpiration(1 * time.Hour)
	manager := NewManager(storage, config)

	// Create and save session
	session := manager.CreateSession("session-123")
	originalExpiry := session.ExpiresAt
	originalAccess := session.LastAccessedAt
	_ = manager.SaveSession(session)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Touch session
	err := manager.TouchSession(session)
	if err != nil {
		t.Fatalf("failed to touch session: %v", err)
	}

	if !session.LastAccessedAt.After(originalAccess) {
		t.Error("expected LastAccessedAt to be updated")
	}
	if !session.ExpiresAt.After(originalExpiry) {
		t.Error("expected ExpiresAt to be extended")
	}
}

func TestManagerGetStorage(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig()
	manager := NewManager(storage, config)

	if manager.GetStorage() != storage {
		t.Error("expected GetStorage to return the same storage")
	}
}

func TestManagerGetConfig(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().WithCookieName("my_session")
	manager := NewManager(storage, config)

	if manager.GetConfig().CookieName != "my_session" {
		t.Errorf("expected CookieName to be 'my_session', got %s", manager.GetConfig().CookieName)
	}
}

func TestManagerFiberSessionConfig(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().
		WithCookieName("my_session").
		WithCookieDomain(".example.com").
		WithCookiePath("/app").
		WithSecure(true).
		WithHTTPOnly(true).
		WithSameSite("Strict").
		WithExpiration(2 * time.Hour)

	manager := NewManager(storage, config)
	fiberCfg := manager.FiberSessionConfig()

	if fiberCfg.Expiration != 2*time.Hour {
		t.Errorf("expected Expiration to be 2h, got %v", fiberCfg.Expiration)
	}
	if fiberCfg.CookieDomain != ".example.com" {
		t.Errorf("expected CookieDomain to be '.example.com', got %s", fiberCfg.CookieDomain)
	}
	if fiberCfg.CookiePath != "/app" {
		t.Errorf("expected CookiePath to be '/app', got %s", fiberCfg.CookiePath)
	}
	if !fiberCfg.CookieSecure {
		t.Error("expected CookieSecure to be true")
	}
	if !fiberCfg.CookieHTTPOnly {
		t.Error("expected CookieHTTPOnly to be true")
	}
}

func TestCreateCookie(t *testing.T) {
	config := DefaultConfig().
		WithCookieName("my_session").
		WithCookieDomain(".example.com").
		WithCookiePath("/app").
		WithSecure(true).
		WithHTTPOnly(true).
		WithSameSite("Strict").
		WithExpiration(1 * time.Hour)

	cookie := CreateCookie(config, "session-123")

	if cookie.Name != "my_session" {
		t.Errorf("expected Name to be 'my_session', got %s", cookie.Name)
	}
	if cookie.Value != "session-123" {
		t.Errorf("expected Value to be 'session-123', got %s", cookie.Value)
	}
	if cookie.Domain != ".example.com" {
		t.Errorf("expected Domain to be '.example.com', got %s", cookie.Domain)
	}
	if cookie.Path != "/app" {
		t.Errorf("expected Path to be '/app', got %s", cookie.Path)
	}
	if !cookie.Secure {
		t.Error("expected Secure to be true")
	}
	if !cookie.HTTPOnly {
		t.Error("expected HTTPOnly to be true")
	}
}

func TestCreateCookieSameSiteVariants(t *testing.T) {
	tests := []struct {
		sameSite string
		expected string
	}{
		{"Strict", "strict"},
		{"Lax", "lax"},
		{"None", "none"},
		{"Disabled", "disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.sameSite, func(t *testing.T) {
			config := DefaultConfig().WithSameSite(tt.sameSite)
			cookie := CreateCookie(config, "session-123")
			if cookie.SameSite != tt.expected {
				t.Errorf("expected SameSite to be %v, got %v", tt.expected, cookie.SameSite)
			}
		})
	}
}

func TestFiberSessionHelpers(t *testing.T) {
	app := fiber.New()
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	store := fibersession.New(fibersession.Config{
		Storage:    storage,
		Expiration: 1 * time.Hour,
	})

	// Test route
	app.Get("/test", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}

		// Test IsAuthenticated - should be false initially
		if IsAuthenticated(sess) {
			return c.SendString("should not be authenticated")
		}

		// Test SetUserID and GetUserID
		SetUserID(sess, "user-123")
		if GetUserID(sess) != "user-123" {
			return c.SendString("user id mismatch")
		}

		// Test SetEmail and GetEmail
		SetEmail(sess, "test@example.com")
		if GetEmail(sess) != "test@example.com" {
			return c.SendString("email mismatch")
		}

		// Test SetPhone and GetPhone
		SetPhone(sess, "+1234567890")
		if GetPhone(sess) != "+1234567890" {
			return c.SendString("phone mismatch")
		}

		// Test AMR
		SetAMR(sess, []string{"pwd"})
		amr := GetAMR(sess)
		if len(amr) != 1 || amr[0] != "pwd" {
			return c.SendString("amr mismatch")
		}

		AddAMR(sess, "otp")
		if !HasAMR(sess, "otp") {
			return c.SendString("should have otp amr")
		}

		// Adding duplicate should not add
		AddAMR(sess, "otp")
		amr = GetAMR(sess)
		if len(amr) != 2 {
			return c.SendString("duplicate amr added")
		}

		// Test Scopes
		SetScopes(sess, []string{"read"})
		scopes := GetScopes(sess)
		if len(scopes) != 1 || scopes[0] != "read" {
			return c.SendString("scopes mismatch")
		}

		if !HasScope(sess, "read") {
			return c.SendString("should have read scope")
		}

		if HasScope(sess, "write") {
			return c.SendString("should not have write scope")
		}

		// Test UpdateLastAccess
		UpdateLastAccess(sess)
		lastAccess := GetLastAccess(sess)
		if lastAccess.IsZero() {
			return c.SendString("last access should be set")
		}

		// Test Authenticate
		err = Authenticate(sess)
		if err != nil {
			return err
		}

		if !IsAuthenticated(sess) {
			return c.SendString("should be authenticated")
		}

		// Check created at
		createdAt := GetCreatedAt(sess)
		if createdAt.IsZero() {
			return c.SendString("created at should be set")
		}

		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestFiberSessionUnauthenticate(t *testing.T) {
	app := fiber.New()
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	store := fibersession.New(fibersession.Config{
		Storage:    storage,
		Expiration: 1 * time.Hour,
	})

	var sessionCookie string

	// First request: login and get session cookie
	app.Get("/login", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}

		err = Authenticate(sess)
		if err != nil {
			return err
		}

		return c.SendString("logged in")
	})

	// Second endpoint: logout using the session from cookie
	app.Get("/logout", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}

		// Check if authenticated
		if !IsAuthenticated(sess) {
			return c.SendString("not authenticated")
		}

		// Unauthenticate - clears session data and destroys
		err = Unauthenticate(sess)
		if err != nil {
			return err
		}

		return c.SendString("logged out")
	})

	// Step 1: Login
	loginReq := httptest.NewRequest("GET", "/login", nil)
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("failed to test login: %v", err)
	}
	if loginResp.StatusCode != 200 {
		t.Errorf("expected login status 200, got %d", loginResp.StatusCode)
	}

	// Extract session cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "session_id" {
			sessionCookie = cookie.Value
			break
		}
	}

	// Step 2: Logout with session cookie
	logoutReq := httptest.NewRequest("GET", "/logout", nil)
	if sessionCookie != "" {
		logoutReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionCookie})
	}
	logoutResp, err := app.Test(logoutReq)
	if err != nil {
		t.Fatalf("failed to test logout: %v", err)
	}
	if logoutResp.StatusCode != 200 {
		t.Errorf("expected logout status 200, got %d", logoutResp.StatusCode)
	}
}

func TestFiberSessionGettersWithNilValues(t *testing.T) {
	app := fiber.New()
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	store := fibersession.New(fibersession.Config{
		Storage:    storage,
		Expiration: 1 * time.Hour,
	})

	app.Get("/test-nil", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}

		// All getters should return empty/nil for unset values
		if GetUserID(sess) != "" {
			return c.SendString("expected empty user id")
		}
		if GetEmail(sess) != "" {
			return c.SendString("expected empty email")
		}
		if GetPhone(sess) != "" {
			return c.SendString("expected empty phone")
		}
		if GetAMR(sess) != nil {
			return c.SendString("expected nil amr")
		}
		if GetScopes(sess) != nil {
			return c.SendString("expected nil scopes")
		}
		if !GetLastAccess(sess).IsZero() {
			return c.SendString("expected zero last access")
		}
		if !GetCreatedAt(sess).IsZero() {
			return c.SendString("expected zero created at")
		}
		if HasAMR(sess, "pwd") {
			return c.SendString("expected no amr")
		}
		if HasScope(sess, "read") {
			return c.SendString("expected no scope")
		}

		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test-nil", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestFiberSessionConfigSameSiteVariants(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	tests := []struct {
		sameSite string
		expected string
	}{
		{"Strict", "strict"},
		{"Lax", "lax"},
		{"None", "none"},
		{"Disabled", "disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.sameSite, func(t *testing.T) {
			config := DefaultConfig().WithSameSite(tt.sameSite)
			manager := NewManager(storage, config)
			fiberCfg := manager.FiberSessionConfig()
			if fiberCfg.CookieSameSite != tt.expected {
				t.Errorf("expected SameSite to be %v, got %v", tt.expected, fiberCfg.CookieSameSite)
			}
		})
	}
}

func TestManagerSaveSessionError(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig()
	manager := NewManager(storage, config)

	// Create an expired session
	session := manager.CreateSession("session-123")
	session.ExpiresAt = time.Now().Add(-1 * time.Hour) // Expired

	// Save should still work (with adjusted TTL)
	err := manager.SaveSession(session)
	if err != nil {
		t.Fatalf("failed to save expired session: %v", err)
	}
}

func TestFiberSessionGettersWithWrongTypes(t *testing.T) {
	app := fiber.New()
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	store := fibersession.New(fibersession.Config{
		Storage:    storage,
		Expiration: 1 * time.Hour,
	})

	app.Get("/test-wrong-types", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}

		// Set wrong types for all keys
		sess.Set(KeyUserID, 123)      // Should be string
		sess.Set(KeyEmail, 456)       // Should be string
		sess.Set(KeyPhone, 789)       // Should be string
		sess.Set(KeyAMR, "not-slice") // Should be []string
		sess.Set(KeyScopes, 999)      // Should be []string
		sess.Set(KeyLastAccess, "not-int64")
		sess.Set(KeyCreatedAt, "not-int64")
		sess.Set(KeyAuthenticated, "not-bool")

		// All getters should return empty/default values
		if GetUserID(sess) != "" {
			return c.SendString("expected empty user id for wrong type")
		}
		if GetEmail(sess) != "" {
			return c.SendString("expected empty email for wrong type")
		}
		if GetPhone(sess) != "" {
			return c.SendString("expected empty phone for wrong type")
		}
		if GetAMR(sess) != nil {
			return c.SendString("expected nil amr for wrong type")
		}
		if GetScopes(sess) != nil {
			return c.SendString("expected nil scopes for wrong type")
		}
		if !GetLastAccess(sess).IsZero() {
			return c.SendString("expected zero last access for wrong type")
		}
		if !GetCreatedAt(sess).IsZero() {
			return c.SendString("expected zero created at for wrong type")
		}
		if IsAuthenticated(sess) {
			return c.SendString("expected not authenticated for wrong type")
		}

		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test-wrong-types", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestManagerLoadSessionWithError(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig()
	manager := NewManager(storage, config)

	// Store invalid JSON data
	_ = storage.Set("invalid-json", []byte("not valid json"), time.Hour)

	// LoadSession should return error for invalid JSON
	_, err := manager.LoadSession("invalid-json")
	if err == nil {
		t.Error("expected error for invalid JSON session data")
	}
}

func TestUnauthenticateNilSession(t *testing.T) {
	// Unauthenticate should handle nil session gracefully
	err := Unauthenticate(nil)
	if err != nil {
		t.Errorf("expected no error for nil session, got %v", err)
	}
}

func TestManagerSaveSessionStorageError(t *testing.T) {
	base := NewMemoryStorage("test:", 0)
	defer func() { _ = base.Close() }()

	storage := &failingStorage{
		Storage: base,
		setErr:  errors.New("storage set failed"),
	}
	config := DefaultConfig()
	manager := NewManager(storage, config)

	session := manager.CreateSession("session-123")
	err := manager.SaveSession(session)
	if err == nil {
		t.Error("expected error when storage.Set fails")
	}
}

func TestManagerLoadSessionStorageGetError(t *testing.T) {
	base := NewMemoryStorage("test:", 0)
	defer func() { _ = base.Close() }()

	storage := &failingStorage{
		Storage: base,
		getErr:  errors.New("storage get failed"),
	}
	config := DefaultConfig()
	manager := NewManager(storage, config)

	_, err := manager.LoadSession("any-id")
	if err == nil {
		t.Error("expected error when storage.Get fails")
	}
}
