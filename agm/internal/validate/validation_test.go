package validate

import (
	"testing"
)

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "valid UUID v4",
			uuid:    "d47174c8-0d57-4421-ba76-c2400fb58ba1",
			wantErr: false,
		},
		{
			name:    "another valid UUID",
			uuid:    "ae9b2c0d-a601-4ab0-8fe5-f243d81af7fa",
			wantErr: false,
		},
		{
			name:    "invalid - no dashes",
			uuid:    "d47174c80d574421ba76c2400fb58ba1",
			wantErr: true,
		},
		{
			name:    "invalid - uppercase",
			uuid:    "D47174C8-0D57-4421-BA76-C2400FB58BA1",
			wantErr: true,
		},
		{
			name:    "invalid - too short",
			uuid:    "d47174c8-0d57-4421",
			wantErr: true,
		},
		{
			name:    "invalid - contains command injection",
			uuid:    "d47174c8; rm -rf /",
			wantErr: true,
		},
		{
			name:    "invalid - shell metacharacters",
			uuid:    "d47174c8-0d57-4421-ba76-c2400fb58ba1 && echo hacked",
			wantErr: true,
		},
		{
			name:    "invalid - empty string",
			uuid:    "",
			wantErr: true,
		},
		{
			name:    "invalid - contains newline",
			uuid:    "d47174c8-0d57-4421-ba76-c2400fb58ba1\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUUID(tt.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUUID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid alphanumeric",
			input:   "test-session",
			want:    "agm-validate-test-session",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			input:   "my_test_session",
			want:    "agm-validate-my_test_session",
			wantErr: false,
		},
		{
			name:    "valid with numbers",
			input:   "session123",
			want:    "agm-validate-session123",
			wantErr: false,
		},
		{
			name:    "valid mixed",
			input:   "My-Test_Session-123",
			want:    "agm-validate-My-Test_Session-123",
			wantErr: false,
		},
		{
			name:    "invalid - contains space",
			input:   "my session",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - contains slash",
			input:   "my/session",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - path traversal",
			input:   "../../../evil",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - shell command",
			input:   "test; rm -rf /",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - contains newline",
			input:   "test\nsession",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - special characters",
			input:   "test@session",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - pipe character",
			input:   "test|session",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - ampersand",
			input:   "test&background",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - dollar sign",
			input:   "test$var",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - backtick",
			input:   "test`command`",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid - empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeSessionName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeSessionName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("sanitizeSessionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateUUID_PreventInjection(t *testing.T) {
	// Test specifically for command injection prevention
	injectionAttempts := []string{
		"; rm -rf /",
		"&& echo hacked",
		"| cat /etc/passwd",
		"$(whoami)",
		"`whoami`",
		"'; DROP TABLE sessions;--",
		"../../../etc/passwd",
		"test\n/bin/sh",
	}

	for _, attempt := range injectionAttempts {
		t.Run("injection: "+attempt, func(t *testing.T) {
			err := validateUUID(attempt)
			if err == nil {
				t.Errorf("validateUUID() should reject injection attempt: %s", attempt)
			}
		})
	}
}

func TestSanitizeSessionName_PreventInjection(t *testing.T) {
	// Test specifically for command injection and path traversal prevention
	injectionAttempts := []string{
		"; rm -rf /",
		"&& echo hacked",
		"| cat /etc/passwd",
		"$(whoami)",
		"`whoami`",
		"../../evil",
		"test\n/bin/sh",
		"test && rm",
		"test; ls",
		"test | grep",
	}

	for _, attempt := range injectionAttempts {
		t.Run("injection: "+attempt, func(t *testing.T) {
			_, err := sanitizeSessionName(attempt)
			if err == nil {
				t.Errorf("sanitizeSessionName() should reject injection attempt: %s", attempt)
			}
		})
	}
}
