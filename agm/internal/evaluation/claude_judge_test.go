package evaluation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockClaudeClient is a test implementation of ClaudeClient
type MockClaudeClient struct {
	Response *ClaudeResponse
	Err      error
}

func (m *MockClaudeClient) CreateMessage(ctx context.Context, req ClaudeRequest) (*ClaudeResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Response, nil
}

func TestClaudeJudgeImplementsInterfaces(t *testing.T) {
	// Verify ClaudeJudge implements both Judge and DetailedJudge interfaces
	mockClient := &MockClaudeClient{}
	judge := NewClaudeJudge(mockClient, DefaultClaudeConfig("test-key"))

	var _ Judge = judge
	var _ DetailedJudge = judge
}

func TestClaudeJudge_EvaluateDetailed(t *testing.T) {
	t.Run("successful evaluation with CoT and passing score", func(t *testing.T) {
		// Create mock response with Chain of Thought
		judgeOutput := ClaudeJudgeOutput{
			ChainOfThought: "First, I analyze the input requirements. Second, I compare with expected output. Third, I assess quality metrics.",
			Pass:           true,
			Score:          0.92,
			Reasoning:      "Output exceeds all quality criteria with clear structure and accurate content",
		}
		outputJSON, _ := json.Marshal(judgeOutput)

		mockClient := &MockClaudeClient{
			Response: &ClaudeResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				}{
					{
						Type: "text",
						Text: string(outputJSON),
					},
				},
			},
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:        "correctness",
			Description: "Output should match expected result with high accuracy",
			Threshold:   0.8,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "test input", "expected output", criteria)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Pass)
		assert.Equal(t, 0.92, result.Score)
		assert.Contains(t, result.Reasoning, "Chain of Thought")
		assert.Contains(t, result.Reasoning, "analyze the input")
		assert.Contains(t, result.Reasoning, "exceeds all quality criteria")
	})

	t.Run("successful evaluation with failing score", func(t *testing.T) {
		judgeOutput := ClaudeJudgeOutput{
			ChainOfThought: "Step 1: Check for harmful content. Step 2: Assess tone. Step 3: Evaluate safety metrics.",
			Pass:           false,
			Score:          0.65,
			Reasoning:      "Output does not meet safety threshold due to ambiguous language",
		}
		outputJSON, _ := json.Marshal(judgeOutput)

		mockClient := &MockClaudeClient{
			Response: &ClaudeResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				}{
					{
						Type: "text",
						Text: string(outputJSON),
					},
				},
			},
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:        "safety",
			Description: "Output must not contain harmful content",
			Threshold:   0.95,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "test input", "expected output", criteria)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.Pass)
		assert.Equal(t, 0.65, result.Score)
		assert.Contains(t, result.Reasoning, "Check for harmful content")
		assert.Contains(t, result.Reasoning, "does not meet safety threshold")
	})

	t.Run("API error propagation", func(t *testing.T) {
		mockClient := &MockClaudeClient{
			Err: assert.AnError,
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API call failed")
	})

	t.Run("empty response handling", func(t *testing.T) {
		mockClient := &MockClaudeClient{
			Response: &ClaudeResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				}{},
			},
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no response from API")
	})

	t.Run("non-text content handling", func(t *testing.T) {
		mockClient := &MockClaudeClient{
			Response: &ClaudeResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				}{
					{
						Type: "image",
						Text: "",
					},
				},
			},
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no text content in response")
	})

	t.Run("invalid JSON response handling", func(t *testing.T) {
		mockClient := &MockClaudeClient{
			Response: &ClaudeResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				}{
					{
						Type: "text",
						Text: "invalid json {{}",
					},
				},
			},
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to parse JSON response")
	})

	t.Run("nil client error", func(t *testing.T) {
		judge := &ClaudeJudge{
			client: nil,
			config: DefaultClaudeConfig("test-api-key"),
		}

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "client not initialized")
	})

	t.Run("CoT reasoning included in result", func(t *testing.T) {
		judgeOutput := ClaudeJudgeOutput{
			ChainOfThought: "Detailed step-by-step analysis here",
			Pass:           true,
			Score:          0.88,
			Reasoning:      "Final verdict",
		}
		outputJSON, _ := json.Marshal(judgeOutput)

		mockClient := &MockClaudeClient{
			Response: &ClaudeResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				}{
					{
						Type: "text",
						Text: string(outputJSON),
					},
				},
			},
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.7,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", criteria)
		require.NoError(t, err)
		assert.Contains(t, result.Reasoning, "Chain of Thought:")
		assert.Contains(t, result.Reasoning, "Detailed step-by-step analysis here")
		assert.Contains(t, result.Reasoning, "Summary:")
		assert.Contains(t, result.Reasoning, "Final verdict")
	})
}

func TestClaudeJudge_Evaluate(t *testing.T) {
	t.Run("simple interface returns score", func(t *testing.T) {
		judgeOutput := ClaudeJudgeOutput{
			ChainOfThought: "Analysis steps",
			Pass:           true,
			Score:          0.75,
			Reasoning:      "Good quality",
		}
		outputJSON, _ := json.Marshal(judgeOutput)

		mockClient := &MockClaudeClient{
			Response: &ClaudeResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text,omitempty"`
				}{
					{
						Type: "text",
						Text: string(outputJSON),
					},
				},
			},
		}

		config := DefaultClaudeConfig("test-api-key")
		judge := NewClaudeJudge(mockClient, config)

		score, err := judge.Evaluate(context.Background(), "prompt", "response")
		require.NoError(t, err)
		assert.Equal(t, 0.75, score)
	})
}

func TestDefaultClaudeConfig(t *testing.T) {
	t.Run("creates valid config", func(t *testing.T) {
		config := DefaultClaudeConfig("test-api-key")
		assert.Equal(t, "test-api-key", config.APIKey)
		assert.Equal(t, "claude-3-5-sonnet-20241022", config.Model)
		assert.Equal(t, 0.0, config.Temperature)
		assert.Greater(t, config.MaxTokens, 0)
	})
}

func TestClaudeRequestStructure(t *testing.T) {
	t.Run("request marshals to valid JSON", func(t *testing.T) {
		req := ClaudeRequest{
			Model:     "claude-3-5-sonnet-20241022",
			MaxTokens: 2048,
			System:    "You are a judge",
			Messages: []ClaudeMessage{
				{Role: "user", Content: "Evaluate this"},
			},
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), "claude-3-5-sonnet")
		assert.Contains(t, string(data), "user")
		assert.Contains(t, string(data), "You are a judge")
	})
}
