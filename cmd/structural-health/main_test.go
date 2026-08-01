package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPathLike(t *testing.T) {
	cases := []struct {
		tok  string
		want bool
	}{
		{"internal/ops/", true},
		{"agm/cmd/agm/", true},
		{"docs/adr/ADR-027.md", true},
		{"Agent", false},     // no slash — prose token
		{"--fields", false},  // flag, no slash
		{"", false},          // empty span
		{"a/b c/d", false},   // contains space
		{"glob/*.go", false}, // glob pattern
		{"https://x/y", false},
		{"/etc/passwd", false},               // absolute, not repo-relative
		{"~/.agm/pending/{session}/", false}, // home-dir + template placeholder
	}
	for _, c := range cases {
		if got := isPathLike(c.tok); got != c.want {
			t.Errorf("isPathLike(%q) = %v, want %v", c.tok, got, c.want)
		}
	}
}

func TestBacktickPaths(t *testing.T) {
	md := "See `internal/ops/` and `Agent`.\n" +
		"```\nfenced/code/should/be/skipped\n```\n" +
		"Also `agm/cmd/agm/` plus `--flag`.\n"
	got := backtickPaths(md)
	want := map[string]bool{"internal/ops/": true, "agm/cmd/agm/": true}
	if len(got) != len(want) {
		t.Fatalf("backtickPaths returned %v, want keys %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected path ref %q", g)
		}
	}
}

func TestScanDocPaths(t *testing.T) {
	root := t.TempDir()
	// Create one real path and reference one missing path.
	if err := os.MkdirAll(filepath.Join(root, "internal", "ops"), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "Real: `internal/ops/`\nGone: `internal/ghost/`\n"
	if err := os.WriteFile(filepath.Join(root, architectureDoc), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	got := scanDocPaths(root)
	if len(got) != 1 || got[0].Key != "internal/ghost/" {
		t.Fatalf("scanDocPaths = %+v, want single finding internal/ghost/", got)
	}
}

func TestBootstrapCommandCarriesAdmissionAndPaths(t *testing.T) {
	got := bootstrapCommand("repo root/$USER's", "custom/baseline.json", true)
	for _, want := range []string{
		`--root 'repo root/$USER'"'"'s'`,
		`--baseline 'custom/baseline.json'`,
		"--update-baseline",
		"--accept-new",
		`--reason '<why>'`,
		`--reference '<bead-or-pr>'`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("bootstrap command %q does not contain %q", got, want)
		}
	}
}

func TestBootstrapCommandOmitsAdmissionForEmptyScan(t *testing.T) {
	got := bootstrapCommand("repo", "empty.json", false)
	for _, unwanted := range []string{"--accept-new", "--reason", "--reference"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("empty-scan bootstrap command %q unexpectedly contains %q", got, unwanted)
		}
	}
	if err := validateAddedKeyAuthorization(0, strings.Contains(got, "--accept-new")); err != nil {
		t.Fatalf("empty-scan bootstrap command is rejected: %v", err)
	}
}

func TestQuoteShellWordRoundTripsPathBytes(t *testing.T) {
	want := "repo $HOME $(exit 42) `exit 43` ' \" back\\slash\nnext"
	// #nosec G204 -- exercising generated shell syntax is the purpose of this test.
	cmd := exec.Command("sh", "-c", "printf %s "+quoteShellWord(want))
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("shell round trip: %v", err)
	}
	if string(got) != want {
		t.Fatalf("shell round trip = %q, want %q", got, want)
	}
}

