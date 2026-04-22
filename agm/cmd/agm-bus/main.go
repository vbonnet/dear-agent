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
	fmt.Fprintln(w, "Usage: agm-bus <serve|status|socket> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  serve    Run the broker daemon until SIGINT/SIGTERM.")
	fmt.Fprintln(w, "  status   Print whether a broker is currently responding on the socket.")
	fmt.Fprintln(w, "  socket   Print the effective socket path.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Environment:")
	fmt.Fprintln(w, "  AGM_BUS_SOCKET  Override socket path (default ~/.agm/bus.sock).")
}

//nolint:gocyclo // linear init: queue + ACL + Discord + heartbeat watcher; splitting obscures the flow
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socket := fs.String("socket", "", "unix socket path (overrides AGM_BUS_SOCKET and the default)")
	queueDir := fs.String("queue-dir", "", "offline-message queue dir (default ~/.agm/bus-queue; pass 'off' to disable)")
	aclPath := fs.String("acl", "", "ACL yaml path (default ~/.agm/bus-acl.yaml; pass 'off' for allow-all)")
	verbose := fs.Bool("verbose", false, "enable debug logging")
	discordEnabled := fs.Bool("discord", false, "enable Discord adapter (requires -discord-token or DISCORD_BOT_TOKEN)")
	discordToken := fs.String("discord-token", "", "Discord bot token (default: DISCORD_BOT_TOKEN env var)")
	discordAllowlist := fs.String("discord-allowlist", "", "comma-separated Discord user IDs allowed to DM the bot")
	supervisorsDir := fs.String("supervisors-dir", "",
		"supervisor heartbeat state dir (default ~/.agm/supervisors; pass 'off' to disable the watcher)")
	heartbeatStaleAfter := fs.Duration("heartbeat-stale-after", 5*time.Minute,
		"report a supervisor heartbeat as stale when older than this")
	heartbeatInterval := fs.Duration("heartbeat-scan-interval", 30*time.Second,
		"scan interval for the supervisor heartbeat watcher")
	if err := fs.Parse(args); err != nil {
		return err
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	srv, err := bus.NewServer(*socket, logger)
	if err != nil {
		return err
	}

	// Attach offline queue unless explicitly disabled.
	switch *queueDir {
	case "off":
		logger.Info("offline queue disabled by flag")
	default:
		dir := *queueDir
		if dir == "" {
			dir = defaultQueueDir
		}
		expanded, err := expandHome(dir)
		if err != nil {
			return fmt.Errorf("resolve queue dir: %w", err)
		}
		q, err := bus.NewQueue(expanded)
		if err != nil {
			return fmt.Errorf("init queue: %w", err)
		}
		srv.Queue = q
		logger.Info("offline queue enabled", "dir", expanded)
	}

	// Attach ACL unless explicitly disabled. A missing file is normal for
	// single-user setups — LoadACL returns nil and Check allows all.
	switch *aclPath {
	case "off":
		logger.Info("ACL enforcement disabled by flag")
	default:
		path := *aclPath
		if path == "" {
			path = defaultACLPath
		}
		expanded, err := expandHome(path)
		if err != nil {
			return fmt.Errorf("resolve acl path: %w", err)
		}
		rac, err := bus.NewReloadableACL(expanded)
		if err != nil {
			return fmt.Errorf("load acl: %w", err)
		}
		srv.ACL = rac
		logger.Info("ACL loaded", "path", expanded)
	}

	// Signal-driven shutdown: SIGINT/SIGTERM cancel the context so the
	// server drains connections and removes the socket file. SIGHUP triggers
	// an ACL reload without restarting — handy for policy updates.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if rac, ok := srv.ACL.(*bus.ReloadableACL); ok && rac != nil {
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
	}

	// Discord adapter — opt-in via -discord flag.
	if *discordEnabled {
		token := *discordToken
		if token == "" {
			token = os.Getenv("DISCORD_BOT_TOKEN")
		}
		if token == "" {
			logger.Warn("discord adapter enabled but no token provided (set -discord-token or DISCORD_BOT_TOKEN); Discord disabled")
		} else {
			var allowlist []string
			if *discordAllowlist != "" {
				for _, id := range strings.Split(*discordAllowlist, ",") {
					id = strings.TrimSpace(id)
					if id != "" {
						allowlist = append(allowlist, id)
					}
				}
			}
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
	} else {
		logger.Info("discord adapter disabled (pass -discord to enable)")
	}

	// Supervisor heartbeat watcher — emits heartbeat_stale events onto
	// the bus when a supervisor's heartbeat is older than the threshold.
	// Disabled by -supervisors-dir off so single-session dev runs
	// without a supervisor mesh don't need the watcher.
	switch *supervisorsDir {
	case "off":
		logger.Info("supervisor heartbeat watcher disabled by flag")
	default:
		dir := *supervisorsDir
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
		watcher.StaleAfter = *heartbeatStaleAfter
		watcher.Interval = *heartbeatInterval
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
	}

	return srv.Start(ctx)
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
