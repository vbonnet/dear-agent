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

// HelperDeploymentStatus is one coherent read-only audit of both helper
// identities required by cooperative and unattended callers.
type HelperDeploymentStatus struct {
	Status           string               `json:"status"`
	Stable           DeployedHelperStatus `json:"stable"`
	ContentAddressed DeployedHelperStatus `json:"content_addressed"`
}

var (
	hashDeployedHelperFile              = readerSHA256
	afterStableHelperInspection         = func() {}
	beforeFinalStableHelperRevalidation = func() {}
)

// ProductionHelperTrustPolicy requires UID 0 ownership through filesystem root.
func ProductionHelperTrustPolicy() HelperTrustPolicy {
	return HelperTrustPolicy{OwnerUID: 0, TrustedRoot: string(filepath.Separator)}
}

// ContentAddressedHelperPath returns the immutable deployment path for one
// exact helper digest. The stable path remains available to cooperative
// callers, while revision-bound launchers execute this no-clobber identity.
func ContentAddressedHelperPath(deployed, expectedSHA256 string) (string, error) {
	deployed, err := cleanAbsolutePath(deployed, "deployed helper")
	if err != nil {
		return "", err
	}
	if err := validateExpectedHelperSHA256(expectedSHA256); err != nil {
		return "", err
	}
	return deployed + "." + expectedSHA256, nil
}

// VerifyContentAddressedHelperInvocation admits execution only through the
// exact digest-derived path. The privileged installer never replaces that
// path, so a process selected before a later stable-path activation remains
// bound to the bytes that were verified for its session.
func VerifyContentAddressedHelperInvocation(runningExecutable, deployed, expectedSHA256 string, policy HelperTrustPolicy) error {
	pinned, err := ContentAddressedHelperPath(deployed, expectedSHA256)
	if err != nil {
		return err
	}
	runningExecutable, err = cleanAbsolutePath(runningExecutable, "running helper executable")
	if err != nil {
		return err
	}
	if runningExecutable != pinned {
		return errors.New("running helper executable is not the revision-bound content-addressed path")
	}
	verified, err := verifyDeployedHelperDigestIdentity(pinned, expectedSHA256, policy)
	if err != nil {
		return err
	}
	// Everything above authenticates the file at the pinned pathname. exec
	// mapped its image before this process could look at it, so on a platform
	// that exposes the running image, bind the two: otherwise an atomic
	// replacement landing between exec and this read would let an older image
	// continue while the new, expected bytes are the ones being hashed. Where
	// no such handle exists the content-addressed pathname is the binding, and
	// the residual is recorded in internal/hookparity/SPEC.md.
	running, available, err := runningImageIdentity()
	if err != nil {
		return err
	}
	if available && !os.SameFile(running, verified) {
		return errors.New("running helper image is not the authenticated revision-bound artifact")
	}
	return nil
}

// VerifyDeployedHelperDigest admits one deployed helper only when its trusted
// filesystem identity remains stable while the exact expected bytes are read.
// It is intended for launch-time consumers that already carry a revision-bound
// expected artifact digest and therefore do not have a local artifact path to
// compare through InspectDeployedHelper.
func VerifyDeployedHelperDigest(deployed, expectedSHA256 string, policy HelperTrustPolicy) error {
	_, err := verifyDeployedHelperDigestIdentity(deployed, expectedSHA256, policy)
	return err
}

// verifyDeployedHelperDigestIdentity is VerifyDeployedHelperDigest plus the
// authenticated leaf identity, so a caller that must also bind the running
// image can compare against the exact file whose bytes were hashed.
func verifyDeployedHelperDigestIdentity(deployed, expectedSHA256 string, policy HelperTrustPolicy) (os.FileInfo, error) {
	deployed, err := cleanAbsolutePath(deployed, "deployed helper")
	if err != nil {
		return nil, err
	}
	trustedRoot, err := cleanAbsolutePath(policy.TrustedRoot, "trusted root")
	if err != nil {
		return nil, err
	}
	if err := validateExpectedHelperSHA256(expectedSHA256); err != nil {
		return nil, err
	}

	// #nosec G703 -- deployed is a validated clean absolute path; this read-only
	// leaf inspection is followed by complete trusted-root ancestry validation.
	info, err := os.Lstat(deployed)
	if err != nil {
		if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
			return nil, ancestryErr
		} else if reason != "" {
			return nil, errors.New(reason)
		}
		return nil, fmt.Errorf("inspect deployed helper: %w", err)
	}
	if reason := validateHelperLeaf(info, policy.OwnerUID); reason != "" {
		return nil, errors.New(reason)
	}
	if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
		return nil, ancestryErr
	} else if reason != "" {
		return nil, errors.New(reason)
	}
	actualSHA256, err := verifiedFileSHA256(deployed, info, policy.OwnerUID)
	if err != nil {
		return nil, err
	}
	if actualSHA256 != expectedSHA256 {
		return nil, errors.New("deployed helper digest does not match the revision-bound expected artifact")
	}
	return info, nil
}

