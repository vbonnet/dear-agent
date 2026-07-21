package status

import (
	"fmt"
	"math"
	"strings"
)

// ValidateV2 validates a V2 StatusV2 struct against the schema
func ValidateV2(status *StatusV2) error {
	if status == nil {
		return fmt.Errorf("status is nil")
	}

	var errors []string

	// Validate required fields
	if err := validateRequiredFields(status); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate enum fields
	if err := validateEnums(status); err != nil {
		errors = append(errors, err.Error())
	}
	if err := validateSkipPhases(status.SkipPhases); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate waypoint history
	if err := validateWaypointHistory(status); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate roadmap
	if err := validateRoadmap(status); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate quality metrics
	if err := validateQualityMetrics(status); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate conditional requirements
	if err := validateConditionalRequirements(status); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

func validateSkipPhases(phases []string) error {
	allowed := map[string]bool{
		WaypointV2Design: true,
		WaypointV2Spec:   true,
		WaypointV2Plan:   true,
	}
	seen := make(map[string]bool, len(phases))
	for _, phase := range phases {
		if !allowed[phase] {
			return fmt.Errorf("skip_phases contains unsafe phase %q; only DESIGN, SPEC, and PLAN may be skipped", phase)
		}
		if seen[phase] {
			return fmt.Errorf("skip_phases contains duplicate phase %q", phase)
		}
		seen[phase] = true
	}
	return nil
}

// ValidateSessionCompletion requires completed history for every canonical
// waypoint except phases explicitly skipped by the session configuration.
func ValidateSessionCompletion(status *StatusV2) error {
	if status == nil {
		return fmt.Errorf("status is nil")
	}
	var incomplete []string
	for _, waypointName := range AllWaypointsV2Schema() {
		if status.IsPhaseSkipped(waypointName) {
			if waypoint := status.GetWaypointHistory(waypointName); waypoint != nil && waypoint.Status != WaypointStatusV2Skipped && waypoint.Status != WaypointStatusV2Completed {
				return fmt.Errorf("configured skipped Wayfinder phase %s has active history status %s", waypointName, waypoint.Status)
			}
			continue
		}
		waypoint := status.GetWaypointHistory(waypointName)
		if waypoint == nil || waypoint.Status != WaypointStatusV2Completed {
			incomplete = append(incomplete, waypointName)
		}
	}
	if len(incomplete) > 0 {
		return fmt.Errorf("required Wayfinder phases are incomplete: %s", strings.Join(incomplete, ", "))
	}
	return nil
}

// validateRequiredFields checks all required fields are present
func validateRequiredFields(status *StatusV2) error {
	var errors []string

	if status.SchemaVersion == "" {
		errors = append(errors, "schema_version is required")
	}
	if strings.TrimSpace(status.ProjectName) == "" {
		errors = append(errors, "project_name is required")
	}
	if status.ProjectType == "" {
		errors = append(errors, "project_type is required")
	}
	if status.RiskLevel == "" {
		errors = append(errors, "risk_level is required")
	}
	if status.CurrentWaypoint == "" {
		errors = append(errors, "current_waypoint is required")
	}
	if status.Status == "" {
		errors = append(errors, "status is required")
	}
	if status.CreatedAt.IsZero() {
		errors = append(errors, "created_at is required")
	}
	if status.UpdatedAt.IsZero() {
		errors = append(errors, "updated_at is required")
	}

	if len(errors) > 0 {
		return fmt.Errorf("required fields missing: %s", strings.Join(errors, ", "))
	}
	return nil
}

// validateEnums checks that enum fields have valid values
func validateEnums(status *StatusV2) error {
	var errors []string

	// Validate schema_version
	if status.SchemaVersion != SchemaVersion {
		errors = append(errors, fmt.Sprintf("schema_version must be '%s', got '%s'", SchemaVersion, status.SchemaVersion))
	}

	// Validate project_type
	if !contains(ValidProjectTypes(), status.ProjectType) {
		errors = append(errors, fmt.Sprintf("invalid project_type '%s', must be one of: %s", status.ProjectType, strings.Join(ValidProjectTypes(), ", ")))
	}

	// Validate risk_level
	if !contains(ValidRiskLevels(), status.RiskLevel) {
		errors = append(errors, fmt.Sprintf("invalid risk_level '%s', must be one of: %s", status.RiskLevel, strings.Join(ValidRiskLevels(), ", ")))
	}

	// Validate current_waypoint
	if !contains(AllWaypointsV2Schema(), status.CurrentWaypoint) {
		errors = append(errors, fmt.Sprintf("invalid current_waypoint '%s', must be one of: %s", status.CurrentWaypoint, strings.Join(AllWaypointsV2Schema(), ", ")))
	}

	// Validate status
	if !contains(ValidStatuses(), status.Status) {
		errors = append(errors, fmt.Sprintf("invalid status '%s', must be one of: %s", status.Status, strings.Join(ValidStatuses(), ", ")))
	}
	if err := validateLifecycleState(status); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("enum validation failed: %s", strings.Join(errors, "; "))
	}
	return nil
}

