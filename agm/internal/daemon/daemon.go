// Package daemon implements the background delivery daemon for the AI Coordination
// Message System. It polls the message queue, detects session state changes, and
// delivers queued messages when sessions become READY.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/monitor/opencode"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

const (
	// PIDFileName is the name of the PID lock file
	PIDFileName = "daemon.pid"
)

// errDeferred is returned by deliverMessage when a message is deferred (session busy).
var errDeferred = errors.New("message deferred")

// Config holds daemon configuration
type Config struct {
	// BaseDir is the root directory for daemon data (~/.agm)
	BaseDir string

	// LogDir is where daemon logs are written
	LogDir string

	// PIDFile is the full path to the PID file
	PIDFile string

	// Queue is the message queue to monitor
	Queue *messages.MessageQueue

	// AckManager handles message acknowledgments
	AckManager *messages.AckManager

	// Logger is the daemon logger instance
	Logger *slog.Logger

	// EventBus is the event bus for adapter integration
	EventBus *eventbus.Hub

	// AppConfig holds application-level configuration (adapters, etc.)
	AppConfig *config.Config

	// DoltAdapter is the Dolt storage adapter for session resolution
	DoltAdapter *dolt.Adapter
}

// Daemon manages the background message delivery process
type Daemon struct {
	cfg             Config
	ticker          *time.Ticker
	ctx             context.Context
	cancel          context.CancelFunc
	metrics         *MetricsCollector
	alerts          []AlertRule
	opencodeAdapter *opencode.Adapter
}

// NewDaemon creates a new daemon instance with the given configuration
func NewDaemon(cfg Config) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())

	d := &Daemon{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		metrics: NewMetricsCollector(),
		alerts:  GetDefaultAlertRules(),
	}

	// Initialize OpenCode adapter if enabled
	if cfg.AppConfig != nil && cfg.AppConfig.Adapters.OpenCode.Enabled {
		if cfg.EventBus == nil {
			cfg.Logger.Warn("OpenCode adapter enabled but EventBus not configured, skipping")
		} else {
			cfg.Logger.Info("Initializing OpenCode SSE adapter...")

			// Convert config.OpenCodeConfig to opencode.Config
			adapterConfig := opencode.Config{
				ServerURL: cfg.AppConfig.Adapters.OpenCode.ServerURL,
				SessionID: "agm-daemon", // Daemon monitors all sessions
				Reconnect: opencode.ReconnectConfig{
					InitialDelay: cfg.AppConfig.Adapters.OpenCode.Reconnect.InitialDelay,
					MaxDelay:     cfg.AppConfig.Adapters.OpenCode.Reconnect.MaxDelay,
					Multiplier:   cfg.AppConfig.Adapters.OpenCode.Reconnect.Multiplier,
				},
				MaxRetries:     0, // Unlimited retries for daemon
				FallbackTmux:   cfg.AppConfig.Adapters.OpenCode.FallbackTmux,
				HealthProbeURL: "/health",
				HealthTimeout:  5 * time.Second,
			}

			adapter, err := opencode.NewAdapter(cfg.EventBus, adapterConfig)
			if err != nil {
				cfg.Logger.Error("Failed to create OpenCode adapter", "error", err)
				if adapterConfig.FallbackTmux {
					cfg.Logger.Info("Fallback enabled: Will use Astrocyte tmux monitoring for OpenCode sessions")
				} else {
					cfg.Logger.Warn("OpenCode adapter creation failed and fallback disabled")
					cfg.Logger.Warn("OpenCode sessions will NOT be monitored until adapter is fixed")
				}
			} else {
				d.opencodeAdapter = adapter
				cfg.Logger.Info("OpenCode SSE adapter initialized", "server", adapterConfig.ServerURL)
			}
		}
	}

	return d
}

