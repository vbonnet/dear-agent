package main

import (
	"strings"
	"testing"
)

func TestRunArgErrors(t *testing.T) {
	cases := []struct {
		name    string
		argv    []string
		wantErr string
	}{
		{"unknown arg", []string{"--nope"}, "unknown argument"},
		{"timeout needs value", []string{"--timeout"}, "requires a value"},
		{"bad timeout", []string{"--timeout", "notaduration"}, "invalid --timeout"},
		{"repo needs value", []string{"--repo"}, "requires a value"},
		// Emergency skips session resolution; Validate then catches the missing
		// reason before any git runs.
		{"emergency no reason", []string{"--repo", "/r", "--branch", "b", "--emergency"}, "requires --reason"},
		// Missing branch is caught by Validate before git, under emergency.
		{"missing branch", []string{"--repo", "/r", "--emergency", "--reason", "x"}, "--branch is required"},
		// No session and no emergency: ResolveSessionDir fails first.
		{"no session", []string{"--repo", "/r", "--branch", "b"}, "no wayfinder session"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WAYFINDER_PROJECT_DIR", "")
			err := run(tc.argv)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("run(%v) = %v, want error containing %q", tc.argv, err, tc.wantErr)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Fatalf("--help should succeed, got %v", err)
	}
}
