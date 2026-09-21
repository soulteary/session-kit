package session

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestStringSliceSurvivesStorageRoundTrip is the regression test for scopes and
// AMR vanishing after the first request. Fiber v3 serialises session data with
// msgpack, which decodes an array as []interface{}; a val.([]string) assertion
// therefore returned nil on every request after the one that wrote it.
func TestStringSliceSurvivesStorageRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want []string
	}{
		{"native slice", []string{"read", "write"}, []string{"read", "write"}},
		{"decoded from msgpack", []interface{}{"read", "write"}, []string{"read", "write"}},
		{"nil", nil, nil},
		{"wrong element type", []interface{}{"read", 42}, nil},
		{"wrong type entirely", "read", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stringSlice(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("stringSlice(%#v) = %#v, want %#v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("stringSlice(%#v)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestMemoryStorageGetReturnsCopy: Set copies on write, so Get must copy on
// read. Returning the internal slice let one caller corrupt every later
// reader's view of the session.
func TestMemoryStorageGetReturnsCopy(t *testing.T) {
	s := NewMemoryStorage("t:", 0)
	defer func() { _ = s.Close() }()

	original := []byte("authenticated")
	if err := s.Set("k", original, time.Minute); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	got[0] = 'X'

	again, err := s.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, original) {
		t.Errorf("stored value became %q after a caller mutated what Get returned", again)
	}
}

// TestMemoryStorageEmptyValueDeletes: writing empty data must not leave the
// previous, still-authenticated payload readable.
func TestMemoryStorageEmptyValueDeletes(t *testing.T) {
	s := NewMemoryStorage("t:", 0)
	defer func() { _ = s.Close() }()

	if err := s.Set("k", []byte("authenticated"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("k", nil, time.Minute); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("Get() = %q after an empty write; the old session data is still readable", got)
	}
}

// TestMemoryStorageCloseIsIdempotent: an unguarded close(done) panicked on the
// second call, which a deferred Close plus an explicit shutdown hits easily.
func TestMemoryStorageCloseIsIdempotent(t *testing.T) {
	s := NewMemoryStorage("t:", time.Hour)
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

// --- Session fixation (login must rotate the session id) ---

// rotatingSession is a session that can rotate its own ID -- the shape Fiber
// v3's *middleware/session.Session has. It records the order of the calls it
// receives, because the order is the fail-closed guarantee: rotation has to be
// attempted before anything marks the session authenticated.
type rotatingSession struct {
	values   map[any]any
	id       string
	rotated  int
	saved    int
	roterr   error
	sequence []string
}

func newRotatingSession() *rotatingSession {
	return &rotatingSession{values: map[any]any{}, id: "planted-id"}
}

func (s *rotatingSession) Get(key any) any { return s.values[key] }

func (s *rotatingSession) Set(key, val any) {
	s.sequence = append(s.sequence, "set")
	s.values[key] = val
}

func (s *rotatingSession) Save() error {
	s.sequence = append(s.sequence, "save")
	s.saved++
	return nil
}

func (s *rotatingSession) Regenerate() error {
	s.sequence = append(s.sequence, "regenerate")
	if s.roterr != nil {
		return s.roterr
	}
	s.rotated++
	s.id = fmt.Sprintf("rotated-%d", s.rotated)
	return nil
}

// TestAuthenticateRotatesSessionID is the regression test for session
// fixation. Authenticate only wrote the authentication markers and saved, so
// the session ID survived login: an ID an attacker had planted in the victim's
// browser came back out of login authenticated, and the attacker's copy of it
// then granted access to the victim's account.
func TestAuthenticateRotatesSessionID(t *testing.T) {
	sess := newRotatingSession()
	SetUserID(sess, "victim-123")
	sess.sequence = nil // from here on, only what Authenticate itself does

	if err := Authenticate(sess); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	if sess.id == "planted-id" {
		t.Error("the session id survived login; an id planted before login is now authenticated")
	}
	if sess.rotated != 1 {
		t.Errorf("Regenerate called %d times, want 1", sess.rotated)
	}
	if !IsAuthenticated(sess) {
		t.Error("session is not authenticated after Authenticate")
	}
	// Rotation keeps the data written before login.
	if got := GetUserID(sess); got != "victim-123" {
		t.Errorf("GetUserID() = %q after rotation, want %q", got, "victim-123")
	}

	// Rotation must come before anything marks the session authenticated.
	if len(sess.sequence) == 0 || sess.sequence[0] != "regenerate" {
		t.Errorf("call sequence = %v, want rotation first", sess.sequence)
	}
	if sess.sequence[len(sess.sequence)-1] != "save" {
		t.Errorf("call sequence = %v, want the save last", sess.sequence)
	}
}

// TestAuthenticateFailsClosedWhenRotationFails: a session that could not be
// rotated must be left unauthenticated, not marked authenticated under an ID
// an attacker may already hold. Nothing may be written or saved either --
// Fiber's middleware persists the session when the handler returns, so a
// marker written before a failed rotation would reach storage under the old
// ID anyway.
func TestAuthenticateFailsClosedWhenRotationFails(t *testing.T) {
	rotationFailed := errors.New("storage unavailable")
	sess := newRotatingSession()
	sess.roterr = rotationFailed

	err := Authenticate(sess)
	if err == nil {
		t.Fatal("Authenticate() = nil, want an error when the id could not be rotated")
	}
	if !errors.Is(err, rotationFailed) {
		t.Errorf("Authenticate() error = %v, want it to wrap %v", err, rotationFailed)
	}
	if IsAuthenticated(sess) {
		t.Error("session was marked authenticated even though rotation failed")
	}
	if sess.saved != 0 {
		t.Errorf("Save called %d times after a failed rotation, want 0", sess.saved)
	}
	if want := []string{"regenerate"}; len(sess.sequence) != len(want) || sess.sequence[0] != want[0] {
		t.Errorf("call sequence = %v, want %v", sess.sequence, want)
	}
}

// TestAuthenticateWithoutRotationSupport: rotation is taken up through
// [Regenerator] rather than required by [Saver], so a session that cannot
// rotate -- the framework-free memorySession in ExampleAuthenticate, for one
// -- still authenticates. Requiring it in Saver instead would break every such
// type that compiles against the released v3 API.
func TestAuthenticateWithoutRotationSupport(t *testing.T) {
	sess := &plainSession{values: map[any]any{}}

	if err := Authenticate(sess); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !IsAuthenticated(sess) {
		t.Error("a session that cannot rotate should still authenticate")
	}
	if sess.saved != 1 {
		t.Errorf("Save called %d times, want 1", sess.saved)
	}
}

// plainSession satisfies Saver and Reader but cannot rotate.
type plainSession struct {
	values map[any]any
	saved  int
}

func (s *plainSession) Get(key any) any  { return s.values[key] }
func (s *plainSession) Set(key, val any) { s.values[key] = val }
func (s *plainSession) Save() error      { s.saved++; return nil }
