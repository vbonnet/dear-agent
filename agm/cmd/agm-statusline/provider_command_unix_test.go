//go:build unix

package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigureProviderCommandTerminatesDescendant(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "provider")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
(
  trap '' HUP
  printf 'ready\n' >&3
  sleep 5
  printf 'late\n' >&3
) &
wait $!
`), 0o755); err != nil {
		t.Fatalf("write provider: %v", err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create readiness pipe: %v", err)
	}
	defer reader.Close()

	cmd := exec.Command(script)
	cmd.ExtraFiles = []*os.File{writer}
	configureProviderCommand(cmd)
	if err := cmd.Start(); err != nil {
		writer.Close()
		t.Fatalf("start provider: %v", err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Cancel()
			_ = cmd.Wait()
		}
	}()
	if err := writer.Close(); err != nil {
		t.Fatalf("close parent readiness writer: %v", err)
	}

	if err := reader.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatalf("set readiness deadline: %v", err)
	}
	ready, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		t.Fatalf("wait for provider descendant readiness: %v", err)
	}
	if ready != "ready\n" {
		t.Fatalf("readiness = %q, want %q", ready, "ready\\n")
	}

	started := time.Now()
	if err := cmd.Cancel(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("cancel provider process group: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("canceled provider exited successfully")
	}

	if err := reader.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set cancellation deadline: %v", err)
	}
	remainder, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("provider descendant kept readiness pipe open: %v", err)
	}
	if strings.Contains(string(remainder), "late") {
		t.Fatalf("provider descendant survived cancellation: %q", remainder)
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("provider process-group cancellation took %s, want less than 2s", elapsed)
	}
}
