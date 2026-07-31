package launchd_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOverrideAuditInstallUsesOneNonCachingPrivilegedTransaction(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	makefile := readFile(t, filepath.Join(root, "Makefile"))
	start := strings.Index(makefile, "install-override-audit-launchdaemon: build-agm")
	end := strings.Index(makefile, "\nuninstall-override-audit-launchdaemon:")
	if start < 0 || end <= start {
		t.Fatal("Makefile does not retain bounded LaunchDaemon audit install targets")
	}
	install := makefile[start:end]
	for _, required := range []string{
		`root_installer="$$(/bin/cat "$$root_installer_path")"`,
		`expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"`,
		`expected_plist_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$plist_candidate")"`,
		`expected_helper_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$helper_artifact")"`,
		`expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"`,
		"IFS= read -r confirmed_audit_hash",
		"IFS= read -r confirmed_plist_hash",
		"IFS= read -r confirmed_helper_hash",
		"IFS= read -r confirmed_installer_hash",
		`test "$$confirmed_helper_hash" = "$$expected_helper_hash"`,
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
	rootInstaller := readFile(t, rootInstallerPath)
	requiredRootTransaction := []string{
		`test "$mode" != "PROBE" || exit "$probe_exit"`,
		`trap 'cleanup "$?"' EXIT`,
		`trap 'cleanup 129' HUP`,
		`trap 'cleanup 130' INT`,
		`trap 'cleanup 143' TERM`,
		`/usr/bin/install -d -o root -g "$root_gid" -m 0755 /usr/local/libexec`,
		`staging=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit-launchdaemon-installer.XXXXXX)`,
		`/usr/bin/install -o root -g "$root_gid" -m 0755 "$helper_artifact" "$staging"`,
		`test "$staged_hash" = "$expected_helper_hash"`,
		`"$staging" "$root_gid" "$4" "$5" "$6" "$7"`,
		`/bin/rm -f "$staging"`,
		`staging=`,
		`trap - EXIT HUP INT TERM`,
	}
	offset := 0
	for _, required := range requiredRootTransaction {
		next := strings.Index(rootInstaller[offset:], required)
		if next < 0 {
			t.Fatalf("fixed LaunchDaemon root bootstrap lacks ordered transaction boundary %q", required)
		}
		offset += next + len(required)
	}
	if strings.Contains(rootInstaller, "sudo") {
		t.Fatal("fixed LaunchDaemon root bootstrap recursively invokes sudo")
	}
	executableLines := 0
	for line := range strings.SplitSeq(rootInstaller, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			executableLines++
		}
	}
	if executableLines > 20 {
		t.Fatalf("fixed LaunchDaemon root bootstrap has %d executable lines, want at most 20", executableLines)
	}
	if output, err := exec.Command("/bin/sh", "-n", rootInstallerPath).CombinedOutput(); err != nil {
		t.Fatalf("fixed LaunchDaemon root bootstrap syntax: %v\n%s", err, output)
	}

	probe := exec.Command(
		"/bin/sh", "-c", rootInstaller,
		"dear-agent-override-audit-launchdaemon-installer",
		"helper", "helper-hash", "0", "audit", "plist", "audit-hash", "plist-hash",
	)
	probe.Stdin = strings.NewReader("PROBE\n")
	err := probe.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
		t.Fatalf("fixed LaunchDaemon root bootstrap probe error = %v, want exit 42", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
