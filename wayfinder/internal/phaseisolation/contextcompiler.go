package phaseisolation

import (
	"fmt"
	"regexp"
	"strings"
)

// ContextCompiler compiles minimal context for phase execution.
type ContextCompiler struct{}

// NewContextCompiler creates a new ContextCompiler.
func NewContextCompiler() *ContextCompiler {
	return &ContextCompiler{}
}

// Compile compiles minimal context for a phase.
func (cc *ContextCompiler) Compile(
	phase PhaseDefinition,
	artifacts map[PhaseID]*PhaseArtifact,
	sessionID, projectPath string,
) PhaseContext {
	deps := GetPhaseDependenciesWithStrategy(phase.ID)

	var priorArtifacts []ArtifactSummary
	for depID, strategy := range deps {
		artifact, ok := artifacts[depID]
		if !ok {
			continue
		}
		if strategy == LoadFull {
			priorArtifacts = append(priorArtifacts, cc.createFullArtifactEntry(artifact))
		} else {
			priorArtifacts = append(priorArtifacts, cc.createArtifactSummary(artifact))
		}
	}

	systemPrompt := cc.buildSystemPrompt(phase, priorArtifacts)
	outputSpec := cc.buildOutputSpec(phase)
	estimatedTokens := cc.estimateTokens(systemPrompt, priorArtifacts, phase)

	return PhaseContext{
		PhaseName:         phase.Name,
		PhaseObjective:    phase.Objective,
		PhaseSystemPrompt: systemPrompt,
		PriorArtifacts:    priorArtifacts,
		SuccessCriteria:   phase.SuccessCriteria,
		OutputSpec:        outputSpec,
		Metadata: ContextMetadata{
			SessionID:       sessionID,
			ProjectPath:     projectPath,
			TokenBudget:     phase.TokenBudget,
			EstimatedTokens: estimatedTokens,
		},
	}
}

func (cc *ContextCompiler) createArtifactSummary(artifact *PhaseArtifact) ArtifactSummary {
	phase := PhaseDefinitions[artifact.PhaseID]
	return ArtifactSummary{
		PhaseID:         artifact.PhaseID,
		PhaseName:       phase.Name,
		Summary:         artifact.Summary,
		DeliverablePath: artifact.FullPath,
		TokenCount:      artifact.Metadata.TokenCount,
	}
}

func (cc *ContextCompiler) createFullArtifactEntry(artifact *PhaseArtifact) ArtifactSummary {
	phase := PhaseDefinitions[artifact.PhaseID]
	return ArtifactSummary{
		PhaseID:         artifact.PhaseID,
		PhaseName:       phase.Name,
		Summary:         artifact.Summary,
		DeliverablePath: artifact.FullPath,
		TokenCount:      artifact.Metadata.TokenCount,
	}
}

func (cc *ContextCompiler) buildSystemPrompt(phase PhaseDefinition, priorArtifacts []ArtifactSummary) string {
	tb := NewTemplateBuilder()
	tb.Heading(1, fmt.Sprintf("Phase %s: %s", phase.ID, phase.Name))
	tb.Text(fmt.Sprintf("**Objective**: %s", phase.Objective))

	if len(priorArtifacts) > 0 {
		tb.Heading(2, "Context from Prior Phases")
		tb.Text("You have access to summaries of previous phases. Read the full deliverables if you need more detail.")

		for _, artifact := range priorArtifacts {
			tb.Heading(3, fmt.Sprintf("%s: %s", artifact.PhaseID, artifact.PhaseName))
			tb.Text(artifact.Summary)
			tb.Text(fmt.Sprintf("**Full deliverable**: `%s`", artifact.DeliverablePath))
		}
	} else {
		tb.Heading(2, "Context")
		tb.Text("This is the first phase. No prior phases available.")
	}

	tb.Heading(2, "Success Criteria")
	tb.Text("Your deliverable must achieve:")
	tb.List(phase.SuccessCriteria, false)

	tb.Heading(2, "Deliverable")
	tb.Text(fmt.Sprintf("Create: **%s**", phase.Deliverable))
	tb.Text("Format: Markdown document")

	tb.Heading(2, "Execution Guidelines")
	tb.List([]string{
		"**Read prior deliverables**: Review full artifacts from dependencies if needed",
		"**Focus on objective**: Stay focused on this phase's specific objective",
		"**Be thorough but concise**: Comprehensive analysis within reasonable scope",
		"**Make decisions**: This is one phase of larger workflow - make reasonable decisions",
		"**Document rationale**: Explain key decisions and tradeoffs",
	}, true)

	tb.Text(cc.getPhaseSpecificGuidance(phase))
	return tb.Build()
}

func (cc *ContextCompiler) getPhaseSpecificGuidance(phase PhaseDefinition) string {
	guidance := map[PhaseID]string{
		PhaseD1:  "**Problem Validation Focus**: Validate with evidence, quantify impact, assess feasibility.",
		PhaseD2:  "**Solutions Search Focus**: Research 3-5 alternatives, compare tradeoffs, provide recommendation.",
		PhaseD3:  "**Approach Decision Focus**: Use decision matrix, assess risks, define success criteria.",
		PhaseD4:  "**Solution Requirements Focus**: Specify architecture, define interfaces, document acceptance criteria.",
		PhaseS4:  "**Stakeholder Alignment Focus**: Present findings, confirm alignment, obtain sign-off.",
		PhaseS5:  "**Research Focus**: Investigate unknowns, document edge cases, assess technical risks.",
		PhaseS6:  "**Design Focus**: Create diagrams, document APIs, design error handling.",
		PhaseS7:  "**Plan Focus**: Break down tasks, identify critical path, plan testing and deployment.",
		PhaseS8:  "**Implementation Focus**: Build components, write tests, document code.",
		PhaseS9:  "**Validation Focus**: Test end-to-end, measure metrics, validate quality.",
		PhaseS10: "**Deploy Focus**: Review code, merge changes, deploy with monitoring.",
		PhaseS11: "**Retrospective Focus**: Capture learnings, document decisions, plan improvements.",
	}
	if g, ok := guidance[phase.ID]; ok {
		return g
	}
	return ""
}

