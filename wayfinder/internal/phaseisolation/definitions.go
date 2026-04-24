package phaseisolation

// PhaseDependencyGraph maps each phase to its dependencies and load strategies.
var PhaseDependencyGraph = map[PhaseID]map[PhaseID]LoadStrategy{
	PhaseD1:  {},
	PhaseD2:  {PhaseD1: LoadSummary},
	PhaseD3:  {PhaseD1: LoadSummary, PhaseD2: LoadFull},
	PhaseD4:  {PhaseD3: LoadFull},
	PhaseS4:  {PhaseD4: LoadFull, PhaseD3: LoadSummary},
	PhaseS5:  {PhaseS4: LoadFull},
	PhaseS6:  {PhaseD4: LoadSummary, PhaseS5: LoadFull},
	PhaseS7:  {PhaseD4: LoadSummary, PhaseS6: LoadFull},
	PhaseS8:  {PhaseS7: LoadFull},
	PhaseS9:  {PhaseS8: LoadFull},
	PhaseS10: {PhaseS9: LoadFull},
	PhaseS11: {PhaseD4: LoadSummary, PhaseS8: LoadSummary, PhaseS10: LoadFull},
}

// V1ToV2PhaseMap maps TypeScript phase IDs to Go wayfinder-session phase names.
var V1ToV2PhaseMap = map[PhaseID]V2PhaseName{
	PhaseD1:  V2Problem,
	PhaseD2:  V2Research,
	PhaseD3:  V2Design,
	PhaseD4:  V2Spec,
	PhaseS4:  V2Plan,
	PhaseS5:  V2Setup,
	PhaseS6:  V2Design,
	PhaseS7:  V2Plan,
	PhaseS8:  V2Build,
	PhaseS9:  V2Build,
	PhaseS10: V2Build,
	PhaseS11: V2Retro,
}

// PhaseDefinitions holds all phase definitions.
var PhaseDefinitions = map[PhaseID]PhaseDefinition{
	PhaseD1: {
		ID:          PhaseD1,
		Name:        "Problem Validation",
		Objective:   "Validate problem is real, significant, and solvable",
		Deliverable: "D1-problem-validation.md",
		SuccessCriteria: []string{
			"Problem clearly defined",
			"Evidence problem is real (data, research, stakeholder input)",
			"Impact quantified (cost, time, quality)",
			"Feasibility assessed (technical, organizational)",
			"Decision: proceed/pivot/halt with clear rationale",
		},
		TokenBudget: 500,
	},
	PhaseD2: {
		ID:          PhaseD2,
		Name:        "Solutions Search",
		Objective:   "Research and evaluate existing solutions to the problem",
		Deliverable: "D2-existing-solutions.md",
		SuccessCriteria: []string{
			"Industry best practices researched",
			"Existing solutions evaluated (3-5 alternatives)",
			"Tradeoffs documented (pros/cons)",
			"Feasibility assessed for each solution",
			"Recommendation provided with rationale",
		},
		TokenBudget: 600,
	},
	PhaseD3: {
		ID:          PhaseD3,
		Name:        "Approach Decision",
		Objective:   "Select best approach based on problem and solutions research",
		Deliverable: "D3-approach-decision.md",
		SuccessCriteria: []string{
			"Decision matrix created (weighted scoring)",
			"Approach selected with clear rationale",
			"Risk assessment documented",
			"Success criteria defined",
			"Implementation strategy outlined",
		},
		TokenBudget: 700,
	},
	PhaseD4: {
		ID:          PhaseD4,
		Name:        "Solution Requirements",
		Objective:   "Define complete architecture and requirements for chosen solution",
		Deliverable: "D4-solution-requirements.md",
		SuccessCriteria: []string{
			"Architecture components specified",
			"Data structures defined",
			"Interfaces documented",
			"Integration points identified",
			"Acceptance criteria (functional + non-functional)",
		},
		TokenBudget: 600,
	},
	PhaseS4: {
		ID:          PhaseS4,
		Name:        "Stakeholder Alignment",
		Objective:   "Validate design meets stakeholder expectations",
		Deliverable: "S4-stakeholder-alignment.md",
		SuccessCriteria: []string{
			"Discovery findings presented to stakeholders",
			"Improvement opportunity confirmed addressable",
			"Approach aligns with architecture vision",
			"Sign-off obtained for implementation",
			"Concerns and requirements documented",
		},
		TokenBudget: 700,
	},
	PhaseS5: {
		ID:          PhaseS5,
		Name:        "Research",
		Objective:   "Investigate remaining technical unknowns",
		Deliverable: "S5-research.md",
		SuccessCriteria: []string{
			"Technical unknowns identified and investigated",
			"Edge cases documented",
			"Performance characteristics understood",
			"Findings and recommendations documented",
			"Technical risks assessed",
		},
		TokenBudget: 800,
	},
	PhaseS6: {
		ID:          PhaseS6,
		Name:        "Design",
		Objective:   "Complete detailed design of all components",
		Deliverable: "S6-design.md",
		SuccessCriteria: []string{
			"Class diagrams created",
			"Sequence diagrams for key flows",
			"Error handling flowcharts",
			"API documentation complete",
			"Database schema defined (if needed)",
		},
		TokenBudget: 900,
	},
	PhaseS7: {
		ID:          PhaseS7,
		Name:        "Plan",
		Objective:   "Break down implementation into concrete tasks",
		Deliverable: "S7-plan.md",
		SuccessCriteria: []string{
			"Task breakdown with estimates",
			"Dependencies and critical path identified",
			"Risk mitigation strategies defined",
			"Testing plan documented",
			"Deployment and rollout plan created",
		},
		TokenBudget: 1000,
	},
	PhaseS8: {
		ID:          PhaseS8,
		Name:        "Implementation",
		Objective:   "Build the solution",
		Deliverable: "S8-implementation.md",
		SuccessCriteria: []string{
			"All components implemented",
			"Unit tests written and passing",
			"Integration tests passing",
			"Code review completed",
			"Documentation updated",
		},
		TokenBudget: 700,
	},
	PhaseS9: {
		ID:          PhaseS9,
		Name:        "Validation",
		Objective:   "Validate implementation meets requirements",
		Deliverable: "S9-validation.md",
		SuccessCriteria: []string{
			"Full workflow tested end-to-end",
			"Metrics collected and validated",
			"Platform compatibility verified",
			"Performance benchmarked",
			"Quality comparison completed",
		},
		TokenBudget: 600,
	},
	PhaseS10: {
		ID:          PhaseS10,
		Name:        "Deploy",
		Objective:   "Deploy to production",
		Deliverable: "S10-deploy.md",
		SuccessCriteria: []string{
			"Code review approved",
			"Merged to main branch",
			"Documentation updated",
			"Migration guide created",
			"Deployment successful with monitoring",
		},
		TokenBudget: 500,
	},
	PhaseS11: {
		ID:          PhaseS11,
		Name:        "Retrospective",
		Objective:   "Document learnings and improvements",
		Deliverable: "S11-retrospective.md",
		SuccessCriteria: []string{
			"What went well captured",
			"Improvement opportunities identified",
			"Architectural decisions documented",
			"Best practices updated",
			"Future enhancements planned",
		},
		TokenBudget: 1000,
	},
}

