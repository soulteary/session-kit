package session

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/redis/go-redis/v9"
)

// TestStringSliceSurvivesStorageRoundTrip is the regression test for scopes and
// AMR vanishing after the first request. Fiber v3 serialises session data with
// msgpack, which decodes an array as []interface{}; a val.([]string) assertion
// therefore returned nil on every request after the one that wrote it.
func TestStringSliceSurvivesStorageRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want []string
	}{
		{"native slice", []string{"read", "write"}, []string{"read", "write"}},
		{"decoded from msgpack", []interface{}{"read", "write"}, []string{"read", "write"}},
		{"nil", nil, nil},
		{"wrong element type", []interface{}{"read", 42}, nil},
		{"wrong type entirely", "read", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stringSlice(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("stringSlice(%#v) = %#v, want %#v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("stringSlice(%#v)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestMemoryStorageGetReturnsCopy: Set copies on write, so Get must copy on
// read. Returning the internal slice let one caller corrupt every later
// reader's view of the session.
func TestMemoryStorageGetReturnsCopy(t *testing.T) {
	s := NewMemoryStorage("t:", 0)
	defer func() { _ = s.Close() }()

	original := []byte("authenticated")
	if err := s.Set("k", original, time.Minute); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	got[0] = 'X'

	again, err := s.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, original) {
		t.Errorf("stored value became %q after a caller mutated what Get returned", again)
	}
}

// TestMemoryStorageEmptyValueDeletes: writing empty data must not leave the
// previous, still-authenticated payload readable.
func TestMemoryStorageEmptyValueDeletes(t *testing.T) {
	s := NewMemoryStorage("t:", 0)
	defer func() { _ = s.Close() }()

	if err := s.Set("k", []byte("authenticated"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("k", nil, time.Minute); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("Get() = %q after an empty write; the old session data is still readable", got)
	}
}

// TestMemoryStorageCloseIsIdempotent: an unguarded close(done) panicked on the
// second call, which a deferred Close plus an explicit shutdown hits easily.
func TestMemoryStorageCloseIsIdempotent(t *testing.T) {
	s := NewMemoryStorage("t:", time.Hour)
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

// --- Codex review follow-ups (PR #4) ---

// TestEmptyValueSetDeletesInEveryBackend is the regression test for the
// delete-on-empty-value semantics being applied to MemoryStorage only.
// RedisStorage still ignored an empty value, so code tested against memory
// appeared to clear old authenticated data and left it readable in production.
func TestEmptyValueSetDeletesInEveryBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	backends := map[string]Storage{
		"memory": NewMemoryStorage("consistency:", 0),
		"redis":  NewRedisStorage(client, "consistency:"),
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

// --- Session fixation (login must rotate the session id) ---

// deleteFailingStorage is a storage backend whose Delete always fails, standing
// in for a backend that is unreachable at the moment a login tries to rotate
// the session ID.
type deleteFailingStorage struct {
	Storage
	err error
}

func (s deleteFailingStorage) Delete(string) error { return s.err }

// TestAuthenticateRotatesSessionID is the regression test for session
// fixation. Authenticate only wrote the authentication markers and saved, so
// the session ID survived login: an ID an attacker had planted in the victim's
// browser before login came out of login authenticated, and the attacker's
// copy of that ID granted access to the victim's account.
func TestAuthenticateRotatesSessionID(t *testing.T) {
	app := fiber.New()
	storage := NewMemoryStorage("fixation:", 0)
	defer func() { _ = storage.Close() }()

	store := fibersession.NewStore(fibersession.Config{
		Storage:     fiberStorageAdapter{storage: storage},
		IdleTimeout: time.Hour,
	})

	// The attacker obtains a real, stored session ID to plant in the victim's
	// browser. Fiber only adopts a client-supplied ID that already exists in
	// storage, which is exactly what this route produces.
	app.Get("/plant", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		sess.Set("planted", true)
		return sess.Save()
	})

	app.Get("/login", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		SetUserID(sess, "victim-123")
		return Authenticate(sess)
	})

	app.Get("/whoami", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		if !IsAuthenticated(sess) {
			return c.SendString("anonymous")
		}
		return c.SendString(GetUserID(sess))
	})

	plantedID := sessionIDFromResponse(t, doRequest(t, app, "/plant", ""))
	if plantedID == "" {
		t.Fatal("/plant did not return a session cookie")
	}

	loginResp := doRequest(t, app, "/login", plantedID)
	rotatedID := sessionIDFromResponse(t, loginResp)
	if rotatedID == "" {
		t.Fatal("/login did not return a session cookie")
	}

	if rotatedID == plantedID {
		t.Errorf("session id %q survived login; an id planted before login is now authenticated", plantedID)
	}

	// The planted id must be gone from storage, not merely unused.
	data, err := storage.Get(plantedID)
	if err != nil {
		t.Fatalf("Get(plantedID) error = %v", err)
	}
	if data != nil {
		t.Errorf("the pre-login session record is still in storage under %q", plantedID)
	}

	// The attacker's copy of the id must not be authenticated.
	if body := bodyOf(t, doRequest(t, app, "/whoami", plantedID)); body != "anonymous" {
		t.Errorf("/whoami with the planted id = %q, want %q", body, "anonymous")
	}

	// The victim's new id must be authenticated and keep the data written
	// before Authenticate was called.
	if body := bodyOf(t, doRequest(t, app, "/whoami", rotatedID)); body != "victim-123" {
		t.Errorf("/whoami with the rotated id = %q, want %q", body, "victim-123")
	}
}

// TestAuthenticateFailsClosedWhenRotationFails: rotation is attempted before
// the authentication markers are written, so a storage failure leaves the
// session unauthenticated instead of authenticated under an id an attacker
// may already hold.
func TestAuthenticateFailsClosedWhenRotationFails(t *testing.T) {
	app := fiber.New()
	backend := NewMemoryStorage("rotate-fail:", 0)
	defer func() { _ = backend.Close() }()

	store := fibersession.NewStore(fibersession.Config{
		Storage: fiberStorageAdapter{storage: deleteFailingStorage{
			Storage: backend,
			err:     errors.New("storage unavailable"),
		}},
		IdleTimeout: time.Hour,
	})

	app.Get("/plant", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		sess.Set("planted", true)
		return sess.Save()
	})

	app.Get("/login", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		if authErr := Authenticate(sess); authErr == nil {
			return c.SendString("authenticate succeeded without rotating")
		}
		if IsAuthenticated(sess) {
			return c.SendString("session marked authenticated despite the failure")
		}
		return c.SendString("failed closed")
	})

	plantedID := sessionIDFromResponse(t, doRequest(t, app, "/plant", ""))
	if plantedID == "" {
		t.Fatal("/plant did not return a session cookie")
	}

	if body := bodyOf(t, doRequest(t, app, "/login", plantedID)); body != "failed closed" {
		t.Errorf("/login = %q, want %q", body, "failed closed")
	}
}

// doRequest issues a GET carrying the given session id, if any.
func doRequest(t *testing.T, app *fiber.App, path, sessionID string) *http.Response {
	t.Helper()

	req := httptest.NewRequest("GET", path, nil)
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET %s: status %d, want 200", path, resp.StatusCode)
	}
	return resp
}

func sessionIDFromResponse(t *testing.T, resp *http.Response) string {
	t.Helper()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_id" {
			return cookie.Value
		}
	}
	return ""
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	_ = resp.Body.Close()
	return string(body)
}