// Start begins the daemon's main loop, handling signals and periodic delivery checks
func (d *Daemon) Start() error {
	slo := contracts.Load()
	pollInterval := slo.Daemon.PollInterval.Duration
	maxRetries := slo.Daemon.MaxRetries

	// Write PID file to prevent multiple daemon instances
	if err := d.writePIDFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Ensure PID file is removed on exit
	defer d.removePIDFile()

	d.cfg.Logger.Info("Daemon starting...")
	d.cfg.Logger.Info("Poll interval configured", "interval", pollInterval)
	d.cfg.Logger.Info("Max retries configured", "max_retries", maxRetries)

	// Start OpenCode adapter if initialized
	if d.opencodeAdapter != nil {
		d.cfg.Logger.Info("Starting OpenCode SSE adapter...")
		if err := d.opencodeAdapter.Start(d.ctx); err != nil {
			d.cfg.Logger.Warn("OpenCode adapter failed to start", "error", err)
			if d.cfg.AppConfig != nil && d.cfg.AppConfig.Adapters.OpenCode.FallbackTmux {
				d.cfg.Logger.Info("Fallback enabled: OpenCode adapter will retry in background")
				d.cfg.Logger.Info("Using Astrocyte tmux monitoring for OpenCode sessions until SSE adapter connects")
			} else {
				d.cfg.Logger.Error("OpenCode adapter start failed and fallback disabled")
				d.cfg.Logger.Warn("OpenCode sessions will NOT be monitored until adapter successfully starts")
			}
		} else {
			d.cfg.Logger.Info("OpenCode SSE adapter started successfully")
		}
	} else if d.cfg.AppConfig != nil && d.cfg.AppConfig.Adapters.OpenCode.Enabled {
		// Adapter was enabled but failed to initialize
		d.cfg.Logger.Warn("OpenCode adapter enabled but not initialized (initialization failed)")
		if d.cfg.AppConfig.Adapters.OpenCode.FallbackTmux {
			d.cfg.Logger.Info("Using Astrocyte tmux monitoring as fallback for OpenCode sessions")
		}
	}

	// Retry recently failed messages (failed within last 24 hours)
	if d.cfg.Queue != nil {
		retried, err := d.cfg.Queue.RetryRecentlyFailed(24 * time.Hour)
		if err != nil {
			d.cfg.Logger.Warn("Failed to retry recently failed messages", "error", err)
		} else if retried > 0 {
			d.cfg.Logger.Info("Reset recently failed messages for retry", "count", retried)
		}
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Create ticker for periodic polling
	d.ticker = time.NewTicker(pollInterval)
	defer d.ticker.Stop()

	d.cfg.Logger.Info("Daemon running. Press Ctrl+C to stop.")

	// Main event loop
	for {
		select {
		case <-d.ctx.Done():
			d.cfg.Logger.Info("Context cancelled, shutting down...")
			return nil

		case sig := <-sigCh:
			d.cfg.Logger.Info("Received signal", "signal", sig)
			d.Stop()
			return nil

		case <-d.ticker.C:
			// Periodic delivery check
			pollStart := time.Now()
			if err := d.deliverPending(); err != nil {
				d.cfg.Logger.Warn("Error during delivery", "error", err)
			}
			pollDuration := time.Since(pollStart)
			d.metrics.RecordPoll(pollDuration)

			// Check for acknowledgment timeouts if AckManager is configured
			if d.cfg.AckManager != nil {
				if timedOut, err := d.cfg.AckManager.CheckTimeout(); err != nil {
					d.cfg.Logger.Warn("Error checking ack timeouts", "error", err)
				} else if timedOut > 0 {
					d.cfg.Logger.Info("Detected timed-out acknowledgments", "count", timedOut)
				}
			}

			// Update queue depth metric
			stats, err := d.cfg.Queue.GetStats()
			if err == nil {
				if queuedCount, ok := stats["queued"]; ok {
					d.metrics.UpdateQueueDepth(queuedCount)
				}
			}

			// Check alerts
			metrics := d.metrics.GetMetrics()
			alerts := CheckAlerts(metrics, d.alerts)
			for _, alert := range alerts {
				d.cfg.Logger.Info("Alert triggered", "level", alert.Level, "message", alert.Message, "value", alert.Value, "threshold", alert.Threshold)
			}
		}
	}
}

// Stop gracefully shuts down the daemon
func (d *Daemon) Stop() {
	d.cfg.Logger.Info("Stopping daemon...")

	// Stop OpenCode adapter if running
	if d.opencodeAdapter != nil {
		d.cfg.Logger.Info("Stopping OpenCode SSE adapter...")
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()

		if err := d.opencodeAdapter.Stop(stopCtx); err != nil {
			d.cfg.Logger.Warn("OpenCode adapter stop failed", "error", err)
		} else {
			d.cfg.Logger.Info("OpenCode SSE adapter stopped")
		}
	}

	d.cancel()
}

// deliverPending processes all queued messages, delivering those whose target
// sessions are in READY state
func (d *Daemon) deliverPending() error {
	entries, err := d.cfg.Queue.GetAllPending()
	if err != nil {
		return fmt.Errorf("failed to get all pending messages: %w", err)
	}

	if len(entries) == 0 {
		return nil
	}

	d.cfg.Logger.Info("Processing queued messages", "count", len(entries))

	delivered := 0
	deferred := 0
	failed := 0

	for _, entry := range entries {
		err := d.deliverMessage(*entry)
		if err == nil {
			delivered++
		} else if errors.Is(err, errDeferred) {
			deferred++
		} else {
			d.cfg.Logger.Warn("Failed to deliver message", "message_id", entry.MessageID, "error", err)
			failed++
		}
	}

	d.cfg.Logger.Info("Delivery summary", "delivered", delivered, "deferred", deferred, "failed", failed)

	return nil
}

// deliverMessage attempts to deliver a single message based on the target session's
// current state. Messages are only delivered when the session is READY.
func (d *Daemon) deliverMessage(entry messages.QueueEntry) error {
	// Resolve recipient session manifest to get tmux session name and manifest path
	recipientManifest, manifestPath, err := session.ResolveIdentifier(entry.To, "", d.cfg.DoltAdapter)
	if err != nil {
		d.cfg.Logger.Warn("Cannot resolve session", "session", entry.To, "error", err)
		d.metrics.RecordStateDetectionError()
		return d.retryLater(entry, err)
	}

	// Display state detection is best-effort for logging/metrics only.
	// It MUST NOT gate delivery decisions — that's CheckSessionDelivery's job.
	currentState, detectErr := session.DetectState(recipientManifest.Tmux.SessionName)
	if detectErr != nil {
		d.cfg.Logger.Warn("Display state detection failed (non-fatal)", "session", entry.To, "error", detectErr)
		d.metrics.RecordStateDetectionError()
		currentState = "unknown"
	} else {
		d.metrics.RecordStateDetection(string(currentState))
	}

	// Delivery readiness check — sole authority for delivery decisions.
	// Checks tmux session existence AND pane content independently of display state.
	canReceive := session.CheckSessionDelivery(recipientManifest.Tmux.SessionName)
	d.cfg.Logger.Info("Session delivery check", "session", entry.To, "display_state", currentState, "can_receive", canReceive, "message_id", entry.MessageID)

	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch canReceive {
	case state.CanReceiveYes:
		// Prompt visible, no dialog blocking → deliver now
		deliveryStart := time.Now()
		if err := d.sendMessage(recipientManifest.Tmux.SessionName, entry.Message); err != nil {
			d.cfg.Logger.Warn("Failed to send message", "session", entry.To, "error", err)
			return d.retryLater(entry, err)
		}

		// Update session state to WORKING after successful delivery
		if err := session.UpdateSessionState(manifestPath, manifest.StateWorking, "daemon", recipientManifest.SessionID, d.cfg.DoltAdapter); err != nil {
			d.cfg.Logger.Warn("Could not update session state", "error", err)
		}

		// Mark as delivered in queue
		if err := d.cfg.Queue.MarkDelivered(entry.MessageID); err != nil {
			d.cfg.Logger.Warn("Could not mark message as delivered", "error", err)
		}

		// Send acknowledgment if AckManager is configured
		if d.cfg.AckManager != nil {
			if err := d.cfg.AckManager.SendAck(entry.MessageID); err != nil {
				d.cfg.Logger.Warn("Could not send acknowledgment", "message_id", entry.MessageID, "error", err)
			} else {
				d.cfg.Logger.Info("Sent acknowledgment", "message_id", entry.MessageID)
			}
		}

		deliveryLatency := time.Since(deliveryStart)
		d.metrics.RecordDeliveryAttempt(true, deliveryLatency)
		d.cfg.Logger.Info("Delivered message to session", "message_id", entry.MessageID, "session", entry.To, "latency", deliveryLatency)
		return nil

	case state.CanReceiveNotFound:
		// Tmux session does not exist — retry (session may have died or not started yet)
		d.cfg.Logger.Warn("Tmux session not found", "session", entry.To, "message_id", entry.MessageID)
		return d.retryLater(entry, fmt.Errorf("tmux session '%s' does not exist", recipientManifest.Tmux.SessionName))

	case state.CanReceiveQueue:
		// Session is busy — defer delivery (leave in queue for next poll)
		d.cfg.Logger.Info("Session busy, deferring message", "session", entry.To, "display_state", currentState, "message_id", entry.MessageID)
		return errDeferred

	case state.CanReceiveNo:
		// Permission dialog or blocker — defer but log warning
		d.cfg.Logger.Warn("Session has active permission prompt, deferring message", "session", entry.To, "message_id", entry.MessageID)
		return errDeferred

	default:
		d.cfg.Logger.Warn("Unknown CanReceive state, deferring message", "session", entry.To, "can_receive", canReceive, "message_id", entry.MessageID)
		return errDeferred
	}
}

// sendMessage delivers a message to the specified tmux session
func (d *Daemon) sendMessage(sessionName, message string) error {
	d.cfg.Logger.Info("Sending message to session", "session", sessionName, "message_preview", truncateMessage(message, 60))

	if err := tmux.SendMultiLinePromptSafe(sessionName, message, false); err != nil {
		return fmt.Errorf("tmux send failed: %w", err)
	}

	return nil
}

// retryLater increments the retry count for a message. If the message has exceeded
// max retries, it is marked as permanently failed. Otherwise, it's left in queue for retry.
func (d *Daemon) retryLater(entry messages.QueueEntry, reason error) error {
	slo := contracts.Load()
	maxRetries := slo.Daemon.MaxRetries
	initialBackoff := slo.Daemon.InitialBackoff.Duration

	newAttemptCount := entry.AttemptCount + 1

	// Record failed delivery attempt (no latency for failures)
	d.metrics.RecordDeliveryAttempt(false, 0)

	if newAttemptCount >= maxRetries {
		d.cfg.Logger.Warn("Message exceeded max retries, marking as failed", "message_id", entry.MessageID, "max_retries", maxRetries)

		if err := d.cfg.Queue.MarkPermanentlyFailed(entry.MessageID); err != nil {
			return fmt.Errorf("failed to mark message as permanently failed: %w", err)
		}

		return fmt.Errorf("max retries exceeded: %w", reason)
	}

	// Increment attempt count
	if err := d.cfg.Queue.IncrementAttempt(entry.MessageID); err != nil {
		d.cfg.Logger.Warn("Could not increment attempt count", "error", err)
	}

	// Calculate exponential backoff delay (for logging only, actual retry happens next poll)
	backoff := initialBackoff * time.Duration(1<<uint(newAttemptCount-1))
	d.cfg.Logger.Info("Message will retry on next poll", "message_id", entry.MessageID, "attempt", newAttemptCount, "max_retries", maxRetries, "backoff", backoff)

	// Message stays in queue and will be retried on next poll cycle
	return fmt.Errorf("queued for retry (attempt %d/%d): %w", newAttemptCount, maxRetries, reason)
}

// writePIDFile creates a PID file to prevent multiple daemon instances
func (d *Daemon) writePIDFile() error {
	// Check if PID file already exists
	if _, err := os.Stat(d.cfg.PIDFile); err == nil {
		// Read existing PID
		data, err := os.ReadFile(d.cfg.PIDFile)
		if err != nil {
			return fmt.Errorf("PID file exists but cannot be read: %w", err)
		}

		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil {
			// Check if process is running
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("daemon already running with PID %d", pid)
				}
			}
		}

		// Stale PID file, remove it
		d.cfg.Logger.Info("Removing stale PID file")
		_ = os.Remove(d.cfg.PIDFile)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(d.cfg.PIDFile), 0o700); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}

	// Write current PID
	pid := os.Getpid()
	if err := os.WriteFile(d.cfg.PIDFile, []byte(fmt.Sprintf("%d\n", pid)), 0o600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	d.cfg.Logger.Info("PID file written", "path", d.cfg.PIDFile, "pid", pid)
	return nil
}

