// Package signing provides signing-related functionality.
package signing

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// MockSigner implements Signer for testing
type MockSigner struct {
	// PrivateKey is the RSA private key for signing
	PrivateKey *rsa.PrivateKey

	// Certificate is the signing certificate
	Certificate *x509.Certificate

	// CertChain is the full certificate chain (leaf → root)
	CertChain []*x509.Certificate
}

// NewMockSigner creates a mock signer with self-signed certificate
func NewMockSigner() (*MockSigner, error) {
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Create self-signed certificate
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Mock Plugin Signer",
			Organization: []string{"Engram Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return &MockSigner{
		PrivateKey:  privateKey,
		Certificate: cert,
		CertChain:   []*x509.Certificate{cert},
	}, nil
}

// SignPlugin signs all files in a plugin directory
func (s *MockSigner) SignPlugin(pluginDir string) (*Signature, error) {
	// Compute file hashes
	fileHashes := make(map[string]string)

	err := filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip signature file itself
		if filepath.Base(path) == "plugin.sig" {
			return nil
		}

		// Compute SHA-256 hash
		data, err := os.ReadFile(path) //nolint:gosec // G122: trusted local paths, symlink TOCTOU not in threat model
		if err != nil {
			return err
		}

		hash := sha256.Sum256(data)
		relPath, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}

		fileHashes[relPath] = fmt.Sprintf("%x", hash)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to hash files: %w", err)
	}

	// Read plugin manifest for metadata
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	_, err = os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Extract plugin name and version (simplified - real implementation would parse YAML)
	pluginName := filepath.Base(pluginDir)
	version := "1.0.0" // Default version

	// Create payload
	payload := SignaturePayload{
		PluginName:     pluginName,
		Version:        version,
		FileHashes:     fileHashes,
		SignedAt:       time.Now(),
		SignerIdentity: s.Certificate.Subject.CommonName,
		Algorithm:      "sha256",
	}

	// Sign payload
	signatureB64, err := s.SignPayload(payload)
	if err != nil {
		return nil, err
	}

	// Encode certificate chain
	certChain := make([]string, len(s.CertChain))
	for i, cert := range s.CertChain {
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		})
		certChain[i] = string(certPEM)
	}

	return &Signature{
		Payload:   payload,
		Signature: signatureB64,
		CertChain: certChain,
		Algorithm: "RSA-SHA256",
	}, nil
}

// SignPayload signs a specific payload
func (s *MockSigner) SignPayload(payload SignaturePayload) (string, error) {
	// Marshal payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Compute SHA-256 hash of payload
	hash := sha256.Sum256(payloadBytes)

	// Sign hash with private key
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.PrivateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign payload: %w", err)
	}

	// Encode signature as base64
	return base64.StdEncoding.EncodeToString(signature), nil
}

// MockVerifier implements Verifier for testing
type MockVerifier struct {
	// TrustedCerts are the trusted root CA certificates
	TrustedCerts []*x509.Certificate

	// SimulateErrors causes verification to fail
	SimulateErrors bool
}

// NewMockVerifier creates a mock verifier
func NewMockVerifier(trustedCerts []*x509.Certificate) *MockVerifier {
	return &MockVerifier{
		TrustedCerts: trustedCerts,
	}
}

// VerifySignature verifies a plugin's signature
func (v *MockVerifier) VerifySignature(pluginDir string, signature *Signature) error {
	if v.SimulateErrors {
		return fmt.Errorf("simulated error: signature verification failed")
	}

	// Parse certificate chain
	certs := make([]*x509.Certificate, len(signature.CertChain))
	for i, certPEM := range signature.CertChain {
		block, _ := pem.Decode([]byte(certPEM))
		if block == nil {
			return fmt.Errorf("failed to decode certificate %d", i)
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse certificate %d: %w", i, err)
		}
		certs[i] = cert
	}

	// Verify certificate chain
	if len(certs) == 0 {
		return fmt.Errorf("empty certificate chain")
	}

	leafCert := certs[0]

	// Check if certificate is trusted (simplified - real implementation would verify full chain)
	trusted := false
	for _, trustedCert := range v.TrustedCerts {
		if leafCert.Equal(trustedCert) {
			trusted = true
			break
		}
	}

	if !trusted {
		return fmt.Errorf("certificate not trusted")
	}

	// Verify signature
	if err := v.VerifyPayload(signature.Payload, signature.Signature, leafCert); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	// Verify file hashes
	for relPath, expectedHash := range signature.Payload.FileHashes {
		fullPath := filepath.Join(pluginDir, relPath)

		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", relPath, err)
		}

		actualHash := sha256.Sum256(data)
		actualHashStr := fmt.Sprintf("%x", actualHash)

		if actualHashStr != expectedHash {
			return fmt.Errorf("file hash mismatch for %s: expected %s, got %s", relPath, expectedHash, actualHashStr)
		}
	}

	return nil
}

// VerifyPayload verifies a signature against a payload
func (v *MockVerifier) VerifyPayload(payload SignaturePayload, signatureB64 string, cert *x509.Certificate) error {
	// Decode signature
	signatureBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Marshal payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Compute hash
	hash := sha256.Sum256(payloadBytes)

	// Verify signature
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("certificate does not contain RSA public key")
	}

	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signatureBytes); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// MockTrustStore implements TrustStore for testing
type MockTrustStore struct {
	// CAs are the trusted CA certificates
	CAs map[string]*x509.Certificate
}

// NewMockTrustStore creates a mock trust store
func NewMockTrustStore() *MockTrustStore {
	return &MockTrustStore{
		CAs: make(map[string]*x509.Certificate),
	}
}

// AddCA adds a trusted root CA certificate
func (t *MockTrustStore) AddCA(cert *x509.Certificate) error {
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(cert.Raw))
	t.CAs[fingerprint] = cert
	return nil
}

// RemoveCA removes a CA certificate
func (t *MockTrustStore) RemoveCA(fingerprint string) error {
	delete(t.CAs, fingerprint)
	return nil
}

// ListCAs returns all trusted CA certificates
func (t *MockTrustStore) ListCAs() ([]*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0, len(t.CAs))
	for _, cert := range t.CAs {
		certs = append(certs, cert)
	}
	return certs, nil
}

// IsTrusted checks if a certificate chain is trusted
func (t *MockTrustStore) IsTrusted(chain []*x509.Certificate) (bool, error) {
	if len(chain) == 0 {
		return false, fmt.Errorf("empty certificate chain")
	}

	// Check if any cert in chain is a trusted CA
	for _, cert := range chain {
		fingerprint := fmt.Sprintf("%x", sha256.Sum256(cert.Raw))
		if _, exists := t.CAs[fingerprint]; exists {
			return true, nil
		}
	}

	return false, nil
}
