//go:build unix

package hookparity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectDeployedHelperTrustAndDrift(t *testing.T) {
	setup := func(t *testing.T) (string, string, HelperTrustPolicy) {
		t.Helper()
		root := t.TempDir()
		if err := os.Chmod(root, 0o700); err != nil {
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
}

func TestProductionHelperTrustPolicyRequiresRootOwnedAncestry(t *testing.T) {
	policy := ProductionHelperTrustPolicy()
	if policy.OwnerUID != 0 || policy.TrustedRoot != string(filepath.Separator) {
		t.Fatalf("ProductionHelperTrustPolicy() = %#v", policy)
	}
}
