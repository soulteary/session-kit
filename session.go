package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	fibersession "github.com/gofiber/fiber/v2/middleware/session"
)

// SessionKeys defines common session key names.
const (
	KeyAuthenticated = "authenticated"
	KeyUserID        = "user_id"
	KeyEmail         = "email"
	KeyPhone         = "phone"
	KeyAMR           = "amr"
	KeyScopes        = "scopes"
	KeyCreatedAt     = "created_at"
	KeyLastAccess    = "last_access"
)

// Manager provides high-level session management operations.
type Manager struct {
	storage Storage
	config  Config
}

// NewManager creates a new session Manager with the given storage and configuration.
func NewManager(storage Storage, config Config) *Manager {
	return &Manager{
		storage: storage,
		config:  config,
	}
}

// GetStorage returns the underlying storage.
func (m *Manager) GetStorage() Storage {
	return m.storage
}

// GetConfig returns the session configuration.
func (m *Manager) GetConfig() Config {
	return m.config
}

// CreateSession creates a new session and returns its data.
func (m *Manager) CreateSession(id string) *SessionData {
	return NewSessionData(id, m.config.Expiration)
}

// SaveSession saves a session to storage.
func (m *Manager) SaveSession(session *SessionData) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		ttl = m.config.Expiration
	}

	return m.storage.Set(session.ID, data, ttl)
}

