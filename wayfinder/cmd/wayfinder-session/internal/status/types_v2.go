package status

import "time"

// StatusV2 represents the WAYFINDER-STATUS.md V2 file structure
// Based on Wayfinder V2 schema with 9-phase consolidation
type StatusV2 struct {
	// Required fields
	SchemaVersion   string    `yaml:"schema_version"` // Must be "2.0"
	ProjectName     string    `yaml:"project_name"`
	ProjectType     string    `yaml:"project_type"`     // feature, research, infrastructure, refactor, bugfix
	RiskLevel       string    `yaml:"risk_level"`       // XS, S, M, L, XL
	CurrentWaypoint string    `yaml:"current_waypoint"` // CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO
	Status          string    `yaml:"status"`           // planning, in-progress, blocked, completed, abandoned
	CreatedAt       time.Time `yaml:"created_at"`
	UpdatedAt       time.Time `yaml:"updated_at"`

	// Optional fields
	Description    string     `yaml:"description,omitempty"`
	Repository     string     `yaml:"repository,omitempty"`
	Branch         string     `yaml:"branch,omitempty"`
	Tags           []string   `yaml:"tags,omitempty"`
	Beads          []string   `yaml:"beads,omitempty"`
	CompletionDate *time.Time `yaml:"completion_date,omitempty"`
	BlockedReason  string     `yaml:"blocked_reason,omitempty"`
	SkipRoadmap    bool       `yaml:"skip_roadmap,omitempty"` // Skip roadmap phases for small projects

	// Waypoint tracking
	WaypointHistory []WaypointHistory `yaml:"waypoint_history"`

	// Roadmap
	Roadmap *Roadmap `yaml:"roadmap,omitempty"`

	// Quality metrics
	QualityMetrics *QualityMetrics `yaml:"quality_metrics,omitempty"`
}

// WaypointHistory represents a waypoint in the history with build metrics
type WaypointHistory struct {
	Name         string     `yaml:"name"`   // CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO
	Status       string     `yaml:"status"` // completed, in-progress, blocked, skipped
	StartedAt    time.Time  `yaml:"started_at"`
	CompletedAt  *time.Time `yaml:"completed_at,omitempty"`
	Deliverables []string   `yaml:"deliverables,omitempty"`
	Notes        string     `yaml:"notes,omitempty"`
	Outcome      *string    `yaml:"outcome,omitempty"` // success, partial, skipped

	// Phase-specific metadata
	StakeholderApproved *bool  `yaml:"stakeholder_approved,omitempty"`  // SPEC
	StakeholderNotes    string `yaml:"stakeholder_notes,omitempty"`     // SPEC
	ResearchNotes       string `yaml:"research_notes,omitempty"`        // PLAN
	TestsFeatureCreated *bool  `yaml:"tests_feature_created,omitempty"` // PLAN
	ValidationStatus    string `yaml:"validation_status,omitempty"`     // BUILD: pending, in-progress, passed, failed
	DeploymentStatus    string `yaml:"deployment_status,omitempty"`     // BUILD: pending, in-progress, deployed, rolled-back
	BuildIterations     int    `yaml:"build_iterations,omitempty"`      // BUILD

	// Build metrics (if this phase had builds)
	BuildMetrics *BuildMetrics `yaml:"build_metrics,omitempty"`
}

// Roadmap represents the native task tracking structure
type Roadmap struct {
	Phases []RoadmapPhase `yaml:"phases"`
}

// RoadmapPhase represents a phase in the roadmap
type RoadmapPhase struct {
	ID          string     `yaml:"id"` // CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO
	Name        string     `yaml:"name"`
	Status      string     `yaml:"status"` // pending, in-progress, completed, blocked, skipped
	StartedAt   *time.Time `yaml:"started_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty"`
	Tasks       []Task     `yaml:"tasks,omitempty"`
}

