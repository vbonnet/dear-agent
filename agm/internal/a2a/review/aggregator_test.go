package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregator_ExtractScores(t *testing.T) {
	a, err := NewAggregator("/tmp/test-channels")
	require.NoError(t, err)

	content := `Some channel content
**Review-Score**: 8/10
More text
**Review-Score**: 7
And another
**Review-Score**: 9/10
`
	scores := a.ExtractScores(content)
	assert.Equal(t, []int{8, 7, 9}, scores)
}

func TestAggregator_ExtractScores_OutOfRange(t *testing.T) {
	a, err := NewAggregator("/tmp/test-channels")
	require.NoError(t, err)

	content := `**Review-Score**: 0
**Review-Score**: 11
**Review-Score**: 5/10`
	scores := a.ExtractScores(content)
	assert.Equal(t, []int{5}, scores)
}

func TestAggregator_CalculateMean(t *testing.T) {
	a, err := NewAggregator("/tmp/test-channels")
	require.NoError(t, err)

	assert.InDelta(t, 8.0, a.CalculateMean([]int{7, 8, 9}), 0.01)
	assert.InDelta(t, 0.0, a.CalculateMean([]int{}), 0.01)
	assert.InDelta(t, 5.0, a.CalculateMean([]int{5}), 0.01)
}

func TestAggregator_DetermineStatus(t *testing.T) {
	a, err := NewAggregator("/tmp/test-channels")
	require.NoError(t, err)

	assert.Equal(t, StatusConsensusReached, a.DetermineStatus(8.0, DefaultThreshold, EscalationThreshold))
	assert.Equal(t, StatusConsensusReached, a.DetermineStatus(7.0, DefaultThreshold, EscalationThreshold))
	assert.Equal(t, StatusBlocked, a.DetermineStatus(5.0, DefaultThreshold, EscalationThreshold))
	assert.Equal(t, StatusEscalate, a.DetermineStatus(3.0, DefaultThreshold, EscalationThreshold))
}

func TestNewAggregator_EmptyDir(t *testing.T) {
	_, err := NewAggregator("")
	assert.Error(t, err)
}
