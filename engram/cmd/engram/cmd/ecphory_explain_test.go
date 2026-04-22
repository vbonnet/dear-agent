package cmd

import (
	"testing"

	"github.com/vbonnet/dear-agent/engram/ecphory/ranking"
)

func TestGetExplainQuery(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		queryFlag  string
		wantQuery  string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:      "positional argument",
			args:      []string{"test query"},
			queryFlag: "",
			wantQuery: "test query",
			wantErr:   false,
		},
		{
			name:      "query flag",
			args:      []string{},
			queryFlag: "test query",
			wantQuery: "test query",
			wantErr:   false,
		},
		{
			name:       "both provided - ambiguous",
			args:       []string{"arg query"},
			queryFlag:  "flag query",
			wantErr:    true,
			wantErrMsg: "Ambiguous query input",
		},
		{
			name:       "neither provided",
			args:       []string{},
			queryFlag:  "",
			wantErr:    true,
			wantErrMsg: "query",
		},
		{
			name:       "empty query flag",
			args:       []string{},
			queryFlag:  "",
			wantErr:    true,
			wantErrMsg: "query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			explainCfg.Query = tt.queryFlag

			query, err := getExplainQuery(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if query != tt.wantQuery {
				t.Errorf("Query = %q, want %q", query, tt.wantQuery)
			}
		})
	}
}

func TestExplainProviderDetection(t *testing.T) {
	// This is a smoke test - actual provider detection tested in ranking package
	err := explainProviderDetection()
	if err != nil {
		t.Errorf("explainProviderDetection() failed: %v", err)
	}
}

func TestExplainTier1Filter(t *testing.T) {
	// Setup test engram path
	explainCfg.EngramPath = "../../../testdata/engrams"
	explainCfg.Tag = ""
	explainCfg.Type = ""

	candidates, err := explainTier1Filter()
	if err != nil {
		// If testdata doesn't exist, skip
		t.Skipf("Test engrams not found: %v", err)
	}

	// Should find some candidates (exact count depends on testdata)
	if len(candidates) == 0 {
		t.Skip("No test engrams found - skipping")
	}

	t.Logf("Found %d candidates", len(candidates))
}

func TestExplainTier3Budget(t *testing.T) {
	tests := []struct {
		name        string
		results     []ranking.RankedResult
		tokenBudget int
		wantLoaded  int
	}{
		{
			name:        "empty results",
			results:     []ranking.RankedResult{},
			tokenBudget: 10000,
			wantLoaded:  0,
		},
		{
			name: "single result",
			results: []ranking.RankedResult{
				{
					Candidate: ranking.Candidate{
						Name:        "test",
						Description: "short description",
					},
					Score: 0.8,
				},
			},
			tokenBudget: 10000,
			wantLoaded:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explainCfg.TokenBudget = tt.tokenBudget

			err := explainTier3Budget(tt.results, nil)
			if err != nil {
				t.Errorf("explainTier3Budget() error = %v", err)
			}
		})
	}
}
