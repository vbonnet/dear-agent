package absencealarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PulseType selects the probe used to look for a pulse's positive event.
type PulseType string

const (
	// PulseFileMtime is present while the file at Path has been modified
	// within Window.
	PulseFileMtime PulseType = "file_mtime"
	// PulseLaunchdLoaded is present while Label appears in the launchd
	// job listing for the GUI domain.
	PulseLaunchdLoaded PulseType = "launchd_loaded"
	// PulseCommand is present while Command exits zero.
	PulseCommand PulseType = "command"
	// PulseJSONTimestamp is present while the timestamp in Path's Field
	// is within Window.
	PulseJSONTimestamp PulseType = "json_timestamp"
)

// Pulse is one expected positive event with a maximum silence window.
type Pulse struct {
	Name    string    `json:"name"`
	Type    PulseType `json:"type"`
	Expect  string    `json:"expect"`
	Path    string    `json:"path,omitempty"`
	Field   string    `json:"field,omitempty"`
	Label   string    `json:"label,omitempty"`
	Command []string  `json:"command,omitempty"`
	Window  string    `json:"window,omitempty"`

	window time.Duration
}

// Status is the classified outcome of probing one pulse.
type Status string

const (
	// StatusPresent means the positive event was observed inside the window.
	StatusPresent Status = "present"
	// StatusAbsent means the probe ran and the event was not observed (AA-01..AA-04).
	StatusAbsent Status = "absent"
	// StatusUndetermined means the probe could not be evaluated (AA-05, AA-06).
	// Undetermined alarms exactly like absent: "could not check" is not health.
	StatusUndetermined Status = "undetermined"
	// StatusSnoozed means a valid unexpired snooze covers the pulse (AA-13).
	StatusSnoozed Status = "snoozed"
)

// Alarming reports whether the status raises an alarm (AA-07).
func (s Status) Alarming() bool { return s == StatusAbsent || s == StatusUndetermined }

// Result is the report entry for one pulse (AA-21).
type Result struct {
	Name     string    `json:"name"`
	Status   Status    `json:"status"`
	Reason   string    `json:"reason,omitempty"`
	Expect   string    `json:"expect,omitempty"`
	Window   string    `json:"window,omitempty"`
	Evidence time.Time `json:"evidence,omitzero"`
	Misses   int       `json:"misses,omitempty"`
}

// PulseConfig is the on-disk configuration document.
type PulseConfig struct {
	Pulses []Pulse `json:"pulses"`
}

// clockSkewTolerance bounds how far in the future an evidence timestamp may
// sit before it stops counting as proof of life (AA-06).
const clockSkewTolerance = 5 * time.Minute

// Probes are the injectable host observations, so tests never touch the
// real launchd, filesystem clock, or subprocesses.
type Probes struct {
	Now         func() time.Time
	StatMtime   func(path string) (mtime time.Time, exists bool, err error)
	ReadFile    func(path string) ([]byte, error)
	LaunchdList func(ctx context.Context) (string, error)
	// RunCommand runs a probe and returns its exit code and captured stdout.
	// The output is carried because a probe that failed already knows why, and
	// its summary is written to be read by whoever gets the escalation.
	RunCommand func(ctx context.Context, argv []string) (exitCode int, output string, err error)
}

// DefaultProbes returns the real host observations used outside tests.
func DefaultProbes() Probes {
	return Probes{
		Now: time.Now,
		StatMtime: func(path string) (time.Time, bool, error) {
			fi, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return time.Time{}, false, nil
				}
				return time.Time{}, false, err
			}
			return fi.ModTime(), true, nil
		},
		ReadFile: os.ReadFile,
		LaunchdList: func(ctx context.Context) (string, error) {
			out, err := exec.CommandContext(ctx, "launchctl", "list").Output()
			if err != nil {
				return "", err
			}
			return string(out), nil
		},
		RunCommand: func(ctx context.Context, argv []string) (int, string, error) {
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			var out strings.Builder
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			if err == nil {
				return 0, out.String(), nil
			}
			// A probe killed by its own deadline was never evaluated. Report
			// that as an evaluation failure (UNDETERMINED, AA-05) rather than
			// letting CommandContext's signal kill surface as a non-zero exit
			// (ABSENT, AA-04). "The check did not finish" and "the check said
			// the thing is missing" are different facts, and collapsing them
			// would blame the monitored subject for the monitor's own timeout.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return -1, out.String(), fmt.Errorf("probe did not finish within its deadline: %w", ctxErr)
			}
			if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
				return exitErr.ExitCode(), out.String(), nil
			}
			return -1, out.String(), err
		},
	}
}

