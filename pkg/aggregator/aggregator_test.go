package aggregator

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type stubCollector struct {
	name    string
	kind    Kind
	signals []Signal
	err     error
}

func (s *stubCollector) Name() string                                  { return s.name }
func (s *stubCollector) Kind() Kind                                    { return s.kind }
func (s *stubCollector) Collect(_ context.Context) ([]Signal, error)   { return s.signals, s.err }

func TestAggregatorRunHappyPath(t *testing.T) {
	t.Parallel()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	at := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	id := 0
	idGen := func() string {
		id++
		return "id" + strconv.Itoa(id)
	}

	a := Aggregator{
		Store: store,
		Collectors: []Collector{
			&stubCollector{
				name: "test.lint", kind: KindLintTrend,
				signals: []Signal{
					{Subject: "a.go", Value: 3},
					{Subject: "b.go", Value: 5},
				},
			},
			&stubCollector{
				name: "test.git", kind: KindGitActivity,
				signals: []Signal{{Subject: "/repo", Value: 12}},
			},
		},
		Now:   func() time.Time { return at },
		IDGen: idGen,
	}
	report, err := a.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Collected["test.lint"] != 2 {
		t.Errorf("test.lint collected = %d, want 2", report.Collected["test.lint"])
	}
	if report.Collected["test.git"] != 1 {
		t.Errorf("test.git collected = %d, want 1", report.Collected["test.git"])
	}
	if len(report.Errors) != 0 {
		t.Errorf("Errors should be empty, got %v", report.Errors)
	}

	got, err := store.Recent(context.Background(), KindLintTrend, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("persisted lint signals = %d, want 2", len(got))
	}
	for _, s := range got {
		if !s.CollectedAt.Equal(at) {
			t.Errorf("CollectedAt = %v, want %v", s.CollectedAt, at)
		}
	}
}

func TestAggregatorRunPartialFailure(t *testing.T) {
	t.Parallel()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	a := Aggregator{
		Store: store,
		Collectors: []Collector{
			&stubCollector{name: "fail", kind: KindLintTrend, err: errors.New("boom")},
			&stubCollector{
				name: "ok", kind: KindGitActivity,
				signals: []Signal{{Subject: "/repo", Value: 1}},
			},
		},
		Now: func() time.Time { return time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC) },
	}
	report, err := a.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Errors["fail"] == nil {
		t.Error("expected error for fail collector")
	}
	if report.ErrorMsgs["fail"] == "" {
		t.Error("expected error message for fail collector")
	}
	if report.Collected["ok"] != 1 {
		t.Errorf("ok collector should still produce signals, got %d",
			report.Collected["ok"])
	}
}

func TestAggregatorRunNilStore(t *testing.T) {
	t.Parallel()
	a := Aggregator{}
	if _, err := a.Run(context.Background()); err == nil {
		t.Error("Run with nil store should fail")
	}
}

func TestStampSignalsDropsKindMismatch(t *testing.T) {
	t.Parallel()
	at := time.Now()
	in := []Signal{
		{Kind: KindLintTrend, Subject: "ok", Value: 1},
		{Kind: KindGitActivity, Subject: "wrong", Value: 1},
		{Subject: "filled-in", Value: 1},
	}
	out := stampSignals(in, KindLintTrend, at, func() string { return "id" })
	if len(out) != 2 {
		t.Fatalf("stamped %d signals, want 2 (mismatch dropped)", len(out))
	}
	for _, s := range out {
		if s.Kind != KindLintTrend {
			t.Errorf("Kind = %s, want lint_trend", s.Kind)
		}
		if s.ID != "id" {
			t.Errorf("ID = %q, want id (default-filled)", s.ID)
		}
		if !s.CollectedAt.Equal(at) {
			t.Errorf("CollectedAt = %v, want %v", s.CollectedAt, at)
		}
	}
}
