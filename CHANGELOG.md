# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Because Go encodes the major version in the import path, every major release
also changes the module path. The current one is
`github.com/soulteary/session-kit/v3`.

## [Unreleased]

## [3.1.0] — 2026-09-21

### Added

- **A configured backend can now be a Cluster or a Sentinel failover setup.**
  `redisstore.New` has accepted every client shape since 3.0.0, but only from
  a caller holding a client it built itself. A caller that *configures* its
  backend — which is what `session.NewStorage` does — could describe a
  standalone server and nothing else: `NewFromConfig`'s four positional
  arguments and `StorageConfig`'s single `RedisAddr` had nowhere to put a
  second address.

  - `redisstore.Options` and `redisstore.NewFromOptions` describe all three
    deployments: `Addr` for one server, `Addrs` for a Cluster's nodes or a
    Sentinel's, `MasterName` to select Sentinel, and `SentinelUsername` /
    `SentinelPassword` for Sentinel's own ACLs — separate from `Password`,
    which authenticates to the Redis server, because the two credentials
    frequently differ and an ACL-protected Sentinel is unreachable without
    them.
  - `session.StorageConfig` gains `RedisAddrs`, `RedisMasterName`,
    `RedisSentinelUsername` and `RedisSentinelPassword`, with
    `WithRedisAddrs`, `WithRedisMasterName` and `WithRedisSentinelAuth`.
    `RedisAddrs` wins over `RedisAddr` when both are set, so
    `DefaultStorageConfig`'s `localhost:6379` cannot shadow a list the caller
    supplied.

  Nothing is removed and no signature changes, so this is a minor release and
  the module path is unchanged. Adding struct fields only breaks an unkeyed
  composite literal, which `go vet` already rejects for a struct from another
  package.

### Changed

- `NewFromConfig` now delegates to `NewFromOptions`. Its arguments land where
  they always did; what goes away is a redundant second `PING` — redis-kit's
  constructors already verify connectivity before returning, and this package
  was pinging again afterwards, so every standalone construction cost two
  round trips instead of one.

- **redis-kit v1.6.0 → v1.7.0.** That release is what makes the above
  possible: it widened its own constructors to `redis.UniversalClient` and
  added `client.NewUniversalClient` along with `Config.Addrs`, `MasterName`
  and the Sentinel credentials. The bump also lifts the minimum this module
  pushes onto its consumers through MVS.

### Tests

- The `StorageConfig` → `Options` handover is asserted field by field against
  the mapping itself, not through a failed dial: "an error came back" would
  pass just as happily with every field dropped, and costs seconds of Sentinel
  retries to learn nothing.
- Runnable examples for the standalone, Cluster, Sentinel and
  from-configuration paths. The Cluster and Sentinel ones carry no `Output:`
  comment — they need a real deployment — but `go test` compiles them.

## [3.0.1] — 2026-09-21

### Fixed — SECURITY

- **`Authenticate` now rotates the session ID (session fixation).** It only
  wrote the authentication markers and saved, leaving the ID untouched. Fiber
  adopts a client-supplied session ID whenever storage holds a record for it,
  so an attacker could mint a real ID, plant it in the victim's browser, and
  have that same ID come back out of login authenticated — their copy then
  granted access to the victim's account. Both READMEs listed ID rotation at
  login as the expected hardening but left it to every caller, and their own
  login examples did not do it.

  Rotation is attempted before the markers are written and a rotation failure
  is returned, so a session that could not be rotated is left unauthenticated
  rather than authenticated under an ID an attacker may already hold. The
  order matters on Fiber's middleware path, where `Save` is a no-op and the
  middleware persists the session when the handler returns.

### Added

- **`Regenerator`**, a session that can rotate its own ID. `Authenticate`
  rotates through it whenever the session offers it;
  `*middleware/session.Session` does, so Fiber code is fixed with no change at
  the call site. Rotation is an optional capability rather than a method on
  `Saver` because `Saver` is part of the released v3 API, and because a
  session that hands the client no ID — the `memorySession` in
  `ExampleAuthenticate`, for one — has nothing to rotate and cannot be
  fixated. A session type that does expose an ID should implement
  `Regenerate`.

