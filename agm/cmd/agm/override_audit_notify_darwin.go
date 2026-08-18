//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func sendOverrideAuditNotification(message string) error {
	// #nosec G204 -- executable and AppleScript are fixed; message is an argv value.
	notification := exec.Command(
		"/usr/bin/osascript",
		"-e", "on run argv",
		"-e", `display notification (item 1 of argv) with title "Dear Agent override audit"`,
		"-e", "end run",
		message,
	)
	notificationOutput, notificationErr := notification.CombinedOutput()

	// #nosec G204 -- executable and logger options are fixed; message is an argv value.
	logOutput, logErr := exec.Command(
		"/usr/bin/logger", "-t", "com.dear-agent.override-audit", message,
	).CombinedOutput()
	if notificationErr == nil || logErr == nil {
		return nil
	}
	return errors.Join(
		fmt.Errorf("deliver Notification Center alert: %w: %s", notificationErr, strings.TrimSpace(string(notificationOutput))),
		fmt.Errorf("write unified-log alert: %w: %s", logErr, strings.TrimSpace(string(logOutput))),
	)
}
