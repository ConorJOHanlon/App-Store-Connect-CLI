package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

// useSigninBackoffStore restores the real sign-in cooldown store, isolated in a
// fresh cache directory, and returns the injected clock.
func useSigninBackoffStore(t *testing.T) *time.Time {
	t.Helper()
	t.Setenv("ASC_WEB_SESSION_CACHE_DIR", t.TempDir())
	origRecord := recordSigninBackoffFn
	origNow := signinBackoffNowFn
	t.Cleanup(func() {
		recordSigninBackoffFn = origRecord
		signinBackoffNowFn = origNow
	})
	recordSigninBackoffFn = webcore.RecordSigninBackoff
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	signinBackoffNowFn = func() time.Time { return now }
	return &now
}

func TestResolveSessionBacksOffAutomaticSigninAfterApple5xx(t *testing.T) {
	start := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name         string
		retryAfter   string
		wantCooldown time.Duration
	}{
		{name: "default without Retry-After", retryAfter: "", wantCooldown: webcore.DefaultSigninBackoff},
		{name: "Retry-After seconds", retryAfter: "120", wantCooldown: 2 * time.Minute},
		{name: "Retry-After HTTP date", retryAfter: start.Add(5 * time.Minute).Format(http.TimeFormat), wantCooldown: 5 * time.Minute},
		{name: "Retry-After clamped", retryAfter: "86400", wantCooldown: webcore.MaxSigninBackoff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSigninUnavailableFixture(t, true)
			now := useSigninBackoffStore(t)
			unavailable := func(stage string) error {
				return &webcore.SigninServiceError{Stage: stage, Status: http.StatusServiceUnavailable, RetryAfter: tc.retryAfter}
			}
			webLoginWithClientFn = func(context.Context, *http.Client, webcore.LoginCredentials) (*webcore.AuthSession, error) {
				fixture.cachedAttempts++
				return nil, unavailable("signin_init")
			}
			webLoginFn = func(context.Context, webcore.LoginCredentials) (*webcore.AuthSession, error) {
				fixture.freshAttempts++
				return nil, unavailable("signin_complete")
			}
			wantNotBefore := start.Add(tc.wantCooldown).Format(time.RFC3339)

			_, _, err := resolveSession(context.Background(), "user@example.com", "", "")
			if !isSigninServerError(err) {
				t.Fatalf("first run error = %v, want Apple's sign-in 5xx", err)
			}
			if !strings.Contains(err.Error(), "will not sign in automatically before "+wantNotBefore) {
				t.Fatalf("first run error = %q, want the next allowed time %s", err, wantNotBefore)
			}
			if fixture.cachedAttempts != 1 || fixture.freshAttempts != 1 {
				t.Fatalf("first run attempts: cached = %d, fresh = %d; want the #2806 retry to still run once", fixture.cachedAttempts, fixture.freshAttempts)
			}

			*now = start.Add(tc.wantCooldown - time.Second)
			_, _, err = resolveSession(context.Background(), "user@example.com", "", "")
			if fixture.cachedAttempts != 1 || fixture.freshAttempts != 1 {
				t.Fatalf("attempts during cooldown: cached = %d, fresh = %d; want no contact with Apple", fixture.cachedAttempts, fixture.freshAttempts)
			}
			if !errors.Is(err, errSigninBackoffActive) || !shared.IsValidationError(err) {
				t.Fatalf("cooldown error = %v, want a validation-class backoff refusal", err)
			}
			if diagnostic, ok := shared.DiagnosticFromError(err); !ok || diagnostic.Code != shared.DiagnosticStateNotReady {
				t.Fatalf("cooldown diagnostic = %+v, want %s", diagnostic, shared.DiagnosticStateNotReady)
			}
			for _, want := range []string{"HTTP 503", "until " + wantNotBefore, "asc web auth login --apple-id \"user@example.com\""} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("cooldown error = %q, want %q", err, want)
				}
			}

			*now = start.Add(tc.wantCooldown)
			if _, _, err := resolveSession(context.Background(), "user@example.com", "", ""); !isSigninServerError(err) {
				t.Fatalf("post-cooldown error = %v, want a fresh sign-in attempt", err)
			}
			if fixture.cachedAttempts != 2 {
				t.Fatalf("cached attempts after cooldown = %d, want automatic sign-in to resume", fixture.cachedAttempts)
			}
		})
	}
}

