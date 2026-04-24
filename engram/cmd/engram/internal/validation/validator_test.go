package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateProjectName(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		{"my-project", true},
		{"project_123", true},
		{"ValidProject", true},
		{"", false},
		{"invalid project", false},
		{"project@test", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProjectName(tc.name)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateEngramName(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		{"my-engram", true},
		{"engram-with-special-chars", true},
		{"Engram 123", true},
		{"", false},
		{"❌ Invalid Emoji", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEngramName(tc.name)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateProjectPath(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/tmp/test-project", true},
		{"~/projects/my-project", true},
		{"", false},
		{"invalid/path/with/nonexistent", true}, // Path validation allows non-existent paths
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			err := ValidateProjectPath(tc.path)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	availableTemplates := []string{"default", "minimal", "advanced"}

	testCases := []struct {
		template string
		expected bool
	}{
		{"default", true},
		{"minimal", true},
		{"advanced", true},
		{"", false},
		{"nonexistent", false},
	}

	for _, tc := range testCases {
		t.Run(tc.template, func(t *testing.T) {
			err := ValidateTemplate(tc.template, availableTemplates)
			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
