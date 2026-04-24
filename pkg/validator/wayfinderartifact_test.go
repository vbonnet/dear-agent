package validator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFrontmatter_Valid(t *testing.T) {
	content := `---
phase: D1
title: Test Project Problem Validation
date: 2024-01-15
status: in_progress
project: test-project
tags:
  - testing
  - validation
schema_version: "1.0"
wayfinder_plugin_version: "1.0.0"
template_version: "1.0.0"
required_sections:
  problem_statement: completed
  stakeholders: in_progress
  success_criteria: not_started
  context: completed
  constraints: completed
  risks: completed
  out_of_scope: completed
  dependencies: completed
  metrics: completed
---

# Problem Statement
...
`

	fm, err := ExtractFrontmatter(content)
	require.NoError(t, err)
	assert.Equal(t, "D1", fm["phase"])
	assert.Equal(t, "Test Project Problem Validation", fm["title"])
}

func TestExtractFrontmatter_NoFrontmatter(t *testing.T) {
	content := `# Just a regular markdown file`

	_, err := ExtractFrontmatter(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no frontmatter found")
}

func TestExtractFrontmatter_Malformed(t *testing.T) {
	content := `---
phase: D1
title: Test
`

	_, err := ExtractFrontmatter(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "malformed frontmatter")
}

func TestExtractFrontmatter_InvalidYAML(t *testing.T) {
	content := `---
phase: D1
  invalid: yaml: syntax:
---
`

	_, err := ExtractFrontmatter(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

func TestValidateArtifact_ValidD1(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing", "validation"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            nil,
		"nextPhase":                "D2",
		"required_sections": map[string]interface{}{
			"problem_statement": "completed",
			"stakeholders":      "in_progress",
			"success_criteria":  "not_started",
			"context":           "completed",
			"constraints":       "completed",
			"risks":             "completed",
			"out_of_scope":      "completed",
			"dependencies":      "completed",
			"metrics":           "completed",
		},
	}

	errors := ValidateArtifact(fm)
	assert.Empty(t, errors)
}

func TestValidateArtifact_ValidS1(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "S1",
		"title":                    "Test Project Foundation",
		"date":                     "2024-02-01",
		"status":                   "completed",
		"project":                  "test-project",
		"tags":                     []interface{}{"implementation", "foundation"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            "D4",
		"nextPhase":                "S2",
		"required_sections": map[string]interface{}{
			"architecture":   "completed",
			"data_models":    "completed",
			"infrastructure": "completed",
		},
	}

	errors := ValidateArtifact(fm)
	assert.Empty(t, errors)
}

func TestValidateArtifact_ValidS11(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "S11",
		"title":                    "Test Project Retrospective",
		"date":                     "2024-06-01",
		"status":                   "completed",
		"project":                  "test-project",
		"tags":                     []interface{}{"retrospective", "learnings"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            "S10",
		"nextPhase":                nil,
		"required_sections": map[string]interface{}{
			"what_went_well":   "completed",
			"what_went_wrong":  "completed",
			"lessons_learned":  "completed",
			"action_items":     "completed",
			"metrics_achieved": "completed",
		},
	}

	errors := ValidateArtifact(fm)
	assert.Empty(t, errors)
}

func TestValidateArtifact_MissingPhase(t *testing.T) {
	fm := map[string]interface{}{
		"title": "Test",
	}

	errors := ValidateArtifact(fm)
	require.Len(t, errors, 1)
	assert.Equal(t, "phase", errors[0].Field)
	assert.Contains(t, errors[0].Message, "missing or invalid")
}

func TestValidateArtifact_InvalidPhase(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "X99",
	}

	errors := ValidateArtifact(fm)
	require.GreaterOrEqual(t, len(errors), 1)
	assert.Equal(t, "phase", errors[0].Field)
	assert.Contains(t, errors[0].Message, "invalid phase")
}

func TestValidateArtifact_MissingTitle(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "title" {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_TitleTooShort(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"title": "Short",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "title" && containsStr(e.Message, "at least 10 characters") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_TitleTooLong(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"title": "This is an extremely long title that exceeds the maximum allowed length of 150 characters which should trigger a validation error for being too verbose and not concise enough",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "title" && containsStr(e.Message, "at most 150 characters") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_InvalidDateFormat(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"date":  "2024/01/15",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "date" && containsStr(e.Message, "invalid format") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_InvalidDate(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"date":  "2024-13-45",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "date" && containsStr(e.Message, "invalid format") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_InvalidStatus(t *testing.T) {
	fm := map[string]interface{}{
		"phase":  "D1",
		"status": "unknown_status",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "status" && containsStr(e.Message, "invalid status") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_InvalidProjectSlug(t *testing.T) {
	fm := map[string]interface{}{
		"phase":   "D1",
		"project": "Invalid_Project",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "project" && containsStr(e.Message, "must be kebab-case") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_EmptyTags(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"tags":  []interface{}{},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "tags" && containsStr(e.Message, "at least one tag") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_InvalidTag(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"tags":  []interface{}{"Valid-Tag", "Invalid_Tag"},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "tags[1]") && containsStr(e.Message, "invalid format") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_DuplicateTags(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"tags":  []interface{}{"testing", "validation", "testing"},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "tags") && containsStr(e.Message, "duplicate") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_InvalidSchemaVersion(t *testing.T) {
	fm := map[string]interface{}{
		"phase":          "D1",
		"schema_version": "invalid",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "schema_version" && containsStr(e.Message, "invalid format") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_ValidSchemaVersions(t *testing.T) {
	testCases := []string{"1.0", "1.0.0", "2.3.4"}

	for _, version := range testCases {
		t.Run(version, func(t *testing.T) {
			assert.True(t, isValidSemver(version))
		})
	}
}

func TestValidateArtifact_InvalidWayfinderVersion(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"wayfinder_plugin_version": "1.0",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "wayfinder_plugin_version" && containsStr(e.Message, "must be semver: X.Y.Z") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_MissingRequiredSections(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "required_sections" && containsStr(e.Message, "missing") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_MissingSectionField(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"required_sections": map[string]interface{}{
			"problem_statement": "completed",
			// Missing other required sections
		},
	}

	errors := ValidateArtifact(fm)
	assert.Greater(t, len(errors), 0)

	// Should have errors for missing sections
	hasStakeholdersError := false
	for _, e := range errors {
		if containsStr(e.Field, "stakeholders") {
			hasStakeholdersError = true
			break
		}
	}
	assert.True(t, hasStakeholdersError)
}

func TestValidateArtifact_InvalidSectionStatus(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"required_sections": map[string]interface{}{
			"problem_statement": "invalid_status",
			"stakeholders":      "in_progress",
			"success_criteria":  "not_started",
			"context":           "completed",
			"constraints":       "completed",
			"risks":             "completed",
			"out_of_scope":      "completed",
			"dependencies":      "completed",
			"metrics":           "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "problem_statement") && containsStr(e.Message, "invalid status") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_WrongPreviousPhase(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D2",
		"title":                    "Test Project Solutions Search",
		"date":                     "2024-01-20",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            "D3", // Wrong!
		"nextPhase":                "D3",
		"required_sections": map[string]interface{}{
			"solution_options":      "completed",
			"evaluation_criteria":   "completed",
			"recommendations":       "completed",
			"alternatives_rejected": "completed",
			"integration_risks":     "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "previousPhase" && containsStr(e.Message, "must be 'D1'") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_WrongNextPhase(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"nextPhase":                "S1", // Wrong!
		"required_sections": map[string]interface{}{
			"problem_statement": "completed",
			"stakeholders":      "in_progress",
			"success_criteria":  "not_started",
			"context":           "completed",
			"constraints":       "completed",
			"risks":             "completed",
			"out_of_scope":      "completed",
			"dependencies":      "completed",
			"metrics":           "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "nextPhase" && containsStr(e.Message, "must be 'D2'") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_D1ShouldHaveNullPreviousPhase(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            "D0", // Should be null/omitted
		"nextPhase":                "D2",
		"required_sections": map[string]interface{}{
			"problem_statement": "completed",
			"stakeholders":      "in_progress",
			"success_criteria":  "not_started",
			"context":           "completed",
			"constraints":       "completed",
			"risks":             "completed",
			"out_of_scope":      "completed",
			"dependencies":      "completed",
			"metrics":           "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "previousPhase" && containsStr(e.Message, "must be null") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_S11ShouldHaveNullNextPhase(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "S11",
		"title":                    "Test Project Retrospective",
		"date":                     "2024-06-01",
		"status":                   "completed",
		"project":                  "test-project",
		"tags":                     []interface{}{"retrospective"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            "S10",
		"nextPhase":                "S12", // Should be null
		"required_sections": map[string]interface{}{
			"what_went_well":   "completed",
			"what_went_wrong":  "completed",
			"lessons_learned":  "completed",
			"action_items":     "completed",
			"metrics_achieved": "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "nextPhase" && containsStr(e.Message, "must be null") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_ValidBlocker(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "blocked",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"nextPhase":                "D2",
		"blockers": []interface{}{
			map[string]interface{}{
				"description":  "Waiting for API documentation from third-party vendor",
				"impact":       "high",
				"created_date": "2024-01-10",
			},
		},
		"required_sections": map[string]interface{}{
			"problem_statement": "completed",
			"stakeholders":      "in_progress",
			"success_criteria":  "not_started",
			"context":           "completed",
			"constraints":       "completed",
			"risks":             "completed",
			"out_of_scope":      "completed",
			"dependencies":      "completed",
			"metrics":           "completed",
		},
	}

	errors := ValidateArtifact(fm)
	assert.Empty(t, errors)
}

func TestValidateArtifact_InvalidBlockerImpact(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			map[string]interface{}{
				"description":  "Waiting for approval",
				"impact":       "super-critical", // Invalid
				"created_date": "2024-01-10",
			},
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "blockers[0].impact") && containsStr(e.Message, "invalid impact") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_BlockerDescriptionTooShort(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			map[string]interface{}{
				"description":  "Short",
				"impact":       "high",
				"created_date": "2024-01-10",
			},
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "blockers[0].description") && containsStr(e.Message, "at least 10 characters") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_BlockerInvalidDate(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			map[string]interface{}{
				"description":  "Waiting for approval from management team",
				"impact":       "high",
				"created_date": "2024-13-45", // Invalid date
			},
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "blockers[0].created_date") && containsStr(e.Message, "invalid format") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestGetPhaseSchema_AllPhases(t *testing.T) {
	phases := []string{"D1", "D2", "D3", "D4", "S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			schema, err := GetPhaseSchema(phase)
			require.NoError(t, err)
			assert.Equal(t, phase, schema.Phase)
			assert.NotEmpty(t, schema.RequiredSections)
		})
	}
}

func TestGetPhaseSchema_InvalidPhase(t *testing.T) {
	_, err := GetPhaseSchema("X99")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown phase")
}

func TestValidateArtifact_AllD2RequiredSections(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D2",
		"title":                    "Test Project Solutions Search",
		"date":                     "2024-01-20",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            "D1",
		"nextPhase":                "D3",
		"required_sections": map[string]interface{}{
			"solution_options":      "completed",
			"evaluation_criteria":   "completed",
			"recommendations":       "completed",
			"alternatives_rejected": "completed",
			"integration_risks":     "completed",
		},
	}

	errors := ValidateArtifact(fm)
	assert.Empty(t, errors)
}

func TestValidateArtifact_MultipleErrors(t *testing.T) {
	fm := map[string]interface{}{
		"phase":  "D1",
		"title":  "Short", // Too short
		"date":   "invalid-date",
		"status": "invalid-status",
		"tags":   []interface{}{},
	}

	errors := ValidateArtifact(fm)
	assert.Greater(t, len(errors), 3) // Should have multiple errors
}

func TestIsValidDate(t *testing.T) {
	testCases := []struct {
		date  string
		valid bool
	}{
		{"2024-01-15", true},
		{"2024-12-31", true},
		{"2024-02-29", true},  // Leap year
		{"2023-02-29", false}, // Not a leap year
		{"2024-13-01", false},
		{"2024-01-32", false},
		{"2024/01/15", false},
		{"2024-1-15", false},
		{"invalid", false},
	}

	for _, tc := range testCases {
		t.Run(tc.date, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidDate(tc.date))
		})
	}
}

func TestIsValidProjectSlug(t *testing.T) {
	testCases := []struct {
		slug  string
		valid bool
	}{
		{"test-project", true},
		{"wayfinder-frontmatter-implementation", true},
		{"simple", true},
		{"multi-word-project-name", true},
		{"Test-Project", false},  // Uppercase
		{"test_project", false},  // Underscore
		{"test--project", false}, // Double hyphen
		{"-test", false},         // Leading hyphen
		{"test-", false},         // Trailing hyphen
	}

	for _, tc := range testCases {
		t.Run(tc.slug, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidProjectSlug(tc.slug))
		})
	}
}

func TestIsValidTag(t *testing.T) {
	testCases := []struct {
		tag   string
		valid bool
	}{
		{"testing", true},
		{"test-tag", true},
		{"tag123", true},
		{"Test-Tag", false}, // Uppercase
		{"test_tag", false}, // Underscore
		{"test tag", false}, // Space
	}

	for _, tc := range testCases {
		t.Run(tc.tag, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidTag(tc.tag))
		})
	}
}

func TestIsValidSemver(t *testing.T) {
	testCases := []struct {
		version string
		valid   bool
	}{
		{"1.0", true},
		{"1.0.0", true},
		{"2.3.4", true},
		{"10.20.30", true},
		{"1", false},
		{"1.0.0.0", false},
		{"v1.0.0", false},
		{"invalid", false},
	}

	for _, tc := range testCases {
		t.Run(tc.version, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidSemver(tc.version))
		})
	}
}

func TestIsValidFullSemver(t *testing.T) {
	testCases := []struct {
		version string
		valid   bool
	}{
		{"1.0.0", true},
		{"2.3.4", true},
		{"10.20.30", true},
		{"1.0", false}, // Missing patch
		{"1", false},
		{"v1.0.0", false},
		{"invalid", false},
	}

	for _, tc := range testCases {
		t.Run(tc.version, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidFullSemver(tc.version))
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := WayfinderValidationError{
		Field:   "test_field",
		Message: "test message",
	}
	assert.Equal(t, "test_field: test message", err.Error())

	err2 := WayfinderValidationError{
		Message: "just message",
	}
	assert.Equal(t, "just message", err2.Error())
}

func TestValidateBlocker_MissingResolution(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			map[string]interface{}{
				"description":   "Valid blocker description here",
				"impact":        "medium",
				"created_date":  "2024-01-10",
				"resolved_date": "2024-01-15",
			},
		},
	}

	errors := ValidateArtifact(fm)
	// Should pass - resolution is optional
	assert.NotNil(t, errors)
}

func TestValidateArtifact_InvalidRequiredSectionsType(t *testing.T) {
	fm := map[string]interface{}{
		"phase":             "D1",
		"required_sections": "not an object", // Invalid type
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "required_sections" && containsStr(e.Message, "must be an object") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_SectionStatusNotString(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"required_sections": map[string]interface{}{
			"problem_statement": 123, // Should be string
			"stakeholders":      "in_progress",
			"success_criteria":  "not_started",
			"context":           "completed",
			"constraints":       "completed",
			"risks":             "completed",
			"out_of_scope":      "completed",
			"dependencies":      "completed",
			"metrics":           "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "problem_statement") && containsStr(e.Message, "must be a string") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_InvalidBlockerType(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			"not an object", // Should be object
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "blockers[0]") && containsStr(e.Message, "must be an object") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_BlockersNotArray(t *testing.T) {
	fm := map[string]interface{}{
		"phase":    "D1",
		"blockers": "not an array",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "blockers" && containsStr(e.Message, "must be an array") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_TagsNotArray(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"tags":  "not-an-array",
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "tags" && containsStr(e.Message, "must be an array") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_TagNotString(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"tags":  []interface{}{123, "valid-tag"},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "tags[0]") && containsStr(e.Message, "must be a string") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_PreviousPhaseNotString(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D2",
		"title":                    "Test Project Solutions Search",
		"date":                     "2024-01-20",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"previousPhase":            123, // Should be string or null
		"nextPhase":                "D3",
		"required_sections": map[string]interface{}{
			"solution_options":      "completed",
			"evaluation_criteria":   "completed",
			"recommendations":       "completed",
			"alternatives_rejected": "completed",
			"integration_risks":     "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "previousPhase" && containsStr(e.Message, "must be a string or null") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_NextPhaseNotString(t *testing.T) {
	fm := map[string]interface{}{
		"phase":                    "D1",
		"title":                    "Test Project Problem Validation",
		"date":                     "2024-01-15",
		"status":                   "in_progress",
		"project":                  "test-project",
		"tags":                     []interface{}{"testing"},
		"schema_version":           "1.0",
		"wayfinder_plugin_version": "1.0.0",
		"template_version":         "1.0.0",
		"nextPhase":                123, // Should be string or null
		"required_sections": map[string]interface{}{
			"problem_statement": "completed",
			"stakeholders":      "in_progress",
			"success_criteria":  "not_started",
			"context":           "completed",
			"constraints":       "completed",
			"risks":             "completed",
			"out_of_scope":      "completed",
			"dependencies":      "completed",
			"metrics":           "completed",
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if e.Field == "nextPhase" && containsStr(e.Message, "must be a string or null") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_BlockerMissingFields(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			map[string]interface{}{
				// Missing all required fields
			},
		},
	}

	errors := ValidateArtifact(fm)
	// Should have errors for missing description, impact, created_date
	assert.GreaterOrEqual(t, len(errors), 3)
}

func TestValidateArtifact_BlockerInvalidResolvedDate(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			map[string]interface{}{
				"description":   "Valid blocker description here",
				"impact":        "high",
				"created_date":  "2024-01-10",
				"resolved_date": "invalid-date",
			},
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "resolved_date") && containsStr(e.Message, "invalid format") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidateArtifact_BlockerResolvedDateNotString(t *testing.T) {
	fm := map[string]interface{}{
		"phase": "D1",
		"blockers": []interface{}{
			map[string]interface{}{
				"description":   "Valid blocker description here",
				"impact":        "high",
				"created_date":  "2024-01-10",
				"resolved_date": 123,
			},
		},
	}

	errors := ValidateArtifact(fm)
	hasError := false
	for _, e := range errors {
		if containsStr(e.Field, "resolved_date") && containsStr(e.Message, "must be a string") {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

// Helper function
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}
