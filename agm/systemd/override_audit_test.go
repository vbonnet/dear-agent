package systemd_test

import (
	"os"
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
	for _, required := range []string{
		"/usr/bin/sudo -k",
		"/usr/bin/sudo /usr/bin/true",
		`expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"`,
		`expected_service_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$service_artifact")"`,
		`expected_timer_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$timer_artifact")"`,
		"IFS= read -r confirmed_audit_hash",
		"IFS= read -r confirmed_service_hash",
		"IFS= read -r confirmed_timer_hash",
		"/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit.XXXXXX",
		"/usr/bin/sudo /usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-service.XXXXXX",
		"/usr/bin/sudo /usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-timer.XXXXXX",
		`test "$$staged_audit_hash" = "$$expected_audit_hash"`,
		`test "$$staged_service_hash" = "$$expected_service_hash"`,
		`test "$$staged_timer_hash" = "$$expected_timer_hash"`,
		"/usr/local/libexec/dear-agent-override-audit",
		"/etc/systemd/system/dear-agent-override-audit@.service",
		"/etc/systemd/system/dear-agent-override-audit@.timer",
		"/usr/bin/sudo /usr/bin/systemctl daemon-reload",
		"sudo systemctl enable --now dear-agent-override-audit@",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("system audit installer does not retain %q", required)
		}
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
		`bin/agm /usr/local/libexec/dear-agent-override-audit`,
		`agm/systemd/dear-agent-override-audit@.service /etc/systemd/system/dear-agent-override-audit@.service`,
		`agm/systemd/dear-agent-override-audit@.timer /etc/systemd/system/dear-agent-override-audit@.timer`,
	} {
		if strings.Contains(install, forbidden) {
			t.Errorf("system audit installer copies mutable bytes directly to a privileged path: %q", forbidden)
		}
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