// Task represents an implementation task
type Task struct {
	ID                 string     `yaml:"id"` // task-1.1, task-2.3, etc.
	Title              string     `yaml:"title"`
	EffortDays         float64    `yaml:"effort_days,omitempty"` // Renamed from Effort for clarity
	Status             string     `yaml:"status"`                // pending, in-progress, completed, blocked
	Deliverables       []string   `yaml:"deliverables,omitempty"`
	TestsStatus        *string    `yaml:"tests_status,omitempty"` // passed, failed, pending
	DependsOn          []string   `yaml:"depends_on,omitempty"`
	Description        string     `yaml:"description,omitempty"`
	Priority           string     `yaml:"priority,omitempty"` // P0, P1, P2
	AssignedTo         string     `yaml:"assigned_to,omitempty"`
	Blocks             []string   `yaml:"blocks,omitempty"`
	AcceptanceCriteria []string   `yaml:"acceptance_criteria,omitempty"`
	StartedAt          *time.Time `yaml:"started_at,omitempty"`
	CompletedAt        *time.Time `yaml:"completed_at,omitempty"`
	BeadID             string     `yaml:"bead_id,omitempty"`
	Notes              string     `yaml:"notes,omitempty"`

	// Verification specification (inspired by Superpowers evidence-based validation)
	VerifyCommand  string     `yaml:"verify_command,omitempty"`  // Command to verify task completion (e.g., "go test ./auth/...")
	VerifyExpected string     `yaml:"verify_expected,omitempty"` // Expected result description (e.g., "exit code 0, all tests pass")
	VerifiedAt     *time.Time `yaml:"verified_at,omitempty"`     // When verification was last run
	VerifyResult   string     `yaml:"verify_result,omitempty"`   // Last verification result (passed, failed)
}

// QualityMetrics represents quality tracking data
type QualityMetrics struct {
	// Test Coverage
	CoveragePercent        float64 `yaml:"coverage_percent,omitempty"`
	CoverageTarget         float64 `yaml:"coverage_target,omitempty"`
	AssertionDensity       float64 `yaml:"assertion_density,omitempty"`
	AssertionDensityTarget float64 `yaml:"assertion_density_target,omitempty"`

	// Review Scores
	MultiPersonaScore    float64 `yaml:"multi_persona_score,omitempty"`
	SecurityScore        float64 `yaml:"security_score,omitempty"`
	PerformanceScore     float64 `yaml:"performance_score,omitempty"`
	ReliabilityScore     float64 `yaml:"reliability_score,omitempty"`
	MaintainabilityScore float64 `yaml:"maintainability_score,omitempty"`

	// Review Issues
	P0Issues int `yaml:"p0_issues,omitempty"`
	P1Issues int `yaml:"p1_issues,omitempty"`
	P2Issues int `yaml:"p2_issues,omitempty"`

	// Effort Tracking
	EstimatedEffortHours float64 `yaml:"estimated_effort_hours,omitempty"`
	ActualEffortHours    float64 `yaml:"actual_effort_hours,omitempty"`
	EffortVariance       float64 `yaml:"effort_variance,omitempty"` // (actual - estimated) / estimated * 100
}

// BuildMetrics represents build-specific metrics for a phase
type BuildMetrics struct {
	TestsPassed       int     `yaml:"tests_passed,omitempty"`
	TestsFailed       int     `yaml:"tests_failed,omitempty"`
	CoveragePercent   float64 `yaml:"coverage_percent,omitempty"`
	AssertionDensity  float64 `yaml:"assertion_density,omitempty"`
	BuildDurationSecs int     `yaml:"build_duration_secs,omitempty"`
}

