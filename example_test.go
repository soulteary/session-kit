package session_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	session "github.com/soulteary/session-kit/v3"
)

// The common case: a Manager over in-memory storage, with nothing outside the
// standard library linked in.
func Example() {
	storage := session.NewMemoryStorage("session:", 10*time.Minute)
	defer func() { _ = storage.Close() }()

	manager := session.NewManager(storage, session.DefaultConfig().WithExpiration(time.Hour))

	record := manager.CreateSession("sid-123")
	record.Authenticated = true
	record.UserID = "user-42"
	record.AddAMR("pwd")
	record.AddScope("read")

	if err := manager.SaveSession(record); err != nil {
		fmt.Println("save:", err)
		return
	}

	loaded, err := manager.LoadSession("sid-123")
	if err != nil {
		fmt.Println("load:", err)
		return
	}

	fmt.Println(loaded.UserID, loaded.IsAuthenticated(), loaded.HasScope("read"), loaded.HasAMR("pwd"))
	// Output: user-42 true true true
}

// Swapping the backend is one constructor. Redis lives in the redisstore
// subpackage, so a service that stays in memory never links go-redis:
//
//	import "github.com/soulteary/session-kit/v3/redisstore"
//
//	storage := redisstore.New(redisClient, "session:")
//	manager := session.NewManager(storage, session.DefaultConfig())
//
// NewStorage picks a backend from configuration instead, which needs the
// subpackage imported for its registration side effect.
func ExampleNewStorage() {
	storage, err := session.NewStorage(session.DefaultStorageConfig().
		WithType(session.StorageTypeMemory).
		WithKeyPrefix("myapp:"))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = storage.Close() }()

	fmt.Printf("%T\n", storage)

	// Asking for Redis without importing redisstore says so, rather than
	// failing as an unknown type.
	_, err = session.NewStorage(session.DefaultStorageConfig().WithType(session.StorageTypeRedis))
	fmt.Println(err != nil)
	// Output:
	// *session.MemoryStorage
	// true
}

// CreateCookie returns a *net/http.Cookie, so http.SetCookie takes it as is.
func ExampleCreateCookie() {
	config := session.DefaultConfig().
		WithCookieName("session_id").
		WithCookieDomain("example.com").
		WithSameSite("Strict").
		WithExpiration(time.Hour)

	rec := httptest.NewRecorder()
	http.SetCookie(rec, session.CreateCookie(config, "sid-123"))

	cookie := rec.Result().Cookies()[0]
	fmt.Println(cookie.Name, cookie.Value, cookie.Domain, cookie.SameSite == http.SameSiteStrictMode)
	fmt.Println(cookie.Secure, cookie.HttpOnly)
	// Output:
	// session_id sid-123 example.com true
	// true true
}

// SameSite=None is only honoured on a Secure cookie, so it forces Secure on
// rather than emitting a cookie no browser stores. Every adapter reads the
// rule from here instead of repeating it, because a SameSite rule that
// disagrees with itself across frameworks is a CSRF hole.
func ExampleConfig_CookieSecure() {
	config := session.DefaultConfig().WithSameSite("None").WithSecure(false)

	fmt.Println(config.CookieSecure(), config.SameSiteMode() == http.SameSiteNoneMode)
	fmt.Println(config.Validate())
	// Output:
	// true true
	// same-site None requires Secure=true
}

// memorySession is all it takes to use the session helpers from outside a web
// framework: Get, Set, Delete, Save and Destroy. Fiber v3's
// *middleware/session.Session already has exactly these, which is why Fiber
// code passes its own session to these helpers unchanged -- and why this
// package does not import Fiber.
type memorySession struct {
	values map[any]any
}

func (s *memorySession) Get(key any) any  { return s.values[key] }
func (s *memorySession) Set(key, val any) { s.values[key] = val }
func (s *memorySession) Delete(key any)   { delete(s.values, key) }
func (s *memorySession) Save() error      { return nil }
func (s *memorySession) Destroy() error   { s.values = map[any]any{}; return nil }

func ExampleAuthenticate() {
	sess := &memorySession{values: map[any]any{}}

	session.SetUserID(sess, "user-42")
	session.SetEmail(sess, "user@example.com")
	session.SetScopes(sess, []string{"read", "write"})
	session.AddAMR(sess, "pwd")

	if err := session.Authenticate(sess); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(session.IsAuthenticated(sess), session.GetUserID(sess), session.HasScope(sess, "write"))

	if err := session.Unauthenticate(sess); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(session.IsAuthenticated(sess), session.GetUserID(sess))
	// Output:
	// true user-42 true
	// false
}
