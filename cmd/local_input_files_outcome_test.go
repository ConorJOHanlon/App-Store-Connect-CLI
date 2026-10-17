package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/telemetry"
)

// localInputOutcomeCase describes a purely local input-file failure that must
// keep its message and exit code while being classified as an expected
// negative instead of an internal error.
type localInputOutcomeCase struct {
	name             string
	args             []string
	env              map[string]string
	isolatedWorkDir  bool
	wantStderrPrefix string
	wantReceipt      bool
	wantDiagnosis    shared.DiagnosticCode
	wantParameter    string
}

func runLocalInputOutcomeCase(t *testing.T, test localInputOutcomeCase) {
	t.Helper()
	resetReportFlags(t)
	tempDir := t.TempDir()
	t.Setenv("HOME", filepath.Join(tempDir, "home"))
	t.Setenv("ASC_BYPASS_KEYCHAIN", "1")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(tempDir, "config.json"))
	for key, value := range test.env {
		t.Setenv(key, value)
	}
	if test.isolatedWorkDir {
		t.Chdir(tempDir)
	}

	originalEmitTelemetry := emitTelemetry
	t.Cleanup(func() { emitTelemetry = originalEmitTelemetry })
	var gotContext telemetry.EventContext
	emitTelemetry = func(_ string, _ string, _ time.Duration, _ int, eventContext telemetry.EventContext) {
		gotContext = eventContext
	}

	var code int
	stdout, stderr := captureCommandOutput(t, func() {
		code = Run(test.args, "5.15.0")
	})

	if code != ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, ExitError, stderr)
	}
	if !strings.HasPrefix(stderr, test.wantStderrPrefix) {
		t.Fatalf("stderr = %q, want prefix %q", stderr, test.wantStderrPrefix)
	}
	if strings.Count(stderr, "Error:") != 1 {
		t.Fatalf("stderr = %q, want exactly one error line", stderr)
	}
	if test.wantReceipt {
		var receipt struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(stdout), &receipt); err != nil {
			t.Fatalf("stdout = %q, want JSON receipt: %v", stdout, err)
		}
		if receipt.Status != "unreadable" {
			t.Fatalf("receipt status = %q, want unreadable", receipt.Status)
		}
	} else if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if gotContext.OutcomeKind != telemetry.OutcomeExpectedNegative ||
		gotContext.DiagnosticCode != string(test.wantDiagnosis) ||
		gotContext.FailureParameter != test.wantParameter {
		t.Fatalf("telemetry context = %+v, want outcome %q, diagnostic %q, parameter %q",
			gotContext, telemetry.OutcomeExpectedNegative, test.wantDiagnosis, test.wantParameter)
	}
}

func writeLocalInputFixture(t *testing.T, path string, data string, mode os.FileMode) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod(%q) error: %v", path, err)
	}
	return path
}

func TestRunLocalInputFileFailuresAreClassified(t *testing.T) {
	dir := t.TempDir()
	missingProfile := filepath.Join(dir, "missing.mobileprovision")
	junkProfile := writeLocalInputFixture(t, filepath.Join(dir, "junk.mobileprovision"), "junk\n", 0o600)
	emptyProfile := writeLocalInputFixture(t, filepath.Join(dir, "empty.mobileprovision"), "", 0o600)

	openErr := func(path string) string {
		t.Helper()
		_, err := os.Open(path)
		if err == nil {
			t.Fatalf("os.Open(%q) unexpectedly succeeded", path)
		}
		return err.Error()
	}

	tests := []localInputOutcomeCase{
		{
			name:             "profiles local install missing path",
			args:             []string{"profiles", "local", "install", "--path", missingProfile},
			wantStderrPrefix: "Error: profiles local install: open input: " + openErr(missingProfile) + "\n",
			wantDiagnosis:    shared.DiagnosticFileNotFound,
			wantParameter:    "--path",
		},
		{
			name:             "profiles local install undecodable path",
			args:             []string{"profiles", "local", "install", "--path", junkProfile},
			wantStderrPrefix: "Error: profiles local install: decode embedded plist: ",
			wantDiagnosis:    shared.DiagnosticFileInvalidFormat,
			wantParameter:    "--path",
		},
		{
			name:             "profiles local install empty path",
			args:             []string{"profiles", "local", "install", "--path", emptyProfile},
			wantStderrPrefix: "Error: profiles local install: profile file is empty\n",
			wantDiagnosis:    shared.DiagnosticFileInvalidFormat,
			wantParameter:    "--path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runLocalInputOutcomeCase(t, test)
		})
	}
}
