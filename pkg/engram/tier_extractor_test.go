package engram_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

const sampleEngram = `---
name: oauth-pattern
type: pattern
---

> [!T0]
> OAuth 2.0 implementation using authorization code flow with PKCE.

> [!T1]
> ## Overview
>
> This pattern implements OAuth 2.0 with PKCE to prevent authorization
> code interception attacks.
>
> **Key Components:**
> - Authorization endpoint
> - Token endpoint
> - PKCE challenge/verifier

> [!T2]
> ## Detailed Implementation
>
> ### Step 1: Generate PKCE Challenge
>
> ` + "```go" + `
> func generatePKCE() (verifier, challenge string, err error) {
>     // Implementation here
> }
> ` + "```" + `
`

func TestExtractTier_T0(t *testing.T) {
	result, err := engram.ExtractTier(sampleEngram, engram.Tier0)
	require.NoError(t, err)

	assert.Contains(t, result, "OAuth 2.0 implementation")
	assert.Contains(t, result, "PKCE")

	// Should NOT contain T1 or T2 content
	assert.NotContains(t, result, "Overview")
	assert.NotContains(t, result, "Detailed Implementation")
}

func TestExtractTier_T1(t *testing.T) {
	result, err := engram.ExtractTier(sampleEngram, engram.Tier1)
	require.NoError(t, err)

	// Should contain T0 and T1 (cumulative)
	assert.Contains(t, result, "OAuth 2.0 implementation") // T0
	assert.Contains(t, result, "Overview")                 // T1
	assert.Contains(t, result, "Key Components")           // T1

	// Should NOT contain T2 content
	assert.NotContains(t, result, "Detailed Implementation")
	assert.NotContains(t, result, "generatePKCE")
}

func TestExtractTier_T2(t *testing.T) {
	result, err := engram.ExtractTier(sampleEngram, engram.Tier2)
	require.NoError(t, err)

	// Should contain all tiers (T0 + T1 + T2)
	assert.Contains(t, result, "OAuth 2.0 implementation") // T0
	assert.Contains(t, result, "Overview")                 // T1
	assert.Contains(t, result, "Detailed Implementation")  // T2
	assert.Contains(t, result, "generatePKCE")             // T2
}

func TestHasTierMarkers(t *testing.T) {
	assert.True(t, engram.HasTierMarkers(sampleEngram))

	noMarkers := `# Regular Markdown
This has no tier markers.
`
	assert.False(t, engram.HasTierMarkers(noMarkers))
}

func TestValidateTierMarkers_Valid(t *testing.T) {
	err := engram.ValidateTierMarkers(sampleEngram)
	assert.NoError(t, err)
}

func TestValidateTierMarkers_MissingTier(t *testing.T) {
	incomplete := `
> [!T0]
> Summary

> [!T2]
> Full content
`
	err := engram.ValidateTierMarkers(incomplete)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing tier marker: [!T1]")
}

func TestValidateTierMarkers_InvalidNumber(t *testing.T) {
	invalid := `
> [!T0]
> Summary

> [!T1]
> Overview

> [!T5]
> Invalid tier
`
	err := engram.ValidateTierMarkers(invalid)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tier number: 5")
}

func TestExtractTier_NoMarkers(t *testing.T) {
	plain := `# Regular Markdown
This has no tier markers.
It should be treated as backward compatible.
`
	result, err := engram.ExtractTier(plain, engram.Tier0)
	require.NoError(t, err)

	// Should return full content for backward compatibility
	assert.Equal(t, plain, result)
}

func TestExtractTier_NoMarkers_AllTiers(t *testing.T) {
	plain := `# Content without markers`

	// All tiers should return same content for backward compat
	t0, err := engram.ExtractTier(plain, engram.Tier0)
	require.NoError(t, err)

	t1, err := engram.ExtractTier(plain, engram.Tier1)
	require.NoError(t, err)

	t2, err := engram.ExtractTier(plain, engram.Tier2)
	require.NoError(t, err)

	assert.Equal(t, plain, t0)
	assert.Equal(t, plain, t1)
	assert.Equal(t, plain, t2)
}

func TestCountTokens(t *testing.T) {
	text := "This is a test sentence with ten words in it today."
	tokens := engram.CountTokens(text)

	// ~10 words * 0.75 = ~7.5 tokens
	assert.InDelta(t, 7.5, tokens, 2.0)
}

func TestCountTokens_Empty(t *testing.T) {
	tokens := engram.CountTokens("")
	assert.Equal(t, 0, tokens)
}

func TestCountTokens_SingleWord(t *testing.T) {
	tokens := engram.CountTokens("Hello")
	assert.Equal(t, 0, tokens) // 1 word * 0.75 = 0.75 -> rounds to 0
}