### Changed

- The session ID now always changes at login. Anything holding the pre-login
  ID externally will see it rotate; that is the fix, not a regression.

## [3.0.0] — 2026-09-21

### Changed — BREAKING

- **Fiber moved to the `fiberadapter` subpackage and Redis to `redisstore`.**
  The root package now depends on nothing outside the standard library, so a
  binary that keeps sessions in memory links neither Fiber nor go-redis.

  Measured against v2.3.0, for a program that imports only the root package and
  builds with `-trimpath -ldflags="-s -w"`:

  | | v2.3.0 | v3.0.0 |
  |---|---|---|
  | Linked packages | 297 | 200 |
  | Modules in the build | 22 | 2 |
  | Binary size | 5,992,711 B | 4,518,151 B |
  | `// indirect` lines in the consumer's `go.mod` | 20 | 0 |
  | Module entries in the consumer's `go.sum` | 32 | 0 |

  That is 97 fewer linked packages and a 24.6% smaller binary. The consumer's
  own `go.mod` ends up with no `// indirect` requirement at all, and its
  `go.sum` empties: Fiber, go-redis, redis-kit and miniredis leave, and so do
  the 28 modules they drag along — fasthttp, gofiber/schema, gofiber/utils,
  klauspost/compress, klauspost/cpuid, tinylib/msgp, philhofer/fwd,
  shamaton/msgpack, molecule-man/go-brrr, valyala/bytebufferpool,
  mattn/go-colorable, mattn/go-isatty, google/uuid, cespare/xxhash,
  zeebo/xxh3, go.uber.org/atomic, yuin/gopher-lua, fxamacker/cbor,
  x448/float16, stretchr/testify, bsm/ginkgo, bsm/gomega, go.yaml.in/yaml,
  golang.org/x/crypto, x/net, x/sys, x/text and x/tools.

  A subpackage is enough; neither dependency needs its own module. Module
  graph pruning keeps a requirement that no imported package needs out of the
  consumer's `go.mod` and `go.sum` entirely. What it does not remove is the
  **minimum version**: the requirements stay in the module graph, so a
  consumer that pulls go-redis in from somewhere else still gets at least the
  version named here through MVS.

  Keeping deprecated shims in the root package was not an option: a shim has
  to import the package it forwards to, which relinks the dependency and gives
  back the entire benefit.

  **The module path is therefore now `github.com/soulteary/session-kit/v3`**,
  by Go's import compatibility rule. Every user must update the import path,
  including services that use neither Fiber nor Redis.

  | Removed from the root package | Replacement |
  |---|---|
  | `(*session.Manager).FiberSessionConfig()` | `fiberadapter.SessionConfig(manager)` |
  | `session.RedisStorage` | `redisstore.Storage` |
  | `session.NewRedisStorage` | `redisstore.New` |
  | `session.NewRedisStorageFromConfig` | `redisstore.NewFromConfig` |
  | `session.RedisStore` | `redisstore.Store` |
  | `session.NewRedisStore` | `redisstore.NewStore` |
  | `(*RedisStorage).GetClient` | `(*redisstore.Storage).Client` |
  | `(*RedisStorage).GetKeyPrefix` | `(*redisstore.Storage).KeyPrefix` |
  | `(*RedisStorage).GetTTL` | `(*redisstore.Storage).TTL` |
  | `StorageConfig.RedisClient`, `WithRedisClient` | `redisstore.New(client, prefix)` |

