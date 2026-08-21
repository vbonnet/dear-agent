package circuitbreaker

// Persistent per-family admission-throttle ledger.
//
// quota.SpawnGate.AllowSpawn reads a published, precomputed verdict: when
// a family is BreakerThrottled it currently just passes, because the
// verdict was frozen at publish time by the scheduled refresh job, not
// evaluated against a live admission count. quota.Breaker.Admit does
// implement the documented hourly allowance, but its counter lives in an
// in-memory map on a *Breaker that only ever exists inside `agm
// quota-meter --refresh` — a fresh process every 30 minutes — and is
// never consulted from the spawn path at all. Nothing enforces the
// throttle band; a family that has crossed into it can still admit an
// unbounded burst of spawns (codex review on #1218).
//
// This ledger is the file-backed equivalent for the process that does
// sit on the spawn path: agm itself. It is deliberately independent of
// quota.Breaker's in-memory admitted map (pkg/llm/quota cannot import
// agm/internal/lock — the flock helper is scoped to the agm module tree)
// and deliberately simpler: it records every admission agm actually
// authorizes for a family regardless of the breaker state at the moment
// of recording, and the hourly cap is enforced only when reading, and
// only while the family is currently throttled. That keeps the write
// path (admission.go's afterAuthorization, called once per authorized
// spawn) unconditional and cheap, and keeps the enforcement decision
// exactly where the finding says it belongs: the final admission
// boundary.
//
// Every failure mode here fails open, matching the rest of the quota
// package's philosophy: a ledger this process cannot read or write is
// not evidence a family is over its allowance, and blocking a spawn on
// lock-file infrastructure would be a worse outcome than the burst this
// ledger exists to bound.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// throttleLedgerPath resolves the ledger file's location. A var (not a
// const) so tests can point it at a temp directory without depending on
// XDG_STATE_HOME/HOME process-global state.
var throttleLedgerPath = quota.DefaultThrottleLedgerPath

// RecordProviderQuotaAdmission records one admission for family in the
// persistent throttle ledger. Callers record exactly once per spawn that
// actually launches — after every live gate has passed and the spawn is
// truly proceeding, never at a preflight check that might not lead to a
// launch — so the ledger reflects real load rather than double-counting
// the same spawn's repeated admission checks.
func RecordProviderQuotaAdmission(family string, now time.Time) error {
	if family == "" {
		return nil
	}
	return recordThrottledAdmission(family, now)
}

// recentThrottledAdmissions reports how many admissions family has
// recorded in the last hour. It fails open (0, nil error semantics for
// callers that treat an error as "cannot evaluate"): a missing ledger is
// the normal state before any family has ever been throttled.
func recentThrottledAdmissions(family string, now time.Time) (int, error) {
	path, err := throttleLedgerPath()
	if err != nil {
		return 0, err
	}
	entries, err := readThrottleLedger(path)
	if err != nil {
		return 0, err
	}
	cutoff := now.Add(-time.Hour)
	count := 0
	for _, at := range entries[family] {
		if at.After(cutoff) {
			count++
		}
	}
	return count, nil
}

// recordThrottledAdmission appends one admission timestamp for family,
// pruning entries older than an hour for every family while it has the
// ledger open. Concurrent agm processes serialize on an flock so a
// read-modify-write race cannot drop an update.
func recordThrottledAdmission(family string, now time.Time) error {
	path, err := throttleLedgerPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	fl, err := lock.New(path + ".lock")
	if err != nil {
		return err
	}
	if err := fl.Lock(); err != nil {
		return err
	}
	defer fl.Unlock()

	entries, err := readThrottleLedger(path)
	if err != nil {
		return err
	}
	cutoff := now.Add(-time.Hour)
	pruned := make(map[string][]time.Time, len(entries))
	for fam, ats := range entries {
		var kept []time.Time
		for _, at := range ats {
			if at.After(cutoff) {
				kept = append(kept, at)
			}
		}
		if fam == family {
			kept = append(kept, now)
		}
		if len(kept) > 0 {
			pruned[fam] = kept
		}
	}
	if _, ok := pruned[family]; !ok {
		pruned[family] = []time.Time{now}
	}
	return writeThrottleLedger(path, pruned)
}

func readThrottleLedger(path string) (map[string][]time.Time, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]time.Time{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return map[string][]time.Time{}, nil
	}
	entries := map[string][]time.Time{}
	if err := json.Unmarshal(data, &entries); err != nil {
		// A corrupt ledger is not evidence of an exhausted allowance —
		// treat it the same as an empty one rather than failing the
		// caller closed.
		return map[string][]time.Time{}, nil //nolint:nilerr // intentional: corrupt ledger degrades to empty, not an error
	}
	return entries, nil
}

// writeThrottleLedger publishes entries atomically (temp file + rename),
// the same pattern quota.WriteStateFile uses: other processes may read
// this file at arbitrary moments and must never see a half-written one.
func writeThrottleLedger(path string, entries map[string][]time.Time) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".throttle-ledger-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
