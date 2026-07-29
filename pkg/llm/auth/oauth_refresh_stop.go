package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrRefreshStopped signals that an earlier refresh crossed a boundary where
// the on-disk token may already be spent and the operator has not explicitly
// re-armed refreshing yet.
var ErrRefreshStopped = errors.New("oauth refresh is stopped after a non-persisted refresh outcome; operator intervention is required")

// ErrRefreshStopNotPersisted signals that the durable fail-closed marker could
// not be written. Callers must surface this together with the original refresh
// failure because another process otherwise has no way to observe the stop.
var ErrRefreshStopNotPersisted = errors.New("oauth refresh stop marker could not be persisted")

// RefreshStopPath returns the credential-scoped durable stop marker. Keeping it
// beside the canonical credentials file prevents independent credential sets
// from stopping or re-arming one another.
func (r OAuthResolver) RefreshStopPath() string {
	path := canonicalRefreshCredentialsPath(r.credentialsPath())
	if path == "" {
		return ""
	}
	return path + ".refresh-stop"
}

// RefreshStopped reports whether automatic refresh is durably stopped.
func (r OAuthResolver) RefreshStopped() (bool, error) {
	path := r.RefreshStopPath()
	if path == "" {
		return false, errors.New("cannot resolve refresh stop path")
	}
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect refresh stop marker %s: %w", path, err)
	}
	return true, nil
}

// WriteRefreshStop prevents every resolver entry point using these credentials
// from presenting the refresh token again until an operator clears the marker.
func (r OAuthResolver) WriteRefreshStop(reason string) error {
	path := r.RefreshStopPath()
	if path == "" {
		return errors.New("cannot resolve refresh stop path")
	}
	if reason == "" {
		reason = ErrRefreshStopped.Error()
	}
	return os.WriteFile(path, []byte(reason+"\n"), 0o600)
}

// ClearRefreshStop explicitly re-arms refreshing for this credential set.
func (r OAuthResolver) ClearRefreshStop() error {
	path := r.RefreshStopPath()
	if path == "" {
		return errors.New("cannot resolve refresh stop path")
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func canonicalRefreshCredentialsPath(path string) string {
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return path
}
