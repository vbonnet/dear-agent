package ops

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// AlertSeverity is how urgent an alert is, as classified from its text.
type AlertSeverity string

// Severity levels, ordered most to least urgent.
const (
	AlertSeverityCritical AlertSeverity = "critical"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityInfo     AlertSeverity = "info"
)

// AlertActionability is who can actually act on an alert, and so decides
// whether it is routed to an agent, paged to a human, or recorded quietly.
type AlertActionability string

// Actionability classes. AlertAgentActionable is the default routing path.
const (
	AlertAgentActionable AlertActionability = "agent_actionable"
	AlertHumanOnly       AlertActionability = "human_only"
	AlertInformational   AlertActionability = "informational"
)

// AlertStatus is the outcome of routing one alert.
type AlertStatus string

// Routing outcomes. Queued means delivery failed or had no target and the
// record is retained for a later attempt.
const (
	AlertStatusQueued     AlertStatus = "queued"
	AlertStatusDispatched AlertStatus = "dispatched"
	AlertStatusPagedHuman AlertStatus = "paged_human"
	AlertStatusQuiet      AlertStatus = "quiet"
	AlertStatusSuppressed AlertStatus = "suppressed"
)

// AlertRequest is an inbound alert before classification and routing.
type AlertRequest struct {
	Kind          string
	Source        string
	Title         string
	Body          string
	Subject       string
	Severity      AlertSeverity
	Actionability AlertActionability
	Target        string
	OccurredAt    time.Time
	Meta          map[string]any
}

// AlertRecord is the durable record of one routed alert, appended to the
// queue file so later routing can dedupe against it.
type AlertRecord struct {
	ID            string             `json:"id"`
	Fingerprint   string             `json:"fingerprint"`
	Kind          string             `json:"kind"`
	Source        string             `json:"source"`
	Title         string             `json:"title"`
	Body          string             `json:"body"`
	Subject       string             `json:"subject,omitempty"`
	Severity      AlertSeverity      `json:"severity"`
	Actionability AlertActionability `json:"actionability"`
	Status        AlertStatus        `json:"status"`
	Target        string             `json:"target,omitempty"`
	OccurredAt    time.Time          `json:"occurred_at"`
	RecordedAt    time.Time          `json:"recorded_at"`
	RepeatCount   int                `json:"repeat_count,omitempty"`
	Error         string             `json:"error,omitempty"`
	Meta          map[string]any     `json:"meta,omitempty"`
}

// AlertRouter classifies alerts and delivers them to a live agent session or
// a human recipient, recording every outcome to the queue file.
type AlertRouter struct {
	ctx            *OpContext
	queuePath      string
	dedupeWindow   time.Duration
	humanRecipient string
	now            func() time.Time
}

// NewAlertRouter builds a router with default queue path and dedupe window.
func NewAlertRouter(ctx *OpContext) *AlertRouter {
	return &AlertRouter{
		ctx:            ctx,
		queuePath:      DefaultAlertQueuePath(),
		dedupeWindow:   30 * time.Minute,
		humanRecipient: strings.TrimSpace(os.Getenv("AGM_HUMAN_ALERT_RECIPIENT")),
		now:            time.Now,
	}
}

// DefaultAlertQueuePath returns the default alert queue file path.
func DefaultAlertQueuePath() string {
	if path := strings.TrimSpace(os.Getenv("AGM_ALERT_QUEUE")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agm-alerts.jsonl")
	}
	return filepath.Join(home, ".agm", "alerts", "queue.jsonl")
}

// SetQueuePath overrides where routed alerts are recorded.
func (r *AlertRouter) SetQueuePath(path string) {
	r.queuePath = path
}

// SetDedupeWindow overrides how long an identical alert is suppressed.
func (r *AlertRouter) SetDedupeWindow(window time.Duration) {
	r.dedupeWindow = window
}

