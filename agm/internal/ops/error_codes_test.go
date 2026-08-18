package ops

import (
	"reflect"
	"testing"
)

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

func TestNewDryRunPreview(t *testing.T) {
	parameters := map[string]string{"session_name": "demo"}
	got := NewDryRunPreview("session/archive", `Would archive session "demo".`, parameters)

	if got.Status != 200 || got.Type != "dry_run" || got.Code != ErrCodeDryRun || got.Title != "Dry run" {
		t.Fatalf("dry-run preview identity = %#v", got)
	}
	if got.Instance != "session/archive" || got.Detail != `Would archive session "demo".` {
		t.Fatalf("dry-run preview context = %#v", got)
	}
	if !reflect.DeepEqual(got.Suggestions, []string{"Remove `--dry-run` to execute."}) {
		t.Fatalf("dry-run suggestions = %#v", got.Suggestions)
	}
	if !reflect.DeepEqual(got.Parameters, parameters) {
		t.Fatalf("dry-run parameters = %#v", got.Parameters)
	}
}