func TestScanFileSize(t *testing.T) {
	root := t.TempDir()
	small := make([]byte, 0)
	for range 10 {
		small = append(small, []byte("package p\n")...)
	}
	big := make([]byte, 0)
	for range fileSizeThreshold + 5 {
		big = append(big, '\n')
	}
	if err := os.WriteFile(filepath.Join(root, "small.go"), small, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "big.go"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanFileSize(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "big.go" {
		t.Fatalf("scanFileSize = %+v, want single finding big.go", got)
	}
}

func TestScanGoroutineRecover(t *testing.T) {
	root := t.TempDir()
	src := `package p

func bad() {
	go func() {
		work()
	}()
}

func good() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				_ = r
			}
		}()
		work()
	}()
}

func work() {}
`
	if err := os.WriteFile(filepath.Join(root, "g.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanGoroutineRecover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("scanGoroutineRecover = %+v, want exactly one finding (the unguarded goroutine)", got)
	}
}

func TestScanGoroutineRecoverSkipsTests(t *testing.T) {
	root := t.TempDir()
	src := `package p

func f() {
	go func() { work() }()
}

func work() {}
`
	if err := os.WriteFile(filepath.Join(root, "g_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanGoroutineRecover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("scanGoroutineRecover flagged a _test.go goroutine: %+v", got)
	}
}

func TestScanRawMemGateFlagsRawFreeGate(t *testing.T) {
	root := t.TempDir()
	// The ce-xj1b anti-pattern: gate a spawn on raw free pages.
	bad := "#!/bin/bash\nfree=$(vm_stat | awk '/Pages free/ {print $3}')\nif [ \"$free\" -lt 1000 ]; then echo 'spawn-unsafe'; fi\n"
	if err := os.WriteFile(filepath.Join(root, "resource-watchdog.sh"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanRawMemGate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "resource-watchdog.sh" {
		t.Fatalf("scanRawMemGate = %+v, want single finding resource-watchdog.sh", got)
	}
}

func TestScanRawMemGateEscapeHatchSuppresses(t *testing.T) {
	root := t.TempDir()
	// Same raw idiom, but the script routes the decision through
	// memory_pressure — the correct macOS source — so it must NOT be flagged.
	ok := "#!/bin/bash\n# vm_stat Pages free is misleading on macOS; use the real source\nfree=$(memory_pressure -Q | awk '/percentage/ {print $5}')\nif [ \"$free\" -lt 10 ]; then echo 'spawn-unsafe'; fi\n"
	if err := os.WriteFile(filepath.Join(root, "procwatch.sh"), []byte(ok), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanRawMemGate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("scanRawMemGate flagged a memory_pressure-based script: %+v", got)
	}
}

func TestScanRawMemGateIgnoresPlainVmStat(t *testing.T) {
	root := t.TempDir()
	// vm_stat used for reporting, without a free-page gate, is not the
	// anti-pattern and must not be flagged.
	neutral := "#!/bin/bash\nvm_stat\necho 'done'\n"
	if err := os.WriteFile(filepath.Join(root, "report.sh"), []byte(neutral), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanRawMemGate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("scanRawMemGate flagged plain vm_stat reporting: %+v", got)
	}
}

func TestDiffDetectsRegressionAndFixed(t *testing.T) {
	current := map[string][]finding{
		"dead-package":      {{Key: "pkg/new"}},
		"file-size":         {},
		"zero-test":         {},
		"doc-path":          {},
		"goroutine-recover": {},
	}
	bl := baseline{
		Version: 1,
		Findings: map[string][]string{
			"dead-package": {"pkg/old"},
		},
	}
	rep := diff(current, bl)
	if rep.regressionCount() != 1 {
		t.Fatalf("regressionCount = %d, want 1", rep.regressionCount())
	}
	var dead scanReport
	for _, s := range rep.Scans {
		if s.Scan == "dead-package" {
			dead = s
		}
	}
	if len(dead.Regressions) != 1 || dead.Regressions[0] != "pkg/new" {
		t.Errorf("regressions = %v, want [pkg/new]", dead.Regressions)
	}
	if len(dead.Fixed) != 1 || dead.Fixed[0] != "pkg/old" {
		t.Errorf("fixed = %v, want [pkg/old]", dead.Fixed)
	}
}

func TestDiffNoRegressionWhenWithinBaseline(t *testing.T) {
	current := map[string][]finding{
		"dead-package": {{Key: "pkg/a"}, {Key: "pkg/b"}},
	}
	bl := baseline{
		Version:  1,
		Findings: map[string][]string{"dead-package": {"pkg/a", "pkg/b", "pkg/c"}},
	}
	rep := diff(current, bl)
	if rep.regressionCount() != 0 {
		t.Fatalf("regressionCount = %d, want 0 (all current findings are baselined)", rep.regressionCount())
	}
}

func TestFindingKeysSortsAndFillsScans(t *testing.T) {
	current := map[string][]finding{
		"dead-package":      {{Key: "pkg/z"}, {Key: "pkg/a"}},
		"file-size":         {{Key: "big.go"}},
		"zero-test":         {},
		"doc-path":          {},
		"goroutine-recover": {},
	}
	got, err := findingKeys(current)
	if err != nil {
		t.Fatal(err)
	}
	// Keys must be sorted on disk.
	dead := got["dead-package"]
	if len(dead) != 2 || dead[0] != "pkg/a" || dead[1] != "pkg/z" {
		t.Errorf("dead-package keys = %v, want sorted [pkg/a pkg/z]", dead)
	}
	// Empty scans must serialize as [] (not absent) so the schema is stable.
	if got["zero-test"] == nil || got["raw-mem-gate"] == nil {
		t.Errorf("zero-test should round-trip as empty slice, got nil")
	}
}
