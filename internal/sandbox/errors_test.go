package sandbox

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestErrorWrapping verifies errors.Is/As works correctly.
func TestErrorWrapping(t *testing.T) {
	t.Run("errors.Is with wrapped error", func(t *testing.T) {
		cause := fmt.Errorf("underlying error")
		err := WrapError(ErrCodeMountFailed, "mount failed", cause)

		if !errors.Is(err, cause) {
			t.Error("errors.Is should find wrapped cause")
		}
	})

	t.Run("errors.As with sandbox error", func(t *testing.T) {
		err := NewError(ErrCodePermissionDenied, "access denied")
		var sandboxErr *Error
		if !errors.As(err, &sandboxErr) {
			t.Error("errors.As should unwrap sandbox error")
		}
		if sandboxErr.Code != ErrCodePermissionDenied {
			t.Errorf("expected code %d, got %d", ErrCodePermissionDenied, sandboxErr.Code)
		}
	})

	t.Run("chained wrapping", func(t *testing.T) {
		original := fmt.Errorf("root cause")
		wrapped := WrapError(ErrCodeMountFailed, "mount failed", original)

		if !errors.Is(wrapped, original) {
			t.Error("should find original error in chain")
		}
	})
}

// TestRecoveryHints verifies recovery hints are present and helpful.
func TestRecoveryHints(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		wantHint bool
	}{
		{
			name:     "mount permission error has hint",
			err:      NewMountPermissionError(fmt.Errorf("permission denied")),
			wantHint: true,
		},
		{
			name:     "kernel too old has hint",
			err:      NewKernelTooOldError("5.4.0", 5, 11),
			wantHint: true,
		},
		{
			name:     "resource exhausted has hint",
			err:      NewResourceExhaustedError("file descriptors", fmt.Errorf("too many open files")),
			wantHint: true,
		},
		{
			name:     "cleanup failed has hint",
			err:      NewCleanupFailedError("/tmp/sandbox", fmt.Errorf("device busy")),
			wantHint: true,
		},
		{
			name:     "orphaned mount has hint",
			err:      NewOrphanedMountError("/mnt/sandbox"),
			wantHint: true,
		},
		{
			name:     "filesystem not supported has hint",
			err:      NewFileSystemNotSupportedError("ntfs", "reflink cloning"),
			wantHint: true,
		},
		{
			name:     "repo not found has hint",
			err:      NewRepoNotFoundError("/path/to/repo"),
			wantHint: true,
		},
		{
			name:     "sandbox not found has hint",
			err:      NewSandboxNotFoundError("sb-123"),
			wantHint: true,
		},
		{
			name:     "invalid config has hint",
			err:      NewInvalidConfigError("SessionID", "must not be empty"),
			wantHint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantHint && tt.err.RecoveryHint == "" {
				t.Error("expected recovery hint to be present")
			}
			if tt.wantHint && len(tt.err.RecoveryHint) < 20 {
				t.Errorf("recovery hint too short: %q", tt.err.RecoveryHint)
			}
		})
	}
}

// TestDiagnosticCommands verifies diagnostic commands are valid.
func TestDiagnosticCommands(t *testing.T) {
	tests := []struct {
		name        string
		err         *Error
		wantCommand bool
	}{
		{
			name:        "mount permission error has diagnostic",
			err:         NewMountPermissionError(fmt.Errorf("permission denied")),
			wantCommand: true,
		},
		{
			name:        "kernel too old has diagnostic",
			err:         NewKernelTooOldError("5.4.0", 5, 11),
			wantCommand: true,
		},
		{
			name:        "resource exhausted has diagnostic",
			err:         NewResourceExhaustedError("file descriptors", fmt.Errorf("too many")),
			wantCommand: true,
		},
		{
			name:        "cleanup failed has diagnostic",
			err:         NewCleanupFailedError("/tmp/sandbox", fmt.Errorf("busy")),
			wantCommand: true,
		},
		{
			name:        "orphaned mount has diagnostic",
			err:         NewOrphanedMountError("/mnt/sandbox"),
			wantCommand: true,
		},
		{
			name:        "filesystem not supported has diagnostic",
			err:         NewFileSystemNotSupportedError("ntfs", "operation"),
			wantCommand: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantCommand && tt.err.DiagnosticCommand == "" {
				t.Error("expected diagnostic command to be present")
			}
			// Verify command looks valid (contains common unix commands)
			if tt.wantCommand {
				cmd := tt.err.DiagnosticCommand
				validCommands := []string{"uname", "mount", "lsof", "ulimit", "df", "ls", "cat", "grep", "git", "id", "groups"}
				hasValidCommand := false
				for _, valid := range validCommands {
					if strings.Contains(cmd, valid) {
						hasValidCommand = true
						break
					}
				}
				if !hasValidCommand {
					t.Errorf("diagnostic command doesn't contain recognized command: %q", cmd)
				}
			}
		})
	}
}

