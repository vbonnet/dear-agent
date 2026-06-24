package main

import (
	"os/exec"
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

// initGitRepo creates a minimal git repo in dir with the given origin remote URL.
func initGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", dir},
		{"-C", dir, "remote", "add", "origin", remoteURL},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestValidateRemoteURL(t *testing.T) {
	cases := []struct {
		name      string
		remoteURL string
		wantErr   string
	}{
		{
			name:      "valid vbonnet remote",
			remoteURL: "https://github.com/vbonnet/dear-agent",
		},
		{
			name:      "wrong org",
			remoteURL: "https://github.com/dear-labs/dear-agent",
			wantErr:   "does not look like a vbonnet GitHub remote",
		},
		{
			name:      "non-github remote",
			remoteURL: "https://gitlab.com/vbonnet/dear-agent",
			wantErr:   "does not look like a vbonnet GitHub remote",
		},
		{
			name:      "ssh remote",
			remoteURL: "git@github.com:vbonnet/dear-agent.git",
			wantErr:   "does not look like a vbonnet GitHub remote",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initGitRepo(t, dir, tc.remoteURL)
			err := validateRemoteURL(dir)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("validateRemoteURL(%q) = %v, want nil", tc.remoteURL, err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("validateRemoteURL(%q) = %v, want substring %q", tc.remoteURL, err, tc.wantErr)
				}
			}
		})
	}
}

func TestValidateRemoteURL_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	err := validateRemoteURL(dir)
	if err == nil || !strings.Contains(err.Error(), "could not resolve origin remote URL") {
		t.Errorf("validateRemoteURL(non-git dir) = %v, want 'could not resolve origin remote URL'", err)
	}
}