- **The session helpers take interfaces instead of `*fibersession.Session`.**
  The twenty-one helpers — `Authenticate`, `Unauthenticate`,
  `IsAuthenticated`, the `Get*`/`Set*` pairs, `AddAMR`, `HasAMR`, `SetScopes`,
  `GetScopes`, `HasScope`, `UpdateLastAccess` — now take the smallest
  interface each one needs: `Reader` (just `Get`), `Writer` (just `Set`),
  `ReadWriter`, `Saver` (`Set` plus `Save`) or `Session` (all five methods).

  `*github.com/gofiber/fiber/v3/middleware/session.Session` satisfies every
  one of them as it stands, so **existing call sites compile unchanged** —
  this is the change that lets the root package drop Fiber without moving the
  helpers out of it. Any other session type with the same `Get`, `Set`,
  `Delete`, `Save` and `Destroy` now works too, which it could not before.

- **`CreateCookie` returns a `*net/http.Cookie`** rather than a
  `*fiber.Cookie`. `http.SetCookie` takes it as is, and every other Go web
  framework accepts or converts one. Fiber users call `fiberadapter.Cookie`
  for the Fiber type; both are built from the same rules, which
  `TestCookieMatchesCreateCookie` holds them to.

  The field spelling differs between the two types: `HttpOnly` on
  `http.Cookie`, `HTTPOnly` on `fiber.Cookie`, and `SameSite` is an
  `http.SameSite` rather than a string. `"Disabled"` maps to
  `http.SameSiteDefaultMode`, which omits the attribute — the meaning Fiber
  spells `fiber.CookieSameSiteDisabled`.

- **`NewStorage` and `NewStorageFromEnv` need `redisstore` imported to build a
  Redis backend.** Only `StorageTypeMemory` is built in; the rest come from
  [`RegisterStorage`], which `redisstore` calls from an `init` function. Add
  the import and both keep working unchanged:

  ```go
  import _ "github.com/soulteary/session-kit/v3/redisstore"
  ```

  That import is what links go-redis in, which is the whole point. Without
  it the error says so by name rather than reporting an unknown type. Code
  holding a client already should skip the factory: `redisstore.New(client,
  "session:")` returns a `session.Storage` that `NewManager` takes directly.

### Added

- The `fiberadapter` subpackage: `SessionConfig` (the old
  `FiberSessionConfig`), `Storage` (the `fiber.Storage` adapter, previously an
  unexported type), `Cookie` and `SameSite`.
- The `redisstore` subpackage. `redisstore.Client` is the part of a go-redis
  client the storage uses, so `*redis.Client`, `*redis.ClusterClient`,
  `*redis.Ring` and `redis.UniversalClient` all work where the root package
  took only `*redis.Client` — a Cluster or Sentinel deployment no longer needs
  a storage of its own.
- `session.RegisterStorage` and `session.StorageBuilder`, the hook
  `redisstore` registers through. It takes a Memcached, DynamoDB or in-house
  backend just as well, so `NewStorage` is no longer limited to the two
  backends this module ships.
- `Config.SameSiteMode` and `Config.CookieSecure`, which name the two cookie
  rules that were previously written out separately in `CreateCookie` and
  `FiberSessionConfig`: how `Config.SameSite` maps onto a real SameSite value,
  and that `SameSite=None` forces `Secure` on. Adapters read them instead of
  repeating the rules, because a SameSite rule that disagrees with itself
  across frameworks is a CSRF hole, not a cosmetic difference.
- A package doc in `doc.go` describing the layout, the two session models
  (`Manager`/`SessionData` versus `KVManager`/`Store`) and how the helper
  interfaces work.
- Runnable examples in all three packages that `go test` verifies, so they
  cannot drift from the API.
- `CHANGELOG.md` and `.github/workflows/release.yml`. Thirteen tags exist with
  nothing having checked any of them, and the mistake a `/vN` module makes
  once is caught at tag time or not at all. The gate runs on a `v*` tag (and
  on demand): the module path must carry the tag's major version, with v0 and
  v1 taking no suffix, and both READMEs' `go get` line must name that same
  path. Then the CI gate against the tagged commit — gofmt, `go mod tidy`
  cleanliness, vet, golangci-lint, `go test -race` with coverage, and
  govulncheck. Verification only: it publishes nothing and takes no write
  permissions.
