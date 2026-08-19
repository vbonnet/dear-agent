package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTouchedLines(t *testing.T) {
	tests := []struct {
		name  string
		diff  string
		want  map[string][]int
		empty []string
	}{
		{
			name: "pure addition hunk owns every added line",
			diff: "--- a/scripts/a.sh\n+++ b/scripts/a.sh\n@@ -12,0 +13,3 @@\n+one\n+two\n+three\n",
			want: map[string][]int{"scripts/a.sh": {13, 14, 15}},
		},
		{
			name: "single-line hunk header without a count means one line",
			diff: "--- a/scripts/a.sh\n+++ b/scripts/a.sh\n@@ -5 +5 @@\n-old\n+new\n",
			want: map[string][]int{"scripts/a.sh": {5}},
		},
		{
			name: "several files are tracked independently",
			diff: "--- a/x.sh\n+++ b/x.sh\n@@ -0,0 +1,1 @@\n+x\n" +
				"--- a/y.sh\n+++ b/y.sh\n@@ -0,0 +9,2 @@\n+y\n+y\n",
			want: map[string][]int{"x.sh": {1}, "y.sh": {9, 10}},
		},
		{
			name:  "a deletion claims no destination lines",
			diff:  "--- a/gone.sh\n+++ /dev/null\n@@ -1,4 +0,0 @@\n-a\n-b\n-c\n-d\n",
			empty: []string{"gone.sh"},
		},
		{
			name: "a zero-line destination range adds nothing",
			diff: "--- a/x.sh\n+++ b/x.sh\n@@ -4,2 +3,0 @@\n-a\n-b\n",
			want: map[string][]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			touched, err := parseTouchedLines(tt.diff)
			if err != nil {
				t.Fatalf("parseTouchedLines: %v", err)
			}
			for file, lines := range tt.want {
				for _, line := range lines {
					if !touched.Contains(file, line) {
						t.Errorf("expected %s:%d to be touched", file, line)
					}
				}
				if got := len(touched[file]); got != len(lines) {
					t.Errorf("%s: touched %d lines, want %d", file, got, len(lines))
				}
			}
			for _, file := range tt.empty {
				if len(touched[file]) != 0 {
					t.Errorf("%s: expected no touched lines, got %v", file, touched[file])
				}
			}
		})
	}
}

func TestParseHunkHeaderRejectsMalformedRanges(t *testing.T) {
	for _, header := range []string{"@@ -1,2 @@", "@@ -1,2 +x,3 @@", "@@ -1,2 +3,y @@"} {
		if _, _, err := parseHunkHeader(header); err == nil {
			t.Errorf("parseHunkHeader(%q) unexpectedly succeeded", header)
		}
	}
}

