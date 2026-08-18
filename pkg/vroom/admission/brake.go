// Package admission carries the cross-process spawn brake: a small, TTL'd
// latch that host watchdogs engage when they can no longer vouch for the
// machine, and that every spawn path reads before admitting new work.
//
// It exists because of the 2026-07-18 host saturation (ce-93lw.18). At the time
// of that incident disk-watchdog was in ALARM and its own remediation
// (`agm worktree sweep --execute`) was being SIGKILLed by the exhaustion it was
// trying to relieve — the single clearest signal that the host was in trouble —
// and nothing consumed it. The mesh kept spawning until the owner had to
// power-cycle the machine.
//
// The brake is a file rather than a daemon on purpose. The failure being
// guarded against is processes dying under resource pressure; a latch that is
// already on disk keeps refusing spawns after every writer is dead, whereas a
// daemon would have to be supervised, restarted, and treated as
// unreachable-means-refuse anyway.
//
// Semantics:
//
//   - Absent file means healthy. The common case costs one failed open(2).
//     This is deliberately not a required-heartbeat design: "no fresh healthy
//     record means block" would wedge the host the first time launchd is
//     disabled.
//   - Unreadable or unparseable file means engaged. A brake we cannot read is
//     not evidence of health.
//   - Every brake expires. A fail-closed latch with no TTL turns one transient
//     probe failure into a permanently dead mesh.
package admission

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultTTL is how long an engaged brake blocks spawns before expiring on its
// own. Six disk-watchdog ticks (which run every 5 minutes), so a host that
// recovers is picked up by a healthy tick long before the TTL matters, and a
// host whose watchdogs are all dead still un-brakes itself eventually.
const DefaultTTL = 30 * time.Minute

// brakeFile is the basename of the latch inside the AGM config directory.
const brakeFile = "admission-brake.json"

// Brake is the on-disk record. Timestamps are UTC so a record written before a
// DST shift compares correctly against one read after it.
type Brake struct {
	// Source names the writer (e.g. "disk-watchdog", "vroom-governor"). It is
	// echoed in the spawn refusal so an operator knows which watchdog to look at.
	Source string `json:"source"`
	// Reason is the human-readable cause, shown verbatim in the refusal.
	Reason string `json:"reason"`
	// SetAtUTC is when the brake was engaged.
	SetAtUTC time.Time `json:"set_at_utc"`
	// ExpiresUTC is when the brake stops blocking spawns. Never zero: a record
	// without an expiry is treated as malformed rather than as permanent.
	ExpiresUTC time.Time `json:"expires_utc"`
}

// Expired reports whether the brake is past its expiry as of now.
func (b Brake) Expired(now time.Time) bool {
	return !now.Before(b.ExpiresUTC)
}

// Age returns how long the brake has been engaged as of now.
func (b Brake) Age(now time.Time) time.Duration {
	return now.Sub(b.SetAtUTC)
}

// DefaultPath returns the canonical brake location,
// $AGM_CONFIG_DIR/admission-brake.json, falling back to
// ~/.agm/admission-brake.json. It mirrors how FileSpawnTimer resolves
// last-spawn.txt so both live in the same directory.
func DefaultPath() string {
	dir := os.Getenv("AGM_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			// No home and no override: keep a relative path rather than
			// returning "" so callers still get a usable, nameable location.
			return filepath.Join(".agm", brakeFile)
		}
		dir = filepath.Join(home, ".agm")
	}
	return filepath.Join(dir, brakeFile)
}

// Engage writes (or replaces) the brake at path, blocking spawns for ttl. A
// ttl of zero or less uses DefaultTTL, so a caller can never accidentally write
// a brake that is born expired.
//
// The write is atomic — temp file in the same directory, then rename(2) — so a
// reader can never observe a half-written record and mistake it for corruption.
func Engage(path, source, reason string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := time.Now().UTC()
	rec := Brake{
		Source:     source,
		Reason:     reason,
		SetAtUTC:   now,
		ExpiresUTC: now.Add(ttl),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding admission brake: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".admission-brake-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp brake file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup: harmless once the rename below has consumed the file.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp brake file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp brake file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp brake file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("installing brake at %s: %w", path, err)
	}
	return nil
}

// Release removes the brake at path unconditionally, whoever engaged it.
// Releasing an absent brake succeeds. This is the operator's escape hatch and
// the equivalent of deleting the file; watchdogs should use ReleaseBySource.
func Release(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("releasing brake at %s: %w", path, err)
	}
	return nil
}

// ReleaseBySource removes the brake at path only if that source engaged it.
// A brake engaged by a different watchdog is left alone, as is one that cannot
// be read.
//
// This scoping is what keeps two watchdogs on different cadences from undoing
// each other. vroom-governor ticks every 30 seconds and disk-watchdog every 5
// minutes; with an unconditional release, a governor whose load and memory
// probes are perfectly healthy would clear a disk-watchdog brake within half a
// minute of it being engaged — silently defeating the whole ce-93lw.18 fix on
// the most likely path, a host that is out of disk but not out of CPU.
//
// Releasing an absent or expired brake succeeds: both mean nothing is in force.
//
// There is a read-then-remove race here. If another writer engages a brake
// between the read and the remove, that brand-new brake is dropped. It is
// bounded and self-correcting — each watchdog re-engages on its next tick, at
// most 5 minutes later — and closing it properly would need file locking that
// buys less than it costs on a single-host latch.
func ReleaseBySource(path, source string) error {
	rec, err := Read(path)
	if err != nil {
		// Unreadable: leave it. An unparseable latch still refuses spawns, and
		// clearing what we cannot read would silently unblock the host.
		return fmt.Errorf("not releasing unreadable brake at %s: %w", path, err)
	}
	if rec == nil {
		return nil // absent or expired — nothing in force
	}
	if rec.Source != source {
		return nil // another watchdog's brake; not ours to clear
	}
	return Release(path)
}

// Read returns the live brake at path.
//
// It returns (nil, nil) when there is no brake in force — either the file does
// not exist or the record has expired. It returns an error when the file exists
// but cannot be read or parsed; callers must treat that as an engaged brake,
// because an unreadable latch is not evidence of a healthy host.
func Read(path string) (*Brake, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading admission brake %s: %w", path, err)
	}

	var rec Brake
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parsing admission brake %s: %w", path, err)
	}
	if rec.ExpiresUTC.IsZero() {
		return nil, fmt.Errorf("admission brake %s has no expiry timestamp", path)
	}
	if rec.Expired(time.Now().UTC()) {
		return nil, nil
	}
	return &rec, nil
}
