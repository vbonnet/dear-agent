package phaseisolation

// AntiPattern defines a section that indicates scope creep when found in the wrong phase.
type AntiPattern struct {
	Section     string   // Section heading that triggers anti-pattern
	BelongsIn   PhaseID  // Which phase this section belongs in
	Severity    string   // "error" or "warning"
	Explanation string   // Human-readable explanation
	Aliases     []string // Alternative headings that also match
}

// LengthRange defines expected word count range for a phase document.
type LengthRange struct {
	Min int
	Max int
}

// PhaseAntiPatterns maps each phase to its anti-patterns.
var PhaseAntiPatterns = map[PhaseID][]AntiPattern{
	PhaseD1: {
		{
			Section:     "Solution Proposal",
			BelongsIn:   PhaseD4,
			Severity:    "error",
			Explanation: "D1 validates the problem; detailed solution specs belong in D4",
			Aliases:     []string{"Proposed Solution", "Solution Options", "Solutions"},
		},
		{
			Section:     "Implementation Plan",
			BelongsIn:   PhaseS7,
			Severity:    "error",
			Explanation: "Implementation planning happens in S7 Plan phase",
			Aliases:     []string{"Implementation", "Plan", "Roadmap", "Timeline"},
		},
		{
			Section:     "Architecture",
			BelongsIn:   PhaseS6,
			Severity:    "warning",
			Explanation: "Architecture design happens in S6 Design phase",
			Aliases:     []string{"Design", "System Design", "Technical Design"},
		},
	},
	PhaseD2: {
		{
			Section:     "Final Decision",
			BelongsIn:   PhaseD3,
			Severity:    "error",
			Explanation: "D2 explores options; decision happens in D3",
			Aliases:     []string{"Chosen Solution", "Decision", "Selected Approach"},
		},
		{
			Section:     "Architecture Design",
			BelongsIn:   PhaseS6,
			Severity:    "warning",
			Explanation: "Detailed design happens in S6 Design phase",
			Aliases:     []string{"Technical Architecture", "System Architecture"},
		},
		{
			Section:     "Requirements",
			BelongsIn:   PhaseD4,
			Severity:    "warning",
			Explanation: "Requirements are specified in D4",
		},
	},
	PhaseD3: {
		{
			Section:     "Acceptance Criteria",
			BelongsIn:   PhaseD4,
			Severity:    "error",
			Explanation: "Acceptance criteria are requirements, defined in D4",
			Aliases:     []string{"Accept Criteria", "AC", "Success Criteria"},
		},
		{
			Section:     "Requirements",
			BelongsIn:   PhaseD4,
			Severity:    "error",
			Explanation: "Requirements are specified in D4, not D3",
			Aliases:     []string{"Functional Requirements", "Requirements Specification", "Specs"},
		},
		{
			Section:     "Task Breakdown",
			BelongsIn:   PhaseS7,
			Severity:    "error",
			Explanation: "Task planning happens in S7 Plan phase, not D3",
			Aliases:     []string{"Tasks", "Implementation Tasks", "Action Items", "Work Items"},
		},
		{
			Section:     "Deployment",
			BelongsIn:   PhaseS10,
			Severity:    "error",
			Explanation: "Deployment planning happens in S10 Deploy phase",
			Aliases:     []string{"Deploy Plan", "Rollout", "Release", "Deployment Plan"},
		},
		{
			Section:     "Implementation",
			BelongsIn:   PhaseS8,
			Severity:    "warning",
			Explanation: "Implementation happens in S8, not during decision phase",
			Aliases:     []string{"Code", "Implementation Details"},
		},
	},
	PhaseD4: {
		{
			Section:     "Implementation Code",
			BelongsIn:   PhaseS8,
			Severity:    "error",
			Explanation: "Code belongs in S8 Implementation phase",
			Aliases:     []string{"Code", "Implementation", "Source Code"},
		},
		{
			Section:     "Test Plan",
			BelongsIn:   PhaseS7,
			Severity:    "warning",
			Explanation: "Test planning happens in S7 Plan phase",
			Aliases:     []string{"Testing Strategy", "Test Strategy", "QA Plan"},
		},
		{
			Section:     "API Documentation",
			BelongsIn:   PhaseS5,
			Severity:    "warning",
			Explanation: "Detailed API research happens in S5 Research phase",
			Aliases:     []string{"API Spec", "API Reference"},
		},
		{
			Section:     "Deployment",
			BelongsIn:   PhaseS10,
			Severity:    "warning",
			Explanation: "Deployment planning happens in S10",
		},
	},
	PhaseS4: {
		{
			Section:     "Implementation",
			BelongsIn:   PhaseS8,
			Severity:    "error",
			Explanation: "Implementation happens after stakeholder alignment",
			Aliases:     []string{"Code", "Implementation Code"},
		},
		{
			Section:     "Deployment",
			BelongsIn:   PhaseS10,
			Severity:    "warning",
			Explanation: "Deployment happens in S10",
		},
	},
	PhaseS5: {
		{
			Section:     "Code Implementation",
			BelongsIn:   PhaseS8,
			Severity:    "error",
			Explanation: "S5 researches HOW to build; S8 implements",
			Aliases:     []string{"Implementation", "Code", "Source Code"},
		},
		{
			Section:     "Deployment Plan",
			BelongsIn:   PhaseS10,
			Severity:    "warning",
			Explanation: "Deployment planning happens in S10",
			Aliases:     []string{"Deploy", "Rollout Plan"},
		},
	},
	PhaseS6: {
		{
			Section:     "Implementation",
			BelongsIn:   PhaseS8,
			Severity:    "error",
			Explanation: "S6 designs; S8 implements",
			Aliases:     []string{"Code", "Implementation Code", "Source Code"},
		},
		{
			Section:     "Task Breakdown",
			BelongsIn:   PhaseS7,
			Severity:    "warning",
			Explanation: "Task planning happens in S7 Plan phase",
			Aliases:     []string{"Tasks", "Implementation Tasks"},
		},
	},
	PhaseS7: {
		{
			Section:     "Implementation",
			BelongsIn:   PhaseS8,
			Severity:    "error",
			Explanation: "S7 plans; S8 implements",
			Aliases:     []string{"Code", "Implementation Code"},
		},
	},
	PhaseS8: {
		{
			Section:     "Deployment",
			BelongsIn:   PhaseS10,
			Severity:    "warning",
			Explanation: "Deployment happens in S10 Deploy phase",
			Aliases:     []string{"Deploy", "Rollout", "Release"},
		},
	},
	PhaseS9:  {}, // Validation phase - flexible
	PhaseS10: {}, // Deploy phase - flexible
	PhaseS11: {}, // Retrospective - flexible
}

