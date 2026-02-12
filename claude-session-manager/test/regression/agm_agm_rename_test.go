package regression

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegressionDocumentation ensures AGM→AGM rename regressions are documented
func TestRegressionDocumentation(t *testing.T) {
	// Verify regression documentation exists
	docPath := filepath.Join("..", "..", "docs", "AGM-RENAME-REGRESSIONS.md")
	_, err := os.Stat(docPath)
	require.NoError(t, err, "Regression documentation should exist at docs/AGM-RENAME-REGRESSIONS.md")

	// Read documentation
	content, err := os.ReadFile(docPath)
	require.NoError(t, err, "Should be able to read regression documentation")

	contentStr := string(content)

	// Verify all 5 regressions are documented
	regressions := []struct {
		name  string
		title string
	}{
		{"Regression 1", "InitSequence Failure"},
		{"Regression 2", "Archive Command"},
		{"Regression 3", "Tab Completion"},
		{"Regression 4", "Documentation Inconsistency"},
		{"Regression 5", "Default Socket Fallback"},
	}

	for _, reg := range regressions {
		assert.Contains(t, contentStr, reg.name, "Documentation should mention %s", reg.name)
		assert.Contains(t, contentStr, reg.title, "Documentation should describe %s", reg.title)
	}

	// Verify key sections exist
	keySections := []string{
		"Executive Summary",
		"Root Cause",
		"Fix Applied",
		"Testing",
		"Pattern Analysis",
		"Common Failure Modes",
		"Architectural Lessons",
	}

	for _, section := range keySections {
		assert.Contains(t, contentStr, section, "Documentation should have '%s' section", section)
	}
}

// TestNoCSMReferencesInUserDocs ensures user-facing docs use AGM not AGM
func TestNoCSMReferencesInUserDocs(t *testing.T) {
	// User-facing documentation files
	userDocs := []string{
		"../../README.md",
		"../../PLUGIN-INSTALLATION.md",
		"../../docs/GETTING-STARTED.md",
		"../../docs/AGM-QUICK-REFERENCE.md",
		"../../docs/AGM-COMMAND-REFERENCE.md",
	}

	// Patterns that should NOT appear in user docs (case-insensitive)
	// Except in historical context or "renamed from AGM" mentions
	forbiddenPatterns := []string{
		"csm new",
		"csm list",
		"csm resume",
		"csm archive",
		"~/.config/csm/",
		"/tmp/csm.sock",
		"csm-assoc", // Should be agm:assoc
	}

	for _, docPath := range userDocs {
		t.Run(filepath.Base(docPath), func(t *testing.T) {
			content, err := os.ReadFile(docPath)
			if os.IsNotExist(err) {
				t.Skipf("Document %s does not exist (optional)", docPath)
				return
			}
			require.NoError(t, err, "Should be able to read %s", docPath)

			contentStr := strings.ToLower(string(content))

			for _, pattern := range forbiddenPatterns {
				// Allow mentions in "renamed from AGM" or historical notes
				if strings.Contains(contentStr, "renamed from csm") ||
					strings.Contains(contentStr, "historical") ||
					strings.Contains(contentStr, "backwards compatibility") {
					continue
				}

				// Check if forbidden pattern exists
				if strings.Contains(contentStr, strings.ToLower(pattern)) {
					t.Errorf("Found AGM reference '%s' in %s - should use AGM branding instead", pattern, docPath)
				}
			}
		})
	}
}

// TestConfigPathsUseAGM ensures config paths use ~/.config/agm not ~/.config/csm
func TestConfigPathsUseAGM(t *testing.T) {
	// Check key documentation files
	docs := []string{
		"../../README.md",
		"../../docs/GETTING-STARTED.md",
		"../../docs/AGM-QUICK-REFERENCE.md",
	}

	for _, docPath := range docs {
		t.Run(filepath.Base(docPath), func(t *testing.T) {
			content, err := os.ReadFile(docPath)
			if os.IsNotExist(err) {
				t.Skipf("Document %s does not exist (optional)", docPath)
				return
			}
			require.NoError(t, err, "Should be able to read %s", docPath)

			contentStr := string(content)

			// Should mention ~/.config/agm
			if strings.Contains(contentStr, "config") {
				assert.Contains(t, contentStr, "~/.config/agm",
					"%s should reference ~/.config/agm for config location", docPath)
			}

			// Should NOT mention ~/.config/csm (except in migration notes)
			if !strings.Contains(contentStr, "migration") &&
				!strings.Contains(contentStr, "renamed from") {
				assert.NotContains(t, contentStr, "~/.config/csm",
					"%s should not reference old AGM config path", docPath)
			}
		})
	}
}

// TestThemeNamesUseAGM ensures theme names use 'agm' not 'csm'
func TestThemeNamesUseAGM(t *testing.T) {
	docs := []string{
		"../../README.md",
		"../../docs/GETTING-STARTED.md",
		"../../docs/AGM-QUICK-REFERENCE.md",
	}

	for _, docPath := range docs {
		t.Run(filepath.Base(docPath), func(t *testing.T) {
			content, err := os.ReadFile(docPath)
			if os.IsNotExist(err) {
				t.Skipf("Document %s does not exist (optional)", docPath)
				return
			}
			require.NoError(t, err, "Should be able to read %s", docPath)

			contentStr := string(content)

			// If document mentions themes, check they use AGM names
			if strings.Contains(contentStr, "theme:") {
				// Should use 'agm' or 'agm-light'
				assert.True(t,
					strings.Contains(contentStr, `theme: "agm"`) ||
						strings.Contains(contentStr, `theme: 'agm'`) ||
						strings.Contains(contentStr, "agm-light"),
					"%s should use AGM theme names (agm, agm-light)", docPath)

				// Should NOT use old AGM theme names (except in migration notes)
				if !strings.Contains(contentStr, "migration") {
					assert.False(t,
						strings.Contains(contentStr, `theme: "csm"`) ||
							strings.Contains(contentStr, `theme: 'csm'`),
						"%s should not reference old AGM theme names", docPath)
				}
			}
		})
	}
}

// TestPluginNameIsAGM ensures plugin is named 'agm' not 'csm-tools'
func TestPluginNameIsAGM(t *testing.T) {
	pluginDoc := "../../PLUGIN-INSTALLATION.md"

	content, err := os.ReadFile(pluginDoc)
	if os.IsNotExist(err) {
		t.Skip("PLUGIN-INSTALLATION.md does not exist (optional)")
		return
	}
	require.NoError(t, err, "Should be able to read plugin documentation")

	contentStr := string(content)

	// Should reference 'agm@ai-tools'
	assert.Contains(t, contentStr, "agm@ai-tools",
		"Plugin installation should use 'agm@ai-tools'")

	// Should reference '/agm:assoc' command
	assert.Contains(t, contentStr, "/agm:assoc",
		"Plugin command should be '/agm:assoc'")

	// Should NOT reference old names (except in historical context)
	if !strings.Contains(contentStr, "renamed from") {
		assert.NotContains(t, contentStr, "csm-tools@ai-tools",
			"Should not use old plugin name 'csm-tools'")
		assert.NotContains(t, contentStr, "/csm-assoc",
			"Should not use old command '/csm-assoc'")
	}
}