// LoadPulseConfig reads and validates the pulse configuration. Any invalid
// pulse refuses the whole run (AA-19): a typo must not silently unmonitor
// part of the fleet.
func LoadPulseConfig(path string) ([]Pulse, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pulse config: %w", err)
	}
	var cfg PulseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse pulse config %s: %w", path, err)
	}
	if len(cfg.Pulses) == 0 {
		return nil, fmt.Errorf("pulse config %s: no pulses configured", path)
	}
	seen := make(map[string]bool, len(cfg.Pulses))
	for i := range cfg.Pulses {
		p := &cfg.Pulses[i]
		if err := validatePulse(p); err != nil {
			return nil, fmt.Errorf("pulse config %s: %w", path, err)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("pulse config %s: duplicate pulse name %q", path, p.Name)
		}
		seen[p.Name] = true
	}
	return cfg.Pulses, nil
}

func validatePulse(p *Pulse) error {
	if p.Name == "" {
		return fmt.Errorf("pulse with empty name")
	}
	switch p.Type {
	case PulseFileMtime:
		if p.Path == "" {
			return fmt.Errorf("pulse %q: file_mtime requires path", p.Name)
		}
		if p.Window == "" {
			return fmt.Errorf("pulse %q: file_mtime requires window", p.Name)
		}
		w, err := time.ParseDuration(p.Window)
		if err != nil {
			return fmt.Errorf("pulse %q: bad window %q: %w", p.Name, p.Window, err)
		}
		if w <= 0 {
			return fmt.Errorf("pulse %q: window must be positive, got %q", p.Name, p.Window)
		}
		p.window = w
		p.Path = expandHome(p.Path)
	case PulseLaunchdLoaded:
		if p.Label == "" {
			return fmt.Errorf("pulse %q: launchd_loaded requires label", p.Name)
		}
	case PulseCommand:
		if len(p.Command) == 0 {
			return fmt.Errorf("pulse %q: command requires a non-empty command", p.Name)
		}
		for i, a := range p.Command {
			p.Command[i] = expandHome(a)
		}
	case PulseJSONTimestamp:
		if p.Path == "" {
			return fmt.Errorf("pulse %q: json_timestamp requires path", p.Name)
		}
		if p.Field == "" {
			return fmt.Errorf("pulse %q: json_timestamp requires field", p.Name)
		}
		if p.Window == "" {
			return fmt.Errorf("pulse %q: json_timestamp requires window", p.Name)
		}
		w, err := time.ParseDuration(p.Window)
		if err != nil {
			return fmt.Errorf("pulse %q: bad window %q: %w", p.Name, p.Window, err)
		}
		if w <= 0 {
			return fmt.Errorf("pulse %q: window must be positive, got %q", p.Name, p.Window)
		}
		p.window = w
		p.Path = expandHome(p.Path)
	default:
		return fmt.Errorf("pulse %q: unknown type %q", p.Name, p.Type)
	}
	return nil
}

// expandHome rewrites a leading "~/" to the current user's home directory so
// pulse configs stay host-portable.
func expandHome(s string) string {
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(s, "~"), "/"))
		}
	}
	return s
}

// statMtimeBounded runs the context-free stat hook under the probe deadline.
// A stat on a stalled network or FUSE mount is not interruptible, so the
// goroutine can outlive this call; it holds only the probe's own arguments and
// ends when the kernel returns. Bounding the wait is the point: without it one
// wedged path stops every later pulse, its notifications, and the heartbeat,
// which is precisely the silent failure this alarm exists to detect.
func statMtimeBounded(
	ctx context.Context,
	stat func(string) (time.Time, bool, error),
	path string,
) (time.Time, bool, error) {
	type statResult struct {
		mtime  time.Time
		exists bool
		err    error
	}
	done := make(chan statResult, 1)
	go func() {
		// The hook is injected and runs on a goroutine that deliberately
		// outlives this call, so a panic here cannot reach the caller's stack
		// and would take the whole tick down: no later pulses, no
		// notifications, no heartbeat. Turn it into an ordinary probe failure
		// so AA-05 classifies the pulse UNDETERMINED instead.
		defer func() {
			if r := recover(); r != nil {
				done <- statResult{err: fmt.Errorf("stat hook panicked: %v", r)}
			}
		}()
		mtime, exists, err := stat(path)
		done <- statResult{mtime, exists, err}
	}()
	select {
	case r := <-done:
		return r.mtime, r.exists, r.err
	case <-ctx.Done():
		return time.Time{}, false, fmt.Errorf("did not complete within the probe deadline: %w", ctx.Err())
	}
}

