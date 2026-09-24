package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testNotificationLogEnv = "TOKEN_REFRESHER_TEST_NOTIFICATION_LOG"

// TestMain puts a local notifier adapter ahead of the host's osascript so no
// package test can raise a real desktop notification.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "token-refresher-notifier-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := filepath.Join(dir, "osascript")
	script := "#!/bin/sh\n" +
		"if [ -n \"$" + testNotificationLogEnv + "\" ]; then\n" +
		"  printf 'alert\\n' >> \"$" + testNotificationLogEnv + "\"\n" +
		"fi\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	if err := os.Unsetenv(testNotificationLogEnv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func testNotificationLog(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alerts.log")
	t.Setenv(testNotificationLogEnv, path)
	return path
}

func assertNotificationCount(t *testing.T, path string, want int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) && want == 0 {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := len(data) / len("alert\n"); got != want || string(data) != strings.Repeat("alert\n", want) {
		t.Fatalf("notifications = %q, want %d alerts", data, want)
	}
}
