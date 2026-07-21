package ops

import "testing"

func TestStableErrorCodesAreUnique(t *testing.T) {
	codes := map[string]string{
		"session not found":   ErrCodeSessionNotFound,
		"session archived":    ErrCodeSessionArchived,
		"tmux not running":    ErrCodeTmuxNotRunning,
		"dolt unavailable":    ErrCodeDoltUnavailable,
		"invalid input":       ErrCodeInvalidInput,
		"permission denied":   ErrCodePermissionDenied,
		"session exists":      ErrCodeSessionExists,
		"harness unavailable": ErrCodeHarnessUnavailable,
		"workspace not found": ErrCodeWorkspaceNotFound,
		"uuid not associated": ErrCodeUUIDNotAssociated,
		"storage error":       ErrCodeStorageError,
		"verification failed": ErrCodeVerificationFailed,
		"kill protected":      ErrCodeKillProtected,
		"active session kill": ErrCodeActiveSessionKill,
		"lock timeout":        ErrCodeLockTimeout,
		"dry run":             ErrCodeDryRun,
	}
	owners := make(map[string]string, len(codes))
	for owner, code := range codes {
		if previous, exists := owners[code]; exists {
			t.Errorf("stable error code %s is shared by %q and %q", code, previous, owner)
		}
		owners[code] = owner
	}
}
