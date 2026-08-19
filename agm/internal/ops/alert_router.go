package ops

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/lock"
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

const (
	// alertQueueTailBytes bounds how much of the queue file a dedupe read
	// touches. Every routed alert performs this read, so scanning the whole
	// file would make alert handling slower the longer a watcher has run.
	alertQueueTailBytes int64 = 1 << 20

	// alertQueueMaxBytes is the size at which the queue rotates. The file
	// receives every dispatched, quiet, suppressed, and queued record, so
	// without rotation it grows without bound on a long-running host.
	alertQueueMaxBytes int64 = 8 << 20

	// alertLockTimeout bounds how long routing waits for the queue lock.
	// On timeout routing proceeds unserialized: a duplicate page is a far
	// better failure than an alert dropped because a peer process was slow.
	alertLockTimeout = 5 * time.Second
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
	// DedupeKey distinguishes two alerts that are otherwise identical.
	// Callers that can recur legitimately within the dedupe window must set
	// it: a persistent worker finishing two tasks produces the same kind,
	// source, subject, and title both times, and without an occurrence
	// identity the second result would be discarded as a duplicate.
	DedupeKey string
	Meta      map[string]any
}

// AlertRecord is the durable record of one routed alert, appended to the
// queue file so later routing can dedupe against it.
type AlertRecord struct {
	ID            string             `json:"id"`
	Fingerprint   string             `json:"fingerprint"`
	Workspace     string             `json:"workspace,omitempty"`
	Kind          string             `json:"kind"`
	Source        string             `json:"source"`
	Title         string             `json:"title"`
	Body          string             `json:"body"`
	Subject       string             `json:"subject,omitempty"`
	Severity      AlertSeverity      `json:"severity"`
	Actionability AlertActionability `json:"actionability"`
	Status        AlertStatus        `json:"status"`
	// PriorStatus is the status of the record a suppressed alert duplicates,
	// so a caller can tell "already delivered" from "already failed".
	PriorStatus AlertStatus    `json:"prior_status,omitempty"`
	Target      string         `json:"target,omitempty"`
	OccurredAt  time.Time      `json:"occurred_at"`
	RecordedAt  time.Time      `json:"recorded_at"`
	RepeatCount int            `json:"repeat_count,omitempty"`
	Error       string         `json:"error,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// Delivered reports whether this routing outcome actually reached someone.
//
// Suppression counts only when the record it duplicates was itself
// delivered. Treating every suppressed result as success would let a
// repeat of an alert that was merely queued report delivery, so a caller
// such as stall recovery would publish "recovered" while nobody had ever
// seen the original page.
func (rec AlertRecord) Delivered() bool {
	switch rec.Status {
	case AlertStatusDispatched, AlertStatusPagedHuman:
		return true
	case AlertStatusSuppressed:
		return rec.PriorStatus == AlertStatusDispatched || rec.PriorStatus == AlertStatusPagedHuman
	case AlertStatusQuiet:
		// Informational alerts have no recipient by design; recording one
		// is the whole delivery.
		return true
	case AlertStatusQueued:
		// Queued is the router's word for "delivery failed"; the record is
		// retained so a later drain can try again.
		return false
	default:
		return false
	}
}

// AlertRouter classifies alerts and delivers them to a live agent session or
// a human recipient, recording every outcome to the queue file.
type AlertRouter struct {
	ctx            *OpContext
	queuePath      string
	dedupeWindow   time.Duration
	humanRecipient string
	workspace      string
	now            func() time.Time
	lockTimeout    time.Duration
	// sendMessage delivers one alert to one recipient. Nil uses the real
	// SendMessage; in-package tests replace it so routing decisions can be
	// exercised without a live tmux session.
	sendMessage func(ctx context.Context, recipient, message string) error
}

// NewAlertRouter builds a router with default queue path and dedupe window.
func NewAlertRouter(ctx *OpContext) *AlertRouter {
	return &AlertRouter{
		ctx:            ctx,
		queuePath:      DefaultAlertQueuePath(),
		dedupeWindow:   30 * time.Minute,
		humanRecipient: strings.TrimSpace(os.Getenv("AGM_HUMAN_ALERT_RECIPIENT")),
		workspace:      workspaceIdentity(ctx),
		now:            time.Now,
		lockTimeout:    alertLockTimeout,
	}
}

// workspaceIdentity names the workspace an alert came from. The default
// queue lives under the user's home directory and is shared by every
// workspace on the host, so without this two workspaces running watchers
// with the same session and checker names would suppress each other's
// alerts, and the record would not say which one produced it.
func workspaceIdentity(ctx *OpContext) string {
	if ctx == nil || ctx.Config == nil {
		return ""
	}
	if ws := strings.TrimSpace(ctx.Config.Storage.Workspace); ws != "" {
		return ws
	}
	return strings.TrimSpace(ctx.Config.Workspace)
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

// SetWorkspace overrides the workspace recorded on and deduped by alerts.
func (r *AlertRouter) SetWorkspace(workspace string) {
	r.workspace = strings.TrimSpace(workspace)
}

// Route classifies req, delivers it to the appropriate target, and appends
// the outcome to the queue. A delivery failure is recorded on the returned
// record rather than returned as an error, so the alert is never lost.
//
// The dedupe read, the delivery, and the append are one critical section
// held under an inter-process lock. They have to be: the queue file is
// shared by every watcher on the host (a launchd one and a hand-started
// one, say), so if each could complete its dedupe read before either
// appended, both would dispatch the page the window exists to collapse.
func (r *AlertRouter) Route(ctx context.Context, req AlertRequest) (AlertRecord, error) {
	req = classifyAlert(req)
	if req.OccurredAt.IsZero() {
		req.OccurredAt = r.now()
	}
	fp := r.fingerprint(req)

	release := r.lockQueue()
	defer release()

	if prev, ok := r.recentDelivered(fp, req.OccurredAt); ok {
		rec := r.record(req, fp)
		rec.ID = prev.ID
		rec.Status = AlertStatusSuppressed
		rec.PriorStatus = prev.Status
		rec.Target = prev.Target
		rec.RepeatCount = prev.RepeatCount + 1
		rec.Error = fmt.Sprintf("suppressed duplicate of %s", prev.RecordedAt.Format(time.RFC3339))
		// Persist the suppression: otherwise repeats are invisible to
		// `agm alerts list` and RepeatCount can never grow past one.
		if err := r.appendLocked(rec); err != nil {
			return rec, err
		}
		return rec, nil
	}

	rec := r.deliver(ctx, req, fp)
	if err := r.appendLocked(rec); err != nil {
		return rec, err
	}
	return rec, nil
}

// deliver performs the routing decision for one classified request.
func (r *AlertRouter) deliver(ctx context.Context, req AlertRequest, fp string) AlertRecord {
	rec := r.record(req, fp)
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
		r.dispatchToAgent(ctx, &rec, req.Target)
	}
	return rec
}

// dispatchToAgent tries every live candidate in preference order before
// giving up.
//
// Stopping at the first candidate would queue a critical alert whenever the
// preferred session merely cannot accept input right now (sitting at a
// permission prompt, say) even though another live supervisor was ready to
// take it.
func (r *AlertRouter) dispatchToAgent(ctx context.Context, rec *AlertRecord, explicit string) {
	candidates := r.deliveryCandidates(explicit)
	if len(candidates) == 0 {
		rec.Status = AlertStatusQueued
		rec.Error = "no live supervisor session discovered"
		return
	}
	var failures []string
	for _, target := range candidates {
		err := r.send(ctx, target, agentAlertMessage(*rec))
		if err == nil {
			rec.Status = AlertStatusDispatched
			rec.Target = target
			return
		}
		failures = append(failures, fmt.Sprintf("%s: %v", target, err))
	}
	rec.Status = AlertStatusQueued
	rec.Target = candidates[0]
	rec.Error = "all candidates failed: " + strings.Join(failures, "; ")
}

// ResolveTarget reports the recipient an agent-actionable alert would be
// delivered to right now, or "" if none is reachable.
//
// Callers that must filter an event against its own routing destination
// use this to take one snapshot, then pass that snapshot back as the
// request Target. Filtering against a separately discovered target and
// then letting Route rediscover its own would reopen the window where an
// event is relayed into the very session it reports.
func (r *AlertRouter) ResolveTarget(explicit string) string {
	candidates := r.deliveryCandidates(explicit)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

// deliveryCandidates lists recipients to try, in order.
//
// An explicitly configured target is checked for liveness only. The
// supervisor-name heuristic belongs to discovery: applying it to an
// explicit target would discard a perfectly good pinned recipient whose
// name happens not to contain a supervisor keyword, which is exactly what
// pinning is for.
func (r *AlertRouter) deliveryCandidates(explicit string) []string {
	var candidates []string
	if target := strings.TrimSpace(explicit); target != "" && r.sessionIsReachable(target) {
		candidates = append(candidates, target)
	}
	for _, name := range r.discoverSupervisors() {
		if !slices.Contains(candidates, name) {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

func classifyAlert(req AlertRequest) AlertRequest {
	text := strings.ToLower(req.Kind + " " + req.Title + " " + req.Body + " " + req.Subject)
	// Remember whether the caller stated an actionability. Keyword inference
	// may fill an unstated one, but must never overrule a stated one: a
	// worker result that merely mentions fixing an OAuth credential is still
	// the agent-actionable completion its caller typed, and reclassifying it
	// human-only would queue it unseen on any host without
	// AGM_HUMAN_ALERT_RECIPIENT.
	actionabilityStated := req.Actionability != ""
	if req.Severity == "" {
		req.Severity = AlertSeverityInfo
	}
	if !actionabilityStated {
		req.Actionability = AlertInformational
	}
	if alertContainsAny(text, "auth at risk", "auth-at-risk", "provider quota halted", "quota halted", "flywheel stalled", "disk floor", "spawn failure", "spawn failures") {
		req.Severity = AlertSeverityCritical
		if !actionabilityStated {
			req.Actionability = AlertAgentActionable
		}
	}
	if !actionabilityStated &&
		alertContainsAny(text, "credential", "oauth", "manual decision", "needs valentin", "human approval") &&
		!alertContainsAny(text, "auth at risk", "auth-at-risk") {
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

func (r *AlertRouter) record(req AlertRequest, fp string) AlertRecord {
	recorded := r.now()
	return AlertRecord{
		ID:            fmt.Sprintf("alert-%d-%s", recorded.Unix(), fp[:12]),
		Fingerprint:   fp,
		Workspace:     r.workspace,
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

// fingerprint is the dedupe identity of an alert.
//
// Severity and actionability are part of it so an alert that escalates
// inside the dedupe window is delivered rather than suppressed: a checker
// going from warning to critical is new information, not a repeated page.
// Workspace is part of it so two workspaces sharing the host-wide queue do
// not silence each other. DedupeKey lets a caller declare occurrence
// identity for alerts that legitimately recur.
func (r *AlertRouter) fingerprint(req AlertRequest) string {
	parts := []string{
		r.workspace, req.Kind, req.Source, req.Subject, req.Title,
		string(req.Severity), string(req.Actionability), req.DedupeKey,
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(parts, "\x00"))))
	return hex.EncodeToString(sum[:])
}

// recentDelivered finds a recent record with this fingerprint that actually
// reached someone.
//
// A prior record that was only queued must not suppress this attempt: that
// earlier alert never got to anyone, so the repeat is the retry that
// finally delivers it. Suppressing against it would strand the alert
// forever behind its own failed predecessor.
func (r *AlertRouter) recentDelivered(fp string, occurredAt time.Time) (AlertRecord, bool) {
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
		if !rec.Delivered() {
			return AlertRecord{}, false
		}
		if occurredAt.Sub(rec.OccurredAt) <= r.dedupeWindow {
			return rec, true
		}
		return AlertRecord{}, false
	}
	return AlertRecord{}, false
}

// DrainQueued re-attempts delivery for alerts recorded as queued and never
// delivered since.
//
// Without this, an auth, quota, disk, or spawn alert raised while no
// supervisor was reachable would sit in the file forever: persistence is
// not delivery, and nothing else reads the queue. Callers run it whenever
// a supervisor may have become available, and it reports how many alerts
// it managed to deliver.
func (r *AlertRouter) DrainQueued(ctx context.Context) (int, error) {
	release := r.lockQueue()
	defer release()

	records, err := ReadAlertRecords(r.queuePath, 500)
	if err != nil {
		return 0, err
	}
	// Latest status per fingerprint: a fingerprint whose most recent record
	// was delivered needs no drain.
	latest := map[string]AlertRecord{}
	order := []string{}
	for _, rec := range records {
		if _, seen := latest[rec.Fingerprint]; !seen {
			order = append(order, rec.Fingerprint)
		}
		latest[rec.Fingerprint] = rec
	}

	delivered := 0
	for _, fp := range order {
		rec := latest[fp]
		if rec.Status != AlertStatusQueued {
			continue
		}
		if rec.Actionability == AlertHumanOnly && r.humanRecipient == "" {
			// Still no human recipient configured; nothing changed.
			continue
		}
		retry := rec
		retry.RecordedAt = r.now()
		retry.Error = ""
		retry.RepeatCount = rec.RepeatCount + 1
		if rec.Actionability == AlertHumanOnly {
			if err := r.send(ctx, r.humanRecipient, humanAlertMessage(retry)); err != nil {
				continue
			}
			retry.Status = AlertStatusPagedHuman
			retry.Target = r.humanRecipient
		} else {
			r.dispatchToAgent(ctx, &retry, rec.Target)
			if retry.Status != AlertStatusDispatched {
				continue
			}
		}
		if err := r.appendLocked(retry); err != nil {
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}

// sessionIsReachable reports whether name is a session that exists and is
// not terminal. It deliberately applies no name heuristic: see
// deliveryCandidates.
func (r *AlertRouter) sessionIsReachable(name string) bool {
	if r.ctx == nil || r.ctx.Storage == nil || name == "" {
		return false
	}
	m, err := r.ctx.Storage.GetSession(name)
	if err != nil || m == nil {
		return false
	}
	return !sessionIsTerminal(m)
}

// discoverSupervisors lists live supervisor sessions in preference order.
//
// A session qualifies by conventional name or by an explicit supervisor
// role tag. Requiring the name alone would queue alerts on a mesh whose
// supervisor is called something like control-plane but is tagged
// role:supervisor, even though it is live and ready.
func (r *AlertRouter) discoverSupervisors() []string {
	if r.ctx == nil || r.ctx.Storage == nil {
		return nil
	}
	sessions, err := r.ctx.Storage.ListSessions(&dolt.SessionFilter{ExcludeArchived: true, Limit: 1000})
	if err != nil {
		return nil
	}
	var names []string
	for _, preferred := range supervisorNamePreference {
		for _, m := range sessions {
			if m == nil || sessionIsTerminal(m) {
				continue
			}
			if strings.Contains(strings.ToLower(m.Name), preferred) && !slices.Contains(names, m.Name) {
				names = append(names, m.Name)
			}
		}
	}
	// Role-tagged supervisors rank after conventionally named ones, but
	// they are candidates rather than being invisible.
	for _, m := range sessions {
		if m == nil || sessionIsTerminal(m) || !sessionHasSupervisorRoleTag(m) {
			continue
		}
		if !slices.Contains(names, m.Name) {
			names = append(names, m.Name)
		}
	}
	return names
}

// supervisorNamePreference orders the conventional supervisor names most to
// least preferred.
var supervisorNamePreference = []string{"dispatch", "orchestrator", "overseer", "supervisor", "meta-"}

// supervisorRoleTags are the role tags that mark a session as a supervisor
// regardless of its name.
var supervisorRoleTags = []string{"role:orchestrator", "role:overseer", "role:supervisor", "role:meta-orchestrator"}

// sessionIsTerminal reports whether a session can no longer receive input.
func sessionIsTerminal(m *manifest.Manifest) bool {
	if m == nil {
		return true
	}
	state := strings.ToUpper(strings.TrimSpace(m.State))
	if state == manifest.StateOffline || state == manifest.StateDone || state == "ARCHIVED" {
		return true
	}
	return m.Lifecycle == manifest.LifecycleArchived
}

func sessionHasSupervisorRoleTag(m *manifest.Manifest) bool {
	if m == nil {
		return false
	}
	for _, tag := range m.Context.Tags {
		if alertContainsAny(strings.ToLower(tag), supervisorRoleTags...) {
			return true
		}
	}
	return false
}

// SessionIsLiveSupervisorCandidate reports whether m is a live session that
// routing may treat as a supervisor, by conventional name or role tag.
//
// This is the one owner of that policy. Escalation routing used to carry
// its own copy, and the two had already drifted on how they recognized an
// archived session, so a change to routing eligibility altered alerting and
// escalation differently.
func SessionIsLiveSupervisorCandidate(m *manifest.Manifest) bool {
	if m == nil || sessionIsTerminal(m) {
		return false
	}
	if alertContainsAny(strings.ToLower(m.Name), supervisorNamePreference...) {
		return true
	}
	return sessionHasSupervisorRoleTag(m)
}

// SessionIsLive reports whether a session can still receive input. It is
// the one liveness rule every routing surface shares, so escalation and
// alerting cannot drift on what counts as an archived or finished session.
func SessionIsLive(m *manifest.Manifest) bool {
	return !sessionIsTerminal(m)
}

// SupervisorNamePreference returns the conventional supervisor name order,
// so other routing surfaces rank candidates the same way this one does.
func SupervisorNamePreference() []string {
	return slices.Clone(supervisorNamePreference)
}

func (r *AlertRouter) send(ctx context.Context, recipient, message string) error {
	if r.sendMessage != nil {
		return r.sendMessage(ctx, recipient, message)
	}
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

// lockQueue takes the inter-process queue lock and returns its release.
//
// Acquisition is bounded: if a peer holds the lock past the timeout the
// caller proceeds anyway, accepting that dedupe may double-send, because
// blocking indefinitely on a wedged peer would stall the alert entirely.
func (r *AlertRouter) lockQueue() func() {
	if r.queuePath == "" {
		return func() {}
	}
	fl, err := lock.New(r.queuePath + ".lock")
	if err != nil {
		return func() {}
	}
	timeout := r.lockTimeout
	if timeout <= 0 {
		timeout = alertLockTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := fl.TryLock(); err == nil {
			return func() { _ = fl.Unlock() }
		}
		if time.Now().After(deadline) {
			_ = fl.Unlock()
			return func() {}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// appendLocked writes rec to the queue. Callers must already hold the queue
// lock; the name says so because the rotation below is only safe when no
// peer is mid-append.
func (r *AlertRouter) appendLocked(rec AlertRecord) error {
	return appendAlertRecord(r.queuePath, rec)
}

func appendAlertRecord(path string, rec AlertRecord) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create alert queue dir: %w", err)
	}
	if err := rotateAlertQueue(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open alert queue: %w", err)
	}
	if err := json.NewEncoder(f).Encode(rec); err != nil {
		_ = f.Close()
		return fmt.Errorf("write alert queue: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close alert queue: %w", err)
	}
	return nil
}

// rotateAlertQueue moves the queue aside once it exceeds the size cap, so a
// watcher that has run for months does not accumulate an unbounded history.
// One generation is kept; older history is not load-bearing because dedupe
// only looks back over its window.
func rotateAlertQueue(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= alertQueueMaxBytes {
		return nil //nolint:nilerr // a missing queue is normal; it is about to be created
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate alert queue: %w", err)
	}
	return nil
}

// ReadAlertRecords reads up to limit most-recent alert records from path.
//
// It reads a bounded tail rather than the whole file: this runs on every
// routed alert, and the queue accumulates every dispatched, quiet, and
// queued record, so a full scan would make alert handling slower the longer
// the host has been up.
func ReadAlertRecords(path string, limit int) ([]AlertRecord, error) {
	return readAlertRecords(path, limit, nil)
}

// ReadAlertRecordsWithStatus reads up to limit most-recent records whose
// status is one of statuses.
//
// Filtering happens before the limit is applied. Reading the newest limit
// records and then filtering would answer "no queued alerts" whenever the
// tail happened to be full of dispatched completions, even though an
// undelivered critical alert was still sitting in the file.
func ReadAlertRecordsWithStatus(path string, limit int, statuses ...AlertStatus) ([]AlertRecord, error) {
	if len(statuses) == 0 {
		return readAlertRecords(path, limit, nil)
	}
	return readAlertRecords(path, limit, func(rec AlertRecord) bool {
		return slices.Contains(statuses, rec.Status)
	})
}

func readAlertRecords(path string, limit int, keep func(AlertRecord) bool) ([]AlertRecord, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open alert queue: %w", err)
	}
	defer func() { _ = f.Close() }()

	reader, err := boundedTail(f)
	if err != nil {
		return nil, err
	}

	var records []AlertRecord
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec AlertRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if keep != nil && !keep(rec) {
			continue
		}
		if limit > 0 && len(records) >= limit {
			// Shift in place so the backing array stays bounded rather than
			// growing to hold every line in the file.
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

// boundedTail returns a reader over at most alertQueueTailBytes of f,
// starting at a record boundary.
func boundedTail(f *os.File) (io.Reader, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat alert queue: %w", err)
	}
	if info.Size() <= alertQueueTailBytes {
		return f, nil
	}
	if _, err := f.Seek(info.Size()-alertQueueTailBytes, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek alert queue: %w", err)
	}
	// The seek almost certainly landed mid-line; drop that partial record so
	// the scanner starts on a whole one.
	buffered := bufio.NewReader(f)
	if _, err := buffered.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("align alert queue tail: %w", err)
	}
	return buffered, nil
}

// jsonMarshalAlert marshals one record; used by tests that build queue
// fixtures directly.
func jsonMarshalAlert(rec AlertRecord) ([]byte, error) {
	return json.Marshal(rec)
}
