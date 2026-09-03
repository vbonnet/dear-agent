package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"time"
)

// Claude Code on macOS keeps OAuth credentials in a composite store it names
// "keychain-with-plaintext-fallback": the login keychain is the primary and
// ~/.claude/.credentials.json is only consulted when the keychain yields
// nothing. dear-agent used to write the file alone, so a keychain item that was
// stale or blanked silently shadowed a perfectly refreshable file and every
// harness process reported "OAuth session expired and could not be refreshed"
// while the refresher logged success. The identity rules below mirror the CLI's
// so we address the same item it does; see docs/adr for the full derivation.
const (
	// keychainServiceBase is the CLI's service name before any config-dir
	// scoping suffix.
	keychainServiceBase = "Claude Code"

	// keychainServiceSuffix is appended to every credential service name.
	keychainCredentialsSuffix = "-credentials"

	// keychainFallbackAccount is what the CLI substitutes when the resolved
	// username is not a safe keychain account.
	keychainFallbackAccount = "claude-code-user"

	// keychainTimeout bounds each `security` invocation. The CLI uses 2s; a
	// contended login keychain can exceed that, and a timeout there is how a
	// rotated credential gets dropped, so we allow more headroom.
	keychainTimeout = 10 * time.Second
)

// keychainAccountPattern is the CLI's guard on the account name.
var keychainAccountPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// KeychainID identifies one Claude Code credential item.
//
// Both halves are derived from ambient environment, which is precisely why this
// host accumulated three stores: a process spawned with a different USER
// resolves to a different item, and a CLAUDE_CONFIG_DIR override changes the
// service name too. Items never merge, and the CLI's write path deletes the
// loser rather than mirroring, so divergence is permanent once it happens.
type KeychainID struct {
	Service string
	Account string
}

// KeychainIdentity derives the credential item identity the Claude Code CLI
// would use for the given environment, replicating its own resolution order.
func KeychainIdentity(getenv func(string) string, currentUser func() (string, error)) KeychainID {
	return KeychainID{
		Service: keychainService(getenv),
		Account: keychainAccount(getenv, currentUser),
	}
}

// keychainService reproduces the CLI's service-name derivation: the base name
// plus a hash of the config directory whenever one is explicitly configured, so
// a scoped config never collides with the default item.
func keychainService(getenv func(string) string) string {
	base := keychainServiceBase + keychainCredentialsSuffix

	// CLAUDE_SECURESTORAGE_CONFIG_DIR wins when set; an explicitly empty value
	// means "no scoping", matching the CLI's `e !== undefined ? !e : ...`.
	if secure, ok := lookupPresent(getenv, "CLAUDE_SECURESTORAGE_CONFIG_DIR"); ok {
		if secure == "" {
			return base
		}
		return base + "-" + shortHash(secure)
	}
	configDir := getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		return base
	}
	return base + "-" + shortHash(configDir)
}

// keychainAccount reproduces the CLI's account derivation: $USER, else the
// current OS user, collapsing anything unsafe to a fixed sentinel.
func keychainAccount(getenv func(string) string, currentUser func() (string, error)) string {
	name := getenv("USER")
	if name == "" {
		resolved, err := currentUser()
		if err != nil {
			return keychainFallbackAccount
		}
		name = resolved
	}
	if !keychainAccountPattern.MatchString(name) {
		return keychainFallbackAccount
	}
	return name
}

// lookupPresent distinguishes "set to empty" from "unset" for a getenv that
// cannot express the difference. A plain getenv returning "" is treated as
// unset, which matches os.Getenv semantics used everywhere else in this package.
func lookupPresent(getenv func(string) string, key string) (string, bool) {
	v := getenv(key)
	if v == "" {
		return "", false
	}
	return v, true
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

// DefaultKeychainIdentity resolves the identity from the real process
// environment and OS user.
func DefaultKeychainIdentity(getenv func(string) string) KeychainID {
	return KeychainIdentity(getenv, func() (string, error) {
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		return u.Username, nil
	})
}

// CredentialStore is one place Claude Code may keep OAuth credentials.
type CredentialStore interface {
	Read() (fullCredentials, bool)
	Write(fullCredentials) error
	Name() CredentialStoreKind
}

// KeychainStore reads and writes the macOS login-keychain credential item.
type KeychainStore struct {
	Identity KeychainID
	// Run executes a `security` invocation. Nil uses the real binary; tests
	// inject a stub so no test ever touches the operator's real keychain.
	Run func(args []string) ([]byte, error)
}

// Name identifies this store.
func (k KeychainStore) Name() CredentialStoreKind { return StoreKeychain }

func (k KeychainStore) run(args []string) ([]byte, error) {
	if k.Run != nil {
		return k.Run(args)
	}
	return runSecurity(args)
}

// Read returns the stored credentials, reporting false when the item is absent
// or unparseable. Absence is meaningful: it is what makes the CLI fall through
// to the file store.
func (k KeychainStore) Read() (fullCredentials, bool) {
	if k.Identity.Service == "" || k.Identity.Account == "" {
		return fullCredentials{}, false
	}
	out, err := k.run([]string{
		"find-generic-password",
		"-a", k.Identity.Account,
		"-s", k.Identity.Service,
		"-w",
	})
	if err != nil {
		return fullCredentials{}, false
	}
	var creds fullCredentials
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &creds); err != nil {
		return fullCredentials{}, false
	}
	return creds, true
}

// Write upserts the credential item.
//
// The payload is hex-encoded and passed via -X because that is how the CLI
// stores it; writing the JSON as a plaintext -w argument produces an item the
// CLI reads back as garbage, and it would also expose the secret in the process
// argument list where any local process could read it.
func (k KeychainStore) Write(creds fullCredentials) error {
	if k.Identity.Service == "" || k.Identity.Account == "" {
		return fmt.Errorf("keychain identity incomplete (service=%q account=%q)",
			k.Identity.Service, k.Identity.Account)
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	if _, err := k.run([]string{
		"add-generic-password",
		"-U",
		"-a", k.Identity.Account,
		"-s", k.Identity.Service,
		"-X", hex.EncodeToString(data),
	}); err != nil {
		return fmt.Errorf("keychain upsert: %w", err)
	}
	return nil
}

// runSecurity invokes the macOS security(1) binary.
func runSecurity(args []string) ([]byte, error) {
	ctx, cancel := contextWithTimeout(keychainTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}
