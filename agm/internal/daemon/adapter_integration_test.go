package daemon

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// TestNewDaemon_WithOpenCodeAdapter tests daemon creation with OpenCode adapter enabled
func TestNewDaemon_WithOpenCodeAdapter(t *testing.T) {
	// Create test event bus
	hub := eventbus.NewHub()
	defer hub.Shutdown()

	// Create test config with OpenCode adapter enabled
	appConfig := config.Default()
	appConfig.Adapters.OpenCode.Enabled = true
	appConfig.Adapters.OpenCode.ServerURL = "http://localhost:4096"

	cfg := Config{
		BaseDir:   "/tmp/agm-test",
		LogDir:    "/tmp/agm-test/logs",
		PIDFile:   "/tmp/agm-test/daemon.pid",
		EventBus:  hub,
		AppConfig: appConfig,
		Logger:    testLogger(t),
	}

	daemon := NewDaemon(cfg)

	if daemon == nil {
		t.Fatal("Expected daemon to be created")
		return
	}

	// OpenCode adapter should be initialized (though it may fail to connect to non-existent server)
	// We just verify it was attempted
	if daemon.opencodeAdapter == nil {
		t.Error("Expected OpenCode adapter to be initialized when enabled")
	}
}

// TestNewDaemon_WithoutOpenCodeAdapter tests daemon creation without OpenCode adapter
func TestNewDaemon_WithoutOpenCodeAdapter(t *testing.T) {
	appConfig := config.Default()
	appConfig.Adapters.OpenCode.Enabled = false

	cfg := Config{
		BaseDir:   "/tmp/agm-test",
		LogDir:    "/tmp/agm-test/logs",
		PIDFile:   "/tmp/agm-test/daemon.pid",
		AppConfig: appConfig,
		Logger:    testLogger(t),
	}

	daemon := NewDaemon(cfg)

	if daemon == nil {
		t.Fatal("Expected daemon to be created")
		return
	}

	// OpenCode adapter should NOT be initialized when disabled
	if daemon.opencodeAdapter != nil {
		t.Error("Expected OpenCode adapter to be nil when disabled")
	}
}

// TestDaemon_GetAdapterHealth tests adapter health status retrieval
func TestDaemon_GetAdapterHealth(t *testing.T) {
	appConfig := config.Default()
	appConfig.Adapters.OpenCode.Enabled = false

	cfg := Config{
		BaseDir:   "/tmp/agm-test",
		LogDir:    "/tmp/agm-test/logs",
		PIDFile:   "/tmp/agm-test/daemon.pid",
		AppConfig: appConfig,
		Logger:    testLogger(t),
	}

	daemon := NewDaemon(cfg)

	adapterHealth := daemon.GetAdapterHealth()

	// With no adapters enabled, health should be empty
	if adapterHealth.OpenCode != nil {
		t.Error("Expected OpenCode adapter health to be nil when adapter disabled")
	}
}

// TestDaemon_StopWithAdapter tests graceful shutdown with adapter
func TestDaemon_StopWithAdapter(t *testing.T) {
	// Create test event bus
	hub := eventbus.NewHub()
	defer hub.Shutdown()

	appConfig := config.Default()
	appConfig.Adapters.OpenCode.Enabled = true
	appConfig.Adapters.OpenCode.ServerURL = "http://localhost:4096"

	cfg := Config{
		BaseDir:   "/tmp/agm-test",
		LogDir:    "/tmp/agm-test/logs",
		PIDFile:   "/tmp/agm-test/daemon.pid",
		EventBus:  hub,
		AppConfig: appConfig,
		Logger:    testLogger(t),
	}

	daemon := NewDaemon(cfg)

	// Start adapter (will fail to connect but that's OK for this test)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if daemon.opencodeAdapter != nil {
		_ = daemon.opencodeAdapter.Start(ctx)
	}

	// Stop daemon should not panic
	daemon.Stop()

	// Verify context was cancelled
	select {
	case <-daemon.ctx.Done():
		// Good, context was cancelled
	default:
		t.Error("Expected daemon context to be cancelled after Stop()")
	}
}

// testLogger creates a test logger that writes to testing.T
func testLogger(t *testing.T) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewTextHandler(&testWriter{t: t}, opts)
	return slog.New(handler)
}

// testWriter wraps testing.T to implement io.Writer
type testWriter struct {
	t *testing.T
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}
