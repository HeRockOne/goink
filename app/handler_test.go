package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	a := New(logger)
	require.NotNil(t, a)
	assert.NotNil(t, a.logger, "logger should be set by New")
}

// ---------------------------------------------------------------------------
// IsInitialized
// ---------------------------------------------------------------------------

func TestIsInitialized_BeforeStartup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	a := New(logger)
	assert.False(t, a.IsInitialized(), "fresh App should not be initialized before startup/setup")
}

func TestIsInitialized_AfterSetup(t *testing.T) {
	a := setupTestApp(t)
	assert.True(t, a.IsInitialized(), "setupTestApp should set cfg so IsInitialized returns true")
}

// ---------------------------------------------------------------------------
// OnStartup
// ---------------------------------------------------------------------------

func TestOnStartup_NoConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	a := New(logger)

	// Call OnStartup. Whether it fully initializes depends on whether
	// ~/.goink/config.json exists. The key invariant is: no panic.
	assert.NotPanics(t, func() {
		a.OnStartup(context.Background())
	}, "OnStartup should not panic regardless of config state")
}

// ---------------------------------------------------------------------------
// OnShutdown
// ---------------------------------------------------------------------------

func TestOnShutdown(t *testing.T) {
	a := setupTestApp(t)

	// OnShutdown should not panic even on a fully initialized app.
	assert.NotPanics(t, func() {
		a.OnShutdown(context.Background())
	}, "OnShutdown should not panic on a fully initialized app")
}
