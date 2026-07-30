package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
	"github.com/vbonnet/dear-agent/pkg/override"
)

func TestOnlyAdmissionBrakeRefused(t *testing.T) {
	tests := []struct {
		name  string
		gates []circuitbreaker.GateResult
		want  bool
	}{
		{
			name: "engaged brake is sole refusal",
			gates: []circuitbreaker.GateResult{
				{Gate: "disk", Passed: true},
				{Gate: "admission_brake", RequiresOverride: true},
			},
			want: true,
		},
		{
			name: "resource gate still refuses",
			gates: []circuitbreaker.GateResult{
				{Gate: "disk", Passed: false},
				{Gate: "admission_brake", RequiresOverride: true},
			},
		},
		{
			name: "unreadable brake is not overrideable",
			gates: []circuitbreaker.GateResult{
				{Gate: "admission_brake", Passed: false},
			},
		},
		{
			name: "no brake",
			gates: []circuitbreaker.GateResult{
				{Gate: "disk", Passed: true},
				{Gate: "admission_brake", Passed: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := circuitbreaker.CheckResult{Gates: tt.gates}
			if got := onlyAdmissionBrakeRefused(result); got != tt.want {
				t.Fatalf("onlyAdmissionBrakeRefused() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyAdmissionBrakeAuthorizationPreservesOtherRefusals(t *testing.T) {
	result := circuitbreaker.CheckResult{
		Gates: []circuitbreaker.GateResult{
			{Gate: "spawn_stagger", Passed: false, Message: "concurrent spawn"},
			{Gate: "admission_brake", RequiresOverride: true, Message: "brake engaged"},
		},
	}
	got := applyAdmissionBrakeAuthorization(result, "operator verified host recovery")
	if got.Allowed {
		t.Fatal("brake authorization erased a concurrent non-brake refusal")
	}
	if got.Gates[1].Passed != true || got.Gates[1].RequiresOverride {
		t.Fatalf("brake gate was not marked authorized: %+v", got.Gates[1])
	}
	if got.Gates[0].Passed {
		t.Fatalf("non-brake gate changed: %+v", got.Gates[0])
	}
}

func TestApplyAdmissionBrakeAuthorizationAllowsSoleBrakeRefusal(t *testing.T) {
	result := circuitbreaker.CheckResult{
		Gates: []circuitbreaker.GateResult{
			{Gate: "disk", Passed: true},
			{Gate: "admission_brake", RequiresOverride: true, Message: "brake engaged"},
		},
	}
	got := applyAdmissionBrakeAuthorization(result, "operator verified host recovery")
	if !got.Allowed {
		t.Fatalf("sole committed brake authorization remained refused: %+v", got)
	}
	if !got.Gates[1].Passed || got.Gates[1].RequiresOverride {
		t.Fatalf("brake gate was not marked authorized: %+v", got.Gates[1])
	}
}

func TestFinalizeAdmissionBrakeOverrideCommitsAfterFinalLiveCheck(t *testing.T) {
	initial := circuitbreaker.CheckResult{Gates: []circuitbreaker.GateResult{{
		Gate: "admission_brake", RequiresOverride: true,
	}}}
	var events []string
	brakeReservation := &override.Reservation{}
	codexHookReservation := &override.Reservation{}
	result, err := finalizeAdmissionBrakeOverride(
		initial,
		"operator verified host recovery",
		"worker-ce-6xfu",
		func() circuitbreaker.CheckResult {
			events = append(events, "final-check")
			return initial
		},
		func(reason, session string) (*override.Reservation, error) {
			events = append(events, "reserve")
			if reason == "" || session != "worker-ce-6xfu" {
				t.Fatalf("reservation attribution = (%q, %q)", reason, session)
			}
			return brakeReservation, nil
		},
		func(reservations ...*override.Reservation) error {
			events = append(events, "commit")
			if len(reservations) != 2 ||
				reservations[0] != brakeReservation ||
				reservations[1] != codexHookReservation {
				t.Fatalf("combined reservations = %v, want brake then Codex hook trust", reservations)
			}
			return nil
		},
		codexHookReservation,
	)
	if err != nil {
		t.Fatalf("finalizeAdmissionBrakeOverride() error = %v", err)
	}
	if got, want := strings.Join(events, ","), "reserve,final-check,commit"; got != want {
		t.Fatalf("authorization events = %q, want %q", got, want)
	}
	if !result.Allowed {
		t.Fatalf("committed sole-brake result remained refused: %+v", result)
	}
}

func TestFinalizeAdmissionBrakeOverrideAbandonsReservationOnConcurrentRefusal(t *testing.T) {
	initial := circuitbreaker.CheckResult{Gates: []circuitbreaker.GateResult{{
		Gate: "admission_brake", RequiresOverride: true,
	}}}
	committed := false
	result, err := finalizeAdmissionBrakeOverride(
		initial,
		"operator verified host recovery",
		"worker-ce-6xfu",
		func() circuitbreaker.CheckResult {
			return circuitbreaker.CheckResult{Gates: []circuitbreaker.GateResult{
				{Gate: "spawn_stagger", Passed: false},
				{Gate: "admission_brake", RequiresOverride: true},
			}}
		},
		func(string, string) (*override.Reservation, error) {
			return &override.Reservation{}, nil
		},
		func(...*override.Reservation) error {
			committed = true
			return nil
		},
		&override.Reservation{},
	)
	if err != nil {
		t.Fatalf("finalizeAdmissionBrakeOverride() error = %v", err)
	}
	if committed {
		t.Fatal("concurrent refusal consumed the reserved ledger use")
	}
	if result.Allowed {
		t.Fatalf("concurrent refusal was erased: %+v", result)
	}
}

func TestFinalizeAdmissionBrakeOverridePropagatesReservationFailure(t *testing.T) {
	initial := circuitbreaker.CheckResult{Gates: []circuitbreaker.GateResult{{
		Gate: "admission_brake", RequiresOverride: true,
	}}}
	wantErr := errors.New("grant expired")
	finalChecked := false
	_, err := finalizeAdmissionBrakeOverride(
		initial,
		"operator verified host recovery",
		"worker-ce-6xfu",
		func() circuitbreaker.CheckResult {
			finalChecked = true
			return initial
		},
		func(string, string) (*override.Reservation, error) {
			return nil, wantErr
		},
		func(...*override.Reservation) error {
			t.Fatal("commit ran without a valid brake reservation")
			return nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if finalChecked {
		t.Fatal("final check ran without a valid authorization reservation")
	}
}
