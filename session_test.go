package session

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"testing"
	"time"
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

// fakeSession is a minimal in-memory Session, standing in for a web
// framework's session type. Exercising the helpers through it is the point of
// Reader/Writer/ReadWriter/Saver/Session: the whole helper surface is covered
// here without this package importing Fiber. The same helpers are run against
// a real *fibersession.Session in the fiberadapter package's tests.
type fakeSession struct {
	values     map[any]any
	saved      map[any]any
	saveErr    error
	destroyErr error
	saveCalls  int
	destroyed  bool
}

func newFakeSession() *fakeSession {
	return &fakeSession{values: make(map[any]any)}
}

func (s *fakeSession) Get(key any) any  { return s.values[key] }
func (s *fakeSession) Set(key, val any) { s.values[key] = val }
func (s *fakeSession) Delete(key any)   { delete(s.values, key) }

func (s *fakeSession) Save() error {
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = maps.Clone(s.values)
	return nil
}

func (s *fakeSession) Destroy() error {
	if s.destroyErr != nil {
		return s.destroyErr
	}
	s.destroyed = true
	s.values = make(map[any]any)
	return nil
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

	config := DefaultConfig().WithExpiration(1 * time.Hour)
	manager := NewManager(storage, config)

	// Write an already-expired record directly: SaveSession refuses these.
	session := manager.CreateSession("session-123")
	session.ExpiresAt = time.Now().Add(-1 * time.Hour)
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("failed to marshal session: %v", err)
	}
	if err := storage.Set(session.ID, data, time.Hour); err != nil {
		t.Fatalf("failed to seed storage: %v", err)
	}

	loaded, err := manager.LoadSession("session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for expired session")
	}

	// The expired record is dropped from storage on read.
	if raw, _ := storage.Get("session-123"); raw != nil {
		t.Error("expected the expired session to be deleted from storage")
	}
}

func TestManagerDeleteSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().WithExpiration(1 * time.Hour)
	manager := NewManager(storage, config)

	session := manager.CreateSession("session-123")
	if err := manager.SaveSession(session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	if err := manager.DeleteSession("session-123"); err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	loaded, err := manager.LoadSession("session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("expected session to be deleted")
	}
}

