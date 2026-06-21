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

// When an endpoint is present the spawn seam must also enable the Claude Code
// CLI's own telemetry — forwarding the endpoint alone is inert (ce-ph5x).
func TestOtelEnvArgs_ClaudeTelemetryEnabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	spawnSessionID = ""
	got := otelEnvArgs()
	for _, want := range []string{
		"CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1",
		"OTEL_TRACES_EXPORTER=otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in args, got %q", want, got)
		}
	}
}

// Without an endpoint the CLI telemetry vars must NOT be injected — enabling
// telemetry with no collector would have the CLI retry-spamming localhost.
func TestOtelEnvArgs_NoEndpointNoClaudeTelemetry(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	spawnSessionID = "only-session"
	defer func() { spawnSessionID = "" }()
	got := otelEnvArgs()
	if strings.Contains(got, "CLAUDE_CODE_ENABLE_TELEMETRY") {
		t.Errorf("did not expect CLAUDE_CODE_ENABLE_TELEMETRY without endpoint, got %q", got)
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
