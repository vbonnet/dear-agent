package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadQualityGatesConfig(t *testing.T) {
	t.Run("valid config with expect_exit", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "build-clean"
    check: "go build ./..."
    expect_exit: 0
  - name: "tests-pass"
    check: "go test ./..."
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadQualityGatesConfig(cfgPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Gates) != 2 {
			t.Fatalf("expected 2 gates, got %d", len(cfg.Gates))
		}
		if cfg.Gates[0].Name != "build-clean" {
			t.Errorf("expected gate name 'build-clean', got %q", cfg.Gates[0].Name)
		}
		if cfg.Gates[0].ExpectExit == nil || *cfg.Gates[0].ExpectExit != 0 {
			t.Errorf("expected expect_exit=0 for gate 0")
		}
	})

	t.Run("valid config with expect_empty", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "no-fmt-errors"
    check: "gofmt -l ."
    expect_empty: true
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadQualityGatesConfig(cfgPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Gates) != 1 {
			t.Fatalf("expected 1 gate, got %d", len(cfg.Gates))
		}
		if cfg.Gates[0].ExpectEmpty == nil || !*cfg.Gates[0].ExpectEmpty {
			t.Error("expected expect_empty=true")
		}
	})

	t.Run("empty gates list", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		if err := os.WriteFile(cfgPath, []byte("gates: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadQualityGatesConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for empty gates")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - check: "echo hello"
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadQualityGatesConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("missing check", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "test"
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadQualityGatesConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for missing check")
		}
	})

	t.Run("missing expectation", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "test"
    check: "echo hello"
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadQualityGatesConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for missing expectation")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadQualityGatesConfig("/nonexistent/path.yaml")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		if err := os.WriteFile(cfgPath, []byte("{{invalid"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadQualityGatesConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for invalid YAML")
		}
	})
}

