package signing

import (
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMockSigner_SignPlugin(t *testing.T) {
	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Create temporary plugin directory
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin dir: %v", err)
	}

	// Create plugin files
	files := map[string]string{
		"plugin.yaml": "name: test-plugin\nversion: 1.0.0\n",
		"main.py":     "print('hello world')",
		"README.md":   "# Test Plugin",
	}

	for name, content := range files {
		path := filepath.Join(pluginDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", name, err)
		}
	}

	// Sign plugin
	signature, err := signer.SignPlugin(pluginDir)
	if err != nil {
		t.Fatalf("SignPlugin() failed: %v", err)
	}

	// Verify signature structure
	if signature.Payload.PluginName == "" {
		t.Error("SignPlugin() payload missing plugin name")
	}

	if signature.Signature == "" {
		t.Error("SignPlugin() signature is empty")
	}

	if len(signature.CertChain) == 0 {
		t.Error("SignPlugin() cert chain is empty")
	}

	if signature.Algorithm != "RSA-SHA256" {
		t.Errorf("SignPlugin() algorithm = %q, want %q", signature.Algorithm, "RSA-SHA256")
	}

	// Verify file hashes were computed
	if len(signature.Payload.FileHashes) != 3 {
		t.Errorf("SignPlugin() file hashes = %d, want 3", len(signature.Payload.FileHashes))
	}
}

func TestMockVerifier_VerifySignature_Valid(t *testing.T) {
	// Create signer
	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Create temporary plugin directory
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin dir: %v", err)
	}

	// Create plugin files
	files := map[string]string{
		"plugin.yaml": "name: test-plugin\nversion: 1.0.0\n",
		"main.py":     "print('hello world')",
	}

	for name, content := range files {
		path := filepath.Join(pluginDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", name, err)
		}
	}

	// Sign plugin
	signature, err := signer.SignPlugin(pluginDir)
	if err != nil {
		t.Fatalf("SignPlugin() failed: %v", err)
	}

	// Create verifier with signer's cert as trusted
	verifier := NewMockVerifier([]*x509.Certificate{signer.Certificate})

	// Verify signature
	if err := verifier.VerifySignature(pluginDir, signature); err != nil {
		t.Errorf("VerifySignature() failed: %v", err)
	}
}

func TestMockVerifier_VerifySignature_UntrustedCert(t *testing.T) {
	// Create signer
	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Create temporary plugin directory
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin dir: %v", err)
	}

	// Create plugin file
	pluginYAML := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(pluginYAML, []byte("name: test\n"), 0644); err != nil {
		t.Fatalf("Failed to write plugin.yaml: %v", err)
	}

	// Sign plugin
	signature, err := signer.SignPlugin(pluginDir)
	if err != nil {
		t.Fatalf("SignPlugin() failed: %v", err)
	}

	// Create verifier WITHOUT signer's cert
	verifier := NewMockVerifier([]*x509.Certificate{})

	// Verify signature (should fail)
	err = verifier.VerifySignature(pluginDir, signature)
	if err == nil {
		t.Error("VerifySignature() should fail with untrusted certificate")
	}

	if !strings.Contains(err.Error(), "not trusted") {
		t.Errorf("VerifySignature() error = %v, want 'not trusted' error", err)
	}
}

func TestMockVerifier_VerifySignature_ModifiedFile(t *testing.T) {
	// Create signer
	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Create temporary plugin directory
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin dir: %v", err)
	}

	// Create plugin file
	mainPy := filepath.Join(pluginDir, "main.py")
	pluginYAML := filepath.Join(pluginDir, "plugin.yaml")

	if err := os.WriteFile(pluginYAML, []byte("name: test\n"), 0644); err != nil {
		t.Fatalf("Failed to write plugin.yaml: %v", err)
	}

	if err := os.WriteFile(mainPy, []byte("print('original')"), 0644); err != nil {
		t.Fatalf("Failed to write main.py: %v", err)
	}

	// Sign plugin
	signature, err := signer.SignPlugin(pluginDir)
	if err != nil {
		t.Fatalf("SignPlugin() failed: %v", err)
	}

	// Modify file AFTER signing
	if err := os.WriteFile(mainPy, []byte("print('modified')"), 0644); err != nil {
		t.Fatalf("Failed to modify main.py: %v", err)
	}

	// Create verifier
	verifier := NewMockVerifier([]*x509.Certificate{signer.Certificate})

	// Verify signature (should fail due to modified file)
	err = verifier.VerifySignature(pluginDir, signature)
	if err == nil {
		t.Error("VerifySignature() should fail with modified file")
	}

	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("VerifySignature() error = %v, want hash mismatch error", err)
	}
}

