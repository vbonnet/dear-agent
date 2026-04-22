package evaluation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDetailedJudge is a test implementation of the DetailedJudge interface
type MockDetailedJudge struct {
	Response *JudgeResponse
	Err      error
}

func (m *MockDetailedJudge) EvaluateDetailed(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Response, nil
}

func TestDetailedJudgeInterface(t *testing.T) {
	t.Run("mock judge implements interface", func(t *testing.T) {
		var _ DetailedJudge = &MockDetailedJudge{}
	})

	t.Run("mock judge returns configured response", func(t *testing.T) {
		expectedResponse := &JudgeResponse{
			Pass:      true,
			Score:     0.85,
			Reasoning: "Output meets all criteria",
		}

		mock := &MockDetailedJudge{Response: expectedResponse}
		ctx := context.Background()

		criteria := EvaluationCriteria{
			Name:        "correctness",
			Description: "Output should match expected result",
			Threshold:   0.7,
		}

		result, err := mock.EvaluateDetailed(ctx, "input", "expected", criteria)
		require.NoError(t, err)
		assert.Equal(t, expectedResponse, result)
	})

	t.Run("mock judge propagates error", func(t *testing.T) {
		expectedErr := assert.AnError
		mock := &MockDetailedJudge{Err: expectedErr}
		ctx := context.Background()

		criteria := EvaluationCriteria{
			Name:      "test",
			Threshold: 0.5,
		}

		result, err := mock.EvaluateDetailed(ctx, "input", "expected", criteria)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, result)
	})
}

func TestJudgeResponse(t *testing.T) {
	t.Run("pass when score meets threshold", func(t *testing.T) {
		response := &JudgeResponse{
			Pass:      true,
			Score:     0.8,
			Reasoning: "Good quality output",
		}
		assert.True(t, response.Pass)
		assert.Equal(t, 0.8, response.Score)
	})

	t.Run("fail when score below threshold", func(t *testing.T) {
		response := &JudgeResponse{
			Pass:      false,
			Score:     0.3,
			Reasoning: "Output quality insufficient",
		}
		assert.False(t, response.Pass)
		assert.Less(t, response.Score, 0.5)
	})
}

func TestEvaluationCriteria(t *testing.T) {
	t.Run("criteria has required fields", func(t *testing.T) {
		criteria := EvaluationCriteria{
			Name:        "safety",
			Description: "Output must not contain harmful content",
			Threshold:   0.9,
		}
		assert.Equal(t, "safety", criteria.Name)
		assert.NotEmpty(t, criteria.Description)
		assert.Equal(t, 0.9, criteria.Threshold)
	})
}

// Interface compliance tests moved to individual implementation test files
// (gpt4_judge_test.go and claude_judge_test.go)
