// Command quota-meter reports remaining provider quota from the local
// CodexBar meter and shows how it reorders each role's candidate chain.
//
//	quota-meter                  # per-provider and per-sub-budget remaining
//	quota-meter --roles          # plus the quota-aware order for every role
//	quota-meter --json           # the same reading as JSON
//
// Exit status is 0 when a reading was produced, 1 when the meter could
// not be read at all, and 2 for a usage error. A provider that needs
// credentials is reported as unreadable, never as exhausted — the
// distinction is the point of the command.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
	"github.com/vbonnet/dear-agent/pkg/llm/router"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type config struct {
	command     string
	timeout     time.Duration
	maxAge      time.Duration
	avoidBelow  float64
	deprioBelow float64
	showRoles   bool
	rolesFile   string
	asJSON      bool
}

func run(args []string, stdout, stderr *os.File) int {
	var cfg config
	fs := flag.NewFlagSet("quota-meter", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.command, "command", quota.DefaultCodexBarCommand, "codexbar executable to read the meter from")
	fs.DurationVar(&cfg.timeout, "timeout", quota.DefaultReadTimeout, "maximum time to wait for one meter read")
	fs.DurationVar(&cfg.maxAge, "max-age", quota.DefaultMaxSnapshotAge, "reject a reading older than this")
	fs.Float64Var(&cfg.avoidBelow, "avoid-below", quota.DefaultAvoidBelowRemainingPercent, "remaining percent at or below which a provider is avoided")
	fs.Float64Var(&cfg.deprioBelow, "deprioritize-below", quota.DefaultDeprioritizeBelowRemainingPercent, "remaining percent at or below which a provider is deprioritized")
	fs.BoolVar(&cfg.showRoles, "roles", false, "also show the quota-aware candidate order for every configured role")
	fs.StringVar(&cfg.rolesFile, "roles-file", "", "path to the router roles config workflow-run uses (default: $DEAR_AGENT_ROLES → ./.dear-agent/roles.yaml → ~/.config/dear-agent/roles.yaml)")
	fs.BoolVar(&cfg.asJSON, "json", false, "emit JSON instead of text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "quota-meter: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	meter := quota.New(quota.Options{
		Reader: quota.CodexBarReader{Command: cfg.command, Timeout: cfg.timeout},
		Policy: quota.Policy{
			AvoidBelowRemainingPercent:        cfg.avoidBelow,
			DeprioritizeBelowRemainingPercent: cfg.deprioBelow,
			MaxSnapshotAge:                    cfg.maxAge,
		},
		// Only the explicit read below should touch the meter.
		RefreshInterval: -1,
	})

	snapshot, readErr := meter.Refresh(context.Background())
	if readErr != nil {
		fmt.Fprintf(stderr, "quota-meter: %v\n", readErr)
		return 1
	}

	report := buildReport(snapshot, meter, cfg)
	if cfg.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "quota-meter: encode json: %v\n", err)
			return 1
		}
		return 0
	}
	writeText(stdout, report, cfg)
	return 0
}

// report is the command's machine-readable output.
type report struct {
	Source        string    `json:"source"`
	SourceVersion string    `json:"sourceVersion,omitempty"`
	GeneratedAt   time.Time `json:"generatedAt"`
	AgeSeconds    float64   `json:"ageSeconds"`
	MaxAgeSeconds float64   `json:"maxAgeSeconds"`
	// SourceStaleAfterSeconds is the meter's own freshness hint, for
	// operators tuning --max-age. It does not itself gate anything.
	SourceStaleAfterSeconds float64          `json:"sourceStaleAfterSeconds,omitempty"`
	Stale                   bool             `json:"stale"`
	Providers               []providerReport `json:"providers"`
	Roles                   []roleReport     `json:"roles,omitempty"`
	RolesSource             string           `json:"rolesSource,omitempty"`
}

