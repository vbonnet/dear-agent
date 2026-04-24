package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractAssertions_RemoveDependency(t *testing.T) {
	purpose := "Remove Temporal SDK dependency (go.temporal.io) from the project"
	assertions := ExtractAssertions(purpose)

	// Should produce negative assertions for the dependency
	var foundDepAssertion bool
	for _, a := range assertions {
		if a.Type == Negative && a.Pattern == "go.temporal.io" {
			foundDepAssertion = true
			break
		}
	}
	if !foundDepAssertion {
		t.Errorf("expected negative assertion for go.temporal.io dependency, got assertions: %+v", assertions)
	}
}

func TestExtractAssertions_DeleteDirectory(t *testing.T) {
	purpose := "delete coordinator/ directory"
	assertions := ExtractAssertions(purpose)

	var foundDirAssertion bool
	for _, a := range assertions {
		if a.Type == Negative && a.PathCheck == "coordinator" {
			foundDirAssertion = true
			break
		}
	}
	if !foundDirAssertion {
		t.Errorf("expected negative path assertion for coordinator/, got assertions: %+v", assertions)
	}
}

func TestExtractAssertions_FixFunction(t *testing.T) {
	purpose := "fix broadcastFromMPC function"
	assertions := ExtractAssertions(purpose)

	var foundFixAssertion bool
	for _, a := range assertions {
		if a.Type == Positive && a.Pattern == "broadcastFromMPC" {
			foundFixAssertion = true
			break
		}
	}
	if !foundFixAssertion {
		t.Errorf("expected positive assertion for broadcastFromMPC, got assertions: %+v", assertions)
	}
}

func TestExtractAssertions_CreateFile(t *testing.T) {
	purpose := "create verify_completion.go at cmd/agm/"
	assertions := ExtractAssertions(purpose)

	var foundCreateAssertion bool
	for _, a := range assertions {
		if a.Type == Positive {
			foundCreateAssertion = true
			break
		}
	}
	if !foundCreateAssertion {
		t.Errorf("expected positive assertion for creation, got assertions: %+v", assertions)
	}
}

func TestExtractAssertions_MultipleRemovalDeps(t *testing.T) {
	purpose := "Remove go.temporal.io/sdk and go.temporal.io/api from the project"
	assertions := ExtractAssertions(purpose)

	foundSDK := false
	foundAPI := false
	for _, a := range assertions {
		if a.Type == Negative && a.Pattern == "go.temporal.io/sdk" {
			foundSDK = true
		}
		if a.Type == Negative && a.Pattern == "go.temporal.io/api" {
			foundAPI = true
		}
	}
	if !foundSDK {
		t.Errorf("expected negative assertion for go.temporal.io/sdk")
	}
	if !foundAPI {
		t.Errorf("expected negative assertion for go.temporal.io/api")
	}
}

func TestCheckAssertion_NegativePattern_Pass(t *testing.T) {
	// Create a temp repo with no matching content
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func main() {
	println("hello")
}
`)

	assertion := Assertion{
		Type:        Negative,
		Description: "should be removed: go.temporal.io",
		Pattern:     "go.temporal.io",
	}

	result := CheckAssertion(dir, assertion)
	if !result.Pass {
		t.Errorf("expected PASS (pattern not found), got FAIL: %s", result.Evidence)
	}
}

func TestCheckAssertion_NegativePattern_Fail(t *testing.T) {
	// Create a temp repo WITH matching content
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/myproject

go 1.21

require go.temporal.io/sdk v1.25.0
`)

	assertion := Assertion{
		Type:        Negative,
		Description: "dependency should be removed: go.temporal.io",
		Pattern:     "go.temporal.io",
		GlobPattern: "go.mod",
	}

	result := CheckAssertion(dir, assertion)
	if result.Pass {
		t.Errorf("expected FAIL (pattern found in go.mod), got PASS")
	}
}

func TestCheckAssertion_PositivePattern_Pass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "handler.go"), `package main

func broadcastFromMPC() {
	// fixed implementation
}
`)

	assertion := Assertion{
		Type:        Positive,
		Description: "should exist: broadcastFromMPC",
		Pattern:     "broadcastFromMPC",
	}

	result := CheckAssertion(dir, assertion)
	if !result.Pass {
		t.Errorf("expected PASS (function found), got FAIL: %s", result.Evidence)
	}
}

