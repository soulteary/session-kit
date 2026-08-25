package session

import (
	"context"
	"time"
)

// fiberStorageAdapter adds Fiber v3's context-aware storage methods while
// preserving session-kit's existing Storage contract.
type fiberStorageAdapter struct {
	storage Storage
}

func (a fiberStorageAdapter) Get(key string) ([]byte, error) {
	return a.storage.Get(key)
}

func (a fiberStorageAdapter) GetWithContext(_ context.Context, key string) ([]byte, error) {
	return a.storage.Get(key)
}

func (a fiberStorageAdapter) Set(key string, value []byte, expiration time.Duration) error {
	return a.storage.Set(key, value, expiration)
}

func (a fiberStorageAdapter) SetWithContext(_ context.Context, key string, value []byte, expiration time.Duration) error {
	return a.storage.Set(key, value, expiration)
}

func (a fiberStorageAdapter) Delete(key string) error {
	return a.storage.Delete(key)
}

func (a fiberStorageAdapter) DeleteWithContext(_ context.Context, key string) error {
	return a.storage.Delete(key)
}

func (a fiberStorageAdapter) Reset() error {
	return a.storage.Reset()
}

func (a fiberStorageAdapter) ResetWithContext(_ context.Context) error {
	return a.storage.Reset()
}

func (a fiberStorageAdapter) Close() error {
	return a.storage.Close()
}