// removePIDFile removes the PID file on daemon shutdown
func (d *Daemon) removePIDFile() {
	if err := os.Remove(d.cfg.PIDFile); err != nil {
		d.cfg.Logger.Warn("Could not remove PID file", "error", err)
	} else {
		d.cfg.Logger.Info("PID file removed")
	}
}

// IsRunning checks if the daemon is currently running by examining the PID file
func IsRunning(pidFile string) bool {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process is alive
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// truncateMessage truncates a message for logging purposes
func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen] + "..."
}

// GetMetrics returns the current daemon metrics snapshot
func (d *Daemon) GetMetrics() MetricsSnapshot {
	return d.metrics.GetMetrics()
}

// GetAdapterHealth returns health status for all adapters
func (d *Daemon) GetAdapterHealth() AdapterHealthStatus {
	status := AdapterHealthStatus{}

	if d.opencodeAdapter != nil {
		health := d.opencodeAdapter.Health()
		status.OpenCode = &health
	}

	return status
}

// HealthStatus represents the overall health status of the daemon
type HealthStatus struct {
	Running           bool
	PID               int
	Uptime            time.Duration
	Metrics           MetricsSnapshot
	ActiveAlerts      []*Alert
	QueueStats        map[string]int
	HealthStatusLevel string // "healthy", "degraded", "critical"
	Adapters          AdapterHealthStatus
}

