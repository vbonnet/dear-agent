package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestBeadsRoadmap_PendingProposals(t *testing.T) {
	t.Run("maps bd ready json to proposals", func(t *testing.T) {
		items := []bdReadyItem{
			{ID: "ce-abc", Title: "Fix the thing", Description: "needs fixing", Priority: 0, IssueType: "bug"},
			{ID: "ce-xyz", Title: "Add feature", Description: "new capability", Priority: 1, IssueType: "feature"},
		}
		payload, _ := json.Marshal(items)

		r := &BeadsRoadmap{
			DB: "/fake/.beads",
			run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return payload, nil
			},
		}

		proposals, err := r.PendingProposals(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(proposals) != 2 {
			t.Fatalf("want 2 proposals, got %d", len(proposals))
		}
		if proposals[0].ID != "ce-abc" || proposals[0].Title != "Fix the thing" {
			t.Errorf("unexpected proposal[0]: %+v", proposals[0])
		}
		if proposals[0].ScopeHint != ScopeFix {
			t.Errorf("want ScopeFix for bug, got %q", proposals[0].ScopeHint)
		}
		if proposals[1].ScopeHint != ScopeFeature {
			t.Errorf("want ScopeFeature for feature, got %q", proposals[1].ScopeHint)
		}
	})

	t.Run("truncates long descriptions", func(t *testing.T) {
		long := string(make([]byte, maxDescriptionBytes+100))
		for i := range long {
			long = long[:i] + "x" + long[i+1:]
		}
		items := []bdReadyItem{{ID: "ce-1", Title: "t", Description: long}}
		payload, _ := json.Marshal(items)

		r := &BeadsRoadmap{
			DB: "/fake/.beads",
			run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return payload, nil
			},
		}

		proposals, err := r.PendingProposals(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reason := proposals[0].Reason
		if len(reason) > maxDescriptionBytes+5 {
			t.Errorf("description not truncated: len=%d", len(reason))
		}
	})

	t.Run("empty description gets placeholder", func(t *testing.T) {
		items := []bdReadyItem{{ID: "ce-1", Title: "t", Description: ""}}
		payload, _ := json.Marshal(items)

		r := &BeadsRoadmap{
			DB: "/fake/.beads",
			run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return payload, nil
			},
		}

		proposals, err := r.PendingProposals(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if proposals[0].Reason != "(no description)" {
			t.Errorf("unexpected reason: %q", proposals[0].Reason)
		}
	})

	t.Run("propagates bd error", func(t *testing.T) {
		r := &BeadsRoadmap{
			DB: "/fake/.beads",
			run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return nil, errors.New("bd: not found")
			},
		}

		_, err := r.PendingProposals(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBeadsRoadmap_Accept(t *testing.T) {
	var capturedArgs []string
	r := &BeadsRoadmap{
		DB: "/fake/.beads",
		run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("ok"), nil
		},
	}

	if err := r.Accept(context.Background(), "ce-abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Contains(capturedArgs, "ce-abc") {
		t.Errorf("bead ID not in args: %v", capturedArgs)
	}
}

func TestBeadsRoadmap_Accept_EnqueuesTask(t *testing.T) {
	q := NewInMemoryQueue()

	r := &BeadsRoadmap{
		DB:    "/fake/.beads",
		Queue: q,
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("ok"), nil
		},
	}

	if err := r.Accept(context.Background(), "ce-xyz"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tasks, err := q.Pending(context.Background())
	if err != nil {
		t.Fatalf("queue error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "ce-xyz" {
		t.Errorf("task not enqueued: %v", tasks)
	}
}

func TestBeadsRoadmap_Reject(t *testing.T) {
	var capturedArgs []string
	r := &BeadsRoadmap{
		DB: "/fake/.beads",
		run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("ok"), nil
		},
	}

	if err := r.Reject(context.Background(), "ce-abc", "duplicate"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Contains(capturedArgs, "ce-abc") {
		t.Errorf("bead ID not in args: %v", capturedArgs)
	}
}

func TestScopeFromIssueType(t *testing.T) {
	cases := []struct {
		in   string
		want ScopeHint
	}{
		{"bug", ScopeFix},
		{"defect", ScopeFix},
		{"feature", ScopeFeature},
		{"enhancement", ScopeFeature},
		{"research", ScopeResearch},
		{"spike", ScopeResearch},
		{"refactor", ScopeRefactor},
		{"infra", ScopeInfra},
		{"chore", ScopeInfra},
		{"ci", ScopeInfra},
		{"task", ScopeUnspecified},
		{"", ScopeUnspecified},
	}
	for _, c := range cases {
		got := scopeFromIssueType(c.in)
		if got != c.want {
			t.Errorf("scopeFromIssueType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
