package telemetry

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateSessionSalt generates a cryptographically secure random session salt
//
// The session salt is used for privacy-preserving prompt hashing. It ensures
// that prompt hashes cannot be correlated across sessions.
//
// Security properties:
//   - 32 bytes (256 bits) of entropy
//   - Cryptographically secure via crypto/rand
//   - Hex-encoded for safe storage/transmission
//
// Returns:
//   - 64-character hex string (32 bytes)
//   - Empty string on error (caller should handle)
//
// Implementation note (P1-2):
// This addresses the "Session salt generation not implemented" P1 issue.
// Uses crypto/rand as specified in the security requirements.
func GenerateSessionSalt() (string, error) {
	salt := make([]byte, 32) // 32 bytes = 256 bits of entropy
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt), nil
}
