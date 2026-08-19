package main

import "testing"

func TestParseWorkflow(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantName     string
		wantPreMerge bool
		wantOK       bool
	}{
		{
			name: "schedule-only workflow is not pre-merge capable",
			content: `name: Tofu Drift Detection

on:
  schedule:
    - cron: "17 8 * * *"
  workflow_dispatch: {}

jobs:
  drift:
    runs-on: ubuntu-latest
`,
			wantName: "Tofu Drift Detection", wantPreMerge: false, wantOK: true,
		},
		{
			name: "pull_request in the trigger block is pre-merge capable",
			content: `name: CI

on:
  push:
    branches: [main]
  pull_request:
  schedule:
    - cron: "0 6 * * *"
`,
			wantName: "CI", wantPreMerge: true, wantOK: true,
		},
		{
			name: "inline trigger form",
			content: `name: Dependabot Auto-Merge

on: pull_request
`,
			wantName: "Dependabot Auto-Merge", wantPreMerge: true, wantOK: true,
		},
		{
			name: "pull_request_target counts",
			content: `name: AI Code Review (5-dimension)

on:
  pull_request_target:
    types: [opened]
`,
			wantName: "AI Code Review (5-dimension)", wantPreMerge: true, wantOK: true,
		},
		{
			// A job named "pull_request_something" under jobs: must not be
			// mistaken for a trigger — the trigger block ended at `jobs:`.
			name: "pull_request outside the trigger block does not count",
			content: `name: Security Audit

on:
  schedule:
    - cron: "30 14 * * *"

jobs:
  pull_request_audit:
    runs-on: ubuntu-latest
`,
			wantName: "Security Audit", wantPreMerge: false, wantOK: true,
		},
		{
			name: "a job step mentioning pull_request does not count",
			content: `name: Merge Audit

on:
  push:
    branches: [main]

jobs:
  audit:
    steps:
      - run: gh api repos/x/y/pulls --jq '.[] | .pull_request'
`,
			wantName: "Merge Audit", wantPreMerge: false, wantOK: true,
		},
		{
			// pull_request_review and pull_request_review_comment fire on
			// review activity, never on the pull request itself. A prefix
			// match folds them in and marks this workflow pre-merge capable,
			// after which a failed review-driven run on main is reported as a
			// path-filter gap.
			name: "review triggers alone are not pre-merge capable",
			content: `name: Claude Code

on:
  issue_comment:
    types: [created]
  pull_request_review_comment:
    types: [created]
  pull_request_review:
    types: [submitted]
`,
			wantName: "Claude Code", wantPreMerge: false, wantOK: true,
		},
		{
			// A workflow_dispatch input whose name merely starts with
			// "pull_request" is not a trigger.
			name: "a dispatch input named after a pull request is not a trigger",
			content: `name: Backport

on:
  workflow_dispatch:
    inputs:
      pull_request_number:
        required: true
`,
			wantName: "Backport", wantPreMerge: false, wantOK: true,
		},
		{
			name:    "no name means nothing to key on",
			content: "on:\n  pull_request:\n",
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, info, ok := parseWorkflow(test.content)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if name != test.wantName {
				t.Errorf("name = %q, want %q", name, test.wantName)
			}
			if info.PreMerge != test.wantPreMerge {
				t.Errorf("preMerge = %v, want %v", info.PreMerge, test.wantPreMerge)
			}
		})
	}
}

// A pull-request-capable workflow can still contain jobs that a pull request
// never runs. Judging the containing workflow instead of the failing job is how
// a scheduled sweep inside CI gets reported as an escape with path-filter
// advice attached.
func TestParseWorkflowJobLevelEventGuards(t *testing.T) {
	content := `name: CI

on:
  pull_request:
  schedule:
    - cron: "0 6 * * *"

jobs:
  build:
    name: Build & Test
    runs-on: ubuntu-latest
  tagged-sweep:
    name: AGM Tagged Sweep
    if: github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'
    runs-on: ubuntu-latest
  nightly-or-pr:
    name: Bash line-limit scan
    if: github.event_name == 'pull_request' || github.event_name == 'schedule'
    runs-on: ubuntu-latest
`

	name, info, ok := parseWorkflow(content)
	if !ok || name != "CI" {
		t.Fatalf("parseWorkflow() = %q, %v; want \"CI\", true", name, ok)
	}
	if !info.PreMerge {
		t.Fatal("workflow should be pre-merge capable")
	}

	if capable, seen := info.Jobs["AGM Tagged Sweep"]; !seen || capable {
		t.Errorf("AGM Tagged Sweep capable = %v (seen %v), want false", capable, seen)
	}
	if capable, seen := info.Jobs["Bash line-limit scan"]; !seen || !capable {
		t.Errorf("Bash line-limit scan capable = %v (seen %v), want true", capable, seen)
	}
	if _, seen := info.Jobs["Build & Test"]; seen {
		t.Error("an unguarded job should record no job-level restriction")
	}
}

