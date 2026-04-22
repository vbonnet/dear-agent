package stophook

import (
	"strings"
	"testing"
)

func TestReadInput(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantCwd string
		wantErr bool
	}{
		{
			name:    "valid input",
			json:    `{"session_id":"abc","cwd":"/tmp/proj","stop_reason":"user"}`,
			wantCwd: "/tmp/proj",
		},
		{
			name:    "empty cwd",
			json:    `{"session_id":"abc"}`,
			wantCwd: "",
		},
		{
			name:    "invalid json",
			json:    `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := ReadInput(strings.NewReader(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if input.Cwd != tt.wantCwd {
				t.Errorf("Cwd = %q, want %q", input.Cwd, tt.wantCwd)
			}
		})
	}
}
