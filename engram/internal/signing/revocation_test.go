package signing

import (
	"testing"
)

func TestMockRevocationChecker_Revoke(t *testing.T) {
	checker := NewMockRevocationChecker()

	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	cert := signer.Certificate

	// Revoke certificate
	if err := checker.Revoke(cert, ReasonKeyCompromise); err != nil {
		t.Fatalf("Revoke() failed: %v", err)
	}

	// Check if revoked
	revoked, err := checker.IsRevoked(cert)
	if err != nil {
		t.Fatalf("IsRevoked() failed: %v", err)
	}

	if !revoked {
		t.Error("IsRevoked() = false, want true after revocation")
	}
}

func TestMockRevocationChecker_IsRevoked_NotRevoked(t *testing.T) {
	checker := NewMockRevocationChecker()

	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	cert := signer.Certificate

	// Check if revoked (should be false)
	revoked, err := checker.IsRevoked(cert)
	if err != nil {
		t.Fatalf("IsRevoked() failed: %v", err)
	}

	if revoked {
		t.Error("IsRevoked() = true, want false for non-revoked cert")
	}
}

func TestMockRevocationChecker_GetCRL(t *testing.T) {
	checker := NewMockRevocationChecker()

	signer1, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	signer2, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Revoke two certificates
	if err := checker.Revoke(signer1.Certificate, ReasonKeyCompromise); err != nil {
		t.Fatalf("Revoke() failed: %v", err)
	}

	if err := checker.Revoke(signer2.Certificate, ReasonSuperseded); err != nil {
		t.Fatalf("Revoke() failed: %v", err)
	}

	// Get CRL
	crl, err := checker.GetCRL()
	if err != nil {
		t.Fatalf("GetCRL() failed: %v", err)
	}

	if len(crl) != 2 {
		t.Errorf("GetCRL() returned %d entries, want 2", len(crl))
	}

	// Verify reasons
	reasons := make(map[RevocationReason]bool)
	for _, entry := range crl {
		reasons[entry.Reason] = true
	}

	if !reasons[ReasonKeyCompromise] {
		t.Error("GetCRL() missing ReasonKeyCompromise")
	}

	if !reasons[ReasonSuperseded] {
		t.Error("GetCRL() missing ReasonSuperseded")
	}
}

func TestMockRevocationChecker_SimulatedError_IsRevoked(t *testing.T) {
	checker := NewMockRevocationChecker()
	checker.SimulateErrors = true

	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	_, err = checker.IsRevoked(signer.Certificate)
	if err == nil {
		t.Error("IsRevoked() should fail with SimulateErrors=true")
	}
}

func TestMockRevocationChecker_SimulatedError_Revoke(t *testing.T) {
	checker := NewMockRevocationChecker()
	checker.SimulateErrors = true

	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	err = checker.Revoke(signer.Certificate, ReasonKeyCompromise)
	if err == nil {
		t.Error("Revoke() should fail with SimulateErrors=true")
	}
}

func TestMockRevocationChecker_SimulatedError_GetCRL(t *testing.T) {
	checker := NewMockRevocationChecker()
	checker.SimulateErrors = true

	_, err := checker.GetCRL()
	if err == nil {
		t.Error("GetCRL() should fail with SimulateErrors=true")
	}
}

func TestBlocklist_BlockPlugin(t *testing.T) {
	blocklist := NewBlocklist()

	blocklist.BlockPlugin("malicious-plugin", "Contains backdoor")

	blocked, reason := blocklist.IsPluginBlocked("malicious-plugin")
	if !blocked {
		t.Error("IsPluginBlocked() = false, want true")
	}

	if reason != "Contains backdoor" {
		t.Errorf("IsPluginBlocked() reason = %q, want %q", reason, "Contains backdoor")
	}
}

