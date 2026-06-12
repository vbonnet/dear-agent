package main

import "testing"

func TestRunCheckArgErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing tier value",
			args:    []string{"--tier"},
			wantErr: "--tier requires a value",
		},
		{
			name:    "invalid tier out of range",
			args:    []string{"--tier", "2"},
			wantErr: "--tier must be 0, 1, or auto",
		},
		{
			name:    "invalid tier non-numeric",
			args:    []string{"--tier", "x"},
			wantErr: "--tier must be 0, 1, or auto",
		},
		{
			name:    "missing changed-files value",
			args:    []string{"--changed-files"},
			wantErr: "--changed-files requires a value",
		},
		{
			name:    "missing format value",
			args:    []string{"--format"},
			wantErr: "--format requires a value",
		},
		{
			name:    "invalid format",
			args:    []string{"--format", "xml"},
			wantErr: "--format must be json or text",
		},
		{
			name:    "unknown flag",
			args:    []string{"--unknown"},
			wantErr: "unknown flag: --unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCheck(tt.args)
			if err == nil {
				t.Fatalf("runCheck(%v) = nil, want error containing %q", tt.args, tt.wantErr)
			}
			if err.Error() != tt.wantErr && len(err.Error()) > 0 {
				// Accept prefix match for flexibility
				found := false
				s := err.Error()
				for i := 0; i+len(tt.wantErr) <= len(s); i++ {
					if s[i:i+len(tt.wantErr)] == tt.wantErr {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("runCheck(%v) error = %q, want error containing %q", tt.args, err, tt.wantErr)
				}
			}
		})
	}
}
