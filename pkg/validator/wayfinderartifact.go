// Package validator provides validation for Wayfinder artifacts.
package validator

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CoreFrontmatter represents the base frontmatter fields required for all phases.
type CoreFrontmatter struct {
	Phase                  string    `yaml:"phase"`
	Title                  string    `yaml:"title"`
	Date                   string    `yaml:"date"`
	Status                 string    `yaml:"status"`
	Project                string    `yaml:"project"`
	Tags                   []string  `yaml:"tags"`
	SchemaVersion          string    `yaml:"schema_version"`
	PreviousPhase          *string   `yaml:"previousPhase,omitempty"`
	NextPhase              *string   `yaml:"nextPhase,omitempty"`
	Blockers               []Blocker `yaml:"blockers,omitempty"`
	WayfinderPluginVersion string    `yaml:"wayfinder_plugin_version"`
	TemplateVersion        string    `yaml:"template_version"`
}

// Blocker represents a project blocker.
type Blocker struct {
	Description  string  `yaml:"description"`
	Impact       string  `yaml:"impact"`
	CreatedDate  string  `yaml:"created_date"`
	Resolution   *string `yaml:"resolution,omitempty"`
	ResolvedDate *string `yaml:"resolved_date,omitempty"`
}

// RequiredSections holds section completion statuses.
type RequiredSections map[string]string

// WayfinderValidationError represents a validation error with context.
type WayfinderValidationError struct {
	Field   string
	Message string
}

