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

// publish writes a state file built from the given providers and returns
// its path.
func publish(t *testing.T, generatedAt time.Time, providers ...quota.ProviderQuota) string {
	t.Helper()
	snapshot := &quota.Snapshot{Source: "test", GeneratedAt: generatedAt, Providers: providers}
	meter := quota.New(quota.Options{
		Reader:          &stubReader{snapshot: snapshot},
		RefreshInterval: -1,
		Now:             func() time.Time { return generatedAt },
	})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	breaker := quota.NewBreaker(meter, quota.BreakerPolicy{})
	state := quota.BuildState(snapshot, meter, breaker, generatedAt)

	path := filepath.Join(t.TempDir(), "latest.json")
	if err := quota.WriteStateFile(path, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

func TestStateFileRoundTrip(t *testing.T) {
	now := time.Now()
	path := publish(t, now, provider("openai", 48, onTrack()), provider("gemini", 99, nil))

	state, err := quota.ReadStateFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if state.Version != quota.StateFileVersion {
		t.Errorf("Version = %d, want %d", state.Version, quota.StateFileVersion)
	}
	openai, ok := state.Provider("openai")
	if !ok {
		t.Fatal("openai missing from published state")
	}
	if !openai.Readable {
		t.Error("want openai readable")
	}
	if openai.RemainingPercent != 48 {
		t.Errorf("RemainingPercent = %v, want 48", openai.RemainingPercent)
	}
	if openai.BreakerState != string(quota.BreakerClosed) {
		t.Errorf("BreakerState = %q, want closed", openai.BreakerState)
	}
	if len(openai.Windows) == 0 {
		t.Error("want sub-budgets published so consumers need no second read")
	}
}

func TestStateFilePublishesTheBreakerVerdict(t *testing.T) {
	now := time.Now()
	path := publish(t, now, provider("openai", 1, nil))
	state, err := quota.ReadStateFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	openai, _ := state.Provider("openai")
	if openai.BreakerState != string(quota.BreakerOpen) {
		t.Errorf("BreakerState = %q, want open", openai.BreakerState)
	}
	if openai.Reason == "" {
		t.Error("want a reason published alongside the verdict")
	}
}

func TestStateFileMarksAnUnreadableProviderAsSuch(t *testing.T) {
	now := time.Now()
	path := publish(t, now, quota.ProviderQuota{
		Family:       "anthropic",
		SourceID:     "claude",
		Availability: quota.AvailabilityAuthRequired,
		Note:         "No Claude session key found in browser cookies.",
	})
	state, err := quota.ReadStateFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	anthropic, _ := state.Provider("anthropic")
	if anthropic.Readable {
		t.Error("an auth failure must not be published as readable")
	}
	if anthropic.Availability != string(quota.AvailabilityAuthRequired) {
		t.Errorf("Availability = %q, want auth_required", anthropic.Availability)
	}
	if anthropic.BreakerState != string(quota.BreakerClosed) {
		t.Errorf("BreakerState = %q, want closed — unreadable is not exhausted", anthropic.BreakerState)
	}
}

func TestReadStateFileDistinguishesAbsentFromCorrupt(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nothing.json")
	if _, err := quota.ReadStateFile(missing); !errors.Is(err, quota.ErrNoStateFile) {
		t.Errorf("err = %v, want ErrNoStateFile", err)
	}

	corrupt := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := quota.ReadStateFile(corrupt)
	if err == nil || errors.Is(err, quota.ErrNoStateFile) {
		t.Errorf("err = %v, want a distinct parse error", err)
	}
}

func TestReadStateFileRejectsAnUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.json")
	body, _ := json.Marshal(map[string]any{"version": quota.StateFileVersion + 1})
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := quota.ReadStateFile(path); err == nil {
		t.Fatal("want an error for an unrecognised state file version")
	}
}

func TestWriteStateFileIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.json")
	state := &quota.State{Version: quota.StateFileVersion, GeneratedAt: time.Now()}
	if err := quota.WriteStateFile(path, state); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".quota-state-") {
			t.Errorf("temp file %q was left behind", e.Name())
		}
	}
}

func TestSpawnGateRefusesAnExhaustedProvider(t *testing.T) {
	now := time.Now()
	gate := &quota.SpawnGate{
		Path: publish(t, now, provider("openai", 1, nil)),
		Now:  func() time.Time { return now },
	}
	got := gate.AllowSpawn("gpt-5.5-pro")
	if got.Allowed {
		t.Fatal("want the spawn refused on an exhausted provider")
	}
	if got.Family != "openai" {
		t.Errorf("Family = %q, want openai", got.Family)
	}
	err := got.RefusalError()
	if err == nil {
		t.Fatal("want a refusal error")
	}
	if !strings.Contains(err.Error(), "another provider") {
		t.Errorf("refusal should tell the operator what to do instead: %v", err)
	}
}

func TestSpawnGateAllowsAHealthyProvider(t *testing.T) {
	now := time.Now()
	gate := &quota.SpawnGate{
		Path: publish(t, now, provider("gemini", 95, onTrack())),
		Now:  func() time.Time { return now },
	}
	got := gate.AllowSpawn("gemini-3.1-pro")
	if !got.Allowed {
		t.Errorf("want the spawn allowed; reason = %q", got.Reason)
	}
	if got.RefusalError() != nil {
		t.Error("an allowed spawn must produce no refusal error")
	}
}

