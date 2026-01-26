# session-kit

A Go library for session management with support for memory and Redis storage backends. Compatible with Fiber v2's session middleware.

## Features

- **Multiple Storage Backends**: Memory (for development) and Redis (for production)
- **Fiber v2 Compatible**: Implements `fiber.Storage` interface
- **Factory Pattern**: Easy storage creation with configuration
- **Session Management**: High-level session operations with SessionData struct
- **Fluent Configuration**: Builder pattern for easy configuration
- **Automatic Expiration**: TTL-based session expiration
- **Thread-Safe**: Safe for concurrent access

## Installation

```bash
go get github.com/soulteary/session-kit
```

## Quick Start

### Memory Storage (Development)

```go
package main

import (
    "time"
    session "github.com/soulteary/session-kit"
)

func main() {
    // Create memory storage
    storage := session.NewMemoryStorage("session:", 10*time.Minute)
    defer storage.Close()

    // Use storage
    storage.Set("session-123", []byte("data"), time.Hour)
    data, _ := storage.Get("session-123")
}
```

### Redis Storage (Production)

```go
package main

import (
    session "github.com/soulteary/session-kit"
)

func main() {
    // Create Redis storage
    storage, err := session.NewRedisStorageFromConfig(
        "localhost:6379",  // addr
        "",                // password
        0,                 // db
        "myapp:session:",  // key prefix
    )
    if err != nil {
        panic(err)
    }
    defer storage.Close()
}
```

### Using Factory Pattern

```go
package main

import (
    session "github.com/soulteary/session-kit"
)

func main() {
    // Create storage from configuration
    cfg := session.DefaultStorageConfig().
        WithType(session.StorageTypeRedis).
        WithRedisAddr("localhost:6379").
        WithKeyPrefix("myapp:session:")

    storage, err := session.NewStorage(cfg)
    if err != nil {
        panic(err)
    }
    defer storage.Close()
}
```

### Using with Fiber

```go
package main

import (
    "github.com/gofiber/fiber/v2"
    fibersession "github.com/gofiber/fiber/v2/middleware/session"
    session "github.com/soulteary/session-kit"
)

func main() {
    app := fiber.New()

    // Create session manager
    storage := session.NewMemoryStorage("session:", 0)
    config := session.DefaultConfig().
        WithCookieName("my_session").
        WithExpiration(24 * time.Hour)

    manager := session.NewManager(storage, config)

    // Create Fiber session store
    store := fibersession.New(manager.FiberSessionConfig())

    app.Get("/", func(c *fiber.Ctx) error {
        sess, _ := store.Get(c)

        if session.IsAuthenticated(sess) {
            return c.SendString("Hello, " + session.GetUserID(sess))
        }

        return c.SendString("Please login")
    })

    app.Post("/login", func(c *fiber.Ctx) error {
        sess, _ := store.Get(c)

        session.SetUserID(sess, "user-123")
        session.SetEmail(sess, "user@example.com")
        session.AddAMR(sess, "pwd")
        session.Authenticate(sess)

        return c.SendString("Logged in")
    })

    app.Listen(":3000")
}
```

## Configuration

### Session Config

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

### Storage Config

```go
cfg := session.DefaultStorageConfig().
    WithType(session.StorageTypeRedis).      // memory or redis
    WithKeyPrefix("session:").               // Key prefix
    WithRedisAddr("localhost:6379").         // Redis address
    WithRedisPassword("secret").             // Redis password
    WithRedisDB(0).                          // Redis database
    WithMemoryGCInterval(10 * time.Minute)   // Memory GC interval
```

## Session Data

The `SessionData` struct provides a rich model for session data:

```go
session := session.NewSessionData("session-123", time.Hour)

// User info
session.UserID = "user-456"
session.Email = "user@example.com"
session.Phone = "+1234567890"
session.Authenticated = true

// Authentication methods (AMR)
session.AddAMR("pwd")     // Password
session.AddAMR("otp")     // OTP
session.HasAMR("pwd")     // Check if has method

// Authorization scopes
session.AddScope("read")
session.AddScope("write")
session.HasScope("read")  // Check if has scope

// Custom data
session.SetValue("custom", "value")
val, ok := session.GetValue("custom")

// State checks
session.IsExpired()
session.IsAuthenticated()
session.Touch()  // Update last access time
```

## Fiber Session Helpers

Helper functions for working with Fiber sessions:

```go
// Authentication
session.Authenticate(sess)      // Mark as authenticated
session.Unauthenticate(sess)    // Destroy session
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

## License

Apache License 2.0
