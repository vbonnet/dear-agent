package ops

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

func TestParseSLOTable(t *testing.T) {
	content := `# SPEC: Test

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Resume ready wait | 5s max | some source |
| Bloat threshold | 100MB file size | another source |
| Session scan limit | 1000 per pass | yet another |

## Dependencies
`
	rows := parseSLOTable(content)
	if len(rows) != 3 {
		t.Fatalf("expected 3 SLO rows, got %d", len(rows))
	}

	tests := []struct {
		idx    int
		metric string
		target string
	}{
		{0, "Resume ready wait", "5s max"},
		{1, "Bloat threshold", "100MB file size"},
		{2, "Session scan limit", "1000 per pass"},
	}

	for _, tt := range tests {
		if rows[tt.idx].Metric != tt.metric {
			t.Errorf("row %d: metric = %q, want %q", tt.idx, rows[tt.idx].Metric, tt.metric)
		}
		if rows[tt.idx].Target != tt.target {
			t.Errorf("row %d: target = %q, want %q", tt.idx, rows[tt.idx].Target, tt.target)
		}
	}
}

func TestParseSLOTable_Empty(t *testing.T) {
	content := `# SPEC: Test

## Purpose
No SLOs here.
`
	rows := parseSLOTable(content)
	if len(rows) != 0 {
		t.Fatalf("expected 0 SLO rows, got %d", len(rows))
	}
}

func TestParseInvariants(t *testing.T) {
	content := `# SPEC: Test

## Invariants

1. **No active session is ever GC'd** — sessions with active tmux are skipped.
2. **Archive is idempotent** — attempting to archive an already-archived session returns error.
3. **Session identifiers are path-safe** — validateIdentifier() rejects bad chars.

## Dependencies
`
	invariants := parseInvariants(content)
	if len(invariants) != 3 {
		t.Fatalf("expected 3 invariants, got %d", len(invariants))
	}

	expected := []string{
		"No active session is ever GC'd",
		"Archive is idempotent",
		"Session identifiers are path-safe",
	}
	for i, want := range expected {
		if invariants[i] != want {
			t.Errorf("invariant %d: got %q, want %q", i+1, invariants[i], want)
		}
	}
}

func TestParseInvariants_Empty(t *testing.T) {
	content := `# SPEC: Test

## Purpose
No invariants.
`
	invariants := parseInvariants(content)
	if len(invariants) != 0 {
		t.Fatalf("expected 0 invariants, got %d", len(invariants))
	}
}

