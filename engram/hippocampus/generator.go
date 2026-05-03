package hippocampus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateConsolidationArtifact creates structured markdown summary.
//
// Phase 5 V1: Simple template-based generation.
// Phase 5 V2+: Enhanced with Go template system.
func (h *Hippocampus) generateConsolidationArtifact(c *Consolidation) error {
	// Generate markdown content
	content := h.renderConsolidation(c)

	// Create artifact path
	artifactPath := filepath.Join(
		h.archiveDir,
		fmt.Sprintf("%s.md", c.Timestamp.Format("2006-01-02-15-04-05")),
	)

	// Write artifact
	if err := os.WriteFile(artifactPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write artifact: %w", err)
	}

	return nil
}

// renderConsolidation generates markdown content from consolidation data.
func (h *Hippocampus) renderConsolidation(c *Consolidation) string {
	var sb strings.Builder

	renderHeader(&sb, c)
	renderDecisions(&sb, c)
	renderOutcomes(&sb, c)
	renderLearnings(&sb, c)
	renderActivePlan(&sb, c)
	renderEngrams(&sb, c)
	renderFooter(&sb, c)

	return sb.String()
}

// renderHeader renders the consolidation header with metadata
func renderHeader(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("# Sleep Cycle Consolidation\n\n")
	fmt.Fprintf(sb, "**Session ID**: %s\n", c.SessionID)
	fmt.Fprintf(sb, "**Timestamp**: %s\n", c.Timestamp.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(sb, "**Tokens Before**: %d\n", c.TokensBefore)
	fmt.Fprintf(sb, "**Tokens After**: %d\n", c.TokensAfter)

	if c.TokensBefore > 0 {
		reduction := 100.0 * float64(c.TokensBefore-c.TokensAfter) / float64(c.TokensBefore)
		fmt.Fprintf(sb, "**Reduction**: %.1f%%\n\n", reduction)
	} else {
		sb.WriteString("**Reduction**: N/A\n\n")
	}

	sb.WriteString("---\n\n")
}

// renderDecisions renders the key decisions section
func renderDecisions(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("## Key Decisions\n\n")
	if len(c.Decisions) > 0 {
		for i, d := range c.Decisions {
			fmt.Fprintf(sb, "%d. **%s**\n", i+1, d.Title)
			if d.Rationale != "" {
				fmt.Fprintf(sb, "   - Rationale: %s\n", d.Rationale)
			}
			if d.Impact != "" {
				fmt.Fprintf(sb, "   - Impact: %s\n", d.Impact)
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No key decisions recorded.\n\n")
	}
	sb.WriteString("---\n\n")
}

// renderOutcomes renders the outcomes achieved section
func renderOutcomes(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("## Outcomes Achieved\n\n")
	if len(c.Outcomes) > 0 {
		for _, o := range c.Outcomes {
			fmt.Fprintf(sb, "- **%s**\n", o.Description)
			if o.Evidence != "" {
				fmt.Fprintf(sb, "  - Evidence: %s\n", o.Evidence)
			}
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("No outcomes recorded.\n\n")
	}
	sb.WriteString("---\n\n")
}

// renderLearnings renders the learnings discovered section
func renderLearnings(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("## Learnings Discovered\n\n")
	renderTechnicalLearnings(sb, c)
	renderProcessLearnings(sb, c)
	sb.WriteString("---\n\n")
}

// renderTechnicalLearnings renders the technical learnings subsection
func renderTechnicalLearnings(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("### Technical Learnings\n\n")
	if len(c.TechnicalLearnings) > 0 {
		for i, l := range c.TechnicalLearnings {
			fmt.Fprintf(sb, "%d. %s\n", i+1, l.Learning)
			if l.Context != "" {
				fmt.Fprintf(sb, "   - Context: %s\n", l.Context)
			}
			if l.Application != "" {
				fmt.Fprintf(sb, "   - Application: %s\n", l.Application)
			}
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("No technical learnings recorded.\n\n")
	}
}

// renderProcessLearnings renders the process learnings subsection
func renderProcessLearnings(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("### Process Learnings\n\n")
	if len(c.ProcessLearnings) > 0 {
		for i, l := range c.ProcessLearnings {
			fmt.Fprintf(sb, "%d. %s\n", i+1, l.Learning)
			if l.Context != "" {
				fmt.Fprintf(sb, "   - Context: %s\n", l.Context)
			}
			if l.Application != "" {
				fmt.Fprintf(sb, "   - Application: %s\n", l.Application)
			}
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("No process learnings recorded.\n\n")
	}
}

// renderActivePlan renders the active plan section
func renderActivePlan(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("## Active Plan\n\n")
	if c.ActivePlan != nil {
		fmt.Fprintf(sb, "**Status**: %s\n", c.ActivePlan.Status)
		fmt.Fprintf(sb, "**Current Phase**: %s\n\n", c.ActivePlan.CurrentPhase)
		if len(c.ActivePlan.NextSteps) > 0 {
			sb.WriteString("**Next Steps**:\n")
			for i, step := range c.ActivePlan.NextSteps {
				fmt.Fprintf(sb, "%d. %s\n", i+1, step)
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No active Wayfinder Plan.\n\n")
	}
	sb.WriteString("---\n\n")
}

// renderEngrams renders the relevant engrams section
func renderEngrams(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("## Relevant Engrams\n\n")
	if len(c.Engrams) > 0 {
		for _, e := range c.Engrams {
			fmt.Fprintf(sb, "- %s\n", e)
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("No engrams recorded.\n\n")
	}
	sb.WriteString("---\n\n")
}

// renderFooter renders the consolidation footer
func renderFooter(sb *strings.Builder, c *Consolidation) {
	sb.WriteString("## Archived Context\n\n")
	fmt.Fprintf(sb, "**Full session history archived to**: %s\n\n", c.ArchivePath)
	sb.WriteString("**Consolidation summary**: This consolidation captures essential context.\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("**Generated by**: Engram Hippocampus (Sleep Cycle consolidation)\n")
	sb.WriteString("**Protocol**: OSS-EBR-28 Sleep Cycle Protocol\n")
}
