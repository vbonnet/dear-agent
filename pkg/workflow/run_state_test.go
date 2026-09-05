package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseRunState(t *testing.T) {
	canonical := []RunState{
		RunStatePending,
		RunStateRunning,
		RunStateAwaitingHITL,
		RunStateSucceeded,
		RunStateFailed,
		RunStateCancelled,
	}
	for _, want := range canonical {
		got, err := ParseRunState(string(want))
		if err != nil {
			t.Fatalf("ParseRunState(%q): %v", want, err)
		}
		if got != want {
			t.Errorf("ParseRunState(%q) = %q, want %q", want, got, want)
		}
		if !got.Valid() {
			t.Errorf("ParseRunState(%q) returned invalid state", want)
		}
	}

	for _, value := range []string{"", "unknown", " running ", "RUNNING"} {
		got, err := ParseRunState(value)
		if !errors.Is(err, ErrInvalidRunState) {
			t.Errorf("ParseRunState(%q) error = %v, want ErrInvalidRunState", value, err)
		}
		if got != "" {
			t.Errorf("ParseRunState(%q) = %q, want empty", value, got)
		}
		if err != nil && !strings.Contains(err.Error(), value) {
			t.Errorf("ParseRunState(%q) error %q omits rejected value", value, err)
		}
	}
}

func TestParseRunStateFilter(t *testing.T) {
	got, err := ParseRunStateFilter("")
	if err != nil || got != "" {
		t.Fatalf("ParseRunStateFilter(empty) = %q, %v; want empty, nil", got, err)
	}

	got, err = ParseRunStateFilter(string(RunStateRunning))
	if err != nil || got != RunStateRunning {
		t.Fatalf("ParseRunStateFilter(running) = %q, %v; want running, nil", got, err)
	}

	if _, err := ParseRunStateFilter("typo"); !errors.Is(err, ErrInvalidRunState) {
		t.Fatalf("ParseRunStateFilter(typo) error = %v, want ErrInvalidRunState", err)
	}
}

func TestListRejectsInvalidRunState(t *testing.T) {
	state := openTestState(t)
	db := state.DB()
	if err := state.Close(); err != nil {
		t.Fatalf("close test database: %v", err)
	}

	runs, err := List(context.Background(), db, ListOptions{State: RunState("typo")})
	if !errors.Is(err, ErrInvalidRunState) {
		t.Fatalf("List(invalid state) error = %v, want ErrInvalidRunState", err)
	}
	if runs != nil {
		t.Fatalf("List(invalid state) runs = %#v, want nil", runs)
	}
}

// A repeated state filter is ambiguous. Reading only the first value made
// validation order-dependent: a valid value first would mask an unknown one.
func TestParseRunStateFilterValues(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    RunState
		wantErr error
	}{
		{name: "absent", values: nil, want: ""},
		{name: "empty any-state", values: []string{""}, want: ""},
		{name: "single valid", values: []string{string(RunStateRunning)}, want: RunStateRunning},
		{name: "single unknown", values: []string{"typo"}, wantErr: ErrInvalidRunState},
		{name: "valid then unknown", values: []string{string(RunStateRunning), "typo"}, wantErr: ErrRepeatedRunState},
		{name: "unknown then valid", values: []string{"typo", string(RunStateRunning)}, wantErr: ErrRepeatedRunState},
		{name: "repeated valid", values: []string{string(RunStateRunning), string(RunStateFailed)}, wantErr: ErrRepeatedRunState},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRunStateFilterValues(tc.values)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ParseRunStateFilterValues(%q) error = %v, want %v", tc.values, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRunStateFilterValues(%q) error = %v", tc.values, err)
			}
			if got != tc.want {
				t.Errorf("ParseRunStateFilterValues(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}
