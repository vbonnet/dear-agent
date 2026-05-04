package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/api"
	"github.com/vbonnet/dear-agent/pkg/gateway"
	gwhttp "github.com/vbonnet/dear-agent/pkg/gateway/adapters/http"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// fixture is the smallest viable wiring: a runs.db, an api.Server, and
// a gateway whose CmdRun handler is a stub that records what it sees.
type fixture struct {
	srv     *api.Server
	ts      *httptest.Server
	state   *workflow.SQLiteState
	gw      *gateway.Gateway
	stubRun *stubRunHandler
}

type stubRunHandler struct {
	last   gateway.Command
	resp   gateway.Response
	called int
}

func (s *stubRunHandler) handle(_ context.Context, cmd gateway.Command) gateway.Response {
	s.called++
	s.last = cmd
	if s.resp.Body == nil && s.resp.Err == nil {
		return gateway.Response{Body: map[string]any{"run_id": "r1", "pid": 4242}}
	}
	return s.resp
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	state, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() })

	stub := &stubRunHandler{}
	gw := gateway.New(gateway.HandlerSet{Run: stub.handle})

	srv := api.New(api.Server{
		RunsDB:     state.DB(),
		Identifier: api.AnonymousIdentifier("alice"),
		Version:    "test",
	})
	a := gwhttp.Wrap(srv, gw)
	if a.Name() != "http" {
		t.Fatalf("Name(): got %q want http", a.Name())
	}
	if a.Server() != srv {
		t.Fatalf("Server() must return the wrapped *api.Server")
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &fixture{srv: srv, ts: ts, state: state, gw: gw, stubRun: stub}
}

func TestWrap_RoutesPostRunThroughGateway(t *testing.T) {
	f := newFixture(t)

	body := bytes.NewBufferString(`{"file":"wf.yaml","inputs":{"k":"v"}}`)
	resp, err := http.Post(f.ts.URL+"/run", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status: got %d want 202", resp.StatusCode)
	}
	if f.stubRun.called != 1 {
		t.Fatalf("handler called %d times, want 1", f.stubRun.called)
	}
	if got := f.stubRun.last.Type; got != gateway.CmdRun {
		t.Errorf("Type: got %q want %q", got, gateway.CmdRun)
	}
	if got := f.stubRun.last.Caller.LoginName; got != "alice" {
		t.Errorf("Caller: got %q want alice", got)
	}
	if got, _ := f.stubRun.last.Args["file"].(string); got != "wf.yaml" {
		t.Errorf("Args[file]: got %q", got)
	}
	inputs, _ := f.stubRun.last.Args["inputs"].(map[string]string)
	if inputs == nil || inputs["k"] != "v" {
		t.Errorf("Args[inputs]: got %+v", f.stubRun.last.Args["inputs"])
	}

	var got struct {
		PID int `json:"pid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PID != 4242 {
		t.Errorf("pid: got %d want 4242", got.PID)
	}
}

func TestWrap_GatewayErrorBecomes500(t *testing.T) {
	f := newFixture(t)
	f.stubRun.resp = gateway.Response{Err: gateway.Errorf(gateway.CodeInternal, "kaboom")}

	body := bytes.NewBufferString(`{"file":"wf.yaml"}`)
	resp, err := http.Post(f.ts.URL+"/run", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d want 500", resp.StatusCode)
	}
}

func TestWrap_PanicsOnNilArgs(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Wrap(nil, gw) should panic")
		}
	}()
	gwhttp.Wrap(nil, gateway.New(gateway.HandlerSet{}))
}

func TestWrap_PanicsOnNilGateway(t *testing.T) {
	srv := api.New(api.Server{})
	defer func() {
		if r := recover(); r == nil {
			t.Error("Wrap(srv, nil) should panic")
		}
	}()
	gwhttp.Wrap(srv, nil)
}

func TestRun_BlocksUntilContextCancelled(t *testing.T) {
	f := newFixture(t)
	a := gwhttp.Wrap(f.srv, f.gw)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx, f.gw) }()

	select {
	case err := <-done:
		t.Fatalf("Run returned before cancel: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run after cancel: got %v want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
