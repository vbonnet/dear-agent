package tokentracking

import (
	"testing"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

func TestExtractTokens(t *testing.T) {
	tests := []struct {
		name        string
		response    *APIResponse
		want        *TokenUsage
		wantErr     bool
		errContains string
	}{
		{
			name: "valid response with all fields",
			response: &APIResponse{
				Usage: struct {
					InputTokens         int `json:"input_tokens"`
					OutputTokens        int `json:"output_tokens"`
					CacheCreationTokens int `json:"cache_creation_input_tokens"`
					CacheReadTokens     int `json:"cache_read_input_tokens"`
				}{
					InputTokens:         100,
					OutputTokens:        50,
					CacheCreationTokens: 10,
					CacheReadTokens:     5,
				},
			},
			want: &TokenUsage{
				InputTokens:         100,
				OutputTokens:        50,
				CacheCreationTokens: 10,
				CacheReadTokens:     5,
				TotalTokens:         150,
			},
			wantErr: false,
		},
		{
			name: "valid response with zero cache tokens",
			response: &APIResponse{
				Usage: struct {
					InputTokens         int `json:"input_tokens"`
					OutputTokens        int `json:"output_tokens"`
					CacheCreationTokens int `json:"cache_creation_input_tokens"`
					CacheReadTokens     int `json:"cache_read_input_tokens"`
				}{
					InputTokens:  1000,
					OutputTokens: 500,
				},
			},
			want: &TokenUsage{
				InputTokens:  1000,
				OutputTokens: 500,
				TotalTokens:  1500,
			},
			wantErr: false,
		},
		{
			name:        "nil response",
			response:    nil,
			wantErr:     true,
			errContains: "nil",
		},
		{
			name: "negative input tokens",
			response: &APIResponse{
				Usage: struct {
					InputTokens         int `json:"input_tokens"`
					OutputTokens        int `json:"output_tokens"`
					CacheCreationTokens int `json:"cache_creation_input_tokens"`
					CacheReadTokens     int `json:"cache_read_input_tokens"`
				}{
					InputTokens:  -100,
					OutputTokens: 50,
				},
			},
			wantErr:     true,
			errContains: "invalid input_tokens",
		},
		{
			name: "negative output tokens",
			response: &APIResponse{
				Usage: struct {
					InputTokens         int `json:"input_tokens"`
					OutputTokens        int `json:"output_tokens"`
					CacheCreationTokens int `json:"cache_creation_input_tokens"`
					CacheReadTokens     int `json:"cache_read_input_tokens"`
				}{
					InputTokens:  100,
					OutputTokens: -50,
				},
			},
			wantErr:     true,
			errContains: "invalid output_tokens",
		},
		{
			name: "unrealistic total tokens (>1M)",
			response: &APIResponse{
				Usage: struct {
					InputTokens         int `json:"input_tokens"`
					OutputTokens        int `json:"output_tokens"`
					CacheCreationTokens int `json:"cache_creation_input_tokens"`
					CacheReadTokens     int `json:"cache_read_input_tokens"`
				}{
					InputTokens:  600000,
					OutputTokens: 500000,
				},
			},
			wantErr:     true,
			errContains: "unrealistic total_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractTokens(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractTokens() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errContains != "" {
					if !contains(err.Error(), tt.errContains) {
						t.Errorf("ExtractTokens() error = %v, should contain %q", err, tt.errContains)
					}
				}
				return
			}
			if got == nil {
				t.Errorf("ExtractTokens() returned nil, want %+v", tt.want)
				return
			}
			if got.InputTokens != tt.want.InputTokens {
				t.Errorf("InputTokens = %v, want %v", got.InputTokens, tt.want.InputTokens)
			}
			if got.OutputTokens != tt.want.OutputTokens {
				t.Errorf("OutputTokens = %v, want %v", got.OutputTokens, tt.want.OutputTokens)
			}
			if got.TotalTokens != tt.want.TotalTokens {
				t.Errorf("TotalTokens = %v, want %v", got.TotalTokens, tt.want.TotalTokens)
			}
		})
	}
}

func TestExtractTokensFromJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		want        *TokenUsage
		wantErr     bool
		errContains string
	}{
		{
			name: "valid JSON",
			json: `{"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}`,
			want: &TokenUsage{
				InputTokens:         100,
				OutputTokens:        50,
				CacheCreationTokens: 10,
				CacheReadTokens:     5,
				TotalTokens:         150,
			},
			wantErr: false,
		},
		{
			name:        "malformed JSON",
			json:        `{"usage":{"input_tokens":`,
			wantErr:     true,
			errContains: "unmarshal",
		},
		{
			name:    "missing usage field",
			json:    `{"other_field":"value"}`,
			want:    &TokenUsage{TotalTokens: 0},
			wantErr: false, // Valid JSON, just zero tokens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractTokensFromJSON([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractTokensFromJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errContains != "" {
					if !contains(err.Error(), tt.errContains) {
						t.Errorf("ExtractTokensFromJSON() error = %v, should contain %q", err, tt.errContains)
					}
				}
				return
			}
			if got == nil {
				t.Errorf("ExtractTokensFromJSON() returned nil, want %+v", tt.want)
				return
			}
			if got.TotalTokens != tt.want.TotalTokens {
				t.Errorf("TotalTokens = %v, want %v", got.TotalTokens, tt.want.TotalTokens)
			}
		})
	}
}

func TestDetermineSeverityLevel(t *testing.T) {
	tests := []struct {
		name        string
		totalTokens int
		want        telemetry.Level
	}{
		{
			name:        "zero tokens -> INFO",
			totalTokens: 0,
			want:        telemetry.LevelInfo,
		},
		{
			name:        "small usage (1000) -> INFO",
			totalTokens: 1000,
			want:        telemetry.LevelInfo,
		},
		{
			name:        "boundary below WARN (49999) -> INFO",
			totalTokens: 49999,
			want:        telemetry.LevelInfo,
		},
		{
			name:        "boundary at WARN (50000) -> WARN",
			totalTokens: 50000,
			want:        telemetry.LevelWarn,
		},
		{
			name:        "high usage (75000) -> WARN",
			totalTokens: 75000,
			want:        telemetry.LevelWarn,
		},
		{
			name:        "boundary below ERROR (99999) -> WARN",
			totalTokens: 99999,
			want:        telemetry.LevelWarn,
		},
		{
			name:        "boundary at ERROR (100000) -> ERROR",
			totalTokens: 100000,
			want:        telemetry.LevelError,
		},
		{
			name:        "extremely high (500000) -> ERROR",
			totalTokens: 500000,
			want:        telemetry.LevelError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineSeverityLevel(tt.totalTokens)
			if got != tt.want {
				t.Errorf("DetermineSeverityLevel(%d) = %v, want %v", tt.totalTokens, got, tt.want)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