type providerReport struct {
	Family            string         `json:"family"`
	SourceID          string         `json:"sourceId"`
	Account           string         `json:"account,omitempty"`
	Plan              string         `json:"plan,omitempty"`
	Availability      string         `json:"availability"`
	Readable          bool           `json:"readable"`
	Class             string         `json:"class"`
	RemainingPercent  *float64       `json:"remainingPercent,omitempty"`
	ConstrainedWindow string         `json:"constrainedWindow,omitempty"`
	Reason            string         `json:"reason"`
	Windows           []windowReport `json:"windows"`
}

type windowReport struct {
	ID               string     `json:"id,omitempty"`
	Label            string     `json:"label,omitempty"`
	RemainingPercent float64    `json:"remainingPercent"`
	UsedPercent      float64    `json:"usedPercent"`
	ResetAt          *time.Time `json:"resetAt,omitempty"`
}

type roleReport struct {
	Role       string   `json:"role"`
	Configured []string `json:"configuredOrder"`
	Effective  []string `json:"effectiveOrder"`
	Reordered  bool     `json:"reordered"`
	Notes      []string `json:"notes,omitempty"`
}

func buildReport(snapshot *quota.Snapshot, meter *quota.Meter, cfg config) report {
	now := time.Now()
	rep := report{
		MaxAgeSeconds: cfg.maxAge.Seconds(),
	}
	if snapshot != nil {
		rep.Source = snapshot.Source
		rep.SourceVersion = snapshot.SourceVersion
		rep.GeneratedAt = snapshot.GeneratedAt
		rep.AgeSeconds = snapshot.Age(now).Seconds()
		rep.SourceStaleAfterSeconds = snapshot.StaleAfter.Seconds()
		rep.Stale = cfg.maxAge > 0 && snapshot.Age(now) > cfg.maxAge
		for _, p := range snapshot.Providers {
			rep.Providers = append(rep.Providers, buildProvider(p, meter))
		}
	}
	if cfg.showRoles {
		rolesReport, source := buildRoles(meter, cfg.rolesFile)
		rep.Roles = rolesReport
		rep.RolesSource = source
	}
	return rep
}

func buildProvider(p quota.ProviderQuota, meter *quota.Meter) providerReport {
	decision := meter.DecisionFor(p.Family)
	out := providerReport{
		Family:            p.Family,
		SourceID:          p.SourceID,
		Account:           p.Account,
		Plan:              p.Plan,
		Availability:      string(p.Availability),
		Readable:          p.Availability.Known(),
		Class:             string(decision.Class),
		ConstrainedWindow: decision.ConstrainedWindow,
		Reason:            decision.Reason,
	}
	if decision.Known() {
		remaining := decision.RemainingPercent
		out.RemainingPercent = &remaining
	}
	for _, w := range p.Windows {
		wr := windowReport{
			ID:               w.ID,
			Label:            w.Label,
			RemainingPercent: w.RemainingPercent,
			UsedPercent:      w.UsedPercent,
		}
		if !w.ResetAt.IsZero() {
			reset := w.ResetAt
			wr.ResetAt = &reset
		}
		out.Windows = append(out.Windows, wr)
	}
	return out
}

