package fiberadapter_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"

	session "github.com/soulteary/session-kit/v3"
	"github.com/soulteary/session-kit/v3/fiberadapter"
)

// newStore wires Fiber's session middleware to a session-kit storage the way
// SessionConfig does, but with an explicit storage so each test is isolated.
func newStore(t *testing.T) *fibersession.Store {
	t.Helper()

	storage := session.NewMemoryStorage("test:", 0)
	t.Cleanup(func() { _ = storage.Close() })

	return fibersession.NewStore(fibersession.Config{
		Storage:     fiberadapter.Storage(storage),
		IdleTimeout: 1 * time.Hour,
	})
}

// The root package's helpers take small interfaces; this is the test that
// *fibersession.Session actually satisfies them, and that Fiber code calls
// them with the session it already has.
func TestSessionHelpersOnFiberSession(t *testing.T) {
	app := fiber.New()
	store := newStore(t)

	app.Get("/test", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}

		if session.IsAuthenticated(sess) {
			return c.SendString("should not be authenticated")
		}

		session.SetUserID(sess, "user-123")
		if session.GetUserID(sess) != "user-123" {
			return c.SendString("user id mismatch")
		}

		session.SetEmail(sess, "test@example.com")
		if session.GetEmail(sess) != "test@example.com" {
			return c.SendString("email mismatch")
		}

		session.SetPhone(sess, "+1234567890")
		if session.GetPhone(sess) != "+1234567890" {
			return c.SendString("phone mismatch")
		}

		session.SetAMR(sess, []string{"pwd"})
		if amr := session.GetAMR(sess); len(amr) != 1 || amr[0] != "pwd" {
			return c.SendString("amr mismatch")
		}

		session.AddAMR(sess, "otp")
		if !session.HasAMR(sess, "otp") {
			return c.SendString("should have otp amr")
		}

		session.AddAMR(sess, "otp") // duplicates are ignored
		if len(session.GetAMR(sess)) != 2 {
			return c.SendString("duplicate amr added")
		}

		session.SetScopes(sess, []string{"read"})
		if scopes := session.GetScopes(sess); len(scopes) != 1 || scopes[0] != "read" {
			return c.SendString("scopes mismatch")
		}
		if !session.HasScope(sess, "read") {
			return c.SendString("should have read scope")
		}
		if session.HasScope(sess, "write") {
			return c.SendString("should not have write scope")
		}

		session.UpdateLastAccess(sess)
		if session.GetLastAccess(sess).IsZero() {
			return c.SendString("last access should be set")
		}

		if err := session.Authenticate(sess); err != nil {
			return err
		}
		if !session.IsAuthenticated(sess) {
			return c.SendString("should be authenticated")
		}
		if session.GetCreatedAt(sess).IsZero() {
			return c.SendString("created at should be set")
		}

		return c.SendString("ok")
	})

	assertOK(t, app, httptest.NewRequest("GET", "/test", nil))
}

// Scopes and AMR used to vanish after the request that wrote them: Fiber
// serialises session data with msgpack, which decodes an array as
// []interface{}, so a val.([]string) assertion returned nil on every later
// request. This is the round trip that catches it, and it needs a real Fiber
// session to be worth anything.
func TestStringSlicesSurviveAFiberRoundTrip(t *testing.T) {
	app := fiber.New()
	store := newStore(t)

	app.Get("/write", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		session.SetScopes(sess, []string{"read", "write"})
		session.SetAMR(sess, []string{"pwd", "otp"})
		return sess.Save()
	})

	app.Get("/read", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		if scopes := session.GetScopes(sess); len(scopes) != 2 || scopes[0] != "read" {
			return c.SendString("scopes did not survive the round trip")
		}
		if !session.HasScope(sess, "write") {
			return c.SendString("HasScope is false after a round trip")
		}
		if amr := session.GetAMR(sess); len(amr) != 2 {
			return c.SendString("amr did not survive the round trip")
		}
		if !session.HasAMR(sess, "otp") {
			return c.SendString("HasAMR is false after a round trip")
		}
		return c.SendString("ok")
	})

	resp := assertOK(t, app, httptest.NewRequest("GET", "/write", nil))

	read := httptest.NewRequest("GET", "/read", nil)
	for _, cookie := range resp.Cookies() {
		read.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	assertOK(t, app, read)
}

