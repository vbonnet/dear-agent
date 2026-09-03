package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/agenticreview"
	"github.com/vbonnet/dear-agent/internal/mergeloop"
)

// The repository's own policy has to load through the merge loop's loader, or
// the loop and the required check would be reading different rules.
func TestLoadReviewPolicyReadsRepositoryConfig(t *testing.T) {
	cfg, err := loadReviewPolicy(filepath.Join("..", "..", agenticreview.DefaultConfigPath), false)
	if err != nil {
		t.Fatalf("loadReviewPolicy: %v", err)
	}
	if cfg == nil {
		t.Fatal("repository policy loaded as nil, so the gate would be silently off")
	}
	if cfg.Quorum != 2 || len(cfg.Families) != 3 {
		t.Fatalf("policy = quorum %d over %d families, want 2 of 3", cfg.Quorum, len(cfg.Families))
	}
}

// The loop is pointed at repositories that have not adopted the gate, so an
// absent default path leaves it off rather than failing every tick.
func TestLoadReviewPolicyTreatsAbsentDefaultAsDisabled(t *testing.T) {
	cfg, err := loadReviewPolicy(filepath.Join(t.TempDir(), "agentic-review.yml"), false)
	if err != nil {
		t.Fatalf("loadReviewPolicy: %v", err)
	}
	if cfg != nil {
		t.Fatal("an absent default policy enabled the gate")
	}
}

// An explicitly requested policy that is missing is an operator error, not a
// reason to merge without review.
func TestLoadReviewPolicyFailsOnMissingExplicitPath(t *testing.T) {
	if _, err := loadReviewPolicy(filepath.Join(t.TempDir(), "absent.yml"), true); err == nil {
		t.Fatal("an explicitly named missing policy was accepted")
	}
}

// A policy that exists but cannot be parsed must never become no policy. That
// is the failure where a gate looks installed and enforces nothing.
func TestLoadReviewPolicyFailsOnUnparseableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentic-review.yml")
	if err := os.WriteFile(path, []byte("families: [claude]\nquorum: 9\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := loadReviewPolicy(path, false); err == nil {
		t.Fatal("an unsatisfiable policy loaded as valid")
	}
}

// A failed clock read leaves the pull request without review timing, which the
// classifier treats as pending. A GitHub read that failed is not evidence that
// a reviewer approved.
func TestAttachReviewClockFailsClosed(t *testing.T) {
	lister := &ghLister{
		now: func() time.Time { return time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC) },
		reviewClock: func(context.Context, string, *mergeloop.PR, time.Time) error {
			return errors.New("github unreachable")
		},
	}
	pr := mergeloop.PR{
		Number:         7,
		ObservedAt:     time.Now(),
		ReadyAt:        time.Now(),
		LabelAppliedAt: map[string]time.Time{"agentic-review:claude:started": time.Now()},
	}

	lister.attachReviewClock(context.Background(), "o/r", &pr)

	if !pr.ObservedAt.IsZero() || !pr.ReadyAt.IsZero() || pr.LabelAppliedAt != nil {
		t.Fatalf("stale review timing survived a failed clock read: %+v", pr)
	}

	policy := mergeloop.NewPolicy()
	cfg, err := loadReviewPolicy(filepath.Join("..", "..", agenticreview.DefaultConfigPath), false)
	if err != nil {
		t.Fatalf("loadReviewPolicy: %v", err)
	}
	policy.AgenticReview = cfg
	pr.Mergeable, pr.MergeStateStatus = "MERGEABLE", "CLEAN"
	if got := policy.Classify(pr, 0, false); got.State != mergeloop.StateCIPending {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, mergeloop.StateCIPending)
	}
}

// With the gate unconfigured the loop makes no extra GitHub calls at all, so
// adopting it is the only thing that adds per-pull-request cost.
func TestAttachReviewClockIsInertWhenGateIsOff(t *testing.T) {
	lister := &ghLister{}
	pr := mergeloop.PR{Number: 7}

	lister.attachReviewClock(context.Background(), "o/r", &pr)

	if !pr.ObservedAt.IsZero() {
		t.Fatal("the review clock ran with the gate unconfigured")
	}
}

func TestAttachReviewClockRecordsObservationTime(t *testing.T) {
	at := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	lister := &ghLister{
		now: func() time.Time { return at },
		reviewClock: func(_ context.Context, _ string, pr *mergeloop.PR, now time.Time) error {
			pr.ObservedAt = now
			pr.ReadyAt = now.Add(-time.Hour)
			return nil
		},
	}
	pr := mergeloop.PR{Number: 7}

	lister.attachReviewClock(context.Background(), "o/r", &pr)

	if !pr.ObservedAt.Equal(at) {
		t.Fatalf("ObservedAt = %s, want %s", pr.ObservedAt, at)
	}
}
