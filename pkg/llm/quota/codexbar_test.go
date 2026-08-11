package quota_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// liveFixture is a real `codexbar dashboard --identity redacted` capture
// from a host running CodexBar 0.49.2, trimmed to the fields this parser
// reads. It is the regression anchor for the schema: if CodexBar changes
// field names, this test fails rather than the router silently deciding
// every provider is unreadable.
func liveFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "codexbar-dashboard-live.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestParseCodexBarDashboardLiveCapture(t *testing.T) {
	snapshot, err := quota.ParseCodexBarDashboard(liveFixture(t), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if snapshot.Source != "codexbar" {
		t.Errorf("Source = %q, want codexbar", snapshot.Source)
	}
	if snapshot.SourceVersion != "0.49.2" {
		t.Errorf("SourceVersion = %q, want 0.49.2", snapshot.SourceVersion)
	}
	if want := time.Date(2026, 8, 11, 20, 43, 2, 0, time.UTC); !snapshot.GeneratedAt.Equal(want) {
		t.Errorf("GeneratedAt = %v, want %v", snapshot.GeneratedAt, want)
	}
	if want := 180 * time.Second; snapshot.StaleAfter != want {
		t.Errorf("StaleAfter = %v, want %v", snapshot.StaleAfter, want)
	}
	if len(snapshot.Providers) != 3 {
		t.Fatalf("got %d providers, want 3", len(snapshot.Providers))
	}
}