func validateLifecycleState(status *StatusV2) error {
	if status.LifecycleState == "" {
		return nil
	}
	expectedStatuses := map[string]string{
		LifecycleWorking:           StatusV2InProgress,
		LifecycleInputRequired:     StatusV2Blocked,
		LifecycleDependencyBlocked: StatusV2Blocked,
		LifecycleValidating:        StatusV2InProgress,
		LifecycleCompleted:         StatusV2Completed,
		LifecycleFailed:            StatusV2Blocked,
		LifecycleCanceled:          StatusV2Abandoned,
	}
	expectedStatus, valid := expectedStatuses[status.LifecycleState]
	if !valid {
		return fmt.Errorf("invalid lifecycle_state %q", status.LifecycleState)
	}
	if status.Status != expectedStatus {
		return fmt.Errorf("lifecycle_state %q requires status %q, got %q", status.LifecycleState, expectedStatus, status.Status)
	}
	switch status.LifecycleState {
	case LifecycleInputRequired:
		if strings.TrimSpace(status.InputNeeded) == "" {
			return fmt.Errorf("lifecycle_state %q requires input_needed", status.LifecycleState)
		}
	case LifecycleDependencyBlocked:
		if strings.TrimSpace(status.BlockedOn) == "" {
			return fmt.Errorf("lifecycle_state %q requires blocked_on", status.LifecycleState)
		}
	case LifecycleFailed:
		if strings.TrimSpace(status.ErrorMessage) == "" {
			return fmt.Errorf("lifecycle_state %q requires error_message", status.LifecycleState)
		}
	}
	return nil
}

// validateWaypointHistory checks waypoint history consistency
func validateWaypointHistory(status *StatusV2) error {
	var errors []string
	allWaypoints := AllWaypointsV2Schema()
	seenWaypoints := make(map[string]bool, len(status.WaypointHistory))

	for i, waypoint := range status.WaypointHistory {
		if seenWaypoints[waypoint.Name] {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: duplicate waypoint name '%s'", i, waypoint.Name))
		}
		seenWaypoints[waypoint.Name] = true

		// Validate waypoint name
		if !contains(allWaypoints, waypoint.Name) {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: invalid waypoint name '%s'", i, waypoint.Name))
		}

		// Validate waypoint status
		validWaypointStatuses := []string{WaypointStatusV2Pending, WaypointStatusV2Completed, WaypointStatusV2InProgress, WaypointStatusV2Blocked, WaypointStatusV2Skipped}
		if !contains(validWaypointStatuses, waypoint.Status) {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: invalid status '%s'", i, waypoint.Status))
		}
		if err := validateWaypointSkipStatus(status, waypoint, i); err != nil {
			errors = append(errors, err.Error())
		}

		// Validate started_at is present
		if waypoint.StartedAt.IsZero() {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: started_at is required", i))
		}

		// Validate completed waypoints have completed_at
		if waypoint.Status == WaypointStatusV2Completed && waypoint.CompletedAt == nil {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: completed waypoints must have completed_at", i))
		}
		if waypoint.Outcome != nil && !contains([]string{OutcomeSuccess, OutcomePartial, OutcomeSkipped}, *waypoint.Outcome) {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: invalid outcome '%s'", i, *waypoint.Outcome))
		}

		// Validate waypoint-specific metadata
		if err := validateWaypointMetadata(waypoint, i); err != nil {
			errors = append(errors, err.Error())
		}

	}
	errors = append(errors, validateWaypointSequence(status, allWaypoints)...)

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

func validateWaypointSkipStatus(status *StatusV2, waypoint WaypointHistory, index int) error {
	configuredSkip := status.IsPhaseSkipped(waypoint.Name)
	if waypoint.Status == WaypointStatusV2Skipped && !configuredSkip {
		return fmt.Errorf("waypoint_history[%d]: mandatory waypoint '%s' cannot be skipped", index, waypoint.Name)
	}
	if configuredSkip && waypoint.Status != WaypointStatusV2Skipped && waypoint.Status != WaypointStatusV2Completed {
		return fmt.Errorf("waypoint_history[%d]: configured skipped waypoint '%s' cannot have active status '%s'", index, waypoint.Name, waypoint.Status)
	}
	return nil
}

func validateWaypointSequence(status *StatusV2, allWaypoints []string) []string {
	positions := make(map[string]int, len(allWaypoints))
	for index, waypoint := range allWaypoints {
		positions[waypoint] = index
	}

	errors := validateWaypointHistoryOrder(status, allWaypoints, positions)
	currentPosition, currentValid := positions[status.CurrentWaypoint]
	if !currentValid {
		return errors
	}
	for index, waypoint := range status.WaypointHistory {
		position, valid := positions[waypoint.Name]
		if valid && position > currentPosition {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: waypoint '%s' cannot be ahead of current_waypoint '%s'", index, waypoint.Name, status.CurrentWaypoint))
		}
	}
	completed := completedWaypoints(status)
	if status.IsPhaseSkipped(status.CurrentWaypoint) && !completed[status.CurrentWaypoint] {
		errors = append(errors, fmt.Sprintf("current_waypoint '%s' is configured to be skipped but has no completed or skipped history", status.CurrentWaypoint))
	}
	for _, predecessor := range missingPredecessors(status, allWaypoints[:currentPosition], completed) {
		errors = append(errors, fmt.Sprintf("current_waypoint '%s' requires completed predecessor '%s'", status.CurrentWaypoint, predecessor))
	}
	return errors
}

func validateWaypointHistoryOrder(status *StatusV2, allWaypoints []string, positions map[string]int) []string {
	var errors []string
	completedBefore := make(map[string]bool, len(status.WaypointHistory))
	lastPosition := -1
	for index, waypoint := range status.WaypointHistory {
		position, valid := positions[waypoint.Name]
		if !valid {
			continue
		}
		if position < lastPosition {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: waypoint '%s' is out of canonical order", index, waypoint.Name))
		}
		for _, predecessor := range missingPredecessors(status, allWaypoints[:position], completedBefore) {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d]: predecessor '%s' must be completed before '%s'", index, predecessor, waypoint.Name))
		}
		if isCompletedWaypoint(status, waypoint) {
			completedBefore[waypoint.Name] = true
		}
		lastPosition = position
	}
	return errors
}

