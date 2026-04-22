package ecphory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/reflection"
)

// TestApplyFailureBoosting_NormalQuery verifies no boosting for normal queries
func TestApplyFailureBoosting_NormalQuery(t *testing.T) {
	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	ranked := []RankingResult{
		{Path: "test1.ai.md", Relevance: 50.0},
		{Path: "test2.ai.md", Relevance: 40.0},
	}

	ecphory.applyFailureBoosting("how to write a for loop", ranked)

	// No boosting for normal query
	if ranked[0].Relevance != 50.0 {
		t.Errorf("ranked[0].Relevance = %f, want 50.0 (no boosting)", ranked[0].Relevance)
	}
	if ranked[1].Relevance != 40.0 {
		t.Errorf("ranked[1].Relevance = %f, want 40.0 (no boosting)", ranked[1].Relevance)
	}
}

// TestApplyFailureBoosting_DebuggingQuery verifies boosting for debugging queries
func TestApplyFailureBoosting_DebuggingQuery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "boosting-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create reflection with syntax_error category
	syntaxReflection := `---
type: strategy
title: 'Reflection: Syntax Error'
error_category: syntax_error
outcome: failure
---

# Syntax Error

Details about syntax error...
`
	syntaxPath := filepath.Join(tmpDir, "syntax-reflection.ai.md")
	err = os.WriteFile(syntaxPath, []byte(syntaxReflection), 0644)
	if err != nil {
		t.Fatalf("failed to write syntax reflection: %v", err)
	}

	// Create reflection with permission_denied category
	permissionReflection := `---
type: strategy
title: 'Reflection: Permission Denied'
error_category: permission_denied
outcome: failure
---

# Permission Denied

Details about permission error...
`
	permissionPath := filepath.Join(tmpDir, "permission-reflection.ai.md")
	err = os.WriteFile(permissionPath, []byte(permissionReflection), 0644)
	if err != nil {
		t.Fatalf("failed to write permission reflection: %v", err)
	}

	// Create non-reflection engram (no error_category)
	normalEngram := `---
type: pattern
title: 'Go Error Handling'
tags: [go, errors]
---

# Go Error Handling

Always check error returns...
`
	normalPath := filepath.Join(tmpDir, "normal-engram.ai.md")
	err = os.WriteFile(normalPath, []byte(normalEngram), 0644)
	if err != nil {
		t.Fatalf("failed to write normal engram: %v", err)
	}

	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	ranked := []RankingResult{
		{Path: syntaxPath, Relevance: 50.0},
		{Path: permissionPath, Relevance: 40.0},
		{Path: normalPath, Relevance: 30.0},
	}

	// Query with syntax error context
	ecphory.applyFailureBoosting("syntax error in my code", ranked)

	// Syntax reflection should be boosted (+25.0)
	if ranked[0].Relevance != 75.0 {
		t.Errorf("ranked[0].Relevance = %f, want 75.0 (50.0 + 25.0 boost)", ranked[0].Relevance)
	}

	// Permission reflection should NOT be boosted (different category)
	if ranked[1].Relevance != 40.0 {
		t.Errorf("ranked[1].Relevance = %f, want 40.0 (no boost for different category)", ranked[1].Relevance)
	}

	// Normal engram should NOT be boosted (no error_category)
	if ranked[2].Relevance != 30.0 {
		t.Errorf("ranked[2].Relevance = %f, want 30.0 (no boost for non-reflection)", ranked[2].Relevance)
	}
}

// TestApplyFailureBoosting_ScoreCapping verifies relevance score is capped at 100.0
func TestApplyFailureBoosting_ScoreCapping(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "boosting-cap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create reflection with high initial score
	highScoreReflection := `---
type: strategy
title: 'Reflection: Timeout Error'
error_category: timeout
outcome: failure
---

# Timeout Error

Details about timeout...
`
	reflectionPath := filepath.Join(tmpDir, "timeout-reflection.ai.md")
	err = os.WriteFile(reflectionPath, []byte(highScoreReflection), 0644)
	if err != nil {
		t.Fatalf("failed to write reflection: %v", err)
	}

	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	ranked := []RankingResult{
		{Path: reflectionPath, Relevance: 85.0},
	}

	// Query with timeout context
	ecphory.applyFailureBoosting("request timeout error", ranked)

	// Score should be capped at 100.0 (85.0 + 25.0 = 110.0 -> 100.0)
	if ranked[0].Relevance != 100.0 {
		t.Errorf("ranked[0].Relevance = %f, want 100.0 (capped)", ranked[0].Relevance)
	}
}

