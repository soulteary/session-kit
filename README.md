# session-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/session-kit/v3.svg)](https://pkg.go.dev/github.com/soulteary/session-kit/v3)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/session-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/session-kit)

[中文文档](README_CN.md)

A Go library for session management: a storage-backend interface with an
in-memory implementation, a manager for server-side session records, a generic
key-value session store, and cookie and session helpers that are not tied to
any web framework.

## Layout

**The root package depends on nothing outside the standard library.** Every
backend or adapter that needs a third-party module lives in a subpackage, so
importing the root package never links a client or a framework the service
does not use:

| Package | Brings in | Provides |
|---|---|---|
| `github.com/soulteary/session-kit/v3` | *(standard library only)* | `Storage`, `MemoryStorage`, `Manager`, `SessionData`, `Store`, `KVManager`, `Config`, `CreateCookie` and the session helpers |
| `.../v3/fiberadapter` | Fiber v3, fasthttp | `SessionConfig`, `Storage`, `Cookie`, `SameSite` |
| `.../v3/redisstore` | go-redis, redis-kit | `Storage`, `New`, `NewFromConfig`, `Store`, `NewStore`, `Client` |

A net/http service on in-memory sessions pays nothing for either subpackage
existing. Measured for a program that imports only the root package, against
v2.3.0: 97 fewer linked packages, 20 fewer modules and a 24.6% smaller binary,
with **no** `// indirect` requirement left in the consumer's own `go.mod`. See
[CHANGELOG.md](CHANGELOG.md) for the full table.

## Features

- **Framework-agnostic core**: the session helpers take small interfaces, so any
  session type with `Get`/`Set`/`Delete`/`Save`/`Destroy` works — Fiber v3's
  included, unchanged
- **Multiple storage backends**: memory built in, Redis in `redisstore`, and
  `RegisterStorage` for your own
- **Fiber v3 support**: `fiberadapter` wires the middleware and adapts
  `fiber.Storage`
- **Session management**: high-level operations over a `SessionData` record
- **Fluent configuration**: builder pattern throughout
- **Automatic expiration**: TTL-based, enforced on both write and read
- **Thread-safe**: safe for concurrent access

## Requirements

- **Go 1.27+** (`go.mod` declares `go 1.27.0`)
- `github.com/gofiber/fiber/v3` v3.5.0+ — only for `fiberadapter`
- `github.com/redis/go-redis/v9` — only for `redisstore`

This v3 module line targets Fiber v3. Applications still on Fiber v2 should
remain on `github.com/soulteary/session-kit` v1.

## Installation

```bash
go get github.com/soulteary/session-kit/v3
```