func completedWaypoints(status *StatusV2) map[string]bool {
	completed := make(map[string]bool, len(status.WaypointHistory))
	for _, waypoint := range status.WaypointHistory {
		if isCompletedWaypoint(status, waypoint) {
			completed[waypoint.Name] = true
		}
	}
	return completed
}

func missingPredecessors(status *StatusV2, predecessors []string, completed map[string]bool) []string {
	var missing []string
	for _, predecessor := range predecessors {
		if !status.IsPhaseSkipped(predecessor) && !completed[predecessor] {
			missing = append(missing, predecessor)
		}
	}
	return missing
}

func isCompletedWaypoint(status *StatusV2, waypoint WaypointHistory) bool {
	return waypoint.Status == WaypointStatusV2Completed ||
		(waypoint.Status == WaypointStatusV2Skipped && status.IsPhaseSkipped(waypoint.Name))
}

// validateWaypointMetadata validates waypoint-specific metadata fields
func validateWaypointMetadata(waypoint WaypointHistory, index int) error {
	var errors []string

	// Phase-specific metadata is optional. start-phase writes an in-progress
	// history entry before these values are known, so structural parsing must
	// accept their absence and validate only values that are present.
	// BUILD enum validation
	if waypoint.Name == WaypointV2Build {
		validValidationStatuses := []string{ValidationStatusPending, ValidationStatusInProgress, ValidationStatusPassed, ValidationStatusFailed}
		if waypoint.ValidationStatus != "" && !contains(validValidationStatuses, waypoint.ValidationStatus) {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d] (BUILD): invalid validation_status '%s'", index, waypoint.ValidationStatus))
		}

		validDeploymentStatuses := []string{DeploymentStatusPending, DeploymentStatusInProgress, DeploymentStatusDeployed, DeploymentStatusRolledBack}
		if waypoint.DeploymentStatus != "" && !contains(validDeploymentStatuses, waypoint.DeploymentStatus) {
			errors = append(errors, fmt.Sprintf("waypoint_history[%d] (BUILD): invalid deployment_status '%s'", index, waypoint.DeploymentStatus))
		}
	}

	if waypoint.BuildMetrics != nil {
		for name, value := range map[string]float64{
			"coverage_percent":  waypoint.BuildMetrics.CoveragePercent,
			"assertion_density": waypoint.BuildMetrics.AssertionDensity,
		} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				errors = append(errors, fmt.Sprintf("waypoint_history[%d] (%s): build_metrics.%s must be finite", index, waypoint.Name, name))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// validateRoadmap validates the roadmap structure and task dependencies
func validateRoadmap(status *StatusV2) error {
	if status.Roadmap == nil {
		return nil // Roadmap is optional
	}

	var errors []string

	// Collect all task IDs for dependency validation
	allTaskIDs := make(map[string]bool)
	allPhaseIDs := make(map[string]bool)
	var allTasks []Task

	for i, phase := range status.Roadmap.Phases {
		// Validate waypoint ID
		if !contains(AllWaypointsV2Schema(), phase.ID) {
			errors = append(errors, fmt.Sprintf("roadmap.phases[%d]: invalid waypoint_id '%s'", i, phase.ID))
		}
		if allPhaseIDs[phase.ID] {
			errors = append(errors, fmt.Sprintf("roadmap.phases[%d]: duplicate waypoint_id '%s'", i, phase.ID))
		}
		allPhaseIDs[phase.ID] = true

		// Validate waypoint status
		validWaypointStatuses := []string{WaypointStatusV2Pending, WaypointStatusV2InProgress, WaypointStatusV2Completed, WaypointStatusV2Blocked, WaypointStatusV2Skipped}
		if !contains(validWaypointStatuses, phase.Status) {
			errors = append(errors, fmt.Sprintf("roadmap.phases[%d]: invalid status '%s'", i, phase.Status))
		}

		// Collect tasks
		for taskIndex, task := range phase.Tasks {
			if strings.TrimSpace(task.ID) == "" {
				errors = append(errors, fmt.Sprintf("roadmap.phases[%d]: task has empty ID", i))
				continue
			}
			if math.IsNaN(task.EffortDays) || math.IsInf(task.EffortDays, 0) {
				errors = append(errors, fmt.Sprintf("roadmap.phases[%d].tasks[%d].effort_days must be finite", i, taskIndex))
			} else if task.EffortDays < 0 {
				errors = append(errors, fmt.Sprintf("roadmap.phases[%d].tasks[%d].effort_days cannot be negative", i, taskIndex))
			}

			if allTaskIDs[task.ID] {
				errors = append(errors, fmt.Sprintf("roadmap.phases[%d]: duplicate task_id '%s'", i, task.ID))
			}
			allTaskIDs[task.ID] = true
			allTasks = append(allTasks, task)
		}
	}

	// Validate task dependencies
	if err := validateTaskDependencies(allTasks, allTaskIDs); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// validateTaskDependencies checks that dependencies are valid and acyclic
func validateTaskDependencies(tasks []Task, validTaskIDs map[string]bool) error {
	var errors []string

	for _, task := range tasks {
		// Validate task status
		validTaskStatuses := []string{TaskStatusPending, TaskStatusInProgress, TaskStatusCompleted, TaskStatusBlocked}
		if !contains(validTaskStatuses, task.Status) {
			errors = append(errors, fmt.Sprintf("task '%s': invalid status '%s'", task.ID, task.Status))
		}

		// Validate depends_on references
		for _, depID := range task.DependsOn {
			if !validTaskIDs[depID] {
				errors = append(errors, fmt.Sprintf("task '%s': depends_on references non-existent task '%s'", task.ID, depID))
			}
		}

		// Validate blocks references
		for _, blockID := range task.Blocks {
			if !validTaskIDs[blockID] {
				errors = append(errors, fmt.Sprintf("task '%s': blocks references non-existent task '%s'", task.ID, blockID))
			}
		}

		// Validate priority
		if task.Priority != "" {
			validPriorities := []string{PriorityP0, PriorityP1, PriorityP2}
			if !contains(validPriorities, task.Priority) {
				errors = append(errors, fmt.Sprintf("task '%s': invalid priority '%s'", task.ID, task.Priority))
			}
		}
	}

	// Check for cyclic dependencies
	if err := detectCyclicDependencies(tasks); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// detectCyclicDependencies uses DFS to detect cycles in the task dependency graph
func detectCyclicDependencies(tasks []Task) error {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, task := range tasks {
		graph[task.ID] = task.DependsOn
	}

	// Track visited nodes (white = 0, gray = 1, black = 2)
	visited := make(map[string]int)
	var path []string

	var dfs func(taskID string) error
	dfs = func(taskID string) error {
		if visited[taskID] == 1 {
			// Found a cycle
			cycleStart := -1
			for i, id := range path {
				if id == taskID {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := make([]string, 0, len(path)-cycleStart+1)
				cycle = append(cycle, path[cycleStart:]...)
				cycle = append(cycle, taskID)
				return fmt.Errorf("cyclic dependency detected: %s", strings.Join(cycle, " -> "))
			}
		}
		if visited[taskID] == 2 {
			return nil // Already processed
		}

		visited[taskID] = 1 // Mark as being visited
		path = append(path, taskID)

		for _, dep := range graph[taskID] {
			if err := dfs(dep); err != nil {
				return err
			}
		}

		path = path[:len(path)-1]
		visited[taskID] = 2 // Mark as fully processed
		return nil
	}

	// Check all nodes
	for taskID := range graph {
		if visited[taskID] == 0 {
			if err := dfs(taskID); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateQualityMetrics checks quality metrics are within valid ranges
//
//nolint:gocyclo // reason: linear validator with one branch per metric
func validateQualityMetrics(status *StatusV2) error {
	if status.QualityMetrics == nil {
		return nil // Quality metrics are optional
	}

	var errors []string
	qm := status.QualityMetrics
	finiteValues := map[string]float64{
		"coverage_percent":         qm.CoveragePercent,
		"coverage_target":          qm.CoverageTarget,
		"assertion_density":        qm.AssertionDensity,
		"assertion_density_target": qm.AssertionDensityTarget,
		"multi_persona_score":      qm.MultiPersonaScore,
		"security_score":           qm.SecurityScore,
		"performance_score":        qm.PerformanceScore,
		"reliability_score":        qm.ReliabilityScore,
		"maintainability_score":    qm.MaintainabilityScore,
		"estimated_effort_hours":   qm.EstimatedEffortHours,
		"actual_effort_hours":      qm.ActualEffortHours,
		"effort_variance":          qm.EffortVariance,
	}
	for name, value := range finiteValues {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			errors = append(errors, fmt.Sprintf("%s must be finite", name))
		}
	}

	// Validate coverage percentages (0-100)
	if qm.CoveragePercent < 0 || qm.CoveragePercent > 100 {
		errors = append(errors, fmt.Sprintf("coverage_percent must be 0-100, got %.2f", qm.CoveragePercent))
	}
	if qm.CoverageTarget < 0 || qm.CoverageTarget > 100 {
		errors = append(errors, fmt.Sprintf("coverage_target must be 0-100, got %.2f", qm.CoverageTarget))
	}

	// Validate scores (0-100)
	scores := map[string]float64{
		"multi_persona_score":   qm.MultiPersonaScore,
		"security_score":        qm.SecurityScore,
		"performance_score":     qm.PerformanceScore,
		"reliability_score":     qm.ReliabilityScore,
		"maintainability_score": qm.MaintainabilityScore,
	}
	for name, score := range scores {
		if score < 0 || score > 100 {
			errors = append(errors, fmt.Sprintf("%s must be 0-100, got %.2f", name, score))
		}
	}

	// Validate non-negative values
	if qm.AssertionDensity < 0 {
		errors = append(errors, "assertion_density cannot be negative")
	}
	if qm.AssertionDensityTarget < 0 {
		errors = append(errors, "assertion_density_target cannot be negative")
	}
	if qm.P0Issues < 0 {
		errors = append(errors, "p0_issues cannot be negative")
	}
	if qm.P1Issues < 0 {
		errors = append(errors, "p1_issues cannot be negative")
	}
	if qm.P2Issues < 0 {
		errors = append(errors, "p2_issues cannot be negative")
	}
	if qm.EstimatedEffortHours < 0 {
		errors = append(errors, "estimated_effort_hours cannot be negative")
	}
	if qm.ActualEffortHours < 0 {
		errors = append(errors, "actual_effort_hours cannot be negative")
	}

	if len(errors) > 0 {
		return fmt.Errorf("quality metrics validation failed: %s", strings.Join(errors, "; "))
	}
	return nil
}

// validateConditionalRequirements checks conditional validation rules
func validateConditionalRequirements(status *StatusV2) error {
	var errors []string

	// If status = completed, completion_date must be present
	if status.Status == StatusV2Completed && status.CompletionDate == nil {
		errors = append(errors, "status is 'completed' but completion_date is missing")
	}
	if status.Status == StatusV2Completed {
		if err := ValidateSessionCompletion(status); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// If status = blocked, blocked_reason must be present.
	if status.Status == StatusV2Blocked && strings.TrimSpace(status.BlockedReason) == "" {
		errors = append(errors, "status is 'blocked' but blocked_reason is empty")
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
