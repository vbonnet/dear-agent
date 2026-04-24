package telemetry

import (
	"encoding/hex"
	"testing"
)

// TestGenerateSessionSalt_Length tests that session salt has correct length
func TestGenerateSessionSalt_Length(t *testing.T) {
	salt, err := GenerateSessionSalt()
	if err != nil {
		t.Fatalf("Failed to generate session salt: %v", err)
	}

	// Should be 64 hex characters (32 bytes)
	if len(salt) != 64 {
		t.Errorf("Expected 64 characters (32 bytes), got %d", len(salt))
	}
}

// TestGenerateSessionSalt_Uniqueness tests that each call generates unique salt
func TestGenerateSessionSalt_Uniqueness(t *testing.T) {
	// Generate 100 salts and verify they're all unique
	salts := make(map[string]bool)

	for i := 0; i < 100; i++ {
		salt, err := GenerateSessionSalt()
		if err != nil {
			t.Fatalf("Failed to generate salt %d: %v", i, err)
		}

		if salts[salt] {
			t.Errorf("Duplicate salt generated: %s", salt)
		}

		salts[salt] = true
	}

	if len(salts) != 100 {
		t.Errorf("Expected 100 unique salts, got %d", len(salts))
	}
}

// TestGenerateSessionSalt_ValidHex tests that salt is valid hex encoding
func TestGenerateSessionSalt_ValidHex(t *testing.T) {
	salt, err := GenerateSessionSalt()
	if err != nil {
		t.Fatalf("Failed to generate session salt: %v", err)
	}

	// Decode hex to verify it's valid
	decoded, err := hex.DecodeString(salt)
	if err != nil {
		t.Errorf("Salt is not valid hex: %v", err)
	}

	// Should decode to 32 bytes
	if len(decoded) != 32 {
		t.Errorf("Expected 32 bytes after decoding, got %d", len(decoded))
	}
}

// TestGenerateSessionSalt_Entropy tests that salt has sufficient entropy
func TestGenerateSessionSalt_Entropy(t *testing.T) {
	salt, err := GenerateSessionSalt()
	if err != nil {
		t.Fatalf("Failed to generate session salt: %v", err)
	}

	// Decode hex
	decoded, err := hex.DecodeString(salt)
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	// Count unique bytes (should have good distribution)
	uniqueBytes := make(map[byte]bool)
	for _, b := range decoded {
		uniqueBytes[b] = true
	}

	// With 32 bytes of crypto/rand, we should have at least 20 unique bytes
	// (This is a weak test but catches obvious issues like all zeros)
	if len(uniqueBytes) < 20 {
		t.Errorf("Low entropy: only %d unique bytes out of 32", len(uniqueBytes))
	}
}

// TestGenerateSessionSalt_NotAllZeros tests that salt is not all zeros
func TestGenerateSessionSalt_NotAllZeros(t *testing.T) {
	salt, err := GenerateSessionSalt()
	if err != nil {
		t.Fatalf("Failed to generate session salt: %v", err)
	}

	// Verify it's not all zeros
	allZeros := "0000000000000000000000000000000000000000000000000000000000000000"
	if salt == allZeros {
		t.Error("Generated salt is all zeros (crypto/rand failure)")
	}
}

// TestGenerateSessionSalt_NotEmpty tests that salt is not empty
func TestGenerateSessionSalt_NotEmpty(t *testing.T) {
	salt, err := GenerateSessionSalt()
	if err != nil {
		t.Fatalf("Failed to generate session salt: %v", err)
	}

	if salt == "" {
		t.Error("Generated salt is empty")
	}
}

// BenchmarkGenerateSessionSalt benchmarks salt generation performance
func BenchmarkGenerateSessionSalt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateSessionSalt()
		if err != nil {
			b.Fatalf("Failed to generate salt: %v", err)
		}
	}
}
