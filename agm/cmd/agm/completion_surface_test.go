package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestEventIsSessionMatchesEveryIdentifierForm(t *testing.T) {
	event := ops.CompletionEvent{
		SessionName: "dispatch-primary",
		SessionID:   "7f3c9a21-4d55-4b0e-9c1a-2b8e6f0d1234",
	}
	for name, tc := range map[string]struct {
		target string
		want   bool
	}{
		"exact name":           {"dispatch-primary", true},
		"name different case":  {"Dispatch-Primary", true},
		"name with whitespace": {"  dispatch-primary\n", true},
		"full id":              {"7f3c9a21-4d55-4b0e-9c1a-2b8e6f0d1234", true},
		"uuid prefix":          {"7f3c9a21", true},
		"unrelated name":       {"vroom-orchestrator", false},
		"unrelated id":         {"11111111-2222-3333-4444-555555555555", false},
		"empty target":         {"", false},
		"too-short prefix":     {"7f3c", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := eventIsSession(event, tc.target); got != tc.want {
				t.Fatalf("eventIsSession(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

// The regression this closes: with --orchestrator empty, routing discovers
// a live supervisor. If the completed session IS that supervisor, filtering
// on cs.orchestrator alone could not recognize it, so its own completion
// was routed back into it, waking it to complete again.
func TestPlanForExcludesTheDiscoveredSupervisor(t *testing.T) {
	surfacer := testSurfacer(t, "", surfacerManifest("Dispatch", manifest.StateReady))

	self := ops.CompletionEvent{SessionName: "Dispatch", SessionID: "Dispatch-id"}
	if plan := surfacer.planFor(self); plan.surface {
		t.Fatal("planFor surfaced the discovered supervisor's own completion; that relay reactivates it and loops")
	}

	worker := ops.CompletionEvent{SessionName: "worker-7", SessionID: "worker-7-id"}
	plan := surfacer.planFor(worker)
	if !plan.surface {
		t.Fatal("planFor dropped an unrelated worker completion")
	}
	if plan.target != "Dispatch" {
		t.Fatalf("plan target = %q, want the discovered supervisor", plan.target)
	}
}

// An explicitly loaded config with zero dispatchers is the documented way
// to turn completion notifications off. Routing must honor it too.
func TestPlanForHonorsExplicitlyDisabledNotifications(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "notify.yaml")
	if err := os.WriteFile(cfg, []byte("dispatchers: []\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	surfacer, err := newCompletionSurfacer(testOpCtx(surfacerManifest("Dispatch", manifest.StateReady)), cfg, "", "")
	if err != nil {
		t.Fatalf("newCompletionSurfacer() error: %v", err)
	}
	if !surfacer.notificationsDisabled {
		t.Fatal("notificationsDisabled = false for an explicit config with zero dispatchers")
	}
	if plan := surfacer.planFor(ops.CompletionEvent{SessionName: "worker-7", SessionID: "id-7"}); plan.surface {
		t.Fatal("planFor surfaced a completion although notifications are explicitly disabled")
	}
}

func TestPlanForAppliesConfiguredExcludes(t *testing.T) {
	surfacer := testSurfacer(t, "", surfacerManifest("Dispatch", manifest.StateReady))
	surfacer.excludes = []string{"overseer"}
	if plan := surfacer.planFor(ops.CompletionEvent{SessionName: "vroom-OVERSEER", SessionID: "id-1"}); plan.surface {
		t.Fatal("planFor surfaced an excluded session")
	}
}

// Surface must refuse a plan its own filter rejected, so a future caller
// that forgets to check plan.surface cannot reintroduce the self-relay.
func TestSurfaceRefusesARejectedPlan(t *testing.T) {
	surfacer := &completionSurfacer{}
	errs := surfacer.Surface(context.Background(),
		ops.CompletionEvent{SessionName: "dispatch", SessionID: "id-1"},
		relayPlan{surface: false, target: "dispatch"})
	if errs != nil {
		t.Fatalf("Surface() = %v, want no delivery for a rejected plan", errs)
	}
}

func testSurfacer(t *testing.T, orchestrator string, sessions ...*manifest.Manifest) *completionSurfacer {
	t.Helper()
	opCtx := testOpCtx(sessions...)
	router := ops.NewAlertRouter(opCtx)
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))
	return &completionSurfacer{opCtx: opCtx, orchestrator: orchestrator, router: router}
}

func testOpCtx(sessions ...*manifest.Manifest) *ops.OpContext {
	storage := dolt.NewMockAdapter()
	for _, session := range sessions {
		if err := storage.Create(session); err != nil {
			panic(err)
		}
	}
	return &ops.OpContext{Storage: storage}
}

func surfacerManifest(name, state string) *manifest.Manifest {
	return &manifest.Manifest{
		SessionID: name + "-id",
		Name:      name,
		State:     state,
		Context:   manifest.Context{Tags: []string{}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}
