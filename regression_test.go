package session

import (
	"bytes"
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