func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"5s max", "5s"},
		{"5s", "5s"},
		{"100MB file size", "100mb"},
		{"1MB", "1mb"},
		{"1000 entries", "1000"},
		{"1000 per pass", "1000"},
		{"50", "50"},
		{"0", "0"},
		{"100", "100"},
		{"5 min", "5m"},
		{"15 min", "15m"},
		{"24h", "24h"},
		{"1h", "1h"},
		{"30 lines", "30"},
		{"50 lines", "50"},
		{"100 chars", "100"},
		{"3 occurrences", "3"},
		{"0755", "0755"},
		{"0644", "0644"},
		{"100mb", "100mb"},
	}

	for _, tt := range tests {
		got := normalizeValue(tt.input)
		if got != tt.want {
			t.Errorf("normalizeValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMatchMetricToField(t *testing.T) {
	tests := []struct {
		metric string
		want   string
	}{
		{"Resume tmux ready wait", "resume"},
		{"Session bloat threshold", "bloat_size"},
		{"Bloat progress entry threshold", "bloat_prog"},
		{"GC session scan limit", "scan_limit"},
		{"Process kill grace period", "kill_grace"},
		{"Base trust score", "base_score"},
		{"Min trust score", "min_score"},
		{"Max trust score", "max_score"},
		{"Score delta range", "delta"},
		{"Default scan interval", "scan_interval"},
		{"Stuck timeout", "stuck"},
		{"Scan gap timeout", "scan_gap"},
		{"Worker commit lookback", "commit_lookback"},
		{"Metrics window", "metrics_window"},
		{"Tmux capture depth", "capture_depth"},
		{"Session list limit", "list_limit"},
		{"Permission prompt timeout", "permission"},
		{"No-commit timeout", "no_commit"},
		{"Error repeat threshold", "error_repeat"},
		{"Error message max length", "error_msg_len"},
		{"Max line buffer", "line_buffer"},
		{"Log directory permissions", "dir_perm"},
		{"Log file permissions", "file_perm"},
	}

	for _, tt := range tests {
		got := matchMetricToField(tt.metric)
		if got != tt.want {
			t.Errorf("matchMetricToField(%q) = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestFmtDur(t *testing.T) {
	tests := []struct {
		dur  time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{5 * time.Minute, "5m"},
		{24 * time.Hour, "24h"},
		{1 * time.Hour, "1h"},
		{2 * time.Second, "2s"},
		{10 * time.Minute, "10m"},
		{15 * time.Minute, "15m"},
	}

	for _, tt := range tests {
		got := fmtDur(tt.dur)
		if got != tt.want {
			t.Errorf("fmtDur(%v) = %q, want %q", tt.dur, got, tt.want)
		}
	}
}

func TestFmtBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{100 * 1024 * 1024, "100mb"},
		{1024 * 1024, "1mb"},
		{1024, "1kb"},
		{500, "500"},
	}

	for _, tt := range tests {
		got := fmtBytes(tt.input)
		if got != tt.want {
			t.Errorf("fmtBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFmtDeltaRange(t *testing.T) {
	deltas := map[string]int{
		"success":          5,
		"false_completion": -15,
		"stall":            -5,
		"error_loop":       -3,
		"permission_churn": -1,
	}
	got := fmtDeltaRange(deltas)
	if got != "-15to+5" {
		t.Errorf("fmtDeltaRange = %q, want %q", got, "-15to+5")
	}
}

func TestContractDrift_Integration(t *testing.T) {
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	// Create a temp dir with a test SPEC
	dir := t.TempDir()

	specContent := `# SPEC: Test Lifecycle

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Resume tmux ready wait | 5s max | source |
| Session bloat threshold | 100MB file size | source |
| Bloat progress entry threshold | 1000 entries | source |
| GC session scan limit | 1000 per pass | source |
| Process kill grace period | 2s before sandbox removal | source |

## Invariants

1. **No active session is ever GC'd** — safety first.
2. **Archive is idempotent** — no double archive.
`
	if err := os.WriteFile(filepath.Join(dir, "SPEC-session-lifecycle.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ContractDrift(nil, &ContractDriftRequest{
		SpecsDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.TotalSpecs != 1 {
		t.Errorf("TotalSpecs = %d, want 1", result.TotalSpecs)
	}

	// All SLOs should match default contracts
	for _, f := range result.Findings {
		if f.Section == "SLO" && f.Severity == DriftFail {
			t.Errorf("unexpected drift FAIL for %q: expected=%q actual=%q detail=%s",
				f.Metric, f.Expected, f.Actual, f.Detail)
		}
	}

	if result.OverallStatus != DriftPass {
		t.Errorf("OverallStatus = %q, want PASS", result.OverallStatus)
	}
}

func TestContractDrift_DetectsMismatch(t *testing.T) {
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	dir := t.TempDir()

	// Write a SPEC with a wrong value
	specContent := `# SPEC: Test

## SLOs

| Metric | Target | Source |
|--------|--------|--------|
| Resume tmux ready wait | 10s max | source |

## Invariants

1. **Something** — test.
`
	if err := os.WriteFile(filepath.Join(dir, "SPEC-session-lifecycle.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ContractDrift(nil, &ContractDriftRequest{
		SpecsDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should detect drift on resume timeout (SPEC says 10s, contract says 5s)
	foundDrift := false
	for _, f := range result.Findings {
		if f.Section == "SLO" && f.Severity == DriftFail {
			foundDrift = true
			if f.Metric != "Resume tmux ready wait" {
				t.Errorf("expected drift on resume metric, got %q", f.Metric)
			}
		}
	}
	if !foundDrift {
		t.Error("expected to find SLO drift but found none")
	}

	if result.OverallStatus != DriftFail {
		t.Errorf("OverallStatus = %q, want FAIL", result.OverallStatus)
	}
}

func TestContractDrift_MissingSpecsDir(t *testing.T) {
	_, err := ContractDrift(nil, &ContractDriftRequest{
		SpecsDir: "",
	})
	if err == nil {
		t.Error("expected error for empty specs_dir")
	}
}

func TestContractDrift_UnmappedSpec(t *testing.T) {
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SPEC-unknown.md"), []byte("# Unknown"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ContractDrift(nil, &ContractDriftRequest{
		SpecsDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.TotalSpecs != 1 {
		t.Errorf("TotalSpecs = %d, want 1", result.TotalSpecs)
	}

	// Should have a WARN finding for unmapped spec
	foundWarn := false
	for _, f := range result.Findings {
		if f.Severity == DriftWarn && f.Metric == "section_mapping" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Error("expected WARN for unmapped SPEC")
	}
}
