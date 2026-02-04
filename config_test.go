package session

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Expiration != 24*time.Hour {
		t.Errorf("expected Expiration to be 24h, got %v", cfg.Expiration)
	}
	if cfg.CookieName != "session_id" {
		t.Errorf("expected CookieName to be 'session_id', got %s", cfg.CookieName)
	}
	if cfg.CookieDomain != "" {
		t.Errorf("expected CookieDomain to be empty, got %s", cfg.CookieDomain)
	}
	if cfg.CookiePath != "/" {
		t.Errorf("expected CookiePath to be '/', got %s", cfg.CookiePath)
	}
	if !cfg.Secure {
		t.Error("expected Secure to be true")
	}
	if !cfg.HTTPOnly {
		t.Error("expected HTTPOnly to be true")
	}
	if cfg.SameSite != "Lax" {
		t.Errorf("expected SameSite to be 'Lax', got %s", cfg.SameSite)
	}
	if cfg.KeyPrefix != "session:" {
		t.Errorf("expected KeyPrefix to be 'session:', got %s", cfg.KeyPrefix)
	}
}

func TestConfigWithMethods(t *testing.T) {
	cfg := DefaultConfig().
		WithExpiration(1 * time.Hour).
		WithCookieName("my_session").
		WithCookieDomain(".example.com").
		WithCookiePath("/app").
		WithSecure(false).
		WithHTTPOnly(false).
		WithSameSite("Strict").
		WithKeyPrefix("myapp:session:")

	if cfg.Expiration != 1*time.Hour {
		t.Errorf("expected Expiration to be 1h, got %v", cfg.Expiration)
	}
	if cfg.CookieName != "my_session" {
		t.Errorf("expected CookieName to be 'my_session', got %s", cfg.CookieName)
	}
	if cfg.CookieDomain != ".example.com" {
		t.Errorf("expected CookieDomain to be '.example.com', got %s", cfg.CookieDomain)
	}
	if cfg.CookiePath != "/app" {
		t.Errorf("expected CookiePath to be '/app', got %s", cfg.CookiePath)
	}
	if cfg.Secure {
		t.Error("expected Secure to be false")
	}
	if cfg.HTTPOnly {
		t.Error("expected HTTPOnly to be false")
	}
	if cfg.SameSite != "Strict" {
		t.Errorf("expected SameSite to be 'Strict', got %s", cfg.SameSite)
	}
	if cfg.KeyPrefix != "myapp:session:" {
		t.Errorf("expected KeyPrefix to be 'myapp:session:', got %s", cfg.KeyPrefix)
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Errorf("expected no error with zero config, got %v", err)
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("expected no error with default config, got %v", err)
	}

	invalidConfig := DefaultConfig().WithCookieName("")
	if err := invalidConfig.Validate(); err == nil {
		t.Error("expected error for empty cookie name, got nil")
	}
}