func TestManagerTouchSession(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig().WithExpiration(1 * time.Hour)
	manager := NewManager(storage, config)

	session := manager.CreateSession("session-123")
	originalExpiry := session.ExpiresAt
	originalAccess := session.LastAccessedAt

	time.Sleep(10 * time.Millisecond)

	if err := manager.TouchSession(session); err != nil {
		t.Fatalf("failed to touch session: %v", err)
	}

	if !session.ExpiresAt.After(originalExpiry) {
		t.Error("expected expiration to be extended")
	}
	if !session.LastAccessedAt.After(originalAccess) {
		t.Error("expected last access to be updated")
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
	if !cookie.HttpOnly {
		t.Error("expected HttpOnly to be true")
	}
	if cookie.Expires.IsZero() {
		t.Error("expected Expires to be set")
	}
}

// CreateCookie returns a *net/http.Cookie, so what it produces must survive a
// round trip through net/http itself.
func TestCreateCookieServesThroughNetHTTP(t *testing.T) {
	config := DefaultConfig().
		WithCookieName("my_session").
		WithCookiePath("/app").
		WithSameSite("Strict")

	got := CreateCookie(config, "session-123").String()
	for _, want := range []string{
		"my_session=session-123",
		"Path=/app",
		"HttpOnly",
		"Secure",
		"SameSite=Strict",
	} {
		if !containsSubstring(got, want) {
			t.Errorf("Set-Cookie header %q is missing %q", got, want)
		}
	}
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestCreateCookieSameSiteVariants(t *testing.T) {
	tests := []struct {
		sameSite string
		expected http.SameSite
	}{
		{"Strict", http.SameSiteStrictMode},
		{"Lax", http.SameSiteLaxMode},
		{"None", http.SameSiteNoneMode},
		{"Disabled", http.SameSiteDefaultMode},
		{"", http.SameSiteLaxMode},
		{"nonsense", http.SameSiteLaxMode},
	}

	for _, tt := range tests {
		t.Run(tt.sameSite, func(t *testing.T) {
			config := DefaultConfig().WithSameSite(tt.sameSite)
			cookie := CreateCookie(config, "session-123")
			if cookie.SameSite != tt.expected {
				t.Errorf("expected SameSite to be %v, got %v", tt.expected, cookie.SameSite)
			}
			if config.SameSiteMode() != tt.expected {
				t.Errorf("SameSiteMode() = %v, want %v", config.SameSiteMode(), tt.expected)
			}
		})
	}
}

// "Disabled" must omit the attribute rather than emit one browsers ignore.
func TestCreateCookieSameSiteDisabledOmitsAttribute(t *testing.T) {
	config := DefaultConfig().WithSameSite("Disabled")
	if got := CreateCookie(config, "sid").String(); containsSubstring(got, "SameSite") {
		t.Errorf("Set-Cookie header %q still carries a SameSite attribute", got)
	}
}

func TestSessionHelpers(t *testing.T) {
	sess := newFakeSession()

	if IsAuthenticated(sess) {
		t.Error("a fresh session must not be authenticated")
	}

	SetUserID(sess, "user-123")
	if got := GetUserID(sess); got != "user-123" {
		t.Errorf("GetUserID() = %q, want %q", got, "user-123")
	}

	SetEmail(sess, "test@example.com")
	if got := GetEmail(sess); got != "test@example.com" {
		t.Errorf("GetEmail() = %q, want %q", got, "test@example.com")
	}

	SetPhone(sess, "+1234567890")
	if got := GetPhone(sess); got != "+1234567890" {
		t.Errorf("GetPhone() = %q, want %q", got, "+1234567890")
	}

	SetAMR(sess, []string{"pwd"})
	if amr := GetAMR(sess); len(amr) != 1 || amr[0] != "pwd" {
		t.Errorf("GetAMR() = %v, want [pwd]", amr)
	}

	AddAMR(sess, "otp")
	if !HasAMR(sess, "otp") {
		t.Error("HasAMR(otp) = false after AddAMR(otp)")
	}

	AddAMR(sess, "otp") // duplicates are ignored
	if amr := GetAMR(sess); len(amr) != 2 {
		t.Errorf("GetAMR() = %v after a duplicate AddAMR, want two entries", amr)
	}

	SetScopes(sess, []string{"read"})
	if scopes := GetScopes(sess); len(scopes) != 1 || scopes[0] != "read" {
		t.Errorf("GetScopes() = %v, want [read]", scopes)
	}
	if !HasScope(sess, "read") {
		t.Error("HasScope(read) = false")
	}
	if HasScope(sess, "write") {
		t.Error("HasScope(write) = true")
	}

	UpdateLastAccess(sess)
	if GetLastAccess(sess).IsZero() {
		t.Error("GetLastAccess() is zero after UpdateLastAccess")
	}

	if err := Authenticate(sess); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !IsAuthenticated(sess) {
		t.Error("IsAuthenticated() = false after Authenticate")
	}
	if GetCreatedAt(sess).IsZero() {
		t.Error("GetCreatedAt() is zero after Authenticate")
	}
	if sess.saveCalls != 1 {
		t.Errorf("Authenticate() called Save %d times, want 1", sess.saveCalls)
	}
}

func TestAuthenticateReportsSaveFailure(t *testing.T) {
	sess := newFakeSession()
	sess.saveErr = errors.New("save failed")

	if err := Authenticate(sess); err == nil {
		t.Error("Authenticate() returned nil although Save failed")
	}
}

func TestUnauthenticate(t *testing.T) {
	sess := newFakeSession()
	SetUserID(sess, "user-123")
	SetEmail(sess, "test@example.com")
	SetScopes(sess, []string{"read"})
	if err := Authenticate(sess); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	if err := Unauthenticate(sess); err != nil {
		t.Fatalf("Unauthenticate() error = %v", err)
	}
	if !sess.destroyed {
		t.Error("Unauthenticate() did not destroy the session")
	}
	// Destroy succeeded, so the cleared state is not also written back.
	if sess.saveCalls != 1 {
		t.Errorf("Save called %d times, want 1 (Authenticate only)", sess.saveCalls)
	}
	if IsAuthenticated(sess) {
		t.Error("the session is still authenticated after Unauthenticate")
	}
}

// A failing Destroy must leave a de-authenticated session behind rather than a
// fully authenticated one: the cleared state is persisted as the fallback.
func TestUnauthenticateSavesWhenDestroyFails(t *testing.T) {
	sess := newFakeSession()
	if err := Authenticate(sess); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	SetUserID(sess, "user-123")
	sess.destroyErr = errors.New("destroy failed")

	err := Unauthenticate(sess)
	if err == nil {
		t.Fatal("Unauthenticate() returned nil although Destroy failed")
	}
	if !errors.Is(err, sess.destroyErr) {
		t.Errorf("Unauthenticate() error = %v, want it to wrap the destroy error", err)
	}
	if sess.saveCalls != 2 {
		t.Errorf("Save called %d times, want 2 (Authenticate plus the fallback)", sess.saveCalls)
	}
	if authenticated, _ := sess.saved[KeyAuthenticated].(bool); authenticated {
		t.Error("the persisted session is still authenticated after a failed destroy")
	}
	if _, ok := sess.saved[KeyUserID]; ok {
		t.Error("the persisted session still carries the user id after a failed destroy")
	}
}

func TestUnauthenticateReportsBothFailures(t *testing.T) {
	sess := newFakeSession()
	sess.destroyErr = errors.New("destroy failed")
	sess.saveErr = errors.New("save failed")

	err := Unauthenticate(sess)
	if err == nil {
		t.Fatal("Unauthenticate() returned nil although both Destroy and Save failed")
	}
	if !containsSubstring(err.Error(), "destroy failed") || !containsSubstring(err.Error(), "save failed") {
		t.Errorf("Unauthenticate() error = %v, want both failures named", err)
	}
}

func TestUnauthenticateNilSession(t *testing.T) {
	// Unauthenticate should handle nil session gracefully
	if err := Unauthenticate(nil); err != nil {
		t.Errorf("expected no error for nil session, got %v", err)
	}
}

// Session is an interface, so the nil that reaches Unauthenticate in practice
// is a nil *Session inside a non-nil interface -- from an unassigned field, or
// a helper that returned early on an error. A plain session == nil misses it
// and the next Set panics.
func TestUnauthenticateTypedNilSession(t *testing.T) {
	var sess *fakeSession
	if err := Unauthenticate(sess); err != nil {
		t.Errorf("Unauthenticate(typed nil) error = %v, want nil", err)
	}
}

func TestSessionGettersWithNilValues(t *testing.T) {
	sess := newFakeSession()

	if got := GetUserID(sess); got != "" {
		t.Errorf("GetUserID() = %q, want empty", got)
	}
	if got := GetEmail(sess); got != "" {
		t.Errorf("GetEmail() = %q, want empty", got)
	}
	if got := GetPhone(sess); got != "" {
		t.Errorf("GetPhone() = %q, want empty", got)
	}
	if got := GetAMR(sess); got != nil {
		t.Errorf("GetAMR() = %v, want nil", got)
	}
	if got := GetScopes(sess); got != nil {
		t.Errorf("GetScopes() = %v, want nil", got)
	}
	if !GetLastAccess(sess).IsZero() {
		t.Error("GetLastAccess() is not zero on a fresh session")
	}
	if !GetCreatedAt(sess).IsZero() {
		t.Error("GetCreatedAt() is not zero on a fresh session")
	}
	if HasAMR(sess, "pwd") {
		t.Error("HasAMR(pwd) = true on a fresh session")
	}
	if HasScope(sess, "read") {
		t.Error("HasScope(read) = true on a fresh session")
	}
	if IsAuthenticated(sess) {
		t.Error("IsAuthenticated() = true on a fresh session")
	}
}

func TestSessionGettersWithWrongTypes(t *testing.T) {
	sess := newFakeSession()

	sess.Set(KeyUserID, 123)         // should be string
	sess.Set(KeyEmail, 456)          // should be string
	sess.Set(KeyPhone, 789)          // should be string
	sess.Set(KeyAMR, "not-slice")    // should be []string
	sess.Set(KeyScopes, 999)         // should be []string
	sess.Set(KeyLastAccess, "nope")  // should be int64
	sess.Set(KeyCreatedAt, "nope")   // should be int64
	sess.Set(KeyAuthenticated, "no") // should be bool

	if got := GetUserID(sess); got != "" {
		t.Errorf("GetUserID() = %q, want empty for a wrong-typed value", got)
	}
	if got := GetEmail(sess); got != "" {
		t.Errorf("GetEmail() = %q, want empty for a wrong-typed value", got)
	}
	if got := GetPhone(sess); got != "" {
		t.Errorf("GetPhone() = %q, want empty for a wrong-typed value", got)
	}
	if got := GetAMR(sess); got != nil {
		t.Errorf("GetAMR() = %v, want nil for a wrong-typed value", got)
	}
	if got := GetScopes(sess); got != nil {
		t.Errorf("GetScopes() = %v, want nil for a wrong-typed value", got)
	}
	if !GetLastAccess(sess).IsZero() {
		t.Error("GetLastAccess() is not zero for a wrong-typed value")
	}
	if !GetCreatedAt(sess).IsZero() {
		t.Error("GetCreatedAt() is not zero for a wrong-typed value")
	}
	if IsAuthenticated(sess) {
		t.Error("IsAuthenticated() = true for a wrong-typed value")
	}
}

// Saving an already-expired session used to succeed and grant it a full fresh
// expiration period, so a session that kept being written never expired at all.
func TestManagerSaveSessionRefusesExpired(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	manager := NewManager(storage, DefaultConfig())

	session := manager.CreateSession("session-123")
	session.ExpiresAt = time.Now().Add(-1 * time.Hour)

	if err := manager.SaveSession(session); err == nil {
		t.Fatal("SaveSession() on an expired session returned nil; it was silently resurrected")
	}
	if data, _ := storage.Get("session-123"); data != nil {
		t.Error("the expired session was written to storage")
	}

	// Extending a live session deliberately still works.
	live := manager.CreateSession("session-456")
	if err := manager.SaveSession(live); err != nil {
		t.Fatalf("SaveSession() on a live session error = %v", err)
	}
	if err := manager.TouchSession(session); err != nil {
		t.Fatalf("TouchSession() should renew an expired session, got %v", err)
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

func TestManagerSaveSessionMarshalError(t *testing.T) {
	storage := NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := DefaultConfig()
	manager := NewManager(storage, config)

	// SessionData with Data that cannot be marshaled (channel is not JSON-serializable)
	session := manager.CreateSession("session-123")
	session.Data = map[string]interface{}{"bad": make(chan int)}

	err := manager.SaveSession(session)
	if err == nil {
		t.Error("expected error when session cannot be marshaled")
	}
}

func TestCreateCookieSameSiteNoneForcesSecure(t *testing.T) {
	// When SameSite is None, cookie Secure must be true (browser requirement)
	config := DefaultConfig().
		WithCookieName("s").
		WithSameSite("None").
		WithSecure(false) // explicitly false

	cookie := CreateCookie(config, "sid")
	if !cookie.Secure {
		t.Error("expected Cookie Secure to be true when SameSite is None")
	}
	if cookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("expected SameSite none, got %v", cookie.SameSite)
	}
	if !config.CookieSecure() {
		t.Error("CookieSecure() = false for SameSite=None")
	}
}

// valueSession is a Session that is not a pointer, so reflect reports a kind
// with no nil to speak of. It must be treated as present, not as nil.
type valueSession struct {
	values map[any]any
}

func (s valueSession) Get(key any) any  { return s.values[key] }
func (s valueSession) Set(key, val any) { s.values[key] = val }
func (s valueSession) Delete(key any)   { delete(s.values, key) }
func (s valueSession) Save() error      { return nil }
func (s valueSession) Destroy() error   { clear(s.values); return nil }

func TestUnauthenticateValueSession(t *testing.T) {
	sess := valueSession{values: map[any]any{KeyAuthenticated: true, KeyUserID: "user-123"}}

	if err := Unauthenticate(sess); err != nil {
		t.Fatalf("Unauthenticate() error = %v", err)
	}
	if IsAuthenticated(sess) {
		t.Error("the session is still authenticated after Unauthenticate")
	}
}
