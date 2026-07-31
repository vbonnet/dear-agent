// Package systemdaudit transactionally installs the Linux override-audit
// executable and system unit templates.
package systemdaudit

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
	"strings"
	"syscall"
	"time"
)

const (
	auditLive   = "/usr/local/libexec/dear-agent-override-audit"
	serviceLive = "/etc/systemd/system/dear-agent-override-audit@.service"
	timerLive   = "/etc/systemd/system/dear-agent-override-audit@.timer"
)

// Config binds the exact reviewed artifacts and digests to the fixed live
// destinations managed by the installer.
type Config struct {
	rootUID int
	rootGID int

	auditArtifact   string
	serviceArtifact string
	timerArtifact   string

	expectedAuditHash   string
	expectedServiceHash string
	expectedTimerHash   string

	auditLive   string
	serviceLive string
	timerLive   string
	trustRoot   string

	reload func(context.Context) error
}

// NewConfig validates the operator-approved inputs and returns the fixed
// production installation configuration.
func NewConfig(
	rootGID int,
	auditArtifact, serviceArtifact, timerArtifact string,
	expectedAuditHash, expectedServiceHash, expectedTimerHash string,
) (Config, error) {
	config := Config{
		rootUID:             0,
		rootGID:             rootGID,
		auditArtifact:       auditArtifact,
		serviceArtifact:     serviceArtifact,
		timerArtifact:       timerArtifact,
		expectedAuditHash:   expectedAuditHash,
		expectedServiceHash: expectedServiceHash,
		expectedTimerHash:   expectedTimerHash,
		auditLive:           auditLive,
		serviceLive:         serviceLive,
		timerLive:           timerLive,
		trustRoot:           "/",
		reload: func(ctx context.Context) error {
			output, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "daemon-reload").CombinedOutput()
			if err != nil {
				return fmt.Errorf("reload systemd manager: %w: %s", err, output)
			}
			return nil
		},
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
		"audit artifact": config.auditArtifact, "service artifact": config.serviceArtifact,
		"timer artifact": config.timerArtifact, "trust root": config.trustRoot,
	} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("%s must be a clean absolute path", name)
		}
	}
	for name, digest := range map[string]string{
		"audit digest": config.expectedAuditHash, "service digest": config.expectedServiceHash,
		"timer digest": config.expectedTimerHash,
	} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("%s must be a SHA-256 value", name)
		}
	}
	if config.reload == nil {
		return errors.New("systemd reload function is required")
	}
	return nil
}

type transaction struct {
	config Config

	auditStaging   string
	serviceStaging string
	timerStaging   string

	auditBackup   string
	serviceBackup string
	timerBackup   string

	auditExisted   bool
	serviceExisted bool
	timerExisted   bool

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
	if err := tx.stageAndVerify(); err != nil {
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
	if err := tx.config.reload(ctx); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	tx.activationComplete = true
	return nil
}

func (tx *transaction) prepareDestination() error {
	directory := filepath.Dir(tx.config.auditLive)
	if err := ensureTrustedDirectory(
		directory, tx.config.trustRoot, tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("prepare audit executable directory: %w", err)
	}
	if err := verifyTrustedAncestry(
		filepath.Dir(tx.config.serviceLive), tx.config.trustRoot, tx.config.rootUID,
	); err != nil {
		return fmt.Errorf("verify systemd unit ancestry: %w", err)
	}
	return nil
}

func ensureTrustedDirectory(path, boundary string, ownerUID, ownerGID int) error {
	if err := verifyTrustedAncestry(filepath.Dir(path), boundary, ownerUID); err != nil {
		return fmt.Errorf("verify parent ancestry: %w", err)
	}
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s must be a real directory", path)
		}
	case os.IsNotExist(err):
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
		if err := os.Chmod(path, 0o755); err != nil { //nolint:gosec // system executable directory
			return fmt.Errorf("set directory mode: %w", err)
		}
		if err := os.Chown(path, ownerUID, ownerGID); err != nil {
			return fmt.Errorf("set directory owner: %w", err)
		}
	default:
		return fmt.Errorf("inspect directory: %w", err)
	}
	return verifyTrustedAncestry(path, boundary, ownerUID)
}

