package hookparity

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVerifyRunningHelperImageRejectsAForeignImage pins the property that made
// this function separate from VerifyContentAddressedHelperInvocation: the
// running executable is resolved, never accepted as an argument, so no caller
// can present an image other than its own. The test binary is never the pinned
// helper, so verification must refuse rather than authenticate a path this
// process is not actually running.
func TestVerifyRunningHelperImageRejectsAForeignImage(t *testing.T) {
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

	policy := HelperTrustPolicy{OwnerUID: uint32(os.Getuid()), TrustedRoot: root}

	// The bytes at the pinned path are authentic, and the path-based check
	// therefore admits them.
	if err := VerifyContentAddressedHelperInvocation(pinned, stable, digest, policy); err != nil {
		t.Fatalf("VerifyContentAddressedHelperInvocation(pinned) = %v, want nil", err)
	}
	// The running-image check must still refuse, because this process is not
	// that helper. Authentic bytes at a path are not evidence about what is
	// executing.
	err = VerifyRunningHelperImage(stable, digest, policy)
	if err == nil {
		t.Fatal("VerifyRunningHelperImage admitted a helper this process is not running")
	}
	if !strings.Contains(err.Error(), "content-addressed path") {
		t.Fatalf("VerifyRunningHelperImage error = %v, want the content-addressed path refusal", err)
	}
}

// TestRunningImageIdentityIsSelfConsistent proves the platform helper reports a
// usable identity where it claims one, so the SameFile binding in
// VerifyRunningHelperImage cannot silently compare against nothing.
func TestRunningImageIdentityIsSelfConsistent(t *testing.T) {
	image, available, err := runningImageIdentity()
	if err != nil {
		t.Fatalf("runningImageIdentity: %v", err)
	}
	if !available {
		if image != nil {
			t.Fatal("runningImageIdentity reported unavailable but returned an identity")
		}
		t.Skip("this platform exposes no handle on the running image (HHP-23 residual)")
	}
	if image == nil {
		t.Fatal("runningImageIdentity reported available but returned no identity")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(image, self) {
		t.Fatalf("running image %v is not os.Executable() %q", image.Name(), executable)
	}
}
