package retrolint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// RLINT-01: When a retrospective file is evaluated, the system shall require at least one declared guard or deferred guard.
func TestRLINT01_RequiresDeclaredGuard(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retroPath := filepath.Join(repoRoot, "retro.md")
	content := `# Incident Retro
Date: 2026-09-05

## Define
Something broke.

## Resolve
Fixed it.
`
	if err := os.WriteFile(retroPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %s", res.Status)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected errors for missing guard, got none")
	}
}

// RLINT-02: When a guard specifies a repository file path, the system shall verify that the path exists in the target repository.
func TestRLINT02_VerifiesPathArtifactExists(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retroPath := filepath.Join(repoRoot, "retro.md")
	existingArtifact := "pkg/foo/foo_test.go"
	fullArtifactPath := filepath.Join(repoRoot, existingArtifact)
	if err := os.MkdirAll(filepath.Dir(fullArtifactPath), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullArtifactPath, []byte("package foo"), 0600); err != nil {
		t.Fatal(err)
	}

	// 1. Existing path passes
	validRetro := `# Incident Retro
Date: 2026-09-05

## Guards
- test: pkg/foo/foo_test.go
`
	if err := os.WriteFile(retroPath, []byte(validRetro), 0600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("expected StatusPass for existing path, got %s (errors: %v)", res.Status, res.Errors)
	}

	// 2. Missing path fails
	missingRetro := `# Incident Retro
Date: 2026-09-05

## Guards
- test: pkg/missing/missing_test.go
`
	if err := os.WriteFile(retroPath, []byte(missingRetro), 0600); err != nil {
		t.Fatal(err)
	}

	res, err = EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("expected StatusFail for missing path, got %s", res.Status)
	}
	if len(res.Errors) == 0 || !strings.Contains(res.Errors[0], "does not exist") {
		t.Fatalf("expected does not exist error, got: %v", res.Errors)
	}
}

// RLINT-03: When a guard specifies a launchd label, the system shall verify that the launchd label is non-empty.
func TestRLINT03_VerifiesLaunchdLabelNonEmpty(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retroPath := filepath.Join(repoRoot, "retro.md")

	// Valid non-empty label
	validRetro := `# Incident Retro
Date: 2026-09-05

## Guards
- launchd: com.dear-agent.recovery-loop
`
	if err := os.WriteFile(retroPath, []byte(validRetro), 0600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %s (%v)", res.Status, res.Errors)
	}

	// Empty label fails
	emptyRetro := `# Incident Retro
Date: 2026-09-05

Guards:
- type: launchd
  label: ""
`
	if err := os.WriteFile(retroPath, []byte(emptyRetro), 0600); err != nil {
		t.Fatal(err)
	}

	res, err = EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %s", res.Status)
	}
}

// RLINT-04: When a guard specifies a deferred action, the system shall require both a non-empty bead identifier and a non-empty rationale.
func TestRLINT04_RequiresBeadAndRationaleForDeferred(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retroPath := filepath.Join(repoRoot, "retro.md")

	// Valid deferred
	validRetro := `# Incident Retro
Date: 2026-09-05

## Guards
- deferred: ce-8v9d3 (Waiting for infrastructure landing)
`
	if err := os.WriteFile(retroPath, []byte(validRetro), 0600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("expected StatusPass for valid deferred guard, got %s (%v)", res.Status, res.Errors)
	}

	// Missing rationale
	noReasonRetro := `# Incident Retro
Date: 2026-09-05

## Guards
- type: deferred
  bead: ce-8v9d3
`
	if err := os.WriteFile(retroPath, []byte(noReasonRetro), 0600); err != nil {
		t.Fatal(err)
	}

	res, err = EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("expected StatusFail for missing rationale, got %s", res.Status)
	}

	// Missing bead
	noBeadRetro := `# Incident Retro
Date: 2026-09-05

## Guards
- type: deferred
  reason: Scheduled for future sprint
`
	if err := os.WriteFile(retroPath, []byte(noBeadRetro), 0600); err != nil {
		t.Fatal(err)
	}

	res, err = EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("expected StatusFail for missing bead, got %s", res.Status)
	}
}

