package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("missing file should return empty slice, got %+v", got)
	}
}

func TestLoadParsesAllCriterionTypes(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
acceptance-criteria:
  - type: tests-pass
    command: "go test ./..."
  - type: lint-clean
    command: "golangci-lint run ./..."
  - type: no-regressions
    description: "No existing tests broken"
  - type: custom
    description: "Manual UI smoke check"
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0].Type != TypeTestsPass || got[0].Command != "go test ./..." {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[2].Type != TypeNoRegressions || got[2].Description != "No existing tests broken" {
		t.Errorf("row 2 = %+v", got[2])
	}
}

func TestLoadAbsentSectionReturnsEmpty(t *testing.T) {
	// .dear-agent.yml exists but has no acceptance-criteria: block.
	dir := t.TempDir()
	yml := `version: 1
repo: demo
output-dirs:
  code: .
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("absent section should give empty slice, got %+v", got)
	}
}

func TestValidateRejectsUnknownType(t *testing.T) {
	err := Validate([]Criterion{{Type: "bogus", Command: "x"}})
	if err == nil {
		t.Fatal("unknown type should fail")
	}
	if !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("error should mention unknown type: %v", err)
	}
}

func TestValidateRequiresCommandForRunnableTypes(t *testing.T) {
	for _, typ := range []Type{TypeTestsPass, TypeLintClean} {
		t.Run(string(typ), func(t *testing.T) {
			if err := Validate([]Criterion{{Type: typ}}); err == nil {
				t.Errorf("%s without command should fail", typ)
			}
			if err := Validate([]Criterion{{Type: typ, Command: "ok"}}); err != nil {
				t.Errorf("%s with command should pass: %v", typ, err)
			}
		})
	}
}

func TestValidateAllowsDescriptionOnlyForNoRegressions(t *testing.T) {
	if err := Validate([]Criterion{{Type: TypeNoRegressions, Description: "stable"}}); err != nil {
		t.Errorf("description-only no-regressions should pass: %v", err)
	}
	if err := Validate([]Criterion{{Type: TypeNoRegressions}}); err != nil {
		t.Errorf("bare no-regressions should pass: %v", err)
	}
}

func TestValidateRejectsEmptyCustom(t *testing.T) {
	if err := Validate([]Criterion{{Type: TypeCustom}}); err == nil {
		t.Error("empty custom should fail")
	}
	if err := Validate([]Criterion{{Type: TypeCustom, Description: "x"}}); err != nil {
		t.Errorf("custom with description should pass: %v", err)
	}
}

func TestParseBytesInvalidYAML(t *testing.T) {
	_, err := ParseBytes([]byte("acceptance-criteria: : :"))
	if err == nil {
		t.Error("invalid YAML should fail")
	}
}

func TestCriterionString(t *testing.T) {
	c := Criterion{Type: TypeTestsPass, Command: "go test"}
	if got := c.String(); !strings.Contains(got, "tests-pass") || !strings.Contains(got, "go test") {
		t.Errorf("String() = %q", got)
	}
}

func TestLoadInvalidYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(": : :"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("invalid YAML should fail Load")
	}
}
