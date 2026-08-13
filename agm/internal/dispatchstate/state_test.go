package dispatchstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveRelayTargetPrefersLiveFileOverFallback(t *testing.T) {
	home := t.TempDir()
	result, err := SetRelayTarget(home, "dispatch-live")
	if err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	if result.Target != "dispatch-live" {
		t.Fatalf("SetRelayTarget target = %q, want dispatch-live", result.Target)
	}
	got := ResolveRelayTarget(home, "vroom-orchestrator", func(string) string { return "" })
	if got.Target != "dispatch-live" {
		t.Fatalf("ResolveRelayTarget target = %q, want live file target", got.Target)
	}
	if got.Source != "file" {
		t.Fatalf("ResolveRelayTarget source = %q, want file", got.Source)
	}
}

func TestResolveRelayTargetEnvOverridesFile(t *testing.T) {
	home := t.TempDir()
	if _, err := SetRelayTarget(home, "dispatch-file"); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	got := ResolveRelayTarget(home, "fallback", func(key string) string {
		if key == relayTargetEnv {
			return "dispatch-env"
		}
		return ""
	})
	if got.Target != "dispatch-env" {
		t.Fatalf("ResolveRelayTarget target = %q, want env target", got.Target)
	}
}

func TestReadQuotaStatusWarnsOnThrottleAndLowRemaining(t *testing.T) {
	home := t.TempDir()
	path := QuotaPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	data := `{"updated_at":"2026-08-12T12:00:00Z","remaining_percent":19,"throttled":true}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	got := ReadQuotaStatus(home, "codex", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
	if !got.Available {
		t.Fatalf("quota available = false, reason %q", got.Reason)
	}
	if !got.Warning {
		t.Fatalf("quota warning = false, want true")
	}
	if got.Stale {
		t.Fatalf("quota stale = true, want false")
	}
}

func TestReadQuotaStatusUnavailableWhenStateMissing(t *testing.T) {
	got := ReadQuotaStatus(t.TempDir(), "codex", time.Now())
	if got.Available {
		t.Fatalf("quota available = true, want false")
	}
	if got.Reason != "quota state not found" {
		t.Fatalf("quota reason = %q, want quota state not found", got.Reason)
	}
}
