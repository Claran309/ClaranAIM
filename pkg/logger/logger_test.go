package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerWritesInfoAndErrorFiles(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("CLARAN_LOG_DIR", logDir)
	t.Cleanup(func() {
		InitServiceWithPath("test-cleanup", filepath.Join(os.TempDir(), "claran-logger-test-cleanup"))
	})

	InitService("logger-test")
	Info("info message", "user_id", int64(1000000001))
	Error("error message", "err", "boom")
	Sync()

	dateDir := filepath.Join(logDir, "logger-test", time.Now().Format("2006-01-02"))
	infoBytes, err := os.ReadFile(filepath.Join(dateDir, "INFO.log"))
	if err != nil {
		t.Fatalf("read INFO.log: %v", err)
	}
	infoText := string(infoBytes)
	if !strings.Contains(infoText, "[logger-test]") || !strings.Contains(infoText, "info message") || !strings.Contains(infoText, "user_id") {
		t.Fatalf("INFO.log missing expected content: %s", infoText)
	}

	errBytes, err := os.ReadFile(filepath.Join(dateDir, "ERR.log"))
	if err != nil {
		t.Fatalf("read ERR.log: %v", err)
	}
	errText := string(errBytes)
	if !strings.Contains(errText, "[logger-test]") || !strings.Contains(errText, "error message") || !strings.Contains(errText, "boom") {
		t.Fatalf("ERR.log missing expected content: %s", errText)
	}
}
