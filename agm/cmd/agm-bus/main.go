// Command agm-bus is the local message broker daemon. It listens on a unix
// socket (default ~/.agm/bus.sock) and routes frames between AGM sessions,
// permission prompts between workers and supervisors, and (via channel
// adapters) messages between sessions and external platforms like Discord.
//
// Usage:
//
//	agm-bus serve            # run the daemon in the foreground
//	agm-bus status           # print whether a daemon is running
//	agm-bus socket           # print the effective socket path and exit
//
// Cancelled with SIGINT or SIGTERM. Clean shutdown removes the socket file.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/bus"
)

// defaultQueueDir is the on-disk location of the offline-message queue
// relative to the user's home. It's co-located with the socket so ops
// can see all broker state in one place.
const defaultQueueDir = "~/.agm/bus-queue"

// defaultACLPath is the canonical ACL YAML. Missing file = allow-all
// (useful for single-user dev); populate it to enforce per-session routing.
const defaultACLPath = "~/.agm/bus-acl.yaml"

// defaultSupervisorsDir is where `agm supervisor heartbeat` writes the
// per-supervisor heartbeat files that the broker's heartbeat watcher
// reads. Keeping these colocated with other broker state under ~/.agm
// means ops sees all the mesh's durable state in one dir.
const defaultSupervisorsDir = "~/.agm/supervisors"

// defaultDiscordAgentsPath is the multi-bot portal config (ADR-028). Holds
// per-agent bot tokens; must be chmod 600 and is gitignored.
const defaultDiscordAgentsPath = "~/.agm/discord-agents.yaml"

// runE signatures match cobra's RunE so we can adopt cobra later without
// reshaping; for now, keep dependencies minimal and use stdlib flag.
type runE func(args []string) error

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var fn runE
	switch cmd {
	case "serve":
		fn = cmdServe
	case "status":
		fn = cmdStatus
	case "socket":
		fn = cmdSocket
	case "discord-reset":
		fn = cmdDiscordReset
	case "-h", "--help", "help":
		usage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "agm-bus: unknown subcommand %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
	if err := fn(args); err != nil {
		fmt.Fprintf(os.Stderr, "agm-bus: %v\n", err)
		os.Exit(1)
	}
}

