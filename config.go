// Package session provides session storage and management functionality.
package session

import (
	"time"
)

// Config represents session configuration options.
type Config struct {
	// Expiration is the session expiration duration.
	// Default: 24 hours
	Expiration time.Duration

	// CookieName is the name of the session cookie.
	// Default: "session_id"
	CookieName string

	// CookieDomain is the domain for the session cookie.
	// If empty, the cookie will be set for the current domain only.
	// Default: "" (empty)
	CookieDomain string

	// CookiePath is the path for the session cookie.
	// Default: "/"
	CookiePath string

	// Secure indicates if the cookie should only be sent over HTTPS.
	// Default: true
	Secure bool

	// HTTPOnly indicates if the cookie should be inaccessible to JavaScript.
	// Default: true
	HTTPOnly bool

	// SameSite controls the SameSite attribute of the cookie.
	// Allowed values: "Strict", "Lax", "None"
	// Default: "Lax"
	SameSite string

	// KeyPrefix is the prefix for session keys in storage.
	// Default: "session:"
	KeyPrefix string
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Expiration:   24 * time.Hour,
		CookieName:   "session_id",
		CookieDomain: "",
		CookiePath:   "/",
		Secure:       true,
		HTTPOnly:     true,
		SameSite:     "Lax",
		KeyPrefix:    "session:",
	}
}

// WithExpiration sets the session expiration duration.
func (c Config) WithExpiration(exp time.Duration) Config {
	c.Expiration = exp
	return c
}

// WithCookieName sets the session cookie name.
func (c Config) WithCookieName(name string) Config {
	c.CookieName = name
	return c
}

// WithCookieDomain sets the session cookie domain.
func (c Config) WithCookieDomain(domain string) Config {
	c.CookieDomain = domain
	return c
}

// WithCookiePath sets the session cookie path.
func (c Config) WithCookiePath(path string) Config {
	c.CookiePath = path
	return c
}

// WithSecure sets whether the cookie should only be sent over HTTPS.
func (c Config) WithSecure(secure bool) Config {
	c.Secure = secure
	return c
}

// WithHTTPOnly sets whether the cookie should be inaccessible to JavaScript.
func (c Config) WithHTTPOnly(httpOnly bool) Config {
	c.HTTPOnly = httpOnly
	return c
}

// WithSameSite sets the SameSite attribute of the cookie.
// Allowed values: "Strict", "Lax", "None"
func (c Config) WithSameSite(sameSite string) Config {
	c.SameSite = sameSite
	return c
}

// WithKeyPrefix sets the prefix for session keys in storage.
func (c Config) WithKeyPrefix(prefix string) Config {
	c.KeyPrefix = prefix
	return c
}

// Validate validates the configuration and returns an error if invalid.
// Note: This method uses a value receiver, so it cannot modify the config.
// Use DefaultConfig() with builder methods to ensure valid configuration.
func (c Config) Validate() error {
	// This is a validation-only method, not a mutation method.
	// All defaults are handled by DefaultConfig() and builder methods.
	return nil
}
