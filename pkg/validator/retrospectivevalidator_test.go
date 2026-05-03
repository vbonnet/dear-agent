package validator

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTempRetrospective creates a temporary retrospective file for testing.
func createTempRetrospective(t *testing.T, content string) string {
	t.Helper()
	tmpfile, err := os.CreateTemp(t.TempDir(), "retrospective-*.md")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpfile.Name()) })

	_, err = tmpfile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	return tmpfile.Name()
}

// TestValidRetrospective tests validation of a complete valid retrospective.
func TestValidRetrospective(t *testing.T) {
	content := `# Test Retrospective

## Valid retrospective with all required fields

` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test Project",
  "metadata": {
    "date_range": "2025-01-01 to 2025-01-31",
    "team_members": ["Alice", "Bob"],
    "total_duration": "30 days",
    "phases_completed": ["D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10"]
  },
  "phases": {
    "D1": {
      "scores": {"clarity": 3, "completeness": 4, "efficiency": 3, "quality": 4, "impact": 4, "overall": 3.6},
      "reasoning": {"clarity": "Clear objectives and well-defined scope", "completeness": "All artifacts delivered", "efficiency": "Completed on time", "quality": "High quality output", "impact": "Positive downstream effects"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear problem statement. Step 2: Improvements - Could add more examples. Step 3: Assessment - Met all clarity criteria. Step 4: Calibration - Closest to 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - All required artifacts present. Step 2: Improvements - Minor documentation gaps. Step 3: Assessment - Comprehensive deliverables. Step 4: Calibration - 4/5 example match. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Completed in planned time. Step 2: Improvements - Some rework needed. Step 3: Assessment - Reasonable efficiency. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "quality": "Step 1: Review - High quality deliverables. Step 2: Improvements - Minor quality issues. Step 3: Assessment - Strong execution. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Prevented downstream issues. Step 2: Improvements - Could have more impact. Step 3: Assessment - Positive effects. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "D2": {
      "scores": {"clarity": 3, "completeness": 3, "efficiency": 4, "quality": 3, "impact": 4, "overall": 3.4},
      "reasoning": {"clarity": "Solutions identified clearly", "completeness": "Comprehensive analysis", "efficiency": "Efficient research", "quality": "Good quality work", "impact": "Enabled good decisions"},
      "cot_analysis": {"clarity": "Step 1: Review - Solutions well documented. Step 2: Improvements - Some ambiguity in requirements. Step 3: Assessment - Generally clear. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "completeness": "Step 1: Review - All options evaluated. Step 2: Improvements - Minor gaps in analysis. Step 3: Assessment - Thorough coverage. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Fast research cycle. Step 2: Improvements - Could optimize search. Step 3: Assessment - Good time management. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - Solid analysis quality. Step 2: Improvements - Some gaps in data. Step 3: Assessment - Adequate quality. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "impact": "Step 1: Review - Good foundation for decisions. Step 2: Improvements - Could have more depth. Step 3: Assessment - Positive impact. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "D3": {
      "scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 4, "impact": 4, "overall": 3.4},
      "reasoning": {"clarity": "Decision criteria clear", "completeness": "Most requirements covered", "efficiency": "Quick decision process", "quality": "Well-structured approach", "impact": "Prevented major technical debt"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear decision rationale. Step 2: Improvements - Could add more details. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - Key decisions made. Step 2: Improvements - Some edge cases missed. Step 3: Assessment - Mostly complete. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "efficiency": "Step 1: Review - Fast decision cycle. Step 2: Improvements - Minor delays in review. Step 3: Assessment - Good efficiency. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - High quality decision making. Step 2: Improvements - Could use more data. Step 3: Assessment - Strong execution. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Prevented major issues and technical debt. Step 2: Improvements - Excellent foresight shown. Step 3: Assessment - Exceptional impact on project success. Step 4: Calibration - 5/5 example perfect match. Step 5: Score - 5/5 for preventing 20+ hours of wasted effort"},
      "root_cause": []
    },
    "D4": {
      "scores": {"clarity": 4, "completeness": 4, "efficiency": 3, "quality": 4, "impact": 4, "overall": 3.8},
      "reasoning": {"clarity": "Requirements well defined", "completeness": "All requirements documented", "efficiency": "Reasonable time investment", "quality": "High quality specs", "impact": "Good foundation for implementation"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear requirement specs. Step 2: Improvements - Some clarifications needed. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - Comprehensive requirements. Step 2: Improvements - Minor gaps in edge cases. Step 3: Assessment - Thorough coverage. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Took longer than expected. Step 2: Improvements - Could streamline process. Step 3: Assessment - Adequate efficiency. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "quality": "Step 1: Review - Well-structured requirements. Step 2: Improvements - Minor quality issues. Step 3: Assessment - High quality output. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Enabled smooth implementation. Step 2: Improvements - Could have more detail. Step 3: Assessment - Good foundation. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "S4": {
      "scores": {"clarity": 4, "completeness": 4, "efficiency": 4, "quality": 4, "impact": 4, "overall": 4.0},
      "reasoning": {"clarity": "Stakeholders aligned", "completeness": "All stakeholders engaged", "efficiency": "Efficient alignment process", "quality": "High quality communication", "impact": "Strong buy-in achieved"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear stakeholder alignment. Step 2: Improvements - Could improve documentation. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - All stakeholders included. Step 2: Improvements - Minor gaps in feedback. Step 3: Assessment - Comprehensive engagement. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Quick alignment achieved. Step 2: Improvements - Some delays in scheduling. Step 3: Assessment - Good efficiency. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - High quality discussions. Step 2: Improvements - Could have more depth. Step 3: Assessment - Strong execution. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Strong stakeholder buy-in. Step 2: Improvements - Could have more engagement. Step 3: Assessment - Positive impact. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "S5": {
      "scores": {"clarity": 3, "completeness": 4, "efficiency": 4, "quality": 3, "impact": 4, "overall": 3.6},
      "reasoning": {"clarity": "Research goals defined", "completeness": "Thorough research conducted", "efficiency": "Efficient research process", "quality": "Good research quality", "impact": "Informed design decisions"},
      "cot_analysis": {"clarity": "Step 1: Review - Research objectives stated. Step 2: Improvements - Some ambiguity in scope. Step 3: Assessment - Generally clear. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "completeness": "Step 1: Review - Comprehensive research. Step 2: Improvements - Minor gaps in coverage. Step 3: Assessment - Thorough work. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Quick research cycle. Step 2: Improvements - Could optimize search. Step 3: Assessment - Good efficiency. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - Solid research quality. Step 2: Improvements - Some gaps in analysis. Step 3: Assessment - Adequate quality. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "impact": "Step 1: Review - Informed good decisions. Step 2: Improvements - Could have more depth. Step 3: Assessment - Positive impact. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "S6": {
      "scores": {"clarity": 4, "completeness": 4, "efficiency": 3, "quality": 4, "impact": 4, "overall": 3.8},
      "reasoning": {"clarity": "Design well documented", "completeness": "All components designed", "efficiency": "Reasonable design time", "quality": "High quality design", "impact": "Good implementation guidance"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear design documentation. Step 2: Improvements - Some clarifications needed. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - Comprehensive design. Step 2: Improvements - Minor gaps in specs. Step 3: Assessment - Thorough coverage. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Took longer than planned. Step 2: Improvements - Could streamline reviews. Step 3: Assessment - Adequate efficiency. Step 4: Calibration - 3/5 example. Step 5: Score - 3/5", "quality": "Step 1: Review - Well-structured design. Step 2: Improvements - Minor quality issues. Step 3: Assessment - High quality output. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Enabled smooth implementation. Step 2: Improvements - Could have more detail. Step 3: Assessment - Good guidance. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "S7": {
      "scores": {"clarity": 4, "completeness": 4, "efficiency": 4, "quality": 4, "impact": 4, "overall": 4.0},
      "reasoning": {"clarity": "Plan clearly structured", "completeness": "All tasks identified", "efficiency": "Efficient planning", "quality": "High quality plan", "impact": "Good execution roadmap"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear plan structure. Step 2: Improvements - Could add more milestones. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - All tasks covered. Step 2: Improvements - Minor gaps in dependencies. Step 3: Assessment - Comprehensive plan. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Quick planning cycle. Step 2: Improvements - Some delays in review. Step 3: Assessment - Good efficiency. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - Well-structured plan. Step 2: Improvements - Could have more detail. Step 3: Assessment - High quality output. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Enabled smooth execution. Step 2: Improvements - Could have more guidance. Step 3: Assessment - Good roadmap. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "S8": {
      "scores": {"clarity": 3, "completeness": 4, "efficiency": 3, "quality": 4, "impact": 4, "overall": 3.6},
      "reasoning": {"clarity": "Implementation well documented", "completeness": "All features delivered", "efficiency": "On-time delivery", "quality": "High quality code", "impact": "Delivered significant user value"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear implementation docs. Step 2: Improvements - Some clarifications needed. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - All features complete. Step 2: Improvements - Minor gaps in edge cases. Step 3: Assessment - Thorough delivery. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - On-time delivery achieved. Step 2: Improvements - Some rework needed. Step 3: Assessment - Good efficiency. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - High quality code delivered with comprehensive tests and documentation. Step 2: Improvements - Minor refactoring opportunities. Step 3: Assessment - Strong execution quality. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Delivered significant value to users and enabled future work successfully. Step 2: Improvements - Exceptional value delivered with no technical debt. Step 3: Assessment - Outstanding impact on project. Step 4: Calibration - 5/5 example perfect match. Step 5: Score - 5/5 for delivering core value and enabling future features"},
      "root_cause": []
    },
    "S9": {
      "scores": {"clarity": 4, "completeness": 4, "efficiency": 4, "quality": 4, "impact": 4, "overall": 4.0},
      "reasoning": {"clarity": "Validation criteria clear", "completeness": "All tests passed", "efficiency": "Efficient validation", "quality": "High quality testing", "impact": "High confidence in delivery"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear validation criteria. Step 2: Improvements - Could add more scenarios. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - All tests executed. Step 2: Improvements - Minor gaps in coverage. Step 3: Assessment - Comprehensive testing. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Quick validation cycle. Step 2: Improvements - Some delays in fixes. Step 3: Assessment - Good efficiency. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - High quality testing with good coverage and automation. Step 2: Improvements - Could add more edge cases. Step 3: Assessment - Strong execution. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - High confidence achieved. Step 2: Improvements - Could have more scenarios. Step 3: Assessment - Good validation. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    },
    "S10": {
      "scores": {"clarity": 4, "completeness": 4, "efficiency": 4, "quality": 4, "impact": 4, "overall": 4.0},
      "reasoning": {"clarity": "Deployment plan clear", "completeness": "All steps completed", "efficiency": "Smooth deployment", "quality": "High quality release", "impact": "Successful production launch"},
      "cot_analysis": {"clarity": "Step 1: Review - Clear deployment documentation. Step 2: Improvements - Could add more rollback details. Step 3: Assessment - Strong clarity. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "completeness": "Step 1: Review - All deployment steps done. Step 2: Improvements - Minor gaps in monitoring. Step 3: Assessment - Comprehensive deployment. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "efficiency": "Step 1: Review - Quick deployment cycle. Step 2: Improvements - Some delays in verification. Step 3: Assessment - Good efficiency. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "quality": "Step 1: Review - High quality release process with proper verification. Step 2: Improvements - Could automate more steps. Step 3: Assessment - Strong execution. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5", "impact": "Step 1: Review - Successful production launch. Step 2: Improvements - Could have more monitoring. Step 3: Assessment - Good delivery. Step 4: Calibration - 4/5 example. Step 5: Score - 4/5"},
      "root_cause": []
    }
  },
  "project_overall": {
    "score": 3.9,
    "dimension_averages": {
      "clarity": 3.8,
      "completeness": 3.9,
      "efficiency": 3.7,
      "quality": 3.8,
      "impact": 4.2
    }
  },
  "validation": {
    "outliers": {
      "iqr_outliers": 0,
      "extreme_percentage": 0.0,
      "flag_extremes": false
    },
    "consistency_checks": [],
    "cot_quality": [],
    "calibration_drift": {
      "project_overall": 3.9,
      "flag_inflated": false
    }
  }
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	errors, warnings, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)
	assert.Empty(t, errors, "Expected no validation errors for valid retrospective")
	assert.Empty(t, warnings, "Expected no validation warnings for valid retrospective")
}

// TestMissingSchemaVersion tests detection of missing schema_version field.
func TestMissingSchemaVersion(t *testing.T) {
	content := `# Test
` + "```json\n" + `{
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {},
  "project_overall": {"score": 0, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	errors, _, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)
	assert.NotEmpty(t, errors)
	found := false
	for _, e := range errors {
		if e.Field == "schema_version" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error for missing schema_version")
}

// TestMissingPhase tests detection of missing required phase.
func TestMissingPhase(t *testing.T) {
	content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {
      "scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0},
      "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"},
      "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5"},
      "root_cause": []
    }
  },
  "project_overall": {"score": 0, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	errors, _, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)
	assert.NotEmpty(t, errors)

	// Should have errors for missing phases D2, D3, D4, S4-S10
	missingPhases := []string{"D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10"}
	for _, phase := range missingPhases {
		found := false
		for _, e := range errors {
			if e.Field == phase {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected error for missing phase %s", phase)
	}
}

// TestInvalidScoreRange tests detection of scores outside 1-5 range.
func TestInvalidScoreRange(t *testing.T) {
	tests := []struct {
		name  string
		score int
	}{
		{"score too low", 0},
		{"score too high", 6},
		{"negative score", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {
      "scores": {"clarity": ` + fmt.Sprintf("%d", tt.score) + `, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0},
      "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"},
      "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"},
      "root_cause": []
    },
    "D2": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D3": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S5": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S6": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S7": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S8": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S9": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S10": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []}
  },
  "project_overall": {"score": 3.0, "dimension_averages": {"clarity": 3.0, "completeness": 3.0, "efficiency": 3.0, "quality": 3.0, "impact": 3.0}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

			filepath := createTempRetrospective(t, content)
			errors, _, err := ValidateRetrospectiveFile(filepath)

			require.NoError(t, err)
			assert.NotEmpty(t, errors)

			found := false
			for _, e := range errors {
				if e.Field == "D1.clarity" && e.Severity == "error" {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected error for invalid score in D1.clarity")
		})
	}
}

// TestShortReasoning tests detection of reasoning that's too short.
func TestShortReasoning(t *testing.T) {
	content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {
      "scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0},
      "reasoning": {"clarity": "short", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"},
      "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5"},
      "root_cause": []
    },
    "D2": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D3": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S5": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S6": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S7": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S8": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S9": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S10": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []}
  },
  "project_overall": {"score": 3.0, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	errors, _, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)
	assert.NotEmpty(t, errors)

	found := false
	for _, e := range errors {
		if e.Field == "D1.reasoning.clarity" && e.Severity == "error" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error for short reasoning in D1.clarity")
}

// TestShortCoTAnalysis tests detection of CoT analysis that's too short.
func TestShortCoTAnalysis(t *testing.T) {
	content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {
      "scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0},
      "reasoning": {"clarity": "test reason here", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"},
      "cot_analysis": {"clarity": "short", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5"},
      "root_cause": []
    },
    "D2": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D3": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S5": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S6": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S7": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S8": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S9": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S10": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []}
  },
  "project_overall": {"score": 3.0, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	errors, _, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)
	assert.NotEmpty(t, errors)

	found := false
	for _, e := range errors {
		if e.Field == "D1.cot_analysis.clarity" && e.Severity == "error" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error for short CoT analysis in D1.clarity")
}

// TestIQROutliers tests IQR outlier detection.
func TestIQROutliers(t *testing.T) {
	// Create data with many 1s and multiple 5s to trigger outlier warning (need >5 outliers)
	content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 1, "quality": 1, "impact": 5, "overall": 3.4}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 perfect", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 complete", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 1/5", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 1/5", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 excellent work"}, "root_cause": []},
    "D2": {"scores": {"clarity": 5, "completeness": 1, "efficiency": 1, "quality": 1, "impact": 1, "overall": 1.8}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D3": {"scores": {"clarity": 1, "completeness": 5, "efficiency": 1, "quality": 1, "impact": 1, "overall": 1.8}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D4": {"scores": {"clarity": 1, "completeness": 1, "efficiency": 5, "quality": 1, "impact": 1, "overall": 1.8}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S4": {"scores": {"clarity": 1, "completeness": 1, "efficiency": 1, "quality": 5, "impact": 1, "overall": 1.8}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S5": {"scores": {"clarity": 1, "completeness": 1, "efficiency": 1, "quality": 1, "impact": 5, "overall": 1.8}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S6": {"scores": {"clarity": 5, "completeness": 1, "efficiency": 1, "quality": 1, "impact": 1, "overall": 1.8}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S7": {"scores": {"clarity": 1, "completeness": 1, "efficiency": 1, "quality": 1, "impact": 1, "overall": 1.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S8": {"scores": {"clarity": 1, "completeness": 1, "efficiency": 1, "quality": 1, "impact": 1, "overall": 1.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S9": {"scores": {"clarity": 1, "completeness": 1, "efficiency": 1, "quality": 1, "impact": 1, "overall": 1.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S10": {"scores": {"clarity": 1, "completeness": 1, "efficiency": 1, "quality": 1, "impact": 1, "overall": 1.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []}
  },
  "project_overall": {"score": 1.1, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	_, warnings, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)

	// With mostly 1s and one 5, we should get outlier warning
	found := false
	for _, w := range warnings {
		if w.Field == "iqr_outliers" && w.Severity == "warning" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected warning for IQR outliers")
}

// TestExtremePercentage tests extreme percentage detection.
func TestExtremePercentage(t *testing.T) {
	// Create data with >80% extreme scores (all 5s)
	content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 perfect work here", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 complete work", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 efficient", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 high quality", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 great impact"}, "root_cause": []},
    "D2": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 perfect", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 complete", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 efficient", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 quality", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 impact"}, "root_cause": []},
    "D3": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "D4": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S4": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S5": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S6": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S7": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S8": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S9": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S10": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []}
  },
  "project_overall": {"score": 5.0, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	_, warnings, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)

	// Should have warning for extreme percentage
	found := false
	for _, w := range warnings {
		if w.Field == "extreme_percentage" && w.Severity == "warning" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected warning for extreme percentage")
}

// TestCoTQualityWarning tests detection of 5/5 scores with shallow CoT.
func TestCoTQualityWarning(t *testing.T) {
	content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {
      "scores": {"clarity": 5, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.4},
      "reasoning": {"clarity": "test reason here", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"},
      "cot_analysis": {"clarity": "Short CoT analysis text here", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 3/5"},
      "root_cause": []
    },
    "D2": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D3": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "D4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S4": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S5": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S6": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S7": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S8": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S9": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []},
    "S10": {"scores": {"clarity": 3, "completeness": 3, "efficiency": 3, "quality": 3, "impact": 3, "overall": 3.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: score"}, "root_cause": []}
  },
  "project_overall": {"score": 3.0, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	errors, warnings, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)

	// Should have error for short CoT (< 50 chars) AND warning for 5/5 with <75 chars
	foundError := false
	foundWarning := false
	for _, e := range errors {
		if e.Field == "D1.cot_analysis.clarity" && e.Severity == "error" {
			foundError = true
		}
	}
	for _, w := range warnings {
		if w.Field == "D1.clarity" && w.Severity == "warning" {
			foundWarning = true
		}
	}
	assert.True(t, foundError, "Expected error for CoT <50 chars")
	assert.True(t, foundWarning, "Expected warning for 5/5 score with CoT <75 chars")
}

// TestCalibrationDrift tests detection of calibration drift (project overall >4.5).
func TestCalibrationDrift(t *testing.T) {
	content := `# Test
` + "```json\n" + `{
  "schema_version": "1.0.0",
  "project": "Test",
  "metadata": {"date_range": "test", "team_members": [], "total_duration": "1d", "phases_completed": []},
  "phases": {
    "D1": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 perfect work", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 complete", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 efficient", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 quality work", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 great impact"}, "root_cause": []},
    "D2": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test reason", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "D3": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "D4": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 4, "quality": 5, "impact": 5, "overall": 4.8}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 4/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S4": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S5": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S6": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S7": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S8": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S9": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []},
    "S10": {"scores": {"clarity": 5, "completeness": 5, "efficiency": 5, "quality": 5, "impact": 5, "overall": 5.0}, "reasoning": {"clarity": "test", "completeness": "test", "efficiency": "test", "quality": "test", "impact": "test"}, "cot_analysis": {"clarity": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "completeness": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "efficiency": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "quality": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score", "impact": "Step 1: test Step 2: test Step 3: test Step 4: test Step 5: 5/5 score"}, "root_cause": []}
  },
  "project_overall": {"score": 4.9, "dimension_averages": {}},
  "validation": {"outliers": {}, "consistency_checks": [], "cot_quality": [], "calibration_drift": {}}
}
` + "```\n"

	filepath := createTempRetrospective(t, content)
	_, warnings, err := ValidateRetrospectiveFile(filepath)

	require.NoError(t, err)

	// Should have warning for calibration drift
	found := false
	for _, w := range warnings {
		if w.Field == "project_overall" && w.Severity == "warning" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected warning for calibration drift (score >4.5)")
}

// TestNoJSONBlock tests handling of markdown without JSON block.
func TestNoJSONBlock(t *testing.T) {
	content := `# Test Retrospective

This is a retrospective without a JSON block.
`

	filepath := createTempRetrospective(t, content)
	_, _, err := ValidateRetrospectiveFile(filepath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no JSON block found")
}

// TestInvalidJSON tests handling of invalid JSON.
func TestInvalidJSON(t *testing.T) {
	content := `# Test
` + "```json\n" + `{
  "invalid": "json",
  "missing": "closing brace"
` + "```\n"

	filepath := createTempRetrospective(t, content)
	_, _, err := ValidateRetrospectiveFile(filepath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

// TestTableDrivenValidation demonstrates table-driven test pattern for validation rules.
func TestTableDrivenValidation(t *testing.T) {
	tests := []struct {
		name          string
		jsonContent   string
		expectError   bool
		expectWarning bool
		errorField    string
		warningField  string
	}{
		{
			name:        "missing project field",
			jsonContent: `{"schema_version":"1.0.0","metadata":{},"phases":{},"project_overall":{},"validation":{}}`,
			expectError: true,
			errorField:  "project",
		},
		{
			name:        "empty schema_version",
			jsonContent: `{"schema_version":"","project":"Test","metadata":{},"phases":{},"project_overall":{},"validation":{}}`,
			expectError: true,
			errorField:  "schema_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "# Test\n```json\n" + tt.jsonContent + "\n```\n"
			filepath := createTempRetrospective(t, content)
			errors, warnings, err := ValidateRetrospectiveFile(filepath)

			require.NoError(t, err)

			if tt.expectError {
				assert.NotEmpty(t, errors)
				if tt.errorField != "" {
					found := false
					for _, e := range errors {
						if e.Field == tt.errorField {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected error for field %s", tt.errorField)
				}
			}

			if tt.expectWarning {
				assert.NotEmpty(t, warnings)
				if tt.warningField != "" {
					found := false
					for _, w := range warnings {
						if w.Field == tt.warningField {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected warning for field %s", tt.warningField)
				}
			}
		})
	}
}