func usage(w *os.File) {
	_, _ = fmt.Fprintln(w, "Usage: agm-bus <serve|status|socket> [flags]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "  serve          Run the broker daemon until SIGINT/SIGTERM.")
	_, _ = fmt.Fprintln(w, "  status         Print whether a broker is currently responding on the socket.")
	_, _ = fmt.Fprintln(w, "  socket         Print the effective socket path.")
	_, _ = fmt.Fprintln(w, "  discord-reset  Purge one Discord channel (gated; requires --confirm).")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Environment:")
	_, _ = fmt.Fprintln(w, "  AGM_BUS_SOCKET  Override socket path (default ~/.agm/bus.sock).")
}

// serveOptions is the parsed shape of the `agm-bus serve` flag set. Keeping
// the parse separate from the wiring is what lets the daemon's setup be
// exercised in-process; the previous single cmdServe body could only be
// reached by building the binary and running it as a subprocess, which is why
// this package read as 0% covered.
type serveOptions struct {
	socket   string
	queueDir string
	aclPath  string
	verbose  bool

	discordEnabled    bool
	discordToken      string
	discordAllowlist  string
	discordMultibot   bool
	discordAgentsPath string

	matrixEnabled    bool
	matrixHomeserver string
	matrixToken      string
	matrixUserID     string
	matrixRoomID     string
	matrixAllowlist  string

	supervisorsDir      string
	heartbeatStaleAfter time.Duration
	heartbeatInterval   time.Duration
}

// parseServeFlags parses the serve flag set, writing usage errors to out.
func parseServeFlags(args []string, out io.Writer) (*serveOptions, error) {
	var opts serveOptions

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&opts.socket, "socket", "", "unix socket path (overrides AGM_BUS_SOCKET and the default)")
	fs.StringVar(&opts.queueDir, "queue-dir", "", "offline-message queue dir (default ~/.agm/bus-queue; pass 'off' to disable)")
	fs.StringVar(&opts.aclPath, "acl", "", "ACL yaml path (default ~/.agm/bus-acl.yaml; pass 'off' for allow-all)")
	fs.BoolVar(&opts.verbose, "verbose", false, "enable debug logging")
	fs.BoolVar(&opts.discordEnabled, "discord", false, "enable Discord adapter (requires -discord-token or DISCORD_BOT_TOKEN)")
	fs.StringVar(&opts.discordToken, "discord-token", "", "Discord bot token (default: DISCORD_BOT_TOKEN env var)")
	fs.StringVar(&opts.discordAllowlist, "discord-allowlist", "", "comma-separated Discord user IDs allowed to DM the bot")
	fs.BoolVar(&opts.discordMultibot, "discord-multibot", false, "enable the multi-bot Discord portal (ADR-028; requires -discord-agents config)")
	fs.StringVar(&opts.discordAgentsPath, "discord-agents", "", "multi-bot portal config path (default ~/.agm/discord-agents.yaml)")
	fs.BoolVar(&opts.matrixEnabled, "matrix", false, "enable Matrix adapter (requires -matrix-homeserver, -matrix-token, -matrix-room)")
	fs.StringVar(&opts.matrixHomeserver, "matrix-homeserver", "", "Matrix homeserver URL (default: MATRIX_HOMESERVER env var)")
	fs.StringVar(&opts.matrixToken, "matrix-token", "", "Matrix access token (default: MATRIX_ACCESS_TOKEN env var)")
	fs.StringVar(&opts.matrixUserID, "matrix-user-id", "", "Matrix bot user id, e.g. @agmbus:example.org (default: MATRIX_USER_ID env var)")
	fs.StringVar(&opts.matrixRoomID, "matrix-room", "", "Matrix room id the adapter listens to and posts to (default: MATRIX_ROOM_ID env var)")
	fs.StringVar(&opts.matrixAllowlist, "matrix-allowlist", "", "comma-separated Matrix user ids (mxids) allowed to send")
	fs.StringVar(&opts.supervisorsDir, "supervisors-dir", "",
		"supervisor heartbeat state dir (default ~/.agm/supervisors; pass 'off' to disable the watcher)")
	fs.DurationVar(&opts.heartbeatStaleAfter, "heartbeat-stale-after", 5*time.Minute,
		"report a supervisor heartbeat as stale when older than this")
	fs.DurationVar(&opts.heartbeatInterval, "heartbeat-scan-interval", 30*time.Second,
		"scan interval for the supervisor heartbeat watcher")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &opts, nil
}

