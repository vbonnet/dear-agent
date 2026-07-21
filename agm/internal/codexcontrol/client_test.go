package codexcontrol

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartRemoteControlAcceptsDaemonStatusWhenStdoutClosesLate(t *testing.T) {
	script := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"mode":"daemon","daemon":{"status":"alreadyRunning"}}'
(sleep 30) &
`), 0o700); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	originalExec := execCommandContext
	execCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, script)
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	// Keep the pipe-close bound well inside the command timeout. The old test
	// used a one-second production WaitDelay against a five-second timeout;
	// under a fully parallel race suite, scheduler contention could let the
	// deadline win and turn this success-path regression into a false timeout.
	client := &Client{Timeout: 30 * time.Second, waitDelay: 25 * time.Millisecond}
	if err := client.StartRemoteControl(context.Background()); err != nil {
		t.Fatalf("StartRemoteControl returned error for daemon status: %v", err)
	}
}

func TestStartRemoteControlRejectsDaemonStatusAfterContextDeadline(t *testing.T) {
	script := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"mode":"daemon","daemon":{"status":"alreadyRunning"}}'
sleep 2
`), 0o700); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	originalExec := execCommandContext
	execCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, script)
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	client := &Client{Timeout: 50 * time.Millisecond}
	err := client.StartRemoteControl(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("StartRemoteControl error = %v, want timeout", err)
	}
}

func TestStartRemoteControlRejectsDaemonStatusAfterCommandFailure(t *testing.T) {
	script := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"mode":"daemon","daemon":{"status":"alreadyRunning"}}'
exit 1
`), 0o700); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	originalExec := execCommandContext
	execCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, script)
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	client := &Client{Timeout: 5 * time.Second}
	err := client.StartRemoteControl(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("StartRemoteControl error = %v, want command failure", err)
	}
}

func TestIsRemoteControlDaemonStatus(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "daemon", output: `{"mode":"daemon","daemon":{"status":"alreadyRunning"}}`, want: true},
		{name: "missing daemon", output: `{"mode":"daemon"}`, want: false},
		{name: "wrong mode", output: `{"mode":"foreground","daemon":{}}`, want: false},
		{name: "malformed", output: `{`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRemoteControlDaemonStatus([]byte(tt.output)); got != tt.want {
				t.Fatalf("isRemoteControlDaemonStatus(%q) = %t, want %t", tt.output, got, tt.want)
			}
		})
	}
}

func TestReadResponseSkipsNotifications(t *testing.T) {
	input := strings.NewReader(strings.Join([]string{
		`{"method":"thread/started","params":{"thread":{"id":"ignored"}}}`,
		`{"id":2,"result":{"thread":{"id":"thr_123","name":"agm-name","cwd":"/repo"}}}`,
	}, "\n"))

	var got struct {
		Thread Thread `json:"thread"`
	}
	if err := readResponse(json.NewDecoder(input), 2, &got); err != nil {
		t.Fatalf("readResponse returned error: %v", err)
	}
	if got.Thread.ID != "thr_123" {
		t.Fatalf("thread id = %q, want thr_123", got.Thread.ID)
	}
}

func TestReadResponseReturnsRPCError(t *testing.T) {
	input := strings.NewReader(`{"id":2,"error":{"code":-32602,"message":"bad params"}}`)

	err := readResponse(json.NewDecoder(input), 2, nil)
	if err == nil {
		t.Fatal("readResponse returned nil error")
	}
	if !strings.Contains(err.Error(), "bad params") {
		t.Fatalf("error = %q, want bad params", err.Error())
	}
}
