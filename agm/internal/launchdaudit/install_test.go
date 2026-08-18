package launchdaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallActivatesVerifiedArtifactSet(t *testing.T) {
	config := testConfig(t, []byte("new audit"), []byte("new plist"))
	validations := 0
	config.validatePlist = func(_ context.Context, path string) error {
		validations++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != "new plist" {
			return errors.New("validator did not receive staged plist")
		}
		return nil
	}

	if err := Install(context.Background(), config); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for path, expected := range map[string]string{
		config.auditLive: "new audit",
		config.plistLive: "new plist",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read installed %s: %v", path, err)
		}
		if string(data) != expected {
			t.Fatalf("installed %s = %q, want %q", path, data, expected)
		}
	}
	if validations != 1 {
		t.Fatalf("plist validations = %d, want 1", validations)
	}
	assertNoTransactionFiles(t, filepath.Dir(config.auditLive))
}

func TestInstallPlistValidationFailureLeavesLiveSetUntouched(t *testing.T) {
	config := testConfig(t, []byte("new audit"), []byte("new plist"))
	writePriorSet(t, config)
	config.validatePlist = func(context.Context, string) error {
		return errors.New("plist invalid")
	}

	err := Install(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "plist invalid") {
		t.Fatalf("Install() error = %v, want plist validation failure", err)
	}
	assertPriorSet(t, config)
	assertNoTransactionFiles(t, filepath.Dir(config.auditLive))
}

func TestInstallDigestMismatchLeavesLiveSetUntouched(t *testing.T) {
	config := testConfig(t, []byte("new audit"), []byte("new plist"))
	writePriorSet(t, config)
	config.expectedAuditHash = digest([]byte("different audit"))
	validations := 0
	config.validatePlist = func(context.Context, string) error {
		validations++
		return nil
	}

	err := Install(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "digest differs") {
		t.Fatalf("Install() error = %v, want digest mismatch", err)
	}
	assertPriorSet(t, config)
	if validations != 0 {
		t.Fatalf("plist validations = %d after digest mismatch, want 0", validations)
	}
	assertNoTransactionFiles(t, filepath.Dir(config.auditLive))
}

func TestInstallCancellationAfterActivationRestoresPriorSet(t *testing.T) {
	config := testConfig(t, []byte("new audit"), []byte("new plist"))
	writePriorSet(t, config)
	ctx, cancel := context.WithCancelCause(context.Background())
	config.finish = func(context.Context) error {
		cancel(errors.New("received terminated"))
		return checkContext(ctx)
	}

	err := Install(ctx, config)
	if err == nil || !strings.Contains(err.Error(), "received terminated") {
		t.Fatalf("Install() error = %v, want cancellation cause", err)
	}
	assertPriorSet(t, config)
	assertNoTransactionFiles(t, filepath.Dir(config.auditLive))
}

func testConfig(t *testing.T, audit, plist []byte) Config {
	t.Helper()
	root := t.TempDir()
	artifactDir := filepath.Join(root, "artifacts")
	liveDir := filepath.Join(root, "live")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	auditArtifact := filepath.Join(artifactDir, "audit")
	plistArtifact := filepath.Join(artifactDir, "audit.plist")
	for path, content := range map[string][]byte{
		auditArtifact: audit,
		plistArtifact: plist,
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config, err := NewConfig(
		os.Getgid(), auditArtifact, plistArtifact,
		digest(audit), digest(plist),
	)
	if err != nil {
		t.Fatal(err)
	}
	config.rootUID = os.Getuid()
	config.auditLive = filepath.Join(liveDir, "audit-live")
	config.plistLive = filepath.Join(liveDir, "plist-live")
	config.trustRoot = root
	config.validatePlist = func(context.Context, string) error { return nil }
	return config
}

func TestVerifyTrustedAncestryRejectsWritableAndSymlinkedDirectories(t *testing.T) {
	root := t.TempDir()
	safe := filepath.Join(root, "safe")
	writable := filepath.Join(root, "writable")
	if err := os.Mkdir(safe, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(writable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(writable, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := verifyTrustedAncestry(safe, root, os.Getuid()); err != nil {
		t.Fatalf("safe ancestry rejected: %v", err)
	}
	if err := verifyTrustedAncestry(writable, root, os.Getuid()); err == nil {
		t.Fatal("world-writable ancestry accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(safe, link); err != nil {
		t.Fatal(err)
	}
	if err := verifyTrustedAncestry(link, root, os.Getuid()); err == nil {
		t.Fatal("symlinked ancestry accepted")
	}
}

func TestEnsureTrustedDirectoryRejectsSymlinkBeforeMutation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := ensureTrustedDirectory(link, root, os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("symlinked destination accepted")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("symlink target mode changed to %04o", got)
	}
}

func writePriorSet(t *testing.T, config Config) {
	t.Helper()
	for _, path := range []string{config.auditLive, config.plistLive} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertPriorSet(t *testing.T, config Config) {
	t.Helper()
	for _, path := range []string{config.auditLive, config.plistLive} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read restored %s: %v", path, err)
		}
		if string(data) != "old" {
			t.Fatalf("restored %s = %q, want old", path, data)
		}
	}
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func assertNoTransactionFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".dear-agent-override-audit") ||
			strings.Contains(entry.Name(), ".com.dear-agent.override-audit") {
			t.Fatalf("transaction file remains after completion: %s", entry.Name())
		}
	}
}
