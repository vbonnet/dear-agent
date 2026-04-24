package phaseisolation

import (
	"strings"
	"testing"
)

func TestAllPhaseIDs(t *testing.T) {
	ids := AllPhaseIDs()
	if len(ids) != 12 {
		t.Fatalf("expected 12 phases, got %d", len(ids))
	}
	if ids[0] != PhaseD1 {
		t.Errorf("expected first phase D1, got %s", ids[0])
	}
	if ids[11] != PhaseS11 {
		t.Errorf("expected last phase S11, got %s", ids[11])
	}
}

func TestGetAllPhases(t *testing.T) {
	phases := GetAllPhases()
	if len(phases) != 12 {
		t.Fatalf("expected 12 phases, got %d", len(phases))
	}
	if phases[0].ID != PhaseD1 {
		t.Errorf("expected first phase D1, got %s", phases[0].ID)
	}
}

func TestGetPhasesFrom(t *testing.T) {
	phases := GetPhasesFrom(PhaseS4)
	if len(phases) != 8 {
		t.Fatalf("expected 8 phases from S4, got %d", len(phases))
	}
	if phases[0].ID != PhaseS4 {
		t.Errorf("expected first phase S4, got %s", phases[0].ID)
	}
}

func TestGetPhaseDependencies(t *testing.T) {
	deps := GetPhaseDependencies(PhaseD1)
	if len(deps) != 0 {
		t.Errorf("D1 should have no dependencies, got %d", len(deps))
	}

	deps = GetPhaseDependencies(PhaseD3)
	if len(deps) != 2 {
		t.Errorf("D3 should have 2 dependencies, got %d", len(deps))
	}
}

func TestValidateDependencyGraph(t *testing.T) {
	if !ValidateDependencyGraph() {
		t.Error("dependency graph should be acyclic")
	}
}

func TestGetPhaseConsumers(t *testing.T) {
	consumers := GetPhaseConsumers(PhaseD4)
	if len(consumers) == 0 {
		t.Error("D4 should have consumers")
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		text    string
		pattern string
		want    bool
	}{
		{"because it broke", "because|issue|problem", true},
		{"no match here", "because|issue|problem", false},
		{"Must finish", "must|required", true},
		{"nothing", "must|required", false},
		{"", "must", false},
		{"must", "", false},
	}

	for _, tt := range tests {
		got := MatchesPattern(tt.text, tt.pattern)
		if got != tt.want {
			t.Errorf("MatchesPattern(%q, %q) = %v, want %v", tt.text, tt.pattern, got, tt.want)
		}
	}
}

