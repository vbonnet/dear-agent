// Package cli is the stdin/stdout adapter for pkg/gateway.
//
// Wire format: one JSON-encoded gateway.Command per line on the input
// reader, one JSON-encoded gateway.Response per line on the output
// writer. The adapter does not own caller identity — it stamps every
// inbound command with the Caller passed at construction time.
//
// Use cases:
//   - A small `dear-agent-gateway-cli` binary that reads one command
//     from argv, marshals it, and pipes through this adapter.
//   - A development REPL.
//   - Tests, which feed a *bytes.Buffer in and assert the output buffer.
//
// The adapter is single-threaded by design: it processes one command at
// a time so output ordering matches input ordering. Callers that need
// concurrency should run multiple adapters against the same Gateway.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/user"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/gateway"
)

// Adapter is the stdin/stdout gateway adapter. Construct with New.
type Adapter struct {
	in     io.Reader
	out    io.Writer
	caller gateway.Caller

	mu sync.Mutex // serialises writes to out
}

// New constructs an Adapter. caller is stamped onto every Command the
// adapter dispatches; pass an explicit Caller for deterministic tests
// or use DefaultCaller() to derive one from os/user.
func New(in io.Reader, out io.Writer, caller gateway.Caller) *Adapter {
	return &Adapter{in: in, out: out, caller: caller}
}

// Name implements gateway.Adapter.
func (*Adapter) Name() string { return "cli" }

// Run reads one Command per line from the input, dispatches it, and
// writes the resulting Response on its own line. Returns nil when the
// reader hits EOF (the standard "command stream closed" signal) or when
// ctx is cancelled.
//
// On a malformed input line the adapter writes a Response with
// CodeInvalidArgs rather than aborting — staying alive matches what
// every other JSON-RPC server does and keeps long-lived stdio sessions
// usable.
func (a *Adapter) Run(ctx context.Context, gw *gateway.Gateway) error {
	scanner := bufio.NewScanner(a.in)
	// 1 MiB is large enough for any plausible Args payload but bounded
	// so a malicious peer can't OOM us by sending a 4 GiB line.
	const maxLine = 1 << 20
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := a.handleLine(ctx, gw, line); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		// io.EOF is the clean shutdown path; only surface real errors.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("cli adapter: scan: %w", err)
	}
	return nil
}

func (a *Adapter) handleLine(ctx context.Context, gw *gateway.Gateway, line []byte) error {
	var cmd gateway.Command
	if err := json.Unmarshal(line, &cmd); err != nil {
		return a.writeResponse(gateway.Response{
			Err: gateway.WrapError(gateway.CodeInvalidArgs, "decode command", err),
		})
	}
	if cmd.Caller.LoginName == "" {
		cmd.Caller = a.caller
	}
	resp := gw.Dispatch(ctx, cmd)
	return a.writeResponse(resp)
}

func (a *Adapter) writeResponse(resp gateway.Response) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	enc := json.NewEncoder(a.out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("cli adapter: encode response: %w", err)
	}
	return nil
}

// DefaultCaller returns a Caller derived from the current OS user.
// Falls back to a synthetic "cli" caller when os/user.Current fails
// (which happens in some restricted execution environments).
func DefaultCaller() gateway.Caller {
	u, err := user.Current()
	if err != nil || u == nil || u.Username == "" {
		return gateway.Caller{LoginName: "cli", Display: "cli"}
	}
	display := u.Name
	if display == "" {
		display = u.Username
	}
	return gateway.Caller{LoginName: u.Username, Display: display}
}
