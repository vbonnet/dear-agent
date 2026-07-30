package main

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
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
