package types

import (
	"strings"
	"testing"
)

func TestFlags_Validate(t *testing.T) {
	tests := []struct {
		name    string
		flags   Flags
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid flags - no custom prompt",
			flags: Flags{
				Type: "article",
			},
			wantErr: false,
		},
		{
			name: "Valid flags - with analyze prompt",
			flags: Flags{
				AnalyzePrompt: "Custom prompt",
				Type:          "video",
			},
			wantErr: false,
		},
		{
			name: "Invalid type",
			flags: Flags{
				Type: "invalid-type",
			},
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name: "Valid mode - general",
			flags: Flags{
				Mode: "general",
			},
			wantErr: false,
		},
		{
			name: "Valid mode - competitive",
			flags: Flags{
				Mode: "competitive",
			},
			wantErr: false,
		},
		{
			name: "Invalid mode",
			flags: Flags{
				Mode: "invalid-mode",
			},
			wantErr: true,
			errMsg:  "invalid mode",
		},
		{
			name: "Valid discovery limit",
			flags: Flags{
				DiscoveryLimit: 10,
			},
			wantErr: false,
		},
		{
			name: "Invalid discovery limit - negative",
			flags: Flags{
				DiscoveryLimit: -1,
			},
			wantErr: true,
			errMsg:  "invalid discovery-limit",
		},
		{
			name: "Invalid discovery limit - too high",
			flags: Flags{
				DiscoveryLimit: 25,
			},
			wantErr: true,
			errMsg:  "invalid discovery-limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flags.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Flags.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Flags.Validate() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}
