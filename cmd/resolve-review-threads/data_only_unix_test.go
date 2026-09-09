//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLoadReplyBodyRejectsNamedFIFOWithoutOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reply.fifo")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("create reply FIFO: %v", err)
	}

	if _, err := loadReplyBody(path, nil); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("named FIFO error = %v, want regular-file refusal", err)
	}
}

func TestLoadReplyBodyRejectsNamedDeviceWithoutReading(t *testing.T) {
	if _, err := loadReplyBody("/dev/zero", nil); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("named device error = %v, want regular-file refusal", err)
	}
}