func TestUnauthenticateOnFiberSession(t *testing.T) {
	app := fiber.New()
	store := newStore(t)

	app.Get("/login", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		session.SetUserID(sess, "user-123")
		if err := session.Authenticate(sess); err != nil {
			return err
		}
		return c.SendString("logged in")
	})

	app.Get("/logout", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		if !session.IsAuthenticated(sess) {
			return c.SendString("not authenticated")
		}
		if err := session.Unauthenticate(sess); err != nil {
			return err
		}
		return c.SendString("logged out")
	})

	app.Get("/check", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		if session.IsAuthenticated(sess) {
			return c.SendString("still authenticated after logout")
		}
		return c.SendString("ok")
	})

	loginResp := assertOK(t, app, httptest.NewRequest("GET", "/login", nil))

	var sessionCookie string
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "session_id" {
			sessionCookie = cookie.Value
			break
		}
	}
	if sessionCookie == "" {
		t.Fatal("login did not set a session cookie")
	}

	logoutReq := httptest.NewRequest("GET", "/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionCookie})
	logoutResp := assertOK(t, app, logoutReq)
	assertBody(t, logoutResp, "logged out")

	// The destroyed session must not come back for the same cookie.
	checkReq := httptest.NewRequest("GET", "/check", nil)
	checkReq.AddCookie(&http.Cookie{Name: "session_id", Value: sessionCookie})
	assertBody(t, assertOK(t, app, checkReq), "ok")
}

func TestSessionConfig(t *testing.T) {
	storage := session.NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := session.DefaultConfig().
		WithCookieName("my_session").
		WithCookieDomain(".example.com").
		WithCookiePath("/app").
		WithSecure(true).
		WithHTTPOnly(true).
		WithSameSite("Strict").
		WithExpiration(2 * time.Hour)

	fiberCfg := fiberadapter.SessionConfig(session.NewManager(storage, config))

	if fiberCfg.IdleTimeout != 2*time.Hour {
		t.Errorf("expected IdleTimeout to be 2h, got %v", fiberCfg.IdleTimeout)
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
	if fiberCfg.Storage == nil {
		t.Error("expected Storage to be wired to the manager's storage")
	}
	if fiberCfg.Extractor.Key != "my_session" {
		t.Errorf("expected the extractor to read the 'my_session' cookie, got %q", fiberCfg.Extractor.Key)
	}
}

// A SessionConfig built from a Manager must actually drive Fiber's middleware
// against that Manager's storage.
func TestSessionConfigSharesTheManagerStorage(t *testing.T) {
	storage := session.NewMemoryStorage("shared:", 0)
	defer func() { _ = storage.Close() }()

	manager := session.NewManager(storage, session.DefaultConfig())
	store := fibersession.NewStore(fiberadapter.SessionConfig(manager))

	app := fiber.New()
	app.Get("/login", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		session.SetUserID(sess, "user-123")
		return sess.Save()
	})

	resp := assertOK(t, app, httptest.NewRequest("GET", "/login", nil))

	var sid string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_id" {
			sid = cookie.Value
		}
	}
	if sid == "" {
		t.Fatal("no session cookie was set")
	}

	raw, err := storage.Get(sid)
	if err != nil {
		t.Fatalf("storage.Get() error = %v", err)
	}
	if raw == nil {
		t.Error("the session was not written to the manager's storage")
	}
}

func TestSessionConfigSameSiteVariants(t *testing.T) {
	storage := session.NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	tests := []struct {
		sameSite string
		expected string
	}{
		{"Strict", fiber.CookieSameSiteStrictMode},
		{"Lax", fiber.CookieSameSiteLaxMode},
		{"None", fiber.CookieSameSiteNoneMode},
		{"Disabled", fiber.CookieSameSiteDisabled},
		{"", fiber.CookieSameSiteLaxMode},
	}

	for _, tt := range tests {
		t.Run(tt.sameSite, func(t *testing.T) {
			config := session.DefaultConfig().WithSameSite(tt.sameSite)
			fiberCfg := fiberadapter.SessionConfig(session.NewManager(storage, config))
			if fiberCfg.CookieSameSite != tt.expected {
				t.Errorf("expected SameSite to be %v, got %v", tt.expected, fiberCfg.CookieSameSite)
			}
		})
	}
}

func TestSessionConfigSameSiteNoneForcesSecure(t *testing.T) {
	storage := session.NewMemoryStorage("test:", 0)
	defer func() { _ = storage.Close() }()

	config := session.DefaultConfig().
		WithSameSite("None").
		WithSecure(false)

	fiberCfg := fiberadapter.SessionConfig(session.NewManager(storage, config))
	if !fiberCfg.CookieSecure {
		t.Error("expected CookieSecure to be true when SameSite is None")
	}
}

func TestCookie(t *testing.T) {
	config := session.DefaultConfig().
		WithCookieName("my_session").
		WithCookieDomain(".example.com").
		WithCookiePath("/app").
		WithSecure(true).
		WithHTTPOnly(true).
		WithSameSite("Strict").
		WithExpiration(1 * time.Hour)

	cookie := fiberadapter.Cookie(config, "session-123")

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
	if cookie.SameSite != fiber.CookieSameSiteStrictMode {
		t.Errorf("expected SameSite to be Strict, got %s", cookie.SameSite)
	}
}

