package main

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/override"
)

func TestOverrideAuditAlertMessageNamesBreachesAndProtectedLedger(t *testing.T) {
	report := override.AuditReport{
		Threshold: 3,
		Breaches: []override.KindBreach{
			{Kind: override.KindCodexHookTrust, Count: 4},
			{Kind: override.KindAdmissionBrake, Count: 3},
		},
	}
	message := overrideAuditAlertMessage(report)
	for _, want := range []string{
		"threshold 3",
		"codex-hook-trust=4",
		"admission-brake=3",
		"/var/log/dear-agent-overrides.jsonl",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("alert message %q does not contain %q", message, want)
		}
	}
}

func TestOverrideAuditAlertMessageUsesReportOrder(t *testing.T) {
	now := time.Now()
	report := override.Audit([]override.Use{
		{Kind: override.KindAdmissionBrake, Reason: "test alert delivery", AtUTC: now},
	}, time.Hour, 1, now)
	if got := overrideAuditAlertMessage(report); !strings.Contains(got, "admission-brake=1") {
		t.Fatalf("alert message = %q", got)
	}
}