// GetAllPhases returns all phase definitions in execution order.
func GetAllPhases() []PhaseDefinition {
	ids := AllPhaseIDs()
	phases := make([]PhaseDefinition, 0, len(ids))
	for _, id := range ids {
		phases = append(phases, PhaseDefinitions[id])
	}
	return phases
}

// GetPhasesFrom returns phases starting from the given phase.
func GetPhasesFrom(startPhase PhaseID) []PhaseDefinition {
	all := GetAllPhases()
	for i, p := range all {
		if p.ID == startPhase {
			return all[i:]
		}
	}
	return all
}

// GetPhaseDependencies returns the dependency phase IDs for a phase.
func GetPhaseDependencies(phaseID PhaseID) []PhaseID {
	deps := PhaseDependencyGraph[phaseID]
	result := make([]PhaseID, 0, len(deps))
	for depID := range deps {
		result = append(result, depID)
	}
	return result
}

// GetPhaseDependenciesWithStrategy returns dependencies with their load strategies.
func GetPhaseDependenciesWithStrategy(phaseID PhaseID) map[PhaseID]LoadStrategy {
	deps, ok := PhaseDependencyGraph[phaseID]
	if !ok {
		return map[PhaseID]LoadStrategy{}
	}
	return deps
}

// GetPhaseConsumers returns phases that depend on the given phase.
func GetPhaseConsumers(phaseID PhaseID) []PhaseID {
	var consumers []PhaseID
	for consumerID, deps := range PhaseDependencyGraph {
		if _, ok := deps[phaseID]; ok {
			consumers = append(consumers, consumerID)
		}
	}
	return consumers
}

// ValidateDependencyGraph checks that the dependency graph is acyclic.
func ValidateDependencyGraph() bool {
	visited := make(map[PhaseID]bool)
	recursionStack := make(map[PhaseID]bool)

	var hasCycle func(PhaseID) bool
	hasCycle = func(phaseID PhaseID) bool {
		visited[phaseID] = true
		recursionStack[phaseID] = true

		for dep := range PhaseDependencyGraph[phaseID] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recursionStack[dep] {
				return true
			}
		}

		delete(recursionStack, phaseID)
		return false
	}

	for phaseID := range PhaseDependencyGraph {
		if !visited[phaseID] {
			if hasCycle(phaseID) {
				return false
			}
		}
	}
	return true
}