// TestErrorMessages verifies error messages are helpful.
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name        string
		err         *Error
		wantContain []string
	}{
		{
			name:        "mount permission error message",
			err:         NewMountPermissionError(fmt.Errorf("denied")),
			wantContain: []string{"permission", "mount"},
		},
		{
			name:        "kernel too old message",
			err:         NewKernelTooOldError("5.4.0", 5, 11),
			wantContain: []string{"kernel", "5.4.0", "5.11"},
		},
		{
			name:        "resource exhausted message",
			err:         NewResourceExhaustedError("file descriptors", nil),
			wantContain: []string{"resource", "file descriptors"},
		},
		{
			name:        "cleanup failed message",
			err:         NewCleanupFailedError("/tmp/sandbox", fmt.Errorf("busy")),
			wantContain: []string{"cleanup", "/tmp/sandbox"},
		},
		{
			name:        "orphaned mount message",
			err:         NewOrphanedMountError("/mnt/sandbox"),
			wantContain: []string{"orphaned", "mount", "/mnt/sandbox"},
		},
		{
			name:        "repo not found message",
			err:         NewRepoNotFoundError("/path/to/repo"),
			wantContain: []string{"repository", "/path/to/repo"},
		},
		{
			name:        "sandbox not found message",
			err:         NewSandboxNotFoundError("sb-123"),
			wantContain: []string{"sandbox", "sb-123"},
		},
		{
			name:        "invalid config message",
			err:         NewInvalidConfigError("SessionID", "must not be empty"),
			wantContain: []string{"configuration", "SessionID", "empty"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			for _, want := range tt.wantContain {
				if !strings.Contains(strings.ToLower(msg), strings.ToLower(want)) {
					t.Errorf("error message %q should contain %q", msg, want)
				}
			}
		})
	}
}

// TestErrorCategorization verifies categories are correct.
func TestErrorCategorization(t *testing.T) {
	tests := []struct {
		name      string
		err       *Error
		wantCat   ErrorCategory
		wantRetry bool
	}{
		{
			name:      "mount permission is permission category",
			err:       NewMountPermissionError(fmt.Errorf("denied")),
			wantCat:   CategoryPermission,
			wantRetry: false,
		},
		{
			name:      "kernel too old is platform category",
			err:       NewKernelTooOldError("5.4.0", 5, 11),
			wantCat:   CategoryPlatform,
			wantRetry: false,
		},
		{
			name:      "resource exhausted is resource category",
			err:       NewResourceExhaustedError("mounts", nil),
			wantCat:   CategoryResource,
			wantRetry: false,
		},
		{
			name:      "cleanup failed is resource category",
			err:       NewCleanupFailedError("/tmp", nil),
			wantCat:   CategoryResource,
			wantRetry: false,
		},
		{
			name:      "orphaned mount is state category",
			err:       NewOrphanedMountError("/mnt"),
			wantCat:   CategoryState,
			wantRetry: false,
		},
		{
			name:      "filesystem not supported is platform category",
			err:       NewFileSystemNotSupportedError("ntfs", "op"),
			wantCat:   CategoryPlatform,
			wantRetry: false,
		},
		{
			name:      "repo not found is state category",
			err:       NewRepoNotFoundError("/repo"),
			wantCat:   CategoryState,
			wantRetry: false,
		},
		{
			name:      "sandbox not found is state category",
			err:       NewSandboxNotFoundError("sb-1"),
			wantCat:   CategoryState,
			wantRetry: false,
		},
		{
			name:      "invalid config is configuration category",
			err:       NewInvalidConfigError("field", "reason"),
			wantCat:   CategoryConfiguration,
			wantRetry: false,
		},
		{
			name:      "mount failed is transient (can retry)",
			err:       WrapError(ErrCodeMountFailed, "mount failed", nil),
			wantCat:   CategoryTransient,
			wantRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Category != tt.wantCat {
				t.Errorf("expected category %q, got %q", tt.wantCat, tt.err.Category)
			}
			if tt.err.IsRetriable() != tt.wantRetry {
				t.Errorf("expected retriable=%v, got %v", tt.wantRetry, tt.err.IsRetriable())
			}
		})
	}
}

