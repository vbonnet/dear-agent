// Package safegit breakglass.go implements the safe-merge P5 break-glass escape hatch
// (docs §4.4/§5): a deliberately high-friction, fully audited path to merge a
// PR when the normal gates cannot pass and a human has decided the risk is
// acceptable.
//
// Break-glass is for HUMANS ONLY. The TTY guard (RequireTTY) structurally
// excludes agent callers — an agent has no interactive terminal on stdin, so
// the command exits before doing anything. Every successful use:
//
//   - posts the typed reason as a PR comment (reason + operator + timestamp),
//   - appends an append-only JSONL audit record,
//   - files a P1 retro bead so the incident is reviewed,
//   - prints a DEAR retro notice.
//
// On ruleset-protected repos (branch protection with enforce_admins), a
// programmatic merge cannot succeed, so break-glass prints the exact
// Settings-edit instruction and exits 0 instead of attempting the merge.
package safegit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

// MinBreakGlassReason is the minimum length of the typed justification. A
// too-short reason is rejected: the audit trail must be meaningful.
const MinBreakGlassReason = 20

// breakGlassBeadDB is the beads database the P1 retro bead is filed against.
const breakGlassBeadDB = "~/beads/context-engine/.beads"

// BreakGlassConfig holds the inputs and injectable seams for a break-glass run.
// The function-valued fields default to real implementations in BreakGlass when
// left nil; tests override them to avoid touching the terminal, GitHub, or the
// beads database.
type BreakGlassConfig struct {
	PRArg string // PR number or URL exactly as the operator typed it
	Repo  string // "owner/repo"

	// I/O seams. Zero values default to the real stdin/stdout/stderr.
	In  io.Reader
	Out io.Writer
	Err io.Writer

	// Injectable seams (nil → real implementation).
	IsTTY     func() bool                          // default: stdin is a terminal
	Now       func() time.Time                     // default: time.Now
	Operator  func() string                        // default: git config user.email
	Protected func(repo string) (bool, error)      // default: enforce_admins check
	RunGH     func(args ...string) ([]byte, error) // default: exec gh
	RunBD     func(args ...string) ([]byte, error) // default: exec bd
	AuditDir  string                               // default: breakGlassAuditDir()
}

// BreakGlassAuditEntry is one append-only JSONL record of a break-glass use.
type BreakGlassAuditEntry struct {
	Timestamp string `json:"ts"`
	PR        string `json:"pr"`
	Repo      string `json:"repo"`
	Reason    string `json:"reason"`
	Operator  string `json:"operator"`
	Outcome   string `json:"outcome"` // "merged" or "ruleset_blocked"
	Bead      string `json:"bead,omitempty"`
}