func (cc *ContextCompiler) buildOutputSpec(phase PhaseDefinition) OutputSpecification {
	sectionsByPhase := map[PhaseID][]string{
		PhaseD1:  {"Problem Definition", "Evidence", "Impact Analysis", "Feasibility Assessment", "Decision"},
		PhaseD2:  {"Solutions Overview", "Solution Evaluation", "Tradeoff Analysis", "Recommendation"},
		PhaseD3:  {"Decision Matrix", "Chosen Approach", "Risk Assessment", "Success Criteria", "Implementation Strategy"},
		PhaseD4:  {"Architecture Overview", "Components", "Data Structures", "Interfaces", "Acceptance Criteria"},
		PhaseS4:  {"Presentation Summary", "Stakeholder Feedback", "Alignment Confirmation", "Action Items"},
		PhaseS5:  {"Research Questions", "Investigation Results", "Edge Cases", "Recommendations"},
		PhaseS6:  {"Class Diagrams", "Sequence Diagrams", "Error Handling", "API Documentation"},
		PhaseS7:  {"Task Breakdown", "Dependencies", "Testing Plan", "Deployment Plan"},
		PhaseS8:  {"Implementation Summary", "Components Built", "Tests Written", "Documentation"},
		PhaseS9:  {"Test Results", "Metrics Collected", "Quality Analysis", "Validation Summary"},
		PhaseS10: {"Deployment Summary", "Code Review", "Migration Guide", "Monitoring"},
		PhaseS11: {"What Went Well", "Improvements", "Learnings", "Future Enhancements"},
	}

	sections := sectionsByPhase[phase.ID]
	if sections == nil {
		sections = []string{"Overview", "Details", "Conclusion"}
	}

	return OutputSpecification{
		Filename: phase.Deliverable,
		Format:   "markdown",
		Sections: sections,
	}
}

func (cc *ContextCompiler) estimateTokens(systemPrompt string, priorArtifacts []ArtifactSummary, phase PhaseDefinition) int {
	totalChars := len(systemPrompt)

	for _, artifact := range priorArtifacts {
		totalChars += len(artifact.Summary)
		totalChars += len(artifact.PhaseName)
		totalChars += len(artifact.DeliverablePath)
	}

	for _, criterion := range phase.SuccessCriteria {
		totalChars += len(criterion)
	}

	return (totalChars + 3) / 4 // ~4 chars per token
}

// SummarizeArtifact creates a summary of a phase artifact (target: 100-200 tokens).
func (cc *ContextCompiler) SummarizeArtifact(phaseID PhaseID, fullContent string) string {
	phase := PhaseDefinitions[phaseID]
	sections := cc.extractKeySections(fullContent)

	var summary []string
	summary = append(summary, fmt.Sprintf("**%s** completed.", phase.Name))
	summary = append(summary, "")

	if len(sections.keyFindings) > 0 {
		summary = append(summary, "Key findings:")
		for _, finding := range sections.keyFindings {
			summary = append(summary, "- "+finding)
		}
		summary = append(summary, "")
	}

	if sections.decision != "" {
		summary = append(summary, "Decision: "+sections.decision)
		summary = append(summary, "")
	}

	if len(sections.metrics) > 0 {
		summary = append(summary, "Metrics:")
		for _, metric := range sections.metrics {
			summary = append(summary, "- "+metric)
		}
	}

	result := strings.Join(summary, "\n")
	if len(result) > 1000 {
		return result[:800] + "..."
	}
	return result
}

type keySections struct {
	keyFindings []string
	decision    string
	metrics     []string
}

var metricsRegex = regexp.MustCompile(`\d+(%|\s*(tokens|ms|MB|KB|seconds))`)

func (cc *ContextCompiler) extractKeySections(content string) keySections {
	var result keySections
	lines := strings.Split(content, "\n")

	// Look for decision/recommendation
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "decision:") || strings.Contains(lower, "recommendation:") {
			cleaned := strings.TrimLeft(line, "#* \t-")
			result.decision = strings.TrimSpace(cleaned)
			break
		}
	}

	// Look for key findings (bullet points in first 50%)
	halfwayPoint := len(lines) / 2
	bulletCount := 0
	for i := 0; i < halfwayPoint && bulletCount < 5; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "•") {
			finding := strings.TrimLeft(line, "-*• \t")
			finding = strings.TrimSpace(finding)
			if len(finding) > 10 && len(finding) < 200 {
				result.keyFindings = append(result.keyFindings, finding)
				bulletCount++
			}
		}
	}

	// Look for metrics
	for _, line := range lines {
		if metricsRegex.MatchString(line) {
			metric := strings.TrimLeft(line, "#* \t-")
			metric = strings.TrimSpace(metric)
			if len(metric) < 100 {
				result.metrics = append(result.metrics, metric)
				if len(result.metrics) >= 3 {
					break
				}
			}
		}
	}

	return result
}
