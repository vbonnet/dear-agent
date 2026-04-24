package errors //nolint:revive // test package for internal errors

import (
	"errors"
	"testing"
)

func TestDevlogError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *DevlogError
		expected string
	}{
		{
			name: "with path",
			err: &DevlogError{
				Op:   "load config",
				Path: "/path/to/config.yaml",
				Err:  ErrConfigNotFound,
			},
			expected: "load config failed for /path/to/config.yaml: config file not found",
		},
		{
			name: "without path",
			err: &DevlogError{
				Op:  "validate config",
				Err: ErrConfigInvalid,
			},
			expected: "validate config failed: config is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDevlogError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &DevlogError{
		Op:  "test op",
		Err: underlying,
	}

	unwrapped := err.Unwrap()
	if !errors.Is(unwrapped, underlying) {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, underlying)
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name string
		op   string
		err  error
		want error
	}{
		{
			name: "wraps error",
			op:   "test operation",
			err:  errors.New("base error"),
			want: &DevlogError{
				Op:  "test operation",
				Err: errors.New("base error"),
			},
		},
		{
			name: "returns nil for nil error",
			op:   "test operation",
			err:  nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Wrap(tt.op, tt.err)
			if tt.want == nil {
				if got != nil {
					t.Errorf("Wrap() = %v, want nil", got)
				}
				return
			}

			var devErr *DevlogError
			if !errors.As(got, &devErr) {
				t.Fatalf("Wrap() returned %T, want *DevlogError", got)
			}

			if devErr.Op != tt.op {
				t.Errorf("Op = %q, want %q", devErr.Op, tt.op)
			}
			if devErr.Err.Error() != tt.err.Error() {
				t.Errorf("Err = %v, want %v", devErr.Err, tt.err)
			}
		})
	}
}

func TestWrapPath(t *testing.T) {
	tests := []struct {
		name     string
		op       string
		path     string
		err      error
		wantOp   string
		wantPath string
	}{
		{
			name:     "wraps error with path",
			op:       "load config",
			path:     "/path/to/config",
			err:      errors.New("file not found"),
			wantOp:   "load config",
			wantPath: "/path/to/config",
		},
		{
			name: "returns nil for nil error",
			op:   "test",
			path: "/path",
			err:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapPath(tt.op, tt.path, tt.err)
			if tt.err == nil {
				if got != nil {
					t.Errorf("WrapPath() = %v, want nil", got)
				}
				return
			}

			var devErr *DevlogError
			if !errors.As(got, &devErr) {
				t.Fatalf("WrapPath() returned %T, want *DevlogError", got)
			}

			if devErr.Op != tt.wantOp {
				t.Errorf("Op = %q, want %q", devErr.Op, tt.wantOp)
			}
			if devErr.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", devErr.Path, tt.wantPath)
			}
		})
	}
}

func TestErrorsIs(t *testing.T) {
	// Test that errors.Is works with wrapped DevlogError
	wrapped := &DevlogError{
		Op:  "test",
		Err: ErrConfigNotFound,
	}

	if !errors.Is(wrapped, ErrConfigNotFound) {
		t.Error("errors.Is(wrapped, ErrConfigNotFound) = false, want true")
	}
}

func TestErrorsAs(t *testing.T) {
	// Test that errors.As works with DevlogError
	err := &DevlogError{
		Op:   "test",
		Path: "/path",
		Err:  errors.New("base"),
	}

	var devErr *DevlogError
	if !errors.As(err, &devErr) {
		t.Error("errors.As() = false, want true")
	}

	if devErr.Op != "test" {
		t.Errorf("As() extracted Op = %q, want %q", devErr.Op, "test")
	}
}

func TestExitCodes(t *testing.T) {
	// Verify exit codes have expected values
	tests := []struct {
		name     string
		exitCode int
		expected int
	}{
		{"ExitSuccess", ExitSuccess, 0},
		{"ExitGeneralError", ExitGeneralError, 1},
		{"ExitConfigError", ExitConfigError, 2},
		{"ExitGitError", ExitGitError, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.exitCode != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.exitCode, tt.expected)
			}
		})
	}
}
