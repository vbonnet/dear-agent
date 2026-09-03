package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestKeychainIdentityDefaults(t *testing.T) {
	id := KeychainIdentity(func(k string) string { return "" }, func() (string, error) { return "vbonnet", nil })
	if id.Service != "Claude Code-credentials" {
		t.Fatalf("service = %q, want %q", id.Service, "Claude Code-credentials")
	}
	if id.Account != "vbonnet" {
		t.Fatalf("account = %q, want vbonnet", id.Account)
	}
}

func TestKeychainIdentityAccountFromUSER(t *testing.T) {
	env := map[string]string{"USER": "someone-else"}
	id := KeychainIdentity(func(k string) string { return env[k] }, func() (string, error) { return "vbonnet", nil })
	if id.Account != "someone-else" {
		t.Fatalf("account = %q, want someone-else", id.Account)
	}
}

// A USER that fails Claude Code's ^[a-zA-Z0-9._-]+$ guard collapses to the
// fixed sentinel, exactly as the CLI does.
func TestKeychainIdentityRejectsInvalidUSER(t *testing.T) {
	env := map[string]string{"USER": "bad user!"}
	id := KeychainIdentity(func(k string) string { return env[k] }, func() (string, error) { return "vbonnet", nil })
	if id.Account != "claude-code-user" {
		t.Fatalf("account = %q, want claude-code-user", id.Account)
	}
}

// A non-empty config-dir override makes the CLI suffix the service name with
// the first 8 hex chars of its SHA-256, so a scoped config gets its own item.
func TestKeychainIdentityConfigDirSuffix(t *testing.T) {
	env := map[string]string{"CLAUDE_CONFIG_DIR": "/tmp/alt-config", "USER": "vbonnet"}
	id := KeychainIdentity(func(k string) string { return env[k] }, func() (string, error) { return "vbonnet", nil })
	if !strings.HasPrefix(id.Service, "Claude Code-credentials-") {
		t.Fatalf("service = %q, want a hashed suffix", id.Service)
	}
	suffix := strings.TrimPrefix(id.Service, "Claude Code-credentials-")
	if len(suffix) != 8 {
		t.Fatalf("suffix = %q, want 8 hex chars", suffix)
	}
}

func TestKeychainStoreReadParsesRecord(t *testing.T) {
	ks := KeychainStore{
		Identity: KeychainID{Service: "svc", Account: "acct"},
		Run: func(args []string) ([]byte, error) {
			return []byte(`{"claudeAiOauth":{"accessToken":"a","refreshToken":"r","expiresAt":123}}`), nil
		},
	}
	creds, ok := ks.Read()
	if !ok {
		t.Fatal("Read() ok = false, want true")
	}
	if creds.ClaudeAIOAuth.RefreshToken != "r" {
		t.Fatalf("refreshToken = %q, want r", creds.ClaudeAIOAuth.RefreshToken)
	}
}

// A missing item must be reported as absent, not as an empty record: absence is
// what lets the CLI fall through to the file store.
func TestKeychainStoreReadMissingItem(t *testing.T) {
	ks := KeychainStore{
		Identity: KeychainID{Service: "svc", Account: "acct"},
		Run:      func(args []string) ([]byte, error) { return nil, errors.New("exit status 44") },
	}
	if _, ok := ks.Read(); ok {
		t.Fatal("Read() ok = true for a missing item, want false")
	}
}

// The CLI stores the blob hex-encoded via `add-generic-password -U ... -X`.
// Writing any other way produces an item the CLI cannot parse.
func TestKeychainStoreWriteUsesHexUpsert(t *testing.T) {
	var got []string
	ks := KeychainStore{
		Identity: KeychainID{Service: "svc", Account: "acct"},
		Run: func(args []string) ([]byte, error) {
			got = args
			return nil, nil
		},
	}
	creds := fullCredentials{}
	creds.ClaudeAIOAuth.AccessToken = "a"
	creds.ClaudeAIOAuth.RefreshToken = "r"
	if err := ks.Write(creds); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{"add-generic-password", "-U", "-a acct", "-s svc", "-X"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	// The hex payload must decode back to the credentials JSON, and must never
	// be passed as a plaintext -w argument.
	if strings.Contains(joined, "-w") {
		t.Fatalf("args %q used -w; the CLI expects hex via -X", joined)
	}
}
