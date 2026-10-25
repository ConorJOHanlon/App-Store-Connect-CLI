package cmdtest

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudrankriyam/App-Store-Connect-CLI/cmd"
)

func TestTestFlightGroupReadsSendRequest(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantPath  string
		wantQuery url.Values
	}{
		{
			name:     "group builds with fields",
			args:     []string{"testflight", "groups", "builds", "list", "--group-id", "group-1", "--fields", "version,uploadedDate,processingState", "--limit", "5"},
			wantPath: "/v1/betaGroups/group-1/builds",
			wantQuery: url.Values{
				"fields[builds]": {"version,uploadedDate,processingState"},
				"limit":          {"5"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupAuth(t)
			t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

			var captured *http.Request
			installDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				captured = req
				return okJSONResponse(`{"data":[],"links":{}}`), nil
			}))

			_, stderr := captureOutput(t, func() {
				if code := cmd.Run(append(test.args, "--output", "json"), "1.2.3"); code != cmd.ExitSuccess {
					t.Fatalf("expected exit code %d, got %d", cmd.ExitSuccess, code)
				}
			})
			if captured == nil {
				t.Fatalf("expected an outbound request, stderr %q", stderr)
			}
			if captured.Method != http.MethodGet || captured.URL.Path != test.wantPath {
				t.Fatalf("request = %s %s, want GET %s", captured.Method, captured.URL.Path, test.wantPath)
			}
			if got := captured.URL.Query().Encode(); got != test.wantQuery.Encode() {
				t.Fatalf("query = %q, want %q", got, test.wantQuery.Encode())
			}
		})
	}
}

func TestTestFlightGroupReadsRejectUsage(t *testing.T) {
	const nextURL = "https://api.appstoreconnect.apple.com/v1/betaGroups/group-1/builds?cursor=AQ"

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "group builds without group id",
			args:    []string{"testflight", "groups", "builds", "list"},
			wantErr: "--group-id is required",
		},
		{
			name:    "group builds fields with next",
			args:    []string{"testflight", "groups", "builds", "list", "--next", nextURL, "--fields", "version"},
			wantErr: "testflight groups builds list: --next cannot be combined with --fields",
		},
		{
			name:    "group builds unknown field",
			args:    []string{"testflight", "groups", "builds", "list", "--group-id", "group-1", "--fields", "name"},
			wantErr: "--fields must be one of: version, uploadedDate",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupAuth(t)
			t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
			installDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
				return nil, nil
			}))

			stdout, stderr := captureOutput(t, func() {
				if code := cmd.Run(test.args, "1.2.3"); code != cmd.ExitUsage {
					t.Fatalf("expected exit code %d, got %d", cmd.ExitUsage, code)
				}
			})
			if stdout != "" {
				t.Fatalf("expected empty stdout, got %q", stdout)
			}
			if !strings.Contains(stderr, test.wantErr) {
				t.Fatalf("expected stderr to contain %q, got %q", test.wantErr, stderr)
			}
		})
	}
}