// readFileBounded runs the context-free read hook under the probe deadline.
func readFileBounded(
	ctx context.Context,
	readFile func(string) ([]byte, error),
	path string,
) ([]byte, error) {
	if readFile == nil {
		readFile = os.ReadFile
	}
	type readResult struct {
		data []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- readResult{err: fmt.Errorf("read hook panicked: %v", r)}
			}
		}()
		data, err := readFile(path)
		done <- readResult{data, err}
	}()
	select {
	case r := <-done:
		return r.data, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("did not complete within the probe deadline: %w", ctx.Err())
	}
}

// extractJSONTimestamp parses JSON data and extracts the timestamp at fieldPath.
func extractJSONTimestamp(data []byte, fieldPath string) (time.Time, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return time.Time{}, fmt.Errorf("parsing JSON: %w", err)
	}
	curr := root
	for part := range strings.SplitSeq(fieldPath, ".") {
		m, ok := curr.(map[string]any)
		if !ok {
			return time.Time{}, fmt.Errorf("field %q: not a JSON object", part)
		}
		val, exists := m[part]
		if !exists {
			return time.Time{}, fmt.Errorf("field %q not found", fieldPath)
		}
		curr = val
	}
	return parseTimestampValue(curr)
}

func parseTimestampValue(val any) (time.Time, error) {
	switch v := val.(type) {
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t, nil
		}
		if t, err := time.Parse(time.DateTime, v); err == nil {
			return t, nil
		}
		if t, err := time.Parse(time.DateOnly, v); err == nil {
			return t, nil
		}
		return time.Time{}, fmt.Errorf("cannot parse timestamp string %q", v)
	case float64:
		return parseNumericTimestamp(int64(v))
	case int64:
		return parseNumericTimestamp(v)
	case int:
		return parseNumericTimestamp(int64(v))
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return parseNumericTimestamp(n)
		}
		if f, err := v.Float64(); err == nil {
			return parseNumericTimestamp(int64(f))
		}
		return time.Time{}, fmt.Errorf("cannot parse json.Number %q", v.String())
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type %T", val)
	}
}

func parseNumericTimestamp(n int64) (time.Time, error) {
	switch {
	case n > 1e18:
		return time.Unix(0, n), nil
	case n > 1e15:
		return time.UnixMicro(n), nil
	case n > 1e12:
		return time.UnixMilli(n), nil
	default:
		return time.Unix(n, 0), nil
	}
}

