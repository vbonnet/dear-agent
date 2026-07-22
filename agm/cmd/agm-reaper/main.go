package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/reaper"
	pkgversion "github.com/vbonnet/dear-agent/pkg/version"
)

var logger = logging.DefaultLogger()

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run() error {
	sessionName := flag.String("session", "", "Session name to archive")
	logFile := flag.String("log-file", "", "Log file path")
	sessionsDir := flag.String("sessions-dir", "", "Sessions directory")
	force := flag.Bool("force", false, "Override archive verification guards (propagated from agm session archive --force)")
	keepSandbox := flag.Bool("keep-sandbox", false, "Preserve the session sandbox")
	outcome := flag.String("outcome", "", "Archive outcome: completed, crashed, killed, or gc-stale")
	expectedRevision := flag.String("expected-revision", "", "Expected AGM VCS revision for detached archive execution")
	checkRevision := flag.String("check-revision", "", "Verify this binary revision and exit")
	flag.Parse()

	pkgversion.PopulateFromBuildInfo()
	actualRevision := pkgversion.Short()
	if *checkRevision != "" {
		return validateRevision(*checkRevision, actualRevision)
	}
	if *expectedRevision != "" {
		if err := validateRevision(*expectedRevision, actualRevision); err != nil {
			return err
		}
	}

	// Validate required flags
	if *sessionName == "" {
		flag.Usage()
		return fmt.Errorf("--session flag is required")
	}

	// Set up logging
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open log file %s: %w", *logFile, err)
		}
		defer func() { _ = f.Close() }()
		// Create logger with file output
		opts := &slog.HandlerOptions{Level: slog.LevelInfo}
		logger = slog.New(slog.NewTextHandler(f, opts))
		slog.SetDefault(logger)
	}

	// Log startup
	logger.Info("Reaper started", "timestamp", time.Now().UTC().Format(time.RFC3339))
	logger.Info("Reaper configuration", "session", *sessionName, "pid", os.Getpid(), "log_file", *logFile, "sessions_dir", *sessionsDir)

	// Create and run reaper
	r := reaper.NewWithOptions(*sessionName, *sessionsDir, reaper.ArchiveOptions{
		Force:       *force,
		KeepSandbox: *keepSandbox,
		Outcome:     manifest.SessionOutcome(*outcome),
	})
	if err := r.Run(); err != nil {
		return fmt.Errorf("reaper failed: %w", err)
	}

	logger.Info("Reaper completed successfully")
	return nil
}

func validateRevision(expected, actual string) error {
	expected = normalizeRevision(expected)
	actual = normalizeRevision(actual)
	if expected == "" || expected == "unknown" {
		return fmt.Errorf("expected AGM revision is unavailable")
	}
	if actual == "" || actual == "unknown" {
		return fmt.Errorf("agm-reaper has no embedded VCS revision; expected %s", expected)
	}
	if expected != actual {
		return fmt.Errorf("agm-reaper revision %s does not match AGM revision %s", actual, expected)
	}
	return nil
}

func normalizeRevision(revision string) string {
	revision = strings.TrimSuffix(revision, "-dirty")
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return revision
}
