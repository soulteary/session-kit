package fiberadapter

import (
	"context"
	"fmt"
	"time"

	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	session "github.com/soulteary/session-kit/v3"
)

// RevocableSessionConfig enables atomic logout with an absolute session lifetime.
// The backend must support RevocationStorage and use a namespace inaccessible to
// ordinary writers. Existing sessions in an ordinary namespace must log in again.
func RevocableSessionConfig(manager *session.Manager) (fibersession.Config, error) {
	config := manager.GetConfig()
	if config.Expiration <= 0 {
		return fibersession.Config{}, fmt.Errorf("revocable sessions require a positive absolute lifetime")
	}
	backend, ok := manager.GetStorage().(session.RevocationStorage)
	if !ok {
		return fibersession.Config{}, fmt.Errorf("storage does not support atomic session revocation")
	}
	result := SessionConfig(manager)
	result.Storage = revocableStorageAdapter{storage: manager.GetStorage(), revocation: backend, lifetime: config.Expiration}
	result.KeyGenerator = func() string { return session.NewRevocableSessionID(config.Expiration) }
	result.AbsoluteTimeout = config.Expiration
	return result, nil
}

type revocableStorageAdapter struct {
	storage    session.Storage
	revocation session.RevocationStorage
	lifetime   time.Duration
}

// validateDeadline allows bounded issuer clock spread only at the future limit.
// It never grants a grace period after the identifier's immutable expiry.
func (a revocableStorageAdapter) validateDeadline(key string, now time.Time) error {
	deadline, err := session.RevocableSessionDeadline(key)
	if err != nil {
		return err
	}
	if !now.Before(deadline) {
		return session.ErrSessionExpired
	}
	allowance := a.lifetime / 100
	if allowance > time.Second {
		allowance = time.Second
	}
	if deadline.After(now.Add(a.lifetime).Add(allowance)) {
		return session.ErrSessionEncoding
	}
	return nil
}

func (a revocableStorageAdapter) Get(key string) ([]byte, error) {
	if err := a.validateDeadline(key, time.Now()); err != nil {
		return nil, nil
	}
	return a.revocation.GetSession(key)
}
func (a revocableStorageAdapter) GetWithContext(_ context.Context, key string) ([]byte, error) {
	return a.Get(key)
}
func (a revocableStorageAdapter) Set(key string, value []byte, expiration time.Duration) error {
	if err := a.validateDeadline(key, time.Now()); err != nil {
		return err
	}
	return a.revocation.SetSessionIfActive(key, value, expiration)
}
func (a revocableStorageAdapter) SetWithContext(_ context.Context, key string, value []byte, expiration time.Duration) error {
	return a.Set(key, value, expiration)
}
func (a revocableStorageAdapter) Delete(key string) error { return a.revocation.RevokeSession(key) }
func (a revocableStorageAdapter) DeleteWithContext(_ context.Context, key string) error {
	return a.Delete(key)
}
func (a revocableStorageAdapter) Reset() error {
	return fmt.Errorf("bulk reset is unavailable for revocable sessions; rotate the namespace instead")
}
func (a revocableStorageAdapter) ResetWithContext(context.Context) error { return a.Reset() }
func (a revocableStorageAdapter) Close() error                           { return a.storage.Close() }
