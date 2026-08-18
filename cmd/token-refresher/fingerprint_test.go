package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// credsWithRefreshToken wraps the package's writeCreds helper for the cases
// here, where only the refresh token is interesting.
func credsWithRefreshToken(t *testing.T, refreshToken string) string {
	t.Helper()
	return writeCreds(t, "access-token", time.Now().Add(time.Hour).UnixMilli(), refreshToken)
}

func TestCredentialsFingerprint_StableForSameToken(t *testing.T) {
	a := credsWithRefreshToken(t, "rt-alpha")
	b := credsWithRefreshToken(t, "rt-alpha")

	fpA, modA := credentialsFingerprint(a)
	fpB, _ := credentialsFingerprint(b)

	if fpA == "" {
		t.Fatal("fingerprint empty for a readable credentials file")
	}
	if fpA != fpB {
		t.Errorf("same refresh token produced different fingerprints: %q vs %q", fpA, fpB)
	}
	if len(fpA) != auth.FingerprintLen {
		t.Errorf("fingerprint length = %d, want %d", len(fpA), auth.FingerprintLen)
	}
	if modA == "" {
		t.Error("expected a modification time for a readable credentials file")
	}
}

// The whole point of the fingerprint is detecting that somebody else rotated
// the refresh token, so a different token must yield a different value.
func TestCredentialsFingerprint_ChangesWhenTokenRotates(t *testing.T) {
	before, _ := credentialsFingerprint(credsWithRefreshToken(t, "rt-alpha"))
	after, _ := credentialsFingerprint(credsWithRefreshToken(t, "rt-beta"))

	if before == after {
		t.Errorf("rotated refresh token kept fingerprint %q — third-party rotation would be invisible", before)
	}
}

// The fingerprint must never leak the token it describes.
func TestCredentialsFingerprint_DoesNotContainToken(t *testing.T) {
	const token = "rt-super-secret-value"
	fp, _ := credentialsFingerprint(credsWithRefreshToken(t, token))

	if fp == token {
		t.Fatal("fingerprint is the raw refresh token")
	}
	if len(fp) >= len(token) {
		t.Errorf("fingerprint %q is not a short digest prefix", fp)
	}
}

func TestCredentialsFingerprint_MissingOrMalformedIsBestEffort(t *testing.T) {
	fp, mod := credentialsFingerprint(filepath.Join(t.TempDir(), "absent.json"))
	if fp != "" || mod != "" {
		t.Errorf("missing file: got (%q, %q), want empty", fp, mod)
	}

	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if fp, mod := credentialsFingerprint(bad); fp != "" || mod == "" {
		t.Errorf("malformed json: got (%q, %q), want empty fingerprint but a mtime", fp, mod)
	}
}