func TestMockVerifier_VerifySignature_SimulatedError(t *testing.T) {
	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin dir: %v", err)
	}

	pluginYAML := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(pluginYAML, []byte("name: test\n"), 0644); err != nil {
		t.Fatalf("Failed to write plugin.yaml: %v", err)
	}

	signature, err := signer.SignPlugin(pluginDir)
	if err != nil {
		t.Fatalf("SignPlugin() failed: %v", err)
	}

	verifier := NewMockVerifier([]*x509.Certificate{signer.Certificate})
	verifier.SimulateErrors = true

	err = verifier.VerifySignature(pluginDir, signature)
	if err == nil {
		t.Error("VerifySignature() should fail with SimulateErrors=true")
	}
}

func TestMockTrustStore_AddCA(t *testing.T) {
	store := NewMockTrustStore()

	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	if err := store.AddCA(signer.Certificate); err != nil {
		t.Errorf("AddCA() failed: %v", err)
	}

	certs, err := store.ListCAs()
	if err != nil {
		t.Fatalf("ListCAs() failed: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("ListCAs() returned %d certs, want 1", len(certs))
	}
}

func TestMockTrustStore_RemoveCA(t *testing.T) {
	store := NewMockTrustStore()

	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Add CA
	if err := store.AddCA(signer.Certificate); err != nil {
		t.Fatalf("AddCA() failed: %v", err)
	}

	// Get fingerprint
	fingerprint := getCertFingerprint(signer.Certificate)

	// Remove CA
	if err := store.RemoveCA(fingerprint); err != nil {
		t.Errorf("RemoveCA() failed: %v", err)
	}

	// Verify removed
	certs, err := store.ListCAs()
	if err != nil {
		t.Fatalf("ListCAs() failed: %v", err)
	}

	if len(certs) != 0 {
		t.Errorf("ListCAs() returned %d certs after removal, want 0", len(certs))
	}
}

func TestMockTrustStore_IsTrusted(t *testing.T) {
	store := NewMockTrustStore()

	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Add signer's cert to trust store
	if err := store.AddCA(signer.Certificate); err != nil {
		t.Fatalf("AddCA() failed: %v", err)
	}

	// Check if trusted
	trusted, err := store.IsTrusted([]*x509.Certificate{signer.Certificate})
	if err != nil {
		t.Fatalf("IsTrusted() failed: %v", err)
	}

	if !trusted {
		t.Error("IsTrusted() = false, want true for trusted cert")
	}

	// Create another signer (not trusted)
	untrustedSigner, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	// Check if untrusted
	trusted, err = store.IsTrusted([]*x509.Certificate{untrustedSigner.Certificate})
	if err != nil {
		t.Fatalf("IsTrusted() failed: %v", err)
	}

	if trusted {
		t.Error("IsTrusted() = true, want false for untrusted cert")
	}
}

func TestMockTrustStore_IsTrusted_EmptyChain(t *testing.T) {
	store := NewMockTrustStore()

	_, err := store.IsTrusted([]*x509.Certificate{})
	if err == nil {
		t.Error("IsTrusted() should fail with empty chain")
	}
}

func TestMockSigner_SignPayload(t *testing.T) {
	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	payload := SignaturePayload{
		PluginName:     "test-plugin",
		Version:        "1.0.0",
		FileHashes:     map[string]string{"main.py": "abc123"},
		SignerIdentity: "Test Signer",
		Algorithm:      "sha256",
	}

	signature, err := signer.SignPayload(payload)
	if err != nil {
		t.Fatalf("SignPayload() failed: %v", err)
	}

	if signature == "" {
		t.Error("SignPayload() returned empty signature")
	}

	// Verify signature
	verifier := NewMockVerifier([]*x509.Certificate{signer.Certificate})
	if err := verifier.VerifyPayload(payload, signature, signer.Certificate); err != nil {
		t.Errorf("VerifyPayload() failed: %v", err)
	}
}

func TestMockVerifier_VerifyPayload_Invalid(t *testing.T) {
	signer, err := NewMockSigner()
	if err != nil {
		t.Fatalf("NewMockSigner() failed: %v", err)
	}

	payload := SignaturePayload{
		PluginName: "test-plugin",
		Version:    "1.0.0",
	}

	signature, err := signer.SignPayload(payload)
	if err != nil {
		t.Fatalf("SignPayload() failed: %v", err)
	}

	// Modify payload AFTER signing
	payload.Version = "2.0.0"

	verifier := NewMockVerifier([]*x509.Certificate{signer.Certificate})
	err = verifier.VerifyPayload(payload, signature, signer.Certificate)
	if err == nil {
		t.Error("VerifyPayload() should fail with modified payload")
	}
}

// Helper function to get certificate fingerprint
func getCertFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return fmt.Sprintf("%x", hash)
}
