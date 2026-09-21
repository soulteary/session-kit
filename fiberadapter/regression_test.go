package fiberadapter_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"

	session "github.com/soulteary/session-kit/v3"
	"github.com/soulteary/session-kit/v3/fiberadapter"
)

// --- Session fixation (login must rotate the session id) ---
//
// The root package proves the rotation through its own Regenerator fake;
// these are the end-to-end tests, over a real Fiber session, real cookies and
// real storage. Fiber's *middleware/session.Session is the one session type
// this repository actually ships a wiring for, so it is the one where the
// fixation hole was reachable: Fiber adopts a client-supplied id whenever
// storage holds a record for it, which is all it takes to plant one.

// newFixationStore returns a Fiber store together with the storage behind it,
// so a test can look at what is on disk under a given session id.
func newFixationStore(t *testing.T, prefix string) (*fibersession.Store, session.Storage) {
	t.Helper()

	storage := session.NewMemoryStorage(prefix, 0)
	t.Cleanup(func() { _ = storage.Close() })

	store := fibersession.NewStore(fibersession.Config{
		Storage:     fiberadapter.Storage(storage),
		IdleTimeout: time.Hour,
	})

	return store, storage
}

// TestAuthenticateRotatesFiberSessionID is the regression test for session
// fixation over the store.Get path. Authenticate only wrote the
// authentication markers and saved, so the id survived login: an id an
// attacker had planted in the victim's browser came back out of login
// authenticated, and the attacker's copy of it then granted access to the
// victim's account.
func TestAuthenticateRotatesFiberSessionID(t *testing.T) {
	app := fiber.New()
	store, storage := newFixationStore(t, "fixation:")

	// The attacker mints a real, stored session id to plant in the victim's
	// browser. Fiber adopts a client-supplied id only when storage already
	// holds a record for it, which is exactly what this route produces.
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
		session.SetUserID(sess, "victim-123")
		return session.Authenticate(sess)
	})

	app.Get("/whoami", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		if !session.IsAuthenticated(sess) {
			return c.SendString("anonymous")
		}
		return c.SendString(session.GetUserID(sess))
	})

	plantedID := sessionIDFrom(t, getWithSession(t, app, "/plant", ""))
	if plantedID == "" {
		t.Fatal("/plant did not return a session cookie")
	}

	rotatedID := sessionIDFrom(t, getWithSession(t, app, "/login", plantedID))
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
	if body := bodyFrom(t, getWithSession(t, app, "/whoami", plantedID)); body != "anonymous" {
		t.Errorf("/whoami with the planted id = %q, want %q", body, "anonymous")
	}

	// The victim's new id must be authenticated and must keep the data
	// written before Authenticate was called.
	if body := bodyFrom(t, getWithSession(t, app, "/whoami", rotatedID)); body != "victim-123" {
		t.Errorf("/whoami with the rotated id = %q, want %q", body, "victim-123")
	}
}

// TestAuthenticateRotatesFiberSessionIDUnderMiddleware covers the other
// supported wiring: with session.New + session.FromContext, Save is a
// deliberate no-op and the middleware persists the session when the handler
// returns. The rotation has to reach storage and the client cookie there too.
func TestAuthenticateRotatesFiberSessionIDUnderMiddleware(t *testing.T) {
	app := fiber.New()
	storage := session.NewMemoryStorage("fixation-mw:", 0)
	t.Cleanup(func() { _ = storage.Close() })

	app.Use(fibersession.New(fibersession.Config{
		Storage:     fiberadapter.Storage(storage),
		IdleTimeout: time.Hour,
	}))

	app.Get("/plant", func(c fiber.Ctx) error {
		fibersession.FromContext(c).Set("planted", true)
		return c.SendString("planted")
	})

	app.Get("/login", func(c fiber.Ctx) error {
		sess := fibersession.FromContext(c).Session
		session.SetUserID(sess, "victim-123")
		if err := session.Authenticate(sess); err != nil {
			return err
		}
		return c.SendString("logged in")
	})

	app.Get("/whoami", func(c fiber.Ctx) error {
		sess := fibersession.FromContext(c).Session
		if !session.IsAuthenticated(sess) {
			return c.SendString("anonymous")
		}
		return c.SendString(session.GetUserID(sess))
	})

	plantedID := sessionIDFrom(t, getWithSession(t, app, "/plant", ""))
	if plantedID == "" {
		t.Fatal("/plant did not return a session cookie")
	}

	rotatedID := sessionIDFrom(t, getWithSession(t, app, "/login", plantedID))
	if rotatedID == "" {
		t.Fatal("/login did not return a session cookie")
	}
	if rotatedID == plantedID {
		t.Errorf("session id %q survived login under the middleware", plantedID)
	}

	if body := bodyFrom(t, getWithSession(t, app, "/whoami", plantedID)); body != "anonymous" {
		t.Errorf("/whoami with the planted id = %q, want %q", body, "anonymous")
	}
	if body := bodyFrom(t, getWithSession(t, app, "/whoami", rotatedID)); body != "victim-123" {
		t.Errorf("/whoami with the rotated id = %q, want %q", body, "victim-123")
	}
}

// TestAuthenticateFailsClosedOnFiberSession: rotation is attempted before the
// authentication markers are written, so a storage failure leaves the session
// unauthenticated instead of authenticated under an id an attacker may
// already hold.
func TestAuthenticateFailsClosedOnFiberSession(t *testing.T) {
	app := fiber.New()
	backend := session.NewMemoryStorage("rotate-fail:", 0)
	t.Cleanup(func() { _ = backend.Close() })

	store := fibersession.NewStore(fibersession.Config{
		Storage: fiberadapter.Storage(deleteFailingStorage{
			Storage: backend,
			err:     errors.New("storage unavailable"),
		}),
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
		if authErr := session.Authenticate(sess); authErr == nil {
			return c.SendString("authenticate succeeded without rotating")
		}
		if session.IsAuthenticated(sess) {
			return c.SendString("session marked authenticated despite the failure")
		}
		return c.SendString("failed closed")
	})

	plantedID := sessionIDFrom(t, getWithSession(t, app, "/plant", ""))
	if plantedID == "" {
		t.Fatal("/plant did not return a session cookie")
	}

	if body := bodyFrom(t, getWithSession(t, app, "/login", plantedID)); body != "failed closed" {
		t.Errorf("/login = %q, want %q", body, "failed closed")
	}
}

// deleteFailingStorage is a storage backend whose Delete always fails,
// standing in for a backend that is unreachable at the moment a login tries to
// rotate the session id.
type deleteFailingStorage struct {
	session.Storage
	err error
}

func (s deleteFailingStorage) Delete(string) error { return s.err }

// getWithSession issues a GET carrying the given session id, if any.
func getWithSession(t *testing.T, app *fiber.App, path, sessionID string) *http.Response {
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

func sessionIDFrom(t *testing.T, resp *http.Response) string {
	t.Helper()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_id" {
			return cookie.Value
		}
	}
	return ""
}

func bodyFrom(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	_ = resp.Body.Close()
	return string(body)
}