func TestParseCodexBarDashboardMapsSourceIDsToFamilies(t *testing.T) {
	snapshot, err := quota.ParseCodexBarDashboard(liveFixture(t), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tests := []struct {
		family   string
		sourceID string
	}{
		{family: "anthropic", sourceID: "claude"},
		{family: "openai", sourceID: "codex"},
		{family: "gemini", sourceID: "antigravity"},
	}
	for _, tc := range tests {
		t.Run(tc.family, func(t *testing.T) {
			got, ok := snapshot.Provider(tc.family)
			if !ok {
				t.Fatalf("family %q missing from snapshot", tc.family)
			}
			if got.SourceID != tc.sourceID {
				t.Errorf("SourceID = %q, want %q", got.SourceID, tc.sourceID)
			}
		})
	}
}

func TestParseCodexBarDashboardReadsSubBudgets(t *testing.T) {
	snapshot, err := quota.ParseCodexBarDashboard(liveFixture(t), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	openai, ok := snapshot.Provider("openai")
	if !ok {
		t.Fatal("openai missing")
	}
	if len(openai.Windows) != 2 {
		t.Fatalf("got %d windows, want 2", len(openai.Windows))
	}
	worst, ok := openai.MostConstrained()
	if !ok {
		t.Fatal("MostConstrained reported no windows")
	}
	if worst.Label != "Weekly" {
		t.Errorf("binding window = %q, want Weekly", worst.Label)
	}
	if worst.RemainingPercent != 48 || worst.UsedPercent != 52 {
		t.Errorf("Weekly = %.1f%% remaining / %.1f%% used, want 48/52",
			worst.RemainingPercent, worst.UsedPercent)
	}
	if worst.ResetAt.IsZero() {
		t.Error("binding window has no reset time")
	}
}

func TestParseCodexBarDashboardWindowsOutrankPartialError(t *testing.T) {
	// The live capture has codex reporting "cost refresh timed out"
	// alongside good rate-limit windows. A partial failure must not
	// discard a usable remaining-quota reading.
	snapshot, err := quota.ParseCodexBarDashboard(liveFixture(t), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	openai, ok := snapshot.Provider("openai")
	if !ok {
		t.Fatal("openai missing")
	}
	if openai.Availability != quota.AvailabilityOK {
		t.Errorf("Availability = %q, want ok", openai.Availability)
	}
	if openai.Note == "" {
		t.Error("partial-failure note was dropped")
	}
}

func TestParseCodexBarDashboardClassifiesAuthFailure(t *testing.T) {
	snapshot, err := quota.ParseCodexBarDashboard(liveFixture(t), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	anthropic, ok := snapshot.Provider("anthropic")
	if !ok {
		t.Fatal("anthropic missing")
	}
	if anthropic.Availability != quota.AvailabilityAuthRequired {
		t.Fatalf("Availability = %q, want auth_required", anthropic.Availability)
	}
	if anthropic.Availability.Known() {
		t.Error("an auth failure must not count as a usable reading")
	}
	if len(anthropic.Windows) != 0 {
		t.Errorf("got %d windows for an unreadable provider, want 0", len(anthropic.Windows))
	}
}

func TestParseCodexBarDashboardClassification(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    quota.Availability
	}{
		{
			name:    "disabled provider",
			payload: `{"providers":[{"id":"claude","enabled":false,"windows":[]}]}`,
			want:    quota.AvailabilityDisabled,
		},
		{
			name:    "oauth token missing is an auth failure",
			payload: `{"providers":[{"id":"claude","enabled":true,"windows":[],"error":{"message":"Claude OAuth access token missing. Run ` + "`claude`" + ` to authenticate."}}]}`,
			want:    quota.AvailabilityAuthRequired,
		},
		{
			name:    "not logged in is an auth failure",
			payload: `{"providers":[{"id":"gemini","enabled":true,"windows":[],"error":{"message":"Not logged in to Gemini. Run 'gemini' in Terminal to authenticate."}}]}`,
			want:    quota.AvailabilityAuthRequired,
		},
		{
			name:    "unsupported provider is merely unavailable",
			payload: `{"providers":[{"id":"openai","enabled":true,"windows":[],"error":{"message":"No available fetch strategy for openai."}}]}`,
			want:    quota.AvailabilityUnavailable,
		},
		{
			name:    "silence is unavailable",
			payload: `{"providers":[{"id":"codex","enabled":true,"windows":[]}]}`,
			want:    quota.AvailabilityUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := quota.ParseCodexBarDashboard([]byte(tc.payload), nil)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(snapshot.Providers) != 1 {
				t.Fatalf("got %d providers, want 1", len(snapshot.Providers))
			}
			if got := snapshot.Providers[0].Availability; got != tc.want {
				t.Errorf("Availability = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseCodexBarDashboardPrefersReadableSourceForFamily(t *testing.T) {
	// gemini and antigravity both map to the gemini family. When the
	// Gemini CLI is signed out and Antigravity holds the subscription,
	// the readable one must win.
	payload := `{"providers":[
	  {"id":"gemini","enabled":true,"windows":[],"error":{"message":"Not logged in to Gemini."}},
	  {"id":"antigravity","enabled":true,"windows":[{"kind":"session","label":"Gemini Models","remainingPercent":80,"usedPercent":20}]}
	]}`
	snapshot, err := quota.ParseCodexBarDashboard([]byte(payload), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gemini, ok := snapshot.Provider("gemini")
	if !ok {
		t.Fatal("gemini family missing")
	}
	if gemini.SourceID != "antigravity" {
		t.Errorf("SourceID = %q, want antigravity", gemini.SourceID)
	}
	if gemini.Availability != quota.AvailabilityOK {
		t.Errorf("Availability = %q, want ok", gemini.Availability)
	}
}

func TestParseCodexBarDashboardTakesMostConstrainedWhenBothSourcesReadable(t *testing.T) {
	payload := `{"providers":[
	  {"id":"gemini","enabled":true,"windows":[{"kind":"weekly","label":"Gemini weekly","remainingPercent":90,"usedPercent":10}]},
	  {"id":"antigravity","enabled":true,"windows":[{"kind":"weekly","label":"Antigravity weekly","remainingPercent":12,"usedPercent":88}]}
	]}`
	snapshot, err := quota.ParseCodexBarDashboard([]byte(payload), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gemini, _ := snapshot.Provider("gemini")
	worst, ok := gemini.MostConstrained()
	if !ok {
		t.Fatal("no windows")
	}
	if worst.RemainingPercent != 12 {
		t.Errorf("RemainingPercent = %.1f, want 12 (the tighter of the two sources)", worst.RemainingPercent)
	}
}

func TestParseCodexBarDashboardKeepsUnmappedProviderUnderItsOwnID(t *testing.T) {
	payload := `{"providers":[{"id":"NewVendor","enabled":true,"windows":[{"kind":"weekly","remainingPercent":40,"usedPercent":60}]}]}`
	snapshot, err := quota.ParseCodexBarDashboard([]byte(payload), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := snapshot.Provider("newvendor"); !ok {
		t.Errorf("unmapped provider vanished; families = %+v", snapshot.Providers)
	}
}

func TestParseCodexBarDashboardHonoursCustomAliases(t *testing.T) {
	payload := `{"providers":[{"id":"zai","enabled":true,"windows":[{"kind":"weekly","remainingPercent":40,"usedPercent":60}]}]}`
	snapshot, err := quota.ParseCodexBarDashboard([]byte(payload), map[string]string{"zai": "openrouter"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := snapshot.Provider("openrouter"); !ok {
		t.Errorf("alias not applied; families = %+v", snapshot.Providers)
	}
}

func TestParseCodexBarDashboardRejectsUnsupportedSchema(t *testing.T) {
	_, err := quota.ParseCodexBarDashboard([]byte(`{"schemaVersion":2,"providers":[]}`), nil)
	if err == nil {
		t.Fatal("want an error for an unsupported schema version")
	}
}

func TestParseCodexBarDashboardToleratesUnparseableTimestamps(t *testing.T) {
	// A window with a usable percentage and a broken clock is still
	// worth routing on; the whole read must not fail.
	payload := `{"schemaVersion":1,"generatedAt":"not-a-time","providers":[
	  {"id":"codex","enabled":true,"windows":[{"kind":"weekly","remainingPercent":30,"usedPercent":70,"resetAt":"tomorrow"}]}
	]}`
	snapshot, err := quota.ParseCodexBarDashboard([]byte(payload), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	openai, ok := snapshot.Provider("openai")
	if !ok {
		t.Fatal("openai missing")
	}
	if openai.Availability != quota.AvailabilityOK {
		t.Errorf("Availability = %q, want ok", openai.Availability)
	}
	if !snapshot.GeneratedAt.IsZero() {
		t.Error("an unparseable generatedAt should leave the zero time")
	}
}

func TestParseCodexBarDashboardRejectsGarbage(t *testing.T) {
	if _, err := quota.ParseCodexBarDashboard([]byte("not json"), nil); err == nil {
		t.Fatal("want a parse error")
	}
}

// fakeRunner stands in for process execution.
type fakeRunner struct {
	out  []byte
	err  error
	name string
	args []string
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.name = name
	f.args = args
	return f.out, f.err
}

func TestCodexBarReaderAlwaysRequestsRedactedIdentity(t *testing.T) {
	runner := &fakeRunner{out: liveFixture(t)}
	reader := quota.CodexBarReader{Runner: runner}
	if _, err := reader.Read(context.Background()); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if runner.name != quota.DefaultCodexBarCommand {
		t.Errorf("command = %q, want %q", runner.name, quota.DefaultCodexBarCommand)
	}
	want := []string{"dashboard", "--identity", "redacted"}
	if len(runner.args) != len(want) {
		t.Fatalf("args = %v, want %v", runner.args, want)
	}
	for i := range want {
		if runner.args[i] != want[i] {
			t.Fatalf("args = %v, want %v", runner.args, want)
		}
	}
}

func TestCodexBarReaderReportsCommandFailure(t *testing.T) {
	runner := &fakeRunner{err: errors.New("codexbar: not installed")}
	reader := quota.CodexBarReader{Runner: runner}
	snapshot, err := reader.Read(context.Background())
	if err == nil {
		t.Fatal("want an error when the command fails")
	}
	if snapshot != nil {
		t.Error("want a nil snapshot on failure")
	}
}
