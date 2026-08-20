//go:build unix

package hookparity

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectDeployedHelperTrustAndDrift(t *testing.T) {
	setup := func(t *testing.T) (string, string, HelperTrustPolicy) {
		t.Helper()
		root := t.TempDir()
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(root, "libexec")
		if err := os.Mkdir(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		artifact := filepath.Join(t.TempDir(), "spec-contract-hook")
		deployed := filepath.Join(bin, "spec-contract-hook")
		for _, path := range []string{artifact, deployed} {
			if err := os.WriteFile(path, []byte("reviewed helper\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return artifact, deployed, HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: root}
	}

	t.Run("current", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperCurrent || status.ExpectedSHA256 == "" || status.ActualSHA256 != status.ExpectedSHA256 {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.Remove(deployed); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperMissing {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("missing leaf beneath writable ancestor", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.Remove(deployed); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Dir(deployed), 0o777); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted ||
			!strings.Contains(status.Reason, "ancestor") || !strings.Contains(status.Reason, "writable") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("missing leaf beneath symlink ancestor", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		ancestor := filepath.Dir(deployed)
		if err := os.Remove(deployed); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(ancestor); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "libexec")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, ancestor); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted ||
			!strings.Contains(status.Reason, "ancestor") || !strings.Contains(status.Reason, "non-symlink") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.WriteFile(deployed, []byte("older helper\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperStale || status.ActualSHA256 == status.ExpectedSHA256 {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		policy.OwnerUID++
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "owner UID") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("wrong mode", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.Chmod(deployed, 0o644); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "not executable") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("owner-only executable leaf", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.Chmod(deployed, 0o744); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "not executable by unprivileged launchers") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("unreadable leaf", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.Chmod(deployed, 0o711); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "not readable by unprivileged launchers") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("writable leaf", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.Chmod(deployed, 0o775); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "writable") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("writable ancestor", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		if err := os.Chmod(filepath.Dir(deployed), 0o777); err != nil {
			t.Fatal(err)
		}
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "ancestor") || !strings.Contains(status.Reason, "writable") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("unsearchable ancestor", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		ancestor := filepath.Dir(deployed)
		if err := os.Chmod(ancestor, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(ancestor, 0o755) })
		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "ancestor") || !strings.Contains(status.Reason, "not searchable by unprivileged launchers") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})

	t.Run("atomic replacement after hash", func(t *testing.T) {
		artifact, deployed, policy := setup(t)
		replacement := filepath.Join(filepath.Dir(deployed), "replacement")
		if err := os.WriteFile(replacement, []byte("replacement helper\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		originalHasher := hashDeployedHelperFile
		hashDeployedHelperFile = func(reader io.Reader) (string, error) {
			digest, err := originalHasher(reader)
			if err != nil {
				return "", err
			}
			if err := os.Rename(replacement, deployed); err != nil {
				return "", err
			}
			return digest, nil
		}
		t.Cleanup(func() { hashDeployedHelperFile = originalHasher })

		status, err := InspectDeployedHelper(artifact, deployed, policy)
		if err != nil || status.Status != HelperUntrusted || !strings.Contains(status.Reason, "changed during validation") {
			t.Fatalf("InspectDeployedHelper() = %#v, %v", status, err)
		}
	})
}

func TestInspectHelperDeploymentRequiresOneStableSnapshot(t *testing.T) {
	t.Run("both hard-linked identities current", func(t *testing.T) {
		artifact, stable, pinned, policy := setupHelperDeployment(t, true)
		status, err := InspectHelperDeployment(artifact, stable, policy)
		if err != nil || status.Status != HelperCurrent ||
			status.Stable.Status != HelperCurrent ||
			status.ContentAddressed.Status != HelperCurrent ||
			status.ContentAddressed.Deployed != pinned {
			t.Fatalf("InspectHelperDeployment() = %#v, %v", status, err)
		}
	})

	t.Run("same bytes on distinct inodes are untrusted", func(t *testing.T) {
		artifact, stable, _, policy := setupHelperDeployment(t, false)
		status, err := InspectHelperDeployment(artifact, stable, policy)
		if err != nil || status.Status != HelperUntrusted ||
			status.Stable.Status != HelperCurrent ||
			status.ContentAddressed.Status != HelperUntrusted ||
			!strings.Contains(status.ContentAddressed.Reason, "not hard links") {
			t.Fatalf("InspectHelperDeployment() = %#v, %v", status, err)
		}
	})

	t.Run("stable replacement after pinned and artifact checks is untrusted", func(t *testing.T) {
		artifact, stable, _, policy := setupHelperDeployment(t, true)
		replacement := filepath.Join(filepath.Dir(stable), "replacement")
		body, err := os.ReadFile(artifact)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(replacement, body, 0o755); err != nil {
			t.Fatal(err)
		}
		previous := beforeFinalStableHelperRevalidation
		beforeFinalStableHelperRevalidation = func() {
			if err := os.Rename(replacement, stable); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { beforeFinalStableHelperRevalidation = previous })

		status, err := InspectHelperDeployment(artifact, stable, policy)
		if err != nil || status.Status != HelperUntrusted ||
			status.Stable.Status != HelperUntrusted ||
			status.ContentAddressed.Status != HelperCurrent ||
			!strings.Contains(status.Stable.Reason, "changed during composite inspection") {
			t.Fatalf("InspectHelperDeployment() = %#v, %v", status, err)
		}
	})

	t.Run("artifact replacement between leaf reads is rejected", func(t *testing.T) {
		artifact, stable, _, policy := setupHelperDeployment(t, true)
		replacement := filepath.Join(filepath.Dir(artifact), "replacement")
		body, err := os.ReadFile(artifact)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(replacement, body, 0o755); err != nil {
			t.Fatal(err)
		}
		previous := afterStableHelperInspection
		afterStableHelperInspection = func() {
			if err := os.Rename(replacement, artifact); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { afterStableHelperInspection = previous })

		if _, err := InspectHelperDeployment(artifact, stable, policy); err == nil ||
			!strings.Contains(err.Error(), "artifact changed during composite inspection") {
			t.Fatalf("InspectHelperDeployment() error = %v", err)
		}
	})
}

func TestAggregateHelperStatusRejectsUnexpectedLeafState(t *testing.T) {
	if _, err := aggregateHelperStatus(HelperCurrent, "unknown"); err == nil ||
		!strings.Contains(err.Error(), "unexpected deployed helper status") {
		t.Fatalf("aggregateHelperStatus() error = %v", err)
	}
}

func setupHelperDeployment(t *testing.T, hardLink bool) (artifact, stable, pinned string, policy HelperTrustPolicy) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("reviewed helper\n")
	artifact = filepath.Join(t.TempDir(), "spec-contract-hook")
	stable = filepath.Join(root, "spec-contract-hook")
	for _, path := range []string{artifact, stable} {
		if err := os.WriteFile(path, body, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	var err error
	pinned, err = ContentAddressedHelperPath(stable, digest)
	if err != nil {
		t.Fatal(err)
	}
	if hardLink {
		err = os.Link(stable, pinned)
	} else {
		err = os.WriteFile(pinned, body, 0o755)
	}
	if err != nil {
		t.Fatal(err)
	}
	policy = HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: root}
	return artifact, stable, pinned, policy
}

func TestValidateHelperLeafRejectsPrivilegedModeBits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte("reviewed helper\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	for name, special := range map[string]os.FileMode{
		"setuid": os.ModeSetuid,
		"setgid": os.ModeSetgid,
		"sticky": os.ModeSticky,
	} {
		t.Run(name, func(t *testing.T) {
			withSpecialMode := fileInfoWithMode{FileInfo: info, mode: info.Mode() | special}
			if reason := validateHelperLeaf(withSpecialMode, uint32(os.Getuid())); !strings.Contains(reason, "privileged or special mode bits") {
				t.Fatalf("validateHelperLeaf() reason = %q", reason)
			}
		})
	}
}

type fileInfoWithMode struct {
	os.FileInfo
	mode os.FileMode
}

func (info fileInfoWithMode) Mode() os.FileMode { return info.mode }

func TestVerifyDeployedHelperDigest(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	deployed := filepath.Join(root, "helper")
	body := []byte("reviewed helper\n")
	if err := os.WriteFile(deployed, body, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	policy := HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: root}
	if err := VerifyDeployedHelperDigest(deployed, digest, policy); err != nil {
		t.Fatalf("VerifyDeployedHelperDigest() error: %v", err)
	}
	if err := VerifyDeployedHelperDigest(deployed, strings.Repeat("0", sha256.Size*2), policy); err == nil || !strings.Contains(err.Error(), "revision-bound") {
		t.Fatalf("VerifyDeployedHelperDigest() mismatch error = %v", err)
	}
	if err := VerifyDeployedHelperDigest(deployed, "not-a-digest", policy); err == nil || !strings.Contains(err.Error(), "64 lowercase") {
		t.Fatalf("VerifyDeployedHelperDigest() malformed error = %v", err)
	}
	if err := os.Chmod(deployed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDeployedHelperDigest(deployed, digest, policy); err == nil || !strings.Contains(err.Error(), "unprivileged launchers") {
		t.Fatalf("VerifyDeployedHelperDigest() inaccessible error = %v", err)
	}
}

func TestContentAddressedHelperPathPinsOldInodeAcrossStableReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, "helper")
	body := []byte("reviewed helper\n")
	if err := os.WriteFile(stable, body, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	pinned, err := ContentAddressedHelperPath(stable, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Link(stable, pinned); err != nil {
		t.Fatal(err)
	}
	pinnedBefore, err := os.Stat(pinned)
	if err != nil {
		t.Fatal(err)
	}

	replacement := filepath.Join(root, "replacement")
	if err := os.WriteFile(replacement, []byte("new revision\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, stable); err != nil {
		t.Fatal(err)
	}
	pinnedAfter, err := os.Stat(pinned)
	if err != nil {
		t.Fatal(err)
	}
	stableAfter, err := os.Stat(stable)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(pinnedBefore, pinnedAfter) || os.SameFile(pinnedAfter, stableAfter) {
		t.Fatalf("content-addressed identity did not retain the old helper inode")
	}
	policy := HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: root}
	if err := VerifyDeployedHelperDigest(pinned, digest, policy); err != nil {
		t.Fatalf("VerifyDeployedHelperDigest(pinned) after stable replacement: %v", err)
	}
	if err := VerifyContentAddressedHelperInvocation(pinned, stable, digest, policy); err != nil {
		t.Fatalf("VerifyContentAddressedHelperInvocation(pinned) after stable replacement: %v", err)
	}
	if err := VerifyContentAddressedHelperInvocation(stable, stable, digest, policy); err == nil ||
		!strings.Contains(err.Error(), "content-addressed path") {
		t.Fatalf("VerifyContentAddressedHelperInvocation(stable) error = %v", err)
	}
}

func TestContentAddressedHelperInvocationUsesActualExecPath(t *testing.T) {
	if base := os.Getenv("DEAR_AGENT_TEST_CONTENT_ADDRESSED_HELPER_BASE"); base != "" {
		digest := os.Getenv("DEAR_AGENT_TEST_CONTENT_ADDRESSED_HELPER_DIGEST")
		running, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		policy := HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: filepath.Dir(base)}
		if err := VerifyContentAddressedHelperInvocation(running, base, digest, policy); err != nil {
			t.Fatal(err)
		}
		return
	}

	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, "helper")
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenFile(stable, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = source.Close()
		_ = destination.Close()
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(stable)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := ContentAddressedHelperPath(stable, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Link(stable, pinned); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement")
	if err := os.WriteFile(replacement, []byte("new revision\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, stable); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(pinned, "-test.run=^TestContentAddressedHelperInvocationUsesActualExecPath$")
	command.Env = append(os.Environ(),
		"DEAR_AGENT_TEST_CONTENT_ADDRESSED_HELPER_BASE="+stable,
		"DEAR_AGENT_TEST_CONTENT_ADDRESSED_HELPER_DIGEST="+digest,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("content-addressed helper subprocess: %v\n%s", err, output)
	}
}

func TestContentAddressedHelperPathRejectsMalformedDigest(t *testing.T) {
	if _, err := ContentAddressedHelperPath("/usr/local/libexec/helper", "not-a-digest"); err == nil ||
		!strings.Contains(err.Error(), "64 lowercase") {
		t.Fatalf("ContentAddressedHelperPath() error = %v", err)
	}
}

func TestProductionHelperTrustPolicyRequiresRootOwnedAncestry(t *testing.T) {
	policy := ProductionHelperTrustPolicy()
	if policy.OwnerUID != 0 || policy.TrustedRoot != string(filepath.Separator) {
		t.Fatalf("ProductionHelperTrustPolicy() = %#v", policy)
	}
}
