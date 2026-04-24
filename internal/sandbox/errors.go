// Package sandbox provides filesystem isolation for repository operations.
package sandbox

import "fmt"

// ErrorCode identifies specific error conditions.
type ErrorCode int

const (
	// ErrCodeUnsupportedPlatform indicates the platform/provider is not supported
	ErrCodeUnsupportedPlatform ErrorCode = iota
	// ErrCodeKernelTooOld indicates the kernel version is too old for the required features
	ErrCodeKernelTooOld
	// ErrCodeMountFailed indicates a mount operation failed
	ErrCodeMountFailed
	// ErrCodeUnmountFailed indicates an unmount operation failed
	ErrCodeUnmountFailed
	// ErrCodePermissionDenied indicates insufficient permissions
	ErrCodePermissionDenied
	// ErrCodeRepoNotFound indicates the repository was not found
	ErrCodeRepoNotFound
	// ErrCodeSandboxNotFound indicates the sandbox was not found
	ErrCodeSandboxNotFound
	// ErrCodeInvalidConfig indicates the configuration is invalid
	ErrCodeInvalidConfig
	// ErrCodeResourceExhausted indicates system resources are exhausted
	ErrCodeResourceExhausted
	// ErrCodeCleanupFailed indicates cleanup operations failed
	ErrCodeCleanupFailed
	// ErrCodeOrphanedMount indicates an orphaned mount was detected
	ErrCodeOrphanedMount
	// ErrCodeFileSystemNotSupported indicates the filesystem type is not supported
	ErrCodeFileSystemNotSupported
)

// ErrorCategory categorizes errors for recovery strategies.
type ErrorCategory string

const (
	// CategoryTransient indicates the error might resolve if retried
	CategoryTransient ErrorCategory = "transient"
	// CategoryConfiguration indicates a configuration problem
	CategoryConfiguration ErrorCategory = "configuration"
	// CategoryPermission indicates a permission problem
	CategoryPermission ErrorCategory = "permission"
	// CategoryResource indicates a resource exhaustion problem
	CategoryResource ErrorCategory = "resource"
	// CategoryPlatform indicates a platform compatibility problem
	CategoryPlatform ErrorCategory = "platform"
	// CategoryState indicates an invalid state problem
	CategoryState ErrorCategory = "state"
)

// Error represents a structured sandbox error with recovery guidance.
type Error struct {
	Code              ErrorCode
	Message           string
	Cause             error
	Context           map[string]string
	RecoveryHint      string
	DiagnosticCommand string
	Category          ErrorCategory
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// IsRetriable returns true if the operation might succeed if retried.
func (e *Error) IsRetriable() bool {
	return e.Category == CategoryTransient
}

// NewError creates a new Error with the given code and message.
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:     code,
		Message:  message,
		Category: errorCodeCategory(code),
	}
}

// WrapError wraps an existing error with a sandbox error code and message.
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:     code,
		Message:  message,
		Cause:    cause,
		Category: errorCodeCategory(code),
	}
}

// WithContext adds context key-value pairs to an error.
func WithContext(err *Error, key, value string) *Error {
	if err.Context == nil {
		err.Context = make(map[string]string)
	}
	err.Context[key] = value
	return err
}

// WithRecoveryHint adds a recovery hint to the error.
func WithRecoveryHint(err *Error, hint string) *Error {
	err.RecoveryHint = hint
	return err
}

// WithDiagnostic adds a diagnostic command to the error.
func WithDiagnostic(err *Error, command string) *Error {
	err.DiagnosticCommand = command
	return err
}

// WithCategory overrides the default error category.
func WithCategory(err *Error, category ErrorCategory) *Error {
	err.Category = category
	return err
}

// errorCodeCategory returns the default category for an error code.
func errorCodeCategory(code ErrorCode) ErrorCategory {
	switch code {
	case ErrCodeUnsupportedPlatform, ErrCodeKernelTooOld, ErrCodeFileSystemNotSupported:
		return CategoryPlatform
	case ErrCodeMountFailed, ErrCodeUnmountFailed:
		return CategoryTransient
	case ErrCodePermissionDenied:
		return CategoryPermission
	case ErrCodeRepoNotFound, ErrCodeSandboxNotFound, ErrCodeOrphanedMount:
		return CategoryState
	case ErrCodeInvalidConfig:
		return CategoryConfiguration
	case ErrCodeResourceExhausted, ErrCodeCleanupFailed:
		return CategoryResource
	default:
		return CategoryState
	}
}

// NewMountPermissionError creates an error for mount permission failures.
func NewMountPermissionError(cause error) *Error {
	err := WrapError(ErrCodePermissionDenied,
		"insufficient permissions to mount filesystem", cause)
	return WithRecoveryHint(
		WithDiagnostic(err, "id -u && groups"),
		"Mount operations may require superuser privileges. Try running with sudo or check kernel version for rootless mount support (Linux 5.11+).",
	)
}