// AdapterHealthStatus holds health status for all adapters
type AdapterHealthStatus struct {
	OpenCode *opencode.HealthStatus `json:"opencode,omitempty"`
}

// GetHealthStatus returns comprehensive health status
// This is a standalone function that can be called without a running daemon instance
func GetHealthStatus(pidFile string, queue *messages.MessageQueue) (*HealthStatus, error) {
	slo := contracts.Load()
	da := slo.DaemonAlerts

	status := &HealthStatus{
		Running: IsRunning(pidFile),
	}

	// Get PID if running
	if status.Running {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			_, _ = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &status.PID)
		}
	}

	// Get queue stats
	if queue != nil {
		stats, err := queue.GetStats()
		if err == nil {
			status.QueueStats = stats
		}
	}

	// Determine overall health level
	if !status.Running {
		status.HealthStatusLevel = "critical"
	} else if status.QueueStats != nil {
		queuedCount := status.QueueStats["queued"]
		if queuedCount > da.QueueDepthCritical {
			status.HealthStatusLevel = "critical"
		} else if queuedCount > da.QueueDepthWarning {
			status.HealthStatusLevel = "degraded"
		} else {
			status.HealthStatusLevel = "healthy"
		}
	} else {
		status.HealthStatusLevel = "healthy"
	}

	return status, nil
}
