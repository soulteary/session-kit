package session

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"
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

// Reader is the read side of a request-scoped session: the one method every
// getter and Has* predicate in this package needs.
//
// The helpers take these interfaces rather than a concrete session type so
// that this package does not import a web framework. Fiber v3's
// *middleware/session.Session satisfies all of them as it is, so Fiber code
// passes the session it already has and compiles unchanged.
type Reader interface {
	// Get returns the value stored under key, or nil when there is none.
	Get(key any) any
}

// Writer is the write side of a request-scoped session.
type Writer interface {
	// Set stores val under key.
	Set(key, val any)
}

// ReadWriter is a session that can be both read and written. The helpers that
// read a value, change it and write it back -- [AddAMR] is the one here --
// take this rather than [Session], so a session type without Destroy can
// still use them.
type ReadWriter interface {
	Reader
	Writer
}

// Saver is a session that can be written and persisted. [Authenticate] takes
// it: it writes two keys and saves, and has no use for Get, Delete or
// Destroy.
type Saver interface {
	Writer

	// Save persists the session.
	Save() error
}

// Regenerator is a session that can rotate its own ID: a new identifier, the
// same data, and the old record dropped from storage. Fiber v3's
// *middleware/session.Session implements it as it stands.
//
// [Authenticate] rotates through this interface whenever the session offers
// it, which is what keeps an ID planted before login from surviving it
// (session fixation). Rotation is a capability rather than a line in [Saver]
// because Saver is part of the released v3 API, and because a session that
// hands the client no ID of its own -- the memorySession in this package's
// ExampleAuthenticate, for one -- has nothing to rotate and nothing an
// attacker can fix in advance.
//
// A session type that DOES hand an ID to the client should implement this.
// Without it Authenticate has no way to rotate, and the ID the caller arrived
// with stays in place across login.
type Regenerator interface {
	// Regenerate gives the session a new ID and deletes the old one from
	// storage, keeping the session's data.
	Regenerate() error
}

// Session is the whole of what this package ever asks of a request-scoped
// session. Only [Unauthenticate] needs all of it, because clearing an
// identity means deleting keys and then destroying the record.
type Session interface {
	ReadWriter

	// Delete removes the value stored under key.
	Delete(key any)

	// Save persists the session.
	Save() error

	// Destroy deletes the stored session and expires its cookie.
	Destroy() error
}

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
		// Falling back to a full expiration period here resurrected an already
		// expired session: saving it granted it another complete lifetime, so
		// a session that kept being written never actually expired. Refuse
		// instead; use TouchSession to extend a live session deliberately.
		return fmt.Errorf("refusing to save expired session %q (expired at %s)", session.ID, session.ExpiresAt)
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

// Helper functions for request-scoped sessions

// Authenticate rotates the session ID when the session can rotate it, then
// marks the session as authenticated.
//
// The rotation closes a session fixation hole: this helper used to write the
// authentication markers and save under the *existing* ID, so an ID an
// attacker had planted in the victim's browser before login came back out of
// login authenticated, and the attacker's copy of that ID then granted access
// to the victim's account. Fiber adopts a client-supplied ID whenever storage
// holds a record for it, so planting one only takes visiting the site first.
// Both READMEs listed ID rotation at login as the expected hardening, but left
// it to every caller to remember -- and their own login examples did not do it.
//
// Rotation runs before the markers are written, and a rotation error is fatal:
// a session that could not be rotated is left unauthenticated rather than
// marked authenticated under an ID an attacker may already hold. The order
// matters on Fiber's middleware (session.FromContext) path in particular,
// where Save is a no-op and the middleware persists the session when the
// handler returns -- markers written before a failed rotation would be saved
// under the old ID anyway.
//
// The capability is taken up through [Regenerator] rather than required by
// [Saver]; see [Regenerator] for why, and for what to do if your own session
// type hands an ID to the client.
//
// Data already set on the session (user ID, email, AMR, scopes) carries over
// to the new ID; only the old storage record is dropped. Callers that already
// rotate the ID themselves stay correct, they simply rotate once more.
func Authenticate(session Saver) error {
	if rotator, ok := session.(Regenerator); ok {
		if err := rotator.Regenerate(); err != nil {
			return fmt.Errorf("failed to rotate session id on login: %w", err)
		}
	}

	session.Set(KeyAuthenticated, true)
	session.Set(KeyCreatedAt, time.Now().Unix())
	return session.Save()
}