func (e WayfinderValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidPhases defines all valid Wayfinder phase identifiers.
var ValidPhases = []string{
	"D1", "D2", "D3", "D4",
	"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11",
}

// ValidStatuses defines valid phase status values.
var ValidStatuses = []string{"not_started", "in_progress", "blocked", "completed", "skipped"}

// ValidSectionStatuses defines valid section status values.
var ValidSectionStatuses = []string{"not_started", "in_progress", "completed", "skipped"}

// ValidImpacts defines valid blocker impact levels.
var ValidImpacts = []string{"critical", "high", "medium", "low"}

// PhaseSchema defines the validation rules for a specific phase.
type PhaseSchema struct {
	Phase            string
	RequiredSections []string
	PreviousPhase    *string
	NextPhase        *string
	OptionalFields   []string
}

// GetPhaseSchema returns the schema definition for a given phase.
func GetPhaseSchema(phase string) (*PhaseSchema, error) {
	schemas := map[string]PhaseSchema{
		// Design Phases
		"D1": {
			Phase: "D1",
			RequiredSections: []string{
				"problem_statement", "stakeholders", "success_criteria",
				"context", "constraints", "risks", "out_of_scope",
				"dependencies", "metrics",
			},
			PreviousPhase: nil,
			NextPhase:     stringPtr("D2"),
			OptionalFields: []string{
				"validation.problem_validated",
				"validation.success_criteria_measurable",
				"validation.risks_identified",
			},
		},
		"D2": {
			Phase: "D2",
			RequiredSections: []string{
				"solution_options", "evaluation_criteria", "recommendations",
				"alternatives_rejected", "integration_risks",
			},
			PreviousPhase: stringPtr("D1"),
			NextPhase:     stringPtr("D3"),
			OptionalFields: []string{
				"solutions_evaluated",
				"recommended_solutions",
			},
		},
		"D3": {
			Phase: "D3",
			RequiredSections: []string{
				"system_architecture", "component_design", "api_contracts",
				"data_models", "security_design",
			},
			PreviousPhase: stringPtr("D2"),
			NextPhase:     stringPtr("D4"),
		},
		"D4": {
			Phase: "D4",
			RequiredSections: []string{
				"implementation_plan", "testing_strategy", "deployment_plan",
				"rollback_plan", "monitoring_plan",
			},
			PreviousPhase: stringPtr("D3"),
			NextPhase:     stringPtr("S1"),
		},
		// Implementation Phases
		"S1": {
			Phase: "S1",
			RequiredSections: []string{
				"architecture", "data_models", "infrastructure",
			},
			PreviousPhase: stringPtr("D4"),
			NextPhase:     stringPtr("S2"),
		},
		"S2": {
			Phase:            "S2",
			RequiredSections: []string{"core_functionality", "error_handling"},
			PreviousPhase:    stringPtr("S1"),
			NextPhase:        stringPtr("S3"),
		},
		"S3": {
			Phase:            "S3",
			RequiredSections: []string{"api_implementation", "integration_tests"},
			PreviousPhase:    stringPtr("S2"),
			NextPhase:        stringPtr("S4"),
		},
		"S4": {
			Phase:            "S4",
			RequiredSections: []string{"unit_tests", "integration_tests", "coverage"},
			PreviousPhase:    stringPtr("S3"),
			NextPhase:        stringPtr("S5"),
		},
		"S5": {
			Phase:            "S5",
			RequiredSections: []string{"documentation", "code_comments", "examples"},
			PreviousPhase:    stringPtr("S4"),
			NextPhase:        stringPtr("S6"),
		},
		"S6": {
			Phase:            "S6",
			RequiredSections: []string{"performance_tests", "optimization"},
			PreviousPhase:    stringPtr("S5"),
			NextPhase:        stringPtr("S7"),
		},
		"S7": {
			Phase:            "S7",
			RequiredSections: []string{"security_review", "vulnerability_assessment"},
			PreviousPhase:    stringPtr("S6"),
			NextPhase:        stringPtr("S8"),
		},
		"S8": {
			Phase:            "S8",
			RequiredSections: []string{"deployment_preparation", "configuration"},
			PreviousPhase:    stringPtr("S7"),
			NextPhase:        stringPtr("S9"),
		},
		"S9": {
			Phase:            "S9",
			RequiredSections: []string{"production_deployment", "monitoring_setup"},
			PreviousPhase:    stringPtr("S8"),
			NextPhase:        stringPtr("S10"),
		},
		"S10": {
			Phase:            "S10",
			RequiredSections: []string{"post_deployment_validation", "user_feedback"},
			PreviousPhase:    stringPtr("S9"),
			NextPhase:        stringPtr("S11"),
		},
		"S11": {
			Phase: "S11",
			RequiredSections: []string{
				"what_went_well", "what_went_wrong", "lessons_learned",
				"action_items", "metrics_achieved",
			},
			PreviousPhase: stringPtr("S10"),
			NextPhase:     nil,
			OptionalFields: []string{
				"retrospective_metrics.total_duration_days",
				"retrospective_metrics.phases_completed",
				"retrospective_metrics.phases_skipped",
				"retrospective_metrics.success_criteria_met",
				"knowledge_sharing.patterns_discovered",
				"knowledge_sharing.anti_patterns",
				"knowledge_sharing.recommended_tools",
				"knowledge_sharing.tools_to_avoid",
			},
		},
	}

	schema, ok := schemas[phase]
	if !ok {
		return nil, fmt.Errorf("unknown phase: %s", phase)
	}
	return &schema, nil
}

// ExtractFrontmatter extracts YAML frontmatter from a Markdown file.
func ExtractFrontmatter(content string) (map[string]interface{}, error) {
	// Check for YAML frontmatter delimiters
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("no frontmatter found (missing opening ---)")
	}

	// Find the closing delimiter
	endIndex := strings.Index(content[4:], "\n---\n")
	if endIndex == -1 {
		return nil, fmt.Errorf("malformed frontmatter (missing closing ---)")
	}

	// Extract and parse YAML
	frontmatterYAML := content[4 : 4+endIndex]
	var frontmatter map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return frontmatter, nil
}

// ValidateArtifact validates a Wayfinder artifact's frontmatter.
func ValidateArtifact(frontmatter map[string]interface{}) []WayfinderValidationError {
	var errors []WayfinderValidationError

	// Extract phase first to determine schema
	phase, ok := frontmatter["phase"].(string)
	if !ok {
		errors = append(errors, WayfinderValidationError{
			Field:   "phase",
			Message: "missing or invalid (must be a string)",
		})
		return errors
	}

	// Validate phase is valid
	if !contains(ValidPhases, phase) {
		errors = append(errors, WayfinderValidationError{
			Field:   "phase",
			Message: fmt.Sprintf("invalid phase '%s' (must be one of: %s)", phase, strings.Join(ValidPhases, ", ")),
		})
		return errors
	}

	// Get phase schema
	schema, err := GetPhaseSchema(phase)
	if err != nil {
		errors = append(errors, WayfinderValidationError{
			Field:   "phase",
			Message: err.Error(),
		})
		return errors
	}

	// Validate core frontmatter fields
	errors = append(errors, validateCoreFields(frontmatter)...)

	// Validate phase-specific constraints
	errors = append(errors, validatePhaseConstraints(frontmatter, schema)...)

	// Validate required sections
	errors = append(errors, validateRequiredSections(frontmatter, schema)...)

	return errors
}

