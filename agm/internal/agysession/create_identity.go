package agysession

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	createIdentityDiscoveryAttempts = 20
	createIdentityDiscoveryDelay    = 500 * time.Millisecond
)

// CreateIdentityTracker owns the provider metadata correlation required for a
// fresh AGY conversation. Callers must hold the workspace create lock from
// Snapshot through Discover so another launch cannot replace AGY's
// workspace-global latest-conversation mapping between those operations.
type CreateIdentityTracker interface {
	Snapshot(context.Context, string) (string, error)
	Discover(context.Context, string, string) (*Metadata, error)
}

type providerCreateIdentityTracker struct {
	userHomeDir func() (string, error)
	findLatest  func(string, string) (*Metadata, error)
	attempts    int
	delay       time.Duration
}

// NewCreateIdentityTracker returns the production AGY identity correlator.
func NewCreateIdentityTracker() CreateIdentityTracker {
	return &providerCreateIdentityTracker{
		userHomeDir: os.UserHomeDir,
		findLatest:  FindLatestForWorkspace,
		attempts:    createIdentityDiscoveryAttempts,
		delay:       createIdentityDiscoveryDelay,
	}
}

func (tracker *providerCreateIdentityTracker) Snapshot(ctx context.Context, workDir string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := contextError(ctx); err != nil {
		return "", err
	}
	homeDir, workspace, err := tracker.identityPaths(workDir)
	if err != nil {
		return "", err
	}
	metadata, err := tracker.findLatest(homeDir, workspace)
	if errors.Is(err, ErrConversationNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("snapshot AGY workspace conversation: %w", err)
	}
	if metadata == nil {
		return "", fmt.Errorf("snapshot AGY workspace conversation: provider returned empty metadata")
	}
	if err := ValidateConversationID(metadata.ConversationID); err != nil {
		return "", fmt.Errorf("snapshot AGY workspace conversation: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return "", err
	}
	return metadata.ConversationID, nil
}

func (tracker *providerCreateIdentityTracker) Discover(ctx context.Context, workDir, previousConversationID string) (*Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if previousConversationID != "" {
		if err := ValidateConversationID(previousConversationID); err != nil {
			return nil, fmt.Errorf("validate pre-create AGY conversation: %w", err)
		}
	}
	homeDir, workspace, err := tracker.identityPaths(workDir)
	if err != nil {
		return nil, err
	}
	attempts := max(1, tracker.attempts)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		metadata, findErr := tracker.findLatest(homeDir, workspace)
		switch {
		case findErr != nil:
			lastErr = findErr
		case metadata == nil:
			lastErr = ErrConversationNotFound
		case metadata.ConversationID == previousConversationID:
			lastErr = fmt.Errorf("provider still reports pre-create conversation %q", previousConversationID)
		default:
			if validateErr := ValidateConversationID(metadata.ConversationID); validateErr != nil {
				lastErr = validateErr
			} else {
				captured := *metadata
				captured.WorkspacePath = workspace
				return &captured, nil
			}
		}
		if attempt < attempts {
			if err := waitForCreateIdentityRetry(ctx, tracker.delay); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("capture native AGY conversation after create: %w", lastErr)
}

func (tracker *providerCreateIdentityTracker) identityPaths(workDir string) (string, string, error) {
	homeDir, err := tracker.userHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("determine home directory: %w", err)
	}
	workspace, err := CanonicalWorkspacePath(workDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve AGY workspace: %w", err)
	}
	return homeDir, workspace, nil
}

func waitForCreateIdentityRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return contextError(ctx)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
