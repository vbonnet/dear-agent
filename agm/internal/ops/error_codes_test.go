package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// TestErrorCatalogPublishesTheActualProblemType guards the cross-surface
// promise AGENTIC-API.md makes: the codes and types it lists are stable
// identifiers agents match on programmatically. A catalog row whose `type`
// cell disagrees with the envelope the code actually emits is worse than an
// absent row — a client written against it matches a string that never
// appears on the wire.
func TestErrorCatalogPublishesTheActualProblemType(t *testing.T) {
	catalog, err := os.ReadFile(filepath.Join("..", "..", "docs", "AGENTIC-API.md"))
	if err != nil {
		t.Fatalf("read error catalog: %v", err)
	}

	published := map[string]string{
		ErrCodeOutputUnavailable: ErrOutputUnavailable("s", "r", nil).Type,
		ErrCodeSessionNotReady:   ErrSessionNotReady("s", "r").Type,
		ErrCodeTmuxNotRunning:    ErrTmuxNotRunning().Type,
	}

	for line := range strings.SplitSeq(string(catalog), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			continue
		}
		code := strings.TrimSpace(cells[1])
		wantType, tracked := published[code]
		if !tracked {
			continue
		}
		gotType := strings.Trim(strings.TrimSpace(cells[3]), "`")
		if gotType != wantType {
			t.Errorf("catalog publishes %s as type %q, but the envelope serializes %q", code, gotType, wantType)
		}
		delete(published, code)
	}
	for code := range published {
		t.Errorf("stable error code %s is emitted but missing from the catalog", code)
	}
}

// TestErrOutputUnavailable_DiagnosesTheConfiguredSocket pins the recovery
// command to the socket this process actually talks to. AGM_TMUX_SOCKET can
// point it at a different server, and a suggestion hard-coded to the default
// sends the operator to probe a backend that had nothing to do with the
// failure — it can answer healthy while the configured one is down.
func TestErrOutputUnavailable_DiagnosesTheConfiguredSocket(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-agm.sock")
	t.Setenv("AGM_TMUX_SOCKET", custom)

	err := ErrOutputUnavailable("worker-1", "socket unreachable", nil)
	joined := strings.Join(err.Suggestions, "\n")
	if !strings.Contains(joined, custom) {
		t.Fatalf("AGM-017 suggestions do not name the configured socket %q: %v", custom, err.Suggestions)
	}
}