// PhaseRequiredSections maps each phase to its required sections.
var PhaseRequiredSections = map[PhaseID][]string{
	PhaseD1:  {"Problem Definition", "Evidence", "Impact Analysis", "Decision"},
	PhaseD2:  {"Solutions Overview", "Tradeoff Analysis", "Recommendation"},
	PhaseD3:  {"Decision Matrix", "Chosen Approach", "Risk Assessment"},
	PhaseD4:  {"Architecture Overview", "Components", "Acceptance Criteria"},
	PhaseS4:  {"Stakeholder Feedback", "Alignment Confirmation"},
	PhaseS5:  {"Research Questions", "Investigation Results"},
	PhaseS6:  {"Design Overview", "API Documentation"},
	PhaseS7:  {"Task Breakdown", "Dependencies", "Testing Plan"},
	PhaseS8:  {"Implementation Summary", "Tests Written"},
	PhaseS9:  {"Test Results", "Validation Summary"},
	PhaseS10: {"Deployment Summary", "Code Review"},
	PhaseS11: {"What Went Well", "Improvements"},
}

// PhaseLengthRanges maps each phase to its expected word count range.
var PhaseLengthRanges = map[PhaseID]LengthRange{
	PhaseD1:  {Min: 1000, Max: 3000},
	PhaseD2:  {Min: 2000, Max: 5000},
	PhaseD3:  {Min: 1000, Max: 2500},
	PhaseD4:  {Min: 1500, Max: 3000},
	PhaseS4:  {Min: 500, Max: 1500},
	PhaseS5:  {Min: 1500, Max: 4000},
	PhaseS6:  {Min: 2000, Max: 6000},
	PhaseS7:  {Min: 1500, Max: 3000},
	PhaseS8:  {Min: 500, Max: 2000},
	PhaseS9:  {Min: 1000, Max: 2000},
	PhaseS10: {Min: 500, Max: 1500},
	PhaseS11: {Min: 1000, Max: 2000},
}
