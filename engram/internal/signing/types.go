package signing

import (
	"crypto/x509"
	"time"
)

// SignaturePayload contains the data to be signed
type SignaturePayload struct {
	// PluginName is the plugin identifier
	PluginName string `json:"plugin_name"`

	// Version is the plugin version
	Version string `json:"version"`

	// FileHashes maps relative file paths to their SHA-256 hashes
	FileHashes map[string]string `json:"file_hashes"`

	// SignedAt is the signature creation timestamp
	SignedAt time.Time `json:"signed_at"`

	// SignerIdentity is the signer's common name from certificate
	SignerIdentity string `json:"signer_identity"`

	// Algorithm is the hash algorithm used (e.g., "sha256")
	Algorithm string `json:"algorithm"`
}

// Signature represents a cryptographic signature for a plugin
type Signature struct {
	// Payload is the signed data
	Payload SignaturePayload `json:"payload"`

	// Signature is the base64-encoded RSA signature
	Signature string `json:"signature"`

	// CertChain is the PEM-encoded certificate chain
	// [0] is leaf (signing cert), [1..n] are intermediates, [n] is root
	CertChain []string `json:"cert_chain"`

	// Algorithm is the signature algorithm (e.g., "RSA-SHA256")
	Algorithm string `json:"algorithm"`
}

// Signer creates cryptographic signatures for plugins
type Signer interface {
	// SignPlugin signs all files in a plugin directory
	SignPlugin(pluginDir string) (*Signature, error)

	// SignPayload signs a specific payload
	SignPayload(payload SignaturePayload) (string, error)
}

// Verifier verifies cryptographic signatures
type Verifier interface {
	// VerifySignature verifies a plugin's signature
	VerifySignature(pluginDir string, signature *Signature) error

	// VerifyPayload verifies a signature against a payload
	VerifyPayload(payload SignaturePayload, signatureB64 string, cert *x509.Certificate) error
}

// TrustStore manages trusted certificate authorities
type TrustStore interface {
	// AddCA adds a trusted root CA certificate
	AddCA(cert *x509.Certificate) error

	// RemoveCA removes a CA certificate
	RemoveCA(fingerprint string) error

	// ListCAs returns all trusted CA certificates
	ListCAs() ([]*x509.Certificate, error)

	// IsTrusted checks if a certificate chain is trusted
	IsTrusted(chain []*x509.Certificate) (bool, error)
}

// RevocationChecker checks if certificates are revoked
type RevocationChecker interface {
	// IsRevoked checks if a certificate is revoked
	IsRevoked(cert *x509.Certificate) (bool, error)

	// Revoke adds a certificate to revocation list
	Revoke(cert *x509.Certificate, reason RevocationReason) error

	// GetCRL returns the current certificate revocation list
	GetCRL() ([]RevokedCertificate, error)
}

// RevocationReason indicates why a certificate was revoked
type RevocationReason string

// Recognized certificate RevocationReason values.
const (
	ReasonUnspecified          RevocationReason = "unspecified"
	ReasonKeyCompromise        RevocationReason = "key_compromise"
	ReasonCACompromise         RevocationReason = "ca_compromise"
	ReasonAffiliationChanged   RevocationReason = "affiliation_changed"
	ReasonSuperseded           RevocationReason = "superseded"
	ReasonCessationOfOperation RevocationReason = "cessation_of_operation"
	ReasonCertificateHold      RevocationReason = "certificate_hold"
)

// RevokedCertificate represents a revoked certificate
type RevokedCertificate struct {
	// SerialNumber is the certificate serial number
	SerialNumber string

	// RevokedAt is the revocation timestamp
	RevokedAt time.Time

	// Reason is why the certificate was revoked
	Reason RevocationReason

	// Fingerprint is SHA-256 hash of certificate
	Fingerprint string
}