func TestMatchesPatternCaseSensitive(t *testing.T) {
	if MatchesPattern("MUST", "must", MatchOptions{CaseSensitive: true, WholeWord: true}) {
		t.Error("case-sensitive match should not match MUST with must")
	}
	if !MatchesPattern("MUST", "must") {
		t.Error("case-insensitive match should match MUST with must")
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"hello world", 2},
		{"  spaces  around  ", 2},
		{"one", 1},
		{"", 0},
		{"  ", 0},
	}

	for _, tt := range tests {
		got := CountWords(tt.text)
		if got != tt.want {
			t.Errorf("CountWords(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

func TestTemplateBuilder(t *testing.T) {
	tb := NewTemplateBuilder()
	result := tb.
		Heading(1, "Title").
		Text("Introduction").
		List([]string{"Item 1", "Item 2"}, false).
		Build()

	if !strings.Contains(result, "# Title") {
		t.Error("missing heading")
	}
	if !strings.Contains(result, "Introduction") {
		t.Error("missing text")
	}
	if !strings.Contains(result, "- Item 1") {
		t.Error("missing list item")
	}
}

func TestTemplateBuilderNumberedList(t *testing.T) {
	tb := NewTemplateBuilder()
	result := tb.List([]string{"First", "Second"}, true).Build()

	if !strings.Contains(result, "1. First") {
		t.Error("missing numbered item 1")
	}
	if !strings.Contains(result, "2. Second") {
		t.Error("missing numbered item 2")
	}
}

func TestTemplateBuilderEmpty(t *testing.T) {
	tb := NewTemplateBuilder()
	if tb.Build() != "" {
		t.Error("empty builder should return empty string")
	}
}

func TestSectionParser(t *testing.T) {
	parser := NewSectionParser()
	markdown := "# Title\n## Subtitle\n### H3\nSome text\n## Another Section"
	sections := parser.Parse(markdown)

	if len(sections) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(sections))
	}
	if sections[0].Heading != "Title" {
		t.Errorf("expected 'Title', got %q", sections[0].Heading)
	}
	if sections[0].Level != 1 {
		t.Errorf("expected level 1, got %d", sections[0].Level)
	}
	if sections[1].Heading != "Subtitle" {
		t.Errorf("expected 'Subtitle', got %q", sections[1].Heading)
	}
}

func TestSectionParserSkipsCodeBlocks(t *testing.T) {
	parser := NewSectionParser()
	markdown := "# Real Heading\n```\n# Fake Heading\n```\n## Another Real"
	sections := parser.Parse(markdown)

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
}

func TestSectionParserSkipsFrontmatter(t *testing.T) {
	parser := NewSectionParser()
	markdown := "---\ntitle: test\n---\n# Real Heading"
	sections := parser.Parse(markdown)

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if sections[0].Heading != "Real Heading" {
		t.Errorf("expected 'Real Heading', got %q", sections[0].Heading)
	}
}

func TestFuzzyMatch(t *testing.T) {
	parser := NewSectionParser()

	if !parser.FuzzyMatch("Acceptance Criteria", "Accept Criteria", 0.75) {
		t.Error("should fuzzy match 'Acceptance Criteria' with 'Accept Criteria'")
	}
	if parser.FuzzyMatch("Task Breakdown", "Deployment", 0.75) {
		t.Error("should not fuzzy match 'Task Breakdown' with 'Deployment'")
	}
}

func TestFindSections(t *testing.T) {
	parser := NewSectionParser()
	sections := []Section{
		{Heading: "Acceptance Criteria", Level: 2, StartLine: 10},
		{Heading: "Task Breakdown", Level: 2, StartLine: 20},
	}

	found := parser.FindSections(sections, "Accept Criteria", true)
	if len(found) != 1 {
		t.Fatalf("expected 1 match, got %d", len(found))
	}
	if found[0].Heading != "Acceptance Criteria" {
		t.Errorf("expected 'Acceptance Criteria', got %q", found[0].Heading)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		got := levenshteinDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestScopeValidator(t *testing.T) {
	parser := NewSectionParser()
	validator := NewScopeValidator(parser)

	// Document with anti-pattern: D3 has "Acceptance Criteria" (belongs in D4)
	doc := `# D3 Approach Decision

## Decision Matrix
Some criteria...

## Chosen Approach
Approach A

## Risk Assessment
Low risk

## Acceptance Criteria
- Must be fast
- Must be correct
`

	result := validator.Validate(PhaseD3, doc)

	if result.Passed {
		t.Error("expected validation to fail due to anti-pattern")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	// Check that the anti-pattern was detected
	found := false
	for _, e := range result.Errors {
		if e.Type == "anti-pattern" && strings.Contains(e.Message, "Acceptance Criteria") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Acceptance Criteria' anti-pattern to be detected")
	}
}

func TestScopeValidatorPasses(t *testing.T) {
	parser := NewSectionParser()
	validator := NewScopeValidator(parser)

	// Valid D3 document (has required sections, no anti-patterns)
	doc := strings.Repeat("word ", 1500) + `

## Decision Matrix
Scoring criteria

## Chosen Approach
Approach A selected

## Risk Assessment
Low risk identified
`

	result := validator.Validate(PhaseD3, doc)
	if !result.Passed {
		t.Errorf("expected validation to pass, errors: %v", result.Errors)
	}
}

func TestScopeValidatorOverride(t *testing.T) {
	parser := NewSectionParser()
	validator := NewScopeValidator(parser)

	doc := "## Acceptance Criteria\n- test\n"
	result := validator.Validate(PhaseD3, doc, ValidationOptions{Override: true})

	if !result.Passed {
		t.Error("expected validation to pass with override")
	}
}

func TestScopeValidatorFormatReport(t *testing.T) {
	parser := NewSectionParser()
	validator := NewScopeValidator(parser)

	doc := "## Acceptance Criteria\n- test\n"
	result := validator.Validate(PhaseD3, doc)

	report := validator.FormatReport(result)
	if !strings.Contains(report, "Phase Scope Validation: D3") {
		t.Error("report should contain phase ID")
	}
}

func TestContextCompiler(t *testing.T) {
	cc := NewContextCompiler()
	phase := PhaseDefinitions[PhaseD1]

	ctx := cc.Compile(phase, map[PhaseID]*PhaseArtifact{}, "session-123", "/tmp/project")

	if ctx.PhaseName != "Problem Validation" {
		t.Errorf("expected 'Problem Validation', got %q", ctx.PhaseName)
	}
	if ctx.Metadata.SessionID != "session-123" {
		t.Errorf("expected session ID 'session-123', got %q", ctx.Metadata.SessionID)
	}
	if len(ctx.PriorArtifacts) != 0 {
		t.Errorf("D1 should have no prior artifacts, got %d", len(ctx.PriorArtifacts))
	}
}

func TestContextCompilerWithDependencies(t *testing.T) {
	cc := NewContextCompiler()
	phase := PhaseDefinitions[PhaseD3]

	artifacts := map[PhaseID]*PhaseArtifact{
		PhaseD1: {
			PhaseID:  PhaseD1,
			Summary:  "D1 summary",
			FullPath: "/tmp/D1.md",
			Metadata: ArtifactMetadata{TokenCount: 100},
		},
		PhaseD2: {
			PhaseID:  PhaseD2,
			Summary:  "D2 summary",
			FullPath: "/tmp/D2.md",
			Metadata: ArtifactMetadata{TokenCount: 200},
		},
	}

	ctx := cc.Compile(phase, artifacts, "session-123", "/tmp/project")

	if len(ctx.PriorArtifacts) != 2 {
		t.Errorf("D3 should have 2 prior artifacts, got %d", len(ctx.PriorArtifacts))
	}
}

func TestSummarizeArtifact(t *testing.T) {
	cc := NewContextCompiler()

	content := `# D1 Problem Validation

- First finding about the problem
- Second finding with details
- Third finding about impact

Decision: Proceed with solution A

Some metric: 40% improvement achieved
`

	summary := cc.SummarizeArtifact(PhaseD1, content)

	if !strings.Contains(summary, "Problem Validation") {
		t.Error("summary should contain phase name")
	}
	if !strings.Contains(summary, "Key findings") {
		t.Error("summary should contain key findings")
	}
}

func TestDetectPlatform(t *testing.T) {
	// Default should be unknown in test environment
	p := DetectPlatform()
	if p != PlatformUnknown {
		// Could be claude-code if running inside CC, that's OK
		if p != PlatformClaudeCode {
			t.Errorf("unexpected platform: %s", p)
		}
	}
}

func TestV1ToV2PhaseMap(t *testing.T) {
	if V1ToV2PhaseMap[PhaseD1] != V2Problem {
		t.Errorf("D1 should map to PROBLEM, got %s", V1ToV2PhaseMap[PhaseD1])
	}
	if V1ToV2PhaseMap[PhaseS11] != V2Retro {
		t.Errorf("S11 should map to RETRO, got %s", V1ToV2PhaseMap[PhaseS11])
	}
}

func TestPhaseDefinitionsComplete(t *testing.T) {
	for _, id := range AllPhaseIDs() {
		def, ok := PhaseDefinitions[id]
		if !ok {
			t.Errorf("missing definition for phase %s", id)
			continue
		}
		if def.Name == "" {
			t.Errorf("phase %s has empty name", id)
		}
		if def.Deliverable == "" {
			t.Errorf("phase %s has empty deliverable", id)
		}
		if len(def.SuccessCriteria) == 0 {
			t.Errorf("phase %s has no success criteria", id)
		}
		if def.TokenBudget == 0 {
			t.Errorf("phase %s has zero token budget", id)
		}
	}
}