// Constants for V2 schema
const (
	SchemaVersionV2 = "2.0"

	// Project types
	ProjectTypeFeature        = "feature"
	ProjectTypeResearch       = "research"
	ProjectTypeInfrastructure = "infrastructure"
	ProjectTypeRefactor       = "refactor"
	ProjectTypeBugfix         = "bugfix"

	// Risk levels
	RiskLevelXS = "XS"
	RiskLevelS  = "S"
	RiskLevelM  = "M"
	RiskLevelL  = "L"
	RiskLevelXL = "XL"

	// V2 Waypoints (9-waypoint consolidation)
	WaypointV2Charter  = "CHARTER"  // Intake & Waypoint
	WaypointV2Problem  = "PROBLEM"  // Discovery & Context
	WaypointV2Research = "RESEARCH" // Investigation & Options
	WaypointV2Design   = "DESIGN"   // Architecture & Design Spec
	WaypointV2Spec     = "SPEC"     // Solution Requirements (includes S4 Stakeholder Alignment)
	WaypointV2Plan     = "PLAN"     // Design (includes S5 Research)
	WaypointV2Setup    = "SETUP"    // Planning & Task Breakdown
	WaypointV2Build    = "BUILD"    // BUILD Loop (includes S9 Validation, S10 Deployment)
	WaypointV2Retro    = "RETRO"    // Closure & Retrospective

	// Status values
	StatusV2Planning   = "planning"
	StatusV2InProgress = "in-progress"
	StatusV2Blocked    = "blocked"
	StatusV2Completed  = "completed"
	StatusV2Abandoned  = "abandoned"

	// Waypoint status values
	WaypointStatusV2Pending    = "pending"
	WaypointStatusV2InProgress = "in-progress"
	WaypointStatusV2Completed  = "completed"
	WaypointStatusV2Blocked    = "blocked"
	WaypointStatusV2Skipped    = "skipped"

	// Task status values
	TaskStatusPending    = "pending"
	TaskStatusInProgress = "in-progress"
	TaskStatusCompleted  = "completed"
	TaskStatusBlocked    = "blocked"

	// Validation status (BUILD)
	ValidationStatusPending    = "pending"
	ValidationStatusInProgress = "in-progress"
	ValidationStatusPassed     = "passed"
	ValidationStatusFailed     = "failed"

	// Deployment status (BUILD)
	DeploymentStatusPending    = "pending"
	DeploymentStatusInProgress = "in-progress"
	DeploymentStatusDeployed   = "deployed"
	DeploymentStatusRolledBack = "rolled-back"

	// Priority levels
	PriorityP0 = "P0"
	PriorityP1 = "P1"
	PriorityP2 = "P2"
)

// AllWaypointsV2Schema returns all 9 waypoints in V2 schema sequence
func AllWaypointsV2Schema() []string {
	return []string{
		WaypointV2Charter,
		WaypointV2Problem,
		WaypointV2Research,
		WaypointV2Design,
		WaypointV2Spec,
		WaypointV2Plan,
		WaypointV2Setup,
		WaypointV2Build,
		WaypointV2Retro,
	}
}

// ValidProjectTypes returns all valid project type values
func ValidProjectTypes() []string {
	return []string{
		ProjectTypeFeature,
		ProjectTypeResearch,
		ProjectTypeInfrastructure,
		ProjectTypeRefactor,
		ProjectTypeBugfix,
	}
}

// ValidRiskLevels returns all valid risk level values
func ValidRiskLevels() []string {
	return []string{
		RiskLevelXS,
		RiskLevelS,
		RiskLevelM,
		RiskLevelL,
		RiskLevelXL,
	}
}

// ValidStatuses returns all valid status values
func ValidStatuses() []string {
	return []string{
		StatusV2Planning,
		StatusV2InProgress,
		StatusV2Blocked,
		StatusV2Completed,
		StatusV2Abandoned,
	}
}

// GetVersion returns the schema version (for test compatibility)
func (s *StatusV2) GetVersion() string {
	if s.SchemaVersion == SchemaVersionV2 {
		return WayfinderV2
	}
	return WayfinderV1
}

// FindWaypointHistory finds a waypoint in the history by name
func (s *StatusV2) FindWaypointHistory(waypointName string) *WaypointHistory {
	for i := range s.WaypointHistory {
		if s.WaypointHistory[i].Name == waypointName {
			return &s.WaypointHistory[i]
		}
	}
	return nil
}

