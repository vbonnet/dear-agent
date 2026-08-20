package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// stubReader returns a canned snapshot without touching a meter binary.
type stubReader struct{ snapshot *quota.Snapshot }

func (s stubReader) Read(context.Context) (*quota.Snapshot, error) { return s.snapshot, nil }

func readableProvider(family string, remaining float64) quota.ProviderQuota {
	return quota.ProviderQuota{
		Family:       family,
		SourceID:     family,
		Plan:         "Test Plan",
		Availability: quota.AvailabilityOK,
		Windows: []quota.Window{{
			ID:               "weekly",
			Label:            "Weekly",
			RemainingPercent: remaining,
			UsedPercent:      100 - remaining,
			ResetAt:          time.Now().Add(48 * time.Hour),
		}},
	}
}

func warmedMeter(t *testing.T, snapshot *quota.Snapshot) *quota.Meter {
	t.Helper()
	meter := quota.New(quota.Options{Reader: stubReader{snapshot: snapshot}, RefreshInterval: -1})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return meter
}

// captureRun runs the command with stdout redirected to a temp file and
// returns the exit code together with everything printed.
func captureRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := run(args, &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

func TestRunRejectsAnUnexpectedArgument(t *testing.T) {
	code, output := captureRun(t, "unexpected")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(output, "unexpected") {
		t.Errorf("output should name the bad argument: %s", output)
	}
}

func TestRunRejectsAnUnknownFlag(t *testing.T) {
	if code, _ := captureRun(t, "--not-a-flag"); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// A meter that is not installed is a read failure, not a report of zero
// quota. The command must say so and exit non-zero rather than print an
// empty table that reads like "everything is spent".
func TestRunReportsAnUnreadableMeter(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-codexbar")
	code, output := captureRun(t, "--command", missing)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(output, "quota-meter:") {
		t.Errorf("output should explain the failure: %s", output)
	}
}

func TestBuildReportCarriesTheReadingAndItsAge(t *testing.T) {
	generated := time.Now().Add(-2 * time.Minute)
	snapshot := &quota.Snapshot{
		Source:        "codexbar",
		SourceVersion: "0.49.2",
		GeneratedAt:   generated,
		StaleAfter:    3 * time.Minute,
		Providers:     []quota.ProviderQuota{readableProvider("openai", 48)},
	}
	meter := warmedMeter(t, snapshot)

	rep := buildReport(snapshot, meter, config{maxAge: 15 * time.Minute})
	if rep.Source != "codexbar" || rep.SourceVersion != "0.49.2" {
		t.Errorf("source = %q %q", rep.Source, rep.SourceVersion)
	}
	if rep.AgeSeconds < 100 {
		t.Errorf("AgeSeconds = %v, want roughly 120", rep.AgeSeconds)
	}
	if rep.SourceStaleAfterSeconds != 180 {
		t.Errorf("SourceStaleAfterSeconds = %v, want 180", rep.SourceStaleAfterSeconds)
	}
	if rep.Stale {
		t.Error("a 2-minute-old reading is not stale against a 15-minute limit")
	}
	if len(rep.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(rep.Providers))
	}
	got := rep.Providers[0]
	if !got.Readable || got.RemainingPercent == nil || *got.RemainingPercent != 48 {
		t.Errorf("provider = %+v, want a readable 48%%", got)
	}
	if len(got.Windows) != 1 || got.Windows[0].Label != "Weekly" {
		t.Errorf("windows = %+v, want the Weekly sub-budget", got.Windows)
	}
}

func TestBuildReportMarksAStaleReading(t *testing.T) {
	snapshot := &quota.Snapshot{
		Source:      "codexbar",
		GeneratedAt: time.Now().Add(-time.Hour),
		Providers:   []quota.ProviderQuota{readableProvider("openai", 48)},
	}
	rep := buildReport(snapshot, warmedMeter(t, snapshot), config{maxAge: time.Minute})
	if !rep.Stale {
		t.Error("want the report marked stale")
	}
}

// The command's central promise: an unreadable provider is reported as
// unreadable, never as exhausted.
func TestBuildReportDistinguishesUnreadableFromExhausted(t *testing.T) {
	snapshot := &quota.Snapshot{
		Source:      "codexbar",
		GeneratedAt: time.Now(),
		Providers: []quota.ProviderQuota{
			{
				Family:       "anthropic",
				SourceID:     "claude",
				Availability: quota.AvailabilityAuthRequired,
				Note:         "No Claude session key found in browser cookies.",
			},
			readableProvider("openai", 0),
		},
	}
	rep := buildReport(snapshot, warmedMeter(t, snapshot), config{maxAge: 15 * time.Minute})

	var unreadable, exhausted providerReport
	for _, p := range rep.Providers {
		switch p.Family {
		case "anthropic":
			unreadable = p
		case "openai":
			exhausted = p
		}
	}

	if unreadable.Readable {
		t.Error("an auth failure must not be reported as readable")
	}
	if unreadable.RemainingPercent != nil {
		t.Error("an unreadable provider must report no remaining percentage at all")
	}
	if !strings.Contains(unreadable.Reason, "not exhaustion") {
		t.Errorf("reason = %q, want it to say this is not exhaustion", unreadable.Reason)
	}
	if !exhausted.Readable || exhausted.RemainingPercent == nil || *exhausted.RemainingPercent != 0 {
		t.Errorf("a genuinely spent provider should report 0%%: %+v", exhausted)
	}
	if unreadable.Class == exhausted.Class {
		t.Errorf("unreadable and exhausted share class %q", unreadable.Class)
	}
}

func TestBuildReportWithNoSnapshot(t *testing.T) {
	rep := buildReport(nil, warmedMeter(t, nil), config{maxAge: time.Minute})
	if len(rep.Providers) != 0 {
		t.Errorf("want no providers, got %d", len(rep.Providers))
	}
}

func TestBuildRolesShowsTheQuotaAwareOrder(t *testing.T) {
	// openai is in a lower band than anthropic, so a role that configures
	// openai first should show anthropic promoted above it.
	snapshot := &quota.Snapshot{
		Source:      "codexbar",
		GeneratedAt: time.Now(),
		Providers: []quota.ProviderQuota{
			readableProvider("openai", 40),
			readableProvider("anthropic", 99),
		},
	}
	meter := warmedMeter(t, snapshot)

	registry := filepath.Join(t.TempDir(), "roles.yaml")
	body := `version: 1
roles:
  reviewer:
    primary: gpt-5.5-pro
    secondary: claude-opus-4-8
`
	if err := os.WriteFile(registry, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	reports, source := buildRoles(meter, registry)
	if source != registry {
		t.Errorf("source = %q, want the explicit file", source)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d roles, want 1", len(reports))
	}
	got := reports[0]
	if !got.Reordered {
		t.Error("want the role marked as reordered")
	}
	if got.Effective[0] != "claude-opus-4-8" {
		t.Errorf("effective order = %v, want claude first", got.Effective)
	}
	if got.Configured[0] != "gpt-5.5-pro" {
		t.Errorf("configured order = %v, want the file's order preserved", got.Configured)
	}
}

// TestBuildRolesNeverPromotesTheUnimplementedGeminiProvider guards the
// same exclusion pkg/llm/quota/meter.go enforces at the CLI-preview
// level: gemini has no constructible provider yet
// (provider.Factory's newGeminiProvider is a stub for both auth
// methods), so a role must never show it promoted ahead of a working
// candidate on the strength of a great quota reading it can never
// redeem (review on #1218).
func TestBuildRolesNeverPromotesTheUnimplementedGeminiProvider(t *testing.T) {
	snapshot := &quota.Snapshot{
		Source:      "codexbar",
		GeneratedAt: time.Now(),
		Providers: []quota.ProviderQuota{
			readableProvider("openai", 40),
			readableProvider("gemini", 99),
		},
	}
	meter := warmedMeter(t, snapshot)

	registry := filepath.Join(t.TempDir(), "roles.yaml")
	body := `version: 1
roles:
  reviewer:
    primary: gpt-5.5-pro
    secondary: gemini-3.1-pro
`
	if err := os.WriteFile(registry, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	reports, _ := buildRoles(meter, registry)
	if len(reports) != 1 {
		t.Fatalf("got %d roles, want 1", len(reports))
	}
	got := reports[0]
	if got.Reordered {
		t.Errorf("want no reordering (gemini excluded from promotion), got Effective = %v", got.Effective)
	}
	if got.Effective[0] != "gpt-5.5-pro" {
		t.Errorf("effective order = %v, want the configured order preserved", got.Effective)
	}
}

func TestBuildRolesReportsALoadFailure(t *testing.T) {
	meter := warmedMeter(t, nil)
	reports, _ := buildRoles(meter, filepath.Join(t.TempDir(), "absent.yaml"))
	if len(reports) != 1 || len(reports[0].Notes) == 0 {
		t.Fatalf("want a single explanatory entry, got %+v", reports)
	}
	if !strings.Contains(reports[0].Notes[0], "load roles") {
		t.Errorf("note = %q, want it to name the load failure", reports[0].Notes[0])
	}
}

func TestLabelOfPrefersLabelThenID(t *testing.T) {
	if got := labelOf(windowReport{Label: "Weekly", ID: "weekly"}); got != "Weekly" {
		t.Errorf("labelOf = %q, want Weekly", got)
	}
	if got := labelOf(windowReport{ID: "weekly"}); got != "weekly" {
		t.Errorf("labelOf = %q, want weekly", got)
	}
	if got := labelOf(windowReport{}); got != "usage window" {
		t.Errorf("labelOf = %q, want the placeholder", got)
	}
}

func TestOrDash(t *testing.T) {
	if orDash("") != "-" {
		t.Error("empty should render as a dash")
	}
	if orDash("codexbar") != "codexbar" {
		t.Error("a set value should pass through")
	}
}