func TestResolveSessionHonorsRetryAfterAllowingImmediateRetry(t *testing.T) {
	start := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, retryAfter := range []string{"0", start.Add(-time.Minute).Format(http.TimeFormat)} {
		t.Run(retryAfter, func(t *testing.T) {
			fixture := newSigninUnavailableFixture(t, true)
			useSigninBackoffStore(t)
			unavailable := func(stage string) error {
				return &webcore.SigninServiceError{Stage: stage, Status: http.StatusServiceUnavailable, RetryAfter: retryAfter}
			}
			webLoginWithClientFn = func(context.Context, *http.Client, webcore.LoginCredentials) (*webcore.AuthSession, error) {
				fixture.cachedAttempts++
				return nil, unavailable("signin_init")
			}
			webLoginFn = func(context.Context, webcore.LoginCredentials) (*webcore.AuthSession, error) {
				fixture.freshAttempts++
				return nil, unavailable("signin_complete")
			}

			_, _, err := resolveSession(context.Background(), "user@example.com", "", "")
			if !isSigninServerError(err) || strings.Contains(err.Error(), "will not sign in automatically") {
				t.Fatalf("first run error = %v, want Apple's 5xx without a cooldown", err)
			}
			if _, _, err := resolveSession(context.Background(), "user@example.com", "", ""); !isSigninServerError(err) {
				t.Fatalf("second run error = %v, want an immediate sign-in attempt", err)
			}
			if fixture.cachedAttempts != 2 {
				t.Fatalf("cached attempts = %d, want Apple's immediate retry allowance honored", fixture.cachedAttempts)
			}
		})
	}
}

func TestWebAuthLoginIgnoresSigninBackoffAndClearsItOnAcceptedSignin(t *testing.T) {
	fixture := newSigninUnavailableFixture(t, true)
	useSigninBackoffStore(t)
	webLoginFn = func(context.Context, webcore.LoginCredentials) (*webcore.AuthSession, error) {
		fixture.freshAttempts++
		return nil, signinUnavailable("signin_init")
	}
	if _, _, err := resolveSession(context.Background(), "user@example.com", "", ""); !isSigninServerError(err) {
		t.Fatalf("setup error = %v, want Apple's sign-in 5xx", err)
	}

	// Apple accepts the explicit login's clean-jar retry, but caching the new
	// session fails locally: the accepted sign-in still ends the cooldown.
	webLoginFn = func(context.Context, webcore.LoginCredentials) (*webcore.AuthSession, error) {
		fixture.freshAttempts++
		return &webcore.AuthSession{UserEmail: "user@example.com"}, nil
	}
	persistWebSessionFn = func(*webcore.AuthSession) error { return errors.New("disk full") }
	_, _, err := resolveSession(contextForWebSignIn(context.Background()), "user@example.com", "", "")
	if err == nil || !strings.Contains(err.Error(), "failed to cache session") {
		t.Fatalf("explicit login error = %v, want the local cache failure", err)
	}
	if fixture.cachedAttempts != 2 || fixture.freshAttempts != 2 {
		t.Fatalf("attempts: cached = %d, fresh = %d; want the explicit login to contact Apple despite the cooldown", fixture.cachedAttempts, fixture.freshAttempts)
	}
	if _, ok, err := webcore.LoadSigninBackoff("user@example.com"); err != nil || ok {
		t.Fatalf("backoff record after Apple accepted the login: ok = %v, err = %v; want it cleared", ok, err)
	}
}

func TestResolveSessionNoAutoLoginUsesOnlyCachedSession(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cached   bool
		explicit bool
	}{
		{name: "expired cached session", cached: true},
		{name: "nothing cached", cached: false},
		{name: "explicit login still signs in", cached: true, explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSigninUnavailableFixture(t, tc.cached)
			t.Setenv(webNoAutoLoginEnv, "1")
			signedIn := &webcore.AuthSession{UserEmail: "user@example.com"}
			webLoginWithClientFn = func(context.Context, *http.Client, webcore.LoginCredentials) (*webcore.AuthSession, error) {
				fixture.cachedAttempts++
				return signedIn, nil
			}
			webLoginFn = func(context.Context, webcore.LoginCredentials) (*webcore.AuthSession, error) {
				fixture.freshAttempts++
				return signedIn, nil
			}
			ctx := context.Background()
			if tc.explicit {
				ctx = contextForWebSignIn(ctx)
			}

			session, _, err := resolveSession(ctx, "user@example.com", "", "")
			if tc.explicit {
				if err != nil || session != signedIn || fixture.cachedAttempts != 1 {
					t.Fatalf("explicit login = (%v, %v) after %d attempts, want one sign-in", session, err, fixture.cachedAttempts)
				}
				return
			}
			if fixture.cachedAttempts != 0 || fixture.freshAttempts != 0 {
				t.Fatalf("attempts: cached = %d, fresh = %d; want no contact with Apple", fixture.cachedAttempts, fixture.freshAttempts)
			}
			var missing *shared.MissingWebSessionError
			if !errors.As(err, &missing) {
				t.Fatalf("error = %v, want the missing-session error", err)
			}
			for _, want := range []string{webNoAutoLoginEnv, "asc web auth login --apple-id user@example.com"} {
				if !strings.Contains(missing.Hint, want) {
					t.Fatalf("hint = %q, want %q", missing.Hint, want)
				}
			}
		})
	}
}
