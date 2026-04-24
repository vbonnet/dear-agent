// Package a2a provides Agent Card generation from AGM session manifests.
// It bridges AGM's session metadata to the A2A protocol's Agent Card format,
// enabling external tool discovery of AGM-managed agents.
package a2a

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// GenerateCard creates an A2A AgentCard from an AGM session manifest.
// The card exposes session metadata in the standard A2A discovery format.
func GenerateCard(m *manifest.Manifest) a2a.AgentCard {
	description := cardDescription(m)
	skills := inferSkills(m)

	return a2a.AgentCard{
		Name:               m.Name,
		Description:        description,
		ProtocolVersion:    string(a2a.Version),
		Skills:             skills,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}
}

// CardJSON serializes an AgentCard to indented JSON bytes.
func CardJSON(card a2a.AgentCard) ([]byte, error) {
	data, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("a2a: marshal agent card: %w", err)
	}
	return data, nil
}

// cardDescription generates a human-readable description for the card.
// It prefers the manifest's Purpose field; falls back to a harness-based default.
func cardDescription(m *manifest.Manifest) string {
	if m.Context.Purpose != "" {
		return m.Context.Purpose
	}
	harness := m.Harness
	if harness == "" {
		harness = "AI"
	}
	return fmt.Sprintf("AGM-managed %s session", harness)
}

// inferSkills generates A2A skills from session metadata.
// Skills are derived from context tags, harness type, and session name patterns.
func inferSkills(m *manifest.Manifest) []a2a.AgentSkill {
	var skills []a2a.AgentSkill

	// Add skill from harness type
	if m.Harness != "" {
		skills = append(skills, a2a.AgentSkill{
			ID:          "harness-" + m.Harness,
			Name:        m.Harness,
			Description: fmt.Sprintf("Runs on %s harness", m.Harness),
			Tags:        []string{"harness", m.Harness},
		})
	}

	// Add skills from context tags
	for _, tag := range m.Context.Tags {
		if role, ok := strings.CutPrefix(tag, "role:"); ok {
			// Role tags get a dedicated skill with role-specific metadata
			skills = append(skills, a2a.AgentSkill{
				ID:          "role-" + sanitizeID(role),
				Name:        role,
				Description: roleDescription(role),
				Tags:        []string{"role", role},
			})
		} else {
			skills = append(skills, a2a.AgentSkill{
				ID:          "tag-" + sanitizeID(tag),
				Name:        tag,
				Description: fmt.Sprintf("Tagged with %s", tag),
				Tags:        []string{"context", tag},
			})
		}
	}

	// Infer skills from session name patterns
	nameSkills := inferNameSkills(m.Name)
	skills = append(skills, nameSkills...)

	// If no skills were inferred, add a generic one
	if len(skills) == 0 {
		skills = append(skills, a2a.AgentSkill{
			ID:          "general",
			Name:        "general",
			Description: "General-purpose agent session",
			Tags:        []string{"general"},
		})
	}

	return skills
}

// inferNameSkills extracts skills from session name patterns.
// Common patterns: "review-*", "fix-*", "test-*", "research-*"
func inferNameSkills(name string) []a2a.AgentSkill {
	var skills []a2a.AgentSkill

	patterns := map[string]string{
		"review":   "Code review and analysis",
		"fix":      "Bug fixing and repairs",
		"test":     "Testing and validation",
		"research": "Research and investigation",
		"refactor": "Code refactoring",
		"docs":     "Documentation writing",
		"debug":    "Debugging and diagnostics",
		"build":    "Build and compilation",
		"deploy":   "Deployment and release",
		"migrate":  "Migration and porting",
	}

	lower := strings.ToLower(name)
	for pattern, desc := range patterns {
		if strings.Contains(lower, pattern) {
			skills = append(skills, a2a.AgentSkill{
				ID:          "inferred-" + pattern,
				Name:        pattern,
				Description: desc,
				Tags:        []string{"inferred", pattern},
			})
		}
	}

	return skills
}

// roleDescription returns a human-readable description for known role tags.
func roleDescription(role string) string {
	switch role {
	case "orchestrator":
		return "Coordinates and delegates work to other agents"
	case "meta-orchestrator":
		return "Manages orchestrator agents and high-level coordination"
	case "researcher":
		return "Performs research and investigation tasks"
	case "worker":
		return "Executes implementation tasks"
	case "reviewer":
		return "Reviews code, plans, and deliverables"
	default:
		return fmt.Sprintf("Agent with %s role", role)
	}
}

// sanitizeID converts a string to a safe identifier (lowercase, no spaces).
func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	return s
}
