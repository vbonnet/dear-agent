package messages

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePriority(t *testing.T) {
	t.Parallel()

	valid := []struct {
		raw  string
		want Priority
	}{
		{raw: "CRITICAL", want: PriorityCritical},
		{raw: "HIGH", want: PriorityHigh},
		{raw: "MEDIUM", want: PriorityMedium},
		{raw: "LOW", want: PriorityLow},
	}
	for _, test := range valid {
		t.Run(test.raw, func(t *testing.T) {
			t.Parallel()

			got, err := ParsePriority(test.raw)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			assert.True(t, got.IsValid())
		})
	}

	for _, raw := range []string{"", "critical", " CRITICAL", "CRITICAL ", "URGENT"} {
		t.Run("invalid_"+raw, func(t *testing.T) {
			t.Parallel()

			got, err := ParsePriority(raw)
			require.Error(t, err)
			assert.Equal(t, Priority(""), got)
			assert.False(t, Priority(raw).IsValid())
		})
	}
}

func TestPriorityIsValidRejectsExplicitConversion(t *testing.T) {
	t.Parallel()

	assert.False(t, Priority("URGENT").IsValid())
}