// InspectHelperDeployment performs one coherent audit of the stable cooperative
// identity and the digest-derived content-addressed identity. It pins and
// hashes the expected artifact once, uses that digest for both leaf reads, and
// revalidates every admitted pathname before reporting aggregate current.
func InspectHelperDeployment(artifact, stablePath string, policy HelperTrustPolicy) (HelperDeploymentStatus, error) {
	artifactSnapshot, err := openHelperArtifactSnapshot(artifact)
	if err != nil {
		return HelperDeploymentStatus{}, err
	}
	defer artifactSnapshot.file.Close()

	stable, stableIdentity, err := inspectDeployedHelperWithDigest(
		artifactSnapshot.path, stablePath, artifactSnapshot.digest, policy,
	)
	if err != nil {
		return HelperDeploymentStatus{}, fmt.Errorf("inspect stable helper: %w", err)
	}
	afterStableHelperInspection()
	pinnedPath, err := ContentAddressedHelperPath(stablePath, artifactSnapshot.digest)
	if err != nil {
		return HelperDeploymentStatus{}, fmt.Errorf("derive content-addressed helper: %w", err)
	}
	pinned, pinnedIdentity, err := inspectDeployedHelperWithDigest(
		artifactSnapshot.path, pinnedPath, artifactSnapshot.digest, policy,
	)
	if err != nil {
		return HelperDeploymentStatus{}, fmt.Errorf("inspect content-addressed helper: %w", err)
	}

	if err := artifactSnapshot.revalidate(); err != nil {
		return HelperDeploymentStatus{}, err
	}
	trustedRoot, err := cleanAbsolutePath(policy.TrustedRoot, "trusted root")
	if err != nil {
		return HelperDeploymentStatus{}, err
	}
	if stable.Status == HelperCurrent && pinned.Status == HelperCurrent {
		pinnedIdentity, err = revalidateAdmittedHelper(pinnedPath, pinnedIdentity, trustedRoot, policy.OwnerUID)
		if err != nil {
			pinned.Status = HelperUntrusted
			pinned.Reason = err.Error()
		}
		beforeFinalStableHelperRevalidation()
		stableIdentity, err = revalidateAdmittedHelper(stablePath, stableIdentity, trustedRoot, policy.OwnerUID)
		if err != nil {
			stable.Status = HelperUntrusted
			stable.Reason = err.Error()
		}
		if stable.Status == HelperCurrent && pinned.Status == HelperCurrent &&
			!os.SameFile(stableIdentity, pinnedIdentity) {
			pinned.Status = HelperUntrusted
			pinned.Reason = "stable and content-addressed helpers are not hard links to the same deployed identity"
		}
	}
	aggregate, err := aggregateHelperStatus(stable.Status, pinned.Status)
	if err != nil {
		return HelperDeploymentStatus{}, err
	}
	return HelperDeploymentStatus{
		Status:           aggregate,
		Stable:           stable,
		ContentAddressed: pinned,
	}, nil
}

// InspectDeployedHelper verifies content identity plus the complete ownership
// and mode chain from one deployed leaf through policy.TrustedRoot.
func InspectDeployedHelper(artifact, deployed string, policy HelperTrustPolicy) (DeployedHelperStatus, error) {
	artifact, err := filepath.Abs(artifact)
	if err != nil {
		return DeployedHelperStatus{}, fmt.Errorf("resolve helper artifact: %w", err)
	}
	artifact = filepath.Clean(artifact)
	expectedSHA256, err := fileSHA256(artifact)
	if err != nil {
		return DeployedHelperStatus{}, fmt.Errorf("hash helper artifact: %w", err)
	}
	status, _, err := inspectDeployedHelperWithDigest(artifact, deployed, expectedSHA256, policy)
	return status, err
}

