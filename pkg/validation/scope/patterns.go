package scope

// PhaseAntiPatterns maps phase IDs to their anti-patterns
var PhaseAntiPatterns = map[PhaseID][]AntiPattern{
	PhaseProblem: {
		{
			Section:     "Solution Proposal",
			BelongsIn:   PhaseSpec,
			Severity:    SeverityError,
			Explanation: "PROBLEM validates the problem; detailed solution specs belong in SPEC",
			Aliases:     []string{"Proposed Solution", "Solution Options", "Solutions"},
		},
		{
			Section:     "Implementation Plan",
			BelongsIn:   PhaseSetup,
			Severity:    SeverityError,
			Explanation: "Implementation planning happens in SETUP phase",
			Aliases:     []string{"Implementation", "Plan", "Roadmap", "Timeline"},
		},
		{
			Section:     "Architecture",
			BelongsIn:   PhasePlan,
			Severity:    SeverityWarning,
			Explanation: "Architecture design happens in PLAN phase",
			Aliases:     []string{"Design", "System Design", "Technical Design"},
		},
	},

	PhaseResearch: {
		{
			Section:     "Final Decision",
			BelongsIn:   PhaseDesign,
			Severity:    SeverityError,
			Explanation: "RESEARCH explores options; decision happens in DESIGN",
			Aliases:     []string{"Chosen Solution", "Decision", "Selected Approach"},
		},
		{
			Section:     "Architecture Design",
			BelongsIn:   PhasePlan,
			Severity:    SeverityWarning,
			Explanation: "Detailed design happens in PLAN phase",
			Aliases:     []string{"Technical Architecture", "System Architecture"},
		},
		{
			Section:     "Requirements",
			BelongsIn:   PhaseSpec,
			Severity:    SeverityWarning,
			Explanation: "Requirements are specified in SPEC",
		},
	},

	PhaseDesign: {
		{
			Section:     "Acceptance Criteria",
			BelongsIn:   PhaseSpec,
			Severity:    SeverityError,
			Explanation: "Acceptance criteria are requirements, defined in SPEC",
			Aliases:     []string{"Accept Criteria", "AC", "Success Criteria"},
		},
		{
			Section:     "Requirements",
			BelongsIn:   PhaseSpec,
			Severity:    SeverityError,
			Explanation: "Requirements are specified in SPEC, not DESIGN",
			Aliases:     []string{"Functional Requirements", "Requirements Specification", "Specs"},
		},
		{
			Section:     "Task Breakdown",
			BelongsIn:   PhaseSetup,
			Severity:    SeverityError,
			Explanation: "Task planning happens in SETUP phase, not DESIGN",
			Aliases:     []string{"Tasks", "Implementation Tasks", "Action Items", "Work Items"},
		},
		{
			Section:     "Deployment",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityError,
			Explanation: "Deployment planning happens in BUILD phase",
			Aliases:     []string{"Deploy Plan", "Rollout", "Release", "Deployment Plan"},
		},
		{
			Section:     "Implementation",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityWarning,
			Explanation: "Implementation happens in BUILD, not during decision phase",
			Aliases:     []string{"Code", "Implementation Details"},
		},
	},

	PhaseSpec: {
		{
			Section:     "Implementation Code",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityError,
			Explanation: "Code belongs in BUILD phase",
			Aliases:     []string{"Code", "Source Code"},
		},
		{
			Section:     "Implementation",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityError,
			Explanation: "Implementation happens after stakeholder alignment",
			Aliases:     []string{"Implementation Code"},
		},
		{
			Section:     "Test Plan",
			BelongsIn:   PhaseSetup,
			Severity:    SeverityWarning,
			Explanation: "Test planning happens in SETUP phase",
			Aliases:     []string{"Testing Strategy", "Test Strategy", "QA Plan"},
		},
		{
			Section:     "Deployment",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityWarning,
			Explanation: "Deployment planning happens in BUILD",
		},
	},

	PhasePlan: {
		{
			Section:     "Implementation",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityError,
			Explanation: "PLAN designs; BUILD implements",
			Aliases:     []string{"Code", "Implementation Code", "Source Code"},
		},
		{
			Section:     "Task Breakdown",
			BelongsIn:   PhaseSetup,
			Severity:    SeverityWarning,
			Explanation: "Task planning happens in SETUP phase",
			Aliases:     []string{"Tasks", "Implementation Tasks"},
		},
		{
			Section:     "Code Implementation",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityError,
			Explanation: "PLAN researches HOW to build; BUILD implements",
		},
		{
			Section:     "Deployment Plan",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityWarning,
			Explanation: "Deployment planning happens in BUILD",
			Aliases:     []string{"Deploy", "Rollout Plan"},
		},
	},

	PhaseSetup: {
		{
			Section:     "Implementation",
			BelongsIn:   PhaseBuild,
			Severity:    SeverityError,
			Explanation: "SETUP plans; BUILD implements",
			Aliases:     []string{"Code", "Implementation Code"},
		},
	},

	PhaseBuild: {},
	PhaseRetro: {},
}

// PhaseRequiredSections maps phase IDs to their required sections
var PhaseRequiredSections = map[PhaseID][]string{
	PhaseProblem: {
		"Problem Definition",
		"Evidence",
		"Impact Analysis",
		"Decision",
	},

	PhaseResearch: {
		"Solutions Overview",
		"Tradeoff Analysis",
		"Recommendation",
	},

	PhaseDesign: {
		"Decision Matrix",
		"Chosen Approach",
		"Risk Assessment",
	},

	PhaseSpec: {
		"Architecture Overview",
		"Components",
		"Acceptance Criteria",
		"Stakeholder Feedback",
		"Alignment Confirmation",
	},

	PhasePlan: {
		"Design Overview",
		"API Documentation",
		"Research Questions",
		"Investigation Results",
	},

	PhaseSetup: {
		"Task Breakdown",
		"Dependencies",
		"Testing Plan",
	},

	PhaseBuild: {
		"Implementation Summary",
		"Tests Written",
		"Test Results",
		"Validation Summary",
		"Deployment Summary",
		"Code Review",
	},

	PhaseRetro: {
		"What Went Well",
		"Improvements",
	},
}

// PhaseLengthRanges defines expected word count ranges for each phase
var PhaseLengthRanges = map[PhaseID]LengthRange{
	PhaseProblem:  {Min: 1000, Max: 3000},
	PhaseResearch: {Min: 2000, Max: 5000},
	PhaseDesign:   {Min: 1000, Max: 2500},
	PhaseSpec:     {Min: 1500, Max: 3000},
	PhasePlan:     {Min: 2000, Max: 6000},
	PhaseSetup:    {Min: 1500, Max: 3000},
	PhaseBuild:    {Min: 500, Max: 2000},
	PhaseRetro:    {Min: 1000, Max: 2000},
}
