package main

import (
	"strings"
	"testing"
)

func TestOtelEnvArgs_NoEndpointNoSession(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	spawnSessionID = ""
	got := otelEnvArgs()
	if got != "" {
		t.Errorf("expected empty string when no OTel config, got %q", got)
	}
}

func TestOtelEnvArgs_EndpointForwarded(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	spawnSessionID = ""
	got := otelEnvArgs()
	if !strings.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317") {
		t.Errorf("expected OTEL_EXPORTER_OTLP_ENDPOINT in args, got %q", got)
	}
}

func TestOtelEnvArgs_SessionIDInjected(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	spawnSessionID = "test-uuid-1234"
	defer func() { spawnSessionID = "" }()
	got := otelEnvArgs()
	if !strings.Contains(got, "ENGRAM_SESSION_ID=test-uuid-1234") {
		t.Errorf("expected ENGRAM_SESSION_ID in args, got %q", got)
	}
}

func TestOtelEnvArgs_BothPresent(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "tempo:4317")
	spawnSessionID = "abc-def"
	defer func() { spawnSessionID = "" }()
	got := otelEnvArgs()
	if !strings.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT=tempo:4317") {
		t.Errorf("missing endpoint in %q", got)
	}
	if !strings.Contains(got, "ENGRAM_SESSION_ID=abc-def") {
		t.Errorf("missing session ID in %q", got)
	}
}
