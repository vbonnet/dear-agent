package evaluation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGPT4Client is a test implementation of GPT4Client
type MockGPT4Client struct {
	Response    *GPT4Response
	Err         error
	LastRequest GPT4Request // captures the most recent request for assertions
}

func (m *MockGPT4Client) CreateChatCompletion(ctx context.Context, req GPT4Request) (*GPT4Response, error) {
	m.LastRequest = req
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Response, nil
}

func TestGPT4JudgeImplementsInterfaces(t *testing.T) {
	// Verify GPT4Judge implements both Judge and DetailedJudge interfaces
	mockClient := &MockGPT4Client{}
	judge := NewGPT4Judge(mockClient, DefaultGPT4Config("test-key"))

	var _ Judge = judge
	var _ DetailedJudge = judge
}

func TestGPT4Judge_EvaluateDetailed(t *testing.T) {
	t.Run("successful evaluation with passing score", func(t *testing.T) {
		// Create mock response with structured JSON
		judgeOutput := JudgeOutput{
			Pass:      true,
			Score:     0.85,
			Reasoning: "Output meets all requirements and is well-formed",
		}
		outputJSON, _ := json.Marshal(judgeOutput)

		mockClient := &MockGPT4Client{
			Response: &GPT4Response{
				Choices: []struct {
					Index   int `json:"index"`
					Message struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
				}{
					{
						Message: struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						}{
							Role:    "assistant",
							Content: string(outputJSON),
						},
					},
				},
			},
		}

		config := DefaultGPT4Config("test-api-key")
		judge := NewGPT4Judge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:        "correctness",
			Description: "Output should match expected result",
			Threshold:   0.7,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "test input", "expected output", "actual output", criteria)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Pass)
		assert.Equal(t, 0.85, result.Score)
		assert.Contains(t, result.Reasoning, "meets all requirements")
	})

	t.Run("successful evaluation with failing score", func(t *testing.T) {
		judgeOutput := JudgeOutput{
			Pass:      false,
			Score:     0.45,
			Reasoning: "Output does not meet the quality threshold",
		}
		outputJSON, _ := json.Marshal(judgeOutput)

		mockClient := &MockGPT4Client{
			Response: &GPT4Response{
				Choices: []struct {
					Index   int `json:"index"`
					Message struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
				}{
					{
						Message: struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						}{
							Role:    "assistant",
							Content: string(outputJSON),
						},
					},
				},
			},
		}

		config := DefaultGPT4Config("test-api-key")
		judge := NewGPT4Judge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:        "safety",
			Description: "Output must not contain harmful content",
			Threshold:   0.9,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "test input", "expected output", "actual output", criteria)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.Pass)
		assert.Equal(t, 0.45, result.Score)
		assert.Contains(t, result.Reasoning, "does not meet")
	})

	t.Run("API error propagation", func(t *testing.T) {
		mockClient := &MockGPT4Client{
			Err: assert.AnError,
		}

		config := DefaultGPT4Config("test-api-key")
		judge := NewGPT4Judge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", "actual", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "API call failed")
	})

	t.Run("empty response handling", func(t *testing.T) {
		mockClient := &MockGPT4Client{
			Response: &GPT4Response{
				Choices: []struct {
					Index   int `json:"index"`
					Message struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
				}{},
			},
		}

		config := DefaultGPT4Config("test-api-key")
		judge := NewGPT4Judge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", "actual", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no response from API")
	})

	t.Run("invalid JSON response handling", func(t *testing.T) {
		mockClient := &MockGPT4Client{
			Response: &GPT4Response{
				Choices: []struct {
					Index   int `json:"index"`
					Message struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
				}{
					{
						Message: struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						}{
							Role:    "assistant",
							Content: "invalid json {{}",
						},
					},
				},
			},
		}

		config := DefaultGPT4Config("test-api-key")
		judge := NewGPT4Judge(mockClient, config)

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", "actual", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to parse JSON response")
	})

	t.Run("nil client error", func(t *testing.T) {
		judge := &GPT4Judge{
			client: nil,
			config: DefaultGPT4Config("test-api-key"),
		}

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := judge.EvaluateDetailed(context.Background(), "input", "output", "actual", criteria)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "client not initialized")
	})
}