// Route classifies req, delivers it to the appropriate target, and appends
// the outcome to the queue. A delivery failure is recorded on the returned
// record rather than returned as an error, so the alert is never lost.
func (r *AlertRouter) Route(ctx context.Context, req AlertRequest) (AlertRecord, error) {
	req = classifyAlert(req)
	if req.OccurredAt.IsZero() {
		req.OccurredAt = r.now()
	}
	fp := alertFingerprint(req)
	if prev, ok := r.recentRecord(fp, req.OccurredAt); ok {
		rec := req.record(fp, r.now)
		rec.ID = prev.ID
		rec.Status = AlertStatusSuppressed
		rec.Target = prev.Target
		rec.RepeatCount = prev.RepeatCount + 1
		rec.Error = fmt.Sprintf("suppressed duplicate of %s", prev.RecordedAt.Format(time.RFC3339))
		return rec, nil
	}

	rec := req.record(fp, r.now)
	switch req.Actionability {
	case AlertInformational:
		rec.Status = AlertStatusQuiet
	case AlertHumanOnly:
		rec.Status = AlertStatusPagedHuman
		if r.humanRecipient == "" {
			rec.Status = AlertStatusQueued
			rec.Error = "human-only alert has no AGM_HUMAN_ALERT_RECIPIENT"
		} else if err := r.send(ctx, r.humanRecipient, humanAlertMessage(rec)); err != nil {
			rec.Status = AlertStatusQueued
			rec.Error = err.Error()
		} else {
			rec.Target = r.humanRecipient
		}
	case AlertAgentActionable:
		fallthrough
	default:
		// Unclassified actionability routes as agent-actionable: an alert
		// nobody classified is better delivered to an agent than dropped.
		target := strings.TrimSpace(req.Target)
		if target == "" || !r.sessionLooksLive(target) {
			target = r.discoverSupervisor()
		}
		if target == "" {
			rec.Status = AlertStatusQueued
			rec.Error = "no live supervisor session discovered"
		} else if err := r.send(ctx, target, agentAlertMessage(rec)); err != nil {
			rec.Status = AlertStatusQueued
			rec.Target = target
			rec.Error = err.Error()
		} else {
			rec.Status = AlertStatusDispatched
			rec.Target = target
		}
	}
	if err := appendAlertRecord(r.queuePath, rec); err != nil {
		return rec, err
	}
	return rec, nil
}

func classifyAlert(req AlertRequest) AlertRequest {
	text := strings.ToLower(req.Kind + " " + req.Title + " " + req.Body + " " + req.Subject)
	if req.Severity == "" {
		req.Severity = AlertSeverityInfo
	}
	if req.Actionability == "" {
		req.Actionability = AlertInformational
	}
	if alertContainsAny(text, "auth at risk", "auth-at-risk", "provider quota halted", "quota halted", "flywheel stalled", "disk floor", "spawn failure", "spawn failures") {
		req.Severity = AlertSeverityCritical
		req.Actionability = AlertAgentActionable
	}
	if alertContainsAny(text, "credential", "oauth", "manual decision", "needs valentin", "human approval") && !alertContainsAny(text, "auth at risk", "auth-at-risk") {
		req.Actionability = AlertHumanOnly
		if req.Severity == "" || req.Severity == AlertSeverityInfo {
			req.Severity = AlertSeverityWarning
		}
	}
	if req.Kind == "completion" && req.Actionability == AlertInformational {
		req.Severity = AlertSeverityInfo
	}
	return req
}

func alertContainsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func (req AlertRequest) record(fp string, now func() time.Time) AlertRecord {
	recorded := now()
	return AlertRecord{
		ID:            fmt.Sprintf("alert-%d-%s", recorded.Unix(), fp[:12]),
		Fingerprint:   fp,
		Kind:          req.Kind,
		Source:        req.Source,
		Title:         req.Title,
		Body:          req.Body,
		Subject:       req.Subject,
		Severity:      req.Severity,
		Actionability: req.Actionability,
		Status:        AlertStatusQueued,
		OccurredAt:    req.OccurredAt,
		RecordedAt:    recorded,
		Meta:          req.Meta,
	}
}

func alertFingerprint(req AlertRequest) string {
	parts := []string{req.Kind, req.Source, req.Subject, req.Title}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(parts, "\x00"))))
	return hex.EncodeToString(sum[:])
}