func inspectDeployedHelperWithDigest(artifact, deployed, expectedSHA256 string, policy HelperTrustPolicy) (DeployedHelperStatus, os.FileInfo, error) {
	deployed, err := cleanAbsolutePath(deployed, "deployed helper")
	if err != nil {
		return DeployedHelperStatus{}, nil, err
	}
	trustedRoot, err := cleanAbsolutePath(policy.TrustedRoot, "trusted root")
	if err != nil {
		return DeployedHelperStatus{}, nil, err
	}
	if err := validateExpectedHelperSHA256(expectedSHA256); err != nil {
		return DeployedHelperStatus{}, nil, err
	}
	status := DeployedHelperStatus{
		Artifact:       artifact,
		Deployed:       deployed,
		ExpectedSHA256: expectedSHA256,
	}

	info, err := os.Lstat(deployed)
	if os.IsNotExist(err) {
		if reason, ancestryErr := validateExistingTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
			status.Status = HelperUntrusted
			status.Reason = ancestryErr.Error()
			//nolint:nilerr // Unsafe ancestry is a reportable status, not a transport failure.
			return status, nil, nil
		} else if reason != "" {
			status.Status = HelperUntrusted
			status.Reason = reason
			return status, nil, nil
		}
		status.Status = HelperMissing
		status.Reason = "deployed helper is missing"
		return status, nil, nil
	}
	if err != nil {
		if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
			return DeployedHelperStatus{}, nil, ancestryErr
		} else if reason != "" {
			status.Status = HelperUntrusted
			status.Reason = reason
			return status, nil, nil
		}
		return DeployedHelperStatus{}, nil, fmt.Errorf("inspect deployed helper: %w", err)
	}
	if reason := validateHelperLeaf(info, policy.OwnerUID); reason != "" {
		status.Status = HelperUntrusted
		status.Reason = reason
		return status, nil, nil
	}
	if reason, ancestryErr := validateTrustedAncestry(filepath.Dir(deployed), trustedRoot, policy.OwnerUID); ancestryErr != nil {
		return DeployedHelperStatus{}, nil, ancestryErr
	} else if reason != "" {
		status.Status = HelperUntrusted
		status.Reason = reason
		return status, nil, nil
	}

	status.ActualSHA256, err = verifiedFileSHA256(deployed, info, policy.OwnerUID)
	if err != nil {
		status.Status = HelperUntrusted
		status.Reason = err.Error()
		// The inspection completed: a leaf that changed while being read is a
		// reportable untrusted deployment state, not a status-command failure.
		//nolint:nilerr // An unsafe deployment is a successful read-only status result.
		return status, nil, nil
	}
	if status.ActualSHA256 != status.ExpectedSHA256 {
		status.Status = HelperStale
		status.Reason = "deployed helper digest does not match the built artifact"
		return status, nil, nil
	}
	status.Status = HelperCurrent
	return status, info, nil
}

func cleanAbsolutePath(path, label string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", fmt.Errorf("%s must be a clean absolute path", label)
	}
	return path, nil
}

func validateExpectedHelperSHA256(expectedSHA256 string) error {
	decoded, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(decoded) != sha256.Size || expectedSHA256 != strings.ToLower(expectedSHA256) {
		return errors.New("expected helper SHA-256 must be 64 lowercase hexadecimal characters")
	}
	return nil
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
	return validateTrustedAncestryAllowMissing(start, root, ownerUID, false)
}

func validateExistingTrustedAncestry(start, root string, ownerUID uint32) (string, error) {
	return validateTrustedAncestryAllowMissing(start, root, ownerUID, true)
}

func validateTrustedAncestryAllowMissing(start, root string, ownerUID uint32, allowMissing bool) (string, error) {
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
		reason, missing, err := inspectTrustedAncestor(current, ownerUID, allowMissing)
		if err != nil || reason != "" || missing {
			return reason, err
		}
	}
	return "", nil
}