// BreakGlass runs the interactive break-glass flow end to end. It returns an
// error only on a refusal or failure that should set a non-zero exit code; the
// ruleset-protected path prints instructions and returns nil (exit 0 by design).
func BreakGlass(cfg BreakGlassConfig) error {
	cfg = withBreakGlassDefaults(cfg)

	// Gate 0: interactive TTY. This is the structural exclusion of agents —
	// it runs before anything else so a non-interactive caller does nothing.
	if !cfg.IsTTY() {
		fmt.Fprintln(cfg.Err, "break-glass requires an interactive TTY — agent callers are excluded by design")
		return fmt.Errorf("no interactive TTY")
	}

	if strings.TrimSpace(cfg.PRArg) == "" {
		return fmt.Errorf("break-glass requires a PR number or URL")
	}
	if !strings.Contains(cfg.Repo, "/") {
		return fmt.Errorf("--repo must be in owner/repo format, got %q", cfg.Repo)
	}

	// Prompt for the typed justification.
	fmt.Fprintln(cfg.Out, "BREAK-GLASS: You are about to bypass safe-merge checks.")
	fmt.Fprintf(cfg.Out, "State the reason (min %d chars):\n", MinBreakGlassReason)
	line, err := bufio.NewReader(cfg.In).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("reading reason: %w", err)
	}
	reason := strings.TrimSpace(line)
	if err := validateBreakGlassReason(reason); err != nil {
		fmt.Fprintln(cfg.Err, err.Error())
		return err
	}

	operator := cfg.Operator()
	now := cfg.Now().UTC()

	// Post the reason as a PR comment (reason + operator + timestamp).
	commentBody := buildBreakGlassComment(reason, operator, now)
	if _, err := cfg.RunGH("pr", "comment", cfg.PRArg, "--repo", cfg.Repo, "--body", commentBody); err != nil {
		return fmt.Errorf("posting PR comment: %w", err)
	}
	fmt.Fprintln(cfg.Out, "break-glass: ✓ reason posted as PR comment")

	// Decide the outcome path: ruleset-protected repos cannot be merged
	// programmatically, so we print the manual instruction instead.
	protected, perr := cfg.Protected(cfg.Repo)
	if perr != nil {
		// Fail safe: if we cannot determine protection status, do NOT attempt
		// a programmatic merge — treat it as protected and print the manual path.
		fmt.Fprintf(cfg.Err, "break-glass: could not determine ruleset status (%v); assuming protected\n", perr)
		protected = true
	}

	outcome := "merged"
	if protected {
		outcome = "ruleset_blocked"
		printRulesetInstruction(cfg.Out, cfg.Repo, cfg.PRArg)
	} else {
		// Immediate squash merge — break-glass is NOT --auto.
		if _, err := cfg.RunGH("pr", "merge", cfg.PRArg, "--repo", cfg.Repo, "--squash"); err != nil {
			// Record the failed attempt before returning so the audit trail and
			// retro bead still capture the use.
			cfg.writeAuditAndBead(reason, operator, now, "merge_failed", "")
			return fmt.Errorf("gh pr merge failed: %w", err)
		}
		fmt.Fprintf(cfg.Out, "break-glass: ✓ PR %s merged (squash)\n", cfg.PRArg)
	}

	// Append the audit record and file the P1 retro bead.
	beadID := cfg.writeAuditAndBead(reason, operator, now, outcome, "")

	// Retro trigger — every break-glass use must produce a retro.
	fmt.Fprintln(cfg.Out, "⚠ DEAR retro required — every break-glass use must produce a retro bead.")
	if beadID != "" {
		fmt.Fprintf(cfg.Out, "Bead filed: %s\n", beadID)
	} else {
		fmt.Fprintln(cfg.Out, "Bead filed: (bead creation failed — file a P1 retro bead manually)")
	}
	return nil
}

// writeAuditAndBead appends the audit record and files the P1 retro bead.
// It returns the filed bead ID (empty if filing failed). Audit-log and bead
// failures are non-fatal: they are logged but never abort a break-glass that
// has already mutated the PR.
func (cfg BreakGlassConfig) writeAuditAndBead(reason, operator string, now time.Time, outcome, _ string) string {
	beadID := cfg.fileRetroBead(reason)
	entry := BreakGlassAuditEntry{
		Timestamp: now.Format(time.RFC3339),
		PR:        cfg.PRArg,
		Repo:      cfg.Repo,
		Reason:    reason,
		Operator:  operator,
		Outcome:   outcome,
		Bead:      beadID,
	}
	appendBreakGlassAudit(cfg.AuditDir, entry)
	return beadID
}

// fileRetroBead files a P1 retro bead and returns its ID (empty on failure).
func (cfg BreakGlassConfig) fileRetroBead(reason string) string {
	title := fmt.Sprintf("DEAR retro: break-glass used on %s — %s", cfg.PRArg, truncate(reason, 50))
	out, err := cfg.RunBD("--db", breakGlassBeadDB, "create", title,
		"--priority", "P1", "--type", "retro", "--json")
	if err != nil {
		slog.Warn("break-glass: failed to file retro bead", "error", err)
		return ""
	}
	return parseBeadID(out)
}

// --- pure helpers (unit-tested) ---

// validateBreakGlassReason enforces the minimum justification length.
func validateBreakGlassReason(reason string) error {
	if len(reason) < MinBreakGlassReason {
		return fmt.Errorf("reason too short (min %d chars)", MinBreakGlassReason)
	}
	return nil
}

// buildBreakGlassComment renders the PR comment body posted on break-glass.
func buildBreakGlassComment(reason, operator string, ts time.Time) string {
	return fmt.Sprintf("BREAK-GLASS: %s\nOperator: %s\nTimestamp: %s",
		reason, operator, ts.UTC().Format(time.RFC3339))
}

// printRulesetInstruction prints the manual merge instruction for a
// ruleset-protected repo. Branch protection with enforce_admins blocks even
// programmatic admin merges, so the operator must temporarily relax it.
func printRulesetInstruction(w io.Writer, repo, prArg string) {
	owner, name := splitRepo(repo)
	fmt.Fprintln(w, "RULESET PROTECTED: Cannot merge programmatically.")
	fmt.Fprintf(w, "Go to: https://github.com/%s/%s/settings/branches\n", owner, name)
	fmt.Fprintf(w, "Temporarily allow admin bypass, merge PR %s, then re-enable.\n", prArg)
}

