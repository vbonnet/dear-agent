// Package codexcontrol talks to the local Codex app-server control surface.
package codexcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

const defaultTimeout = 20 * time.Second

var execCommandContext = exec.CommandContext

// Client controls Codex threads through `codex app-server proxy`. The proxy
// bridges raw WebSocket-over-UDS bytes, so the client performs the HTTP Upgrade
// and exchanges JSON-RPC as WebSocket messages over its stdin/stdout pipes.
type Client struct {
	CodexPath string
	Timeout   time.Duration
	waitDelay time.Duration
}

// Thread is the subset of Codex app-server thread metadata AGM needs.
type Thread struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	CWD       string `json:"cwd"`
	Path      string `json:"path"`
	Preview   string `json:"preview"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// StartThreadOptions configures a Codex thread created for an AGM session.
type StartThreadOptions struct {
	CWD   string
	Model string
}

// ListThreadsOptions filters Codex threads for reconciliation.
type ListThreadsOptions struct {
	Archived *bool
	CWD      string
	Limit    int
}

// ThreadList is returned by thread/list.
type ThreadList struct {
	Data       []Thread `json:"data"`
	NextCursor string   `json:"nextCursor"`
}

// New returns a client for the codex binary on PATH.
func New() *Client {
	return &Client{CodexPath: "codex", Timeout: defaultTimeout}
}

// StartRemoteControl ensures the local app-server daemon is running with remote
// control enabled.
func (c *Client) StartRemoteControl(ctx context.Context) error {
	timeoutCtx, cancel := c.timeoutContext(ctx)
	defer cancel()

	cmd := execCommandContext(timeoutCtx, c.codexPath(), "remote-control", "start", "--json")
	// WaitDelay bounds how long Output() blocks on the command's I/O pipes after
	// the process exits or timeoutCtx is cancelled. `codex remote-control start`
	// daemonizes the app-server, which inherits the stdout pipe and holds its
	// write-end open indefinitely; without WaitDelay, cmd.Output() blocks reading
	// that inherited pipe to EOF forever, and the context timeout cannot rescue it
	// (CommandContext only SIGKILLs the direct child, which has already exited
	// after forking the daemon). This was ce-fmxv: `agm session new` hung forever
	// before the harness was ever launched. WaitDelay force-closes the pipes so the
	// call returns and the caller can fall back to the local Codex CLI.
	//
	// Process-group isolation + group-cancel (per repo subprocess-execution
	// guidelines) ensures the daemonized app-server the direct child forked is
	// killed alongside it on cancel, rather than surviving as an orphan holding
	// the inherited pipe open.
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = c.pipeWaitDelay()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if output, err := cmd.Output(); err != nil {
		if timeoutErr := timeoutCtx.Err(); timeoutErr != nil {
			if errors.Is(timeoutErr, context.DeadlineExceeded) {
				return fmt.Errorf("codex remote-control start timed out after %s", c.timeout())
			}
			return fmt.Errorf("codex remote-control start canceled: %w", timeoutErr)
		}
		// A daemonized app-server can keep the stdout pipe open after it has
		// already emitted its successful machine-readable status. In that case
		// WaitDelay makes Output return an error even though startup succeeded.
		// ErrWaitDelay is the narrow, non-cancellation case in which the direct
		// child succeeded but a daemon retained its stdout pipe. Treat only that
		// complete daemon-status response as success; other command failures,
		// stderr-only responses, and malformed responses still surface.
		if errors.Is(err, exec.ErrWaitDelay) && isRemoteControlDaemonStatus(output) {
			return nil
		}
		msg := strings.TrimSpace(stderr.String())
		if out := strings.TrimSpace(string(output)); out != "" {
			if msg != "" {
				msg += ": "
			}
			msg += out
		}
		if msg != "" {
			return fmt.Errorf("codex remote-control start failed: %w: %s", err, msg)
		}
		return fmt.Errorf("codex remote-control start failed: %w", err)
	}
	return nil
}

func (c *Client) pipeWaitDelay() time.Duration {
	if c.waitDelay > 0 {
		return c.waitDelay
	}
	return time.Second
}

func isRemoteControlDaemonStatus(output []byte) bool {
	var status struct {
		Mode   string          `json:"mode"`
		Daemon json.RawMessage `json:"daemon"`
	}
	if err := json.Unmarshal(output, &status); err != nil {
		return false
	}
	return status.Mode == "daemon" && len(status.Daemon) > 0 && string(status.Daemon) != "null"
}

// StartThread creates a new Codex thread.
func (c *Client) StartThread(ctx context.Context, opts StartThreadOptions) (*Thread, error) {
	params := map[string]any{
		"cwd":     opts.CWD,
		"model":   opts.Model,
		"sandbox": "workspace-write",
	}
	var resp struct {
		Thread Thread `json:"thread"`
	}
	if err := c.request(ctx, "thread/start", params, &resp); err != nil {
		return nil, err
	}
	if resp.Thread.ID == "" {
		return nil, errors.New("codex thread/start returned empty thread id")
	}
	return &resp.Thread, nil
}

// SetThreadName sets the user-visible Codex thread name.
func (c *Client) SetThreadName(ctx context.Context, threadID, name string) error {
	if threadID == "" {
		return errors.New("thread id is required")
	}
	return c.request(ctx, "thread/name/set", map[string]any{
		"threadId": threadID,
		"name":     name,
	}, nil)
}

// ArchiveThread archives a Codex thread.
func (c *Client) ArchiveThread(ctx context.Context, threadID string) error {
	if threadID == "" {
		return errors.New("thread id is required")
	}
	return c.request(ctx, "thread/archive", map[string]any{"threadId": threadID}, nil)
}

// ListThreads lists Codex threads.
func (c *Client) ListThreads(ctx context.Context, opts ListThreadsOptions) (*ThreadList, error) {
	params := map[string]any{}
	if opts.Archived != nil {
		params["archived"] = *opts.Archived
	}
	if opts.CWD != "" {
		params["cwd"] = opts.CWD
	}
	if opts.Limit > 0 {
		params["limit"] = opts.Limit
	}
	var resp ThreadList
	if err := c.request(ctx, "thread/list", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) request(ctx context.Context, method string, params any, result any) error {
	timeoutCtx, cancel := c.timeoutContext(ctx)
	defer cancel()

	cmd := execCommandContext(timeoutCtx, c.codexPath(), "app-server", "proxy")
	// ce-fmxv: bound the post-exit / post-cancel I/O wait so a proxy child that
	// leaves an inherited pipe open (e.g. via the shared app-server daemon) cannot
	// hang cmd.Wait() past the context deadline. Process-group isolation +
	// group-cancel matches StartRemoteControl above so no descendant survives
	// cancellation to keep holding the pipe.
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 1 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("create codex app-server stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start codex app-server proxy: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	proxyConn := &proxyPipeConn{reader: stdout, writer: stdin}
	conn, err := dialProxyWebSocket(timeoutCtx, proxyConn, c.timeout())
	if err != nil {
		return addProxyStderr(fmt.Errorf("connect codex app-server WebSocket: %w", err), stderr.String())
	}
	defer func() { _ = conn.Close() }()

	if err := conn.WriteJSON(rpcRequest{
		ID:     1,
		Method: "initialize",
		Params: map[string]any{
			"clientInfo": map[string]string{
				"name":    "agm",
				"title":   "AGM",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{"experimentalApi": true},
		},
	}); err != nil {
		return fmt.Errorf("send codex initialize: %w", err)
	}
	if err := readWebSocketResponse(conn, 1, nil); err != nil {
		return addProxyStderr(err, stderr.String())
	}
	if err := conn.WriteJSON(rpcNotification{Method: "initialized", Params: map[string]any{}}); err != nil {
		return fmt.Errorf("send codex initialized: %w", err)
	}
	if err := conn.WriteJSON(rpcRequest{ID: 2, Method: method, Params: params}); err != nil {
		return fmt.Errorf("send codex %s: %w", method, err)
	}
	if err := readWebSocketResponse(conn, 2, result); err != nil {
		return addProxyStderr(err, stderr.String())
	}
	return nil
}

// dialProxyWebSocket performs the documented WebSocket HTTP Upgrade over the
// raw byte stream provided by `codex app-server proxy`. The proxy owns control
// socket discovery; AGM only supplies a net.Conn facade over the process pipes.
func dialProxyWebSocket(ctx context.Context, raw net.Conn, handshakeTimeout time.Duration) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		Proxy:            nil,
		HandshakeTimeout: handshakeTimeout,
		NetDialContext: func(context.Context, string, string) (net.Conn, error) {
			return raw, nil
		},
	}
	conn, _, err := dialer.DialContext(ctx, "ws://localhost/rpc", nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// proxyPipeConn adapts the proxy process pipes to the net.Conn interface
// Gorilla uses to write the HTTP Upgrade and WebSocket frames. Context-driven
// proxy process cancellation bounds reads because pipes cannot set deadlines.
type proxyPipeConn struct {
	reader io.ReadCloser
	writer io.WriteCloser
	close  sync.Once
}

func (c *proxyPipeConn) Read(p []byte) (int, error)       { return c.reader.Read(p) }
func (c *proxyPipeConn) Write(p []byte) (int, error)      { return c.writer.Write(p) }
func (c *proxyPipeConn) LocalAddr() net.Addr              { return proxyPipeAddr("proxy-stdin") }
func (c *proxyPipeConn) RemoteAddr() net.Addr             { return proxyPipeAddr("codex-app-server") }
func (c *proxyPipeConn) SetDeadline(time.Time) error      { return nil }
func (c *proxyPipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *proxyPipeConn) SetWriteDeadline(time.Time) error { return nil }

func (c *proxyPipeConn) Close() error {
	var closeErr error
	c.close.Do(func() {
		if err := c.writer.Close(); err != nil {
			closeErr = err
		}
		if err := c.reader.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	})
	return closeErr
}

type proxyPipeAddr string

func (a proxyPipeAddr) Network() string { return "codex-app-server-proxy" }
func (a proxyPipeAddr) String() string  { return string(a) }

type rpcRequest struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type rpcNotification struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func readResponse(dec *json.Decoder, id int, result any) error {
	return readResponseMessage(dec.Decode, id, result)
}

func readWebSocketResponse(conn *websocket.Conn, id int, result any) error {
	return readResponseMessage(conn.ReadJSON, id, result)
}

func readResponseMessage(read func(any) error, id int, result any) error {
	for {
		var raw json.RawMessage
		if err := read(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("codex app-server closed before response %d", id)
			}
			return fmt.Errorf("read codex app-server response %d: %w", id, err)
		}
		var probe struct {
			ID *int `json:"id"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil || probe.ID == nil || *probe.ID != id {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("decode codex app-server response %d: %w", id, err)
		}
		if resp.Error != nil {
			return fmt.Errorf("codex app-server error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if result == nil || len(resp.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("decode codex app-server result %d: %w", id, err)
		}
		return nil
	}
}

func (c *Client) codexPath() string {
	if c.CodexPath != "" {
		return c.CodexPath
	}
	return "codex"
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultTimeout
}

func (c *Client) timeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, c.timeout())
}

func addProxyStderr(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, stderr)
}
