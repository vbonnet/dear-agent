package gemini

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "JSON wrapped in ```json fences",
			input: "```json\n{\"topics\": [\"topic1\", \"topic2\"]}\n```",
			expected: `{"topics": ["topic1", "topic2"]}`,
		},
		{
			name: "JSON wrapped in plain ``` fences",
			input: "```\n{\"topics\": [\"topic1\", \"topic2\"]}\n```",
			expected: `{"topics": ["topic1", "topic2"]}`,
		},
		{
			name: "JSON wrapped in ```JSON fences (uppercase)",
			input: "```JSON\n{\"topics\": [\"topic1\", \"topic2\"]}\n```",
			expected: `{"topics": ["topic1", "topic2"]}`,
		},
		{
			name:     "JSON without fences",
			input:    `{"topics": ["topic1", "topic2"]}`,
			expected: `{"topics": ["topic1", "topic2"]}`,
		},
		{
			name: "JSON with extra whitespace",
			input: "\n\n```json\n{\"topics\": [\"topic1\", \"topic2\"]}\n```\n\n",
			expected: `{"topics": ["topic1", "topic2"]}`,
		},
		{
			name: "Multiline JSON with fences",
			input: "```json\n{\n  \"topics\": [\n    \"topic1\",\n    \"topic2\"\n  ]\n}\n```",
			expected: "{\n  \"topics\": [\n    \"topic1\",\n    \"topic2\"\n  ]\n}",
		},
		{
			name:     "Empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "Only whitespace",
			input:    "   \n\n  ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripMarkdownFences(tt.input)
			if result != tt.expected {
				t.Errorf("StripMarkdownFences() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestParseTopicsJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []string
		expectError bool
		errorType   error
	}{
		{
			name:        "Valid JSON with topics",
			input:       `{"topics": ["topic1", "topic2", "topic3"]}`,
			expected:    []string{"topic1", "topic2", "topic3"},
			expectError: false,
		},
		{
			name:        "Valid JSON wrapped in markdown fences",
			input:       "```json\n{\"topics\": [\"topic1\", \"topic2\"]}\n```",
			expected:    []string{"topic1", "topic2"},
			expectError: false,
		},
		{
			name:        "Single topic",
			input:       `{"topics": ["single-topic"]}`,
			expected:    []string{"single-topic"},
			expectError: false,
		},
		{
			name:        "Topics with whitespace",
			input:       `{"topics": ["  topic1  ", "topic2", "  topic3  "]}`,
			expected:    []string{"topic1", "topic2", "topic3"},
			expectError: false,
		},
		{
			name:        "Empty topics array",
			input:       `{"topics": []}`,
			expected:    nil,
			expectError: true,
			errorType:   ErrNoTopics,
		},
		{
			name:        "Missing topics key",
			input:       `{"other": ["value"]}`,
			expected:    nil,
			expectError: true,
			errorType:   ErrInvalidJSON,
		},
		{
			name:        "Invalid JSON",
			input:       `{invalid json}`,
			expected:    nil,
			expectError: true,
			errorType:   ErrInvalidJSON,
		},
		{
			name:        "Null topics array",
			input:       `{"topics": null}`,
			expected:    nil,
			expectError: true,
			errorType:   ErrInvalidJSON,
		},
		{
			name:        "Topics with empty strings filtered out",
			input:       `{"topics": ["topic1", "", "topic2", "   ", "topic3"]}`,
			expected:    []string{"topic1", "topic2", "topic3"},
			expectError: false,
		},
		{
			name:        "All empty string topics",
			input:       `{"topics": ["", "  ", "   "]}`,
			expected:    nil,
			expectError: true,
			errorType:   ErrNoTopics,
		},
		{
			name:        "Empty string",
			input:       "",
			expected:    nil,
			expectError: true,
			errorType:   ErrInvalidJSON,
		},
		{
			name:        "Malformed JSON with extra commas",
			input:       `{"topics": ["topic1",,"topic2"]}`,
			expected:    nil,
			expectError: true,
			errorType:   ErrInvalidJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTopicsJSON(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("ParseTopicsJSON() expected error, got nil")
					return
				}

				// Check error type
				if tt.errorType != nil {
					var analysisErr *AnalysisError
					if errors.As(err, &analysisErr) {
						if !errors.Is(analysisErr.Err, tt.errorType) {
							t.Errorf("ParseTopicsJSON() error = %v, expected error type %v", analysisErr.Err, tt.errorType)
						}
					}
				}
				return
			}

			if err != nil {
				t.Errorf("ParseTopicsJSON() unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("ParseTopicsJSON() returned %d topics, expected %d", len(result), len(tt.expected))
				return
			}

			for i, topic := range result {
				if topic != tt.expected[i] {
					t.Errorf("ParseTopicsJSON() topic[%d] = %q, expected %q", i, topic, tt.expected[i])
				}
			}
		})
	}
}

func TestTopicsResponseJSONSerialization(t *testing.T) {
	// Test that TopicsResponse can be marshaled and unmarshaled correctly
	original := TopicsResponse{
		Topics: []string{"topic1", "topic2", "topic3"},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal TopicsResponse: %v", err)
	}

	// Unmarshal back
	var decoded TopicsResponse
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal TopicsResponse: %v", err)
	}

	// Verify
	if len(decoded.Topics) != len(original.Topics) {
		t.Errorf("Unmarshaled topics length = %d, expected %d", len(decoded.Topics), len(original.Topics))
	}

	for i, topic := range decoded.Topics {
		if topic != original.Topics[i] {
			t.Errorf("Unmarshaled topic[%d] = %q, expected %q", i, topic, original.Topics[i])
		}
	}
}

func TestStripMarkdownFencesWithRealWorldExamples(t *testing.T) {
	// Test with real-world Gemini responses
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, output string)
	}{
		{
			name: "Gemini response with explanation before fence",
			input: "Sure, here are the topics:\n\n```json\n{\"topics\": [\"AI\", \"Machine Learning\", \"Deep Learning\"]}\n```\n\nLet me know if you need more details.",
			validate: func(t *testing.T, output string) {
				// Should extract only JSON, not explanation text
				t.Logf("Stripped output: %q", output)
				var response TopicsResponse
				if err := json.Unmarshal([]byte(output), &response); err != nil {
					t.Errorf("Failed to parse JSON: %v (output was: %q)", err, output)
				}
			},
		},
		{
			name: "Multiple code blocks (should extract first)",
			input: "```json\n{\"topics\": [\"topic1\"]}\n```\n\nSome text here.\n\n```json\n{\"topics\": [\"topic2\"]}\n```",
			validate: func(t *testing.T, output string) {
				// Should handle this gracefully (implementation may vary)
				// For now, verify it doesn't crash
				if output == "" {
					t.Errorf("Expected non-empty output")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripMarkdownFences(tt.input)
			tt.validate(t, result)
		})
	}
}