// parseBeadID extracts the created bead ID from `bd create --json` output.
// bd prints a JSON object containing an "id" field; if the shape differs it
// falls back to scanning for the first id-like token.
func parseBeadID(out []byte) string {
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &obj); err == nil && obj.ID != "" {
		return obj.ID
	}
	// Some bd versions wrap the created issue under "issue" or print an array.
	var wrapper struct {
		Issue struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(out, &wrapper); err == nil && wrapper.Issue.ID != "" {
		return wrapper.Issue.ID
	}
	return ""
}

// splitRepo splits "owner/repo" into its parts (best effort).
func splitRepo(repo string) (owner, name string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return repo, ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// appendBreakGlassAudit appends one JSON line to the break-glass audit log.
// The log is append-only and never truncated. Failures are logged via slog but
// never returned — a break-glass that already mutated the PR must not be undone
// by an audit-log problem.
func appendBreakGlassAudit(dir string, entry BreakGlassAuditEntry) {
	if dir == "" {
		dir = breakGlassAuditDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		slog.Warn("break-glass: failed to create audit dir", "error", err)
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "break-glass-audit.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Warn("break-glass: failed to open audit log", "error", err)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("break-glass: failed to close audit log", "error", cerr)
		}
	}()
	b, _ := json.Marshal(entry)
	if _, werr := fmt.Fprintln(f, string(b)); werr != nil {
		slog.Warn("break-glass: failed to write audit entry", "error", werr)
	}
}

// breakGlassAuditDir returns the directory for the break-glass audit log.
// Overridable via BREAK_GLASS_AUDIT_DIR for testing.
func breakGlassAuditDir() string {
	if d := os.Getenv("BREAK_GLASS_AUDIT_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return filepath.Join(home, ".config", "dear-agent")
}

// --- default seam implementations ---

func withBreakGlassDefaults(cfg BreakGlassConfig) BreakGlassConfig {
	if cfg.In == nil {
		cfg.In = os.Stdin
	}
	if cfg.Out == nil {
		cfg.Out = os.Stdout
	}
	if cfg.Err == nil {
		cfg.Err = os.Stderr
	}
	if cfg.IsTTY == nil {
		cfg.IsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Operator == nil {
		cfg.Operator = gitConfigEmail
	}
	if cfg.Protected == nil {
		cfg.Protected = rulesetProtected
	}
	if cfg.RunGH == nil {
		cfg.RunGH = func(args ...string) ([]byte, error) {
			return runCommand(exec.Command("gh", args...))
		}
	}
	if cfg.RunBD == nil {
		cfg.RunBD = func(args ...string) ([]byte, error) {
			return runCommand(exec.Command("bd", args...))
		}
	}
	if cfg.AuditDir == "" {
		cfg.AuditDir = breakGlassAuditDir()
	}
	return cfg
}

// gitConfigEmail returns the operator's git email, or "unknown" if unset.
func gitConfigEmail() string {
	out, err := exec.Command("git", "config", "user.email").Output()
	if err != nil {
		return "unknown"
	}
	email := strings.TrimSpace(string(out))
	if email == "" {
		return "unknown"
	}
	return email
}

// rulesetProtected reports whether the repo's main branch is protected such
// that a programmatic merge would fail (branch protection with enforce_admins).
// On any API error it returns the error so the caller can fail safe.
func rulesetProtected(repo string) (bool, error) {
	owner, name := splitRepo(repo)
	out, err := runCommand(exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/%s/branches/main/protection", owner, name)))
	if err != nil {
		// A 404 means no protection configured → not protected. gh returns a
		// non-zero exit for 404, so distinguish it from real failures.
		if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return parseRulesetProtected(out)
}

// parseRulesetProtected reports whether branch-protection JSON indicates a
// merge would be blocked for the operator (enforce_admins enabled).
func parseRulesetProtected(data []byte) (bool, error) {
	var resp struct {
		EnforceAdmins struct {
			Enabled bool `json:"enabled"`
		} `json:"enforce_admins"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return false, fmt.Errorf("parsing branch protection: %w", err)
	}
	return resp.EnforceAdmins.Enabled, nil
}
