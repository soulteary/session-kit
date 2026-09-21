package fiberadapter_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"

	session "github.com/soulteary/session-kit/v3"
	"github.com/soulteary/session-kit/v3/fiberadapter"
)

// Wiring Fiber's session middleware to a session-kit storage, and using the
// root package's helpers on the session Fiber hands back. The helpers take
// small interfaces that *fibersession.Session already satisfies, so there is
// no Fiber-specific version of them to import.
func Example() {
	storage := session.NewMemoryStorage("session:", 10*time.Minute)
	defer func() { _ = storage.Close() }()

	manager := session.NewManager(storage, session.DefaultConfig().WithExpiration(time.Hour))
	store := fibersession.NewStore(fiberadapter.SessionConfig(manager))

	app := fiber.New()
	app.Get("/login", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		session.SetUserID(sess, "user-42")
		session.SetScopes(sess, []string{"read"})
		return session.Authenticate(sess)
	})
	app.Get("/me", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		if !session.IsAuthenticated(sess) {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.SendString(session.GetUserID(sess) + " " + fmt.Sprint(session.HasScope(sess, "read")))
	})

	login, err := app.Test(httptest.NewRequest(http.MethodGet, "/login", nil))
	if err != nil {
		fmt.Println(err)
		return
	}

	me := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, cookie := range login.Cookies() {
		me.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}

	resp, err := app.Test(me)
	if err != nil {
		fmt.Println(err)
		return
	}

	body := make([]byte, 32)
	n, _ := resp.Body.Read(body)
	fmt.Println(resp.StatusCode, string(body[:n]))
	// Output: 200 user-42 true
}

// Cookie is session.CreateCookie in Fiber's cookie type, for handing a session
// id to another domain by hand. Both are built from the same
// Config.SameSiteMode and Config.CookieSecure rules.
func ExampleCookie() {
	config := session.DefaultConfig().
		WithCookieName("session_id").
		WithCookieDomain("example.com").
		WithSameSite("Strict")

	cookie := fiberadapter.Cookie(config, "sid-123")

	fmt.Println(cookie.Name, cookie.Value, cookie.Domain, cookie.SameSite)
	fmt.Println(cookie.Secure, cookie.HTTPOnly)
	// Output:
	// session_id sid-123 example.com Strict
	// true true
}