Upgrading from v2? See [Upgrade notes (v3.0.0)](#upgrade-notes-v300) — the
import path changes for everyone, but most call sites do not.

## Quick start

### Memory storage (development)

```go
package main

import (
    "time"

    session "github.com/soulteary/session-kit/v3"
)

func main() {
    storage := session.NewMemoryStorage("session:", 10*time.Minute)
    defer storage.Close()

    manager := session.NewManager(storage, session.DefaultConfig())

    record := manager.CreateSession("session-123")
    record.UserID = "user-456"
    record.Authenticated = true
    _ = manager.SaveSession(record)

    loaded, _ := manager.LoadSession("session-123")
    _ = loaded
}
```

### Redis storage (production)

Redis lives in `redisstore`, so this import is what links go-redis in:

```go
package main

import (
    "github.com/redis/go-redis/v9"

    session "github.com/soulteary/session-kit/v3"
    "github.com/soulteary/session-kit/v3/redisstore"
)

func main() {
    client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

    storage := redisstore.New(client, "myapp:session:")
    defer storage.Close()

    manager := session.NewManager(storage, session.DefaultConfig())
    _ = manager
}
```

`redisstore.New` takes a `redisstore.Client`, the handful of commands the
storage actually uses — so `*redis.Client`, `*redis.ClusterClient`,
`*redis.Ring` and `redis.UniversalClient` all work. A Cluster or Sentinel
deployment needs no storage of its own.

To have the storage open the connection for you, including a connectivity
check:

```go
storage, err := redisstore.NewFromConfig("localhost:6379", "", 0, "myapp:session:")
```

### Choosing a backend from configuration

`NewStorage` builds memory storage on its own. Every other backend has to be
registered first, which for Redis means importing the subpackage for its side
effect:

```go
import (
    session "github.com/soulteary/session-kit/v3"
    _ "github.com/soulteary/session-kit/v3/redisstore"
)

cfg := session.DefaultStorageConfig().
    WithType(session.StorageTypeRedis).
    WithRedisAddr("localhost:6379").
    WithKeyPrefix("myapp:session:")

storage, err := session.NewStorage(cfg)
```

Without that import, `NewStorage` says so by name instead of failing as an
unknown type. `RegisterStorage` is exported, so a Memcached, DynamoDB or
in-house backend plugs into the same factory.

### Using with Fiber

```go
package main

import (
    "time"

    "github.com/gofiber/fiber/v3"
    fibersession "github.com/gofiber/fiber/v3/middleware/session"

    session "github.com/soulteary/session-kit/v3"
    "github.com/soulteary/session-kit/v3/fiberadapter"
)

func main() {
    app := fiber.New()

    storage := session.NewMemoryStorage("session:", 0)
    config := session.DefaultConfig().
        WithCookieName("my_session").
        WithExpiration(24 * time.Hour)

    manager := session.NewManager(storage, config)

    store := fibersession.NewStore(fiberadapter.SessionConfig(manager))

    app.Get("/", func(c fiber.Ctx) error {
        sess, _ := store.Get(c)

        if session.IsAuthenticated(sess) {
            return c.SendString("Hello, " + session.GetUserID(sess))
        }

        return c.SendString("Please login")
    })

    app.Post("/login", func(c fiber.Ctx) error {
        sess, _ := store.Get(c)

        session.SetUserID(sess, "user-123")
        session.SetEmail(sess, "user@example.com")
        session.AddAMR(sess, "pwd")

        // Rotates the session ID first, then marks the session authenticated.
        return session.Authenticate(sess)
    })

    app.Listen(":3000")
}
```

Note that the helpers come from the **root** package, not from `fiberadapter`.
That is the point of the interfaces below.

## Session helpers and the interfaces they take

Each helper takes the smallest interface it needs, rather than a concrete
session type:

| Interface | Methods | Used by |
|---|---|---|
| `Reader` | `Get` | every getter, `IsAuthenticated`, `HasAMR`, `HasScope` |
| `Writer` | `Set` | every setter, `UpdateLastAccess` |
| `ReadWriter` | `Get`, `Set` | `AddAMR` |
| `Saver` | `Set`, `Save` | `Authenticate` |
| `Session` | `Get`, `Set`, `Delete`, `Save`, `Destroy` | `Unauthenticate` |
| `Regenerator` | `Regenerate` | `Authenticate`, when the session offers it |

`*github.com/gofiber/fiber/v3/middleware/session.Session` satisfies all six as
it stands, which is why Fiber code passes the session it already has — and why
the root package does not import Fiber. Any other type with the same five
methods works too:

```go
type mySession struct{ values map[any]any }

func (s *mySession) Get(key any) any  { return s.values[key] }
func (s *mySession) Set(key, val any) { s.values[key] = val }
func (s *mySession) Delete(key any)   { delete(s.values, key) }
func (s *mySession) Save() error      { return nil }
func (s *mySession) Destroy() error   { clear(s.values); return nil }
```

`Regenerator` is the one optional interface. `Authenticate` rotates the session
ID through it whenever the session has a `Regenerate() error` method, which is
what keeps an ID planted before login from surviving it (session fixation). It
is a capability rather than a line in `Saver` because `Saver` is part of the
released v3 API, and because a session like `mySession` above hands the client
no ID of its own: there is nothing to rotate and nothing an attacker can fix in
advance. **If your session type does hand an ID to the client, implement
`Regenerate`** — without it `Authenticate` has no way to rotate, and the ID the
caller arrived with stays in place across login.

```go
// Authentication
session.Authenticate(sess)      // Rotate the ID (when supported), mark authenticated, save
session.Unauthenticate(sess)    // Clear the identity and destroy
session.IsAuthenticated(sess)   // Check if authenticated

// User info
session.SetUserID(sess, "user-123")
session.GetUserID(sess)

session.SetEmail(sess, "user@example.com")
session.GetEmail(sess)

session.SetPhone(sess, "+1234567890")
session.GetPhone(sess)

// AMR (Authentication Methods References)
session.SetAMR(sess, []string{"pwd", "otp"})
session.GetAMR(sess)
session.AddAMR(sess, "pwd")
session.HasAMR(sess, "pwd")

// Scopes
session.SetScopes(sess, []string{"read", "write"})
session.GetScopes(sess)
session.HasScope(sess, "read")

// Timestamps
session.UpdateLastAccess(sess)
session.GetLastAccess(sess)
session.GetCreatedAt(sess)
```

## Configuration

### Session config

```go
cfg := session.DefaultConfig().
    WithExpiration(24 * time.Hour).   // Session duration
    WithCookieName("my_session").     // Cookie name
    WithCookieDomain(".example.com"). // Cookie domain
    WithCookiePath("/").              // Cookie path
    WithSecure(true).                 // HTTPS only
    WithHTTPOnly(true).               // No JS access
    WithSameSite("Lax").              // SameSite policy
    WithKeyPrefix("myapp:session:")   // Storage key prefix
```

`cfg.SameSiteMode()` returns the SameSite attribute as an `http.SameSite`, and
`cfg.CookieSecure()` reports whether the cookie must carry `Secure`. Adapters
read those two instead of interpreting `Config.SameSite` themselves: a SameSite
rule that disagrees with itself across frameworks is a CSRF hole, not a
cosmetic difference.

### Security notes

- **Start from `DefaultConfig()`**, not `session.Config{}`. The zero value does
  not validate: it would produce an unnamed cookie with `Secure=false` and
  `HTTPOnly=false`, and a validator that approves the least safe configuration
  available gives false assurance.
- **Validate configuration**: call `cfg.Validate()` before use to catch unsafe or
  invalid combinations (e.g. `SameSite=None` without `Secure=true`).
- **SameSite behavior**: Supported values are `Strict`, `Lax`, `None`, and
  `Disabled`. Use `None` only when cross-site requests are required, and always
  with `Secure=true` — `CookieSecure()` forces it on regardless, because a
  `SameSite=None` cookie without `Secure` is dropped by browsers rather than
  relaxed.
- **Login hardening**: `Authenticate()` rotates the session ID itself, so an ID
  planted in the victim's browser before login cannot survive it (session
  fixation). Rotation is attempted before the authentication markers are
  written and a rotation failure is returned, so a session that could not be
  rotated is deliberately left unauthenticated — check the error. Sessions that
  expose no ID are left alone; see `Regenerator` above if yours does.
