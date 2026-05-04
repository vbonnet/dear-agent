// Package deepresearch provides deepresearch functionality.
package deepresearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/workflow"
)

// ResearchApplicator analyzes research reports and generates actionable improvement proposals.
type ResearchApplicator struct {

	// projectID is the GCP project ID
	projectID string

	// repos are the target repositories for proposals
	repos []string
}

// NewResearchApplicator creates a new research applicator.
func NewResearchApplicator(repos []string) (*ResearchApplicator, error) {
	// Get API key from environment or default location
	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		projectID = "default-project"
	}

	// For now, we'll use a placeholder for API key
	// TODO: Integrate with actual Gemini API

	if len(repos) == 0 {
		repos = []string{"engram", "ai-tools"}
	}

	return &ResearchApplicator{
		projectID: projectID,
		repos:     repos,
	}, nil
}

// ApplicationResult contains the results of applying research to repositories.
type ApplicationResult struct {
	// Proposals contains improvement proposals categorized by repository
	Proposals map[string][]Proposal

	// CrossCuttingIdeas contains ideas that apply to multiple repositories
	CrossCuttingIdeas []string

	// Summary is a brief summary of the application results
	Summary string
}

// Proposal represents a specific improvement proposal for a repository.
type Proposal struct {
	// Title is a brief title for the proposal
	Title string

	// Description is a detailed description of the proposal
	Description string

	// Category is the proposal category (architecture, ux, performance, etc.)
	Category string

	// Priority is the proposal priority (high, medium, low)
	Priority string

	// TestableIdeas contains specific testable ideas
	TestableIdeas []string
}

// Apply analyzes research reports and generates improvement proposals for repositories.
func (a *ResearchApplicator) Apply(ctx context.Context, artifacts []workflow.Artifact) (ApplicationResult, error) {
	// Read all research reports
	var researchContent []string
	for _, artifact := range artifacts {
		if artifact.Type != "research-report" {
			continue
		}

		content, err := os.ReadFile(artifact.Path)
		if err != nil {
			return ApplicationResult{}, fmt.Errorf("read research report %s: %w", artifact.Path, err)
		}

		researchContent = append(researchContent, string(content))
	}

	if len(researchContent) == 0 {
		return ApplicationResult{}, fmt.Errorf("no research reports found")
	}

	// For MVP: Generate basic proposals by parsing research content
	// TODO: Replace with actual LLM-based analysis
	proposals := a.generateBasicProposals(researchContent)

	// Categorize by repository
	categorized := a.categorizeProposals(proposals)

	// Generate summary
	summary := a.generateSummary(categorized)

	return ApplicationResult{
		Proposals:         categorized,
		CrossCuttingIdeas: a.extractCrossCuttingIdeas(researchContent),
		Summary:           summary,
	}, nil
}

// generateBasicProposals creates basic proposals from research content.
// This is a placeholder implementation that will be replaced with LLM-based analysis.
func (a *ResearchApplicator) generateBasicProposals(researchContent []string) []Proposal {
	var proposals []Proposal

	// MVP: Simple keyword-based proposal generation
	// This will be replaced with actual LLM analysis in production

	keywords := map[string]string{
		"architecture":    "architecture",
		"design pattern":  "architecture",
		"ux":              "ux",
		"user experience": "ux",
		"performance":     "performance",
		"optimization":    "performance",
		"testing":         "testing",
		"automation":      "automation",
	}

	for i, content := range researchContent {
		// Extract potential improvement areas based on keywords
		lowerContent := strings.ToLower(content)

		for keyword, category := range keywords {
			if strings.Contains(lowerContent, keyword) {
				proposals = append(proposals, Proposal{
					Title:       fmt.Sprintf("Research %d: %s improvements", i+1, category),
					Description: fmt.Sprintf("Apply insights from research %d related to %s", i+1, category),
					Category:    category,
					Priority:    "medium",
					TestableIdeas: []string{
						fmt.Sprintf("Review research %d for %s patterns", i+1, category),
						fmt.Sprintf("Prototype %s improvements based on findings", category),
					},
				})
			}
		}
	}

	// If no proposals generated, create a generic one
	if len(proposals) == 0 {
		proposals = append(proposals, Proposal{
			Title:       "General improvements from research",
			Description: "Review all research reports for applicable insights",
			Category:    "general",
			Priority:    "medium",
			TestableIdeas: []string{
				"Analyze research findings for applicable patterns",
				"Identify cross-cutting themes",
			},
		})
	}

	return proposals
}

// categorizeProposals categorizes proposals by repository.
func (a *ResearchApplicator) categorizeProposals(proposals []Proposal) map[string][]Proposal {
	categorized := make(map[string][]Proposal)

	// Initialize with target repos
	for _, repo := range a.repos {
		categorized[repo] = []Proposal{}
	}

	// Simple categorization: assign all proposals to all repos
	// TODO: Use LLM to intelligently categorize based on content
	for _, repo := range a.repos {
		categorized[repo] = append(categorized[repo], proposals...)
	}

	return categorized
}

// generateSummary creates a summary of application results.
func (a *ResearchApplicator) generateSummary(categorized map[string][]Proposal) string {
	totalProposals := 0
	for _, proposals := range categorized {
		totalProposals += len(proposals)
	}

	return fmt.Sprintf("Generated %d improvement proposals across %d repositories",
		totalProposals, len(categorized))
}

// extractCrossCuttingIdeas extracts ideas that apply across multiple repositories.
func (a *ResearchApplicator) extractCrossCuttingIdeas(researchContent []string) []string {
	// MVP: Return placeholder cross-cutting ideas
	// TODO: Use LLM to extract actual cross-cutting themes

	return []string{
		"Review research findings for patterns applicable to multiple projects",
		"Identify common themes across research reports",
		"Consider architectural patterns that benefit both engram and ai-tools",
	}
}

// WriteProposalsToMarkdown writes proposals to a markdown file.
func WriteProposalsToMarkdown(result ApplicationResult, outputPath string) error {
	var content strings.Builder

	// Header
	content.WriteString("# Research-Based Improvement Proposals\n\n")
	fmt.Fprintf(&content, "**Summary**: %s\n\n", result.Summary)
	content.WriteString("---\n\n")

	// Proposals by repository
	for repo, proposals := range result.Proposals {
		fmt.Fprintf(&content, "## %s Proposals\n\n", repo)

		for i, proposal := range proposals {
			fmt.Fprintf(&content, "### %d. %s\n\n", i+1, proposal.Title)
			fmt.Fprintf(&content, "**Category**: %s  \n", proposal.Category)
			fmt.Fprintf(&content, "**Priority**: %s\n\n", proposal.Priority)
			fmt.Fprintf(&content, "%s\n\n", proposal.Description)

			if len(proposal.TestableIdeas) > 0 {
				content.WriteString("**Testable Ideas**:\n")
				for _, idea := range proposal.TestableIdeas {
					fmt.Fprintf(&content, "- %s\n", idea)
				}
				content.WriteString("\n")
			}
		}
	}

	// Cross-cutting ideas
	if len(result.CrossCuttingIdeas) > 0 {
		content.WriteString("## Cross-Cutting Ideas\n\n")
		for _, idea := range result.CrossCuttingIdeas {
			fmt.Fprintf(&content, "- %s\n", idea)
		}
		content.WriteString("\n")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, []byte(content.String()), 0o600); err != nil {
		return fmt.Errorf("write proposals to %s: %w", outputPath, err)
	}

	return nil
}