// GetWaypointStatus returns the status of a waypoint (for test compatibility)
func (s *StatusV2) GetWaypointStatus(waypointName string) string {
	wh := s.FindWaypointHistory(waypointName)
	if wh != nil {
		return wh.Status
	}
	return WaypointStatusV2Pending
}

// GetSessionID returns a session ID for compatibility with V1 code
// V2 doesn't have session_id, so we generate one from project_name
func (s *StatusV2) GetSessionID() string {
	// Use project_name as session ID for V2
	// If empty, generate one
	if s.ProjectName != "" {
		return s.ProjectName
	}
	return "unknown-session"
}

// UpdateWaypoint updates an existing waypoint or creates it (for V1 compatibility)
// Maps to UpdateWaypointHistory for V2
func (s *StatusV2) UpdateWaypoint(waypointName string, waypointStatus string, outcome string) {
	now := time.Now()

	// Normalize V1 status constants to V2 format (underscore → hyphen)
	// V1: "in_progress" → V2: "in-progress"
	waypointStatus = normalizeStatusV1ToV2(waypointStatus)

	// Find existing waypoint index
	waypointIdx := -1
	for i := range s.WaypointHistory {
		if s.WaypointHistory[i].Name == waypointName {
			waypointIdx = i
			break
		}
	}

	if waypointIdx == -1 {
		// Create new waypoint
		newWaypoint := WaypointHistory{
			Name:      waypointName,
			Status:    waypointStatus,
			StartedAt: now,
		}
		if waypointStatus == WaypointStatusV2Completed {
			newWaypoint.CompletedAt = &now
			if outcome != "" {
				newWaypoint.Outcome = &outcome
			}
		}
		s.WaypointHistory = append(s.WaypointHistory, newWaypoint)
	} else {
		// Update existing waypoint directly in the slice
		s.WaypointHistory[waypointIdx].Status = waypointStatus
		if waypointStatus == WaypointStatusV2InProgress && s.WaypointHistory[waypointIdx].StartedAt.IsZero() {
			s.WaypointHistory[waypointIdx].StartedAt = now
		}
		if waypointStatus == WaypointStatusV2Completed {
			if s.WaypointHistory[waypointIdx].CompletedAt == nil {
				s.WaypointHistory[waypointIdx].CompletedAt = &now
			}
			if outcome != "" {
				s.WaypointHistory[waypointIdx].Outcome = &outcome
			}
		}
	}

	// Update timestamp
	s.UpdatedAt = now
}

// WriteTo writes STATUS file to specified directory (for V1 compatibility)
// Wraps WriteV2ToDir
func (s *StatusV2) WriteTo(dir string) error {
	return WriteV2ToDir(s, dir)
}

// FindWaypoint finds a waypoint and returns it in V1 Phase format (for validator compatibility)
// Converts WaypointHistory to Phase
func (s *StatusV2) FindWaypoint(waypointName string) *Phase {
	wh := s.FindWaypointHistory(waypointName)
	if wh == nil {
		return nil
	}

	// Convert WaypointHistory to Phase (V1 format)
	outcome := ""
	if wh.Outcome != nil {
		outcome = *wh.Outcome
	}

	// Normalize V2 status to V1 format (hyphen → underscore)
	// V2: "in-progress" → V1: "in_progress"
	v1Status := normalizeStatusV2ToV1(wh.Status)

	return &Phase{
		Name:        wh.Name,
		Status:      v1Status,
		StartedAt:   &wh.StartedAt,
		CompletedAt: wh.CompletedAt,
		Outcome:     outcome,
	}
}

// GetCurrentWaypoint returns the current waypoint for V2 Status
func (s *StatusV2) GetCurrentWaypoint() string {
	return s.CurrentWaypoint
}

// GetStartedAt returns the created time for V2 Status (maps CreatedAt to StartedAt for compatibility)
func (s *StatusV2) GetStartedAt() time.Time {
	return s.CreatedAt
}

// GetSkipRoadmap returns the skip_roadmap flag for V2 Status
func (s *StatusV2) GetSkipRoadmap() bool {
	return s.SkipRoadmap
}

