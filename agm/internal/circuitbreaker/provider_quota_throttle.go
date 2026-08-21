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
// quota.Breaker's in-memory admitted map, because pkg/llm/quota cannot
// import agm/internal/lock — the flock helper is scoped to the agm
// module tree.
//
// admission.go calls into this in two layers: a cheap, read-only,
// advisory check (recentThrottledAdmissions, via checkProviderQuota) that
// can refuse early before any launch preparation happens, and the
// authoritative ReserveThrottledAdmission — a single atomic
// check-then-append under one flock — called once a spawn has passed
// every other live gate and is about to proceed. The advisory check
// alone is not enough: two concurrent agm processes can each observe
// "under the limit" before either has recorded its own admission, and
// both proceed. Only a check and an append performed under the same lock
// closes that race; that is what ReserveThrottledAdmission does, and why
// admission.go's beforeSpawn can still refuse a spawn at that point even
// after the earlier advisory check passed it (codex review on #1218,
// third pass).
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

// ReserveThrottledAdmission atomically checks family's hourly admission
// count against limit and, only if it is still under the limit, records
// this admission — all under the same cross-process lock. allowed is
// meaningful only when err is nil; a non-nil err means the ledger could
// not be evaluated at all (fail open, like every other quota read in this
// package: unreadable is never evidence of exhaustion).
//
// The check-then-append MUST happen under one lock, not as two separate
// calls (a read via recentThrottledAdmissions, then a write via a
// separate append): two concurrent agm processes each observing
// count < limit before either has written would otherwise both proceed,
// admitting one more start than the limit allows — worse under a larger
// number of concurrent starts. Callers record exactly once per spawn that
// actually launches — after every live gate has passed and the spawn is
// truly proceeding, never at a preflight check that might not lead to a
// launch — so the ledger reflects real load rather than double-counting
// the same spawn's repeated admission checks.
func ReserveThrottledAdmission(family string, limit int, now time.Time) (allowed bool, err error) {
	if family == "" {
		return true, nil
	}
	if limit < 0 {
		return true, nil
	}
	return reserveThrottledAdmission(family, limit, now)
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

// reserveThrottledAdmission is ReserveThrottledAdmission's implementation:
// prune entries older than an hour for every family, check family's
// remaining count against limit, and append only if it is still under —
// all while holding the ledger's flock, so a concurrent agm process
// cannot observe the pre-append count and race this one.
func reserveThrottledAdmission(family string, limit int, now time.Time) (bool, error) {
	path, err := throttleLedgerPath()
	if err != nil {
		return true, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return true, err
	}

	fl, err := lock.New(path + ".lock")
	if err != nil {
		return true, err
	}
	if err := fl.Lock(); err != nil {
		return true, err
	}
	defer fl.Unlock()

	entries, err := readThrottleLedger(path)
	if err != nil {
		return true, err
	}

	cutoff := now.Add(-time.Hour)
	pruned := pruneExpiredEntries(entries, cutoff, family)
	keptFamily := activeEntriesSince(entries[family], cutoff)

	allowed := len(keptFamily) < limit
	if allowed {
		keptFamily = append(keptFamily, now)
	}
	if len(keptFamily) > 0 {
		pruned[family] = keptFamily
	}

	if err := writeThrottleLedger(path, pruned); err != nil {
		return true, err
	}
	return allowed, nil
}

// pruneExpiredEntries copies entries, dropping every timestamp at or
// before cutoff and omitting exceptFamily entirely — the caller computes
// exceptFamily's kept slice separately, since a reservation may still
// append to it.
func pruneExpiredEntries(entries map[string][]time.Time, cutoff time.Time, exceptFamily string) map[string][]time.Time {
	pruned := make(map[string][]time.Time, len(entries))
	for fam, ats := range entries {
		if fam == exceptFamily {
			continue
		}
		if kept := activeEntriesSince(ats, cutoff); len(kept) > 0 {
			pruned[fam] = kept
		}
	}
	return pruned
}

// activeEntriesSince returns the timestamps in ats that are after cutoff.
func activeEntriesSince(ats []time.Time, cutoff time.Time) []time.Time {
	var kept []time.Time
	for _, at := range ats {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	return kept
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
