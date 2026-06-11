package trace

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestRecordSessionLifecycleSpan_NoOp(t *testing.T) {
	// No OTEL_EXPORTER_OTLP_ENDPOINT is set in tests, so the global provider is
	// a no-op. Verify the function does not panic and returns cleanly.
	m := &manifest.Manifest{
		SessionID: "test-session-id",
		Name:      "test-session",
		Model:     "claude-sonnet-4-6",
		Harness:   "claude-code",
		Workspace: "oss",
		CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	end := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	RecordSessionLifecycleSpan(context.Background(), m, end, 0)
}

func TestRecordSessionLifecycleSpan_ZeroCreatedAt(t *testing.T) {
	// When CreatedAt is zero the function should fall back to endTime as startTime.
	m := &manifest.Manifest{
		SessionID: "test-zero-start",
		Name:      "test-zero",
		Model:     "claude-haiku-4-5-20251001",
		Harness:   "claude-code",
	}

	end := time.Now()
	RecordSessionLifecycleSpan(context.Background(), m, end, -1)
}

func TestRecordSessionLifecycleSpan_EmptyModel(t *testing.T) {
	// Attributes with empty strings are valid — ensure they don't cause a panic.
	m := &manifest.Manifest{
		SessionID: "test-empty-model",
		CreatedAt: time.Now().Add(-30 * time.Minute),
	}

	RecordSessionLifecycleSpan(context.Background(), m, time.Now(), 1)
}