// SetCurrentWaypoint sets the current waypoint for V2 Status
func (s *StatusV2) SetCurrentWaypoint(waypoint string) {
	s.CurrentWaypoint = waypoint
	s.UpdatedAt = time.Now()
}

// normalizeStatusV1ToV2 converts V1 status constants to V2 format
// V1 uses underscores (in_progress), V2 uses hyphens (in-progress)
func normalizeStatusV1ToV2(status string) string {
	switch status {
	case "in_progress":
		return "in-progress"
	case "pending", "completed", "skipped", "blocked":
		return status // Same in both versions
	default:
		return status // Unknown status, pass through
	}
}

// normalizeStatusV2ToV1 converts V2 status constants to V1 format
// V2 uses hyphens (in-progress), V1 uses underscores (in_progress)
func normalizeStatusV2ToV1(status string) string {
	switch status {
	case "in-progress":
		return "in_progress"
	case "pending", "completed", "skipped", "blocked":
		return status // Same in both versions
	default:
		return status // Unknown status, pass through
	}
}

// ============================================================================
// Backward Compatibility Aliases (for StatusInterface during transition)
// ============================================================================
// These methods provide the old "phase" API while internal migration is ongoing.
// TODO: Remove these after all internal packages are updated to waypoint terminology.

// GetCurrentPhase is a backward-compatibility alias for GetCurrentWaypoint
func (s *StatusV2) GetCurrentPhase() string {
	return s.GetCurrentWaypoint()
}

// SetCurrentPhase is a backward-compatibility alias for SetCurrentWaypoint
func (s *StatusV2) SetCurrentPhase(waypoint string) {
	s.SetCurrentWaypoint(waypoint)
}

// FindPhase is a backward-compatibility alias for FindWaypoint
func (s *StatusV2) FindPhase(waypointName string) *Phase {
	return s.FindWaypoint(waypointName)
}

// UpdatePhase is a backward-compatibility alias for UpdateWaypoint
func (s *StatusV2) UpdatePhase(waypointName string, waypointStatus string, outcome string) {
	s.UpdateWaypoint(waypointName, waypointStatus, outcome)
}

// GetPhaseStatus is a backward-compatibility alias for GetWaypointStatus
func (s *StatusV2) GetPhaseStatus(waypointName string) string {
	return s.GetWaypointStatus(waypointName)
}

// FindPhaseHistory is a backward-compatibility alias for FindWaypointHistory
func (s *StatusV2) FindPhaseHistory(waypointName string) *WaypointHistory {
	return s.FindWaypointHistory(waypointName)
}

// ============================================================================
// Type Aliases (for backward compatibility during transition)
// ============================================================================

// PhaseHistory is a backward-compatibility type alias for WaypointHistory
type PhaseHistory = WaypointHistory

// ============================================================================
// Constant Aliases (for backward compatibility during transition)
// ============================================================================

// Phase constant aliases for backward compatibility
const (
	// Waypoint ID aliases
	PhaseV2Charter  = WaypointV2Charter
	PhaseV2Problem  = WaypointV2Problem
	PhaseV2Research = WaypointV2Research
	PhaseV2Design   = WaypointV2Design
	PhaseV2Spec     = WaypointV2Spec
	PhaseV2Plan     = WaypointV2Plan
	PhaseV2Setup    = WaypointV2Setup
	PhaseV2Build    = WaypointV2Build
	PhaseV2Retro    = WaypointV2Retro

	// Status aliases
	PhaseStatusV2Pending    = WaypointStatusV2Pending
	PhaseStatusV2InProgress = WaypointStatusV2InProgress
	PhaseStatusV2Completed  = WaypointStatusV2Completed
	PhaseStatusV2Blocked    = WaypointStatusV2Blocked
	PhaseStatusV2Skipped    = WaypointStatusV2Skipped
)

// AllPhasesV2Schema is a backward-compatibility alias for AllWaypointsV2Schema
func AllPhasesV2Schema() []string {
	return AllWaypointsV2Schema()
}
