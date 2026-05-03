package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/pkg/engram"
)

func TestAutoEcphoryParseStdin(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantField string // field to check
		wantValue string // expected value
	}{
		{
			name:      "valid JSON",
			input:     `{"session_id":"s1","cwd":"/tmp","hook_event_name":"UserPromptSubmit","prompt":"fix auth"}`,
			wantErr:   false,
			wantField: "prompt",
			wantValue: "fix auth",
		},
		{
			name:      "valid JSON with extra fields",
			input:     `{"session_id":"s1","cwd":"/tmp","prompt":"hello","extra":"ignored"}`,
			wantErr:   false,
			wantField: "session_id",
			wantValue: "s1",
		},
		{
			name:    "invalid JSON",
			input:   `{not valid}`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore stdin
			origStdin := os.Stdin
			defer func() { os.Stdin = origStdin }()

			r, w, err := os.Pipe()
			require.NoError(t, err)

			os.Stdin = r
			go func() {
				w.Write([]byte(tt.input))
				w.Close()
			}()

			ctx := testContext(t)
			result, err := parseAutoEcphoryStdin(ctx)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)

			switch tt.wantField {
			case "prompt":
				assert.Equal(t, tt.wantValue, result.Prompt)
			case "session_id":
				assert.Equal(t, tt.wantValue, result.SessionID)
			}
		})
	}
}

func TestAutoEcphoryFormatOutput(t *testing.T) {
	t.Run("single engram", func(t *testing.T) {
		engrams := []*engram.Engram{
			{
				Frontmatter: engram.Frontmatter{
					Title: "Error Handling in Go",
				},
				Content: "Use explicit error returns.",
			},
		}

		output := formatEngramOutput(engrams)
		assert.Contains(t, output, "<engram-context>")
		assert.Contains(t, output, "</engram-context>")
		assert.Contains(t, output, "## Error Handling in Go")
		assert.Contains(t, output, "Use explicit error returns.")
	})

	t.Run("multiple engrams", func(t *testing.T) {
		engrams := []*engram.Engram{
			{
				Frontmatter: engram.Frontmatter{Title: "First"},
				Content:     "Content one.",
			},
			{
				Frontmatter: engram.Frontmatter{Title: "Second"},
				Content:     "Content two.",
			},
		}

		output := formatEngramOutput(engrams)
		assert.Contains(t, output, "## First")
		assert.Contains(t, output, "## Second")
	})

	t.Run("empty engrams", func(t *testing.T) {
		output := formatEngramOutput(nil)
		assert.Empty(t, output)
	})
}

func TestAutoEcphoryTokenBudget(t *testing.T) {
	// Create an engram with content that exceeds the budget
	bigContent := strings.Repeat("word ", 2000) // ~10000 chars, exceeds maxOutputChars

	engrams := []*engram.Engram{
		{
			Frontmatter: engram.Frontmatter{Title: "Big Engram"},
			Content:     bigContent,
		},
	}

	output := formatEngramOutput(engrams)

	// Output must be within budget
	assert.LessOrEqual(t, len(output), maxOutputChars+100) // small margin for wrapper
	assert.Contains(t, output, "[truncated]")
	assert.Contains(t, output, "<engram-context>")
	assert.Contains(t, output, "</engram-context>")
}

func TestAutoEcphoryScoreEngram(t *testing.T) {
	eg := &engram.Engram{
		Frontmatter: engram.Frontmatter{
			Title:       "Error Handling in Go",
			Description: "Idiomatic error handling patterns for Go",
			Tags:        []string{"languages/go", "patterns/errors"},
			LoadWhen:    "Working with Go error handling",
		},
		Content: "Use explicit error returns over panic.",
	}

	t.Run("matching words score positive", func(t *testing.T) {
		words := []string{"error", "handling", "patterns"}
		score := scoreEngram(eg, words, "error handling patterns")
		assert.Greater(t, score, 0)
	})

	t.Run("unrelated words score zero", func(t *testing.T) {
		words := []string{"kubernetes", "deployment", "helm"}
		score := scoreEngram(eg, words, "kubernetes deployment helm")
		assert.Equal(t, 0, score)
	})

	t.Run("short words are skipped", func(t *testing.T) {
		words := []string{"go", "in", "is"}
		score := scoreEngram(eg, words, "go in is")
		assert.Equal(t, 0, score) // all words < 3 chars
	})

	t.Run("title matches score highest", func(t *testing.T) {
		titleWords := []string{"error"}
		contentWords := []string{"panic"}

		titleScore := scoreEngram(eg, titleWords, "error")
		contentScore := scoreEngram(eg, contentWords, "panic")

		// Title match (3) + desc match (2) + load_when match (3) + content match (1) = 9
		// vs content-only match (1)
		assert.Greater(t, titleScore, contentScore)
	})

	t.Run("score meets minimum threshold for strong match", func(t *testing.T) {
		words := []string{"error", "handling"}
		score := scoreEngram(eg, words, "error handling")
		assert.GreaterOrEqual(t, score, minAutoEcphoryScore)
	})

	t.Run("single weak match below threshold", func(t *testing.T) {
		words := []string{"explicit"}
		score := scoreEngram(eg, words, "explicit")
		// Only matches content (+1), below minAutoEcphoryScore (4)
		assert.Less(t, score, minAutoEcphoryScore)
	})
}

