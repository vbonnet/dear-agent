package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/gateway"
	"github.com/vbonnet/dear-agent/pkg/gateway/adapters/cli"
)

// pipeReader is a thread-safe reader that hands the adapter one line at
// a time on demand. Tests push commands and EOF the reader to shut down
// the adapter.
type pipeReader struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func newPipeReader() *pipeReader {
	r, w := io.Pipe()
	return &pipeReader{r: r, w: w}
}

func (p *pipeReader) write(s string) {
	_, _ = p.w.Write([]byte(s))
}

func (p *pipeReader) close()                                { _ = p.w.Close() }
func (p *pipeReader) Read(b []byte) (int, error)            { return p.r.Read(b) }

// safeBuffer is a bytes.Buffer guarded by a mutex; the adapter writes
// concurrently with the test reading.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func runAdapter(t *testing.T, in io.Reader, out io.Writer, gw *gateway.Gateway) chan error {
	t.Helper()
	a := cli.New(in, out, gateway.Caller{LoginName: "alice"})
	if a.Name() != "cli" {
		t.Fatalf("Name(): got %q want %q", a.Name(), "cli")
	}
	done := make(chan error, 1)
	go func() {
		done <- a.Run(context.Background(), gw)
	}()
	return done
}

func TestAdapter_DispatchesEachLineAsCommand(t *testing.T) {
	gw := gateway.New(gateway.HandlerSet{
		Status: func(_ context.Context, cmd gateway.Command) gateway.Response {
			return gateway.Response{CommandID: cmd.ID, Body: map[string]any{
				"caller": cmd.Caller.LoginName,
				"run_id": cmd.Args["run_id"],
			}}
		},
	})

	in := newPipeReader()
	out := &safeBuffer{}
	done := runAdapter(t, in, out, gw)

	in.write(`{"id":"1","type":"status","args":{"run_id":"r1"}}` + "\n")
	in.write(`{"id":"2","type":"status","args":{"run_id":"r2"}}` + "\n")
	in.close()

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), out.String())
	}
	for i, line := range lines {
		var resp gateway.Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("line %d decode: %v: %q", i, err, line)
		}
		if resp.Err != nil {
			t.Errorf("line %d: err %+v", i, resp.Err)
		}
		if resp.CommandID != fmt.Sprintf("%d", i+1) {
			t.Errorf("line %d: CommandID %q", i, resp.CommandID)
		}
		if resp.Body["caller"] != "alice" {
			t.Errorf("line %d: caller %q (want alice)", i, resp.Body["caller"])
		}
	}
}

func TestAdapter_StampsDefaultCallerWhenAbsent(t *testing.T) {
	got := make(chan gateway.Caller, 1)
	gw := gateway.New(gateway.HandlerSet{
		Status: func(_ context.Context, cmd gateway.Command) gateway.Response {
			got <- cmd.Caller
			return gateway.Response{CommandID: cmd.ID}
		},
	})

	in := newPipeReader()
	out := &safeBuffer{}
	done := runAdapter(t, in, out, gw)

	in.write(`{"id":"1","type":"status"}` + "\n")
	in.close()

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	caller := <-got
	if caller.LoginName != "alice" {
		t.Errorf("caller: got %q want alice", caller.LoginName)
	}
}

func TestAdapter_PreservesExplicitCaller(t *testing.T) {
	got := make(chan gateway.Caller, 1)
	gw := gateway.New(gateway.HandlerSet{
		Status: func(_ context.Context, cmd gateway.Command) gateway.Response {
			got <- cmd.Caller
			return gateway.Response{CommandID: cmd.ID}
		},
	})

	in := newPipeReader()
	out := &safeBuffer{}
	done := runAdapter(t, in, out, gw)

	in.write(`{"id":"1","type":"status","caller":{"login_name":"bob"}}` + "\n")
	in.close()

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	caller := <-got
	if caller.LoginName != "bob" {
		t.Errorf("caller: got %q want bob", caller.LoginName)
	}
}

func TestAdapter_SurfacesDecodeErrors(t *testing.T) {
	gw := gateway.New(gateway.HandlerSet{})

	in := newPipeReader()
	out := &safeBuffer{}
	done := runAdapter(t, in, out, gw)

	in.write("not-json\n")
	in.close()

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	line := strings.TrimSpace(out.String())
	var resp gateway.Response
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("decode response: %v: %q", err, line)
	}
	if resp.Err == nil || resp.Err.Code != gateway.CodeInvalidArgs {
		t.Fatalf("want CodeInvalidArgs, got %+v (line=%q)", resp.Err, line)
	}
}

func TestAdapter_SkipsEmptyLines(t *testing.T) {
	called := 0
	gw := gateway.New(gateway.HandlerSet{
		Status: func(_ context.Context, cmd gateway.Command) gateway.Response {
			called++
			return gateway.Response{CommandID: cmd.ID}
		},
	})

	in := newPipeReader()
	out := &safeBuffer{}
	done := runAdapter(t, in, out, gw)

	in.write("\n\n" + `{"id":"1","type":"status"}` + "\n\n")
	in.close()

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called != 1 {
		t.Errorf("handler called %d times, want 1", called)
	}
}

func TestAdapter_StopsOnContextCancel(t *testing.T) {
	gw := gateway.New(gateway.HandlerSet{})

	in := newPipeReader() // never closed
	out := &safeBuffer{}

	a := cli.New(in, out, gateway.Caller{LoginName: "alice"})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- a.Run(ctx, gw) }()

	in.write(`{"id":"1","type":"status"}` + "\n")
	cancel()
	// After cancel, push another line so the scanner wakes up and the
	// adapter's ctx.Err() check fires.
	go func() { in.write(`{"id":"2","type":"status"}` + "\n") }()

	err := <-done
	if err == nil {
		t.Fatal("Run: want non-nil error after cancel, got nil")
	}
}

func TestDefaultCaller_ReturnsNonEmpty(t *testing.T) {
	c := cli.DefaultCaller()
	if c.LoginName == "" {
		t.Error("LoginName empty")
	}
}
