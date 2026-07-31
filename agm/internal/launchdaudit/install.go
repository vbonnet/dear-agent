// Package launchdaudit transactionally installs the macOS override-audit
// executable and LaunchDaemon property list.
package launchdaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	auditLive = "/usr/local/libexec/dear-agent-override-audit"
	plistLive = "/Library/LaunchDaemons/com.dear-agent.override-audit.plist"
)

// Config binds the exact reviewed artifacts and digests to the fixed live
// destinations managed by the installer.
type Config struct {
	rootUID int
	rootGID int

	auditArtifact string
	plistArtifact string

	expectedAuditHash string
	expectedPlistHash string

	auditLive string
	plistLive string

	validatePlist func(context.Context, string) error
	finish        func(context.Context) error
}

// NewConfig validates the operator-approved inputs and returns the fixed
// production installation configuration.
func NewConfig(
	rootGID int,
	auditArtifact, plistArtifact string,
	expectedAuditHash, expectedPlistHash string,
) (Config, error) {
	config := Config{
		rootUID:           0,
		rootGID:           rootGID,
		auditArtifact:     auditArtifact,
		plistArtifact:     plistArtifact,
		expectedAuditHash: expectedAuditHash,
		expectedPlistHash: expectedPlistHash,
		auditLive:         auditLive,
		plistLive:         plistLive,
		validatePlist: func(ctx context.Context, path string) error {
			output, err := exec.CommandContext(ctx, "/usr/bin/plutil", "-lint", path).CombinedOutput()
			if err != nil {
				return fmt.Errorf("validate LaunchDaemon plist: %w: %s", err, output)
			}
			return nil
		},
		finish: checkContext,
	}
	if err := config.validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config Config) validate() error {
	if config.rootUID < 0 || config.rootGID < 0 {
		return errors.New("root user and group IDs must be non-negative")
	}
	for name, path := range map[string]string{
		"audit artifact": config.auditArtifact,
		"plist artifact": config.plistArtifact,
	} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("%s must be a clean absolute path", name)
		}
	}
	for name, digest := range map[string]string{
		"audit digest": config.expectedAuditHash,
		"plist digest": config.expectedPlistHash,
	} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("%s must be a SHA-256 value", name)
		}
	}
	if config.validatePlist == nil {
		return errors.New("LaunchDaemon plist validator is required")
	}
	if config.finish == nil {
		return errors.New("LaunchDaemon completion check is required")
	}
	return nil
}

type transaction struct {
	config Config

	auditStaging string
	plistStaging string

	auditBackup string
	plistBackup string

	auditExisted bool
	plistExisted bool

	activationStarted  bool
	activationComplete bool
}

// Install stages, verifies, and activates the configured artifact set. A
// cancellation or failure before completion restores the complete prior set.
func Install(ctx context.Context, config Config) error {
	if err := config.validate(); err != nil {
		return err
	}
	tx := &transaction{config: config}
	if err := tx.run(ctx); err != nil {
		return errors.Join(err, tx.rollback())
	}
	return tx.cleanup(true)
}

func (tx *transaction) run(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := tx.prepareDestination(); err != nil {
		return err
	}
	if err := tx.stageAndVerify(ctx); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := tx.backupLiveSet(); err != nil {
		return err
	}
	if err := tx.activateSet(ctx); err != nil {
		return err
	}
	if err := tx.config.finish(ctx); err != nil {
		return err
	}
	tx.activationComplete = true
	return nil
}

func (tx *transaction) prepareDestination() error {
	directory := filepath.Dir(tx.config.auditLive)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create audit executable directory: %w", err)
	}
	// This root-owned executable directory is intentionally searchable by unprivileged services.
	if err := os.Chmod(directory, 0o755); err != nil { //nolint:gosec // system executable directory
		return fmt.Errorf("set audit executable directory mode: %w", err)
	}
	if err := os.Chown(directory, tx.config.rootUID, tx.config.rootGID); err != nil {
		return fmt.Errorf("set audit executable directory owner: %w", err)
	}
	return nil
}