func TestBlocklist_UnblockPlugin(t *testing.T) {
	blocklist := NewBlocklist()

	blocklist.BlockPlugin("test-plugin", "Test reason")
	blocklist.UnblockPlugin("test-plugin")

	blocked, _ := blocklist.IsPluginBlocked("test-plugin")
	if blocked {
		t.Error("IsPluginBlocked() = true after unblock, want false")
	}
}

func TestBlocklist_IsPluginBlocked_NotBlocked(t *testing.T) {
	blocklist := NewBlocklist()

	blocked, reason := blocklist.IsPluginBlocked("safe-plugin")
	if blocked {
		t.Error("IsPluginBlocked() = true for non-blocked plugin, want false")
	}

	if reason != "" {
		t.Errorf("IsPluginBlocked() reason = %q, want empty string", reason)
	}
}

func TestBlocklist_BlockCert(t *testing.T) {
	blocklist := NewBlocklist()

	blocklist.BlockCert("1234567890", "Key compromised")

	blocked, reason := blocklist.IsCertBlocked("1234567890")
	if !blocked {
		t.Error("IsCertBlocked() = false, want true")
	}

	if reason != "Key compromised" {
		t.Errorf("IsCertBlocked() reason = %q, want %q", reason, "Key compromised")
	}
}

func TestBlocklist_UnblockCert(t *testing.T) {
	blocklist := NewBlocklist()

	blocklist.BlockCert("1234567890", "Test reason")
	blocklist.UnblockCert("1234567890")

	blocked, _ := blocklist.IsCertBlocked("1234567890")
	if blocked {
		t.Error("IsCertBlocked() = true after unblock, want false")
	}
}

func TestBlocklist_IsCertBlocked_NotBlocked(t *testing.T) {
	blocklist := NewBlocklist()

	blocked, reason := blocklist.IsCertBlocked("9999999999")
	if blocked {
		t.Error("IsCertBlocked() = true for non-blocked cert, want false")
	}

	if reason != "" {
		t.Errorf("IsCertBlocked() reason = %q, want empty string", reason)
	}
}

func TestBlocklist_GetBlockedPlugins(t *testing.T) {
	blocklist := NewBlocklist()

	blocklist.BlockPlugin("plugin1", "Reason 1")
	blocklist.BlockPlugin("plugin2", "Reason 2")
	blocklist.BlockPlugin("plugin3", "Reason 3")

	blocked := blocklist.GetBlockedPlugins()
	if len(blocked) != 3 {
		t.Errorf("GetBlockedPlugins() returned %d plugins, want 3", len(blocked))
	}

	if blocked["plugin1"] != "Reason 1" {
		t.Errorf("GetBlockedPlugins()[plugin1] = %q, want %q", blocked["plugin1"], "Reason 1")
	}
}

func TestBlocklist_GetBlockedCerts(t *testing.T) {
	blocklist := NewBlocklist()

	blocklist.BlockCert("111", "Reason 1")
	blocklist.BlockCert("222", "Reason 2")

	blocked := blocklist.GetBlockedCerts()
	if len(blocked) != 2 {
		t.Errorf("GetBlockedCerts() returned %d certs, want 2", len(blocked))
	}

	if blocked["111"] != "Reason 1" {
		t.Errorf("GetBlockedCerts()[111] = %q, want %q", blocked["111"], "Reason 1")
	}
}

func TestBlocklist_Concurrent(t *testing.T) {
	blocklist := NewBlocklist()

	// Concurrent writes
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(n int) {
			blocklist.BlockPlugin("plugin-"+string(rune('0'+n)), "Test")
			blocklist.BlockCert("cert-"+string(rune('0'+n)), "Test")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all blocked
	plugins := blocklist.GetBlockedPlugins()
	certs := blocklist.GetBlockedCerts()

	if len(plugins) != 10 {
		t.Errorf("GetBlockedPlugins() returned %d, want 10", len(plugins))
	}

	if len(certs) != 10 {
		t.Errorf("GetBlockedCerts() returned %d, want 10", len(certs))
	}
}