// TestGPT4Judge_GradesActualNotExpected is a regression test for a bug where
// the judge passed expectedOutput into both the Expected and Actual prompt
// slots, so it always graded expected-vs-expected and could never detect a
// wrong answer. The prompt must place the distinct actual output into the
// Actual Output slot.
func TestGPT4Judge_GradesActualNotExpected(t *testing.T) {
	judgeOutput := JudgeOutput{
		Pass:      false,
		Score:     0.1,
		Reasoning: "actual does not match expected",
	}
	outputJSON, _ := json.Marshal(judgeOutput)

	mockClient := &MockGPT4Client{
		Response: &GPT4Response{
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{Role: "assistant", Content: string(outputJSON)},
				},
			},
		},
	}

	judge := NewGPT4Judge(mockClient, DefaultGPT4Config("test-api-key"))
	criteria := EvaluationCriteria{Name: "correctness", Threshold: 0.8}

	const expected = "REFERENCE_ANSWER_42"
	const actual = "WRONG_ANSWER_99"

	_, err := judge.EvaluateDetailed(context.Background(), "input", expected, actual, criteria)
	require.NoError(t, err)

	// Messages are [system, user]; the user message carries the evaluation prompt.
	require.Len(t, mockClient.LastRequest.Messages, 2)
	prompt := mockClient.LastRequest.Messages[1].Content

	// The actual output must reach the Actual Output slot; the expected output
	// the Expected Output slot. The bug filled both slots with expected.
	assert.Contains(t, prompt, "Expected Output: "+expected)
	assert.Contains(t, prompt, "Actual Output: "+actual)
	assert.NotContains(t, prompt, "Actual Output: "+expected,
		"actual slot must contain the actual output, not the expected output")
}

func TestGPT4Judge_Evaluate(t *testing.T) {
	t.Run("simple interface returns score", func(t *testing.T) {
		judgeOutput := JudgeOutput{
			Pass:      true,
			Score:     0.8,
			Reasoning: "Good quality",
		}
		outputJSON, _ := json.Marshal(judgeOutput)

		mockClient := &MockGPT4Client{
			Response: &GPT4Response{
				Choices: []struct {
					Index   int `json:"index"`
					Message struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
				}{
					{
						Message: struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						}{
							Role:    "assistant",
							Content: string(outputJSON),
						},
					},
				},
			},
		}

		config := DefaultGPT4Config("test-api-key")
		judge := NewGPT4Judge(mockClient, config)

		score, err := judge.Evaluate(context.Background(), "prompt", "response")
		require.NoError(t, err)
		assert.Equal(t, 0.8, score)
	})
}

func TestDefaultGPT4Config(t *testing.T) {
	t.Run("creates valid config", func(t *testing.T) {
		config := DefaultGPT4Config("test-api-key")
		assert.Equal(t, "test-api-key", config.APIKey)
		assert.Equal(t, "gpt-4-turbo-preview", config.Model)
		assert.Equal(t, 0.0, config.Temperature)
		assert.Greater(t, config.MaxTokens, 0)
	})
}

func TestGPT4RequestStructure(t *testing.T) {
	t.Run("request marshals to valid JSON", func(t *testing.T) {
		req := GPT4Request{
			Model: "gpt-4",
			Messages: []GPT4Message{
				{Role: "system", Content: "You are a judge"},
				{Role: "user", Content: "Evaluate this"},
			},
			Temperature: 0.0,
			MaxTokens:   1000,
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), "gpt-4")
		assert.Contains(t, string(data), "system")
	})

	t.Run("request with schema marshals correctly", func(t *testing.T) {
		schema := &JSONSchema{
			Name:   "test_schema",
			Strict: true,
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"score": map[string]interface{}{"type": "number"},
				},
			},
		}

		req := GPT4Request{
			Model:    "gpt-4",
			Messages: []GPT4Message{{Role: "user", Content: "test"}},
			ResponseFormat: &ResponseFormat{
				Type:       "json_schema",
				JSONSchema: schema,
			},
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), "json_schema")
		assert.Contains(t, string(data), "test_schema")
	})
}
