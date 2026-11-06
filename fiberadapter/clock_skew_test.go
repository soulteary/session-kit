package fiberadapter

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	session "github.com/soulteary/session-kit/v3"
)

func tokenAt(deadline time.Time) string {
	return "r1." + strconv.FormatInt(deadline.UnixMilli(), 36) + "." + strings.Repeat("A", 26)
}

func TestDeadlineValidationBoundsClockSpreadWithoutExpiryGrace(t *testing.T) {
	now := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name                string
		lifetime, allowance time.Duration
	}{
		{"normal lifetime", time.Hour, time.Second},
		{"short lifetime", 100 * time.Millisecond, time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := revocableStorageAdapter{lifetime: tc.lifetime}
			for _, offset := range []time.Duration{tc.lifetime, tc.lifetime + tc.allowance} {
				if err := adapter.validateDeadline(tokenAt(now.Add(offset)), now); err != nil {
					t.Fatalf("within bound %s: %v", offset, err)
				}
			}
			if err := adapter.validateDeadline(tokenAt(now.Add(tc.lifetime+tc.allowance+time.Millisecond)), now); !errors.Is(err, session.ErrSessionEncoding) {
				t.Fatalf("far future accepted: %v", err)
			}
			for _, deadline := range []time.Time{now, now.Add(-time.Millisecond)} {
				if err := adapter.validateDeadline(tokenAt(deadline), now); !errors.Is(err, session.ErrSessionExpired) {
					t.Fatalf("expired identifier accepted: %v", err)
				}
			}
			if err := adapter.validateDeadline("invalid", now); !errors.Is(err, session.ErrSessionEncoding) {
				t.Fatalf("invalid encoding accepted: %v", err)
			}
		})
	}
}

func TestDeadlineValidationDoesNotOverflowLargeLifetime(t *testing.T) {
	now := time.Unix(1700000000, 0)
	adapter := revocableStorageAdapter{lifetime: time.Duration(1<<63 - 1)}
	deadline := now.Add(adapter.lifetime).Add(500 * time.Millisecond)
	if err := adapter.validateDeadline(tokenAt(deadline), now); err != nil {
		t.Fatalf("large lifetime overflowed: %v", err)
	}
}