- `.github/dependabot.yml`. Weekly gomod and github-actions updates, minor and
  patch grouped into one PR, majors left separate — for this module a
  dependency major is a judgement call. The release gate is an action too, so
  a silently stale action would be a stale release check.

### Fixed

- `Unauthenticate` handled `nil` but not a **typed** nil. Its parameter is now
  an interface, where the nil that actually arrives is a nil
  `*fibersession.Session` inside a non-nil interface — from an unassigned
  field, or a helper that returned early on an error. A plain `session == nil`
  misses that and the next `Set` panics, so the check goes through `reflect`
  instead. `redisstore` guards its `Client` the same way, which also fixes the
  equivalent hazard for a nil `*redis.Client`.
- `(*redisstore.Storage).Close` no longer assumes it owns a `*redis.Client`. A
  `Client` that does not expose `Close` — a deliberately shared handle — makes
  it a no-op instead of a compile error.
- Both READMEs described `TouchSession` as `manager.TouchSession(ctx,
  sessionID)`. It takes a `*SessionData` and no context.
- Both READMEs still said `Unauthenticate` "clears the identity keys, saves
  that cleared state, and then destroys the session". Since v2.2.0 it destroys
  first and saves only when that fails, which avoids writing data that is
  about to be deleted.

### Changed

- Test coverage is 97.0%, up from 94.9%: the root package and `fiberadapter`
  are at 100%, `redisstore` at 90.9% (the remainder is Redis error paths).
- `gofiber/schema` 1.8.6 → 1.8.7, `gofiber/utils/v2` 2.5.1 → 2.5.2,
  `molecule-man/go-brrr` 1.1.0 → 1.1.1, `go.uber.org/atomic` 1.11.0 → 1.12.0.
  All indirect, all patch or minor. Fiber v3.5.0, go-redis v9.22.0,
  miniredis v2.39.0 and redis-kit v1.6.0 are already the latest published
  releases.

## [2.3.0] — 2026-09-14

### Changed

- Dependency refresh only. No API was removed and no call needed rewriting.
  Test Redis moved to `miniredis` v2.39.0 (was v2.36.1); `redis-kit` stayed at
  v1.6.0, which had no newer published release.

## [2.2.0] — 2026-09-12

### Changed — BREAKING

- `session.Config{}` no longer validates. The zero value was accepted as
  "nothing configured yet", so it passed `Validate()` and produced an unnamed
  cookie with `Secure=false` and `HTTPOnly=false`. Start from
  `DefaultConfig()`.

### Fixed

- Scopes and AMR survive storage. `GetScopes` and `GetAMR` asserted
  `val.([]string)`, which succeeded only inside the request that wrote the
  value: Fiber serialises session data with msgpack, which decodes an array
  back as `[]interface{}`. On every later request they silently returned
  nothing and `HasScope`/`HasAMR` always reported false.
- `SaveSession` refuses an already-expired session. Writing one granted it
  another full lifetime, so a session that kept being written never expired.
- `Unauthenticate` persists the cleared identity rather than clearing it only
  in memory, so a failed `Destroy` no longer leaves a fully authenticated
  session in storage.
- `MemoryStorage.Get` returns a copy. It copied on write but returned its
  internal slice on read, so a caller mutating what it read corrupted the
  stored session for every later reader, and raced with concurrent writers.
- `MemoryStorage.Set` with an empty value deletes the key rather than ignoring
  the write, which had kept the old, still-authenticated payload readable.
  `RedisStorage` was given the same behaviour so the two backends agree.
- `MemoryStorage.Close` is idempotent. A second call panicked.

[Unreleased]: https://github.com/soulteary/session-kit/compare/v3.0.1...HEAD
[3.0.1]: https://github.com/soulteary/session-kit/compare/v3.0.0...v3.0.1
[3.0.0]: https://github.com/soulteary/session-kit/compare/v2.3.0...v3.0.0
[2.3.0]: https://github.com/soulteary/session-kit/compare/v2.2.0...v2.3.0
[2.2.0]: https://github.com/soulteary/session-kit/compare/v2.1.0...v2.2.0
