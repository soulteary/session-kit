package session

import (
	"sync"
	"time"
)

// memoryEntry represents an entry in the memory storage.
type memoryEntry struct {
	data      []byte
	expiresAt time.Time
}

// isExpired checks if the entry has expired.
func (e *memoryEntry) isExpired() bool {
	if e.expiresAt.IsZero() {
		return false // Never expires
	}
	return time.Now().After(e.expiresAt)
}

// MemoryStorage implements Storage interface using in-memory map.
// This is useful for development and testing, but not suitable for production
// with multiple server instances as sessions won't be shared.
type MemoryStorage struct {
	mu        sync.RWMutex
	data      map[string]*memoryEntry
	keyPrefix string
	gcTicker  *time.Ticker
	done      chan struct{}
	closeOnce sync.Once
}

// NewMemoryStorage creates a new in-memory storage.
// The gcInterval parameter specifies how often to run garbage collection
// to clean up expired entries. If gcInterval is 0, garbage collection is disabled.
func NewMemoryStorage(keyPrefix string, gcInterval time.Duration) *MemoryStorage {
	if keyPrefix == "" {
		keyPrefix = "session:"
	} else if len(keyPrefix) > 0 && keyPrefix[len(keyPrefix)-1] != ':' {
		keyPrefix += ":"
	}

	s := &MemoryStorage{
		data:      make(map[string]*memoryEntry),
		keyPrefix: keyPrefix,
		done:      make(chan struct{}),
	}

	// Start garbage collection if interval is set
	if gcInterval > 0 {
		s.gcTicker = time.NewTicker(gcInterval)
		go s.runGC()
	}

	return s
}

// runGC runs periodic garbage collection.
func (s *MemoryStorage) runGC() {
	for {
		select {
		case <-s.gcTicker.C:
			s.gc()
		case <-s.done:
			return
		}
	}
}

// gc removes expired entries.
func (s *MemoryStorage) gc() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, entry := range s.data {
		if entry.isExpired() {
			delete(s.data, key)
		}
	}
}

// buildKey constructs the full key with prefix.
func (s *MemoryStorage) buildKey(key string) string {
	return s.keyPrefix + key
}

// Get retrieves the value for the given key.
// Returns nil, nil if the key does not exist or has expired.
func (s *MemoryStorage) Get(key string) ([]byte, error) {
	fullKey := s.buildKey(key)

	s.mu.RLock()
	entry, ok := s.data[fullKey]
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	if entry.isExpired() {
		// Clean up expired entry
		s.mu.Lock()
		delete(s.data, fullKey)
		s.mu.Unlock()
		return nil, nil
	}

	// Return a copy. Set already copies on write; returning the internal slice
	// here made the pair asymmetric, so a caller mutating what it read
	// corrupted the stored session for every later reader -- and raced with
	// concurrent writers.
	out := make([]byte, len(entry.data))
	copy(out, entry.data)
	return out, nil
}

// Set stores the given value for the given key along with an expiration value.
// If expiration is 0, the value never expires.
//
// An empty key is ignored without an error, matching fiber.Storage. An empty
// value DELETES the key rather than being ignored: treating it as a no-op left
// the previous value in place, so overwriting a session with empty data kept
// the old, still-authenticated payload readable.
func (s *MemoryStorage) Set(key string, val []byte, exp time.Duration) error {
	if key == "" {
		return nil
	}
	if len(val) == 0 {
		return s.Delete(key)
	}

	fullKey := s.buildKey(key)

	entry := &memoryEntry{
		data: make([]byte, len(val)),
	}
	copy(entry.data, val)

	if exp > 0 {
		entry.expiresAt = time.Now().Add(exp)
	}

	s.mu.Lock()
	s.data[fullKey] = entry
	s.mu.Unlock()

	return nil
}

// Delete removes the value for the given key.
// It returns no error if the storage does not contain the key.
func (s *MemoryStorage) Delete(key string) error {
	fullKey := s.buildKey(key)

	s.mu.Lock()
	delete(s.data, fullKey)
	s.mu.Unlock()

	return nil
}

// Reset removes all keys with the configured prefix.
func (s *MemoryStorage) Reset() error {
	s.mu.Lock()
	s.data = make(map[string]*memoryEntry)
	s.mu.Unlock()

	return nil
}

// Close stops the garbage collector and releases resources.
//
// Safe to call more than once: an unguarded close(s.done) panicked on the
// second call, which is easy to hit with a deferred Close plus an explicit
// shutdown path.
func (s *MemoryStorage) Close() error {
	s.closeOnce.Do(func() {
		if s.gcTicker != nil {
			s.gcTicker.Stop()
		}
		close(s.done)
	})
	return nil
}

// Len returns the number of entries in the storage (including expired ones).
func (s *MemoryStorage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}