func TestCookieSameSiteNoneForcesSecure(t *testing.T) {
	config := session.DefaultConfig().
		WithCookieName("s").
		WithSameSite("None").
		WithSecure(false)

	cookie := fiberadapter.Cookie(config, "sid")
	if !cookie.Secure {
		t.Error("expected Secure to be true when SameSite is None")
	}
	if cookie.SameSite != fiber.CookieSameSiteNoneMode {
		t.Errorf("expected SameSite None, got %s", cookie.SameSite)
	}
}

// The cookie session.CreateCookie builds and the one Cookie builds describe
// the same cookie; only the type differs. A rule that disagrees between the
// two is a CSRF hole, not a cosmetic difference.
func TestCookieMatchesCreateCookie(t *testing.T) {
	for _, sameSite := range []string{"Strict", "Lax", "None", "Disabled", "", "nonsense"} {
		t.Run(sameSite, func(t *testing.T) {
			config := session.DefaultConfig().
				WithCookieName("s").
				WithCookiePath("/app").
				WithCookieDomain(".example.com").
				WithSameSite(sameSite).
				WithSecure(false)

			std := session.CreateCookie(config, "sid")
			fib := fiberadapter.Cookie(config, "sid")

			if std.Name != fib.Name || std.Value != fib.Value ||
				std.Path != fib.Path || std.Domain != fib.Domain ||
				std.Secure != fib.Secure || std.HttpOnly != fib.HTTPOnly {
				t.Errorf("cookies disagree: net/http %+v vs fiber %+v", std, fib)
			}
			if want := fiberadapter.SameSite(std.SameSite); fib.SameSite != want {
				t.Errorf("SameSite = %q, want %q", fib.SameSite, want)
			}
		})
	}
}

func TestSameSite(t *testing.T) {
	tests := []struct {
		mode http.SameSite
		want string
	}{
		{http.SameSiteStrictMode, fiber.CookieSameSiteStrictMode},
		{http.SameSiteLaxMode, fiber.CookieSameSiteLaxMode},
		{http.SameSiteNoneMode, fiber.CookieSameSiteNoneMode},
		{http.SameSiteDefaultMode, fiber.CookieSameSiteDisabled},
		{http.SameSite(0), fiber.CookieSameSiteLaxMode},
	}

	for _, tt := range tests {
		if got := fiberadapter.SameSite(tt.mode); got != tt.want {
			t.Errorf("SameSite(%v) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestStorageAdapterForwardsEveryMethod(t *testing.T) {
	backing := session.NewMemoryStorage("adapter:", 0)
	defer func() { _ = backing.Close() }()

	adapter := fiberadapter.Storage(backing)
	ctx := t.Context()

	if err := adapter.Set("k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if got, err := adapter.Get("k"); err != nil || string(got) != "v" {
		t.Fatalf("Get() = (%q, %v), want (\"v\", nil)", got, err)
	}
	if got, err := adapter.GetWithContext(ctx, "k"); err != nil || string(got) != "v" {
		t.Fatalf("GetWithContext() = (%q, %v), want (\"v\", nil)", got, err)
	}

	if err := adapter.SetWithContext(ctx, "ctx", []byte("v"), time.Minute); err != nil {
		t.Fatalf("SetWithContext() error = %v", err)
	}
	if got, _ := backing.Get("ctx"); string(got) != "v" {
		t.Errorf("SetWithContext() did not reach the backing storage, got %q", got)
	}

	if err := adapter.Delete("k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got, _ := adapter.Get("k"); got != nil {
		t.Errorf("Get() = %q after Delete, want nil", got)
	}

	if err := adapter.DeleteWithContext(ctx, "ctx"); err != nil {
		t.Fatalf("DeleteWithContext() error = %v", err)
	}
	if got, _ := backing.Get("ctx"); got != nil {
		t.Errorf("DeleteWithContext() did not reach the backing storage, got %q", got)
	}

	if err := adapter.Set("reset-me", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := adapter.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if backing.Len() != 0 {
		t.Errorf("Reset() left %d entries behind", backing.Len())
	}

	if err := adapter.Set("reset-me", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := adapter.ResetWithContext(ctx); err != nil {
		t.Fatalf("ResetWithContext() error = %v", err)
	}
	if backing.Len() != 0 {
		t.Errorf("ResetWithContext() left %d entries behind", backing.Len())
	}

	if err := adapter.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func assertOK(t *testing.T, app *fiber.App, req *http.Request) *http.Response {
	t.Helper()

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test(%s) error = %v", req.URL.Path, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("app.Test(%s) status = %d, want 200", req.URL.Path, resp.StatusCode)
	}
	return resp
}

func assertBody(t *testing.T, resp *http.Response, want string) {
	t.Helper()

	body := make([]byte, len(want)+32)
	n, _ := resp.Body.Read(body)
	if got := string(body[:n]); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
