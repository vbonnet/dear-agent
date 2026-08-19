package main

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dispatchstate"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestCompletionSurfacerRelayTargetUsesLiveStateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := dispatchstate.SetRelayTarget(home, "dispatch-live"); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	t.Setenv("AGM_COMPLETION_RELAY_TARGET", "")

	cs := &completionSurfacer{orchestrator: "vroom-orchestrator"}
	if got := cs.relayTarget(); got != "dispatch-live" {
		t.Fatalf("relayTarget() = %q, want dispatch-live", got)
	}
}

func TestCompletionSurfacerRelayTargetKeepsFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGM_COMPLETION_RELAY_TARGET", "")

	cs := &completionSurfacer{orchestrator: "vroom-orchestrator"}
	if got := cs.relayTarget(); got != "vroom-orchestrator" {
		t.Fatalf("relayTarget() = %q, want fallback", got)
	}
	if _, err := os.Stat(dispatchstate.RelayTargetPath(home)); !os.IsNotExist(err) {
		t.Fatalf("relay target state unexpectedly exists: %v", err)
	}
}

// The relay target may be set to any identifier AGM accepts for a session:
// its name, its full ID, or a UUID prefix. The self-filter originally
// compared the name only, so every ID-shaped target slipped past it and the
// Dispatch session's own completion was relayed back into itself.
func TestEventIsSessionMatchesEveryIdentifierForm(t *testing.T) {
	event := ops.CompletionEvent{
		SessionName: "dispatch-primary",
		SessionID:   "7f3c9a21-4d55-4b0e-9c1a-2b8e6f0d1234",
	}
	for name, tc := range map[string]struct {
		target string
		want   bool
	}{
		"exact name":             {"dispatch-primary", true},
		"name different case":    {"Dispatch-Primary", true},
		"name with whitespace":   {"  dispatch-primary\n", true},
		"full id":                {"7f3c9a21-4d55-4b0e-9c1a-2b8e6f0d1234", true},
		"uuid prefix":            {"7f3c9a21", true},
		"long uuid prefix":       {"7f3c9a21-4d55", true},
		"unrelated name":         {"vroom-orchestrator", false},
		"unrelated id":           {"11111111-2222-3333-4444-555555555555", false},
		"empty target":           {"", false},
		"too-short prefix":       {"7f3c", false},
		"prefix of the wrong id": {"11111111", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := eventIsSession(event, tc.target); got != tc.want {
				t.Fatalf("eventIsSession(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

// A short target must not match: over-matching silently drops an unrelated
// worker's completion, which is the worse direction of a wrong answer.
func TestEventIsSessionIgnoresShortPrefixes(t *testing.T) {
	event := ops.CompletionEvent{SessionName: "worker-a", SessionID: "abcdef1234567890"}
	for _, target := range []string{"a", "ab", "abc", "abcd", "abcde", "abcdef", "abcdef1"} {
		if eventIsSession(event, target) {
			t.Fatalf("eventIsSession(%q) = true, want false for a prefix under %d chars", target, minIDPrefixMatch)
		}
	}
}

func TestPlanForExcludesTheRelayTargetByID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGM_COMPLETION_RELAY_TARGET", "")
	const dispatchID = "7f3c9a21-4d55-4b0e-9c1a-2b8e6f0d1234"
	if _, err := dispatchstate.SetRelayTarget(home, dispatchID); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	cs := &completionSurfacer{orchestrator: "vroom-orchestrator"}

	self := ops.CompletionEvent{SessionName: "dispatch-primary", SessionID: dispatchID}
	if plan := cs.planFor(self); plan.surface {
		t.Fatal("planFor surfaced the relay target's own completion; that relay reactivates the session and loops")
	}

	other := ops.CompletionEvent{SessionName: "worker-7", SessionID: "11111111-2222-3333-4444-555555555555"}
	plan := cs.planFor(other)
	if !plan.surface {
		t.Fatal("planFor dropped an unrelated worker completion")
	}
	if plan.target != dispatchID {
		t.Fatalf("plan target = %q, want the live target %q", plan.target, dispatchID)
	}
}

func TestPlanForAppliesConfiguredExcludes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGM_COMPLETION_RELAY_TARGET", "")
	cs := &completionSurfacer{orchestrator: "dispatch", excludes: []string{"overseer"}}
	if plan := cs.planFor(ops.CompletionEvent{SessionName: "vroom-OVERSEER", SessionID: "id-1"}); plan.surface {
		t.Fatal("planFor surfaced an excluded session")
	}
}

// The self-filter is only sound if one target snapshot governs both the
// filter and the delivery. With two independent reads, a retarget landing
// between them lets an event pass the filter against the old target and
// then be relayed against the new one, into the very session it reports.
// Run under -race.
func TestPlanForNeverRoutesAnEventIntoItsOwnSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGM_COMPLETION_RELAY_TARGET", "")

	const (
		alphaID = "aaaaaaaa-1111-2222-3333-444444444444"
		betaID  = "bbbbbbbb-1111-2222-3333-444444444444"
	)
	if _, err := dispatchstate.SetRelayTarget(home, alphaID); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	cs := &completionSurfacer{orchestrator: "vroom-orchestrator"}
	events := []ops.CompletionEvent{
		{SessionName: "alpha", SessionID: alphaID},
		{SessionName: "beta", SessionID: betaID},
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	bad := make(chan string, 8)

	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, event := range events {
					plan := cs.planFor(event)
					// The invariant: a surfaced event is never delivered
					// to the session it reports.
					if plan.surface && eventIsSession(event, plan.target) {
						select {
						case bad <- fmt.Sprintf("event %s would relay into itself (target %q)", event.SessionID, plan.target):
						default:
						}
						return
					}
				}
			}
		})
	}

	for i := range 400 {
		target := alphaID
		if i%2 == 0 {
			target = betaID
		}
		if _, err := dispatchstate.SetRelayTarget(home, target); err != nil {
			t.Errorf("SetRelayTarget() error: %v", err)
			break
		}
	}
	close(stop)
	wg.Wait()
	close(bad)

	for msg := range bad {
		t.Fatalf("self-relay window observed: %s", msg)
	}
}
