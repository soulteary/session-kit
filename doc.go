// Package session provides session storage and management for Go services:
// a storage-backend interface with an in-memory implementation, a Manager for
// server-side session records, a generic key-value session Store, and cookie
// and session helpers that are not tied to any web framework.
//
// # Layout
//
// The root package depends on nothing outside the standard library. Every
// backend or adapter that needs a third-party module lives in a subpackage
// instead, so importing the root package never links a client or a framework
// the service does not use:
//
//   - github.com/soulteary/session-kit/v3/fiberadapter -- the Fiber v3
//     middleware config, the fiber.Storage adapter and *fiber.Cookie, and
//     with them fasthttp.
//   - github.com/soulteary/session-kit/v3/redisstore -- the Redis storage and
//     KV store, and with them go-redis.
//
// A net/http service on in-memory sessions pays nothing for either one
// existing; only importing the subpackage links it in.
//
// # Getting started
//
//	storage := session.NewMemoryStorage("session:", 10*time.Minute)
//	manager := session.NewManager(storage, session.DefaultConfig())
//
//	http.SetCookie(w, session.CreateCookie(manager.GetConfig(), sessionID))
//
// Swapping in Redis is one import and one constructor:
//
//	storage := redisstore.New(redisClient, "session:")
//
// # Working with a session
//
// The [Authenticate], [SetUserID], [HasScope] and similar helpers take the
// smallest interface each one needs -- [Reader], [Writer], [ReadWriter],
// [Saver] or [Session] -- rather than a concrete session type.
// *github.com/gofiber/fiber/v3/middleware/session.Session satisfies all of
// them, so Fiber code calls these helpers with the session it already has:
//
//	sess, err := store.Get(c)
//	session.SetUserID(sess, "user-123")
//	err = session.Authenticate(sess)
//
// Any other type with the same Get, Set, Delete, Save and Destroy methods
// works the same way, which is what keeps Fiber out of this package.
//
// [Regenerator] is the one optional interface: [Authenticate] rotates the
// session ID through it when the session has a Regenerate method, which is
// what keeps an ID planted before login from surviving it. A session type
// that hands an ID to the client should implement it.
//
// # Two session models
//
// [Manager] and [SessionData] are the server-side record model: a JSON
// document per session, saved through a [Storage] backend, with expiry
// enforced on both write and read.
//
// [KVManager] and [Store] are the generic key-value model used by services
// that want Create/Get/Set/Delete/Exists with a TTL and an opaque map of
// values. [redisstore.NewStore] is the Redis implementation.
package session
