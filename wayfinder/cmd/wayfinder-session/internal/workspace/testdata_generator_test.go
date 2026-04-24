package workspace

import (
	"testing"
)

func TestGenerateTestData(t *testing.T) {
	// Clear WORKSPACE env var to ensure path-based detection is used
	t.Setenv("WORKSPACE", "")

	testRoot := t.TempDir()

	config := TestDataConfig{
		RootDir:           testRoot,
		OSSProjects:       3,
		AcmeProjects:      3,
		IncludePhaseFiles: true,
	}

	// Generate test data
	if err := GenerateTestData(config); err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	// Validate test data
	result, err := ValidateTestData(config)
	if err != nil {
		t.Fatalf("Failed to validate test data: %v", err)
	}

	// Check for violations
	if !result.IsValid {
		t.Error("Test data validation failed:")
		for _, violation := range result.Violations {
			t.Errorf("  - %s", violation)
		}
	}

	// Verify project counts
	if len(result.OSSProjects) != config.OSSProjects {
		t.Errorf("Expected %d OSS projects, got %d", config.OSSProjects, len(result.OSSProjects))
	}

	if len(result.AcmeProjects) != config.AcmeProjects {
		t.Errorf("Expected %d Acme projects, got %d", config.AcmeProjects, len(result.AcmeProjects))
	}

	// Verify workspace isolation
	for _, project := range result.OSSProjects {
		if project.Workspace != "oss" {
			t.Errorf("OSS project has wrong workspace: %s", project.Workspace)
		}
	}

	for _, project := range result.AcmeProjects {
		if project.Workspace != "acme" {
			t.Errorf("Acme project has wrong workspace: %s", project.Workspace)
		}
	}

	// Verify different phases
	phases := make(map[string]bool)
	for _, project := range result.OSSProjects {
		phases[project.CurrentPhase] = true
	}
	for _, project := range result.AcmeProjects {
		phases[project.CurrentPhase] = true
	}

	if len(phases) < 2 {
		t.Error("Expected projects to be in different phases for better testing")
	}

	t.Logf("Successfully generated and validated test data:")
	t.Logf("  - OSS projects: %d", len(result.OSSProjects))
	t.Logf("  - Acme projects: %d", len(result.AcmeProjects))
	t.Logf("  - Unique phases: %d", len(phases))
}

func TestValidateTestDataFailures(t *testing.T) {
	// Clear WORKSPACE env var to ensure path-based detection is used
	t.Setenv("WORKSPACE", "")

	testRoot := t.TempDir()

	config := TestDataConfig{
		RootDir:           testRoot,
		OSSProjects:       2,
		AcmeProjects:      2,
		IncludePhaseFiles: false,
	}

	// Generate test data
	if err := GenerateTestData(config); err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	// Validate with wrong expected counts
	wrongConfig := config
	wrongConfig.OSSProjects = 5  // Wrong count
	wrongConfig.AcmeProjects = 1 // Wrong count

	result, err := ValidateTestData(wrongConfig)
	if err != nil {
		t.Fatalf("Failed to validate test data: %v", err)
	}

	// Should have violations
	if result.IsValid {
		t.Error("Expected validation to fail with wrong counts")
	}

	if len(result.Violations) == 0 {
		t.Error("Expected violations to be reported")
	}

	t.Logf("Correctly detected %d validation violations", len(result.Violations))
}