// EvaluatePulse Probes one pulse and classifies it (AA-01..AA-06).
func EvaluatePulse(ctx context.Context, p Pulse, pr Probes, launchdListing string, launchdErr error) Result {
	res := Result{Name: p.Name, Expect: p.Expect, Window: p.Window}
	switch p.Type {
	case PulseFileMtime:
		// Evidence defaults to the moment the file was probed (AA-09),
		// ensuring missing or error records carry an observation timestamp.
		// If the file exists, it is updated below to the actual mtime.
		res.Evidence = pr.Now()
		mtime, exists, err := statMtimeBounded(ctx, pr.StatMtime, p.Path)
		if err != nil {
			res.Status = StatusUndetermined
			res.Reason = fmt.Sprintf("stat %s: %v", p.Path, err)
			return res
		}
		if !exists {
			res.Status = StatusAbsent
			res.Reason = fmt.Sprintf("%s does not exist", p.Path)
			return res
		}
		res.Evidence = mtime
		now := pr.Now()
		if mtime.After(now.Add(clockSkewTolerance)) {
			res.Status = StatusUndetermined
			res.Reason = fmt.Sprintf("%s modified %s in the future", p.Path, mtime.Sub(now).Round(time.Second))
			return res
		}
		if age := now.Sub(mtime); age > p.window {
			res.Status = StatusAbsent
			res.Reason = fmt.Sprintf("%s last modified %s ago (window %s)", p.Path, age.Round(time.Minute), p.Window)
			return res
		}
		res.Status = StatusPresent
	case PulseLaunchdLoaded:
		// Evidence = the moment the listing was obtained (AA-09); set before any
		// early return so error records also carry an observation timestamp.
		res.Evidence = pr.Now()
		if launchdErr != nil {
			res.Status = StatusUndetermined
			res.Reason = fmt.Sprintf("launchctl list: %v", launchdErr)
			return res
		}
		if !launchdListingContains(launchdListing, p.Label) {
			res.Status = StatusAbsent
			res.Reason = fmt.Sprintf("launchd job %s is not loaded", p.Label)
			return res
		}
		res.Status = StatusPresent
	case PulseCommand:
		code, output, err := pr.RunCommand(ctx, p.Command)
		// Evidence = the moment the command returned (AA-09); set before any
		// early return so timeout/error records also carry an observation timestamp.
		res.Evidence = pr.Now()
		if err != nil {
			res.Status = StatusUndetermined
			res.Reason = fmt.Sprintf("run %s: %v", strings.Join(p.Command, " "), err)
			return res
		}
		if code != 0 {
			res.Status = StatusAbsent
			res.Reason = commandReason(p.Command, code, output)
			return res
		}
		res.Status = StatusPresent
	case PulseJSONTimestamp:
		res.Evidence = pr.Now()
		data, err := readFileBounded(ctx, pr.ReadFile, p.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
				res.Status = StatusAbsent
				res.Reason = fmt.Sprintf("%s does not exist", p.Path)
				return res
			}
			res.Status = StatusUndetermined
			res.Reason = fmt.Sprintf("read %s: %v", p.Path, err)
			return res
		}
		ts, err := extractJSONTimestamp(data, p.Field)
		if err != nil {
			res.Status = StatusUndetermined
			res.Reason = fmt.Sprintf("%s: %v", p.Path, err)
			return res
		}
		res.Evidence = ts
		now := pr.Now()
		if ts.After(now.Add(clockSkewTolerance)) {
			res.Status = StatusUndetermined
			res.Reason = fmt.Sprintf("%s field %q timestamp %s in the future", p.Path, p.Field, ts.Sub(now).Round(time.Second))
			return res
		}
		if age := now.Sub(ts); age > p.window {
			res.Status = StatusAbsent
			res.Reason = fmt.Sprintf("%s field %q timestamp is %s old (window %s)", p.Path, p.Field, age.Round(time.Minute), p.Window)
			return res
		}
		res.Status = StatusPresent
	}
	return res
}

// maxReasonBytes bounds how much probe output is carried into an escalation.
// Probe output is untrusted in length, and this text ends up in a desktop
// notification and an append-only journal, so a runaway probe must not be able
// to flood either. The limit is generous enough for a full probe summary.
const maxReasonBytes = 1200

// commandReason builds the escalation text for a failing command pulse.
//
// It prefers the probe's own stdout over a bare exit code. A probe that failed
// already knows which check broke and what to do about it, and discarding that
// reproduces inside the alarm the exact failure this package exists to end: a
// monitor that reports something is wrong without saying what. The command and
// exit code are still appended so the reason stays self-locating.
func commandReason(argv []string, code int, output string) string {
	joined := strings.Join(argv, " ")
	summary := strings.TrimSpace(output)
	if summary == "" {
		return fmt.Sprintf("%s exited %d", joined, code)
	}
	if len(summary) > maxReasonBytes {
		limit := maxReasonBytes
		for limit > 0 && (summary[limit]&0xC0 == 0x80) {
			limit--
		}
		summary = summary[:limit] + "\n... (truncated)"
	}
	return fmt.Sprintf("%s\n(%s exited %d)", summary, joined, code)
}

// launchdListingContains reports whether a `launchctl list` output line names
// the label exactly (the label is the third whitespace-separated column).
func launchdListingContains(listing, label string) bool {
	for line := range strings.SplitSeq(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == label {
			return true
		}
	}
	return false
}
