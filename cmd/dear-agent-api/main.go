// Command dear-agent-api serves dear-agent's JSON HTTP control surface
// over a Tailscale-internal listener. The API mirrors the workflow- and
// audit-CLI surfaces; see docs/adrs/ADR-013-tailscale-api.md for the
// design.
//
// Usage:
//
//	dear-agent-api --runs-db ./runs.db [--audit-db ./.dear-agent/audit.db] \
//	               [--hostname dear-agent] [--state-dir ./tsnet-state]
//
// By default the binary joins the operator's tailnet using the
// TS_AUTHKEY environment variable (or interactive auth if absent) and
// listens on :443 over TLS. Pass --loopback HOST:PORT to skip tsnet
// and bind to a local address — useful only for development on the
// same host since loopback mode has no authentication.
//
// Exit codes: 0 on clean shutdown (SIGINT/SIGTERM), 1 on startup or
// runtime error.
package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
	"tailscale.com/tsnet"

	"github.com/vbonnet/dear-agent/pkg/api"
	"github.com/vbonnet/dear-agent/pkg/audit"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// flags is the set of command-line flags. Pulled into a struct so run
// stays small enough to read.
type flags struct {
	runsDBPath  string
	auditDBPath string
	hostname    string
	stateDir    string
	loopback    string
	workflowBin string
	workingDir  string
	verbose     bool
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dear-agent-api", flag.ContinueOnError)
	fs.SetOutput(stderr)
	f := flags{}
	fs.StringVar(&f.runsDBPath, "runs-db", "runs.db", "path to the workflow runs database")
	fs.StringVar(&f.auditDBPath, "audit-db", filepath.Join(".dear-agent", "audit.db"), "path to the audit database (empty = disable audit endpoints)")
	fs.StringVar(&f.hostname, "hostname", "dear-agent", "hostname to register on the tailnet")
	fs.StringVar(&f.stateDir, "state-dir", "", "directory for tsnet state (default: ~/.config/dear-agent-api/<hostname>)")
	fs.StringVar(&f.loopback, "loopback", "", "skip tsnet and bind to this address (development only; no auth)")
	fs.StringVar(&f.workflowBin, "workflow-bin", "workflow-run", "path or name of the workflow-run binary used by POST /run")
	fs.StringVar(&f.workingDir, "cwd", "", "working directory passed to spawned workflow-run processes")
	fs.BoolVar(&f.verbose, "verbose", false, "debug logging")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: dear-agent-api [flags]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	level := slog.LevelInfo
	if f.verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	runsDB, err := openSQLite(f.runsDBPath)
	if err != nil {
		fmt.Fprintf(stderr, "open runs db %s: %v\n", f.runsDBPath, err)
		return 1
	}
	defer runsDB.Close()

	var auditStore audit.Store
	if f.auditDBPath != "" {
		s, err := audit.OpenSQLiteStore(f.auditDBPath)
		if err != nil {
			fmt.Fprintf(stderr, "open audit db %s: %v\n", f.auditDBPath, err)
			return 1
		}
		defer s.Close()
		auditStore = s
	}

	srv := api.New(api.Server{
		RunsDB:     runsDB,
		AuditStore: auditStore,
		Identifier: nil, // filled in below
		Runner: &api.ExecRunner{
			Binary:     f.workflowBin,
			WorkingDir: f.workingDir,
			Logger:     logger,
		},
		Logger: logger,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if f.loopback != "" {
		return runLoopback(ctx, logger, srv, f.loopback, stdout, stderr)
	}
	return runTailscale(ctx, logger, srv, f, stdout, stderr)
}

// runLoopback serves the API on a loopback address with no Tailscale
// integration. The Identifier returns a synthetic "loopback" caller
// for every request — only safe on a private interface.
func runLoopback(ctx context.Context, logger *slog.Logger, srv *api.Server, addr string, stdout, stderr io.Writer) int {
	srv.Identifier = api.AnonymousIdentifier("loopback")
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Fprintf(stdout, "dear-agent-api: listening on %s (loopback, no auth)\n", addr)
	return serveAndShutdown(ctx, logger, httpSrv, nil, stderr)
}

// runTailscale brings up a tsnet.Server, plumbs WhoIs into an
// Identifier, and serves the API over HTTPS on :443.
func runTailscale(ctx context.Context, logger *slog.Logger, srv *api.Server, f flags, stdout, stderr io.Writer) int {
	stateDir := f.stateDir
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "resolve home dir: %v\n", err)
			return 1
		}
		stateDir = filepath.Join(home, ".config", "dear-agent-api", f.hostname)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		fmt.Fprintf(stderr, "create state dir %s: %v\n", stateDir, err)
		return 1
	}

	ts := &tsnet.Server{
		Hostname: f.hostname,
		Dir:      stateDir,
		Logf: func(format string, a ...any) {
			if f.verbose {
				logger.Debug("tsnet", "msg", fmt.Sprintf(format, a...))
			}
		},
		AuthKey: os.Getenv("TS_AUTHKEY"),
	}
	defer ts.Close()

	startCtx, startCancel := context.WithTimeout(ctx, 60*time.Second)
	defer startCancel()
	if _, err := ts.Up(startCtx); err != nil {
		fmt.Fprintf(stderr, "bring tsnet up: %v\n", err)
		return 1
	}

	lc, err := ts.LocalClient()
	if err != nil {
		fmt.Fprintf(stderr, "tsnet local client: %v\n", err)
		return 1
	}
	srv.Identifier = api.IdentifierFunc(func(ctx context.Context, r *http.Request) (api.Caller, error) {
		ipport := stripZone(r.RemoteAddr)
		who, err := lc.WhoIs(ctx, ipport)
		if err != nil {
			return api.Caller{}, fmt.Errorf("whois %s: %w", ipport, err)
		}
		if who == nil || who.UserProfile == nil {
			return api.Caller{}, errors.New("no user profile in whois response")
		}
		return api.Caller{
			LoginName: who.UserProfile.LoginName,
			Display:   who.UserProfile.DisplayName,
		}, nil
	})

	ln, err := ts.ListenTLS("tcp", ":443")
	if err != nil {
		fmt.Fprintf(stderr, "listen :443 over tsnet: %v\n", err)
		return 1
	}
	httpSrv := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}

	ip4, ip6 := ts.TailscaleIPs()
	fmt.Fprintf(stdout, "dear-agent-api: tailnet hostname=%q ipv4=%s ipv6=%s\n",
		f.hostname, addrOrDash(ip4), addrOrDash(ip6))
	fmt.Fprintf(stdout, "dear-agent-api: serving HTTPS on tailnet :443\n")

	return serveAndShutdown(ctx, logger, httpSrv, ln, stderr)
}

// serveAndShutdown runs httpSrv until ctx is cancelled, then performs
// a bounded graceful shutdown. Returns the process exit code.
func serveAndShutdown(ctx context.Context, logger *slog.Logger, httpSrv *http.Server, ln net.Listener, stderr io.Writer) int {
	var wg sync.WaitGroup
	wg.Add(1)
	// The shutdown goroutine deliberately uses a fresh detached
	// context after the parent ctx fires: ctx is the cancel signal,
	// and we want a bounded deadline that survives ctx's cancellation.
	go func() { //nolint:gosec // G118: detached shutdown deadline is the intended pattern
		defer wg.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("http shutdown", "err", err)
		}
	}()

	var err error
	if ln != nil {
		err = httpSrv.Serve(ln)
	} else {
		err = httpSrv.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		wg.Wait()
		return 1
	}
	wg.Wait()
	return 0
}

func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	return db, nil
}

// stripZone removes an IPv6 zone identifier from a host:port string,
// because the local API endpoint rejects them. It also handles
// host-only inputs by returning them unchanged.
func stripZone(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}
	return net.JoinHostPort(host, port)
}

func addrOrDash(a netip.Addr) string {
	if !a.IsValid() {
		return "—"
	}
	return a.String()
}
