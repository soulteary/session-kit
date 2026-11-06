package fiberadapter_test

import (
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
	session "github.com/soulteary/session-kit/v3"
	"github.com/soulteary/session-kit/v3/fiberadapter"
	"github.com/soulteary/session-kit/v3/redisstore"
)

func TestRevocablePeerClockSpreadPreservesSessionAndRevocation(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var storage interface {
				session.Storage
				session.RevocationStorage
			}
			if backend == "memory" {
				storage = session.NewMemoryStorage("clock-test:", 0)
			} else {
				server := miniredis.RunT(t)
				client := redis.NewClient(&redis.Options{Addr: server.Addr()})
				t.Cleanup(func() { _ = client.Close() })
				storage = redisstore.New(client, "clock-test:")
			}
			defer func() {
				if err := storage.Close(); err != nil {
					t.Error(err)
				}
			}()
			lifetime := time.Hour
			cfg, err := fiberadapter.RevocableSessionConfig(session.NewManager(storage, session.DefaultConfig().WithExpiration(lifetime).WithCookieName("sid")))
			if err != nil {
				t.Fatal(err)
			}
			store := fibersession.NewStore(cfg)
			app := fiber.New()
			app.Get("/seed", func(c fiber.Ctx) error {
				s, e := store.Get(c)
				if e != nil {
					return e
				}
				defer s.Release()
				if e = session.Authenticate(s); e != nil {
					return e
				}
				return c.SendString(s.ID())
			})
			app.Get("/check", func(c fiber.Ctx) error {
				s, e := store.Get(c)
				if e != nil {
					return e
				}
				defer s.Release()
				if session.IsAuthenticated(s) {
					if err := s.Save(); err != nil {
						return err
					}
					return c.SendString(s.ID())
				}
				return c.SendStatus(401)
			})
			response, err := app.Test(httptest.NewRequest("GET", "http://gateway.example/seed", nil))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(response.Body)
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if err != nil || response.StatusCode != 200 {
				t.Fatalf("seed status=%d err=%v", response.StatusCode, err)
			}
			payload, err := storage.GetSession(string(raw))
			if err != nil || len(payload) == 0 {
				t.Fatal("seed missing", err)
			}
			// This token deadline is what an otherwise identical issuer with a 50-millisecond
			// ahead wall clock produces. Persist valid session bytes as that issuer would.
			aheadID := session.NewRevocableSessionID(lifetime + 50*time.Millisecond)
			if err = storage.SetSessionIfActive(aheadID, payload, lifetime); err != nil {
				t.Fatal(err)
			}
			if valid, err := storage.GetSession(aheadID); err != nil || len(valid) == 0 {
				t.Fatal("shared backend rejected live record", err)
			}
			request := httptest.NewRequest("GET", "http://gateway.example/check", nil)
			request.AddCookie(&http.Cookie{Name: "sid", Value: aheadID})
			response, err = app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			returnedID, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if string(returnedID) != aheadID {
				t.Fatal("peer rotated an existing session identifier")
			}
			if response.StatusCode != 200 {
				t.Fatalf("peer rejected bounded clock spread: status=%d", response.StatusCode)
			}
			if err := storage.RevokeSession(aheadID); err != nil {
				t.Fatal(err)
			}
			if err := cfg.Storage.Set(aheadID, payload, lifetime); !errors.Is(err, session.ErrSessionRevoked) {
				t.Fatalf("skew allowance bypassed revocation: %v", err)
			}

		})
	}
}
