package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/internal/corpus"
	"github.com/vbonnet/dear-agent/wayfinder/internal/project"
	"github.com/vbonnet/dear-agent/wayfinder/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/internal/tracker"
)

var (
	depthFlag        string
	autoClassifyFlag bool
	projectDirFlag   string
)

var startCmd = &cobra.Command{
	Use:   "start <what you want to build>",
	Short: "Create new Wayfinder project",
	Long: `Create a new Wayfinder project from a natural language prompt.

Workspace is determined by:
  1. WAYFINDER_WORKSPACE env var (oss or acme)
  2. Current directory (auto-detected)
  3. Default to 'oss' workspace

Examples:
  wayfinder start "Implement OAuth authentication with Google"
  wayfinder start "Fix bug in user authentication flow"
  WAYFINDER_WORKSPACE=acme wayfinder start "Build feature"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVar(&depthFlag, "depth", "", "Depth tier (XS, S, M, L, XL)")
	startCmd.Flags().BoolVar(&autoClassifyFlag, "auto-classify", false, "Auto-classify depth from project description")
	startCmd.Flags().StringVar(&projectDirFlag, "project-dir", "", "Override project root directory (default: ~/src/ws/{workspace}/wf/)")
}

func runStart(cmd *cobra.Command, args []string) error {
	// Join all args as the prompt
	prompt := strings.Join(args, " ")

	// Determine workspace
	workspace, err := project.DetermineWorkspace()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Creating project in workspace: %s\n", workspace)

	// Generate project ID from prompt
	projectID := project.GenerateID(prompt)

	// Determine project root
	var projectRoot string
	if projectDirFlag != "" {
		projectRoot = projectDirFlag
	} else {
		// Try to detect from git repo context
		gitRoot, err := detectGitRepoRoot()
		if err == nil && gitRoot != "" {
			projectRoot = filepath.Join(gitRoot, "wf")
		} else {
			// Fall back to workspace convention
			projectRoot = filepath.Join(os.Getenv("HOME"), "src", "ws", workspace, "wf")
		}
	}

	// Validate root exists or create it
	if _, statErr := os.Stat(projectRoot); os.IsNotExist(statErr) {
		if err := os.MkdirAll(projectRoot, 0o700); err != nil {
			return fmt.Errorf("failed to create project root %s: %w", projectRoot, err)
		}
	}

	projectDir := filepath.Join(projectRoot, projectID)

	// Check if directory already exists
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("project directory already exists: %s\n\nPlease choose a different description or remove the existing directory", projectDir)
	}

	// Create project directory
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create session directly (instead of calling wayfinder-session via exec.Command)
	st := status.New(projectDir)

	// Handle depth and auto-classification
	switch {
	case autoClassifyFlag:
		classification, estimatedTime, err := runClassification(prompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: auto-classification failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Defaulting to depth: %s\n", status.DefaultDepth)
			st.Depth = status.DefaultDepth
			st.DepthSource = status.DepthSourceUserOverride
		} else {
			st.Depth = classification.PredictedDepth
			st.DepthSource = status.DepthSourceAutoClassified
			st.Classification = classification
			st.EstimatedTime = estimatedTime

			// Display classification results
			fmt.Fprintf(os.Stderr, "\n✨ Auto-classification results:\n")
			fmt.Fprintf(os.Stderr, "  Predicted depth: %s\n", classification.PredictedDepth)
			fmt.Fprintf(os.Stderr, "  Confidence: %s\n", classification.Confidence)
			fmt.Fprintf(os.Stderr, "  Estimated time: %s\n", estimatedTime)
			fmt.Fprintf(os.Stderr, "  Rationale: %s\n\n", classification.Rationale)
		}
	case depthFlag != "":
		// Validate manual depth
		validDepths := []string{status.DepthXS, status.DepthS, status.DepthM, status.DepthL, status.DepthXL}
		isValid := false
		for _, d := range validDepths {
			if depthFlag == d {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid depth: %s (must be one of: XS, S, M, L, XL)", depthFlag)
		}
		st.Depth = depthFlag
		st.DepthSource = status.DepthSourceUserOverride
	default:
		// Default depth
		st.Depth = status.DefaultDepth
		st.DepthSource = status.DepthSourceUserOverride
	}

	// Initialize tracker
	tr, err := tracker.New(st.SessionID)
	if err != nil {
		// Clean up directory on failure
		os.RemoveAll(projectDir)
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer tr.Close(context.Background())

	// Publish session.started event
	if err := tr.StartSession(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish session.started event: %v\n", err)
	}

	// Write STATUS file
	if err := st.WriteTo(projectDir); err != nil {
		os.RemoveAll(projectDir)
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	// Create S11-retrospective.md
	if err := createRetrospective(projectDir, projectID, prompt, st.SessionID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create S11-retrospective.md: %v\n", err)
	}

	// Register Wayfinder schemas with corpus callosum (if available)
	if err := corpus.RegisterWayfinderSchemas(workspace); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to register Wayfinder schemas: %v\n", err)
	}

	// Publish project to corpus callosum
	projectData := map[string]interface{}{
		"session_id":    st.SessionID,
		"project_path":  projectDir,
		"project_id":    projectID,
		"status":        st.Status,
		"current_phase": st.CurrentPhase,
		"depth":         st.Depth,
		"started_at":    st.StartedAt.Format(time.RFC3339),
		"updated_at":    st.StartedAt.Format(time.RFC3339),
	}
	if err := corpus.PublishProject(workspace, projectData); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish project to corpus callosum: %v\n", err)
	}

	// Output response
	fmt.Printf("I understand you want to: %s\n\n", prompt)
	fmt.Printf("Created Wayfinder project: %s\n", projectID)
	fmt.Printf("Location: %s\n\n", projectDir)
	fmt.Printf("✅ Wayfinder session started\n")
	fmt.Printf("Session ID: %s\n", st.SessionID)
	fmt.Printf("Project: %s\n", projectID)
	fmt.Printf("Created: %s\n\n", status.StatusFilename)
	fmt.Printf("Next steps:\n\n")
	fmt.Printf("Run phases manually:\n")
	fmt.Printf("  /wayfinder-next-phase\n\n")
	fmt.Printf("Or use autopilot (runs all phases):\n")
	fmt.Printf("  /wayfinder-run-all-phases\n\n")
	fmt.Printf("End session:\n")
	fmt.Printf("  wayfinder-session end\n")

	return nil
}

// createRetrospective creates S11-retrospective.md in the project directory
func createRetrospective(projectDir, projectID, prompt, sessionID string) error {
	date := time.Now().Format("2006-01-02")

	content := livingRetrospectiveTemplate
	content = strings.ReplaceAll(content, "{PROJECT_NAME}", prompt)
	content = strings.ReplaceAll(content, "{PROJECT_ID}", projectID)
	content = strings.ReplaceAll(content, "{SESSION_ID}", sessionID)
	content = strings.ReplaceAll(content, "{DATE}", date)

	retrospectivePath := filepath.Join(projectDir, "S11-retrospective.md")
	return os.WriteFile(retrospectivePath, []byte(content), 0o600)
}

// livingRetrospectiveTemplate is the template for S11-retrospective.md
const livingRetrospectiveTemplate = `---
phase: S11
title: Living Retrospective - {PROJECT_NAME}
date: {DATE}
status: in_progress
project: {PROJECT_ID}
session_id: {SESSION_ID}
tags: [retrospective, living-document]
schema_version: "2.0"
previousPhase: S10
living_retrospective: true
living_updates:
  discovery_complete: false
  design_complete: false
  implementation_complete: false
  deployment_complete: false
---

# S11: Living Retrospective - {PROJECT_NAME}

**Session ID:** {SESSION_ID}
**Created:** {DATE}
**Status:** Living document (updated throughout workflow)

> **Living Retrospective:** This document is created at project start and updated incrementally as you progress through Wayfinder phases. Add brief entries at phase transitions to capture learnings while they're fresh.

---

## Living Updates (Incremental)

### Phase D1-D4 (Discovery)

**Last Updated:** _[Not yet updated]_

**Problem Validation (D1):**
- What did we learn about the problem?
- Was the problem well-defined or did it evolve?
- [Add your observations here]

**Solutions Search (D2):**
- Which solution options were most valuable to explore?
- Any surprises in evaluation?
- [Add your observations here]

**Approach Decision (D3):**
- Was the chosen approach the right one?
- What factors drove the decision?
- [Add your observations here]

**Requirements (D4):**
- Were requirements clear and complete?
- What gaps emerged during multi-persona review?
- [Add your observations here]

**Overall Discovery Learnings:**
- [What worked well in discovery phases?]
- [What would you do differently?]
- [Time spent: estimated vs actual]

---

### Phase S4-S6 (Requirements & Design)

**Last Updated:** _[Not yet updated]_

**Stakeholder Alignment (S4):**
- Were stakeholders aligned on requirements?
- Any conflicts or clarifications needed?
- [Add your observations here]

**Research (S5):**
- Did research answer our questions?
- Need to return from S6 for more research?
- [Add your observations here]

**Design (S6):**
- Was the design comprehensive?
- Did multi-persona review catch issues?
- What would you design differently?
- [Add your observations here]

**Overall Design Learnings:**
- [What worked well in design phases?]
- [What would you do differently?]
- [Time spent: estimated vs actual]

---

### Phase S7-S9 (Planning & Implementation)

**Last Updated:** _[Not yet updated]_

**Plan (S7):**
- Were estimates accurate?
- Did exit criteria help catch gaps?
- [Add your observations here]

**Implementation (S8):**
- Did implementation go according to plan?
- What blockers emerged?
- [Add your observations here]

**Validation (S9):**
- Did validation catch issues?
- What tier was appropriate?
- [Add your observations here]

**Overall Implementation Learnings:**
- [What worked well in implementation phases?]
- [What would you do differently?]
- [Time spent: estimated vs actual]

---

### Phase S10 (Deployment)

**Last Updated:** _[Not yet updated]_

**Deploy (S10):**
- Did deployment go smoothly?
- Any production issues?
- [Add your observations here]

**Overall Deployment Learnings:**
- [What worked well in deployment?]
- [What would you do differently?]
- [Time spent: estimated vs actual]

---

## Compaction Reflections

> **Before compacting phase artifacts** (summarizing detailed documents), capture key learnings here.

### Before D4→S4 Transition

**Date:** _[Not yet updated]_
**What we're compacting:** D1-D4 discovery artifacts

**Key decisions to preserve:**
- [What problem did we validate?]
- [Which solution did we choose and why?]
- [What requirements are critical?]

**Learnings before moving to implementation:**
- [What worked well in discovery?]
- [What would we do differently next time?]

---

### Before S6→S7 Transition

**Date:** _[Not yet updated]_
**What we're compacting:** S6 design artifact

**Key decisions to preserve:**
- [What architecture did we choose?]
- [What trade-offs did we make?]
- [What did multi-persona review catch?]

**Learnings before moving to planning:**
- [Was design thorough enough?]
- [Did we need to return to S5 research?]

---

### Before S9→S10 Transition

**Date:** _[Not yet updated]_
**What we're compacting:** S7-S9 implementation artifacts

**Key decisions to preserve:**
- [What changed from original plan?]
- [What blockers did we hit?]
- [What did validation reveal?]

**Learnings before deployment:**
- [Were estimates accurate?]
- [What would we do differently?]

---

## Final Retrospective (S11)

> **Complete this section at S11 phase** for comprehensive project review.

### What Went Well

**Team Collaboration:**
- [What collaboration practices were effective?]

**Technical Decisions:**
- [Which technical choices paid off?]

**Process Adherence:**
- [Did Wayfinder D1-D4, S4-S11 process work well?]
- [Which phases were most valuable?]

---

### What Could Improve

**Technical Challenges:**
- [What technical problems emerged?]
- [How could we have anticipated them?]

**Process Breakdowns:**
- [Where did the process fail us?]
- [Which phases need improvement?]

**Communication Gaps:**
- [Where did communication break down?]

---

### Lessons Learned

**About the Problem Domain:**
- [What did we learn about this problem space?]

**About Our Tools and Process:**
- [What did we learn about Wayfinder workflow?]
- [What did we learn about our development process?]

**About Estimation:**
- [How accurate were our estimates?]
- [What factors did we underestimate/overestimate?]

---

### Action Items

**Immediate Actions (This Week):**
- [ ] [Action with owner]

**Short-Term Actions (This Month):**
- [ ] [Process improvement]

**Long-Term Actions (This Quarter):**
- [ ] [Strategic change]

---

### Success Criteria Review

**From D1 (Problem Validation):**

**Must have:**
1. ✅/❌ [Criterion 1] - [Actual result]
2. ✅/❌ [Criterion 2] - [Actual result]

**Should have:**
3. ✅/❌ [Criterion 3] - [Actual result]

**Nice to have:**
4. ✅/❌ [Criterion 4] - [Actual result]

**Success Rate:** [X]% of criteria met

---

### Metrics

**Phase Duration:**
- D1-D4 (Discovery): [X] hours (estimated: [Y] hours)
- S4-S6 (Design): [X] hours (estimated: [Y] hours)
- S7-S9 (Implementation): [X] hours (estimated: [Y] hours)
- S10 (Deployment): [X] hours (estimated: [Y] hours)
- **Total:** [X] hours (estimated: [Y] hours)
- **Variance:** [+/-X]%

**Phases Completed:** [List: D1, D2, D3, D4, S4, S5, S6, S7, S8, S9, S10, S11]
**Phases Skipped:** [None - all 12 phases required]

**Multi-Persona Review:**
- Total personas invoked: [X]
- Blocking issues found: [X]
- Issues prevented in production: [X]
- Rework avoided: ~[X] hours (estimated based on 5:1 ROI)

---

### Knowledge Sharing

**Patterns Discovered:**
- [Pattern name]: [When to use, why it works]

**Anti-Patterns:**
- [Anti-pattern name]: [Why it failed, what to do instead]

**Recommended Tools:**
- [Tool name]: [Why it worked, when to use]

**Tools to Avoid:**
- [Tool name]: [Why it failed, alternative]

---

## Retrospective Instructions

### When to Update This Document

**Required Updates:**
1. **After D4 completion** - Capture discovery phase learnings before moving to S4
2. **After S6 completion** - Capture design decisions and multi-persona review findings
3. **After S9 completion** - Capture implementation and validation learnings
4. **At S11 phase** - Complete final comprehensive retrospective

**Optional Updates (Recommended):**
- After each phase completion - Brief 1-2 sentence learnings
- When blockers encountered - Document issue and resolution
- When estimates were wrong - Note variance and reason

### How to Update

**For Living Updates (during workflow):**
1. Find the relevant phase section above
2. Add dated entry with brief observations (1-3 sentences)
3. Focus on "what changed from expectations" not comprehensive docs
4. Update "Last Updated" timestamp

**For Compaction Reflections:**
1. Before compacting artifacts (D4→S4, S6→S7, S9→S10)
2. Capture key decisions that will be lost in summary
3. Note learnings before moving to next phase group

**For Final Retrospective:**
1. At S11 phase, complete all sections comprehensively
2. Review living updates for patterns and themes
3. Extract actionable improvements
4. Update metrics and success criteria review

---

**Project Status:** In Progress
**Next Update:** [After current phase completes]
`

// detectGitRepoRoot finds the git repository root from the current working directory.
// Returns empty string if not in a git repository.
func detectGitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// runClassification calls wayfinder-classify and returns classification results
func runClassification(prompt string) (*status.Classification, string, error) {
	// Find wayfinder-classify command
	classifyPath := filepath.Join(os.Getenv("HOME"), "src/engram/core/cortex/lib/wayfinder-classify")
	if _, err := os.Stat(classifyPath); os.IsNotExist(err) {
		classifyPath = "wayfinder-classify" // Try PATH
	}

	// Execute classification
	cmd := exec.Command(classifyPath, "--charter-text", prompt, "--format", "json") //nolint:gosec // G702: classifyPath is internally constructed; prompt passed as separate arg, not shell-composed
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to run wayfinder-classify: %w", err)
	}

	// Parse JSON result
	var result struct {
		Depth              string                 `json:"depth"`
		Confidence         string                 `json:"confidence"`
		Rationale          string                 `json:"rationale"`
		Signals            map[string]interface{} `json:"signals"`
		EstimatedTime      string                 `json:"estimated_time"`
		EscalationTriggers []string               `json:"escalation_triggers"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse classification result: %w", err)
	}

	// Build Classification struct
	classification := &status.Classification{
		PredictedDepth:     result.Depth,
		Confidence:         result.Confidence,
		Rationale:          result.Rationale,
		Signals:            result.Signals,
		EscalationTriggers: result.EscalationTriggers,
	}

	return classification, result.EstimatedTime, nil
}
