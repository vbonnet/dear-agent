package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestParseArchiveOutcome(t *testing.T) {
	valid := map[string]manifest.SessionOutcome{
		"":          manifest.OutcomeUnknown,
		"completed": manifest.OutcomeCompleted,
		"crashed":   manifest.OutcomeCrashed,
		"killed":    manifest.OutcomeKilled,
		"gc-stale":  manifest.OutcomeGCStale,
	}
	for in, want := range valid {
		got, err := parseArchiveOutcome(in)
		if err != nil {
			t.Errorf("parseArchiveOutcome(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Errorf("parseArchiveOutcome(%q) = %q, want %q", in, got, want)
		}
	}

	if _, err := parseArchiveOutcome("bogus"); err == nil {
		t.Error("parseArchiveOutcome(\"bogus\") expected an error, got nil")
	}
}

func TestOutcomeCell(t *testing.T) {
	if got := outcomeCell(ops.SessionSummary{Outcome: ""}); got != "-" {
		t.Errorf("outcomeCell(empty) = %q, want \"-\"", got)
	}
	if got := outcomeCell(ops.SessionSummary{Outcome: "killed"}); got != "killed" {
		t.Errorf("outcomeCell(killed) = %q, want \"killed\"", got)
	}
}

// TestPrintSessionSummaryTable_ShowsOutcomeColumn verifies the OUTCOME column
// appears (with values) only when showOutcome is set.
func TestPrintSessionSummaryTable_ShowsOutcomeColumn(t *testing.T) {
	sessions := []ops.SessionSummary{
		{ID: "1", Name: "done-worker", Status: "archived", Outcome: "completed", Harness: "claude-code"},
		{ID: "2", Name: "dead-worker", Status: "archived", Outcome: "gc-stale", Harness: "claude-code"},
	}

	// With outcome column.
	var withOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&withOut)
	printSessionSummaryTable(cmd, sessions, false, true)
	got := withOut.String()
	if !strings.Contains(got, "OUTCOME") {
		t.Errorf("expected OUTCOME header in output:\n%s", got)
	}
	for _, want := range []string{"completed", "gc-stale"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected outcome %q in output:\n%s", want, got)
		}
	}

	// Without outcome column.
	var noOut bytes.Buffer
	cmd2 := &cobra.Command{}
	cmd2.SetOut(&noOut)
	printSessionSummaryTable(cmd2, sessions, false, false)
	if strings.Contains(noOut.String(), "OUTCOME") {
		t.Errorf("did not expect OUTCOME header when showOutcome=false:\n%s", noOut.String())
	}
}
