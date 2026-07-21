package agysession

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateIdentityTrackerCanonicalizesSymlinkWorkspace(t *testing.T) {
	workspace, err := CanonicalWorkspacePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	aliasWorkDir := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(workspace, aliasWorkDir); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}
	var findWorkDirs []string
	tracker := &providerCreateIdentityTracker{
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		findLatest: func(_ string, gotWorkspace string) (*Metadata, error) {
			findWorkDirs = append(findWorkDirs, gotWorkspace)
			if len(findWorkDirs) == 1 {
				return nil, ErrConversationNotFound
			}
			return &Metadata{ConversationID: "native-new", WorkspacePath: gotWorkspace}, nil
		},
		attempts: 1,
	}

	previous, err := tracker.Snapshot(t.Context(), aliasWorkDir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	metadata, err := tracker.Discover(t.Context(), aliasWorkDir, previous)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(findWorkDirs) != 2 || findWorkDirs[0] != workspace || findWorkDirs[1] != workspace || metadata.WorkspacePath != workspace {
		t.Fatalf("canonical snapshot/discovery paths = %v, metadata workspace %q, want %q", findWorkDirs, metadata.WorkspacePath, workspace)
	}
}

func TestCreateIdentityTrackerSnapshotsAbsenceAndDiscoversNewIdentity(t *testing.T) {
	workspace, err := CanonicalWorkspacePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	findCalls := 0
	tracker := &providerCreateIdentityTracker{
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		findLatest: func(_ string, gotWorkspace string) (*Metadata, error) {
			if gotWorkspace != workspace {
				t.Fatalf("workspace = %q, want %q", gotWorkspace, workspace)
			}
			findCalls++
			if findCalls == 1 {
				return nil, ErrConversationNotFound
			}
			return &Metadata{ConversationID: "native-new", WorkspacePath: workspace}, nil
		},
		attempts: 2,
	}

	previous, err := tracker.Snapshot(t.Context(), workspace)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if previous != "" {
		t.Fatalf("previous identity = %q, want empty", previous)
	}
	metadata, err := tracker.Discover(t.Context(), workspace, previous)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if metadata.ConversationID != "native-new" {
		t.Fatalf("discovered identity = %q, want native-new", metadata.ConversationID)
	}
}

func TestCreateIdentityTrackerRejectsStaleIdentityAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	tracker := &providerCreateIdentityTracker{
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		findLatest: func(string, string) (*Metadata, error) {
			cancel()
			return &Metadata{ConversationID: "native-old"}, nil
		},
		attempts: 20,
		delay:    time.Hour,
	}

	_, err := tracker.Discover(ctx, t.TempDir(), "native-old")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover error = %v, want context.Canceled", err)
	}
}

func TestCreateIdentityTrackerFailsClosedOnSnapshotCorruption(t *testing.T) {
	wantErr := errors.New("corrupt provider metadata")
	tracker := &providerCreateIdentityTracker{
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		findLatest:  func(string, string) (*Metadata, error) { return nil, wantErr },
	}

	_, err := tracker.Snapshot(t.Context(), t.TempDir())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Snapshot error = %v, want %v", err, wantErr)
	}
}

func TestCreateIdentityTrackerFailsClosedOnEmptySnapshotMetadata(t *testing.T) {
	tracker := &providerCreateIdentityTracker{
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		findLatest:  func(string, string) (*Metadata, error) { return nil, nil },
	}

	_, err := tracker.Snapshot(t.Context(), t.TempDir())
	if err == nil {
		t.Fatal("Snapshot accepted empty provider metadata")
	}
}
