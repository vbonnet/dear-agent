package fuzzy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindSimilar(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		candidates []string
		threshold  float64
		expected   []string // Expected names in order
		minSim     float64  // Minimum expected similarity for first match
	}{
		{
			name:       "exact match",
			input:      "test",
			candidates: []string{"test", "best", "rest"},
			threshold:  0.6,
			expected:   []string{"test"},
			minSim:     1.0,
		},
		{
			name:       "typo - transposition (lower threshold)",
			input:      "tset",
			candidates: []string{"test", "best", "asset"},
			threshold:  0.5,
			expected:   []string{"asset", "test"}, // asset: 0.60, test: 0.50, best: 0.25 (below threshold)
			minSim:     0.50,
		},
		{
			name:       "typo - higher threshold filters more",
			input:      "tset",
			candidates: []string{"test", "best", "asset"},
			threshold:  0.6,
			expected:   []string{"asset"}, // Only "asset" has 0.60 similarity (2/5)
			minSim:     0.60,
		},
		{
			name:       "partial match - short vs long",
			input:      "myproj",
			candidates: []string{"myproject", "project", "mywork"},
			threshold:  0.6,
			expected:   []string{"myproject"},
			minSim:     0.60,
		},
		{
			name:       "case insensitive",
			input:      "MyProject",
			candidates: []string{"myproject", "MYPROJECT", "MyProJeCt"},
			threshold:  0.6,
			expected:   []string{"myproject", "MYPROJECT", "MyProJeCt"},
			minSim:     1.0,
		},
		{
			name:       "no matches below threshold",
			input:      "abc",
			candidates: []string{"xyz", "def", "ghi"},
			threshold:  0.6,
			expected:   []string{},
		},
		{
			name:       "transposition",
			input:      "poject",
			candidates: []string{"project", "object", "subject"},
			threshold:  0.6,
			expected:   []string{"project"},
			minSim:     0.70,
		},
		{
			name:       "top 5 limit",
			input:      "test",
			candidates: []string{"test1", "test2", "test3", "test4", "test5", "test6", "test7"},
			threshold:  0.6,
			expected:   []string{"test1", "test2", "test3", "test4", "test5"},
		},
		{
			name:       "empty candidates",
			input:      "test",
			candidates: []string{},
			threshold:  0.6,
			expected:   []string{},
		},
		{
			name:       "multiple good matches",
			input:      "session",
			candidates: []string{"session-1", "session-2", "my-session", "sessions"},
			threshold:  0.6,
			expected:   []string{"sessions", "session-1", "session-2", "my-session"},
		},
		{
			name:       "threshold filtering",
			input:      "test",
			candidates: []string{"test", "testing", "te", "t", "xyz"},
			threshold:  0.7,
			expected:   []string{"test"}, // Only exact match meets 0.7 threshold; "testing" is 0.57
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := FindSimilar(tt.input, tt.candidates, tt.threshold)

			// Check number of results
			assert.LessOrEqual(t, len(matches), 5, "Should return at most 5 results")

			if len(tt.expected) == 0 {
				assert.Empty(t, matches, "Expected no matches")
				return
			}

			// Check we got matches
			assert.NotEmpty(t, matches, "Expected matches but got none")

			// Check first match meets minimum similarity
			if tt.minSim > 0 && len(matches) > 0 {
				assert.GreaterOrEqual(t, matches[0].Similarity, tt.minSim,
					"First match similarity should be >= %f", tt.minSim)
			}

			// Check expected names are present (order matters for first few)
			gotNames := make([]string, len(matches))
			for i, m := range matches {
				gotNames[i] = m.Name
			}

			// Verify all expected names are in results
			for _, expName := range tt.expected {
				assert.Contains(t, gotNames, expName, "Expected name %s not found in results", expName)
			}

			// Check sorting (similarity descending)
			for i := 1; i < len(matches); i++ {
				assert.GreaterOrEqual(t, matches[i-1].Similarity, matches[i].Similarity,
					"Results should be sorted by similarity (descending)")
			}
		})
	}
}

func TestFindSimilarEdgeCases(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		matches := FindSimilar("", []string{"test", "best"}, 0.6)
		// Empty input should have high distance from non-empty strings
		// Similarity = 1 - (distance / maxLen) where maxLen > 0
		// This should result in low similarity, below threshold
		assert.Empty(t, matches)
	})

	t.Run("zero threshold", func(t *testing.T) {
		matches := FindSimilar("test", []string{"xyz"}, 0.0)
		// Even completely different strings should match with threshold 0.0
		assert.NotEmpty(t, matches)
	})

	t.Run("threshold 1.0 exact only", func(t *testing.T) {
		matches := FindSimilar("test", []string{"test", "testing", "tset"}, 1.0)
		assert.Len(t, matches, 1)
		assert.Equal(t, "test", matches[0].Name)
		assert.Equal(t, 1.0, matches[0].Similarity)
		assert.Equal(t, 0, matches[0].Distance)
	})
}

func TestMatchStructure(t *testing.T) {
	matches := FindSimilar("test", []string{"test", "best"}, 0.6)

	assert.Len(t, matches, 2)

	// First match should be exact
	assert.Equal(t, "test", matches[0].Name)
	assert.Equal(t, 1.0, matches[0].Similarity)
	assert.Equal(t, 0, matches[0].Distance)

	// Second match
	assert.Equal(t, "best", matches[1].Name)
	assert.Greater(t, matches[1].Similarity, 0.6)
	assert.Greater(t, matches[1].Distance, 0)
}

// Benchmark to ensure performance
func BenchmarkFindSimilar(b *testing.B) {
	candidates := make([]string, 100)
	for i := 0; i < 100; i++ {
		candidates[i] = "session-" + string(rune('a'+i%26))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FindSimilar("sesion", candidates, 0.6)
	}
}