// Unauthenticate destroys a session.
//
// The identity keys are cleared and then persisted BEFORE Destroy is
// attempted, so that a failing Destroy leaves a de-authenticated session
// rather than a fully authenticated one. Previously the clearing happened only
// in memory -- Save was never called -- so the "in case Destroy fails" comment
// described a safeguard that did not exist: if Destroy failed, the stored
// session was untouched and still authenticated.
//
// This function handles nil session gracefully.
func Unauthenticate(session Session) error {
	if isNil(session) {
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

	// Destroy first; persist the cleared state only as a fallback.
	//
	// Destroy deletes the stored record and expires the cookie, which makes a
	// preceding Save a write of data that is about to be deleted -- a wasted
	// round trip against Redis, and a window in which the cleared session is
	// persisted under the id being removed. Saving only when Destroy fails
	// keeps the safeguard the comment above describes: a failed destroy still
	// leaves a de-authenticated session rather than an authenticated one.
	destroyErr := session.Destroy()
	if destroyErr == nil {
		return nil
	}

	if saveErr := session.Save(); saveErr != nil {
		return fmt.Errorf("destroy failed (%w) and the cleared session could not be saved either: %v", destroyErr, saveErr)
	}
	return destroyErr
}

// isNil reports whether there is no session to act on.
//
// Session is an interface, so a plain session == nil misses the case that
// actually reaches Unauthenticate: a nil *middleware/session.Session stored
// in it, which a caller gets from an unassigned field or from a helper that
// returned early on an error. Calling Set on that panics, and a logout path
// that takes the process down is worse than the failed logout it was meant to
// report.
func isNil(session Session) bool {
	if session == nil {
		return true
	}
	v := reflect.ValueOf(session)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// IsAuthenticated checks if a session is authenticated.
func IsAuthenticated(session Reader) bool {
	val := session.Get(KeyAuthenticated)
	if val == nil {
		return false
	}
	authenticated, ok := val.(bool)
	return ok && authenticated
}

// SetUserID sets the user ID in a session.
func SetUserID(session Writer, userID string) {
	session.Set(KeyUserID, userID)
}

// GetUserID gets the user ID from a session.
func GetUserID(session Reader) string {
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

// SetEmail sets the email in a session.
func SetEmail(session Writer, email string) {
	session.Set(KeyEmail, email)
}

// GetEmail gets the email from a session.
func GetEmail(session Reader) string {
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

// SetPhone sets the phone in a session.
func SetPhone(session Writer, phone string) {
	session.Set(KeyPhone, phone)
}

// GetPhone gets the phone from a session.
func GetPhone(session Reader) string {
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

// SetAMR sets the authentication methods references in a session.
func SetAMR(session Writer, amr []string) {
	session.Set(KeyAMR, amr)
}

// GetAMR gets the authentication methods references from a session.
func GetAMR(session Reader) []string {
	return stringSlice(session.Get(KeyAMR))
}

// stringSlice coerces a value read back from session storage into []string.
//
// Fiber v3 serialises session data with msgpack, which decodes an array into
// []interface{} rather than []string. A plain val.([]string) assertion
// therefore succeeded only within the request that wrote the value and
// returned nil on every subsequent one -- so scopes and AMR silently vanished
// after the first round-trip, and HasScope/HasAMR always reported false.
func stringSlice(val interface{}) []string {
	switch v := val.(type) {
	case nil:
		return nil
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

// AddAMR adds an authentication method reference to a session.
func AddAMR(session ReadWriter, method string) {
	amr := GetAMR(session)
	for _, m := range amr {
		if m == method {
			return
		}
	}
	amr = append(amr, method)
	SetAMR(session, amr)
}

// HasAMR checks if a session has a specific authentication method.
func HasAMR(session Reader, method string) bool {
	amr := GetAMR(session)
	for _, m := range amr {
		if m == method {
			return true
		}
	}
	return false
}

// SetScopes sets the authorization scopes in a session.
func SetScopes(session Writer, scopes []string) {
	session.Set(KeyScopes, scopes)
}

// GetScopes gets the authorization scopes from a session.
func GetScopes(session Reader) []string {
	return stringSlice(session.Get(KeyScopes))
}

// HasScope checks if a session has a specific scope.
func HasScope(session Reader, scope string) bool {
	scopes := GetScopes(session)
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// UpdateLastAccess updates the last access timestamp in a session.
func UpdateLastAccess(session Writer) {
	session.Set(KeyLastAccess, time.Now().Unix())
}

// GetLastAccess gets the last access timestamp from a session.
func GetLastAccess(session Reader) time.Time {
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

// GetCreatedAt gets the session creation timestamp from a session.
func GetCreatedAt(session Reader) time.Time {
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

// CreateCookie builds the session cookie described by config, for sharing a
// session across domains or setting it by hand.
//
// It returns a *net/http.Cookie, which net/http writes with http.SetCookie
// and every other Go web framework accepts or converts. Fiber users call
// fiberadapter.Cookie for a *fiber.Cookie instead; both are built from the
// same [Config.SameSiteMode] and [Config.CookieSecure] rules.
func CreateCookie(config Config, sessionID string) *http.Cookie {
	return &http.Cookie{
		Name:     config.CookieName,
		Value:    sessionID,
		Expires:  time.Now().Add(config.Expiration),
		Path:     config.CookiePath,
		Domain:   config.CookieDomain,
		Secure:   config.CookieSecure(),
		HttpOnly: config.HTTPOnly,
		SameSite: config.SameSiteMode(),
	}
}
