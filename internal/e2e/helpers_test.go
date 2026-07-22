//go:build cgo && e2e

package e2e

import (
	"log/slog"
	"os"
	"testing"

	"gorm.io/gorm"
)

// sharedDB is the global SQLite database initialized in TestMain and used by all tests.
var sharedDB *gorm.DB

// testLogger creates a slog.Logger that outputs to stderr for test visibility.
func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// getSharedDB returns the shared GORM database initialized in TestMain.
func getSharedDB(t *testing.T) *gorm.DB {
	t.Helper()
	if sharedDB == nil {
		t.Fatal("sharedDB is nil — was TestMain initialization successful?")
	}
	return sharedDB
}
