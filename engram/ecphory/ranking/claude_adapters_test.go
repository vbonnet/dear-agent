package ranking

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

type recordingCostSink struct {
	cost *costtrack.CostInfo
	meta *costtrack.CostMetadata
}

func (s *recordingCostSink) Record(_ context.Context, cost *costtrack.CostInfo, meta *costtrack.CostMetadata) error {
	s.cost = cost
	s.meta = meta
	return nil
}

func (s *recordingCostSink) Close(context.Context) error { return nil }

type claudeAdapterCase struct {
	name         string
	providerName string
	model        string
	apiErrText   string
	newProvider  func(anthropic.Client, costtrack.CostSink) Provider
}

func claudeAdapterCases() []claudeAdapterCase {
	return []claudeAdapterCase{
		{
			name:         "Anthropic",
			providerName: "anthropic",
			model:        "claude-anthropic-test",
			apiErrText:   "failed to call Anthropic API",
			newProvider: func(client anthropic.Client, sink costtrack.CostSink) Provider {
				return &AnthropicProvider{client: client, model: "claude-anthropic-test", costSink: sink}
			},
		},
		{
			name:         "Vertex AI Claude",
			providerName: "vertexai-claude",
			model:        "claude-vertex-test",
			apiErrText:   "failed to call Vertex AI Claude",
			newProvider: func(client anthropic.Client, sink costtrack.CostSink) Provider {
				return &VertexAIClaudeProvider{client: client, model: "claude-vertex-test", costSink: sink}
			},
		},
	}
}

type claudeRankRequest struct {
	Method    string `json:"-"`
	Path      string `json:"-"`
	Model     string `json:"model"`
	MaxTokens int64  `json:"max_tokens"`
	System    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"system"`
	Messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
}

func TestClaudeRankingAdaptersUseSharedPolicyAndKeepCostMetadata(t *testing.T) {
	candidates := []Candidate{{Name: "first"}, {Name: "second"}}
	wantResults := []RankedResult{
		{Candidate: candidates[1], Score: 0.9, Reasoning: "best"},
		{Candidate: candidates[0], Score: 0.4, Reasoning: "last"},
	}

	for _, tc := range claudeAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan claudeRankRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request claudeRankRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				request.Method = r.Method
				request.Path = r.URL.Path
				requests <- request
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg_rank_test",
					"type":        "message",
					"role":        "assistant",
					"model":       tc.model,
					"content":     []map[string]string{{"type": "text", "text": `[{"index":1,"score":0.9,"reasoning":"best"},{"index":0,"score":0.4,"reasoning":"last"}]`}},
					"stop_reason": "end_turn",
					"usage": map[string]int64{
						"input_tokens":                11,
						"output_tokens":               7,
						"cache_creation_input_tokens": 3,
						"cache_read_input_tokens":     5,
					},
				}); err != nil {
					t.Errorf("encode Claude response: %v", err)
				}
			}))
			defer server.Close()

			client := anthropic.NewClient(
				option.WithoutEnvironmentDefaults(),
				option.WithAPIKey("test-key"),
				option.WithBaseURL(server.URL),
				option.WithHTTPClient(server.Client()),
				option.WithMaxRetries(0),
			)
			sink := &recordingCostSink{}
			provider := tc.newProvider(client, sink)

			got, err := provider.Rank(context.Background(), "rank this", candidates)
			require.NoError(t, err)
			assert.Equal(t, wantResults, got)

			request := <-requests
			assert.Equal(t, http.MethodPost, request.Method)
			// This is the stable SDK path before any provider-specific HTTP middleware.
			assert.Equal(t, "/v1/messages", request.Path)
			assert.Equal(t, tc.model, request.Model)
			assert.Equal(t, int64(4096), request.MaxTokens)
			require.Len(t, request.System, 1)
			assert.Equal(t, "text", request.System[0].Type)
			assert.Equal(t, claudeRankingSystemPrompt, request.System[0].Text)
			require.Len(t, request.Messages, 1)
			assert.Equal(t, "user", request.Messages[0].Role)
			require.Len(t, request.Messages[0].Content, 1)
			assert.Equal(t, "text", request.Messages[0].Content[0].Type)
			assert.Equal(t, buildClaudeRankingPrompt("rank this", candidates), request.Messages[0].Content[0].Text)

			require.NotNil(t, sink.cost)
			require.NotNil(t, sink.meta)
			assert.Equal(t, tc.providerName, sink.cost.Provider)
			assert.Equal(t, tc.model, sink.cost.Model)
			assert.Equal(t, 11, sink.cost.Tokens.Input)
			assert.Equal(t, 7, sink.cost.Tokens.Output)
			if tc.providerName == "anthropic" {
				assert.Equal(t, 3, sink.cost.Tokens.CacheWrite)
				assert.Equal(t, 5, sink.cost.Tokens.CacheRead)
			} else {
				assert.Zero(t, sink.cost.Tokens.CacheWrite)
				assert.Zero(t, sink.cost.Tokens.CacheRead)
			}
			assert.Equal(t, "rank", sink.meta.Operation)
			assert.Equal(t, "msg_rank_test", sink.meta.RequestID)
		})
	}
}

func TestClaudeRankingAdaptersPreserveAPIErrorContext(t *testing.T) {
	for _, tc := range claudeAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "transport failed", http.StatusBadGateway)
			}))
			defer server.Close()

			client := anthropic.NewClient(
				option.WithoutEnvironmentDefaults(),
				option.WithAPIKey("test-key"),
				option.WithBaseURL(server.URL),
				option.WithHTTPClient(server.Client()),
				option.WithMaxRetries(0),
			)
			sink := &recordingCostSink{}
			provider := tc.newProvider(client, sink)

			results, err := provider.Rank(context.Background(), "rank this", []Candidate{{Name: "candidate"}})
			require.ErrorContains(t, err, tc.apiErrText)
			assert.Nil(t, results)
			assert.Nil(t, sink.cost)
			assert.Nil(t, sink.meta)
		})
	}
}
