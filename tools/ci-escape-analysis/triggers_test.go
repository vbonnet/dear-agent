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
			name:    "no name means nothing to key on",
			content: "on:\n  pull_request:\n",
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, preMerge, ok := parseWorkflow(test.content)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if name != test.wantName {
				t.Errorf("name = %q, want %q", name, test.wantName)
			}
			if preMerge != test.wantPreMerge {
				t.Errorf("preMerge = %v, want %v", preMerge, test.wantPreMerge)
			}
		})
	}
}

// Unknown workflows must default to pre-merge capable: misreporting a real
// escape as post-merge-only would silently drop it, which is the failure that
// actually costs something.
func TestUnknownWorkflowDefaultsToPreMergeCapable(t *testing.T) {
	if !(sweepOptions{}).PreMergeCapable("A Workflow That Does Not Exist") {
		t.Error("PreMergeCapable = false for an unknown workflow, want true")
	}
}