func TestCheckAssertion_PositivePattern_Fail(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "handler.go"), `package main

func main() {}
`)

	assertion := Assertion{
		Type:        Positive,
		Description: "should exist: broadcastFromMPC",
		Pattern:     "broadcastFromMPC",
	}

	result := CheckAssertion(dir, assertion)
	if result.Pass {
		t.Errorf("expected FAIL (function not found), got PASS")
	}
}

func TestCheckAssertion_DirectoryNotExist_Pass(t *testing.T) {
	dir := t.TempDir()
	// Don't create coordinator/ directory

	assertion := Assertion{
		Type:        Negative,
		Description: "directory should not exist: coordinator/",
		PathCheck:   "coordinator",
	}

	result := CheckAssertion(dir, assertion)
	if !result.Pass {
		t.Errorf("expected PASS (directory doesn't exist), got FAIL: %s", result.Evidence)
	}
}

func TestCheckAssertion_DirectoryNotExist_Fail(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "coordinator"), 0755)

	assertion := Assertion{
		Type:        Negative,
		Description: "directory should not exist: coordinator/",
		PathCheck:   "coordinator",
	}

	result := CheckAssertion(dir, assertion)
	if result.Pass {
		t.Errorf("expected FAIL (directory still exists), got PASS")
	}
}

func TestVerify_MixedAssertions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func broadcastFromMPC() {}
`)

	assertions := []Assertion{
		{
			Type:        Negative,
			Description: "should be removed: go.temporal.io",
			Pattern:     "go.temporal.io",
		},
		{
			Type:        Positive,
			Description: "should exist: broadcastFromMPC",
			Pattern:     "broadcastFromMPC",
		},
	}

	report := Verify("test-session", "fix broadcastFromMPC and remove Temporal", dir, assertions)

	if !report.Passed() {
		t.Errorf("expected all assertions to pass, got %d failures", report.FailCount())
		for _, r := range report.Results {
			if !r.Pass {
				t.Errorf("  FAIL: %s — %s", r.Assertion.Description, r.Evidence)
			}
		}
	}
	if report.PassCount() != 2 {
		t.Errorf("expected 2 passes, got %d", report.PassCount())
	}
}

func TestVerify_FalseCompletion(t *testing.T) {
	// Simulate a false completion: prompt says "remove Temporal" but it's still there
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/test

require go.temporal.io/sdk v1.25.0
`)
	writeFile(t, filepath.Join(dir, "worker.go"), `package main

import "go.temporal.io/sdk/client"

func startWorker() {
	c, _ := client.Dial(client.Options{})
	defer c.Close()
}
`)

	assertions := []Assertion{
		{
			Type:        Negative,
			Description: "dependency should be removed: go.temporal.io",
			Pattern:     "go.temporal.io",
			GlobPattern: "go.mod",
		},
		{
			Type:        Negative,
			Description: "import should be removed: go.temporal.io",
			Pattern:     "go.temporal.io",
			GlobPattern: "*.go",
		},
	}

	report := Verify("false-completion-session", "Remove Temporal SDK", dir, assertions)

	if report.Passed() {
		t.Error("expected report to FAIL (false completion not caught)")
	}
	if report.FailCount() != 2 {
		t.Errorf("expected 2 failures, got %d", report.FailCount())
	}
}

func TestReport_EmptyResults(t *testing.T) {
	report := &Report{
		SessionID: "test",
		Purpose:   "test",
	}
	if report.Passed() {
		t.Error("empty report should not be considered passing")
	}
	if report.FailCount() != 0 {
		t.Errorf("expected 0 failures for empty report, got %d", report.FailCount())
	}
}

func TestSearchFiles_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	writeFile(t, filepath.Join(dir, ".git", "config"), "go.temporal.io")
	writeFile(t, filepath.Join(dir, "main.go"), "package main")

	matches := searchFiles(dir, "go.temporal.io", "")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (should skip .git), got %d: %v", len(matches), matches)
	}
}

func TestSearchFiles_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "image.png"), "go.temporal.io")
	writeFile(t, filepath.Join(dir, "main.go"), "package main")

	matches := searchFiles(dir, "go.temporal.io", "")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (should skip .png), got %d: %v", len(matches), matches)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