func TestParseFindingsRejectsUnusableDocuments(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "the legacy bare-array json format is rejected, not read as clean",
			raw:     `[{"file":"a.sh","line":1,"column":1,"level":"warning","code":2086,"message":"quote it"}]`,
			wantErr: "JSON1",
		},
		{name: "an empty document is rejected", raw: `{}`, wantErr: "comments"},
		{name: "a finding without a file is rejected", raw: `{"comments":[{"line":1,"level":"warning"}]}`, wantErr: "no file"},
		{
			name:    "a finding without a level is rejected",
			raw:     `{"comments":[{"file":"a.sh","line":1}]}`,
			wantErr: "no level",
		},
		{
			name:    "a finding on line zero is rejected",
			raw:     `{"comments":[{"file":"a.sh","line":0,"level":"warning"}]}`,
			wantErr: "non-positive line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFindings([]byte(tt.raw))
			if err == nil {
				t.Fatalf("parseFindings unexpectedly succeeded")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestSelectBlockingIgnoresUntouchedLinesAndLowSeverity(t *testing.T) {
	findings := []Finding{
		{File: "a.sh", Line: 10, Column: 3, Level: "warning", Code: 2086, Message: "quote it"},
		{File: "a.sh", Line: 99, Column: 1, Level: "error", Code: 1009, Message: "legacy debt"},
		{File: "a.sh", Line: 10, Column: 1, Level: "style", Code: 2006, Message: "use $()"},
		{File: "b.sh", Line: 4, Column: 1, Level: "info", Code: 1091, Message: "not followed"},
	}
	touched := TouchedLines{
		"a.sh": {10: true},
		"b.sh": {4: true},
	}

	warningThreshold, err := severityRank("warning")
	if err != nil {
		t.Fatalf("severityRank: %v", err)
	}
	blocking := selectBlocking(findings, touched, warningThreshold)
	if len(blocking) != 1 || blocking[0].Code != 2086 {
		t.Fatalf("expected only the touched warning to block, got %+v", blocking)
	}

	styleThreshold, err := severityRank("style")
	if err != nil {
		t.Fatalf("severityRank: %v", err)
	}
	blocking = selectBlocking(findings, touched, styleThreshold)
	if len(blocking) != 3 {
		t.Fatalf("expected every touched finding to block at style, got %+v", blocking)
	}
	// Line 99 carries the most severe level in the set and must still be
	// excluded: severity never overrides the changed-line scope.
	for _, f := range blocking {
		if f.Line == 99 {
			t.Fatalf("an untouched line blocked the gate: %+v", f)
		}
	}
}

func TestSeverityRankRejectsUnknownLevels(t *testing.T) {
	if _, err := severityRank("critical"); err == nil {
		t.Fatal("severityRank unexpectedly accepted an unknown level")
	}
}

func TestRunEndToEnd(t *testing.T) {
	tests := []struct {
		name        string
		diff        string
		findings    string
		minSeverity string
		wantErr     string
		wantOutput  string
	}{
		{
			name:        "a warning introduced by the change fails the gate",
			diff:        "--- a/a.sh\n+++ b/a.sh\n@@ -0,0 +1,1 @@\n+echo $x\n",
			findings:    `{"comments":[{"file":"a.sh","line":1,"column":6,"level":"warning","code":2086,"message":"Double quote"}]}`,
			minSeverity: "warning",
			wantErr:     "1 ShellCheck finding(s)",
			wantOutput:  "a.sh:1:6: warning: Double quote [SC2086]",
		},
		{
			name:        "pre-existing debt on an untouched line passes",
			diff:        "--- a/a.sh\n+++ b/a.sh\n@@ -0,0 +1,1 @@\n+echo \"$x\"\n",
			findings:    `{"comments":[{"file":"a.sh","line":42,"column":6,"level":"error","code":1009,"message":"Legacy"}]}`,
			minSeverity: "warning",
			wantOutput:  "No ShellCheck findings",
		},
		{
			name:        "a style finding does not fail a warning-level gate",
			diff:        "--- a/a.sh\n+++ b/a.sh\n@@ -0,0 +1,1 @@\n+echo `date`\n",
			findings:    `{"comments":[{"file":"a.sh","line":1,"column":6,"level":"style","code":2006,"message":"Use $()"}]}`,
			minSeverity: "warning",
			wantOutput:  "No ShellCheck findings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			diffPath := filepath.Join(dir, "changed.patch")
			findingsPath := filepath.Join(dir, "findings.json")
			if err := os.WriteFile(diffPath, []byte(tt.diff), 0o600); err != nil {
				t.Fatalf("write diff: %v", err)
			}
			if err := os.WriteFile(findingsPath, []byte(tt.findings), 0o600); err != nil {
				t.Fatalf("write findings: %v", err)
			}

			var out bytes.Buffer
			err := run([]string{
				"--diff", diffPath,
				"--findings", findingsPath,
				"--min-severity", tt.minSeverity,
			}, &out)

			if tt.wantErr == "" && err != nil {
				t.Fatalf("run failed: %v (output %q)", err, out.String())
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("run unexpectedly succeeded: %q", out.String())
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
			}
			if !strings.Contains(out.String(), tt.wantOutput) {
				t.Fatalf("output %q does not contain %q", out.String(), tt.wantOutput)
			}
		})
	}
}

func TestRunRequiresBothInputs(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--diff", "only.patch"}, &out); err == nil {
		t.Fatal("run unexpectedly succeeded without --findings")
	}
}
