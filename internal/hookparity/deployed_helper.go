package hookparity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// HelperCurrent means the deployed helper is trusted and byte-current.
	HelperCurrent = "current"
	// HelperMissing means no deployed leaf exists.
	HelperMissing = "missing"
	// HelperStale means the trusted deployed leaf has different bytes.
	HelperStale = "stale"
	// HelperUntrusted means ownership, mode, type, or ancestry was unsafe.
	HelperUntrusted = "untrusted"
)

// HelperTrustPolicy defines the immutable ownership boundary expected for a
// deployed terminal-hook helper. Production uses UID 0 and the filesystem
// root; tests can use an isolated, caller-owned directory without privilege.
type HelperTrustPolicy struct {
	OwnerUID    uint32
	TrustedRoot string
}

// DeployedHelperStatus is a read-only comparison of a built artifact with its
// deployed copy. It never creates, repairs, chmods, chowns, or replaces files.
type DeployedHelperStatus struct {
	Status         string `json:"status"`
	Artifact       string `json:"artifact"`
	Deployed       string `json:"deployed"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

var hashDeployedHelperFile = readerSHA256

// ProductionHelperTrustPolicy requires UID 0 ownership through filesystem root.
func ProductionHelperTrustPolicy() HelperTrustPolicy {
	return HelperTrustPolicy{OwnerUID: 0, TrustedRoot: string(filepath.Separator)}
}

// VerifyDeployedHelperDigest admits one deployed helper only when its trusted
// filesystem identity remains stable while the exact expected bytes are read.
// It is intended for launch-time consumers that already carry a revision-bound
// expected artifact digest and therefore do not have a local artifact path to
// compare through InspectDeployedHelper.
func VerifyDeployedHelperDigest(deployed, expectedSHA256 string, policy HelperTrustPolicy) error {
	deployed, err := cleanAbsolutePath(deployed, "deployed helper")
	if err != nil {
		return err
	}
	trustedRoot, err := cleanAbsolutePath(policy.TrustedRoot, "trusted root")
	if err != nil {
		return err
	}
	decoded, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(decoded) != sha256.Size || expectedSHA256 != strings.ToLower(expectedSHA256) {
		return errors.New("expected helper SHA-256 must be 64 lowercase hexadecimal characters")
	}

	info, err := os.Lstat(deployed)
	if err != nil {
		if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
			return ancestryErr
		} else if reason != "" {
			return errors.New(reason)
		}
		return fmt.Errorf("inspect deployed helper: %w", err)
	}
	if reason := validateHelperLeaf(info, policy.OwnerUID); reason != "" {
		return errors.New(reason)
	}
	if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
		return ancestryErr
	} else if reason != "" {
		return errors.New(reason)
	}
	actualSHA256, err := verifiedFileSHA256(deployed, info, policy.OwnerUID)
	if err != nil {
		return err
	}
	if actualSHA256 != expectedSHA256 {
		return errors.New("deployed helper digest does not match the revision-bound expected artifact")
	}
	return nil
}

// InspectDeployedHelper verifies content identity plus the complete ownership
// and mode chain from the deployed leaf through policy.TrustedRoot.
func InspectDeployedHelper(artifact, deployed string, policy HelperTrustPolicy) (DeployedHelperStatus, error) {
	artifact, err := filepath.Abs(artifact)
	if err != nil {
		return DeployedHelperStatus{}, fmt.Errorf("resolve helper artifact: %w", err)
	}
	artifact = filepath.Clean(artifact)
	deployed, err = cleanAbsolutePath(deployed, "deployed helper")
	if err != nil {
		return DeployedHelperStatus{}, err
	}
	trustedRoot, err := cleanAbsolutePath(policy.TrustedRoot, "trusted root")
	if err != nil {
		return DeployedHelperStatus{}, err
	}
	status := DeployedHelperStatus{Artifact: artifact, Deployed: deployed}
	status.ExpectedSHA256, err = fileSHA256(artifact)
	if err != nil {
		return DeployedHelperStatus{}, fmt.Errorf("hash helper artifact: %w", err)
	}

	info, err := os.Lstat(deployed)
	if os.IsNotExist(err) {
		status.Status = HelperMissing
		status.Reason = "deployed helper is missing"
		return status, nil
	}
	if err != nil {
		if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
			return DeployedHelperStatus{}, ancestryErr
		} else if reason != "" {
			status.Status = HelperUntrusted
			status.Reason = reason
			return status, nil
		}
		return DeployedHelperStatus{}, fmt.Errorf("inspect deployed helper: %w", err)
	}
	if reason := validateHelperLeaf(info, policy.OwnerUID); reason != "" {
		status.Status = HelperUntrusted
		status.Reason = reason
		return status, nil
	}
	if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
		return DeployedHelperStatus{}, ancestryErr
	} else if reason != "" {
		status.Status = HelperUntrusted
		status.Reason = reason
		return status, nil
	}

	status.ActualSHA256, err = verifiedFileSHA256(deployed, info, policy.OwnerUID)
	if err != nil {
		status.Status = HelperUntrusted
		status.Reason = err.Error()
		// The inspection completed: a leaf that changed while being read is a
		// reportable untrusted deployment state, not a status-command failure.
		//nolint:nilerr // An unsafe deployment is a successful read-only status result.
		return status, nil
	}
	if status.ActualSHA256 != status.ExpectedSHA256 {
		status.Status = HelperStale
		status.Reason = "deployed helper digest does not match the built artifact"
		return status, nil
	}
	status.Status = HelperCurrent
	return status, nil
}

func cleanAbsolutePath(path, label string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", fmt.Errorf("%s must be a clean absolute path", label)
	}
	return path, nil
}

func validateHelperLeaf(info os.FileInfo, ownerUID uint32) string {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "deployed helper is not a regular non-symlink file"
	}
	uid, err := fileOwnerUID(info)
	if err != nil {
		return "deployed helper owner is unavailable: " + err.Error()
	}
	if uid != ownerUID {
		return fmt.Sprintf("deployed helper owner UID is %d, want %d", uid, ownerUID)
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return "deployed helper has privileged or special mode bits"
	}
	if info.Mode().Perm()&0o001 == 0 {
		return "deployed helper is not executable by unprivileged launchers"
	}
	if info.Mode().Perm()&0o004 == 0 {
		return "deployed helper is not readable by unprivileged launchers"
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "deployed helper is group- or world-writable"
	}
	return ""
}

func validateTrustedAncestry(start, root string, ownerUID uint32) (string, error) {
	relative, err := filepath.Rel(root, start)
	if err != nil {
		return "", fmt.Errorf("resolve deployed helper ancestry: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "deployed helper is outside the trusted ancestry root", nil
	}
	ancestry := make([]string, 0, 8)
	for current := start; ; {
		ancestry = append(ancestry, current)
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "deployed helper ancestry did not reach the trusted root", nil
		}
		current = parent
	}
	for _, current := range slices.Backward(ancestry) {
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return "", fmt.Errorf("inspect deployed helper ancestor %s: %w", current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Sprintf("deployed helper ancestor %s is not a non-symlink directory", current), nil
		}
		uid, ownerErr := fileOwnerUID(info)
		if ownerErr != nil {
			return "", fmt.Errorf("inspect deployed helper ancestor owner %s: %w", current, ownerErr)
		}
		if uid != ownerUID {
			return fmt.Sprintf("deployed helper ancestor %s owner UID is %d, want %d", current, uid, ownerUID), nil
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Sprintf("deployed helper ancestor %s is group- or world-writable", current), nil
		}
		if info.Mode().Perm()&0o001 == 0 {
			return fmt.Sprintf("deployed helper ancestor %s is not searchable by unprivileged launchers", current), nil
		}
	}
	return "", nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return readerSHA256(file)
}

func verifiedFileSHA256(path string, expected os.FileInfo, ownerUID uint32) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open deployed helper: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect opened deployed helper: %w", err)
	}
	if !os.SameFile(expected, opened) || validateHelperLeaf(opened, ownerUID) != "" {
		return "", fmt.Errorf("deployed helper changed during validation")
	}
	digest, err := hashDeployedHelperFile(file)
	if err != nil {
		return "", fmt.Errorf("hash deployed helper: %w", err)
	}
	openedAfterHash, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("reinspect opened deployed helper: %w", err)
	}
	pathAfterHash, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("reinspect deployed helper path: %w", err)
	}
	if !sameHelperSnapshot(expected, openedAfterHash, ownerUID) ||
		!sameHelperSnapshot(opened, openedAfterHash, ownerUID) ||
		!sameHelperSnapshot(openedAfterHash, pathAfterHash, ownerUID) {
		return "", fmt.Errorf("deployed helper changed during validation")
	}
	return digest, nil
}

func sameHelperSnapshot(before, after os.FileInfo, ownerUID uint32) bool {
	return os.SameFile(before, after) &&
		before.Mode() == after.Mode() &&
		before.Size() == after.Size() &&
		before.ModTime().Equal(after.ModTime()) &&
		validateHelperLeaf(after, ownerUID) == ""
}

func readerSHA256(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