// RLINT-05: If a retrospective path is present in the grandfathered baseline store, then the system shall waive missing guard requirements for that file.
func TestRLINT05_GrandfatheredBaselineWaiver(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retrosDir := filepath.Join(repoRoot, "retrospectives")
	if err := os.MkdirAll(retrosDir, 0750); err != nil {
		t.Fatal(err)
	}

	retroName := "2026-06-20-old-incident.md"
	retroPath := filepath.Join(retrosDir, retroName)
	content := `# Old Incident
Date: 2026-06-20

No guards block here.
`
	if err := os.WriteFile(retroPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	baselineContent := `{"path": "2026-06-20-old-incident.md", "status": "grandfathered", "reason": "historical"}
`
	baseline, err := LoadBaseline(strings.NewReader(baselineContent))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, baseline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusWaived {
		t.Fatalf("expected StatusWaived, got %s", res.Status)
	}
	if !res.Waived {
		t.Fatalf("expected Waived to be true")
	}
	if len(res.Errors) > 0 {
		t.Fatalf("expected 0 errors for waived entry, got: %v", res.Errors)
	}
}

// RLINT-06: While ratchet mode is enabled, the system shall reject baseline entries that declare valid guards or reference removed files.
func TestRLINT06_RatchetRejectsValidGuardsOrMissingFiles(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retrosDir := filepath.Join(repoRoot, "retrospectives")
	if err := os.MkdirAll(retrosDir, 0750); err != nil {
		t.Fatal(err)
	}

	// 1. File that now declares a valid guard
	fixedRetroPath := filepath.Join(retrosDir, "fixed-retro.md")
	fixedContent := `# Fixed Retro
Date: 2026-09-05

## Guards
- deferred: ce-12345 (Scheduled)
`
	if err := os.WriteFile(fixedRetroPath, []byte(fixedContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Baseline lists both fixed-retro.md and a removed file deleted-retro.md
	baselineContent := `{"path": "fixed-retro.md", "status": "grandfathered"}
{"path": "deleted-retro.md", "status": "grandfathered"}
`
	baseline, err := LoadBaseline(strings.NewReader(baselineContent))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	errors, err := CheckRatchet(ctx, repoRoot, retrosDir, baseline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(errors) != 2 {
		t.Fatalf("expected 2 ratchet errors, got %d: %v", len(errors), errors)
	}

	hasValidGuardErr := false
	hasRemovedErr := false
	for _, errStr := range errors {
		if strings.Contains(errStr, "now declares valid guards") {
			hasValidGuardErr = true
		}
		if strings.Contains(errStr, "references non-existent or removed file") {
			hasRemovedErr = true
		}
	}
	if !hasValidGuardErr {
		t.Errorf("missing valid guards ratchet error: %v", errors)
	}
	if !hasRemovedErr {
		t.Errorf("missing removed file ratchet error: %v", errors)
	}
}

// RLINT-07: When evaluated under absence-alarm mode, the system shall classify retrospectives added within the lookback window lacking valid guards as ABSENT.
func TestRLINT07_AbsenceAlarmClassifiesRecentLackingGuardsAsAbsent(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retrosDir := filepath.Join(repoRoot, "retrospectives")
	if err := os.MkdirAll(retrosDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Recent retro (2 days ago) lacking guards
	recentRetro := filepath.Join(retrosDir, "2026-09-03-recent-defect.md")
	if err := os.WriteFile(recentRetro, []byte("# Defect\nDate: 2026-09-03\nNo guards.\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Old retro (30 days ago) lacking guards
	oldRetro := filepath.Join(retrosDir, "2026-08-01-old-defect.md")
	if err := os.WriteFile(oldRetro, []byte("# Old Defect\nDate: 2026-08-01\nNo guards.\n"), 0600); err != nil {
		t.Fatal(err)
	}

	now, err := time.Parse("2006-01-02", "2026-09-05")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	opts := Options{
		RepoRoot:        repoRoot,
		RetrosDir:       retrosDir,
		AbsenceLookback: 7 * 24 * time.Hour,
		Now:             now,
	}

	report, err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Status != StatusAbsent {
		t.Fatalf("expected report status ABSENT, got %s", report.Status)
	}

	foundRecentAbsent := false
	for _, res := range report.Results {
		if strings.Contains(res.Path, "2026-09-03-recent-defect.md") {
			if res.Status == StatusAbsent {
				foundRecentAbsent = true
			}
		}
	}
	if !foundRecentAbsent {
		t.Fatalf("expected recent retro to be marked StatusAbsent")
	}
}

// RLINT-12: When a retrospective is evaluated, the system shall bound execution with a timeout so file reads cannot hang indefinitely.
func TestRLINT12_TimeoutBoundedExecution(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retroPath := filepath.Join(repoRoot, "retro.md")
	if err := os.WriteFile(retroPath, []byte("# Retro\nDate: 2026-09-05\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Pre-canceled context should fail immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := EvaluateRetrospectiveFile(ctx, repoRoot, retroPath, nil)
	if err == nil {
		t.Fatalf("expected context canceled error, got nil")
	}
}
