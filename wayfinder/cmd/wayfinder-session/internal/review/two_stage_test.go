package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestPersonaStage verifies stage classification: spec compliance is Stage 1,
// every code-quality persona is Stage 2.
func TestPersonaStage(t *testing.T) {
	if got := PersonaStage(PersonaSpecCompliance); got != StageSpecCompliance {
		t.Errorf("spec compliance should be Stage 1, got %d", got)
	}
	for _, p := range CodeQualityPersonas() {
		if got := PersonaStage(p); got != StageCodeQuality {
			t.Errorf("persona %s should be Stage 2, got %d", p, got)
		}
	}
}

// TestSpecCompliancePersonaConfig checks the new Stage 1 persona is fully defined.
func TestSpecCompliancePersonaConfig(t *testing.T) {
	cfg := GetPersonaConfig(PersonaSpecCompliance)

	if cfg.Name == "" {
		t.Error("spec-compliance persona must have a name")
	}
	if len(cfg.FocusAreas) == 0 {
		t.Error("spec-compliance persona must declare focus areas")
	}
	if !strings.Contains(cfg.Prompt, "STAGE 1") {
		t.Error("spec-compliance prompt should identify itself as Stage 1")
	}
	if !strings.Contains(strings.ToLower(cfg.Prompt), "spec") {
		t.Error("spec-compliance prompt should reference the spec/requirements")
	}
}

// TestAllPersonasOrdering ensures AllPersonas lists Stage 1 before Stage 2.
func TestAllPersonasOrdering(t *testing.T) {
	all := AllPersonas()
	if len(all) == 0 {
		t.Fatal("AllPersonas returned no personas")
	}
	if all[0] != PersonaSpecCompliance {
		t.Errorf("AllPersonas should start with the Stage 1 spec-compliance persona, got %s", all[0])
	}
	if len(all) != len(CodeQualityPersonas())+1 {
		t.Errorf("AllPersonas should be spec compliance + all code-quality personas, got %d", len(all))
	}
}

// TestAntiSycophancyInAllPrompts asserts every persona prompt carries the
// anti-sycophancy directive instructing the worker to flag disagreements rather
// than validate the prior step.
func TestAntiSycophancyInAllPrompts(t *testing.T) {
	markers := []string{
		"ANTI-SYCOPHANCY",
		"INDEPENDENT reviewer",
		"flag the disagreement",
		"not to validate",
	}

	for _, persona := range AllPersonas() {
		cfg := GetPersonaConfig(persona)
		for _, m := range markers {
			if !strings.Contains(cfg.Prompt, m) {
				t.Errorf("persona %s prompt missing anti-sycophancy marker %q", persona, m)
			}
		}
		// Each prompt should also declare its review stage.
		if !strings.Contains(cfg.Prompt, "REVIEW STAGE:") {
			t.Errorf("persona %s prompt missing stage framing", persona)
		}
	}
}

// TestUnknownPersonaHasNoFabricatedPrompt ensures GetPersonaConfig does not
// invent a prompt for an unknown persona.
func TestUnknownPersonaHasNoFabricatedPrompt(t *testing.T) {
	cfg := GetPersonaConfig(PersonaType("does-not-exist"))
	if cfg.Prompt != "" {
		t.Errorf("unknown persona should have empty prompt, got %q", cfg.Prompt)
	}
}

// TestReviewTaskRunsSpecComplianceFirst verifies Stage 1 runs and is ordered
// before the Stage 2 code-quality personas in a per-task review.
func TestReviewTaskRunsSpecComplianceFirst(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	testFile := filepath.Join(tmpDir, "feature.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc Feature() {}\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	task := &status.Task{
		ID:           "task-1",
		Description:  "implement authentication feature",
		Deliverables: []string{"feature.go"},
	}

	result, err := engine.ReviewTask(task)
	if err != nil {
		t.Fatalf("ReviewTask failed: %v", err)
	}

	if len(result.PersonaResults) == 0 {
		t.Fatal("expected persona results")
	}
	if result.PersonaResults[0].Persona != PersonaSpecCompliance {
		t.Errorf("Stage 1 spec compliance should run first, got %s", result.PersonaResults[0].Persona)
	}
	if !hasPersona(result.PersonaResults, PersonaSpecCompliance) {
		t.Error("per-task review should include spec-compliance persona")
	}
}

// TestSpecComplianceBlocksOnUnmetRequirement verifies a not-implemented marker
// surfaces as a blocking spec-compliance issue — the right thing must be built
// before code quality matters.
func TestSpecComplianceBlocksOnUnmetRequirement(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	code := "package main\n\nfunc Login() error {\n\tpanic(\"not implemented\")\n}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "login.go"), []byte(code), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	task := &status.Task{
		ID:           "task-spec",
		Description:  "implement login per spec",
		Deliverables: []string{"login.go"},
	}

	result, err := engine.ReviewTask(task)
	if err != nil {
		t.Fatalf("ReviewTask failed: %v", err)
	}

	var spec *PersonaResult
	for i := range result.PersonaResults {
		if result.PersonaResults[i].Persona == PersonaSpecCompliance {
			spec = &result.PersonaResults[i]
			break
		}
	}
	if spec == nil {
		t.Fatal("expected a spec-compliance persona result")
	}
	if len(spec.Issues) == 0 {
		t.Error("spec-compliance should flag the unimplemented requirement")
	}
	if result.Metrics.SpecComplianceScore >= 100.0 {
		t.Errorf("spec-compliance score should drop below 100, got %.1f", result.Metrics.SpecComplianceScore)
	}
}
