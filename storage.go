package session

import (
	"time"
)

// Storage is the interface for session storage backends.
// This interface is compatible with fiber.Storage interface.
type Storage interface {
	// Get retrieves the value for the given key.
	// Returns nil, nil if the key does not exist.
	Get(key string) ([]byte, error)

	// Set stores the given value for the given key along with an expiration value.
	// If expiration is 0, the value never expires.
	// Empty key or value will be ignored without an error.
	Set(key string, val []byte, exp time.Duration) error

	// Delete removes the value for the given key.
	// It returns no error if the storage does not contain the key.
	Delete(key string) error

	// Reset removes all keys with the configured prefix.
	Reset() error

	// Close closes the storage connection.
	Close() error
}

// SessionData represents the data stored in a session.
type SessionData struct {
	// ID is the unique session identifier.
	ID string `json:"id"`

	// UserID is the authenticated user's ID.
	UserID string `json:"user_id,omitempty"`

	// Email is the authenticated user's email.
	Email string `json:"email,omitempty"`

	// Phone is the authenticated user's phone number.
	Phone string `json:"phone,omitempty"`

	// Authenticated indicates if the session is authenticated.
	Authenticated bool `json:"authenticated"`

	// Data is a map for storing arbitrary session data.
	Data map[string]interface{} `json:"data,omitempty"`

	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the session expires.
	ExpiresAt time.Time `json:"expires_at"`

	// LastAccessedAt is when the session was last accessed.
	LastAccessedAt time.Time `json:"last_accessed_at"`

	// AMR (Authentication Methods References) records how the user authenticated.
	AMR []string `json:"amr,omitempty"`

	// Scopes are the authorization scopes for this session.
	Scopes []string `json:"scopes,omitempty"`
}

// NewSessionData creates a new SessionData with the given ID and expiration.
func NewSessionData(id string, expiration time.Duration) *SessionData {
	now := time.Now()
	return &SessionData{
		ID:             id,
		Authenticated:  false,
		Data:           make(map[string]interface{}),
		CreatedAt:      now,
		ExpiresAt:      now.Add(expiration),
		LastAccessedAt: now,
	}
}

// IsExpired checks if the session has expired.
func (s *SessionData) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsAuthenticated returns true if the session is authenticated and not expired.
func (s *SessionData) IsAuthenticated() bool {
	return s.Authenticated && !s.IsExpired()
}

// Touch updates the last accessed time to now.
func (s *SessionData) Touch() {
	s.LastAccessedAt = time.Now()
}

// SetValue sets a value in the session data map.
func (s *SessionData) SetValue(key string, value interface{}) {
	if s.Data == nil {
		s.Data = make(map[string]interface{})
	}
	s.Data[key] = value
}

// GetValue gets a value from the session data map.
func (s *SessionData) GetValue(key string) (interface{}, bool) {
	if s.Data == nil {
		return nil, false
	}
	v, ok := s.Data[key]
	return v, ok
}

// DeleteValue removes a value from the session data map.
func (s *SessionData) DeleteValue(key string) {
	if s.Data != nil {
		delete(s.Data, key)
	}
}

// AddAMR adds an authentication method reference.
func (s *SessionData) AddAMR(method string) {
	for _, m := range s.AMR {
		if m == method {
			return
		}
	}
	s.AMR = append(s.AMR, method)
}

// HasAMR checks if the session has a specific authentication method.
func (s *SessionData) HasAMR(method string) bool {
	for _, m := range s.AMR {
		if m == method {
			return true
		}
	}
	return false
}

// AddScope adds an authorization scope.
func (s *SessionData) AddScope(scope string) {
	for _, sc := range s.Scopes {
		if sc == scope {
			return
		}
	}
	s.Scopes = append(s.Scopes, scope)
}

// HasScope checks if the session has a specific scope.
func (s *SessionData) HasScope(scope string) bool {
	for _, sc := range s.Scopes {
		if sc == scope {
			return true
		}
	}
	return false
}