func (tx *transaction) stageAndVerify(ctx context.Context) error {
	var err error
	if tx.auditStaging, err = stageArtifact(
		tx.config.auditArtifact, filepath.Dir(tx.config.auditLive),
		".dear-agent-override-audit.", 0o755, tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("stage audit executable: %w", err)
	}
	if tx.plistStaging, err = stageArtifact(
		tx.config.plistArtifact, filepath.Dir(tx.config.plistLive),
		".com.dear-agent.override-audit.", 0o644, tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("stage LaunchDaemon plist: %w", err)
	}
	if err := verifyDigest(tx.auditStaging, tx.config.expectedAuditHash); err != nil {
		return fmt.Errorf("verify staged audit executable: %w", err)
	}
	if err := verifyDigest(tx.plistStaging, tx.config.expectedPlistHash); err != nil {
		return fmt.Errorf("verify staged LaunchDaemon plist: %w", err)
	}
	if err := tx.config.validatePlist(ctx, tx.plistStaging); err != nil {
		return err
	}
	return nil
}

func (tx *transaction) backupLiveSet() error {
	var err error
	if tx.auditBackup, tx.auditExisted, err = backupArtifact(
		tx.config.auditLive, ".dear-agent-override-audit.backup.",
		tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("back up audit executable: %w", err)
	}
	if tx.plistBackup, tx.plistExisted, err = backupArtifact(
		tx.config.plistLive, ".com.dear-agent.override-audit.backup.",
		tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("back up LaunchDaemon plist: %w", err)
	}
	return nil
}

func (tx *transaction) activateSet(ctx context.Context) error {
	tx.activationStarted = true
	if err := tx.activate(ctx, &tx.auditStaging, tx.config.auditLive); err != nil {
		return fmt.Errorf("activate audit executable: %w", err)
	}
	if err := tx.activate(ctx, &tx.plistStaging, tx.config.plistLive); err != nil {
		return fmt.Errorf("activate LaunchDaemon plist: %w", err)
	}
	return nil
}

func (tx *transaction) activate(ctx context.Context, staging *string, live string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := os.Rename(*staging, live); err != nil {
		return err
	}
	*staging = ""
	return checkContext(ctx)
}

func (tx *transaction) rollback() error {
	var rollbackErrors []error
	if tx.activationStarted && !tx.activationComplete {
		rollbackErrors = append(rollbackErrors,
			restoreArtifact(tx.config.auditLive, &tx.auditBackup, tx.auditExisted),
			restoreArtifact(tx.config.plistLive, &tx.plistBackup, tx.plistExisted),
		)
	}
	// If activation started, preserve any backup whose restore failed for
	// explicit operator recovery instead of deleting the last good bytes.
	rollbackErrors = append(rollbackErrors, tx.cleanup(!tx.activationStarted))
	return errors.Join(rollbackErrors...)
}

func (tx *transaction) cleanup(removeBackups bool) error {
	cleanupErrors := []error{
		removeIfPresent(tx.auditStaging),
		removeIfPresent(tx.plistStaging),
	}
	if removeBackups {
		cleanupErrors = append(cleanupErrors,
			removeIfPresent(tx.auditBackup),
			removeIfPresent(tx.plistBackup),
		)
	}
	return errors.Join(cleanupErrors...)
}

func stageArtifact(
	source, directory, pattern string,
	mode os.FileMode,
	uid, gid int,
) (string, error) {
	staging, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := staging.Name()
	if err := staging.Close(); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	if err := copyArtifact(source, path, mode, uid, gid); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	return path, nil
}

func backupArtifact(live, pattern string, uid, gid int) (string, bool, error) {
	info, err := os.Stat(live)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	backup, err := os.CreateTemp(filepath.Dir(live), pattern)
	if err != nil {
		return "", false, err
	}
	path := backup.Name()
	if err := backup.Close(); err != nil {
		return "", false, errors.Join(err, os.Remove(path))
	}
	if err := copyArtifact(live, path, info.Mode().Perm(), uid, gid); err != nil {
		return "", false, errors.Join(err, os.Remove(path))
	}
	return path, true, nil
}

func copyArtifact(source, destination string, mode os.FileMode, uid, gid int) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Chmod(destination, mode); err != nil {
		return err
	}
	return os.Chown(destination, uid, gid)
}

func verifyDigest(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != expected {
		return fmt.Errorf("digest differs: got %s, want %s", actual, expected)
	}
	return nil
}

func restoreArtifact(live string, backup *string, existed bool) error {
	if existed {
		if *backup == "" {
			return fmt.Errorf("restore %s: backup is missing", live)
		}
		if err := os.Rename(*backup, live); err != nil {
			return err
		}
		*backup = ""
		return nil
	}
	return removeIfPresent(live)
}

func removeIfPresent(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func checkContext(ctx context.Context) error {
	if err := context.Cause(ctx); err != nil {
		return fmt.Errorf("installation interrupted: %w", err)
	}
	return nil
}
