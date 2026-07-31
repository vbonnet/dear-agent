package systemd_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOverrideAuditSystemTimerIsOperatorOwnedAndJournalDelivered(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(sourceFile)
	service := readUnit(t, filepath.Join(dir, "dear-agent-override-audit@.service"))
	timer := readUnit(t, filepath.Join(dir, "dear-agent-override-audit@.timer"))

	for _, required := range []string{
		"User=%i",
		"ExecStart=/usr/local/libexec/dear-agent-override-audit --config /dev/null override audit --window 168h --threshold 5 --notify",
		"WorkingDirectory=/",
		`Environment="HOME=/"`,
		"StandardOutput=journal",
		"StandardError=journal",
		"SyslogIdentifier=dear-agent-override-audit",
	} {
		if !strings.Contains(service, required) {
			t.Errorf("service does not retain %q", required)
		}
	}
	for _, required := range []string{
		"OnCalendar=*-*-* 10:00:00",
		"Persistent=true",
		"Unit=dear-agent-override-audit@%i.service",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(timer, required) {
			t.Errorf("timer does not retain %q", required)
		}
	}
	for _, forbidden := range []string{"%h", "WantedBy=default.target"} {
		if strings.Contains(service+"\n"+timer, forbidden) {
			t.Errorf("operator-owned units retain user-manager surface %q", forbidden)
		}
	}
}

func TestOverrideAuditInstallTargetsSystemManager(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	makefile := readUnit(t, filepath.Join(root, "Makefile"))
	start := strings.Index(makefile, "install-override-audit-systemd: build-agm")
	end := strings.Index(makefile, "\nuninstall-override-audit-systemd:")
	if start < 0 || end <= start {
		t.Fatal("Makefile does not retain bounded system audit install targets")
	}
	install := makefile[start:end]
	rootInstaller := readUnit(t, filepath.Join(root, "scripts", "install-override-audit-systemd-root.sh"))
	for _, required := range []string{
		`root_installer="$$(/bin/cat "$$root_installer_path")"`,
		`expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"`,
		`expected_service_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$service_artifact")"`,
		`expected_timer_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$timer_artifact")"`,
		`expected_helper_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$helper_artifact")"`,
		`expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"`,
		"IFS= read -r confirmed_audit_hash",
		"IFS= read -r confirmed_service_hash",
		"IFS= read -r confirmed_timer_hash",
		"IFS= read -r confirmed_helper_hash",
		"IFS= read -r confirmed_installer_hash",
		`test "$$confirmed_helper_hash" = "$$expected_helper_hash"`,
		`test "$$confirmed_installer_hash" = "$$expected_installer_hash"`,
		`printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer"`,
		`test "$$probe_status" = 1`,
		`printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`,
		"sudo systemctl enable --now dear-agent-override-audit@",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("system audit installer does not retain %q", required)
		}
	}
	if strings.Count(install, "/usr/bin/sudo") != 2 {
		t.Errorf("system audit installer must use exactly one probe and one non-caching privileged transaction")
	}
	for _, forbidden := range []string{".config/systemd/user", "install-override-audit-systemd-user"} {
		if strings.Contains(makefile, forbidden) {
			t.Errorf("Makefile retains same-user audit control surface %q", forbidden)
		}
	}
	for _, forbidden := range []string{"@systemctl --user", "echo \"  systemctl --user"} {
		if strings.Contains(install, forbidden) {
			t.Errorf("system audit installer retains same-user command %q", forbidden)
		}
	}
	for _, forbidden := range []string{
		"/usr/bin/sudo /usr/bin/true",
		"/usr/bin/sudo /usr/bin/install",
		`bin/agm /usr/local/libexec/dear-agent-override-audit`,
		`agm/systemd/dear-agent-override-audit@.service /etc/systemd/system/dear-agent-override-audit@.service`,
		`agm/systemd/dear-agent-override-audit@.timer /etc/systemd/system/dear-agent-override-audit@.timer`,
	} {
		if strings.Contains(install, forbidden) {
			t.Errorf("system audit installer copies mutable bytes directly to a privileged path: %q", forbidden)
		}
	}

	requiredRootTransaction := []string{
		`test "$mode" != "PROBE" || exit "$probe_exit"`,
		`trap 'cleanup "$?"' EXIT`,
		`trap 'cleanup 129' HUP`,
		`trap 'cleanup 130' INT`,
		`trap 'cleanup 143' TERM`,
		`/usr/bin/install -d -o root -g "$root_gid" -m 0755 /usr/local/libexec`,
		`staging=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit-systemd-installer.XXXXXX)`,
		`/usr/bin/install -o root -g "$root_gid" -m 0755 "$helper_artifact" "$staging"`,
		`test "$staged_hash" = "$expected_helper_hash"`,
		`"$staging" "$root_gid" "$4" "$5" "$6" "$7" "$8" "$9"`,
		`/bin/rm -f "$staging"`,
		`staging=`,
		`trap - EXIT HUP INT TERM`,
	}
	offset := 0
	for _, required := range requiredRootTransaction {
		next := strings.Index(rootInstaller[offset:], required)
		if next < 0 {
			t.Fatalf("fixed root installer lacks ordered transaction boundary %q", required)
		}
		offset += next + len(required)
	}
	if strings.Contains(rootInstaller, "sudo") {
		t.Fatal("fixed root installer recursively invokes sudo")
	}
	cleanupStart := strings.Index(rootInstaller, "cleanup()")
	signalTraps := strings.Index(rootInstaller, `trap 'cleanup "$?"' EXIT`)
	if cleanupStart < 0 || signalTraps <= cleanupStart {
		t.Fatal("fixed root installer lacks a bounded cleanup function")
	}
	cleanup := rootInstaller[cleanupStart:signalTraps]
	if clear := strings.Index(cleanup, "trap - EXIT HUP INT TERM"); clear < 0 {
		t.Fatal("root transaction cleanup does not clear signal traps")
	} else if bestEffort := strings.Index(cleanup, "set +e"); bestEffort < clear {
		t.Fatal("root transaction cleanup does not clear traps before best-effort rollback")
	}
	executableLines := 0
	for line := range strings.SplitSeq(rootInstaller, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			executableLines++
		}
	}
	if executableLines > 20 {
		t.Fatalf("fixed root bootstrap has %d executable lines, want at most 20", executableLines)
	}

	cmd := exec.Command(
		"/bin/sh", "-c", rootInstaller,
		"dear-agent-override-audit-systemd-installer",
		"helper", "helper-hash", "0", "audit", "service", "timer",
		"audit-hash", "service-hash", "timer-hash",
	)
	cmd.Stdin = strings.NewReader("PROBE\n")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
		t.Fatalf("fixed root installer probe error = %v, want exit 42", err)
	}
}

func readUnit(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
