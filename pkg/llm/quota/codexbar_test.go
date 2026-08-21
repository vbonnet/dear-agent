package quota_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// TestParseCodexBarDashboardExcludesForeignWindowsFromGeminiFamily guards
// against Antigravity's "Claude/GPT" third-party-proxy sub-budgets, bucketed
// under the "gemini" family alongside genuine Gemini windows, dragging down
// (or propping up) MostConstrained() for a Gemini candidate. The live
// dashboard capture in testdata reproduces this mix (codex review on #1218).
func TestParseCodexBarDashboardExcludesForeignWindowsFromGeminiFamily(t *testing.T) {
	snapshot, err := quota.ParseCodexBarDashboard(liveFixture(t), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gemini, ok := snapshot.Provider("gemini")
	if !ok {
		t.Fatal("gemini missing")
	}
	for _, w := range gemini.Windows {
		lower := strings.ToLower(w.Label)
		if strings.Contains(lower, "claude") || strings.Contains(lower, "gpt") {
			t.Errorf("gemini family retained a foreign window: %+v", w)
		}
	}
	worst, ok := gemini.MostConstrained()
	if !ok {
		t.Fatal("MostConstrained reported no windows")
	}
	if strings.Contains(strings.ToLower(worst.Label), "claude") || strings.Contains(strings.ToLower(worst.Label), "gpt") {
		t.Errorf("MostConstrained picked a foreign window: %+v", worst)
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
			payload: `{"schemaVersion":1,"providers":[{"id":"claude","enabled":false,"windows":[]}]}`,
			want:    quota.AvailabilityDisabled,
		},
		{
			name:    "oauth token missing is an auth failure",
			payload: `{"schemaVersion":1,"providers":[{"id":"claude","enabled":true,"windows":[],"error":{"message":"Claude OAuth access token missing. Run ` + "`claude`" + ` to authenticate."}}]}`,
			want:    quota.AvailabilityAuthRequired,
		},
		{
			name:    "not logged in is an auth failure",
			payload: `{"schemaVersion":1,"providers":[{"id":"gemini","enabled":true,"windows":[],"error":{"message":"Not logged in to Gemini. Run 'gemini' in Terminal to authenticate."}}]}`,
			want:    quota.AvailabilityAuthRequired,
		},
		{
			name:    "unsupported provider is merely unavailable",
			payload: `{"schemaVersion":1,"providers":[{"id":"openai","enabled":true,"windows":[],"error":{"message":"No available fetch strategy for openai."}}]}`,
			want:    quota.AvailabilityUnavailable,
		},
		{
			name:    "silence is unavailable",
			payload: `{"schemaVersion":1,"providers":[{"id":"codex","enabled":true,"windows":[]}]}`,
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
	payload := `{"schemaVersion":1,"providers":[
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
	payload := `{"schemaVersion":1,"providers":[
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
	payload := `{"schemaVersion":1,"providers":[{"id":"NewVendor","enabled":true,"windows":[{"kind":"weekly","remainingPercent":40,"usedPercent":60}]}]}`
	snapshot, err := quota.ParseCodexBarDashboard([]byte(payload), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := snapshot.Provider("newvendor"); !ok {
		t.Errorf("unmapped provider vanished; families = %+v", snapshot.Providers)
	}
}

func TestParseCodexBarDashboardHonoursCustomAliases(t *testing.T) {
	payload := `{"schemaVersion":1,"providers":[{"id":"zai","enabled":true,"windows":[{"kind":"weekly","remainingPercent":40,"usedPercent":60}]}]}`
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

// fakeRunner stands in for process execution, recording every call so a
// multi-invocation read can be asserted in full.
type fakeRunner struct {
	out  []byte
	err  error
	name string
	args []string

	calls [][]string
	// paceOut, when set, is returned for the usage invocation.
	paceOut []byte
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.name = name
	f.args = args
	f.calls = append(f.calls, args)
	if len(args) > 0 && args[0] == "usage" {
		if f.paceOut == nil {
			return nil, errors.New("no pace configured")
		}
		return f.paceOut, nil
	}
	return f.out, f.err
}

func (f *fakeRunner) callWith(subcommand string) ([]string, bool) {
	for _, args := range f.calls {
		if len(args) > 0 && args[0] == subcommand {
			return args, true
		}
	}
	return nil, false
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
	got, ok := runner.callWith("dashboard")
	if !ok {
		t.Fatalf("no dashboard invocation; calls = %v", runner.calls)
	}
	want := []string{"dashboard", "--identity", "redacted"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}
}

// The usage surface has no redaction mode, so the guarantee has to come
// from the parser's shape rather than a flag. This pins the invocation so
// a future edit cannot quietly start asking for full identities.
func TestCodexBarReaderNeverRequestsFullIdentity(t *testing.T) {
	runner := &fakeRunner{out: liveFixture(t)}
	reader := quota.CodexBarReader{Runner: runner}
	if _, err := reader.Read(context.Background()); err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, args := range runner.calls {
		for i, arg := range args {
			if arg != "--identity" {
				continue
			}
			if i+1 >= len(args) || args[i+1] != "redacted" {
				t.Errorf("call %v requests a non-redacted identity", args)
			}
		}
	}
}

func TestCodexBarReaderSkipPaceAvoidsTheSecondInvocation(t *testing.T) {
	runner := &fakeRunner{out: liveFixture(t)}
	reader := quota.CodexBarReader{Runner: runner, SkipPace: true}
	if _, err := reader.Read(context.Background()); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, ok := runner.callWith("usage"); ok {
		t.Errorf("SkipPace still ran the usage invocation; calls = %v", runner.calls)
	}
}

// A burn-rate read that fails must not cost us the quota windows we
// already have.
func TestCodexBarReaderKeepsWindowsWhenPaceReadFails(t *testing.T) {
	runner := &fakeRunner{out: liveFixture(t)} // paceOut nil → usage call errors
	reader := quota.CodexBarReader{Runner: runner}
	snapshot, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	openai, ok := snapshot.Provider("openai")
	if !ok {
		t.Fatal("openai missing")
	}
	if openai.Availability != quota.AvailabilityOK {
		t.Errorf("Availability = %q, want ok", openai.Availability)
	}
	if openai.Pace != nil {
		t.Errorf("Pace = %+v, want nil after a failed burn-rate read", openai.Pace)
	}
}

func TestCodexBarReaderAttachesPaceToTheRightProvider(t *testing.T) {
	runner := &fakeRunner{
		out: liveFixture(t),
		paceOut: []byte(`[{"provider":"codex","pace":{"secondary":{
		  "stage":"farAhead","deltaPercent":39,"expectedUsedPercent":14,
		  "willLastToReset":false,"etaSeconds":73570,
		  "summary":"39% in deficit | Expected 14% used | Runs out in 20h 27m"}}}]`),
	}
	reader := quota.CodexBarReader{Runner: runner}
	snapshot, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	openai, _ := snapshot.Provider("openai")
	if openai.Pace == nil {
		t.Fatal("pace was not attached to the openai family")
	}
	if !openai.Pace.Overspending() {
		t.Error("want Overspending for a window that will not last to reset")
	}
	if openai.Pace.ExhaustsIn != 73570*time.Second {
		t.Errorf("ExhaustsIn = %v, want 73570s", openai.Pace.ExhaustsIn)
	}
	gemini, _ := snapshot.Provider("gemini")
	if gemini.Pace != nil {
		t.Errorf("pace leaked onto gemini: %+v", gemini.Pace)
	}
}

// The usage payload carries account emails and has no redaction flag.
// The parser's field set is the guarantee, so assert the type genuinely
// cannot carry identity through.
func TestParseCodexBarPaceDropsAccountIdentity(t *testing.T) {
	payload := []byte(`[{"provider":"codex","usage":{"accountEmail":"someone@example.com"},
	  "pace":{"secondary":{"stage":"onTrack","willLastToReset":true,"deltaPercent":1}}}]`)
	byID, err := quota.ParseCodexBarPace(payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pace, ok := byID["codex"]
	if !ok {
		t.Fatal("codex pace missing")
	}
	if pace.Overspending() {
		t.Error("an on-track window must not read as overspending")
	}
	rendered, err := json.Marshal(pace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(rendered), "example.com") {
		t.Errorf("account identity survived the parse: %s", rendered)
	}
}

func TestParseCodexBarPaceTakesTheMostPressingRung(t *testing.T) {
	payload := []byte(`[{"provider":"codex","pace":{
	  "primary":{"stage":"onTrack","willLastToReset":true,"deltaPercent":2},
	  "secondary":{"stage":"farAhead","willLastToReset":false,"deltaPercent":39}}}]`)
	byID, err := quota.ParseCodexBarPace(payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !byID["codex"].Overspending() {
		t.Error("a provider with any window that will not last must read as overspending")
	}
	if byID["codex"].DeltaPercent != 39 {
		t.Errorf("DeltaPercent = %v, want the worst rung's 39", byID["codex"].DeltaPercent)
	}
}

func TestNilPaceIsNotOverspending(t *testing.T) {
	var p *quota.Pace
	if p.Overspending() {
		t.Error("a missing burn-rate reading must never read as overspending")
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

// ADR-038's audited floor (engram-research #313): every consumer that
// gates on a CodexBar reading — cmd/workflow-run's routing meter and
// agm's scheduled refresh alike — must reject a build below this
// version rather than trust unaudited evidence. One shared
// implementation is what makes "every consumer" actually true.
func TestMeetsMinCodexBarVersion(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		want      bool
	}{
		{name: "audited version", installed: "0.49.0", want: true},
		{name: "well above the floor", installed: "0.49.2", want: true},
		{name: "future major version", installed: "1.0.0", want: true},
		{name: "below the floor", installed: "0.48.9", want: false},
		{name: "well below the floor", installed: "0.30.0", want: false},
		{name: "unparseable version does not meet the floor", installed: "not-a-version", want: false},
		{name: "empty version does not meet the floor", installed: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quota.MeetsMinCodexBarVersion(tt.installed); got != tt.want {
				t.Errorf("MeetsMinCodexBarVersion(%q) = %t, want %t", tt.installed, got, tt.want)
			}
		})
	}
}
