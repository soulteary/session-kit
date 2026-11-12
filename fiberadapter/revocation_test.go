package fiberadapter_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRevocableFiberLogoutRejectsInflightSave(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var first, second session.Storage
			if backend == "memory" {
				first = session.NewMemoryStorage("revocable:", 0)
				second = first
				t.Cleanup(func() { _ = first.Close() })
			} else {
				server := miniredis.RunT(t)
				client := redis.NewClient(&redis.Options{Addr: server.Addr()})
				t.Cleanup(func() { _ = client.Close() })
				first = redisstore.New(client, "revocable:")
				second = redisstore.New(client, "revocable:")
			}
			settings := session.DefaultConfig().WithCookieName("revocable_id").WithExpiration(time.Hour)
			firstConfig, err := fiberadapter.RevocableSessionConfig(session.NewManager(first, settings))
			if err != nil {
				t.Fatal(err)
			}
			secondConfig, err := fiberadapter.RevocableSessionConfig(session.NewManager(second, settings))
			if err != nil {
				t.Fatal(err)
			}
			store1, store2 := fibersession.NewStore(firstConfig), fibersession.NewStore(secondConfig)
			app := fiber.New()
			app.Get("/seed", func(c fiber.Ctx) error {
				sess, err := store1.Get(c)
				if err != nil {
					return err
				}
				defer sess.Release()
				sess.Set(session.KeyUserID, "user-1")
				if err := session.Authenticate(sess); err != nil {
					return err
				}
				return c.SendString(sess.ID())
			})
			entered, resume := make(chan struct{}), make(chan struct{})
			saveResult := make(chan error, 1)
			app.Get("/hold", func(c fiber.Ctx) error {
				sess, err := store1.Get(c)
				if err != nil {
					return err
				}
				defer sess.Release()
				close(entered)
				<-resume
				sess.Set("role", "viewer")
				err = sess.Save()
				saveResult <- err
				if err != nil {
					return c.SendStatus(409)
				}
				return c.SendStatus(200)
			})
			app.Post("/logout", func(c fiber.Ctx) error {
				sess, err := store2.Get(c)
				if err != nil {
					return err
				}
				defer sess.Release()
				if err := session.Unauthenticate(sess); err != nil {
					return err
				}
				return c.SendStatus(200)
			})
			app.Get("/check", func(c fiber.Ctx) error {
				sess, err := store2.Get(c)
				if err != nil {
					return err
				}
				defer sess.Release()
				if !session.IsAuthenticated(sess) {
					return c.SendStatus(401)
				}
				return c.SendStatus(200)
			})
			response, err := app.Test(httptest.NewRequest("GET", "http://example.com/seed", nil))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != 200 {
				t.Fatalf("seed %d %v", response.StatusCode, err)
			}
			id := string(raw)
			request := func(method, path string) *http.Request {
				r := httptest.NewRequest(method, "http://example.com"+path, nil)
				r.AddCookie(&http.Cookie{Name: "revocable_id", Value: id})
				return r
			}
			held := make(chan error, 1)
			go func() {
				response, err := app.Test(request("GET", "/hold"))
				if err == nil {
					_ = response.Body.Close()
					if response.StatusCode != 200 && response.StatusCode != 409 {
						err = fmt.Errorf("hold %d", response.StatusCode)
					}
				}
				held <- err
			}()
			<-entered
			response, err = app.Test(request("POST", "/logout"))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("logout %d", response.StatusCode)
			}
			cleared := false
			for _, cookie := range response.Cookies() {
				if cookie.Name == "revocable_id" && cookie.Value == "" {
					cleared = true
				}
			}
			if !cleared {
				t.Fatal("logout did not clear browser cookie")
			}
			close(resume)
			if err := <-held; err != nil {
				t.Fatal(err)
			}
			// The logout ran in this process. Fiber v3.7.0 notices that itself and
			// drops the save without calling the storage, so Save returns nil;
			// otherwise the storage refuses the write. Either way nothing is
			// stored, which the /check request below verifies.
			if err := <-saveResult; err != nil && !errors.Is(err, session.ErrSessionRevoked) {
				t.Fatalf("unexpected save result: %v", err)
			}
			response, err = app.Test(request("GET", "/check"))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != 401 {
				t.Fatal("old cookie authenticated after logout")
			}
			expired := session.NewRevocableSessionID(-time.Second)
			if err := firstConfig.Storage.Set(expired, []byte("stale snapshot after retention"), time.Hour); !errors.Is(err, session.ErrSessionExpired) {
				t.Fatalf("expired writer accepted: %v", err)
			}
			if err := firstConfig.Storage.Reset(); err == nil {
				t.Fatal("reset could delete revocation markers")
			}
			if payload, err := firstConfig.Storage.Get("legacy-cookie"); err != nil || payload != nil {
				t.Fatalf("legacy identity accepted: %q %v", payload, err)
			}
			if err := firstConfig.Storage.Set("legacy-cookie", []byte("x"), time.Hour); err == nil {
				t.Fatal("legacy writer accepted")
			}
			tooFar := session.NewRevocableSessionID(2 * time.Hour)
			if payload, err := firstConfig.Storage.Get(tooFar); err != nil || payload != nil {
				t.Fatal("future identifier accepted")
			}
			if err := firstConfig.Storage.Set(tooFar, []byte("x"), time.Hour); err == nil {
				t.Fatal("future writer accepted")
			}
			if strings.Contains(id, "user-1") {
				t.Fatal("identifier exposed identity")
			}
		})
	}
}

