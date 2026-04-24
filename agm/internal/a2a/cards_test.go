package a2a

import (
	"encoding/json"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestGenerateCard_BasicFields(t *testing.T) {
	m := &manifest.Manifest{
		Name:    "my-session",
		Harness: "claude-code",
		Context: manifest.Context{
			Purpose: "Refactoring the database layer",
		},
	}

	card := GenerateCard(m)

	assert.Equal(t, "my-session", card.Name)
	assert.Equal(t, "Refactoring the database layer", card.Description)
	assert.Equal(t, string(a2a.Version), card.ProtocolVersion)
	assert.Equal(t, []string{"text/plain"}, card.DefaultInputModes)
	assert.Equal(t, []string{"text/plain"}, card.DefaultOutputModes)
}

func TestGenerateCard_DescriptionFallback(t *testing.T) {
	tests := []struct {
		name     string
		manifest *manifest.Manifest
		want     string
	}{
		{
			name: "uses purpose when available",
			manifest: &manifest.Manifest{
				Name:    "s1",
				Harness: "claude-code",
				Context: manifest.Context{Purpose: "Custom purpose"},
			},
			want: "Custom purpose",
		},
		{
			name: "falls back to harness description",
			manifest: &manifest.Manifest{
				Name:    "s2",
				Harness: "gemini-cli",
			},
			want: "AGM-managed gemini-cli session",
		},
		{
			name: "falls back to AI when no harness",
			manifest: &manifest.Manifest{
				Name: "s3",
			},
			want: "AGM-managed AI session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := GenerateCard(tt.manifest)
			assert.Equal(t, tt.want, card.Description)
		})
	}
}

func TestGenerateCard_SkillsFromHarness(t *testing.T) {
	m := &manifest.Manifest{
		Name:    "test",
		Harness: "claude-code",
	}

	card := GenerateCard(m)

	require.NotEmpty(t, card.Skills)
	found := false
	for _, sk := range card.Skills {
		if sk.ID == "harness-claude-code" {
			found = true
			assert.Equal(t, "claude-code", sk.Name)
			assert.Contains(t, sk.Tags, "harness")
		}
	}
	assert.True(t, found, "expected harness skill")
}

func TestGenerateCard_SkillsFromTags(t *testing.T) {
	m := &manifest.Manifest{
		Name:    "test",
		Harness: "claude-code",
		Context: manifest.Context{
			Tags: []string{"backend", "golang"},
		},
	}

	card := GenerateCard(m)

	tagIDs := make(map[string]bool)
	for _, sk := range card.Skills {
		tagIDs[sk.ID] = true
	}
	assert.True(t, tagIDs["tag-backend"], "expected tag-backend skill")
	assert.True(t, tagIDs["tag-golang"], "expected tag-golang skill")
}

func TestGenerateCard_SkillsFromNamePattern(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		wantSkill   string
	}{
		{"review pattern", "review-pr-123", "inferred-review"},
		{"fix pattern", "fix-login-bug", "inferred-fix"},
		{"test pattern", "test-api-routes", "inferred-test"},
		{"research pattern", "research-a2a-protocol", "inferred-research"},
		{"debug pattern", "debug-memory-leak", "inferred-debug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifest.Manifest{Name: tt.sessionName}
			card := GenerateCard(m)

			found := false
			for _, sk := range card.Skills {
				if sk.ID == tt.wantSkill {
					found = true
					break
				}
			}
			assert.True(t, found, "expected skill %s for session %s", tt.wantSkill, tt.sessionName)
		})
	}
}

func TestGenerateCard_GenericSkillWhenNoOther(t *testing.T) {
	m := &manifest.Manifest{
		Name: "plain-session",
	}

	card := GenerateCard(m)

	require.Len(t, card.Skills, 1)
	assert.Equal(t, "general", card.Skills[0].ID)
}

func TestCardJSON_ValidJSON(t *testing.T) {
	m := &manifest.Manifest{
		Name:    "json-test",
		Harness: "claude-code",
		Context: manifest.Context{
			Purpose: "Testing JSON output",
			Tags:    []string{"test"},
		},
	}

	card := GenerateCard(m)
	data, err := CardJSON(card)
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "json-test", parsed["name"])
	assert.Equal(t, "Testing JSON output", parsed["description"])
}

func TestGenerateCard_RoleTagSkills(t *testing.T) {
	m := &manifest.Manifest{
		Name:    "worker-session",
		Harness: "claude-code",
		Context: manifest.Context{
			Tags: []string{"role:worker", "role:reviewer", "cap:web-search"},
		},
	}

	card := GenerateCard(m)

	skillIDs := make(map[string]bool)
	for _, sk := range card.Skills {
		skillIDs[sk.ID] = true
		// Role skills should have role-specific descriptions
		if sk.ID == "role-worker" {
			assert.Equal(t, "Executes implementation tasks", sk.Description)
			assert.Contains(t, sk.Tags, "role")
		}
		if sk.ID == "role-reviewer" {
			assert.Equal(t, "Reviews code, plans, and deliverables", sk.Description)
		}
	}
	assert.True(t, skillIDs["role-worker"], "expected role-worker skill")
	assert.True(t, skillIDs["role-reviewer"], "expected role-reviewer skill")
	assert.True(t, skillIDs["tag-cap:web-search"], "expected tag for cap:web-search")
	assert.False(t, skillIDs["tag-role:worker"], "role tags should not create tag- skills")
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"path/to/thing", "path-to-thing"},
		{"already-clean", "already-clean"},
		{"MixedCase", "mixedcase"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeID(tt.input))
		})
	}
}
