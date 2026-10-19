package web

import (
	"testing"
	"time"
)

// An implausible record must never pause automatic sign-in for longer than
// MaxSigninBackoff after the failure it records.
func TestSigninBackoffActiveUntilCannotExceedCap(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		backoff    SigninBackoff
		wantActive bool
		wantUntil  time.Time
	}{
		{
			name:       "far-future deadline is capped from the failure",
			backoff:    SigninBackoff{FailedAt: now.Add(-30 * time.Minute), NotBefore: now.Add(1000 * time.Hour)},
			wantActive: true,
			wantUntil:  now.Add(30 * time.Minute),
		},
		{
			name:    "capped deadline already passed",
			backoff: SigninBackoff{FailedAt: now.Add(-2 * time.Hour), NotBefore: now.Add(1000 * time.Hour)},
		},
		{
			name:    "failure recorded in the future",
			backoff: SigninBackoff{FailedAt: now.Add(48 * time.Hour), NotBefore: now.Add(49 * time.Hour)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			until, active := tc.backoff.ActiveUntil(now)
			if active != tc.wantActive || (tc.wantActive && !until.Equal(tc.wantUntil)) {
				t.Fatalf("ActiveUntil = (%s, %v), want (%s, %v)", until, active, tc.wantUntil, tc.wantActive)
			}
		})
	}
}
