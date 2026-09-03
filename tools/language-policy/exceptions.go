// Package main implements the repository's shell-script language policy check.
//
// The exception store is line-oriented JSON (`.jsonl`) rather than a binary
// database on purpose: every waiver is a single line, so `git blame` attributes
// it to the commit and author that granted it, and a reviewer can read the
// store without installing a database client. See ADR and the retro referenced
// in .github/language-policy/README.md.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Exception is one waiver line in the JSONL store.
//
// Field order here is the field order emitted on disk; keep them in sync so a
// rewritten store produces a minimal diff.
type Exception struct {
	Rule     string  `json:"rule"`
	Path     string  `json:"path"`
	Status   string  `json:"status"`
	Reason   string  `json:"reason"`
	Approver string  `json:"approver"`
	Sunset   *string `json:"sunset"`
	Added    string  `json:"added"`
}

// validStatuses are the only statuses the checker understands. An unrecognised
// status is a hard error rather than a silent "not active": a typo like
// "activ" would otherwise revoke a waiver without anyone noticing.
var validStatuses = map[string]bool{
	"active":        true,
	"grandfathered": true,
	"revoked":       true,
	"expired":       true,
}

// normalizePath makes store paths and filesystem paths comparable. The store
// holds repo-relative paths with no leading "./"; callers may pass either form.
func normalizePath(p string) string {
	p = strings.TrimPrefix(p, "./")
	return strings.TrimPrefix(p, "/")
}

// Store is the parsed exception set, indexed for lookup.
type Store struct {
	All   []Exception
	byKey map[string]Exception
}

func key(rule, path string) string { return rule + "\x00" + normalizePath(path) }

// LoadStore parses a JSONL exception store.
//
// Parse errors are fatal and name the line number: a malformed store must never
// degrade to "no exceptions found", which would fail every waived script at
// once and look like a mass policy violation instead of a corrupt file.
func LoadStore(r io.Reader) (*Store, error) {
	s := &Store{byKey: map[string]Exception{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Exception
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", n, err)
		}
		if e.Rule == "" || e.Path == "" {
			return nil, fmt.Errorf("line %d: rule and path are required", n)
		}
		if !validStatuses[e.Status] {
			return nil, fmt.Errorf("line %d: unknown status %q", n, e.Status)
		}
		k := key(e.Rule, e.Path)
		if _, dup := s.byKey[k]; dup {
			return nil, fmt.Errorf("line %d: duplicate entry for rule %q path %q", n, e.Rule, e.Path)
		}
		s.byKey[k] = e
		s.All = append(s.All, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading store: %w", err)
	}
	return s, nil
}

// Active reports whether an unexpired waiver covers rule/path as of `now`.
func (s *Store) Active(rule, path string, now time.Time) bool {
	e, ok := s.byKey[key(rule, path)]
	if !ok || e.Status == "revoked" || e.Status == "expired" {
		return false
	}
	if e.Sunset != nil {
		d, err := time.Parse("2006-01-02", *e.Sunset)
		// An unparseable sunset date is treated as expired rather than
		// ignored, so a typo fails closed instead of granting a waiver
		// that outlives its intended window.
		if err != nil || !d.After(now) {
			return false
		}
	}
	return true
}

// Expired lists waivers whose sunset date has passed, for the nightly sweep.
func (s *Store) Expired(now time.Time) []Exception {
	var out []Exception
	for _, e := range s.All {
		if e.Status == "revoked" || e.Status == "expired" {
			continue
		}
		if e.Sunset == nil {
			continue
		}
		if d, err := time.Parse("2006-01-02", *e.Sunset); err != nil || !d.After(now) {
			out = append(out, e)
		}
	}
	return out
}

// ExpiringWithin lists active waivers whose sunset falls within the next
// `days` UTC calendar dates. Sunset dates are policy dates, not elapsed
// durations: a sweep's wall-clock hour must not move the inclusive boundary.
func (s *Store) ExpiringWithin(now time.Time, days int) []Exception {
	if days <= 0 {
		return nil
	}
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	horizon := today.AddDate(0, 0, days)

	var out []Exception
	for _, e := range s.All {
		if e.Status == "revoked" || e.Status == "expired" {
			continue
		}
		if e.Sunset == nil {
			continue
		}
		d, err := time.Parse("2006-01-02", *e.Sunset)
		if err != nil || !d.After(today) || d.After(horizon) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if *out[i].Sunset != *out[j].Sunset {
			return *out[i].Sunset < *out[j].Sunset
		}
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// CheckSorted verifies the store is sorted by (rule, path) and free of
// duplicates. Sorted order keeps insertions to a single-line diff, which is
// what makes per-line blame useful.
func CheckSorted(all []Exception) error {
	sorted := make([]Exception, len(all))
	copy(sorted, all)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Rule != sorted[j].Rule {
			return sorted[i].Rule < sorted[j].Rule
		}
		return sorted[i].Path < sorted[j].Path
	})
	for i := range all {
		if all[i].Rule != sorted[i].Rule || all[i].Path != sorted[i].Path {
			return fmt.Errorf("store is not sorted by (rule, path); first difference at entry %d (%s %s)", i+1, all[i].Rule, all[i].Path)
		}
	}
	return nil
}

// WriteStore emits the canonical on-disk form: one compact JSON object per
// line, sorted by (rule, path).
func WriteStore(w io.Writer, all []Exception) error {
	sorted := make([]Exception, len(all))
	copy(sorted, all)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Rule != sorted[j].Rule {
			return sorted[i].Rule < sorted[j].Rule
		}
		return sorted[i].Path < sorted[j].Path
	})
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	// Keep <, > and & literal. The default HTML escaping would render a
	// reason like "requires >20 lines" as "requires \u003e20 lines", which
	// defeats the point of a store a human can read in a diff.
	enc.SetEscapeHTML(false)
	for _, e := range sorted {
		// Encode writes its own trailing newline, which is the line
		// separator this format needs.
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// Baseline is the waiver ratchet: a ceiling on how many waivers a rule may
// carry. It exists because this store grew to 110 entries against 22 compliant
// scripts without any check observing it. The ceiling may only be lowered, so
// the backlog can shrink but never silently refill.
type Baseline struct {
	Rules map[string]struct {
		MaxWaivers int    `json:"max_waivers"`
		Goal       string `json:"goal"`
	} `json:"rules"`
}

// LoadBaseline reads the ratchet file.
func LoadBaseline(r io.Reader) (*Baseline, error) {
	var b Baseline
	if err := json.NewDecoder(r).Decode(&b); err != nil {
		return nil, fmt.Errorf("invalid baseline: %w", err)
	}
	return &b, nil
}

// CheckRatchet reports an error when a rule carries more waivers than its
// declared ceiling.
func (b *Baseline) CheckRatchet(rule string, count int) error {
	r, ok := b.Rules[rule]
	if !ok {
		return fmt.Errorf("no baseline declared for rule %q; add one to the baseline file", rule)
	}
	if count > r.MaxWaivers {
		return fmt.Errorf(
			"waiver ratchet exceeded for %s: %d waivers against a ceiling of %d. "+
				"Adding a waiver means a script was exempted rather than shortened or tested. "+
				"Add a test under %s/ or shorten the script instead of raising the ceiling",
			rule, count, r.MaxWaivers, testDir)
	}
	return nil
}