// The gate's safety property: it is fail-open on every path that is not
// a fresh, positive reading of exhaustion.
func TestSpawnGateFailsOpen(t *testing.T) {
	now := time.Now()
	stale := publish(t, now.Add(-6*time.Hour), provider("openai", 0, overspending()))
	unreadable := publish(t, now, quota.ProviderQuota{
		Family:       "openai",
		Availability: quota.AvailabilityAuthRequired,
		Note:         "sign in",
	})

	tests := []struct {
		name string
		gate *quota.SpawnGate
		want string
	}{
		{name: "nil gate", gate: nil, want: "no quota guardrail is wired"},
		{
			name: "no published reading",
			gate: &quota.SpawnGate{Path: filepath.Join(t.TempDir(), "absent.json"), Now: func() time.Time { return now }},
			want: "no usable quota reading",
		},
		{
			name: "reading past the gating age",
			gate: &quota.SpawnGate{Path: stale, MaxAge: time.Minute, Now: func() time.Time { return now }},
			want: "gating limit",
		},
		{
			name: "provider needs credentials",
			gate: &quota.SpawnGate{Path: unreadable, Now: func() time.Time { return now }},
			want: "unreadable, not exhausted",
		},
		{
			name: "unmapped model",
			gate: &quota.SpawnGate{Path: publish(t, now, provider("openai", 0, nil)), Now: func() time.Time { return now }},
			want: "not mapped",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := "gpt-5.5-pro"
			if tc.name == "unmapped model" {
				model = "some-house-model-9000"
			}
			got := tc.gate.AllowSpawn(model)
			if !got.Allowed {
				t.Fatalf("want the spawn allowed; reason = %q", got.Reason)
			}
			if !strings.Contains(got.Reason, tc.want) {
				t.Errorf("Reason = %q, want it to mention %q", got.Reason, tc.want)
			}
			// Fail-open is allowed, but it is not evidence. A caller must be
			// able to tell this pass from a measured one.
			if got.Evaluated {
				t.Error("a fail-open must not report Evaluated")
			}
		})
	}
}

func TestSpawnGateCorruptStateFileFailsOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := (&quota.SpawnGate{Path: path}).AllowSpawn("gpt-5.5-pro")
	if !got.Allowed {
		t.Errorf("a corrupt reading must not stop work; reason = %q", got.Reason)
	}
}

func TestModelFamilyHeuristic(t *testing.T) {
	tests := map[string]string{
		"claude-opus-4-8":      "anthropic",
		"gpt-5.5-pro":          "openai",
		"codex-mini":           "openai",
		"gemini-3.1-pro":       "gemini",
		"some-house-model-900": "",
	}
	for model, want := range tests {
		if got := quota.ModelFamilyHeuristic(model); got != want {
			t.Errorf("ModelFamilyHeuristic(%q) = %q, want %q", model, got, want)
		}
	}
}

// The built-in heuristic reads vendor tokens out of full model identifiers,
// and every AGM harness default is a bare tier alias that contains none. This
// test pins that blindness in place as a documented property rather than a
// latent surprise: it is why SpawnGate.FamilyForModel exists and why a caller
// that spawns by alias must wire it. Leaving it nil is what made the guardrail
// fail open on the default spawn of every provider.
func TestModelFamilyHeuristicCannotResolveTierAliases(t *testing.T) {
	for _, alias := range []string{"sonnet", "sonnet-200k", "opus", "haiku", "5.5", "3.5-flash", "glm-5.2"} {
		if got := quota.ModelFamilyHeuristic(alias); got != "" {
			t.Errorf("ModelFamilyHeuristic(%q) = %q; if the heuristic learns aliases,"+
				" update SpawnGate's documented contract with it", alias, got)
		}
	}
}

// An unwired gate must announce that it is not checking anything, so an inert
// guardrail cannot pass for a working one.
func TestSpawnGateWarnsOnEveryFailOpen(t *testing.T) {
	var warnings []string
	gate := &quota.SpawnGate{
		Path: filepath.Join(t.TempDir(), "absent.json"),
		Warn: func(msg string) { warnings = append(warnings, msg) },
	}
	got := gate.AllowSpawn("claude-opus-4-8")
	if !got.Allowed || got.Evaluated {
		t.Fatalf("want an unevaluated pass, got Allowed=%v Evaluated=%v", got.Allowed, got.Evaluated)
	}
	if len(warnings) != 1 {
		t.Fatalf("want one warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "without checking") {
		t.Errorf("warning does not say the spawn went unchecked: %s", warnings[0])
	}
}

// A measured verdict — either direction — must report Evaluated, so callers
// can distinguish evidence from absence of evidence.
func TestSpawnGateMarksMeasuredVerdictsEvaluated(t *testing.T) {
	now := time.Now()
	healthy := publish(t, now, provider("openai", 80, nil))
	if got := (&quota.SpawnGate{Path: healthy, Now: func() time.Time { return now }}).AllowSpawn("gpt-5.5-pro"); !got.Evaluated {
		t.Error("a healthy measured pass must report Evaluated")
	}
}
