package notify

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogDispatcher_NameAndCloseAreNoops(t *testing.T) {
	d := NewLogDispatcher(nil)
	if d.Name() != "log" {
		t.Fatalf("Name = %q, want %q", d.Name(), "log")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestLogDispatcher_NilLoggerFallsBackToDefault(t *testing.T) {
	// Constructor must accept nil and substitute slog.Default; Dispatch must
	// not panic on the resulting struct.
	d := NewLogDispatcher(nil)
	err := d.Dispatch(context.Background(), &Notification{
		ID: "evt-1", Title: "t", Body: "b", Level: slog.LevelInfo, Source: "test",
	})
	if err != nil {
		t.Fatalf("Dispatch with default logger: %v", err)
	}
}

func TestLogDispatcher_DispatchEmitsExpectedFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d := NewLogDispatcher(logger)

	n := &Notification{
		ID:        "evt-42",
		Title:     "the-title",
		Body:      "the-body",
		Level:     slog.LevelWarn,
		Source:    "the-source",
		Timestamp: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}
	if err := d.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	out := buf.String()
	wantFields := []string{
		`"msg":"notification"`,
		`"id":"evt-42"`,
		`"title":"the-title"`,
		`"body":"the-body"`,
		`"source":"the-source"`,
		`"level":"WARN"`,
	}
	for _, w := range wantFields {
		if !strings.Contains(out, w) {
			t.Errorf("expected %s in log output, got: %s", w, out)
		}
	}
}

func TestLogDispatcher_LevelRespected(t *testing.T) {
	// Below the handler's threshold → no output written.
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewLogDispatcher(logger)

	err := d.Dispatch(context.Background(), &Notification{
		ID: "x", Title: "t", Body: "b", Level: slog.LevelInfo, Source: "s",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Info-level notification should be filtered out by Error-only handler, got: %s", buf.String())
	}
}