func (tx *transaction) stageAndVerify() error {
	var err error
	if tx.auditStaging, err = stageArtifact(
		tx.config.auditArtifact, filepath.Dir(tx.config.auditLive),
		".dear-agent-override-audit.", 0o755, tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("stage audit executable: %w", err)
	}
	if tx.serviceStaging, err = stageArtifact(
		tx.config.serviceArtifact, filepath.Dir(tx.config.serviceLive),
		".dear-agent-override-audit-service.", 0o644, tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("stage systemd service: %w", err)
	}
	if tx.timerStaging, err = stageArtifact(
		tx.config.timerArtifact, filepath.Dir(tx.config.timerLive),
		".dear-agent-override-audit-timer.", 0o644, tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("stage systemd timer: %w", err)
	}
	if err := verifyDigest(tx.auditStaging, tx.config.expectedAuditHash); err != nil {
		return fmt.Errorf("verify staged audit executable: %w", err)
	}
	if err := verifyDigest(tx.serviceStaging, tx.config.expectedServiceHash); err != nil {
		return fmt.Errorf("verify staged systemd service: %w", err)
	}
	if err := verifyDigest(tx.timerStaging, tx.config.expectedTimerHash); err != nil {
		return fmt.Errorf("verify staged systemd timer: %w", err)
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
	if tx.serviceBackup, tx.serviceExisted, err = backupArtifact(
		tx.config.serviceLive, ".dear-agent-override-audit-service.backup.",
		tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("back up systemd service: %w", err)
	}
	if tx.timerBackup, tx.timerExisted, err = backupArtifact(
		tx.config.timerLive, ".dear-agent-override-audit-timer.backup.",
		tx.config.rootUID, tx.config.rootGID,
	); err != nil {
		return fmt.Errorf("back up systemd timer: %w", err)
	}
	return nil
}

func (tx *transaction) activateSet(ctx context.Context) error {
	tx.activationStarted = true
	if err := tx.activate(ctx, &tx.auditStaging, tx.config.auditLive); err != nil {
		return fmt.Errorf("activate audit executable: %w", err)
	}
	if err := tx.activate(ctx, &tx.serviceStaging, tx.config.serviceLive); err != nil {
		return fmt.Errorf("activate systemd service: %w", err)
	}
	if err := tx.activate(ctx, &tx.timerStaging, tx.config.timerLive); err != nil {
		return fmt.Errorf("activate systemd timer: %w", err)
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
			restoreArtifact(tx.config.serviceLive, &tx.serviceBackup, tx.serviceExisted),
			restoreArtifact(tx.config.timerLive, &tx.timerBackup, tx.timerExisted),
		)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		rollbackErrors = append(rollbackErrors, tx.config.reload(ctx))
		cancel()
	}
	// If activation started, preserve any backup whose restore failed for
	// explicit operator recovery instead of deleting the last good bytes.
	rollbackErrors = append(rollbackErrors, tx.cleanup(!tx.activationStarted))
	return errors.Join(rollbackErrors...)
}

func (tx *transaction) cleanup(removeBackups bool) error {
	cleanupErrors := []error{
		removeIfPresent(tx.auditStaging), removeIfPresent(tx.serviceStaging),
		removeIfPresent(tx.timerStaging),
	}
	if removeBackups {
		cleanupErrors = append(cleanupErrors,
			removeIfPresent(tx.auditBackup), removeIfPresent(tx.serviceBackup),
			removeIfPresent(tx.timerBackup),
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

func verifyTrustedAncestry(path, boundary string, ownerUID int) error {
	path = filepath.Clean(path)
	boundary = filepath.Clean(boundary)
	relative, err := filepath.Rel(boundary, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside trusted boundary %s", path, boundary)
	}
	current := boundary
	components := []string{boundary}
	if relative != "." {
		for part := range strings.SplitSeq(relative, string(filepath.Separator)) {
			current = filepath.Join(current, part)
			components = append(components, current)
		}
	}
	for _, component := range components {
		info, err := os.Lstat(component)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", component, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s must be a real directory", component)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != ownerUID {
			return fmt.Errorf("%s must be owned by uid %d", component, ownerUID)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s must not be group- or world-writable", component)
		}
	}
	return nil
}