// newServeLogger returns the daemon's logger at the verbosity the flags asked
// for.
func newServeLogger(verbose bool, w io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// splitAllowlist turns a comma-separated allowlist flag into ids, dropping
// empty entries so a trailing comma or a stray space cannot admit "".
func splitAllowlist(raw string) []string {
	if raw == "" {
		return nil
	}
	var ids []string
	for id := range strings.SplitSeq(raw, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// buildServer constructs the broker and attaches the optional offline queue
// and ACL. It performs no listening and starts no goroutines, so a test can
// assert on the wiring without binding a socket.
func buildServer(opts *serveOptions, logger *slog.Logger) (*bus.Server, error) {
	srv, err := bus.NewServer(opts.socket, logger)
	if err != nil {
		return nil, err
	}

	// Attach offline queue unless explicitly disabled.
	switch opts.queueDir {
	case "off":
		logger.Info("offline queue disabled by flag")
	default:
		dir := opts.queueDir
		if dir == "" {
			dir = defaultQueueDir
		}
		expanded, err := expandHome(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve queue dir: %w", err)
		}
		q, err := bus.NewQueue(expanded)
		if err != nil {
			return nil, fmt.Errorf("init queue: %w", err)
		}
		srv.Queue = q
		logger.Info("offline queue enabled", "dir", expanded)
	}

	// Attach ACL unless explicitly disabled. A missing file is normal for
	// single-user setups — LoadACL returns nil and Check allows all.
	switch opts.aclPath {
	case "off":
		logger.Info("ACL enforcement disabled by flag")
	default:
		path := opts.aclPath
		if path == "" {
			path = defaultACLPath
		}
		expanded, err := expandHome(path)
		if err != nil {
			return nil, fmt.Errorf("resolve acl path: %w", err)
		}
		rac, err := bus.NewReloadableACL(expanded)
		if err != nil {
			return nil, fmt.Errorf("load acl: %w", err)
		}
		srv.ACL = rac
		logger.Info("ACL loaded", "path", expanded)
	}

	return srv, nil
}

// startDiscordAdapter starts the single-bot DM adapter when it is enabled and
// a token is available. A missing token degrades Discord routing rather than
// stopping the broker.
func startDiscordAdapter(ctx context.Context, opts *serveOptions, srv *bus.Server, logger *slog.Logger) {
	if !opts.discordEnabled {
		logger.Info("discord adapter disabled (pass -discord to enable)")
		return
	}

	token := opts.discordToken
	if token == "" {
		token = os.Getenv("DISCORD_BOT_TOKEN")
	}
	if token == "" {
		logger.Warn("discord adapter enabled but no token provided (set -discord-token or DISCORD_BOT_TOKEN); Discord disabled")
		return
	}

	allowlist := splitAllowlist(opts.discordAllowlist)
	adapter := &bus.DiscordAdapter{
		Token:     "Bot " + token,
		Registry:  srv.Registry,
		ACL:       srv.ACL,
		Logger:    logger,
		Allowlist: allowlist,
	}
	go func() {
		if err := adapter.Start(ctx); err != nil {
			logger.Error("discord adapter stopped with error", "err", err)
		}
	}()
	logger.Info("discord adapter starting", "users", len(allowlist))
}

// startMultiBotPortal starts the multi-bot Discord portal (ADR-028). It is
// independent of the single-bot DM adapter and can run alongside it. A config
// that cannot be loaded disables the portal rather than the broker; only an
// unresolvable config path is fatal.
func startMultiBotPortal(ctx context.Context, opts *serveOptions, srv *bus.Server, logger *slog.Logger) error {
	if !opts.discordMultibot {
		logger.Info("discord-multibot portal disabled (pass -discord-multibot to enable)")
		return nil
	}

	path := opts.discordAgentsPath
	if path == "" {
		path = defaultDiscordAgentsPath
	}
	expanded, err := expandHome(path)
	if err != nil {
		return fmt.Errorf("resolve discord-agents path: %w", err)
	}

	cfg, loose, err := bus.LoadDiscordAgentsConfig(expanded)
	if err != nil {
		logger.Error("discord-multibot config error; portal disabled", "err", err)
		return nil
	}
	if loose {
		logger.Warn("discord-multibot: config is group/other-readable; chmod 600 it", "path", expanded)
	}

	portal := &bus.MultiBotDiscordAdapter{
		Agents:          cfg.ToAgents(),
		ChannelID:       cfg.Channel,
		GuildID:         cfg.Guild,
		AuthorAllowlist: cfg.AuthorAllowlist,
		Registry:        srv.Registry,
		ACL:             srv.ACL,
		Logger:          logger,
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("discord-multibot portal panicked", "panic", r)
			}
		}()
		if err := portal.Start(ctx); err != nil {
			logger.Error("discord-multibot portal stopped with error", "err", err)
		}
	}()
	logger.Info("discord-multibot portal starting",
		"agents", len(cfg.Agents), "channel", cfg.Channel)
	return nil
}

// startMatrixAdapter starts the Matrix adapter when it is enabled and fully
// configured. It plays the same role as Discord but against a Matrix
// homeserver, and supports Google Chat via mautrix-googlechat bridging since
// bridged users appear as regular Matrix users in the configured room. Any
// missing connection setting degrades Matrix routing, never the broker.
func startMatrixAdapter(ctx context.Context, opts *serveOptions, srv *bus.Server, logger *slog.Logger) {
	if !opts.matrixEnabled {
		logger.Info("matrix adapter disabled (pass -matrix to enable)")
		return
	}

	homeserver := opts.matrixHomeserver
	if homeserver == "" {
		homeserver = os.Getenv("MATRIX_HOMESERVER")
	}
	token := opts.matrixToken
	if token == "" {
		token = os.Getenv("MATRIX_ACCESS_TOKEN")
	}
	userID := opts.matrixUserID
	if userID == "" {
		userID = os.Getenv("MATRIX_USER_ID")
	}
	roomID := opts.matrixRoomID
	if roomID == "" {
		roomID = os.Getenv("MATRIX_ROOM_ID")
	}

	switch {
	case homeserver == "":
		logger.Warn("matrix enabled but no homeserver (set -matrix-homeserver or MATRIX_HOMESERVER); Matrix disabled")
		return
	case token == "":
		logger.Warn("matrix enabled but no token (set -matrix-token or MATRIX_ACCESS_TOKEN); Matrix disabled")
		return
	case roomID == "":
		logger.Warn("matrix enabled but no room (set -matrix-room or MATRIX_ROOM_ID); Matrix disabled")
		return
	}

	allowlist := splitAllowlist(opts.matrixAllowlist)
	adapter := &bus.MatrixAdapter{
		HomeserverURL: homeserver,
		AccessToken:   token,
		UserID:        userID,
		RoomID:        roomID,
		Allowlist:     allowlist,
		Registry:      srv.Registry,
		ACL:           srv.ACL,
		Logger:        logger,
	}
	go func() {
		if err := adapter.Start(ctx); err != nil {
			logger.Error("matrix adapter stopped with error", "err", err)
		}
	}()
	logger.Info("matrix adapter starting", "room", roomID, "users", len(allowlist))
}

// startHeartbeatWatcher starts the supervisor heartbeat watcher, which emits
// heartbeat_stale events onto the bus when a supervisor's heartbeat is older
// than the threshold. Disabled by -supervisors-dir off so single-session dev
// runs without a supervisor mesh do not need the watcher.
func startHeartbeatWatcher(ctx context.Context, opts *serveOptions, srv *bus.Server, logger *slog.Logger) error {
	if opts.supervisorsDir == "off" {
		logger.Info("supervisor heartbeat watcher disabled by flag")
		return nil
	}

	dir := opts.supervisorsDir
	if dir == "" {
		dir = defaultSupervisorsDir
	}
	expanded, err := expandHome(dir)
	if err != nil {
		return fmt.Errorf("resolve supervisors dir: %w", err)
	}

	em := bus.NewEmitter("agm-bus-daemon")
	em.SocketPath = srv.SocketPath
	watcher := bus.NewSupervisorHeartbeatWatcher(expanded, em)
	watcher.StaleAfter = opts.heartbeatStaleAfter
	watcher.Interval = opts.heartbeatInterval
	watcher.Logger = logger
	go func() {
		if err := watcher.Run(ctx); err != nil {
			logger.Error("supervisor heartbeat watcher stopped with error", "err", err)
		}
	}()
	logger.Info("supervisor heartbeat watcher started",
		"dir", expanded,
		"stale_after", watcher.StaleAfter,
		"interval", watcher.Interval)
	return nil
}

// startAdapters brings up every optional adapter for a configured server.
func startAdapters(ctx context.Context, opts *serveOptions, srv *bus.Server, logger *slog.Logger) error {
	startDiscordAdapter(ctx, opts, srv, logger)
	if err := startMultiBotPortal(ctx, opts, srv, logger); err != nil {
		return err
	}
	startMatrixAdapter(ctx, opts, srv, logger)
	return startHeartbeatWatcher(ctx, opts, srv, logger)
}

// serve runs the broker for the lifetime of ctx: it builds the server, brings
// up the optional adapters, installs the SIGHUP ACL reloader, and blocks in
// Start until ctx is cancelled.
//
// This is the whole daemon orchestration and the only copy of it. cmdServe
// supplies a signal-cancelled context and calls straight through, so a test
// driving serve with its own context exercises the same ordering production
// uses. An earlier revision had cmdServe repeat these steps independently,
// which let the two drift and would have kept the in-process tests green
// through a production reordering.
func serve(ctx context.Context, opts *serveOptions, logger *slog.Logger) error {
	srv, err := buildServer(opts, logger)
	if err != nil {
		return err
	}

	stopACLWatch := watchACLReload(srv, logger)
	defer stopACLWatch()

	if err := startAdapters(ctx, opts, srv, logger); err != nil {
		return err
	}
	return srv.Start(ctx)
}

// watchACLReload reloads a reloadable ACL on SIGHUP so policy updates do not
// require a broker restart. It returns a stop function the caller must invoke
// to unregister the handler.
func watchACLReload(srv *bus.Server, logger *slog.Logger) func() {
	rac, ok := srv.ACL.(*bus.ReloadableACL)
	if !ok || rac == nil {
		return func() {}
	}

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			if err := rac.Reload(); err != nil {
				logger.Warn("acl reload failed, keeping previous policy", "err", err)
			} else {
				logger.Info("acl reloaded", "path", rac.Path)
			}
		}
	}()
	return func() {
		signal.Stop(hup)
		close(hup)
	}
}

