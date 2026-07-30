package systemd_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOverrideAuditTimerSchedulesJournalDeliveredAudit(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(sourceFile)
	service := readUnit(t, filepath.Join(dir, "dear-agent-override-audit.service"))
	timer := readUnit(t, filepath.Join(dir, "dear-agent-override-audit.timer"))

	for _, required := range []string{
		"ExecStart=%h/go/bin/agm override audit --window 168h --threshold 5 --notify",
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
		"Unit=dear-agent-override-audit.service",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(timer, required) {
			t.Errorf("timer does not retain %q", required)
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