// TestApplyFailureBoosting_MultipleMatchingReflections verifies all matching reflections are boosted
func TestApplyFailureBoosting_MultipleMatchingReflections(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "boosting-multi-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple reflections with same category
	reflection1 := `---
type: strategy
title: 'Reflection: Tool Misuse 1'
error_category: tool_misuse
outcome: failure
---

# Tool Misuse 1

First tool misuse reflection...
`
	path1 := filepath.Join(tmpDir, "tool-misuse-1.ai.md")
	err = os.WriteFile(path1, []byte(reflection1), 0644)
	if err != nil {
		t.Fatalf("failed to write reflection 1: %v", err)
	}

	reflection2 := `---
type: strategy
title: 'Reflection: Tool Misuse 2'
error_category: tool_misuse
outcome: failure
---

# Tool Misuse 2

Second tool misuse reflection...
`
	path2 := filepath.Join(tmpDir, "tool-misuse-2.ai.md")
	err = os.WriteFile(path2, []byte(reflection2), 0644)
	if err != nil {
		t.Fatalf("failed to write reflection 2: %v", err)
	}

	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	ranked := []RankingResult{
		{Path: path1, Relevance: 60.0},
		{Path: path2, Relevance: 45.0},
	}

	// Query with tool_misuse context
	ecphory.applyFailureBoosting("incorrect usage of API", ranked)

	// Both reflections should be boosted
	if ranked[0].Relevance != 85.0 {
		t.Errorf("ranked[0].Relevance = %f, want 85.0 (60.0 + 25.0)", ranked[0].Relevance)
	}
	if ranked[1].Relevance != 70.0 {
		t.Errorf("ranked[1].Relevance = %f, want 70.0 (45.0 + 25.0)", ranked[1].Relevance)
	}
}

// TestExtractErrorCategory verifies error_category extraction from frontmatter
func TestExtractErrorCategory(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "syntax_error",
			content: `---
type: strategy
title: 'Reflection: Syntax Error'
error_category: syntax_error
outcome: failure
---

# Content
`,
			expected: "syntax_error",
		},
		{
			name: "permission_denied",
			content: `---
type: strategy
error_category: permission_denied
---

# Content
`,
			expected: "permission_denied",
		},
		{
			name: "no_error_category",
			content: `---
type: pattern
title: 'Go Errors'
---

# Content
`,
			expected: "",
		},
		{
			name: "invalid_frontmatter",
			content: `---
invalid: [yaml: {
---

# Content
`,
			expected: "",
		},
		{
			name:     "no_frontmatter",
			content:  `# Just content without frontmatter`,
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractErrorCategory([]byte(tc.content))
			if result != tc.expected {
				t.Errorf("extractErrorCategory() = %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestApplyFailureBoosting_AllErrorCategories verifies boosting works for all 5 error categories
func TestApplyFailureBoosting_AllErrorCategories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "boosting-categories-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create reflections for each error category
	categories := []struct {
		category reflection.ErrorCategory
		query    string
	}{
		{reflection.ErrorCategorySyntax, "syntax error in code"},
		{reflection.ErrorCategoryPermissionDenied, "permission denied error"},
		{reflection.ErrorCategoryTimeout, "request timeout"},
		{reflection.ErrorCategoryToolMisuse, "wrong tool used"},
		{reflection.ErrorCategoryOther, "generic error occurred"},
	}

	for i, cat := range categories {
		// Create reflection with this category
		content := "---\ntype: strategy\nerror_category: " + string(cat.category) + "\n---\n\n# Test\n"
		path := filepath.Join(tmpDir, string(cat.category)+".ai.md")
		err = os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			t.Fatalf("failed to write reflection for %s: %v", cat.category, err)
		}

		detector := NewContextDetector()
		ecphory := &Ecphory{
			contextDetector: detector,
		}

		ranked := []RankingResult{
			{Path: path, Relevance: 50.0},
		}

		// Test boosting for this category
		ecphory.applyFailureBoosting(cat.query, ranked)

		if ranked[0].Relevance != 75.0 {
			t.Errorf("Category %s (query %q): Relevance = %f, want 75.0 (boosted)",
				cat.category, cat.query, ranked[0].Relevance)
		} else {
			t.Logf("✓ Category %s: correctly boosted from 50.0 to 75.0", cat.category)
		}

		// Clean up for next iteration
		os.Remove(path)

		// Avoid variable shadowing (technically unused but clear for next iteration)
		_ = i
	}
}
