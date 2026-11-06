package session

import (
	"crypto/rand"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSessionRevoked  = errors.New("session has been revoked")
	ErrSessionExpired  = errors.New("session identifier has expired")
	ErrSessionEncoding = errors.New("invalid revocable session encoding")
)

// RevocationStorage atomically prevents writes after a session identifier has
// been revoked. Implementations retain the revocation until the immutable
// identifier deadline; writes after that deadline are rejected independently.
// Use a separate key prefix from ordinary Storage writers.
type RevocationStorage interface {
	GetSession(key string) ([]byte, error)
	SetSessionIfActive(key string, value []byte, expiration time.Duration) error
	RevokeSession(key string) error
}

const ActiveSessionPrefix = "\x00session-kit:active:v1:"
const RevokedSessionValue = "\x00session-kit:revoked:v1"

// NewRevocableSessionID creates a random identifier with an absolute deadline.
// Changing the deadline changes the entire storage key, not an existing identity.
func NewRevocableSessionID(lifetime time.Duration) string {
	return "r1." + strconv.FormatInt(time.Now().Add(lifetime).UnixMilli(), 36) + "." + rand.Text()
}

// RevocableSessionDeadline parses the bounded identifier format. Callers must
// also check expiration; the backend checks it again inside the atomic write.
func RevocableSessionDeadline(key string) (time.Time, error) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "r1" || len(parts[2]) < 26 || len(parts[2]) > 128 {
		return time.Time{}, ErrSessionEncoding
	}
	for _, c := range parts[2] {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '7') {
			return time.Time{}, ErrSessionEncoding
		}
	}
	value, err := strconv.ParseInt(parts[1], 36, 64)
	if err != nil || value <= 0 {
		return time.Time{}, ErrSessionEncoding
	}
	return time.UnixMilli(value), nil
}

// DecodeActiveSession never interprets tombstones or ordinary storage payloads
// as session data. Legacy sessions must reauthenticate in the new namespace.
func DecodeActiveSession(value []byte) ([]byte, error) {
	if len(value) == 0 || string(value) == RevokedSessionValue {
		return nil, nil
	}
	if !strings.HasPrefix(string(value), ActiveSessionPrefix) {
		return nil, ErrSessionEncoding
	}
	return append([]byte(nil), value[len(ActiveSessionPrefix):]...), nil
}