// NewKernelTooOldError creates an error for kernel version incompatibility.
func NewKernelTooOldError(currentVersion string, requiredMajor, requiredMinor int) *Error {
	err := NewError(ErrCodeKernelTooOld,
		fmt.Sprintf("kernel version %s is too old (need %d.%d+)", currentVersion, requiredMajor, requiredMinor))
	return WithRecoveryHint(
		WithDiagnostic(err, "uname -r"),
		fmt.Sprintf("Upgrade kernel to version %d.%d or later. Check your distribution's package manager for kernel updates.", requiredMajor, requiredMinor),
	)
}

// NewResourceExhaustedError creates an error for resource exhaustion.
func NewResourceExhaustedError(resourceType string, cause error) *Error {
	err := WrapError(ErrCodeResourceExhausted,
		fmt.Sprintf("system resource exhausted: %s", resourceType), cause)
	hint := "System resources are exhausted. "
	diagnostic := ""
	switch resourceType {
	case "file descriptors":
		hint += "Increase file descriptor limits using 'ulimit -n <value>' or edit /etc/security/limits.conf."
		diagnostic = "ulimit -n && cat /proc/sys/fs/file-max"
	case "mounts":
		hint += "Too many active mounts. Clean up unused mounts or increase /proc/sys/fs/mount-max."
		diagnostic = "cat /proc/mounts | wc -l && cat /proc/sys/fs/mount-max"
	case "disk space":
		hint += "Free up disk space or use a different workspace directory."
		diagnostic = "df -h"
	default:
		hint += "Check system resource usage with diagnostic commands."
		diagnostic = "ulimit -a && df -h"
	}
	return WithRecoveryHint(WithDiagnostic(err, diagnostic), hint)
}

// NewCleanupFailedError creates an error for cleanup failures.
func NewCleanupFailedError(path string, cause error) *Error {
	err := WrapError(ErrCodeCleanupFailed,
		fmt.Sprintf("failed to cleanup sandbox resources: %s", path), cause)
	return WithRecoveryHint(
		WithDiagnostic(err, fmt.Sprintf("ls -la %s && cat /proc/mounts | grep %s", path, path)),
		fmt.Sprintf("Manual cleanup may be required. Check for active mounts with 'mount | grep %s', unmount with 'umount %s', then remove with 'rm -rf %s'.", path, path, path),
	)
}

// NewOrphanedMountError creates an error for orphaned mount detection.
func NewOrphanedMountError(mountPoint string) *Error {
	err := NewError(ErrCodeOrphanedMount,
		fmt.Sprintf("orphaned mount detected: %s", mountPoint))
	return WithRecoveryHint(
		WithDiagnostic(err, fmt.Sprintf("mount | grep %s && lsof +D %s", mountPoint, mountPoint)),
		fmt.Sprintf("Orphaned mount found. Check for processes using the mount with 'lsof +D %s', kill if needed, then unmount with 'umount %s' or 'umount -l %s' for lazy unmount.", mountPoint, mountPoint, mountPoint),
	)
}

// NewFileSystemNotSupportedError creates an error for unsupported filesystems.
func NewFileSystemNotSupportedError(fsType, operation string) *Error {
	err := NewError(ErrCodeFileSystemNotSupported,
		fmt.Sprintf("filesystem '%s' does not support %s", fsType, operation))
	return WithRecoveryHint(
		WithDiagnostic(err, "df -T . && mount | grep $(df . | tail -1 | awk '{print $1}')"),
		fmt.Sprintf("The current filesystem (%s) does not support %s. Use a supported filesystem (APFS on macOS, ext4/btrfs/xfs on Linux) or try a different workspace directory.", fsType, operation),
	)
}

// NewRepoNotFoundError creates an error for missing repositories.
func NewRepoNotFoundError(repoPath string) *Error {
	err := NewError(ErrCodeRepoNotFound,
		fmt.Sprintf("repository not found: %s", repoPath))
	return WithRecoveryHint(
		WithDiagnostic(err, fmt.Sprintf("ls -la %s && git -C %s status 2>&1", repoPath, repoPath)),
		fmt.Sprintf("Verify the repository path exists and is a valid git repository. Check permissions and that the path is absolute: %s", repoPath),
	)
}

// NewSandboxNotFoundError creates an error for missing sandboxes.
func NewSandboxNotFoundError(sandboxID string) *Error {
	err := NewError(ErrCodeSandboxNotFound,
		fmt.Sprintf("sandbox not found: %s", sandboxID))
	return WithRecoveryHint(
		WithDiagnostic(err, "mount | grep overlay"),
		fmt.Sprintf("The sandbox '%s' does not exist or has already been destroyed. List active sandboxes to verify.", sandboxID),
	)
}

// NewInvalidConfigError creates an error for configuration issues.
func NewInvalidConfigError(field, reason string) *Error {
	err := NewError(ErrCodeInvalidConfig,
		fmt.Sprintf("invalid configuration: %s - %s", field, reason))
	return WithRecoveryHint(err,
		fmt.Sprintf("Fix the configuration field '%s': %s. Check the documentation for valid values and formats.", field, reason),
	)
}
