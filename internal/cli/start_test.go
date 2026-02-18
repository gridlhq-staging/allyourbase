package cli

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- portError ---

func TestPortErrorAddressInUse(t *testing.T) {
	err := portError(8090, fmt.Errorf("listen tcp :8090: bind: address already in use"))
	testutil.True(t, err != nil, "expected non-nil error")

	msg := err.Error()
	testutil.Contains(t, msg, "port 8090 is already in use")
	testutil.Contains(t, msg, "Try:")
	testutil.Contains(t, msg, "--port 8091")
	testutil.Contains(t, msg, "ayb stop")
}

func TestPortErrorOtherError(t *testing.T) {
	original := fmt.Errorf("permission denied")
	err := portError(8090, original)
	// Non-address-in-use errors should pass through unmodified.
	testutil.Equal(t, original, err)
}

func TestPortErrorSuggestsNextPort(t *testing.T) {
	err := portError(3000, fmt.Errorf("address already in use"))
	msg := err.Error()
	testutil.Contains(t, msg, "--port 3001")
}

// --- startupProgress ---

func TestStartupProgressHeader(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, true, false)
	sp.header("0.2.0")

	out := buf.String()
	testutil.Contains(t, out, "AllYourBase v0.2.0")
	testutil.Contains(t, out, "👾")
}

func TestStartupProgressInactiveIsNoop(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, false, false)
	sp.header("0.2.0")
	sp.step("Connecting...")
	sp.done()
	sp.fail()

	testutil.Equal(t, "", buf.String())
}

func TestStartupProgressStepDone(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, true, false)
	sp.step("Loading schema...")
	sp.done()

	out := buf.String()
	testutil.Contains(t, out, "Loading schema...")
	testutil.Contains(t, out, "✓")
}

func TestStartupProgressStepFail(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, true, false)
	sp.step("Starting server...")
	sp.fail()

	out := buf.String()
	testutil.Contains(t, out, "Starting server...")
	testutil.Contains(t, out, "✗")
}

// --- logFilePath ---

func TestLogFilePathFormat(t *testing.T) {
	p := logFilePath()
	if p == "" {
		t.Skip("logFilePath returned empty (likely no HOME)")
	}
	testutil.Contains(t, p, ".ayb/logs/ayb-")
	testutil.Contains(t, p, ".log")
	// Should contain today's date in YYYYMMDD format.
	today := time.Now().Format("20060102")
	testutil.Contains(t, p, today)
}

// --- cleanOldLogs ---

func TestCleanOldLogsRemovesStale(t *testing.T) {
	tmpDir := t.TempDir()
	logsDir := filepath.Join(tmpDir, ".ayb", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create an old log file (modification time 10 days ago).
	oldFile := filepath.Join(logsDir, "ayb-20260101.log")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().AddDate(0, 0, -10)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create a recent log file.
	newFile := filepath.Join(logsDir, "ayb-20260218.log")
	if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Override HOME so cleanOldLogs uses our temp dir.
	t.Setenv("HOME", tmpDir)
	cleanOldLogs()

	// Old file should be removed.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("expected old log file to be removed")
	}
	// New file should remain.
	if _, err := os.Stat(newFile); err != nil {
		t.Error("expected recent log file to remain")
	}
}

func TestCleanOldLogsNoDir(t *testing.T) {
	// Should not panic when the logs directory doesn't exist.
	t.Setenv("HOME", t.TempDir())
	cleanOldLogs() // no-op, should not panic
}

// --- newLogger ---

func TestNewLoggerReturnsComponents(t *testing.T) {
	logger, lvl, logPath, closer := newLogger("info", "json")
	defer closer()

	testutil.True(t, logger != nil, "logger should not be nil")
	testutil.True(t, lvl != nil, "level var should not be nil")
	// logPath may be empty if HOME is weird, but if present should have .log extension.
	if logPath != "" {
		testutil.Contains(t, logPath, ".log")
	}
}

func TestNewLoggerTextFormat(t *testing.T) {
	logger, _, _, closer := newLogger("info", "text")
	defer closer()
	testutil.True(t, logger != nil, "text logger should not be nil")
}

func TestNewLoggerLevelAdjustable(t *testing.T) {
	_, lvl, _, closer := newLogger("info", "json")
	defer closer()

	lvl.Set(slog.LevelWarn)
	testutil.Equal(t, slog.LevelWarn, lvl.Level())
}

// --- Banner body-only path ---

func TestBannerBodyToContainsAPIURL(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	printBannerBodyTo(&buf, cfg, false, false, "", "")

	out := buf.String()
	testutil.Contains(t, out, "http://localhost:8090/api")
	// Body only should NOT contain the version header.
	testutil.False(t, strings.Contains(out, "AllYourBase v"))
}
