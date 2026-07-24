package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

// Regression: the worker cap used to count every tmux session on the shared
// AGM socket, so a single `go test` fixture session ("test") or a supervisor
// pane consumed a worker slot and deadlocked dispatch (ce-2mib).
func TestCountWorkerSessions_NonWorkersDoNotConsumeSlots(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  int
	}{
		{
			name:  "test fixture session is not a worker",
			names: []string{"test"},
			want:  0,
		},
		{
			name:  "sentinel tmux fixtures are not workers",
			names: []string{"test", "test-openai-codex-idle"},
			want:  0,
		},
		{
			name:  "supervisor panes are not workers",
			names: []string{"vroom-meta-o", "orchestrator", "overseer"},
			want:  0,
		},
		{
			name:  "workers counted alongside noise",
			names: []string{"test", "worker-ce-2mib", "vroom-meta-o", "worker-ce-3knl"},
			want:  2,
		},
		{
			name:  "no sessions",
			names: nil,
			want:  0,
		},
		{
			name:  "prefix must anchor at the start",
			names: []string{"not-worker-ce-1234"},
			want:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := countWorkerSessions(tc.names, nil); got != tc.want {
				t.Errorf("countWorkerSessions(%v) = %d, want %d", tc.names, got, tc.want)
			}
		})
	}
}

// A worker whose session name predates the worker- convention is still counted
// when the AGM session DB reports it as tagged role:worker.
func TestCountWorkerSessions_KnownWorkersRescuesUnprefixedNames(t *testing.T) {
	names := []string{"test", "legacy-fleet-01", "worker-ce-2mib"}
	known := func() (map[string]bool, error) {
		return map[string]bool{"legacy-fleet-01": true}, nil
	}

	if got := countWorkerSessions(names, known); got != 2 {
		t.Errorf("countWorkerSessions = %d, want 2 (worker-ce-2mib + legacy-fleet-01)", got)
	}
}

// An unreadable session DB falls back to prefix-only classification. It must
// not fail closed by counting everything again.
func TestCountWorkerSessions_ResolverErrorFallsBackToPrefix(t *testing.T) {
	names := []string{"test", "vroom-meta-o", "worker-ce-2mib"}
	known := func() (map[string]bool, error) {
		return nil, errors.New("dolt unavailable")
	}

	if got := countWorkerSessions(names, known); got != 1 {
		t.Errorf("countWorkerSessions = %d, want 1 (prefix-only fallback)", got)
	}
}

// The resolver is skipped entirely when every live session is prefix-matched,
// and consulted at most once otherwise.
func TestCountWorkerSessions_ResolverCalledLazilyAndOnce(t *testing.T) {
	calls := 0
	known := func() (map[string]bool, error) {
		calls++
		return nil, nil
	}

	if got := countWorkerSessions([]string{"worker-a", "worker-b"}, known); got != 2 {
		t.Fatalf("countWorkerSessions = %d, want 2", got)
	}
	if calls != 0 {
		t.Errorf("resolver called %d times for all-prefixed names, want 0", calls)
	}

	if got := countWorkerSessions([]string{"noise-a", "noise-b", "noise-c"}, known); got != 0 {
		t.Fatalf("countWorkerSessions = %d, want 0", got)
	}
	if calls != 1 {
		t.Errorf("resolver called %d times for 3 unprefixed names, want 1", calls)
	}
}

// The MaxWorkers gate must admit a spawn when the only sessions on the socket
// are non-workers — the exact live repro from ce-2mib.
func TestCheckMaxWorkers_TestFixtureDoesNotBlockDispatch(t *testing.T) {
	cfg := Config{MaxWorkers: 1}
	counter := stubWorkers{count: countWorkerSessions([]string{"test"}, nil)}

	gate := checkMaxWorkers(cfg, counter)
	if !gate.Passed {
		t.Errorf("max_workers gate refused with zero real workers: %s", gate.Message)
	}
}

// A resolver that hangs must not hang session admission: the lookup is bounded
// and a timeout degrades to prefix-only classification.
func TestBoundedKnownWorkers_TimeoutFallsBackToPrefix(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	counter := TmuxWorkerCounter{
		KnownWorkersTimeout: 20 * time.Millisecond,
		KnownWorkers: func() (map[string]bool, error) {
			<-release // never returns before the test ends
			return map[string]bool{"stuck": true}, nil
		},
	}

	start := time.Now()
	got := countWorkerSessions([]string{"stuck", "worker-ce-2mib"}, counter.boundedKnownWorkers())
	elapsed := time.Since(start)

	if got != 1 {
		t.Errorf("countWorkerSessions = %d, want 1 (prefix-only after timeout)", got)
	}
	if elapsed > time.Second {
		t.Errorf("lookup took %s — the timeout did not bound it", elapsed)
	}
}

// A resolver that answers in time is used normally, and the default timeout
// applies when none is configured.
func TestBoundedKnownWorkers_FastResolverIsUsed(t *testing.T) {
	counter := TmuxWorkerCounter{
		KnownWorkers: func() (map[string]bool, error) {
			return map[string]bool{"legacy-fleet-01": true}, nil
		},
	}

	if got := countWorkerSessions([]string{"legacy-fleet-01", "test"}, counter.boundedKnownWorkers()); got != 1 {
		t.Errorf("countWorkerSessions = %d, want 1", got)
	}
}

// A nil resolver stays nil through the bounding wrapper, so countWorkerSessions
// keeps its no-resolver fast path.
func TestBoundedKnownWorkers_NilStaysNil(t *testing.T) {
	if got := (TmuxWorkerCounter{}).boundedKnownWorkers(); got != nil {
		t.Error("boundedKnownWorkers wrapped a nil resolver; want nil")
	}
}

func TestSplitSessionNames(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{name: "empty output", out: "", want: nil},
		{name: "whitespace only", out: "\n  \n", want: nil},
		{
			name: "trailing newline",
			out:  "test\nworker-ce-2mib\n",
			want: []string{"test", "worker-ce-2mib"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSessionNames(tc.out)
			if len(got) != len(tc.want) {
				t.Fatalf("splitSessionNames(%q) = %v, want %v", tc.out, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitSessionNames(%q)[%d] = %q, want %q", tc.out, i, got[i], tc.want[i])
				}
			}
		})
	}
}
