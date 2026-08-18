package validator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeSpecFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "SPEC-solution-requirements.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SPEC-solution-requirements.md: %v", err)
	}
}

// TestValidateSpecEARS_Valid confirms the SPEC deliverable with conforming EARS
// requirements passes the SPEC/SPEC gate deterministically (no API key needed).
func TestValidateSpecEARS_Valid(t *testing.T) {
	dir := t.TempDir()
	writeSpecFile(t, dir, `# SPEC

## Requirements

- When the user submits the form, the system shall validate all fields.
- The system shall not store plaintext passwords.
- The system shall record an audit entry for each request.
`)
	if err := validateDocQuality("SPEC", dir); err != nil {
		t.Fatalf("expected SPEC to pass with valid EARS requirements, got: %v", err)
	}
}

// TestValidateSpecEARS_ZeroRequirements confirms a prose-only deliverable fails.
func TestValidateSpecEARS_ZeroRequirements(t *testing.T) {
	dir := t.TempDir()
	writeSpecFile(t, dir, "# SPEC\n\nThis is just prose with no requirements.\n")
	err := validateDocQuality("SPEC", dir)
	if err == nil {
		t.Fatal("expected error for the SPEC deliverable with zero requirements")
	}
	verr := &ValidationError{}
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Phase != "complete SPEC" {
		t.Errorf("expected Phase 'complete SPEC', got %q", verr.Phase)
	}
}

// TestValidateSpecEARS_NonConformingStrict confirms the gate is strict: a
// requirement containing the keyword but not matching any template fails even
// when other valid requirements are present.
func TestValidateSpecEARS_NonConformingStrict(t *testing.T) {
	dir := t.TempDir()
	writeSpecFile(t, dir, `# SPEC

- The system shall log requests.
- Eventually the thing shall happen somehow.
`)
	err := validateDocQuality("SPEC", dir)
	if err == nil {
		t.Fatal("expected strict gate to fail on a non-conforming requirement")
	}
	verr := &ValidationError{}
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if !containsString(verr.Fix, "Eventually the thing shall happen somehow") {
		t.Errorf("expected Fix to cite the offending line, got: %s", verr.Fix)
	}
}

// TestValidateSpecEARS_Missing confirms a missing SPEC deliverable is reported.
func TestValidateSpecEARS_Missing(t *testing.T) {
	dir := t.TempDir()
	err := validateDocQuality("SPEC", dir)
	if err == nil {
		t.Fatal("expected error for missing SPEC-solution-requirements.md")
	}
	verr := &ValidationError{}
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if !containsString(verr.Reason, "SPEC-solution-requirements.md does not exist") {
		t.Errorf("expected Reason to mention the missing SPEC deliverable, got: %s", verr.Reason)
	}
}

// TestValidateSpecEARS_ConfigOverride confirms a project-local .earslint.yml
// changes the accepted patterns/keyword.
func TestValidateSpecEARS_ConfigOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := "requirement_keyword: must\npatterns:\n  - name: must\n    regex: '(?i)^the\\s+.+\\s+must\\s+.+'\n"
	if err := os.WriteFile(filepath.Join(dir, earsConfigFile), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSpecFile(t, dir, "The system must persist all state.\n")
	if err := validateDocQuality("SPEC", dir); err != nil {
		t.Fatalf("expected pass with custom EARS config, got: %v", err)
	}
}

// TestValidateSpecEARS_BadConfig confirms a malformed override is surfaced.
func TestValidateSpecEARS_BadConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, earsConfigFile), []byte("patterns: [oops\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSpecFile(t, dir, "The system shall work.\n")
	err := validateDocQuality("SPEC", dir)
	if err == nil {
		t.Fatal("expected error for malformed .earslint.yml")
	}
}
