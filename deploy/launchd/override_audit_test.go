package launchd

import (
	"errors"
	"os"
	"os/exec"
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
		"<string>--config</string>",
		"<string>/dev/null</string>",
		"<key>HOME</key>",
		"launchctl bootstrap system",
		"<key>UserName</key>",
		"<string>__OPERATOR_USER__</string>",
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

func TestOverrideAuditInstallUsesOneNonCachingPrivilegedTransaction(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	makefile := readLaunchdFile(t, filepath.Join(root, "Makefile"))
	start := strings.Index(makefile, "install-override-audit-launchdaemon: build-agm")
	end := strings.Index(makefile, "\nuninstall-override-audit-launchdaemon:")
	if start < 0 || end <= start {
		t.Fatal("Makefile does not retain bounded LaunchDaemon audit install targets")
	}
	install := makefile[start:end]
	for _, required := range []string{
		"test -t 0",
		`operator_user="$$(/usr/bin/id -un)"`,
		"s|__OPERATOR_USER__|$$operator_user|g",
		`root_installer="$$(/bin/cat "$$root_installer_path")"`,
		`expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"`,
		`expected_plist_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$plist_candidate")"`,
		`expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"`,
		"IFS= read -r confirmed_audit_hash",
		"IFS= read -r confirmed_plist_hash",
		"IFS= read -r confirmed_installer_hash",
		`test "$$confirmed_installer_hash" = "$$expected_installer_hash"`,
		`printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer"`,
		`test "$$probe_status" = 1`,
		`printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`,
	} {
		if !strings.Contains(install, required) {
			t.Errorf("LaunchDaemon audit installer does not retain %q", required)
		}
	}
	if got := strings.Count(install, "/usr/bin/sudo"); got != 2 {
		t.Errorf("LaunchDaemon audit installer uses %d sudo invocations, want one exact probe and one transaction", got)
	}
	for _, forbidden := range []string{
		"/usr/bin/sudo /usr/bin/true",
		"/usr/bin/sudo -n /usr/bin/true",
		"/usr/bin/sudo /usr/bin/install",
		"/usr/bin/sudo /bin/mv",
		"/usr/bin/sudo /bin/rm",
	} {
		if strings.Contains(install, forbidden) {
			t.Errorf("LaunchDaemon audit installer retains reusable multi-command sudo flow %q", forbidden)
		}
	}

	rootInstallerPath := filepath.Join(root, "scripts", "install-override-audit-launchdaemon-root.sh")
	rootInstaller := readLaunchdFile(t, rootInstallerPath)
	requiredRootTransaction := []string{
		`if test "$mode" = "PROBE"; then`,
		`exit "$probe_exit"`,
		`audit_live=/usr/local/libexec/dear-agent-override-audit`,
		`plist_live=/Library/LaunchDaemons/com.dear-agent.override-audit.plist`,
		`trap 'cleanup_launchdaemon_staging "$?"' EXIT`,
		`trap 'cleanup_launchdaemon_staging 129' HUP`,
		`trap 'cleanup_launchdaemon_staging 130' INT`,
		`trap 'cleanup_launchdaemon_staging 143' TERM`,
		`/usr/bin/install -o root -g "$root_gid" -m 0755 "$audit_artifact" "$audit_staging"`,
		`/usr/bin/install -o root -g "$root_gid" -m 0644 "$plist_artifact" "$plist_staging"`,
		`test "$staged_audit_hash" = "$expected_audit_hash"`,
		`test "$staged_plist_hash" = "$expected_plist_hash"`,
		`/usr/bin/plutil -lint "$plist_staging"`,
		`activation_started=1`,
		`/bin/mv -f "$audit_staging" "$audit_live"`,
		`/bin/mv -f "$plist_staging" "$plist_live"`,
		`activation_complete=1`,
		`trap - EXIT HUP INT TERM`,
	}
	offset := 0
	for _, required := range requiredRootTransaction {
		next := strings.Index(rootInstaller[offset:], required)
		if next < 0 {
			t.Fatalf("fixed LaunchDaemon root installer lacks ordered transaction boundary %q", required)
		}
		offset += next + len(required)
	}
	if strings.Contains(rootInstaller, "sudo") {
		t.Fatal("fixed LaunchDaemon root installer recursively invokes sudo")
	}
	cleanupStart := strings.Index(rootInstaller, "cleanup_launchdaemon_staging()")
	signalTraps := strings.Index(rootInstaller, `trap 'cleanup_launchdaemon_staging "$?"' EXIT`)
	if cleanupStart < 0 || signalTraps <= cleanupStart {
		t.Fatal("fixed LaunchDaemon root installer lacks a bounded cleanup function")
	}
	cleanup := rootInstaller[cleanupStart:signalTraps]
	clear := strings.Index(cleanup, "trap - EXIT HUP INT TERM")
	bestEffort := strings.Index(cleanup, "set +e")
	exit := strings.Index(cleanup, `exit "$status"`)
	if clear < 0 || bestEffort < clear || exit < bestEffort {
		t.Fatal("LaunchDaemon cleanup does not clear traps, roll back, and exit in order")
	}
	if output, err := exec.Command("/bin/sh", "-n", rootInstallerPath).CombinedOutput(); err != nil {
		t.Fatalf("fixed LaunchDaemon root installer syntax: %v\n%s", err, output)
	}

	probe := exec.Command(
		"/bin/sh", "-c", rootInstaller,
		"dear-agent-override-audit-launchdaemon-installer",
		"0", "audit", "plist", "audit-hash", "plist-hash",
	)
	probe.Stdin = strings.NewReader("PROBE\n")
	err := probe.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
		t.Fatalf("fixed LaunchDaemon root installer probe error = %v, want exit 42", err)
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
