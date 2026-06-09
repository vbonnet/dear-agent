package synchub

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// startTestServer builds a hub + server pair bound to an ephemeral
// localhost port, returns a configured Client and a cleanup hook.
func startTestServer(t *testing.T) (*Hub, *Server, *Client) {
	t.Helper()
	h := newTestHub(t, Options{SessionID: "srv-test"})
	tokenFile := filepath.Join(t.TempDir(), "synchub.token")
	srv, err := NewServer(h, ServerOptions{
		Listen:    "127.0.0.1:0",
		TokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Close(context.Background())
	})
	client, err := NewClientFromTokenFile(tokenFile)
	if err != nil {
		t.Fatalf("NewClientFromTokenFile: %v", err)
	}
	return h, srv, client
}

// TestServer_RoundTrip: full Q&A + lock round trip over HTTP.
func TestServer_RoundTrip(t *testing.T) {
	_, _, client := startTestServer(t)
	ctx := context.Background()

	qid, err := client.AskQuestion(ctx, "color?")
	if err != nil {
		t.Fatalf("AskQuestion: %v", err)
	}
	ans, err := client.Answer(ctx, qid, SurfaceDiscord, []byte("blue"))
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if ans.Winner != SurfaceDiscord {
		t.Errorf("winner=%s", ans.Winner)
	}

	hdl, err := client.Acquire(ctx, "input", "discord-bot", AcquireOptions{})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := hdl.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

// TestServer_FirstWriterWinsAcrossClients: two independent HTTP clients
// race to answer the same round. Exactly one wins; the other gets a
// ClosedError-typed remote error.
func TestServer_FirstWriterWinsAcrossClients(t *testing.T) {
	_, _, c1 := startTestServer(t)
	// Second client reads same token file as c1.
	c2 := *c1

	ctx := context.Background()
	qid, _ := c1.AskQuestion(ctx, "?")

	var wins int32
	var loserErr error
	var mu sync.Mutex
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, cli := range []*Client{c1, &c2} {
		cli := cli
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := cli.Answer(ctx, qid, SurfaceTerminal, []byte("x"))
			if err == nil {
				atomic.AddInt32(&wins, 1)
				return
			}
			mu.Lock()
			loserErr = err
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("wins=%d, want 1", wins)
	}
	if !errors.Is(loserErr, ErrClosed) {
		t.Fatalf("loser err=%v should satisfy errors.Is(ErrClosed)", loserErr)
	}
}

// TestServer_RequiresAuth: missing or wrong token returns 401.
func TestServer_RequiresAuth(t *testing.T) {
	_, srv, _ := startTestServer(t)
	resp, err := http.Get("http://" + srv.Addr() + "/v1/qa/ask")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no auth: status=%d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, "http://"+srv.Addr()+"/v1/qa/ask", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: status=%d, want 401", resp.StatusCode)
	}
}

// TestServer_RejectsPublicBind: 0.0.0.0 is denied at constructor.
func TestServer_RejectsPublicBind(t *testing.T) {
	h := newTestHub(t, Options{SessionID: "x"})
	for _, addr := range []string{"0.0.0.0:0", ":0", "::"} {
		if _, err := NewServer(h, ServerOptions{Listen: addr, TokenFile: filepath.Join(t.TempDir(), "tok")}); err == nil {
			t.Errorf("NewServer accepted public bind %q", addr)
		}
	}
}

// TestServer_NotFoundErrorMapping: 404 from server -> ErrNotFound on
// client.
func TestServer_NotFoundErrorMapping(t *testing.T) {
	_, _, client := startTestServer(t)
	_, err := client.Answer(context.Background(), QuestionID("q-bogus"), SurfaceTerminal, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v, want ErrNotFound", err)
	}
}

// TestServer_LockReleaseSurvivesAutoTimeout: client Acquires with a
// short auto-release deadline, server auto-releases it, client Release
// is a no-op (200 OK, no panic).
func TestServer_LockReleaseSurvivesAutoTimeout(t *testing.T) {
	_, _, client := startTestServer(t)
	ctx := context.Background()
	hdl, err := client.Acquire(ctx, "input", "h1", AcquireOptions{Deadline: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	// Server has auto-released; client Release must not error.
	if err := hdl.Release(ctx); err != nil {
		t.Fatalf("stale client Release: %v", err)
	}
}

// TestServer_TokenFilePermissions verifies the token file is 0600. A
// process running as another local user must not be able to read the
// session's hub token off disk.
func TestServer_TokenFilePermissions(t *testing.T) {
	_, srv, _ := startTestServer(t)
	info, err := os.Stat(srv.opts.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		t.Fatalf("token file mode=%o, want no group/other bits", mode)
	}
}