// validateCoreFields validates the core frontmatter fields.
func validateCoreFields(fm map[string]interface{}) []WayfinderValidationError {
	var errors []WayfinderValidationError
	errors = append(errors, validateTitle(fm)...)
	errors = append(errors, validateDate(fm)...)
	errors = append(errors, validateStatus(fm)...)
	errors = append(errors, validateProject(fm)...)
	errors = append(errors, validateTags(fm)...)
	errors = append(errors, validateVersionFields(fm)...)

	// Blockers validation (optional)
	if blockersRaw, exists := fm["blockers"]; exists {
		blockers, ok := blockersRaw.([]interface{})
		if !ok {
			errors = append(errors, WayfinderValidationError{Field: "blockers", Message: "must be an array"})
		} else {
			for i, blockerRaw := range blockers {
				blocker, ok := blockerRaw.(map[string]interface{})
				if !ok {
					errors = append(errors, WayfinderValidationError{
						Field:   fmt.Sprintf("blockers[%d]", i),
						Message: "must be an object",
					})
					continue
				}
				errors = append(errors, validateBlocker(blocker, i)...)
			}
		}
	}

	return errors
}

// validatePhaseConstraints validates phase-specific constraints.
func validateTitle(fm map[string]interface{}) []WayfinderValidationError {
	var errs []WayfinderValidationError
	title, ok := fm["title"].(string)
	switch {
	case !ok:
		errs = append(errs, WayfinderValidationError{Field: "title", Message: "missing or invalid (must be a string)"})
	case len(title) < 10:
		errs = append(errs, WayfinderValidationError{Field: "title", Message: "must be at least 10 characters"})
	case len(title) > 150:
		errs = append(errs, WayfinderValidationError{Field: "title", Message: "must be at most 150 characters"})
	}
	return errs
}

func validateDate(fm map[string]interface{}) []WayfinderValidationError {
	var errs []WayfinderValidationError
	var dateStr string
	switch v := fm["date"].(type) {
	case string:
		dateStr = v
	case time.Time:
		dateStr = v.Format("2006-01-02")
	default:
		errs = append(errs, WayfinderValidationError{Field: "date", Message: "missing or invalid (must be a string)"})
	}
	if dateStr != "" && !isValidDate(dateStr) {
		errs = append(errs, WayfinderValidationError{Field: "date", Message: "invalid format (must be YYYY-MM-DD)"})
	}
	return errs
}

func validateStatus(fm map[string]interface{}) []WayfinderValidationError {
	status, ok := fm["status"].(string)
	if !ok {
		return []WayfinderValidationError{{Field: "status", Message: "missing or invalid (must be a string)"}}
	}
	if !contains(ValidStatuses, status) {
		return []WayfinderValidationError{{
			Field:   "status",
			Message: fmt.Sprintf("invalid status '%s' (must be one of: %s)", status, strings.Join(ValidStatuses, ", ")),
		}}
	}
	return nil
}

func validateProject(fm map[string]interface{}) []WayfinderValidationError {
	project, ok := fm["project"].(string)
	if !ok {
		return []WayfinderValidationError{{Field: "project", Message: "missing or invalid (must be a string)"}}
	}
	if !isValidProjectSlug(project) {
		return []WayfinderValidationError{{Field: "project", Message: "invalid format (must be kebab-case)"}}
	}
	return nil
}

func validateTags(fm map[string]interface{}) []WayfinderValidationError {
	var errs []WayfinderValidationError
	tagsRaw, ok := fm["tags"]
	if !ok {
		return []WayfinderValidationError{{Field: "tags", Message: "missing"}}
	}
	tags, ok := tagsRaw.([]interface{})
	switch {
	case !ok:
		return []WayfinderValidationError{{Field: "tags", Message: "must be an array"}}
	case len(tags) == 0:
		return []WayfinderValidationError{{Field: "tags", Message: "must have at least one tag"}}
	}
	seen := make(map[string]bool)
	for i, tagRaw := range tags {
		tag, ok := tagRaw.(string)
		if !ok {
			errs = append(errs, WayfinderValidationError{
				Field:   fmt.Sprintf("tags[%d]", i),
				Message: "must be a string",
			})
			continue
		}
		if !isValidTag(tag) {
			errs = append(errs, WayfinderValidationError{
				Field:   fmt.Sprintf("tags[%d]", i),
				Message: "invalid format (must be lowercase alphanumeric with hyphens)",
			})
		}
		if seen[tag] {
			errs = append(errs, WayfinderValidationError{
				Field:   fmt.Sprintf("tags[%d]", i),
				Message: fmt.Sprintf("duplicate tag '%s'", tag),
			})
		}
		seen[tag] = true
	}
	return errs
}

