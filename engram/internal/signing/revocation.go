package signing

import (
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"sync"
	"time"
)

// MockRevocationChecker implements RevocationChecker for testing
type MockRevocationChecker struct {
	// Revoked tracks revoked certificates by fingerprint
	Revoked map[string]RevokedCertificate
	mu      sync.RWMutex

	// SimulateErrors causes operations to fail
	SimulateErrors bool
}

// NewMockRevocationChecker creates a mock revocation checker
func NewMockRevocationChecker() *MockRevocationChecker {
	return &MockRevocationChecker{
		Revoked: make(map[string]RevokedCertificate),
	}
}

// IsRevoked checks if a certificate is revoked
func (r *MockRevocationChecker) IsRevoked(cert *x509.Certificate) (bool, error) {
	if r.SimulateErrors {
		return false, fmt.Errorf("simulated error: revocation check failed")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	fingerprint := fmt.Sprintf("%x", sha256.Sum256(cert.Raw))
	_, revoked := r.Revoked[fingerprint]
	return revoked, nil
}

// Revoke adds a certificate to revocation list
func (r *MockRevocationChecker) Revoke(cert *x509.Certificate, reason RevocationReason) error {
	if r.SimulateErrors {
		return fmt.Errorf("simulated error: revocation failed")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fingerprint := fmt.Sprintf("%x", sha256.Sum256(cert.Raw))
	serialNumber := cert.SerialNumber.String()

	r.Revoked[fingerprint] = RevokedCertificate{
		SerialNumber: serialNumber,
		RevokedAt:    time.Now(),
		Reason:       reason,
		Fingerprint:  fingerprint,
	}

	return nil
}

// GetCRL returns the current certificate revocation list
func (r *MockRevocationChecker) GetCRL() ([]RevokedCertificate, error) {
	if r.SimulateErrors {
		return nil, fmt.Errorf("simulated error: CRL retrieval failed")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	crl := make([]RevokedCertificate, 0, len(r.Revoked))
	for _, cert := range r.Revoked {
		crl = append(crl, cert)
	}

	return crl, nil
}

// Blocklist manages plugin and certificate blocklists
type Blocklist struct {
	// BlockedPlugins maps plugin names to block reasons
	BlockedPlugins map[string]string

	// BlockedCerts tracks blocked certificates by serial number
	BlockedCerts map[string]string

	mu sync.RWMutex
}

// NewBlocklist creates a new blocklist
func NewBlocklist() *Blocklist {
	return &Blocklist{
		BlockedPlugins: make(map[string]string),
		BlockedCerts:   make(map[string]string),
	}
}

// BlockPlugin adds a plugin to the blocklist
func (b *Blocklist) BlockPlugin(name string, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.BlockedPlugins[name] = reason
}

// UnblockPlugin removes a plugin from the blocklist
func (b *Blocklist) UnblockPlugin(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.BlockedPlugins, name)
}

// IsPluginBlocked checks if a plugin is blocked
func (b *Blocklist) IsPluginBlocked(name string) (bool, string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	reason, blocked := b.BlockedPlugins[name]
	return blocked, reason
}

// BlockCert adds a certificate to the blocklist
func (b *Blocklist) BlockCert(serialNumber string, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.BlockedCerts[serialNumber] = reason
}

// UnblockCert removes a certificate from the blocklist
func (b *Blocklist) UnblockCert(serialNumber string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.BlockedCerts, serialNumber)
}

// IsCertBlocked checks if a certificate is blocked
func (b *Blocklist) IsCertBlocked(serialNumber string) (bool, string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	reason, blocked := b.BlockedCerts[serialNumber]
	return blocked, reason
}

// GetBlockedPlugins returns all blocked plugins
func (b *Blocklist) GetBlockedPlugins() map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]string, len(b.BlockedPlugins))
	for k, v := range b.BlockedPlugins {
		result[k] = v
	}
	return result
}

// GetBlockedCerts returns all blocked certificates
func (b *Blocklist) GetBlockedCerts() map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]string, len(b.BlockedCerts))
	for k, v := range b.BlockedCerts {
		result[k] = v
	}
	return result
}