// TestErrorContext verifies context can be added to errors.
func TestErrorContext(t *testing.T) {
	err := NewError(ErrCodeMountFailed, "mount failed")
	WithContext(err, "sandbox_id", "sb-123")
	WithContext(err, "mount_point", "/mnt/sandbox")

	if err.Context["sandbox_id"] != "sb-123" {
		t.Errorf("expected sandbox_id=sb-123, got %q", err.Context["sandbox_id"])
	}
	if err.Context["mount_point"] != "/mnt/sandbox" {
		t.Errorf("expected mount_point=/mnt/sandbox, got %q", err.Context["mount_point"])
	}
}

// TestErrorChaining verifies WithRecoveryHint and WithDiagnostic can be chained.
func TestErrorChaining(t *testing.T) {
	err := NewError(ErrCodeMountFailed, "mount failed")
	err = WithRecoveryHint(err, "try sudo")
	err = WithDiagnostic(err, "mount | grep overlay")
	err = WithContext(err, "path", "/mnt")

	if err.RecoveryHint != "try sudo" {
		t.Errorf("expected recovery hint 'try sudo', got %q", err.RecoveryHint)
	}
	if err.DiagnosticCommand != "mount | grep overlay" {
		t.Errorf("expected diagnostic 'mount | grep overlay', got %q", err.DiagnosticCommand)
	}
	if err.Context["path"] != "/mnt" {
		t.Errorf("expected context path=/mnt, got %q", err.Context["path"])
	}
}

// TestResourceExhaustedTypes verifies different resource types have appropriate hints.
func TestResourceExhaustedTypes(t *testing.T) {
	tests := []struct {
		resourceType string
		wantHint     string
		wantDiag     string
	}{
		{
			resourceType: "file descriptors",
			wantHint:     "ulimit",
			wantDiag:     "ulimit -n",
		},
		{
			resourceType: "mounts",
			wantHint:     "mount",
			wantDiag:     "cat /proc/mounts",
		},
		{
			resourceType: "disk space",
			wantHint:     "disk space",
			wantDiag:     "df -h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			err := NewResourceExhaustedError(tt.resourceType, nil)

			if !strings.Contains(err.RecoveryHint, tt.wantHint) {
				t.Errorf("recovery hint should mention %q, got %q", tt.wantHint, err.RecoveryHint)
			}
			if !strings.Contains(err.DiagnosticCommand, tt.wantDiag) {
				t.Errorf("diagnostic should contain %q, got %q", tt.wantDiag, err.DiagnosticCommand)
			}
		})
	}
}

// TestErrorCodeCategory verifies default categories for error codes.
func TestErrorCodeCategory(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want ErrorCategory
	}{
		{ErrCodeUnsupportedPlatform, CategoryPlatform},
		{ErrCodeKernelTooOld, CategoryPlatform},
		{ErrCodeMountFailed, CategoryTransient},
		{ErrCodeUnmountFailed, CategoryTransient},
		{ErrCodePermissionDenied, CategoryPermission},
		{ErrCodeRepoNotFound, CategoryState},
		{ErrCodeSandboxNotFound, CategoryState},
		{ErrCodeInvalidConfig, CategoryConfiguration},
		{ErrCodeResourceExhausted, CategoryResource},
		{ErrCodeCleanupFailed, CategoryResource},
		{ErrCodeOrphanedMount, CategoryState},
		{ErrCodeFileSystemNotSupported, CategoryPlatform},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			got := errorCodeCategory(tt.code)
			if got != tt.want {
				t.Errorf("errorCodeCategory(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

// TestWithCategory verifies category can be overridden.
func TestWithCategory(t *testing.T) {
	err := NewError(ErrCodeMountFailed, "mount failed")
	// Default should be transient
	if err.Category != CategoryTransient {
		t.Errorf("expected default category %q, got %q", CategoryTransient, err.Category)
	}

	// Override to configuration
	WithCategory(err, CategoryConfiguration)
	if err.Category != CategoryConfiguration {
		t.Errorf("expected overridden category %q, got %q", CategoryConfiguration, err.Category)
	}
}