func cmdServe(args []string) error {
	opts, err := parseServeFlags(args, os.Stderr)
	if err != nil {
		return err
	}
	logger := newServeLogger(opts.verbose, os.Stderr)

	// Signal-driven shutdown: SIGINT/SIGTERM cancel the context so the
	// server drains connections and removes the socket file. SIGHUP is
	// handled inside serve, which reloads the ACL without restarting.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return serve(ctx, opts, logger)
}

// expandHome replicates bus.expandHome (unexported) so the CLI can resolve
// ~/ paths before handing them to the library. Duplication is minimal; the
// alternative (exporting bus.expandHome) leaks an implementation detail.
func expandHome(path string) (string, error) {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// cmdStatus probes the socket by Dialing it with a short timeout. A success
// means a broker is accepting connections; Dial failure (including socket
// missing) means no broker is live.
func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socket := fs.String("socket", "", "unix socket path (overrides AGM_BUS_SOCKET and the default)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := *socket
	if path == "" {
		p, err := bus.SocketPath()
		if err != nil {
			return err
		}
		path = p
	}

	if _, err := os.Stat(path); err != nil {
		// Reporting "not running" IS the successful outcome for the
		// status subcommand when the socket is absent, so we intentionally
		// swallow the Stat error and return nil.
		fmt.Printf("agm-bus: not running (socket %s does not exist)\n", path)
		return nil //nolint:nilerr // "not running" is a normal status outcome
	}

	d := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := d.Dial("unix", path)
	if err != nil {
		fmt.Printf("agm-bus: socket present but not accepting: %v\n", err)
		return nil
	}
	_ = conn.Close()
	fmt.Printf("agm-bus: listening on %s\n", path)
	return nil
}

func cmdSocket(args []string) error {
	fs := flag.NewFlagSet("socket", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := bus.SocketPath()
	if err != nil {
		return err
	}
	fmt.Println(p)
	return nil
}

// cmdDiscordReset purges a single Discord channel. Destructive and explicitly
// gated: without -confirm it only prints what it would do (dry run). It is
// never invoked from serve. The acting bot (default: first agent in the
// portal config) must have MANAGE_MESSAGES in the target channel.
func cmdDiscordReset(args []string) error {
	fs := flag.NewFlagSet("discord-reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	agentsPath := fs.String("agents", "", "portal config path (default ~/.agm/discord-agents.yaml)")
	agentName := fs.String("agent", "", "which configured agent's bot to act as (default: first)")
	channelOverride := fs.String("channel", "", "channel id to purge (default: config 'channel')")
	confirm := fs.Bool("confirm", false, "actually delete; without this it is a dry run")
	verbose := fs.Bool("verbose", false, "debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	path := *agentsPath
	if path == "" {
		path = defaultDiscordAgentsPath
	}
	expanded, err := expandHome(path)
	if err != nil {
		return fmt.Errorf("resolve agents path: %w", err)
	}
	cfg, _, err := bus.LoadDiscordAgentsConfig(expanded)
	if err != nil {
		return err
	}

	// Pick the acting bot token.
	token := ""
	actor := ""
	for _, a := range cfg.Agents {
		if *agentName == "" || a.Name == *agentName {
			token, actor = a.Token, a.Name
			break
		}
	}
	if token == "" {
		return fmt.Errorf("discord-reset: agent %q not found in config", *agentName)
	}

	channelID := cfg.Channel
	if *channelOverride != "" {
		channelID = *channelOverride
	}

	if !*confirm {
		fmt.Printf("DRY RUN: would purge ALL messages in channel %s acting as bot %q.\n", channelID, actor)
		fmt.Println("Re-run with -confirm to actually delete. This is irreversible.")
		return nil
	}

	n, err := bus.ResetChannelByToken(token, channelID, logger)
	fmt.Printf("discord-reset: deleted %d message(s) from channel %s\n", n, channelID)
	return err
}