- **Redis hardening**: Treat Redis as a trusted backend—use network isolation and credentials, and add timeouts at the client layer to prevent resource exhaustion.

### Storage config

```go
cfg := session.DefaultStorageConfig().
    WithType(session.StorageTypeRedis).      // memory or redis
    WithKeyPrefix("session:").               // Key prefix
    WithRedisAddr("localhost:6379").         // Redis address
    WithRedisPassword("secret").             // Redis password
    WithRedisDB(0).                          // Redis database
    WithMemoryGCInterval(10 * time.Minute)   // Memory GC interval
```

There is no `WithRedisClient`: a config describes a connection the factory
should open. Code that already holds a client calls `redisstore.New(client,
prefix)` directly, which returns a `session.Storage` that `NewManager` takes
as is.

## Session data

The `SessionData` struct provides a rich model for session data:

```go
record := session.NewSessionData("session-123", time.Hour)

// User info
record.UserID = "user-456"
record.Email = "user@example.com"
record.Phone = "+1234567890"
record.Authenticated = true

// Authentication methods (AMR)
record.AddAMR("pwd")     // Password
record.AddAMR("otp")     // OTP
record.HasAMR("pwd")     // Check if has method

// Authorization scopes
record.AddScope("read")
record.AddScope("write")
record.HasScope("read")  // Check if has scope

// Custom data
record.SetValue("custom", "value")
val, ok := record.GetValue("custom")

// State checks
record.IsExpired()
record.IsAuthenticated()
record.Touch()  // Update last access time
```