func validateVersionFields(fm map[string]interface{}) []WayfinderValidationError {
	var errs []WayfinderValidationError
	if schemaVersion, ok := fm["schema_version"].(string); !ok {
		errs = append(errs, WayfinderValidationError{Field: "schema_version", Message: "missing or invalid (must be a string)"})
	} else if !isValidSemver(schemaVersion) {
		errs = append(errs, WayfinderValidationError{Field: "schema_version", Message: "invalid format (must be semver: X.Y or X.Y.Z)"})
	}
	if wayfinderVersion, ok := fm["wayfinder_plugin_version"].(string); !ok {
		errs = append(errs, WayfinderValidationError{Field: "wayfinder_plugin_version", Message: "missing or invalid (must be a string)"})
	} else if !isValidFullSemver(wayfinderVersion) {
		errs = append(errs, WayfinderValidationError{Field: "wayfinder_plugin_version", Message: "invalid format (must be semver: X.Y.Z)"})
	}
	if templateVersion, ok := fm["template_version"].(string); !ok {
		errs = append(errs, WayfinderValidationError{Field: "template_version", Message: "missing or invalid (must be a string)"})
	} else if !isValidFullSemver(templateVersion) {
		errs = append(errs, WayfinderValidationError{Field: "template_version", Message: "invalid format (must be semver: X.Y.Z)"})
	}
	return errs
}

func validatePhaseConstraints(fm map[string]interface{}, schema *PhaseSchema) []WayfinderValidationError {
	var errors []WayfinderValidationError

	// Validate previousPhase
	if schema.PreviousPhase == nil {
		// Should be null or omitted
		if prev, exists := fm["previousPhase"]; exists && prev != nil {
			errors = append(errors, WayfinderValidationError{
				Field:   "previousPhase",
				Message: fmt.Sprintf("must be null for phase %s", schema.Phase),
			})
		}
	} else {
		// Should match expected value
		prev, ok := fm["previousPhase"].(string)
		if !ok && fm["previousPhase"] != nil {
			errors = append(errors, WayfinderValidationError{
				Field:   "previousPhase",
				Message: "must be a string or null",
			})
		} else if ok && prev != *schema.PreviousPhase {
			errors = append(errors, WayfinderValidationError{
				Field:   "previousPhase",
				Message: fmt.Sprintf("must be '%s' for phase %s", *schema.PreviousPhase, schema.Phase),
			})
		}
	}

	// Validate nextPhase
	if schema.NextPhase == nil {
		// Should be null or omitted
		if next, exists := fm["nextPhase"]; exists && next != nil {
			errors = append(errors, WayfinderValidationError{
				Field:   "nextPhase",
				Message: fmt.Sprintf("must be null for phase %s", schema.Phase),
			})
		}
	} else {
		// Should match expected value
		next, ok := fm["nextPhase"].(string)
		if !ok && fm["nextPhase"] != nil {
			errors = append(errors, WayfinderValidationError{
				Field:   "nextPhase",
				Message: "must be a string or null",
			})
		} else if ok && next != *schema.NextPhase {
			errors = append(errors, WayfinderValidationError{
				Field:   "nextPhase",
				Message: fmt.Sprintf("must be '%s' for phase %s", *schema.NextPhase, schema.Phase),
			})
		}
	}

	return errors
}