func buildRoles(meter *quota.Meter, rolesFile string) ([]roleReport, string) {
	cfg, source, err := loadRouterConfig(rolesFile)
	if err != nil {
		return []roleReport{{Role: "(none)", Notes: []string{fmt.Sprintf("load roles: %v", err)}}}, source
	}
	names := make([]string, 0, len(cfg.Roles))
	for name := range cfg.Roles {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []roleReport
	for _, name := range names {
		configured := cfg.Roles[name].Candidates()
		if len(configured) == 0 {
			continue
		}
		effective, decisions := meter.OrderModels(configured)
		entry := roleReport{
			Role:       name,
			Configured: configured,
			Effective:  effective,
			Reordered:  !equalSlices(configured, effective),
		}
		for i, model := range effective {
			if d := decisions[i]; d.Known() {
				entry.Notes = append(entry.Notes, fmt.Sprintf("%s: %s (%s)", model, d.Class, d.Reason))
			}
		}
		out = append(out, entry)
	}
	return out, source
}

// loadRouterConfig reads the same roles config schema workflow-run's
// buildExecutor feeds to router.New, so the preview this command shows
// matches what actually runs. It previously loaded pkg/workflow/roles'
// tier-object schema (primary: {model: ...}), which is not what
// router.LoadConfig accepts (primary: <model string>) — a router role file
// parsed as a roles.Registry silently produced no candidates instead of
// previewing the real chain.
func loadRouterConfig(explicitPath string) (*router.Config, string, error) {
	path := resolveRolesPath(explicitPath)
	if path == "" {
		return nil, "", errors.New("no roles file found ($DEAR_AGENT_ROLES, ./.dear-agent/roles.yaml, ~/.config/dear-agent/roles.yaml)")
	}
	cfg, err := router.LoadConfig(path)
	if err != nil {
		return nil, path, err
	}
	return cfg, path, nil
}

// resolveRolesPath mirrors the search order the -roles-file flag help text
// documents: an explicit path first, then $DEAR_AGENT_ROLES, then
// cwd-relative .dear-agent/roles.yaml, then the operator's
// ~/.config/dear-agent/roles.yaml. Returns "" when nothing exists.
func resolveRolesPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	var candidates []string
	if env := os.Getenv("DEAR_AGENT_ROLES"); env != "" {
		candidates = append(candidates, env)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".dear-agent", "roles.yaml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "dear-agent", "roles.yaml"))
	}
	for _, p := range candidates {
		// #nosec G703 -- p is one of a fixed set of operator-controlled
		// locations (env var, cwd-relative, home-relative), the same
		// existence check pkg/workflow/roles.AutoLoad performs; this only
		// decides which path to hand to router.LoadConfig, it never reads
		// file contents itself.
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeText(stdout *os.File, rep report, cfg config) {
	age := time.Duration(rep.AgeSeconds * float64(time.Second)).Round(time.Second)
	staleNote := ""
	if rep.Stale {
		staleNote = "  STALE"
	}
	sourceHint := ""
	if rep.SourceStaleAfterSeconds > 0 {
		sourceHint = fmt.Sprintf("   source stale-after: %s",
			time.Duration(rep.SourceStaleAfterSeconds*float64(time.Second)).Round(time.Second))
	}
	fmt.Fprintf(stdout, "source: %s %s   generated: %s   age: %s (max %s)%s%s\n\n",
		orDash(rep.Source), orDash(rep.SourceVersion),
		rep.GeneratedAt.Format(time.RFC3339), age, cfg.maxAge, sourceHint, staleNote)

	for _, p := range rep.Providers {
		remaining := "     —"
		if p.RemainingPercent != nil {
			remaining = fmt.Sprintf("%5.1f%%", *p.RemainingPercent)
		}
		fmt.Fprintf(stdout, "%-12s %-10s %-14s %s  %s\n",
			p.SourceID, p.Family, p.Class, remaining, p.Reason)
		for _, w := range p.Windows {
			reset := ""
			if w.ResetAt != nil {
				reset = "  resets " + w.ResetAt.Format(time.RFC3339)
			}
			fmt.Fprintf(stdout, "%-12s   · %-28s %5.1f%% left (%.1f%% used)%s\n",
				"", labelOf(w), w.RemainingPercent, w.UsedPercent, reset)
		}
	}

	if !cfg.showRoles {
		return
	}
	fmt.Fprintf(stdout, "\nroles: %s\n", orDash(rep.RolesSource))
	for _, r := range rep.Roles {
		marker := "  "
		if r.Reordered {
			marker = "→ "
		}
		fmt.Fprintf(stdout, "%s%-14s %s\n", marker, r.Role, strings.Join(r.Effective, " → "))
		if r.Reordered {
			fmt.Fprintf(stdout, "  %-14s (configured: %s)\n", "", strings.Join(r.Configured, " → "))
		}
		for _, note := range r.Notes {
			fmt.Fprintf(stdout, "  %-14s   %s\n", "", note)
		}
	}
}

func labelOf(w windowReport) string {
	switch {
	case w.Label != "":
		return w.Label
	case w.ID != "":
		return w.ID
	default:
		return "usage window"
	}
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