## Session lifecycle

### Expiry is not renewed by saving

`SaveSession` **refuses an already-expired session**. Writing one used to grant it
another full lifetime, so a session that kept being written never expired.

`TouchSession` is the deliberate way to extend a session. It takes the record,
not an ID:

```go
if err := manager.TouchSession(record); err != nil {
    // the session is gone or expired; re-authenticate
}
```

### Logout

`Unauthenticate` clears the identity keys, destroys the session, and persists
the cleared state only if the destroy fails — so a failed `Destroy` still
leaves a de-authenticated session rather than a fully authenticated one:

```go
if err := session.Unauthenticate(sess); err != nil {
    // The session is no longer authenticated even if Destroy failed —
    // the cleared state was persisted as the fallback.
    log.Printf("logout: %v", err)
}
```

The clearing used to live only in memory, so when `Destroy` failed the stored
session was untouched and still fully authenticated — exactly the case the
safeguard was there to cover.

A `nil` session is handled, including a **typed** nil such as an unassigned
`*fibersession.Session` field. The parameter is an interface, where a plain
`== nil` check would miss that and panic on the next write.

### Scopes and AMR survive a round trip

Fiber v3 serialises session data with msgpack, which decodes an array back as
`[]interface{}` rather than `[]string`. `GetScopes`, `GetAMR`, `HasScope` and
`HasAMR` accept either representation, so a scope set on one request is still
visible on the next:

```go
session.SetScopes(sess, []string{"read", "write"})
// on a later request:
session.HasScope(sess, "read") // true
```

### Session sharing (cross-domain)

To share a session ID with another domain (e.g. subdomain or partner app),
build a cookie with `session.CreateCookie(config, sessionID)` and set it on the
response. It returns a `*net/http.Cookie`, so `http.SetCookie` takes it as is:

```go
http.SetCookie(w, session.CreateCookie(config, sessionID))
```

Fiber users want a `*fiber.Cookie` instead:

```go
c.Cookie(fiberadapter.Cookie(config, sessionID))
```

Both are built from the same rules, `SameSite=None` forcing `Secure` among
them.

## Server-side KV sessions (Store)

For services that need a generic key-value session store (e.g. server-side challenge/session records), use the `Store` interface and `KVManager`:

- **Store**: `Create`, `Get`, `Set`, `Delete`, `Exists` with TTL.
- **KVManager**: Wraps a `Store` and provides default TTL plus `Refresh`.
- **redisstore.Store**: Redis implementation of `Store`; use `redisstore.NewStore(client, keyPrefix)`.

```go
// redisClient is a redisstore.Client; require "context" and "time" imports.
ctx := context.Background()
store := redisstore.NewStore(redisClient, "myapp:session:")
mgr := session.NewKVManager(store, 10*time.Minute)

id, _ := mgr.Create(ctx, map[string]interface{}{"user_id": "u1"}, 0)
rec, _ := mgr.Get(ctx, id)
_ = mgr.Set(ctx, id, map[string]interface{}{"user_id": "u1", "step": 2}, 0)
_ = mgr.Refresh(ctx, id, 0)
_ = mgr.Delete(ctx, id)
```

## Factory helpers

- **NewStorageFromEnv(redisEnabled, redisAddr, redisPassword, redisDB, keyPrefix)** — build Storage from env-like flags (memory if `redisEnabled` is false). Redis needs `redisstore` imported.
- **MustNewStorage(cfg)** — same as `NewStorage(cfg)` but panics on error (e.g. in `main()`).
- **RegisterStorage(type, builder)** — teach `NewStorage` about a backend this module does not ship.

## Upgrade notes (v3.0.1)

Security fix. No API was removed and no call needs rewriting.

- `Authenticate` rotates the session ID before it marks the session
  authenticated, closing a session fixation hole. Fiber code gets this with no
  change at the call site, because `*middleware/session.Session` already has
  `Regenerate() error`.
- The session ID therefore always changes at login. Anything holding the
  pre-login ID externally will see it rotate — that is the fix, not a
  regression.
- A rotation failure is returned instead of the session being marked
  authenticated, so check the error `Authenticate` gives you.