// validateRequiredSections validates the required_sections field.
func validateRequiredSections(fm map[string]interface{}, schema *PhaseSchema) []WayfinderValidationError {
	var errors []WayfinderValidationError

	sectionsRaw, ok := fm["required_sections"]
	if !ok {
		errors = append(errors, WayfinderValidationError{
			Field:   "required_sections",
			Message: "missing",
		})
		return errors
	}

	sections, ok := sectionsRaw.(map[string]interface{})
	if !ok {
		errors = append(errors, WayfinderValidationError{
			Field:   "required_sections",
			Message: "must be an object",
		})
		return errors
	}

	// Check all required sections are present
	for _, reqSection := range schema.RequiredSections {
		statusRaw, exists := sections[reqSection]
		if !exists {
			errors = append(errors, WayfinderValidationError{
				Field:   fmt.Sprintf("required_sections.%s", reqSection),
				Message: "missing",
			})
			continue
		}

		status, ok := statusRaw.(string)
		if !ok {
			errors = append(errors, WayfinderValidationError{
				Field:   fmt.Sprintf("required_sections.%s", reqSection),
				Message: "must be a string",
			})
			continue
		}

		if !contains(ValidSectionStatuses, status) {
			errors = append(errors, WayfinderValidationError{
				Field: fmt.Sprintf("required_sections.%s", reqSection),
				Message: fmt.Sprintf("invalid status '%s' (must be one of: %s)",
					status, strings.Join(ValidSectionStatuses, ", ")),
			})
		}
	}

	return errors
}

// validateBlocker validates a single blocker object.
func validateBlocker(blocker map[string]interface{}, index int) []WayfinderValidationError {
	var errors []WayfinderValidationError
	prefix := fmt.Sprintf("blockers[%d]", index)

	// Description
	desc, ok := blocker["description"].(string)
	if !ok {
		errors = append(errors, WayfinderValidationError{
			Field:   prefix + ".description",
			Message: "missing or invalid (must be a string)",
		})
	} else if len(desc) < 10 {
		errors = append(errors, WayfinderValidationError{
			Field:   prefix + ".description",
			Message: "must be at least 10 characters",
		})
	}

	// Impact
	impact, ok := blocker["impact"].(string)
	if !ok {
		errors = append(errors, WayfinderValidationError{
			Field:   prefix + ".impact",
			Message: "missing or invalid (must be a string)",
		})
	} else if !contains(ValidImpacts, impact) {
		errors = append(errors, WayfinderValidationError{
			Field:   prefix + ".impact",
			Message: fmt.Sprintf("invalid impact '%s' (must be one of: %s)", impact, strings.Join(ValidImpacts, ", ")),
		})
	}

	// Created date
	var createdDateStr string
	switch v := blocker["created_date"].(type) {
	case string:
		createdDateStr = v
	case time.Time:
		createdDateStr = v.Format("2006-01-02")
	default:
		errors = append(errors, WayfinderValidationError{
			Field:   prefix + ".created_date",
			Message: "missing or invalid (must be a string)",
		})
	}
	if createdDateStr != "" && !isValidDate(createdDateStr) {
		errors = append(errors, WayfinderValidationError{
			Field:   prefix + ".created_date",
			Message: "invalid format (must be YYYY-MM-DD)",
		})
	}

	// Resolved date (optional)
	if resolvedDateRaw, exists := blocker["resolved_date"]; exists && resolvedDateRaw != nil {
		var resolvedDateStr string
		switch v := resolvedDateRaw.(type) {
		case string:
			resolvedDateStr = v
		case time.Time:
			resolvedDateStr = v.Format("2006-01-02")
		default:
			errors = append(errors, WayfinderValidationError{
				Field:   prefix + ".resolved_date",
				Message: "must be a string",
			})
		}
		if resolvedDateStr != "" && !isValidDate(resolvedDateStr) {
			errors = append(errors, WayfinderValidationError{
				Field:   prefix + ".resolved_date",
				Message: "invalid format (must be YYYY-MM-DD)",
			})
		}
	}

	return errors
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

var (
	dateRegex       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	projectSlugRe   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	tagRegex        = regexp.MustCompile(`^[a-z0-9-]+$`)
	semverRegex     = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	fullSemverRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

func isValidDate(date string) bool {
	if !dateRegex.MatchString(date) {
		return false
	}
	// Also validate it's a real date
	_, err := time.Parse("2006-01-02", date)
	return err == nil
}

func isValidProjectSlug(slug string) bool {
	return projectSlugRe.MatchString(slug)
}

func isValidTag(tag string) bool {
	return tagRegex.MatchString(tag)
}

func isValidSemver(version string) bool {
	return semverRegex.MatchString(version)
}

func isValidFullSemver(version string) bool {
	return fullSemverRegex.MatchString(version)
}