func TestExtractTierWithMetrics(t *testing.T) {
	metrics, err := engram.ExtractTierWithMetrics(sampleEngram, engram.Tier0)
	require.NoError(t, err)

	assert.NotEmpty(t, metrics.Content)
	assert.Greater(t, metrics.Tokens, 0)
	assert.Greater(t, metrics.Characters, 0)
	assert.Greater(t, metrics.Lines, 0)
}

func TestExtractTierWithMetrics_AllTiers(t *testing.T) {
	t0, err := engram.ExtractTierWithMetrics(sampleEngram, engram.Tier0)
	require.NoError(t, err)

	t1, err := engram.ExtractTierWithMetrics(sampleEngram, engram.Tier1)
	require.NoError(t, err)

	t2, err := engram.ExtractTierWithMetrics(sampleEngram, engram.Tier2)
	require.NoError(t, err)

	// T1 should have more content than T0
	assert.Greater(t, t1.Tokens, t0.Tokens)
	assert.Greater(t, t1.Characters, t0.Characters)

	// T2 should have more content than T1
	assert.Greater(t, t2.Tokens, t1.Tokens)
	assert.Greater(t, t2.Characters, t1.Characters)
}

func TestExtractTier_PreservesWhitespace(t *testing.T) {
	withWhitespace := `> [!T0]
> First line
>
> Second paragraph with spacing
>
> Third paragraph

> [!T1]
> T1 content

> [!T2]
> T2 content
`
	result, err := engram.ExtractTier(withWhitespace, engram.Tier0)
	require.NoError(t, err)

	// Should preserve empty lines (converted from "> " to "")
	assert.Contains(t, result, "\n\n")
}

func TestExtractTier_StripsBlockquotePrefix(t *testing.T) {
	simple := `> [!T0]
> Line 1
> Line 2

> [!T1]
> T1

> [!T2]
> T2
`
	result, err := engram.ExtractTier(simple, engram.Tier0)
	require.NoError(t, err)

	// Should not contain blockquote prefix
	assert.NotContains(t, result, "> ")

	// Should contain clean content
	assert.Contains(t, result, "Line 1")
	assert.Contains(t, result, "Line 2")
}

func TestValidateTierMarkers_DuplicateTiers(t *testing.T) {
	// Duplicate tier markers are allowed (last wins in extraction)
	// Validation only checks all required tiers exist
	duplicate := `> [!T0]
> Summary 1

> [!T0]
> Summary 2

> [!T1]
> Overview

> [!T2]
> Full
`
	err := engram.ValidateTierMarkers(duplicate)
	assert.NoError(t, err) // Duplicates are valid
}

func TestExtractTier_EmptyTier(t *testing.T) {
	emptyT0 := `> [!T0]

> [!T1]
> T1 content

> [!T2]
> T2 content
`
	result, err := engram.ExtractTier(emptyT0, engram.Tier0)
	require.NoError(t, err)

	// Should return empty (or just whitespace)
	assert.Equal(t, "", strings.TrimSpace(result))
}

func TestExtractTier_NestedBlockquotes(t *testing.T) {
	// Tier markers with nested blockquotes (edge case)
	nested := `> [!T0]
> > This is a quoted section
> > within the tier marker

> [!T1]
> T1

> [!T2]
> T2
`
	result, err := engram.ExtractTier(nested, engram.Tier0)
	require.NoError(t, err)

	// Should strip first "> " but preserve nested ">"
	assert.Contains(t, result, "> This is")
}

func TestTierLevel_Constants(t *testing.T) {
	// Verify tier level constants
	assert.Equal(t, 0, int(engram.Tier0))
	assert.Equal(t, 1, int(engram.Tier1))
	assert.Equal(t, 2, int(engram.Tier2))
}

func TestExtractTier_LargeContent(t *testing.T) {
	// Test with large content (performance)
	var builder strings.Builder
	builder.WriteString("> [!T0]\n")
	for i := 0; i < 1000; i++ {
		builder.WriteString("> Line ")
		fmt.Fprint(&builder, i)
		builder.WriteString("\n")
	}
	builder.WriteString("\n> [!T1]\n> T1\n\n> [!T2]\n> T2\n")

	result, err := engram.ExtractTier(builder.String(), engram.Tier0)
	require.NoError(t, err)

	// Should contain all 1000 lines
	assert.Contains(t, result, "Line 0")
	assert.Contains(t, result, "Line 999")
}

func TestValidateTierMarkers_NoMarkers(t *testing.T) {
	noMarkers := `# Regular content
No tier markers here
`
	err := engram.ValidateTierMarkers(noMarkers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing tier marker: [!T0]")
}

func TestExtractTier_OnlyT0(t *testing.T) {
	onlyT0 := `> [!T0]
> Summary only

> [!T1]
> T1

> [!T2]
> T2
`
	result, err := engram.ExtractTier(onlyT0, engram.Tier0)
	require.NoError(t, err)

	assert.Equal(t, "Summary only", result)
}
