package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultSigninBackoff is how long automatic sign-in pauses after Apple's
	// sign-in service answered with a 5xx and sent no usable Retry-After.
	DefaultSigninBackoff = 15 * time.Minute
	// MaxSigninBackoff caps any cooldown, including one Apple asked for, so a
	// huge Retry-After or a tampered record cannot block sign-in indefinitely.
	MaxSigninBackoff = time.Hour

	signinBackoffVersion = 1
)

// SigninBackoff records that Apple's IdMSA sign-in service answered an
// automatic or explicit sign-in with a 5xx, so later automatic sign-ins for the
// same Apple Account can wait instead of adding to Apple's throttling count.
type SigninBackoff struct {
	Version    int       `json:"version"`
	FailedAt   time.Time `json:"failed_at"`
	NotBefore  time.Time `json:"not_before"`
	Stage      string    `json:"stage,omitempty"`
	Status     int       `json:"status,omitempty"`
	RetryAfter string    `json:"retry_after,omitempty"`
}

// NewSigninBackoff builds the cooldown for a sign-in 5xx observed at now. A
// Retry-After from Apple, in seconds or as an HTTP date, sets the cooldown;
// without one DefaultSigninBackoff applies. Either is capped at
// MaxSigninBackoff.
func NewSigninBackoff(serviceErr *SigninServiceError, now time.Time) SigninBackoff {
	backoff := SigninBackoff{Version: signinBackoffVersion, FailedAt: now.UTC()}
	cooldown := DefaultSigninBackoff
	if serviceErr != nil {
		backoff.Stage = serviceErr.Stage
		backoff.Status = serviceErr.Status
		backoff.RetryAfter = serviceErr.RetryAfter
		if requested, ok := parseSigninRetryAfter(serviceErr.RetryAfter, now); ok {
			cooldown = requested
		}
	}
	backoff.NotBefore = backoff.FailedAt.Add(min(cooldown, MaxSigninBackoff))
	return backoff
}

// ActiveUntil reports when automatic sign-in may resume and whether that is
// still in the future at now. The deadline never lies more than
// MaxSigninBackoff past now, so a clock change or an edited record cannot
// extend the pause beyond the cap.
func (b SigninBackoff) ActiveUntil(now time.Time) (time.Time, bool) {
	notBefore := b.NotBefore
	if limit := now.Add(MaxSigninBackoff); notBefore.After(limit) {
		notBefore = limit
	}
	return notBefore, now.Before(notBefore)
}

func parseSigninRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0, false
		}
		if seconds > int64(MaxSigninBackoff/time.Second) {
			return MaxSigninBackoff, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := at.Sub(now); delay > 0 {
			return delay, true
		}
	}
	return 0, false
}

func signinBackoffFilePath(appleID string) (string, error) {
	dir, err := webSessionCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "signin-backoff-"+webSessionCacheKey(appleID)+".json"), nil
}

// LoadSigninBackoff returns the recorded sign-in cooldown for appleID, if any.
func LoadSigninBackoff(appleID string) (SigninBackoff, bool, error) {
	path, err := signinBackoffFilePath(appleID)
	if err != nil {
		return SigninBackoff{}, false, err
	}
	raw, err := readSessionCacheFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SigninBackoff{}, false, nil
		}
		return SigninBackoff{}, false, err
	}
	var backoff SigninBackoff
	if err := json.Unmarshal(raw, &backoff); err != nil {
		return SigninBackoff{}, false, fmt.Errorf("sign-in backoff record %q is malformed: %w", path, err)
	}
	if backoff.Version != signinBackoffVersion {
		return SigninBackoff{}, false, nil
	}
	return backoff, true, nil
}

// RecordSigninBackoff stores the sign-in cooldown for appleID next to the web
// session cache, readable only by the current user.
func RecordSigninBackoff(appleID string, backoff SigninBackoff) error {
	path, err := signinBackoffFilePath(appleID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("failed to create session cache dir: %w", err)
	}
	backoff.Version = signinBackoffVersion
	raw, err := json.Marshal(backoff)
	if err != nil {
		return fmt.Errorf("failed to marshal sign-in backoff: %w", err)
	}
	temp, err := createAndWriteSessionTempFile(path, raw)
	if err != nil {
		return fmt.Errorf("failed to write sign-in backoff: %w", err)
	}
	defer temp.cleanup()
	if err := publishSessionTempFile(temp, path); err != nil {
		return fmt.Errorf("failed to finalize sign-in backoff: %w", err)
	}
	return nil
}

// ClearSigninBackoff removes any recorded sign-in cooldown for appleID.
func ClearSigninBackoff(appleID string) error {
	path, err := signinBackoffFilePath(appleID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
