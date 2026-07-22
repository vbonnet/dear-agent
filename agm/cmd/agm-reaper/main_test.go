package main

import (
	"strings"
	"testing"
)

// TestBuildSentinel verifies the agm-reaper binary compiles.
func TestBuildSentinel(t *testing.T) {}

func TestValidateRevision(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		wantErr  string
	}{
		{name: "same revision", expected: "0123456789ab", actual: "0123456789ab"},
		{name: "full and short revision", expected: "0123456789abcdef", actual: "0123456789ab"},
		{name: "dirty suffix normalized", expected: "0123456789ab-dirty", actual: "0123456789ab"},
		{name: "mismatch", expected: "0123456789ab", actual: "fedcba987654", wantErr: "does not match"},
		{name: "missing expected", expected: "unknown", actual: "0123456789ab", wantErr: "expected AGM revision is unavailable"},
		{name: "missing actual", expected: "0123456789ab", actual: "unknown", wantErr: "no embedded VCS revision"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRevision(tc.expected, tc.actual)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRevision() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateRevision() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}