func TestRevocableConfigRejectsUnsupportedStorageAndUnboundedLifetime(t *testing.T) {
	memory := session.NewMemoryStorage("test:", 0)
	defer func() { _ = memory.Close() }()
	settings := session.DefaultConfig().WithExpiration(0)
	if _, err := fiberadapter.RevocableSessionConfig(session.NewManager(memory, settings)); err == nil {
		t.Fatal("unbounded configuration accepted")
	}
	wrapped := &ordinaryStorage{Storage: memory}
	if _, err := fiberadapter.RevocableSessionConfig(session.NewManager(wrapped, session.DefaultConfig())); err == nil {
		t.Fatal("non-atomic backend accepted")
	}
}

type ordinaryStorage struct{ session.Storage }

// A revocation made by another replica reaches this process only through the
// storage, which must refuse the stale save.
func TestRevocableFiberSaveAfterPeerRevocationFails(t *testing.T) {
	storage := session.NewMemoryStorage("revocable:", 0)
	t.Cleanup(func() { _ = storage.Close() })
	settings := session.DefaultConfig().WithCookieName("revocable_id").WithExpiration(time.Hour)
	config, err := fiberadapter.RevocableSessionConfig(session.NewManager(storage, settings))
	if err != nil {
		t.Fatal(err)
	}
	store := fibersession.NewStore(config)
	app := fiber.New()
	app.Get("/seed", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		defer sess.Release()
		sess.Set(session.KeyUserID, "user-1")
		if err := sess.Save(); err != nil {
			return err
		}
		return c.SendString(sess.ID())
	})
	var saveErr error
	var id string
	app.Get("/save", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		defer sess.Release()
		if err := storage.RevokeSession(id); err != nil {
			return err
		}
		sess.Set("role", "viewer")
		saveErr = sess.Save()
		return c.SendStatus(200)
	})
	response, err := app.Test(httptest.NewRequest("GET", "http://example.com/seed", nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("seed %d %v", response.StatusCode, err)
	}
	id = string(raw)
	request := httptest.NewRequest("GET", "http://example.com/save", nil)
	request.AddCookie(&http.Cookie{Name: "revocable_id", Value: id})
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("save %d", response.StatusCode)
	}
	if !errors.Is(saveErr, session.ErrSessionRevoked) {
		t.Fatalf("save after peer revocation: %v", saveErr)
	}
	if payload, err := storage.GetSession(id); err != nil || payload != nil {
		t.Fatalf("revoked session readable: %q %v", payload, err)
	}
}
