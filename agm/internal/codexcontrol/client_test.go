package codexcontrol

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCodexProxyHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_PROXY_HELPER") != "1" {
		return
	}

	conn, err := net.Dial("tcp", os.Args[len(os.Args)-1])
	if err != nil {
		os.Exit(1)
	}
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(conn, os.Stdin)
		_ = conn.Close()
		close(copyDone)
	}()
	_, _ = io.Copy(os.Stdout, conn)
	<-copyDone
	os.Exit(0)
}

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

	// Process startup can be delayed well beyond five seconds while the full
	// repository test matrix is saturating the host. Preserve ample separation
	// from the production timeout so this command-failure assertion observes the
	// injected exit status instead of winning a scheduler race with the deadline.
	client := &Client{Timeout: 30 * time.Second}
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

func TestRequestUsesWebSocketProxy(t *testing.T) {
	serverErr := make(chan error, 1)
	report := func(err error) {
		select {
		case serverErr <- err:
		default:
		}
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			report(err)
			return
		}
		defer conn.Close()

		var initialize rpcRequest
		if err := conn.ReadJSON(&initialize); err != nil {
			report(err)
			return
		}
		if initialize.ID != 1 || initialize.Method != "initialize" {
			report(&unexpectedRequestError{got: initialize, wantID: 1, wantMethod: "initialize"})
			return
		}
		if err := conn.WriteJSON(rpcNotification{Method: "remoteControl/status/changed", Params: map[string]any{}}); err != nil {
			report(err)
			return
		}
		if err := conn.WriteJSON(rpcResponse{ID: 1, Result: json.RawMessage(`{}`)}); err != nil {
			report(err)
			return
		}

		var initialized rpcNotification
		if err := conn.ReadJSON(&initialized); err != nil {
			report(err)
			return
		}
		if initialized.Method != "initialized" {
			report(&unexpectedRequestError{got: initialized, wantMethod: "initialized"})
			return
		}

		var list rpcRequest
		if err := conn.ReadJSON(&list); err != nil {
			report(err)
			return
		}
		if list.ID != 2 || list.Method != "thread/list" {
			report(&unexpectedRequestError{got: list, wantID: 2, wantMethod: "thread/list"})
			return
		}
		if err := conn.WriteJSON(rpcResponse{ID: 2, Result: json.RawMessage(`{"data":[{"id":"thr_123"}]}`)}); err != nil {
			report(err)
		}
	}))
	defer server.Close()

	originalExec := execCommandContext
	execCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestCodexProxyHelperProcess", "--", strings.TrimPrefix(server.URL, "http://"))
		cmd.Env = append(os.Environ(), "GO_WANT_CODEX_PROXY_HELPER=1")
		return cmd
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	archived := false
	threads, err := (&Client{Timeout: 5 * time.Second}).ListThreads(context.Background(), ListThreadsOptions{Archived: &archived, Limit: 5})
	if err != nil {
		t.Fatalf("ListThreads returned error: %v", err)
	}
	if len(threads.Data) != 1 || threads.Data[0].ID != "thr_123" {
		t.Fatalf("ListThreads data = %#v, want thread thr_123", threads.Data)
	}
	select {
	case err := <-serverErr:
		t.Fatalf("WebSocket proxy received unexpected request: %v", err)
	default:
	}
}

type unexpectedRequestError struct {
	got        any
	wantID     int
	wantMethod string
}

func (e *unexpectedRequestError) Error() string {
	return "unexpected request"
}
