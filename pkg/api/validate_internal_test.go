package api

import "testing"

func TestValidateExecArg(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"plain identifier", "workflow.yaml", false},
		{"absolute path", "/tmp/x.yaml", false},
		{"path with spaces", "/tmp/foo bar.yaml", false},
		{"unicode letters", "résumé.yaml", false},

		{"empty", "", true},
		{"newline", "x\ny", true},
		{"carriage return", "x\ry", true},
		{"null byte", "x\x00y", true},
		{"DEL", "x\x7fy", true},
		{"bell", "x\x07y", true},
		{"tab", "x\ty", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecArg(tt.in)
			if tt.wantErr && err == nil {
				t.Errorf("validateExecArg(%q) = nil, want error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateExecArg(%q) = %v, want nil", tt.in, err)
			}
		})
	}
}

func TestValidateInputKV(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		val     string
		wantErr bool
	}{
		{"alnum key + plain val", "FILE", "/tmp/x", false},
		{"key with underscore", "input_file", "v", false},
		{"key with dash", "input-file", "v", false},
		{"key with dot", "input.file", "v", false},
		{"value with spaces", "k", "hello world", false},
		{"empty value", "k", "", false}, // explicit empty is allowed

		{"empty key", "", "v", true},
		{"key with space", "in put", "v", true},
		{"key with slash", "input/file", "v", true},
		{"key with newline", "k\nv", "v", true},
		{"value with newline", "k", "a\nb", true},
		{"value with carriage return", "k", "a\rb", true},
		{"value with null", "k", "a\x00b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputKV(tt.key, tt.val)
			if tt.wantErr && err == nil {
				t.Errorf("validateInputKV(%q, %q) = nil, want error", tt.key, tt.val)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateInputKV(%q, %q) = %v, want nil", tt.key, tt.val, err)
			}
		})
	}
}
