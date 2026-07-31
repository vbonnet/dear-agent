package systemdaudit

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
	config := testConfig(t, []byte("new audit"), []byte("new service"), []byte("new timer"))
	reloads := 0
	config.reload = func(context.Context) error {
		reloads++
		return nil
	}

	if err := Install(context.Background(), config); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for path, expected := range map[string]string{
		config.auditLive: "new audit", config.serviceLive: "new service", config.timerLive: "new timer",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read installed %s: %v", path, err)
		}
		if string(data) != expected {
			t.Fatalf("installed %s = %q, want %q", path, data, expected)
		}
	}
	if reloads != 1 {
		t.Fatalf("systemd reloads = %d, want 1", reloads)
	}
	assertNoTransactionFiles(t, filepath.Dir(config.auditLive))
}

func TestInstallReloadFailureRestoresCompletePriorSet(t *testing.T) {
	config := testConfig(t, []byte("new audit"), []byte("new service"), []byte("new timer"))
	for path, content := range map[string]string{
		config.auditLive: "old audit", config.serviceLive: "old service", config.timerLive: "old timer",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reloads := 0
	config.reload = func(context.Context) error {
		reloads++
		if reloads == 1 {
			return errors.New("daemon-reload failed")
		}
		return nil
	}

	err := Install(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "daemon-reload failed") {
		t.Fatalf("Install() error = %v, want reload failure", err)
	}
	for path, expected := range map[string]string{
		config.auditLive: "old audit", config.serviceLive: "old service", config.timerLive: "old timer",
	} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read restored %s: %v", path, readErr)
		}
		if string(data) != expected {
			t.Fatalf("restored %s = %q, want %q", path, data, expected)
		}
	}
	if reloads != 2 {
		t.Fatalf("systemd reloads = %d, want failed activation plus rollback reload", reloads)
	}
	assertNoTransactionFiles(t, filepath.Dir(config.auditLive))
}

func TestInstallDigestMismatchLeavesLiveSetUntouched(t *testing.T) {
	config := testConfig(t, []byte("new audit"), []byte("new service"), []byte("new timer"))
	config.expectedAuditHash = digest([]byte("different audit"))
	for _, path := range []string{config.auditLive, config.serviceLive, config.timerLive} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reloads := 0
	config.reload = func(context.Context) error {
		reloads++
		return nil
	}

	err := Install(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "digest differs") {
		t.Fatalf("Install() error = %v, want digest mismatch", err)
	}
	for _, path := range []string{config.auditLive, config.serviceLive, config.timerLive} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read unchanged %s: %v", path, readErr)
		}
		if string(data) != "old" {
			t.Fatalf("unchanged %s = %q, want old", path, data)
		}
	}
	if reloads != 0 {
		t.Fatalf("systemd reloads = %d after pre-activation digest failure, want 0", reloads)
	}
	assertNoTransactionFiles(t, filepath.Dir(config.auditLive))
}

func TestInstallCancellationAfterActivationRestoresPriorSet(t *testing.T) {
	config := testConfig(t, []byte("new audit"), []byte("new service"), []byte("new timer"))
	for _, path := range []string{config.auditLive, config.serviceLive, config.timerLive} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	reloads := 0
	config.reload = func(context.Context) error {
		reloads++
		if reloads == 1 {
			cancel(errors.New("received terminated"))
		}
		return nil
	}

	err := Install(ctx, config)
	if err == nil || !strings.Contains(err.Error(), "received terminated") {
		t.Fatalf("Install() error = %v, want cancellation cause", err)
	}
	for _, path := range []string{config.auditLive, config.serviceLive, config.timerLive} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read restored %s: %v", path, readErr)
		}
		if string(data) != "old" {
			t.Fatalf("restored %s = %q, want old", path, data)
		}
	}
	if reloads != 2 {
		t.Fatalf("systemd reloads = %d, want activation plus rollback reload", reloads)
	}
}

func testConfig(t *testing.T, audit, service, timer []byte) Config {
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
	serviceArtifact := filepath.Join(artifactDir, "service")
	timerArtifact := filepath.Join(artifactDir, "timer")
	for path, content := range map[string][]byte{
		auditArtifact: audit, serviceArtifact: service, timerArtifact: timer,
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config, err := NewConfig(
		os.Getgid(), auditArtifact, serviceArtifact, timerArtifact,
		digest(audit), digest(service), digest(timer),
	)
	if err != nil {
		t.Fatal(err)
	}
	config.rootUID = os.Getuid()
	config.auditLive = filepath.Join(liveDir, "audit-live")
	config.serviceLive = filepath.Join(liveDir, "service-live")
	config.timerLive = filepath.Join(liveDir, "timer-live")
	config.trustRoot = root
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
		if strings.Contains(entry.Name(), ".dear-agent-override-audit") {
			t.Fatalf("transaction file remains after completion: %s", entry.Name())
		}
	}
}