func inspectTrustedAncestor(current string, ownerUID uint32, allowMissing bool) (reason string, missing bool, err error) {
	// #nosec G703 -- current is built from a clean absolute path already proven
	// to be within the clean absolute trusted root; Lstat rejects symlink hops.
	info, statErr := os.Lstat(current)
	if statErr != nil {
		if allowMissing && errors.Is(statErr, os.ErrNotExist) {
			return "", true, nil
		}
		return "", false, fmt.Errorf("inspect deployed helper ancestor %s: %w", current, statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Sprintf("deployed helper ancestor %s is not a non-symlink directory", current), false, nil
	}
	uid, ownerErr := fileOwnerUID(info)
	if ownerErr != nil {
		return "", false, fmt.Errorf("inspect deployed helper ancestor owner %s: %w", current, ownerErr)
	}
	if uid != ownerUID {
		return fmt.Sprintf("deployed helper ancestor %s owner UID is %d, want %d", current, uid, ownerUID), false, nil
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Sprintf("deployed helper ancestor %s is group- or world-writable", current), false, nil
	}
	if info.Mode().Perm()&0o001 == 0 {
		return fmt.Sprintf("deployed helper ancestor %s is not searchable by unprivileged launchers", current), false, nil
	}
	return "", false, nil
}

type helperArtifactSnapshot struct {
	path   string
	file   *os.File
	info   os.FileInfo
	digest string
}

func openHelperArtifactSnapshot(path string) (*helperArtifactSnapshot, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve helper artifact: %w", err)
	}
	path = filepath.Clean(path)
	pathBefore, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect helper artifact: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open helper artifact: %w", err)
	}
	fail := func(err error) (*helperArtifactSnapshot, error) {
		_ = file.Close()
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		return fail(fmt.Errorf("inspect opened helper artifact: %w", err))
	}
	if !sameArtifactSnapshot(pathBefore, opened) {
		return fail(fmt.Errorf("helper artifact changed during composite inspection"))
	}
	digest, err := readerSHA256(file)
	if err != nil {
		return fail(fmt.Errorf("hash helper artifact: %w", err))
	}
	openedAfterHash, err := file.Stat()
	if err != nil {
		return fail(fmt.Errorf("reinspect opened helper artifact: %w", err))
	}
	pathAfterHash, err := os.Stat(path)
	if err != nil {
		return fail(fmt.Errorf("reinspect helper artifact path: %w", err))
	}
	if !sameArtifactSnapshot(pathBefore, openedAfterHash) ||
		!sameArtifactSnapshot(opened, openedAfterHash) ||
		!sameArtifactSnapshot(openedAfterHash, pathAfterHash) {
		return fail(fmt.Errorf("helper artifact changed during composite inspection"))
	}
	return &helperArtifactSnapshot{path: path, file: file, info: openedAfterHash, digest: digest}, nil
}

func (snapshot *helperArtifactSnapshot) revalidate() error {
	if _, err := snapshot.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind helper artifact: %w", err)
	}
	digest, err := readerSHA256(snapshot.file)
	if err != nil {
		return fmt.Errorf("rehash helper artifact: %w", err)
	}
	opened, err := snapshot.file.Stat()
	if err != nil {
		return fmt.Errorf("reinspect opened helper artifact: %w", err)
	}
	pathInfo, err := os.Stat(snapshot.path)
	if err != nil {
		return fmt.Errorf("reinspect helper artifact path: %w", err)
	}
	if digest != snapshot.digest ||
		!sameArtifactSnapshot(snapshot.info, opened) ||
		!sameArtifactSnapshot(opened, pathInfo) {
		return fmt.Errorf("helper artifact changed during composite inspection")
	}
	return nil
}

func sameArtifactSnapshot(before, after os.FileInfo) bool {
	return os.SameFile(before, after) &&
		before.Mode() == after.Mode() &&
		before.Size() == after.Size() &&
		before.ModTime().Equal(after.ModTime())
}

func revalidateAdmittedHelper(path string, admitted os.FileInfo, trustedRoot string, ownerUID uint32) (os.FileInfo, error) {
	if admitted == nil {
		return nil, fmt.Errorf("deployed helper has no admitted identity")
	}
	if reason, err := validateTrustedAncestry(filepath.Dir(path), trustedRoot, ownerUID); err != nil {
		return nil, err
	} else if reason != "" {
		return nil, errors.New(reason)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("reinspect deployed helper path: %w", err)
	}
	if !sameHelperSnapshot(admitted, current, ownerUID) {
		return nil, fmt.Errorf("deployed helper changed during composite inspection")
	}
	return current, nil
}

func aggregateHelperStatus(statuses ...string) (string, error) {
	result := HelperCurrent
	priority := 0
	for _, status := range statuses {
		var candidatePriority int
		switch status {
		case HelperCurrent:
			candidatePriority = 0
		case HelperMissing:
			candidatePriority = 1
		case HelperStale:
			candidatePriority = 2
		case HelperUntrusted:
			candidatePriority = 3
		default:
			return "", fmt.Errorf("unexpected deployed helper status %q", status)
		}
		if candidatePriority > priority {
			result = status
			priority = candidatePriority
		}
	}
	return result, nil
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
	// #nosec G703 -- callers pass a clean absolute path whose leaf and complete trusted ancestry were validated immediately before this read.
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
	// #nosec G703 -- this is the same validated clean absolute path; the result
	// is identity-compared with the already-open trusted leaf below.
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
