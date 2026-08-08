//go:build unix

package hookparity

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
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

func TestProductionHelperTrustPolicyRequiresRootOwnedAncestry(t *testing.T) {
	policy := ProductionHelperTrustPolicy()
	if policy.OwnerUID != 0 || policy.TrustedRoot != string(filepath.Separator) {
		t.Fatalf("ProductionHelperTrustPolicy() = %#v", policy)
	}
}
