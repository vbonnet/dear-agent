//go:build !darwin

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestOverrideAuditNotificationUsesSystemLogSink(t *testing.T) {
	original := runOverrideAuditLogger
	t.Cleanup(func() { runOverrideAuditLogger = original })

	var delivered string
	runOverrideAuditLogger = func(message string) ([]byte, error) {
		delivered = message
		return nil, nil
	}
	if err := sendOverrideAuditNotification("breach details"); err != nil {
		t.Fatalf("sendOverrideAuditNotification() error: %v", err)
	}
	if delivered != "breach details" {
		t.Fatalf("delivered message = %q", delivered)
	}

	runOverrideAuditLogger = func(string) ([]byte, error) {
		return []byte("logger unavailable"), errors.New("exit 1")
	}
	if err := sendOverrideAuditNotification("breach details"); err == nil ||
		!strings.Contains(err.Error(), "logger unavailable") {
		t.Fatalf("delivery error = %v, want logger output", err)
	}
}