func (r *AlertRouter) recentRecord(fp string, occurredAt time.Time) (AlertRecord, bool) {
	if r.queuePath == "" || r.dedupeWindow <= 0 {
		return AlertRecord{}, false
	}
	records, err := ReadAlertRecords(r.queuePath, 500)
	if err != nil {
		return AlertRecord{}, false
	}
	for _, rec := range slices.Backward(records) {
		if rec.Fingerprint != fp || rec.Status == AlertStatusSuppressed {
			continue
		}
		if occurredAt.Sub(rec.OccurredAt) <= r.dedupeWindow {
			return rec, true
		}
		return AlertRecord{}, false
	}
	return AlertRecord{}, false
}

func (r *AlertRouter) sessionLooksLive(name string) bool {
	if r.ctx == nil || r.ctx.Storage == nil || name == "" {
		return false
	}
	m, err := r.ctx.Storage.GetSession(name)
	if err != nil || m == nil {
		return false
	}
	return sessionIsLiveSupervisorCandidate(m)
}

func (r *AlertRouter) discoverSupervisor() string {
	if r.ctx == nil || r.ctx.Storage == nil {
		return ""
	}
	sessions, err := r.ctx.Storage.ListSessions(&dolt.SessionFilter{ExcludeArchived: true, Limit: 1000})
	if err != nil {
		return ""
	}
	for _, preferred := range []string{"dispatch", "orchestrator", "overseer", "supervisor", "meta-"} {
		for _, m := range sessions {
			if sessionIsLiveSupervisorCandidate(m) && strings.Contains(strings.ToLower(m.Name), preferred) {
				return m.Name
			}
		}
	}
	return ""
}

func sessionIsLiveSupervisorCandidate(m *manifest.Manifest) bool {
	if m == nil {
		return false
	}
	state := strings.ToUpper(strings.TrimSpace(m.State))
	if state == manifest.StateOffline || state == manifest.StateDone || state == "ARCHIVED" {
		return false
	}
	name := strings.ToLower(m.Name)
	if alertContainsAny(name, "dispatch", "orchestrator", "overseer", "supervisor", "meta-") {
		return true
	}
	for _, tag := range m.Context.Tags {
		if alertContainsAny(strings.ToLower(tag), "role:orchestrator", "role:overseer", "role:supervisor", "role:meta-orchestrator") {
			return true
		}
	}
	return false
}

func (r *AlertRouter) send(ctx context.Context, recipient, message string) error {
	if r.ctx == nil {
		return fmt.Errorf("nil OpContext in AlertRouter")
	}
	relayCtx := *r.ctx
	relayCtx.Context = ctx
	result, err := SendMessage(&relayCtx, &SendMessageRequest{Recipient: recipient, Message: message})
	if err != nil {
		return err
	}
	if !result.Delivered {
		return fmt.Errorf("send to %s not delivered", recipient)
	}
	return nil
}

func agentAlertMessage(rec AlertRecord) string {
	return fmt.Sprintf("[agm-alert] %s/%s: %s\nSubject: %s\n%s\n\nAct autonomously if possible. If blocked by human-only authority or credentials, escalate upward with receipts.",
		rec.Severity, rec.Actionability, rec.Title, rec.Subject, rec.Body)
}

func humanAlertMessage(rec AlertRecord) string {
	return fmt.Sprintf("[agm-human-alert] %s: %s\nSubject: %s\n%s", rec.Severity, rec.Title, rec.Subject, rec.Body)
}

func appendAlertRecord(path string, rec AlertRecord) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create alert queue dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open alert queue: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(rec); err != nil {
		return fmt.Errorf("write alert queue: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close alert queue: %w", err)
	}
	return nil
}

// ReadAlertRecords reads up to limit most-recent alert records from path.
func ReadAlertRecords(path string, limit int) ([]AlertRecord, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open alert queue: %w", err)
	}
	defer f.Close()

	var records []AlertRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec AlertRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if limit > 0 && len(records) >= limit {
			copy(records, records[1:])
			records[limit-1] = rec
		} else {
			records = append(records, rec)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read alert queue: %w", err)
	}
	return records, nil
}
