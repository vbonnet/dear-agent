package main

import (
	"strings"
	"testing"
)

// run is exercised only on paths that fail before any gh execution; the gh
// invocation itself is covered by internal/safepr unit tests plus the manual
// dogfood (this feature's own PR is created through safe-pr).
func TestRun_Errors(t *testing.T) {
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	cases := []struct {
		name    string
		argv    []string
		wantErr string
	}{
		{"wayfinder missing value", []string{"create", "--wayfinder"}, "--wayfinder requires"},
		{"bad timeout", []string{"create", "--timeout", "soon"}, "invalid --timeout"},
		{"no session", []string{"create", "--title", "t"}, "no wayfinder session"},
		{"session dir unreadable", []string{"create", "--wayfinder", "/nonexistent-wf-dir",
			"--title", "t"}, "cannot read"},
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
	for _, argv := range [][]string{nil, {"--help"}, {"create", "-h"}} {
		if err := run(argv); err != nil {
			t.Errorf("run(%v) = %v, want nil (help text)", argv, err)
		}
	}
}

func TestParsePRURL(t *testing.T) {
	repo, num, ok := parsePRURL("https://github.com/vbonnet/dear-agent/pull/582")
	if !ok || repo != "vbonnet/dear-agent" || num != "582" {
		t.Errorf("parsePRURL = (%q, %q, %v), want (vbonnet/dear-agent, 582, true)", repo, num, ok)
	}
	if _, _, ok := parsePRURL("not a url"); ok {
		t.Error("parsePRURL should fail on a non-PR string")
	}
	if _, _, ok := parsePRURL("https://github.com/vbonnet/dear-agent/issues/582"); ok {
		t.Error("parsePRURL should not match an issues URL")
	}
}

func TestParseArgs_VerifyCI(t *testing.T) {
	p, err := parseArgs([]string{"create", "--verify-ci", "--title", "t"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if !p.verifyCI {
		t.Error("--verify-ci should set verifyCI true")
	}
	// --verify-ci must not leak into the gh args passed through to gh pr create.
	for _, a := range p.req.GhArgs {
		if a == "--verify-ci" {
			t.Error("--verify-ci leaked into gh args")
		}
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("cebb82eb05bea83"); got != "cebb82e" {
		t.Errorf("shortSHA = %q, want cebb82e", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("shortSHA = %q, want abc", got)
	}
}
