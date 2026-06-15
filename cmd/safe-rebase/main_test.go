package main

import (
	"strings"
	"testing"
)

func TestRun_Errors(t *testing.T) {
	cases := []struct {
		name    string
		argv    []string
		wantErr string
	}{
		{"missing -C value", []string{"-C"}, "-C requires"},
		{"missing --base value", []string{"--base"}, "--base requires"},
		{"missing --timeout value", []string{"--timeout"}, "--timeout requires"},
		{"bad timeout", []string{"--timeout", "soon"}, "invalid --timeout"},
		{"unknown flag", []string{"--bogus"}, "unknown flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.argv)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("run(%v) = %v, want substring %q", tc.argv, err, tc.wantErr)
			}
		})
	}
}

func TestRun_HelpIsNotAnError(t *testing.T) {
	for _, argv := range [][]string{{"--help"}, {"-h"}} {
		if err := run(argv); err != nil {
			t.Errorf("run(%v) = %v, want nil (help text)", argv, err)
		}
	}
}
