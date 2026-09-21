// Package fiberadapter serves session-kit's sessions over Fiber v3.
//
// It lives in its own package so that importing the root package does not drag
// Fiber -- and with it fasthttp -- into binaries that never use it. A service
// on net/http, Echo, Gin or chi pays nothing for Fiber support existing; only
// importing this package links it in.
//
// Everything here is a translation layer. The session helpers
// ([session.Authenticate], [session.SetUserID], [session.HasScope] and the
// rest) take small interfaces that *middleware/session.Session already
// satisfies, so they are called from the root package directly and have no
// counterpart here:
//
//	store := fibersession.NewStore(fiberadapter.SessionConfig(manager))
//
//	app.Get("/me", func(c fiber.Ctx) error {
//	    sess, err := store.Get(c)
//	    if err != nil {
//	        return err
//	    }
//	    if !session.IsAuthenticated(sess) {
//	        return c.SendStatus(fiber.StatusUnauthorized)
//	    }
//	    return c.SendString(session.GetUserID(sess))
//	})
//
// What does need translating is the storage interface, the middleware config
// and the cookie type.
package fiberadapter

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"

	session "github.com/soulteary/session-kit/v3"
)

// Storage adapts a session.Storage to fiber.Storage, which additionally
// requires context-aware methods.
//
// The context is accepted and discarded: session.Storage has no context in its
// contract, and the backends behind it -- memory, and Redis on
// context.Background -- do not honour cancellation. Passing one through would
// promise a deadline that nothing enforces.
func Storage(storage session.Storage) fiber.Storage {
	return storageAdapter{storage: storage}
}

type storageAdapter struct {
	storage session.Storage
}

// Compile-time proof that the adapter satisfies what Fiber asks for.
var _ fiber.Storage = storageAdapter{}

func (a storageAdapter) Get(key string) ([]byte, error) {
	return a.storage.Get(key)
}

func (a storageAdapter) GetWithContext(_ context.Context, key string) ([]byte, error) {
	return a.storage.Get(key)
}

func (a storageAdapter) Set(key string, value []byte, expiration time.Duration) error {
	return a.storage.Set(key, value, expiration)
}

func (a storageAdapter) SetWithContext(_ context.Context, key string, value []byte, expiration time.Duration) error {
	return a.storage.Set(key, value, expiration)
}

func (a storageAdapter) Delete(key string) error {
	return a.storage.Delete(key)
}

func (a storageAdapter) DeleteWithContext(_ context.Context, key string) error {
	return a.storage.Delete(key)
}

func (a storageAdapter) Reset() error {
	return a.storage.Reset()
}

func (a storageAdapter) ResetWithContext(_ context.Context) error {
	return a.storage.Reset()
}

func (a storageAdapter) Close() error {
	return a.storage.Close()
}

// SessionConfig returns a fiber/v3/middleware/session.Config wired to the
// Manager's storage and cookie settings. It is the Fiber counterpart of the
// cookie [session.CreateCookie] builds, and replaces the
// (*session.Manager).FiberSessionConfig method that used to live in the root
// package.
//
//	store := fibersession.NewStore(fiberadapter.SessionConfig(manager))
func SessionConfig(manager *session.Manager) fibersession.Config {
	config := manager.GetConfig()

	return fibersession.Config{
		IdleTimeout:    config.Expiration,
		Storage:        Storage(manager.GetStorage()),
		Extractor:      extractors.FromCookie(config.CookieName),
		CookieDomain:   config.CookieDomain,
		CookiePath:     config.CookiePath,
		CookieSecure:   config.CookieSecure(),
		CookieHTTPOnly: config.HTTPOnly,
		CookieSameSite: SameSite(config.SameSiteMode()),
	}
}

// Cookie builds the session cookie described by config as a *fiber.Cookie,
// for c.Cookie. It is [session.CreateCookie] in Fiber's cookie type, built
// from the same rules; session.CreateCookie itself returns a
// *net/http.Cookie.
func Cookie(config session.Config, sessionID string) *fiber.Cookie {
	return &fiber.Cookie{
		Name:     config.CookieName,
		Value:    sessionID,
		Expires:  time.Now().Add(config.Expiration),
		Path:     config.CookiePath,
		Domain:   config.CookieDomain,
		Secure:   config.CookieSecure(),
		HTTPOnly: config.HTTPOnly,
		SameSite: SameSite(config.SameSiteMode()),
	}
}

// SameSite translates a net/http SameSite mode into the string Fiber expects
// in fiber.Cookie.SameSite and session.Config.CookieSameSite.
//
// http.SameSiteDefaultMode means "omit the attribute", which Fiber spells
// fiber.CookieSameSiteDisabled. Anything unrecognised falls back to Lax, the
// same default [session.Config.SameSiteMode] applies.
func SameSite(mode http.SameSite) string {
	switch mode {
	case http.SameSiteStrictMode:
		return fiber.CookieSameSiteStrictMode
	case http.SameSiteNoneMode:
		return fiber.CookieSameSiteNoneMode
	case http.SameSiteDefaultMode:
		return fiber.CookieSameSiteDisabled
	default:
		return fiber.CookieSameSiteLaxMode
	}
}
