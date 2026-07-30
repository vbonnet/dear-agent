package launchd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOverrideAuditUsesRootOwnedSystemDomain(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(sourceFile)
	plist := readLaunchdFile(t, filepath.Join(dir, "com.dear-agent.override-audit.plist"))
	for _, required := range []string{
		"/usr/local/libexec/dear-agent-override-audit",
		"/var/log/dear-agent-override-audit.out.log",
		"/var/log/dear-agent-override-audit.err.log",
		"/var/empty",
		"launchctl bootstrap system",
	} {
		if !strings.Contains(plist, required) {
			t.Errorf("system audit plist does not retain %q", required)
		}
	}
	for _, forbidden := range []string{
		"__HOME__",
		"/go/bin/agm",
		"Library/LaunchAgents",
		"launchctl bootstrap gui/",
	} {
		if strings.Contains(plist, forbidden) {
			t.Errorf("system audit plist retains same-user control surface %q", forbidden)
		}
	}
}

func TestOverrideAuditInstallerRequiresFreshOperatorBoundary(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	makefile := readLaunchdFile(t, filepath.Join(root, "Makefile"))
	start := strings.Index(makefile, "install-override-audit-launchdaemon: build-agm")
	end := strings.Index(makefile, "\nuninstall-override-audit-launchdaemon:")
	if start < 0 || end <= start {
		t.Fatal("Makefile does not retain bounded LaunchDaemon audit install target")
	}
	install := makefile[start:end]
	for _, required := range []string{
		"test -t 0",
		"/usr/bin/sudo -k",
		"/usr/bin/sudo -v",
		"/usr/local/libexec/dear-agent-override-audit",
		"/Library/LaunchDaemons/com.dear-agent.override-audit.plist",
		"sudo launchctl bootstrap system",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("system audit installer does not retain %q", required)
		}
	}
	for _, forbidden := range []string{"Library/LaunchAgents", "launchctl bootstrap gui/"} {
		if strings.Contains(install, forbidden) {
			t.Errorf("system audit installer retains same-user control surface %q", forbidden)
		}
	}
	if strings.Contains(makefile, "override-audit-launchagent") {
		t.Error("Makefile retains the retired user-agent audit target")
	}

	manifest := readLaunchdFile(t, filepath.Join(root, "deploy", "manifest.yaml"))
	if strings.Contains(manifest, "com.dear-agent.override-audit") {
		t.Fatal("user-scoped deploy manifest still owns the system audit")
	}
}

func readLaunchdFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