// Unknown workflows must default to pre-merge capable: misreporting a real
// escape as post-merge-only would silently drop it, which is the failure that
// actually costs something.
func TestUnknownWorkflowDefaultsToPreMergeCapable(t *testing.T) {
	if !(sweepOptions{}).PreMergeCapable("A Workflow That Does Not Exist", "Some Check") {
		t.Error("PreMergeCapable = false for an unknown workflow, want true")
	}
}

func TestIfPermitsPullRequest(t *testing.T) {
	tests := []struct {
		condition string
		want      bool
	}{
		{"github.event_name == 'schedule'", false},
		{"github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'", false},
		{"github.event_name == 'pull_request'", true},
		{"github.event_name != 'schedule'", true},
		{"always()", true},
		{"needs.detect.outputs.changed == 'true'", true},
	}
	for _, test := range tests {
		if got := ifPermitsPullRequest(test.condition); got != test.want {
			t.Errorf("ifPermitsPullRequest(%q) = %v, want %v", test.condition, got, test.want)
		}
	}
}

// A substring test reads `!= 'pull_request'` as permission, exactly inverting
// the condition. sbom-scan.yml guards two jobs that way.
func TestIfPermitsPullRequestParsesTheOperator(t *testing.T) {
	tests := []struct {
		condition string
		want      bool
	}{
		{"github.event_name != 'pull_request'", false},
		{"github.event_name != \"pull_request\"", false},
		{"github.event_name != 'pull_request' && github.event_name != 'schedule'", false},
		{"github.event_name == 'pull_request'", true},
		{"github.event_name == 'schedule' || github.event_name == 'pull_request'", true},
		{"github.event_name == 'schedule'", false},
		{"github.event_name != 'schedule'", true},
		{"github.event_name == 'pull_request_target'", true},
		{"github.event_name != 'pull_request_target'", false},
		{"contains(github.event.head_commit.message, 'pull_request')", true},
	}
	for _, test := range tests {
		if got := ifPermitsPullRequest(test.condition); got != test.want {
			t.Errorf("ifPermitsPullRequest(%q) = %v, want %v", test.condition, got, test.want)
		}
	}
}

// A workflow with no trigger reaching main cannot be red on main today,
// whatever its run history says. Getting this wrong in either direction is
// costly: too strict and the sweep sees nothing at all, too loose and a
// workflow that stopped running on main re-files an incident forever.
func TestParseWorkflowDetectsMainTriggers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "block-form push trigger",
			content: "name: CI\n\non:\n  push:\n    branches: [main]\n  pull_request:\n",
			want:    true,
		},
		{
			name:    "schedule alone reaches main",
			content: "name: Drift\n\non:\n  schedule:\n    - cron: \"0 6 * * *\"\n",
			want:    true,
		},
		{
			name:    "pull_request alone does not",
			content: "name: Routing Enforcement\n\non:\n  pull_request:\n    types: [opened]\n",
			want:    false,
		},
		{
			name:    "interaction events alone do not",
			content: "name: Claude Code\n\non:\n  issue_comment:\n    types: [created]\n  pull_request_review:\n    types: [submitted]\n",
			want:    false,
		},
		{
			name:    "inline form",
			content: "name: Inline\n\non: [push, pull_request]\n",
			want:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, info, ok := parseWorkflow(test.content)
			if !ok {
				t.Fatal("parseWorkflow() returned ok=false")
			}
			if info.MainTriggered != test.want {
				t.Errorf("MainTriggered = %v, want %v", info.MainTriggered, test.want)
			}
		})
	}
}
