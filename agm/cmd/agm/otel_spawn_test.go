package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func telemetryLaunchCommand(sessionID string) string {
	return testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "telemetry",
		SessionID: sessionID, WorkDir: "/tmp/work", ForwardTelemetry: true,
	})
}

func telemetryChildEnvironment(parent []string, sessionID string) []string {
	return harnessexec.ClaudeEnvironment(parent, harnessexec.ClaudeLaunch{
		SessionName: "telemetry", SessionID: sessionID, Model: "claude-test",
		ForwardTelemetry: true,
	}, "")
}

func TestOtelEnvArgs_NoEndpointNoSession(t *testing.T) {
	got := telemetryChildEnvironment(nil, "")
	if containsEnvPrefix(got, "OTEL_") || containsEnvPrefix(got, "ENGRAM_SESSION_ID=") {
		t.Errorf("expected no telemetry env when no OTel config, got %q", got)
	}
}

func TestOtelEnvArgs_EndpointForwardedOutOfBand(t *testing.T) {
	const endpoint = "localhost:4317"
	got := telemetryChildEnvironment([]string{"OTEL_EXPORTER_OTLP_ENDPOINT=" + endpoint}, "")
	if !slices.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT="+endpoint) {
		t.Errorf("expected OTEL endpoint in child environment, got %q", got)
	}
	if strings.Contains(telemetryLaunchCommand(""), endpoint) {
		t.Errorf("OTEL endpoint appeared in tmux command: %q", telemetryLaunchCommand(""))
	}
}

func TestOtelEnvArgs_EndpointMetacharactersNeverReachCommand(t *testing.T) {
	const endpoint = "evil:4317; rm -rf /"
	got := telemetryChildEnvironment([]string{"OTEL_EXPORTER_OTLP_ENDPOINT=" + endpoint}, "")
	if !slices.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT="+endpoint) {
		t.Errorf("expected exact endpoint in child environment, got %q", got)
	}
	if strings.Contains(telemetryLaunchCommand(""), endpoint) {
		t.Errorf("endpoint metacharacters appeared in shell command: %q", telemetryLaunchCommand(""))
	}
}

// When an endpoint is present the private executor must also enable the Claude
// CLI's own telemetry; forwarding the endpoint alone is inert (ce-ph5x).
func TestOtelEnvArgs_ClaudeTelemetryEnabled(t *testing.T) {
	got := telemetryChildEnvironment([]string{"OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317"}, "")
	for _, want := range []string{
		"CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1",
		"OTEL_TRACES_EXPORTER=otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("expected %q in child environment, got %q", want, got)
		}
	}
}

func TestOtelEnvArgs_NoEndpointNoClaudeTelemetry(t *testing.T) {
	got := telemetryChildEnvironment(nil, "only-session")
	if containsEnvPrefix(got, "CLAUDE_CODE_ENABLE_TELEMETRY=") {
		t.Errorf("did not expect Claude telemetry without endpoint, got %q", got)
	}
}

func TestOtelEnvArgs_SessionIDInjectedOutOfBand(t *testing.T) {
	got := telemetryChildEnvironment(nil, "test-uuid-1234")
	if !slices.Contains(got, "ENGRAM_SESSION_ID=test-uuid-1234") {
		t.Errorf("expected ENGRAM_SESSION_ID in child environment, got %q", got)
	}
	command := telemetryLaunchCommand("test-uuid-1234")
	if !strings.Contains(command, "--session-id 'test-uuid-1234'") {
		t.Errorf("expected non-secret session id in private protocol, got %q", command)
	}
}

func TestOtelEnvArgs_BothPresent(t *testing.T) {
	got := telemetryChildEnvironment([]string{"OTEL_EXPORTER_OTLP_ENDPOINT=tempo:4317"}, "abc-def")
	for _, want := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT=tempo:4317", "ENGRAM_SESSION_ID=abc-def"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func containsEnvPrefix(environment []string, prefix string) bool {
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
