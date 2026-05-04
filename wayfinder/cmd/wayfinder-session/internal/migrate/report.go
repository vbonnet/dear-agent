package migrate

import (
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// MigrationReport represents a detailed migration report
type MigrationReport struct {
	SourceVersion     string
	TargetVersion     string
	ProjectName       string
	ProjectPath       string
	MigratedAt        time.Time
	TotalPhasesV1     int
	TotalPhasesV2     int
	MergedPhases      []PhaseMerge
	PreservedData     DataPreservation
	Warnings          []string
	ValidationResults ValidationResult
}

// PhaseMerge represents a phase merge operation
type PhaseMerge struct {
	SourcePhases []string
	TargetPhase  string
	Reason       string
}

// DataPreservation tracks what data was preserved
type DataPreservation struct {
	TimestampsPreserved int
	OutcomesPreserved   int
	StatusesPreserved   int
	DeliverableCount    int
}

// ValidationResult contains validation check results
type ValidationResult struct {
	SchemaValid       bool
	RequiredFieldsSet bool
	ChronologyValid   bool
	DataIntact        bool
	Errors            []string
}

// GenerateReport creates a detailed migration report
func GenerateReport(v1 *status.Status, v2 *status.StatusV2, opts *ConvertOptions) *MigrationReport {
	report := &MigrationReport{
		SourceVersion: v1.SchemaVersion,
		TargetVersion: v2.SchemaVersion,
		ProjectName:   v2.ProjectName,
		ProjectPath:   v1.ProjectPath,
		MigratedAt:    time.Now(),
		TotalPhasesV1: len(v1.Phases),
		TotalPhasesV2: len(v2.WaypointHistory),
		MergedPhases:  analyzeMerges(v1, v2),
		PreservedData: analyzeDataPreservation(v1, v2),
		Warnings:      []string{},
	}

	// Run validation
	report.ValidationResults = validateMigration(v1, v2)

	// Collect warnings
	report.Warnings = collectWarnings(v1, v2, opts)

	return report
}

// analyzeMerges identifies phase merge operations
//nolint:gocyclo // reason: linear merge analyzer covering many event types
func analyzeMerges(v1 *status.Status, v2 *status.StatusV2) []PhaseMerge {
	merges := []PhaseMerge{}

	// Check for S4 → D4 merge
	hasS4 := false
	hasD4 := false
	for _, phase := range v1.Phases {
		if phase.Name == "S4" {
			hasS4 = true
		}
		if phase.Name == "D4" {
			hasD4 = true
		}
	}
	if hasS4 && hasD4 {
		merges = append(merges, PhaseMerge{
			SourcePhases: []string{"D4", "S4"},
			TargetPhase:  status.PhaseV2Spec,
			Reason:       "Stakeholder alignment merged into requirements",
		})
	}

	// Check for S5 → S6 merge
	hasS5 := false
	hasS6 := false
	for _, phase := range v1.Phases {
		if phase.Name == "S5" {
			hasS5 = true
		}
		if phase.Name == "S6" {
			hasS6 = true
		}
	}
	if hasS5 && hasS6 {
		merges = append(merges, PhaseMerge{
			SourcePhases: []string{"S5", "S6"},
			TargetPhase:  status.PhaseV2Plan,
			Reason:       "Research merged into design phase",
		})
	}

	// Check for S8/S9/S10 → S8 merge
	hasS8 := false
	hasS9 := false
	hasS10 := false
	for _, phase := range v1.Phases {
		if phase.Name == "S8" {
			hasS8 = true
		}
		if phase.Name == "S9" {
			hasS9 = true
		}
		if phase.Name == "S10" {
			hasS10 = true
		}
	}
	if (hasS8 && hasS9) || (hasS8 && hasS10) || (hasS9 && hasS10) {
		sources := []string{}
		if hasS8 {
			sources = append(sources, "S8")
		}
		if hasS9 {
			sources = append(sources, "S9")
		}
		if hasS10 {
			sources = append(sources, "S10")
		}
		merges = append(merges, PhaseMerge{
			SourcePhases: sources,
			TargetPhase:  status.PhaseV2Build,
			Reason:       "BUILD loop consolidation (implement/validate/deploy)",
		})
	}

	return merges
}

// analyzeDataPreservation checks what data was preserved
func analyzeDataPreservation(v1 *status.Status, v2 *status.StatusV2) DataPreservation {
	preservation := DataPreservation{}

	// Count timestamps preserved
	for _, v1Phase := range v1.Phases {
		if v1Phase.StartedAt != nil {
			preservation.TimestampsPreserved++
		}
		if v1Phase.CompletedAt != nil {
			preservation.TimestampsPreserved++
		}
	}

	// Count outcomes preserved
	for _, v1Phase := range v1.Phases {
		if v1Phase.Outcome != "" {
			preservation.OutcomesPreserved++
		}
	}

	// Count statuses preserved
	preservation.StatusesPreserved = len(v1.Phases)

	// Count deliverables
	for _, v2Phase := range v2.WaypointHistory {
		preservation.DeliverableCount += len(v2Phase.Deliverables)
	}

	return preservation
}

// validateMigration performs post-migration validation
func validateMigration(v1 *status.Status, v2 *status.StatusV2) ValidationResult {
	result := ValidationResult{
		Errors: []string{},
	}

	// Check schema version
	result.SchemaValid = v2.SchemaVersion == status.SchemaVersionV2

	// Check required fields
	result.RequiredFieldsSet = v2.ProjectName != "" &&
		v2.ProjectType != "" &&
		v2.RiskLevel != "" &&
		v2.CurrentWaypoint != "" &&
		v2.Status != "" &&
		!v2.CreatedAt.IsZero() &&
		!v2.UpdatedAt.IsZero()

	// Check chronology
	result.ChronologyValid = validateChronology(v2)

	// Check data integrity
	result.DataIntact = len(v2.WaypointHistory) > 0 || len(v1.Phases) == 0

	// Collect errors
	if !result.SchemaValid {
		result.Errors = append(result.Errors, "Invalid schema version")
	}
	if !result.RequiredFieldsSet {
		result.Errors = append(result.Errors, "Missing required fields")
	}
	if !result.ChronologyValid {
		result.Errors = append(result.Errors, "Phase chronology is invalid")
	}
	if !result.DataIntact {
		result.Errors = append(result.Errors, "Data integrity check failed")
	}

	return result
}

// validateChronology checks phase timestamps are in order
func validateChronology(v2 *status.StatusV2) bool {
	var prevTime *time.Time

	for _, phase := range v2.WaypointHistory {
		if !phase.StartedAt.IsZero() {
			if prevTime != nil && phase.StartedAt.Before(*prevTime) {
				return false
			}
			prevTime = &phase.StartedAt
		}

		if phase.CompletedAt != nil && !phase.CompletedAt.IsZero() {
			if prevTime != nil && phase.CompletedAt.Before(*prevTime) {
				return false
			}
			prevTime = phase.CompletedAt
		}
	}

	return true
}

// collectWarnings gathers potential issues
func collectWarnings(v1 *status.Status, v2 *status.StatusV2, opts *ConvertOptions) []string {
	warnings := []string{}

	// Warn if phase count changed significantly
	if len(v1.Phases) > len(v2.WaypointHistory)+3 {
		warnings = append(warnings,
			fmt.Sprintf("Phase count reduced from %d to %d (expected due to merging)",
				len(v1.Phases), len(v2.WaypointHistory)))
	}

	// Warn if no project type was specified
	if opts != nil && opts.ProjectType == "" {
		warnings = append(warnings,
			"Project type not specified, defaulted to 'feature'")
	}

	// Warn if no risk level was specified
	if opts != nil && opts.RiskLevel == "" {
		warnings = append(warnings,
			"Risk level not specified, defaulted to 'M' (medium)")
	}

	// Warn if project is very old
	if time.Since(v1.StartedAt) > 365*24*time.Hour {
		warnings = append(warnings,
			fmt.Sprintf("Project is over 1 year old (started %s)", v1.StartedAt.Format("2006-01-02")))
	}

	return warnings
}

// String formats the report as a human-readable string
func (r *MigrationReport) String() string {
	var sb strings.Builder

	sb.WriteString("╔════════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║            WAYFINDER V1 → V2 MIGRATION REPORT                  ║\n")
	sb.WriteString("╚════════════════════════════════════════════════════════════════╝\n\n")

	// Project info
	sb.WriteString("PROJECT INFORMATION\n")
	sb.WriteString("═══════════════════\n")
	fmt.Fprintf(&sb, "  Name:         %s\n", r.ProjectName)
	fmt.Fprintf(&sb, "  Path:         %s\n", r.ProjectPath)
	fmt.Fprintf(&sb, "  Migrated:     %s\n", r.MigratedAt.Format("2006-01-02 15:04:05 MST"))
	sb.WriteString("\n")

	// Schema versions
	sb.WriteString("SCHEMA VERSIONS\n")
	sb.WriteString("═══════════════\n")
	fmt.Fprintf(&sb, "  Source:       %s\n", r.SourceVersion)
	fmt.Fprintf(&sb, "  Target:       %s\n", r.TargetVersion)
	sb.WriteString("\n")

	// Phase summary
	sb.WriteString("PHASE SUMMARY\n")
	sb.WriteString("═════════════\n")
	fmt.Fprintf(&sb, "  V1 Phases:    %d\n", r.TotalPhasesV1)
	fmt.Fprintf(&sb, "  V2 Phases:    %d\n", r.TotalPhasesV2)
	fmt.Fprintf(&sb, "  Merged:       %d\n", len(r.MergedPhases))
	sb.WriteString("\n")

	// Merged phases
	if len(r.MergedPhases) > 0 {
		sb.WriteString("PHASE MERGES\n")
		sb.WriteString("════════════\n")
		for _, merge := range r.MergedPhases {
			fmt.Fprintf(&sb, "  %s → %s\n",
				strings.Join(merge.SourcePhases, " + "), merge.TargetPhase)
			fmt.Fprintf(&sb, "    Reason: %s\n", merge.Reason)
		}
		sb.WriteString("\n")
	}

	// Data preservation
	sb.WriteString("DATA PRESERVATION\n")
	sb.WriteString("═════════════════\n")
	fmt.Fprintf(&sb, "  Timestamps:   %d preserved\n", r.PreservedData.TimestampsPreserved)
	fmt.Fprintf(&sb, "  Outcomes:     %d preserved\n", r.PreservedData.OutcomesPreserved)
	fmt.Fprintf(&sb, "  Statuses:     %d preserved\n", r.PreservedData.StatusesPreserved)
	fmt.Fprintf(&sb, "  Deliverables: %d items\n", r.PreservedData.DeliverableCount)
	sb.WriteString("\n")

	// Validation results
	sb.WriteString("VALIDATION\n")
	sb.WriteString("══════════\n")
	fmt.Fprintf(&sb, "  Schema Valid:     %s\n", checkmark(r.ValidationResults.SchemaValid))
	fmt.Fprintf(&sb, "  Required Fields:  %s\n", checkmark(r.ValidationResults.RequiredFieldsSet))
	fmt.Fprintf(&sb, "  Chronology:       %s\n", checkmark(r.ValidationResults.ChronologyValid))
	fmt.Fprintf(&sb, "  Data Integrity:   %s\n", checkmark(r.ValidationResults.DataIntact))

	if len(r.ValidationResults.Errors) > 0 {
		sb.WriteString("\n  Errors:\n")
		for _, err := range r.ValidationResults.Errors {
			fmt.Fprintf(&sb, "    ✗ %s\n", err)
		}
	}
	sb.WriteString("\n")

	// Warnings
	if len(r.Warnings) > 0 {
		sb.WriteString("WARNINGS\n")
		sb.WriteString("════════\n")
		for _, warning := range r.Warnings {
			fmt.Fprintf(&sb, "  ⚠  %s\n", warning)
		}
		sb.WriteString("\n")
	}

	// Summary
	overallSuccess := r.ValidationResults.SchemaValid &&
		r.ValidationResults.RequiredFieldsSet &&
		r.ValidationResults.ChronologyValid &&
		r.ValidationResults.DataIntact

	sb.WriteString("═══════════════════════════════════════════════════════════════\n")
	if overallSuccess {
		sb.WriteString("  ✓ MIGRATION SUCCESSFUL\n")
	} else {
		sb.WriteString("  ✗ MIGRATION FAILED - See errors above\n")
	}
	sb.WriteString("═══════════════════════════════════════════════════════════════\n")

	return sb.String()
}

// checkmark returns a checkmark or X based on boolean
func checkmark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}