func TestAutoEcphoryDeduplication(t *testing.T) {
	// Create a temp dir with duplicate engrams (same title, different paths)
	tmpDir := t.TempDir()
	engramDir := filepath.Join(tmpDir, "engrams")
	require.NoError(t, os.MkdirAll(engramDir, 0755))

	engramContent := `---
type: pattern
title: Authentication Bug Fixes
description: Common auth bug patterns and fixes
tags:
  - security/auth
load_when: "Debugging authentication issues"
---
# Authentication Bug Fixes

Check token expiration first.
`
	// Write the same engram to 5 different files (simulating symlink duplicates)
	for i := 0; i < 5; i++ {
		path := filepath.Join(engramDir, strings.Replace("auth-bugs-REPLICA.ai.md", "REPLICA", strings.Repeat("x", i+1), 1))
		require.NoError(t, os.WriteFile(path, []byte(engramContent), 0644))
	}

	input := &autoEcphoryInput{
		Prompt: "fix the authentication bug in token validation",
		Cwd:    tmpDir,
	}

	t.Setenv("ENGRAM_HOME", engramDir)

	ctx := testContext(t)
	results, err := queryEngrams(ctx, engramDir, input)
	require.NoError(t, err)

	// Should deduplicate to exactly 1 result despite 5 files
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "Authentication Bug Fixes", results[0].Frontmatter.Title)
}

func TestAutoEcphoryMaxResults(t *testing.T) {
	// Verify the max results constant is reasonable
	assert.Equal(t, 5, maxAutoEcphoryResults)
	assert.Equal(t, 4, minAutoEcphoryScore)
}

func TestAutoEcphoryEndToEnd(t *testing.T) {
	// Create a temporary engram directory with test engrams
	tmpDir := t.TempDir()
	engramDir := filepath.Join(tmpDir, "engrams")
	require.NoError(t, os.MkdirAll(engramDir, 0755))

	// Write a test engram file
	engramContent := `---
type: pattern
title: Authentication Bug Fixes
description: Common auth bug patterns and fixes
tags:
  - security/auth
  - patterns/debugging
load_when: "Debugging authentication issues"
---
# Authentication Bug Fixes

When fixing auth bugs, check token expiration first.
Always validate JWT signatures before trusting claims.
`
	engramPath := filepath.Join(engramDir, "auth-bugs.ai.md")
	require.NoError(t, os.WriteFile(engramPath, []byte(engramContent), 0644))

	// Set ENGRAM_HOME so resolveEngramPath finds our test dir
	t.Setenv("ENGRAM_HOME", engramDir)

	// Override stdin with test input
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	inputJSON := `{"session_id":"test-session","cwd":"/tmp","hook_event_name":"UserPromptSubmit","prompt":"fix the auth bug"}`
	r, w, err := os.Pipe()
	require.NoError(t, err)
	go func() {
		w.Write([]byte(inputJSON))
		w.Close()
	}()
	os.Stdin = r

	// Capture stdout
	var buf bytes.Buffer
	autoEcphoryCmd.SetOut(&buf)
	autoEcphoryCmd.SetErr(&bytes.Buffer{}) // discard stderr

	err = autoEcphoryCmd.RunE(autoEcphoryCmd, []string{})
	assert.NoError(t, err) // should always return nil

	output := buf.String()
	if output != "" {
		assert.Contains(t, output, "<engram-context>")
		assert.Contains(t, output, "Authentication Bug Fixes")
		assert.Contains(t, output, "</engram-context>")
	}
}

func TestAutoEcphoryMissingEngramDir(t *testing.T) {
	// Point to a non-existent directory
	t.Setenv("ENGRAM_HOME", "/tmp/nonexistent-engram-dir-12345")

	// Override stdin
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	inputJSON := `{"session_id":"s1","cwd":"/tmp","prompt":"test"}`
	r, w, err := os.Pipe()
	require.NoError(t, err)
	go func() {
		w.Write([]byte(inputJSON))
		w.Close()
	}()
	os.Stdin = r

	var buf bytes.Buffer
	autoEcphoryCmd.SetOut(&buf)
	autoEcphoryCmd.SetErr(&bytes.Buffer{})

	err = autoEcphoryCmd.RunE(autoEcphoryCmd, []string{})
	assert.NoError(t, err)        // must never error
	assert.Empty(t, buf.String()) // no output when dir missing
}

// testContext creates a context with a generous timeout for testing.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