func TestRunQualityGates(t *testing.T) {
	t.Run("all gates pass", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "exit-zero"
    check: "true"
    expect_exit: 0
  - name: "empty-output"
    check: "true"
    expect_empty: true
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			RecordTrust: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Passed {
			t.Error("expected all gates to pass")
		}
		if result.TotalGates != 2 {
			t.Errorf("expected 2 total gates, got %d", result.TotalGates)
		}
		if result.PassedCount != 2 {
			t.Errorf("expected 2 passed, got %d", result.PassedCount)
		}
		if result.FailedCount != 0 {
			t.Errorf("expected 0 failed, got %d", result.FailedCount)
		}
	})

	t.Run("gate fails on exit code", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "should-fail"
    check: "false"
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			RecordTrust: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Passed {
			t.Error("expected gate to fail")
		}
		if result.FailedCount != 1 {
			t.Errorf("expected 1 failed, got %d", result.FailedCount)
		}
		if result.Gates[0].ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", result.Gates[0].ExitCode)
		}
	})

	t.Run("gate fails on non-empty output", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "should-be-empty"
    check: "echo 'some output'"
    expect_empty: true
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			RecordTrust: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Passed {
			t.Error("expected gate to fail")
		}
		if result.Gates[0].Passed {
			t.Error("expected first gate to fail")
		}
	})

	t.Run("mixed pass and fail", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "passes"
    check: "true"
    expect_exit: 0
  - name: "fails"
    check: "false"
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			RecordTrust: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Passed {
			t.Error("expected overall result to fail")
		}
		if result.PassedCount != 1 || result.FailedCount != 1 {
			t.Errorf("expected 1 pass + 1 fail, got %d pass + %d fail", result.PassedCount, result.FailedCount)
		}
	})

	t.Run("branch substitution", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		// $BRANCH should be substituted with the branch name
		content := `gates:
  - name: "branch-check"
    check: "echo $BRANCH"
    expect_empty: true
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			Branch:      "feature/test",
			RecordTrust: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// $BRANCH gets substituted to "feature/test", so output is non-empty → fails
		if result.Passed {
			t.Error("expected gate to fail (non-empty output after branch substitution)")
		}
		if result.Gates[0].Output == "" {
			t.Error("expected output to contain branch name")
		}
	})

	t.Run("records trust on pass", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "pass-gate"
    check: "true"
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		// Set up trust dir for recording
		trustDirPath := filepath.Join(dir, ".agm", "trust")
		if err := os.MkdirAll(trustDirPath, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", dir)

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "trust-test-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			RecordTrust: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Passed {
			t.Error("expected pass")
		}

		// Verify trust event was recorded
		events, err := readTrustEvents("trust-test-session")
		if err != nil {
			t.Fatalf("failed to read trust events: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 trust event, got %d", len(events))
		}
		if events[0].EventType != TrustEventSuccess {
			t.Errorf("expected success event, got %q", events[0].EventType)
		}
	})

	t.Run("records trust on failure", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "fail-gate"
    check: "false"
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		trustDirPath := filepath.Join(dir, ".agm", "trust")
		if err := os.MkdirAll(trustDirPath, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", dir)

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "trust-fail-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			RecordTrust: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Passed {
			t.Error("expected failure")
		}

		events, err := readTrustEvents("trust-fail-session")
		if err != nil {
			t.Fatalf("failed to read trust events: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 trust event, got %d", len(events))
		}
		if events[0].EventType != TrustEventQualityGateFailure {
			t.Errorf("expected quality_gate_failure event, got %q", events[0].EventType)
		}
	})

	t.Run("missing session name", func(t *testing.T) {
		ctx := &OpContext{}
		_, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			ConfigPath: "/some/path",
			RepoDir:    "/some/dir",
		})
		if err == nil {
			t.Fatal("expected error for missing session name")
		}
	})

	t.Run("missing config path", func(t *testing.T) {
		ctx := &OpContext{}
		_, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test",
			RepoDir:     "/some/dir",
		})
		if err == nil {
			t.Fatal("expected error for missing config path")
		}
	})

	t.Run("missing repo dir", func(t *testing.T) {
		ctx := &OpContext{}
		_, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test",
			ConfigPath:  "/some/path",
		})
		if err == nil {
			t.Fatal("expected error for missing repo dir")
		}
	})

	t.Run("duration is recorded", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "quality-gates.yaml")
		content := `gates:
  - name: "sleep-gate"
    check: "sleep 0.1"
    expect_exit: 0
`
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx := &OpContext{}
		result, err := RunQualityGates(ctx, &RunQualityGatesRequest{
			SessionName: "test-session",
			ConfigPath:  cfgPath,
			RepoDir:     dir,
			RecordTrust: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Gates[0].DurationMs < 50 {
			t.Errorf("expected duration >= 50ms, got %dms", result.Gates[0].DurationMs)
		}
	})
}

func TestRunSingleGate(t *testing.T) {
	dir := t.TempDir()

	t.Run("expect_exit matches", func(t *testing.T) {
		exitCode := 0
		gate := QualityGate{
			Name:       "test",
			Check:      "true",
			ExpectExit: &exitCode,
		}
		result := runSingleGate(gate, dir, "")
		if !result.Passed {
			t.Error("expected gate to pass")
		}
		if result.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.ExitCode)
		}
	})

	t.Run("expect_exit mismatch", func(t *testing.T) {
		exitCode := 0
		gate := QualityGate{
			Name:       "test",
			Check:      "exit 1",
			ExpectExit: &exitCode,
		}
		result := runSingleGate(gate, dir, "")
		if result.Passed {
			t.Error("expected gate to fail")
		}
		if result.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", result.ExitCode)
		}
	})

	t.Run("expect_empty passes with no output", func(t *testing.T) {
		expectEmpty := true
		gate := QualityGate{
			Name:        "test",
			Check:       "true",
			ExpectEmpty: &expectEmpty,
		}
		result := runSingleGate(gate, dir, "")
		if !result.Passed {
			t.Error("expected gate to pass")
		}
	})

	t.Run("expect_empty fails with output", func(t *testing.T) {
		expectEmpty := true
		gate := QualityGate{
			Name:        "test",
			Check:       "echo hello",
			ExpectEmpty: &expectEmpty,
		}
		result := runSingleGate(gate, dir, "")
		if result.Passed {
			t.Error("expected gate to fail")
		}
	})

	t.Run("command not found", func(t *testing.T) {
		exitCode := 0
		gate := QualityGate{
			Name:       "test",
			Check:      "nonexistent_command_xyz_123",
			ExpectExit: &exitCode,
		}
		result := runSingleGate(gate, dir, "")
		if result.Passed {
			t.Error("expected gate to fail for nonexistent command")
		}
	})

	t.Run("branch substitution in check command", func(t *testing.T) {
		exitCode := 0
		gate := QualityGate{
			Name:       "test",
			Check:      "echo $BRANCH | grep feature",
			ExpectExit: &exitCode,
		}
		result := runSingleGate(gate, dir, "feature/my-branch")
		if !result.Passed {
			t.Errorf("expected gate to pass, output: %s, error: %s", result.Output, result.Error)
		}
	})
}
