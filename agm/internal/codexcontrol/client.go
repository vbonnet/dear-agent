// Package codexcontrol talks to the local Codex app-server control surface.
package codexcontrol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

const defaultTimeout = 20 * time.Second

var execCommandContext = exec.CommandContext

// Client controls Codex threads through `codex app-server proxy`.
type Client struct {
	CodexPath string
	Timeout   time.Duration
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
	cmd.WaitDelay = 1 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if output, err := cmd.Output(); err != nil {
		if timeoutCtx.Err() != nil {
			return fmt.Errorf("codex remote-control start timed out after %s", c.timeout())
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

	enc := json.NewEncoder(stdin)
	dec := json.NewDecoder(bufio.NewReader(stdout))
	if err := enc.Encode(rpcRequest{
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
	if err := readResponse(dec, 1, nil); err != nil {
		return addProxyStderr(err, stderr.String())
	}
	if err := enc.Encode(rpcNotification{Method: "initialized", Params: map[string]any{}}); err != nil {
		return fmt.Errorf("send codex initialized: %w", err)
	}
	if err := enc.Encode(rpcRequest{ID: 2, Method: method, Params: params}); err != nil {
		return fmt.Errorf("send codex %s: %w", method, err)
	}
	if err := readResponse(dec, 2, result); err != nil {
		return addProxyStderr(err, stderr.String())
	}
	return nil
}

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
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
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