- New `Regenerator` interface. **If your own session type hands an ID to the
  client, implement `Regenerate() error`** — without it there is nothing for
  `Authenticate` to rotate, and the ID the caller arrived with stays in place
  across login.

## Upgrade notes (v3.0.0)

The import path changes for everyone. Most call sites do not.

**Unchanged:** every session helper (`Authenticate`, `IsAuthenticated`,
`SetUserID`, `HasScope`, …). They take interfaces now, and
`*fibersession.Session` satisfies them as it stands, so the calls compile
exactly as before. `Manager`, `SessionData`, `Storage`, `MemoryStorage`,
`Store`, `KVManager` and `Config` are unchanged too.

**Changed:**

| Before | After |
|---|---|
| `github.com/soulteary/session-kit/v2` | `github.com/soulteary/session-kit/v3` |
| `manager.FiberSessionConfig()` | `fiberadapter.SessionConfig(manager)` |
| `session.NewRedisStorage(client, prefix)` | `redisstore.New(client, prefix)` |
| `session.NewRedisStorageFromConfig(...)` | `redisstore.NewFromConfig(...)` |
| `session.NewRedisStore(client, prefix)` | `redisstore.NewStore(client, prefix)` |
| `session.RedisStorage` / `session.RedisStore` | `redisstore.Storage` / `redisstore.Store` |
| `storage.GetClient()` / `GetKeyPrefix()` / `GetTTL(k)` | `storage.Client()` / `KeyPrefix()` / `TTL(k)` |
| `cfg.WithRedisClient(client)` | `redisstore.New(client, prefix)` |
| `CreateCookie` returned `*fiber.Cookie` | returns `*http.Cookie`; `fiberadapter.Cookie` returns `*fiber.Cookie` |
| `session.NewStorage(cfg)` with `Type: redis` | same, plus `import _ ".../v3/redisstore"` |

No compatibility shims are provided, deliberately: a shim in the root package
would have to import the package it forwards to, which relinks Fiber or
go-redis and gives back the entire benefit of the split.

## Upgrade notes (v2.3.0)

Dependency refresh only. No API was removed and no call needs rewriting.

- Test Redis is `miniredis` v2.39.0 (was v2.36.1).
- Still requires `redis-kit` v1.6.0. That module had no newer published release.

## Upgrade notes (v2.2.0)

No API was added or removed. Four behaviours change, and one configuration that
used to validate no longer does.

- **`session.Config{}` no longer validates.** The zero value was accepted as
  "nothing configured yet", so it passed `Validate()` and produced an unnamed
  cookie with `Secure=false` and `HTTPOnly=false`. **Start from
  `DefaultConfig()`** — if you built a `Config` literally and relied on
  `Validate()` returning nil, it now reports the empty cookie name.
- **Scopes and AMR survive storage.** `GetScopes` and `GetAMR` asserted
  `val.([]string)`, which succeeded only inside the request that wrote the value:
  msgpack decodes an array back as `[]interface{}`, so on **every later request
  they silently returned nothing and `HasScope`/`HasAMR` always reported false**.
  If you worked around that — re-deriving scopes per request, or treating
  `HasScope` as unreliable — you can stop.
- **`SaveSession` refuses an expired session.** Writing one granted it another
  full lifetime, so a session that kept being written never expired. Use
  `TouchSession` to extend one deliberately. **A save that used to succeed now
  returns an error**, which is the correct answer.
- **Logout persists before destroying.** `Unauthenticate` cleared the identity
  keys "in case Destroy fails" and never saved them, so a failed `Destroy` left the
  stored session fully authenticated. The cleared state is now persisted as the
  fallback, and a `Save` failure is reported alongside a `Destroy` failure
  instead of being discarded.
- **`MemoryStorage.Get` returns a copy.** It copied on write but returned its
  internal slice on read, so a caller mutating what it read corrupted the stored
  session for every later reader, and raced with concurrent writers.
- **`MemoryStorage.Set` with an empty value deletes the key.** It ignored the
  write, leaving the previous payload in place — so overwriting a session with
  empty data kept the old, still authenticated bytes readable.
- **`MemoryStorage.Close` is idempotent.** A second call panicked.

## Testing

```bash
go test ./...
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out
```

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