// LoadSession loads a session from storage.
func (m *Manager) LoadSession(id string) (*SessionData, error) {
	data, err := m.storage.Get(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	if session.IsExpired() {
		_ = m.storage.Delete(id)
		return nil, nil
	}

	return &session, nil
}

// DeleteSession removes a session from storage.
func (m *Manager) DeleteSession(id string) error {
	return m.storage.Delete(id)
}

// TouchSession updates the last access time and extends expiration.
func (m *Manager) TouchSession(session *SessionData) error {
	session.Touch()
	session.ExpiresAt = time.Now().Add(m.config.Expiration)
	return m.SaveSession(session)
}

// FiberSessionConfig returns a fiber/v2/middleware/session.Config configured to use the Manager's storage.
func (m *Manager) FiberSessionConfig() fibersession.Config {
	sameSite := fiber.CookieSameSiteLaxMode
	normalizedSameSite := normalizeSameSite(m.config.SameSite)
	switch normalizedSameSite {
	case "Strict":
		sameSite = fiber.CookieSameSiteStrictMode
	case "None":
		sameSite = fiber.CookieSameSiteNoneMode
	case "Disabled":
		sameSite = fiber.CookieSameSiteDisabled
	}

	cookieSecure := m.config.Secure
	if normalizedSameSite == "None" && !cookieSecure {
		cookieSecure = true
	}

	return fibersession.Config{
		Expiration:     m.config.Expiration,
		Storage:        m.storage,
		KeyLookup:      fmt.Sprintf("cookie:%s", m.config.CookieName),
		CookieDomain:   m.config.CookieDomain,
		CookiePath:     m.config.CookiePath,
		CookieSecure:   cookieSecure,
		CookieHTTPOnly: m.config.HTTPOnly,
		CookieSameSite: sameSite,
	}
}

// Helper functions for Fiber sessions

// Authenticate marks a fiber session as authenticated.
func Authenticate(session *fibersession.Session) error {
	session.Set(KeyAuthenticated, true)
	session.Set(KeyCreatedAt, time.Now().Unix())
	return session.Save()
}

// Unauthenticate destroys a fiber session.
// Note: session.Destroy() requires a valid context (ctx).
// If session has been previously saved, the context may be released.
// This function handles nil session gracefully.
func Unauthenticate(session *fibersession.Session) error {
	if session == nil {
		return nil
	}
	// Clear authenticated flag first (in case Destroy fails)
	session.Set(KeyAuthenticated, false)
	session.Delete(KeyUserID)
	session.Delete(KeyEmail)
	session.Delete(KeyPhone)
	session.Delete(KeyAMR)
	session.Delete(KeyScopes)
	session.Delete(KeyCreatedAt)
	session.Delete(KeyLastAccess)
	return session.Destroy()
}

// IsAuthenticated checks if a fiber session is authenticated.
func IsAuthenticated(session *fibersession.Session) bool {
	val := session.Get(KeyAuthenticated)
	if val == nil {
		return false
	}
	authenticated, ok := val.(bool)
	return ok && authenticated
}

// SetUserID sets the user ID in a fiber session.
func SetUserID(session *fibersession.Session, userID string) {
	session.Set(KeyUserID, userID)
}

// GetUserID gets the user ID from a fiber session.
func GetUserID(session *fibersession.Session) string {
	val := session.Get(KeyUserID)
	if val == nil {
		return ""
	}
	userID, ok := val.(string)
	if !ok {
		return ""
	}
	return userID
}

// SetEmail sets the email in a fiber session.
func SetEmail(session *fibersession.Session, email string) {
	session.Set(KeyEmail, email)
}

// GetEmail gets the email from a fiber session.
func GetEmail(session *fibersession.Session) string {
	val := session.Get(KeyEmail)
	if val == nil {
		return ""
	}
	email, ok := val.(string)
	if !ok {
		return ""
	}
	return email
}

// SetPhone sets the phone in a fiber session.
func SetPhone(session *fibersession.Session, phone string) {
	session.Set(KeyPhone, phone)
}

// GetPhone gets the phone from a fiber session.
func GetPhone(session *fibersession.Session) string {
	val := session.Get(KeyPhone)
	if val == nil {
		return ""
	}
	phone, ok := val.(string)
	if !ok {
		return ""
	}
	return phone
}

// SetAMR sets the authentication methods references in a fiber session.
func SetAMR(session *fibersession.Session, amr []string) {
	session.Set(KeyAMR, amr)
}

// GetAMR gets the authentication methods references from a fiber session.
func GetAMR(session *fibersession.Session) []string {
	val := session.Get(KeyAMR)
	if val == nil {
		return nil
	}
	amr, ok := val.([]string)
	if !ok {
		return nil
	}
	return amr
}

// AddAMR adds an authentication method reference to a fiber session.
func AddAMR(session *fibersession.Session, method string) {
	amr := GetAMR(session)
	for _, m := range amr {
		if m == method {
			return
		}
	}
	amr = append(amr, method)
	SetAMR(session, amr)
}

// HasAMR checks if a fiber session has a specific authentication method.
func HasAMR(session *fibersession.Session, method string) bool {
	amr := GetAMR(session)
	for _, m := range amr {
		if m == method {
			return true
		}
	}
	return false
}

// SetScopes sets the authorization scopes in a fiber session.
func SetScopes(session *fibersession.Session, scopes []string) {
	session.Set(KeyScopes, scopes)
}

// GetScopes gets the authorization scopes from a fiber session.
func GetScopes(session *fibersession.Session) []string {
	val := session.Get(KeyScopes)
	if val == nil {
		return nil
	}
	scopes, ok := val.([]string)
	if !ok {
		return nil
	}
	return scopes
}

// HasScope checks if a fiber session has a specific scope.
func HasScope(session *fibersession.Session, scope string) bool {
	scopes := GetScopes(session)
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// UpdateLastAccess updates the last access timestamp in a fiber session.
func UpdateLastAccess(session *fibersession.Session) {
	session.Set(KeyLastAccess, time.Now().Unix())
}

// GetLastAccess gets the last access timestamp from a fiber session.
func GetLastAccess(session *fibersession.Session) time.Time {
	val := session.Get(KeyLastAccess)
	if val == nil {
		return time.Time{}
	}
	timestamp, ok := val.(int64)
	if !ok {
		return time.Time{}
	}
	return time.Unix(timestamp, 0)
}

// GetCreatedAt gets the session creation timestamp from a fiber session.
func GetCreatedAt(session *fibersession.Session) time.Time {
	val := session.Get(KeyCreatedAt)
	if val == nil {
		return time.Time{}
	}
	timestamp, ok := val.(int64)
	if !ok {
		return time.Time{}
	}
	return time.Unix(timestamp, 0)
}

// CreateCookie creates a fiber.Cookie for session sharing across domains.
func CreateCookie(config Config, sessionID string) *fiber.Cookie {
	sameSite := fiber.CookieSameSiteLaxMode
	normalizedSameSite := normalizeSameSite(config.SameSite)
	switch normalizedSameSite {
	case "Strict":
		sameSite = fiber.CookieSameSiteStrictMode
	case "None":
		sameSite = fiber.CookieSameSiteNoneMode
	case "Disabled":
		sameSite = fiber.CookieSameSiteDisabled
	}

	cookieSecure := config.Secure
	if normalizedSameSite == "None" && !cookieSecure {
		cookieSecure = true
	}

	cookie := &fiber.Cookie{
		Name:     config.CookieName,
		Value:    sessionID,
		Expires:  time.Now().Add(config.Expiration),
		Path:     config.CookiePath,
		Domain:   config.CookieDomain,
		Secure:   cookieSecure,
		HTTPOnly: config.HTTPOnly,
		SameSite: sameSite,
	}

	return cookie
}