// TestAuthenticateRotatesSessionIDUnderMiddleware covers the other supported
// wiring: with session.New + session.FromContext, Save is a deliberate no-op
// and the middleware persists the session when the handler returns. The
// rotation has to reach storage and the client cookie on that path too.
func TestAuthenticateRotatesSessionIDUnderMiddleware(t *testing.T) {
	app := fiber.New()
	storage := NewMemoryStorage("fixation-mw:", 0)
	defer func() { _ = storage.Close() }()

	app.Use(fibersession.New(fibersession.Config{
		Storage:     fiberStorageAdapter{storage: storage},
		IdleTimeout: time.Hour,
	}))

	app.Get("/plant", func(c fiber.Ctx) error {
		fibersession.FromContext(c).Set("planted", true)
		return c.SendString("planted")
	})

	app.Get("/login", func(c fiber.Ctx) error {
		sess := fibersession.FromContext(c).Session
		SetUserID(sess, "victim-123")
		if err := Authenticate(sess); err != nil {
			return err
		}
		return c.SendString("logged in")
	})

	app.Get("/whoami", func(c fiber.Ctx) error {
		sess := fibersession.FromContext(c).Session
		if !IsAuthenticated(sess) {
			return c.SendString("anonymous")
		}
		return c.SendString(GetUserID(sess))
	})

	plantedID := sessionIDFromResponse(t, doRequest(t, app, "/plant", ""))
	if plantedID == "" {
		t.Fatal("/plant did not return a session cookie")
	}

	rotatedID := sessionIDFromResponse(t, doRequest(t, app, "/login", plantedID))
	if rotatedID == "" {
		t.Fatal("/login did not return a session cookie")
	}
	if rotatedID == plantedID {
		t.Errorf("session id %q survived login under the middleware", plantedID)
	}

	if body := bodyOf(t, doRequest(t, app, "/whoami", plantedID)); body != "anonymous" {
		t.Errorf("/whoami with the planted id = %q, want %q", body, "anonymous")
	}
	if body := bodyOf(t, doRequest(t, app, "/whoami", rotatedID)); body != "victim-123" {
		t.Errorf("/whoami with the rotated id = %q, want %q", body, "victim-123")
	}
}
