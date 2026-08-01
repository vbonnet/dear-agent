//go:build !darwin

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

var runOverrideAuditLogger = func(message string) ([]byte, error) {
	// #nosec G204 -- executable and options are fixed; message is one argv value.
	return exec.Command(
		"/usr/bin/logger", "-t", "dear-agent-override-audit", "--", message,
	).CombinedOutput()
}

func sendOverrideAuditNotification(message string) error {
	output, err := runOverrideAuditLogger(message)
	if err != nil {
		return fmt.Errorf("deliver override alert to the system log: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
