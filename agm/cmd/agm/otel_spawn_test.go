package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func telemetryLaunchCommand(sessionID string) string {
	return testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "telemetry",
		SessionID: sessionID, WorkDir: "/tmp/work", ForwardTelemetry: true,
	})
}

func TestOtelEnvArgs_NoEndpointNoSession(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	got := telemetryLaunchCommand("")
	if strings.Contains(got, "OTEL_") || strings.Contains(got, "ENGRAM_SESSION_ID") {
		t.Errorf("expected no telemetry env when no OTel config, got %q", got)
	}
}

func TestOtelEnvArgs_EndpointForwarded(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	got := telemetryLaunchCommand("")
	if !strings.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT='localhost:4317'") {
		t.Errorf("expected shell-quoted OTEL_EXPORTER_OTLP_ENDPOINT in args, got %q", got)
	}
}

// The endpoint is interpolated into a shell command run via tmux, so a value
// with shell metacharacters must be shell-quoted to prevent command injection.
func TestOtelEnvArgs_EndpointShellQuoted(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "evil:4317; rm -rf /")
	got := telemetryLaunchCommand("")
	if !strings.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT='evil:4317; rm -rf /'") {
		t.Errorf("expected metacharacters to be shell-quoted, got %q", got)
	}
}

// When an endpoint is present the spawn seam must also enable the Claude Code
// CLI's own telemetry — forwarding the endpoint alone is inert (ce-ph5x).
func TestOtelEnvArgs_ClaudeTelemetryEnabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	got := telemetryLaunchCommand("")
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
	got := telemetryLaunchCommand("only-session")
	if strings.Contains(got, "CLAUDE_CODE_ENABLE_TELEMETRY") {
		t.Errorf("did not expect CLAUDE_CODE_ENABLE_TELEMETRY without endpoint, got %q", got)
	}
}

func TestOtelEnvArgs_SessionIDInjected(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	got := telemetryLaunchCommand("test-uuid-1234")
	if !strings.Contains(got, "ENGRAM_SESSION_ID='test-uuid-1234'") {
		t.Errorf("expected shell-quoted ENGRAM_SESSION_ID in args, got %q", got)
	}
}

func TestOtelEnvArgs_BothPresent(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "tempo:4317")
	got := telemetryLaunchCommand("abc-def")
	if !strings.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT='tempo:4317'") {
		t.Errorf("missing endpoint in %q", got)
	}
	if !strings.Contains(got, "ENGRAM_SESSION_ID='abc-def'") {
		t.Errorf("missing session ID in %q", got)
	}
}
