package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/codexhooks"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/pkg/override"
)

var (
	overrideTTL            time.Duration
	overrideNote           string
	overrideCodexSource    string
	overrideAuditWindow    time.Duration
	overrideAuditThreshold int
	overrideAuditJSON      bool
	overrideAuditNotify    bool
)

// overrideAuditBreachExit is the distinct exit code a scheduled audit uses to
// signal "used too often". A generic failure exit would make an alert
// indistinguishable from a broken cron entry.
const overrideAuditBreachExit = 3

var deliverOverrideAuditNotification = sendOverrideAuditNotification

var overrideCmd = &cobra.Command{
	Use:   "override",
	Short: "Approve, inspect, and audit dangerous overrides",
	Long: `Manage unattended launch overrides that switch off a launch-time safety control.

Every unattended launch override requires a stated reason, an operator-owned
approval that expires, and a ledger entry. Approvals require both an
interactive terminal and elevated access to the system approval store, so a
same-user unattended agent cannot approve its own override.

Kinds:
  codex-hook-trust   run Codex hooks without per-path trust review
  admission-brake    admit a spawn while an admission brake is engaged
  supervisor-oauth-check
                      launch a supervisor without a current OAuth token`,
}

var overrideApproveCmd = &cobra.Command{
	Use:   "approve <kind>",
	Short: "Approve an override kind for a bounded window (interactive only)",
	Args:  cobra.ExactArgs(1),
	RunE:  runOverrideApprove,
}

var overrideRevokeCmd = &cobra.Command{
	Use:   "revoke <kind>",
	Short: "Revoke an override approval",
	Args:  cobra.ExactArgs(1),
	RunE:  runOverrideRevoke,
}

var overrideStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current approvals and recent override use",
	RunE:  runOverrideStatus,
}

var overrideAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Review collected override reasons and alert when use climbs",
	Long: `Aggregate recorded override use over a window.

Exits ` + fmt.Sprint(overrideAuditBreachExit) + ` when any kind reaches the alert threshold. Scheduled runs use
--notify to deliver the breach through the host notification service before exiting. A repeated reason is the signal to act on: the
override has become a standing workaround for a root cause nobody fixed, and
the fix is either to remove that cause or to close the loophole.`,
	RunE: runOverrideAudit,
}

func init() {
	overrideApproveCmd.Flags().DurationVar(&overrideTTL, "ttl", time.Hour, "How long the approval remains valid")
	overrideApproveCmd.Flags().StringVar(&overrideNote, "note", "", "Optional note recorded with the approval")
	overrideApproveCmd.Flags().StringVar(&overrideCodexSource, "codex-hook-source", "",
		"Repository whose current committed Codex hook bytes are being approved (required for codex-hook-trust)")
	overrideAuditCmd.Flags().DurationVar(&overrideAuditWindow, "window", 7*24*time.Hour, "Window to review")
	overrideAuditCmd.Flags().IntVar(&overrideAuditThreshold, "threshold", 5, "Alert when a kind reaches this many uses (0 disables)")
	overrideAuditCmd.Flags().BoolVar(&overrideAuditJSON, "json", false, "Emit the report as JSON")
	overrideAuditCmd.Flags().BoolVar(&overrideAuditNotify, "notify", false, "Deliver threshold breaches through the host notification service")

	overrideCmd.AddCommand(overrideApproveCmd, overrideRevokeCmd, overrideStatusCmd, overrideAuditCmd)
	rootCmd.AddCommand(overrideCmd)
}

func parseOverrideKind(arg string) (override.Kind, error) {
	kind := override.Kind(strings.TrimSpace(arg))
	if !kind.Valid() {
		return "", fmt.Errorf("%w: %q (known: %s)", override.ErrUnknownKind, arg, joinKinds())
	}
	return kind, nil
}

func joinKinds() string {
	names := make([]string, 0, len(override.Kinds()))
	for _, kind := range override.Kinds() {
		names = append(names, string(kind))
	}
	return strings.Join(names, ", ")
}

// interactiveStdin requires the typed confirmation to happen at a terminal.
// The separate OS authorization prompt and root-owned destination are the
// authority boundary; a TTY alone is not treated as proof of a human.
func interactiveStdin() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func runOverrideApprove(cmd *cobra.Command, args []string) error {
	kind, err := parseOverrideKind(args[0])
	if err != nil {
		return err
	}
	if !interactiveStdin() {
		return fmt.Errorf("approving %q requires an interactive terminal: a human, not an agent, authorizes a dangerous override", kind)
	}
	if overrideTTL <= 0 {
		return fmt.Errorf("--ttl must be positive")
	}
	codexSource, err := resolveOverrideApprovalCodexSource(cmd.Context(), kind, overrideCodexSource)
	if err != nil {
		return err
	}
	if _, err := override.LoadGrant(kind); err != nil {
		return fmt.Errorf("inspect existing override grant before approval: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nApproving dangerous override %q for %s.\n", kind, overrideTTL)
	fmt.Fprintf(cmd.OutOrStdout(), "The approval is installed in operator-owned storage at %s.\n", override.GrantPath(kind))
	fmt.Fprintf(cmd.OutOrStdout(), "This disables a safety control. Every use is recorded to %s.\n", override.LedgerPath())
	if codexSource != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Reviewed Codex hook repository: %s\n", codexSource.Repository)
		fmt.Fprintf(cmd.OutOrStdout(), "Reviewed source commit: %s\n", codexSource.Commit)
		fmt.Fprintf(cmd.OutOrStdout(), "Reviewed committed hook digest: %s\n", codexSource.Digest)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nType the override kind to confirm: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if strings.TrimSpace(line) != string(kind) {
		return fmt.Errorf("confirmation did not match %q; nothing was approved", kind)
	}

	now := time.Now().UTC()
	grant := override.Grant{
		Kind:          kind,
		ApprovedBy:    approvalActor(),
		ApprovedAtUTC: now,
		ExpiresUTC:    now.Add(overrideTTL),
		Note:          overrideNote,
		CodexHooks:    codexSource,
	}
	if err := installConfirmedGrant(grant); err != nil {
		return err
	}
	ui.PrintSuccess(fmt.Sprintf("Approved %s until %s", kind, grant.ExpiresUTC.Format(time.RFC3339)))
	return nil
}

func resolveOverrideApprovalCodexSource(
	ctx context.Context,
	kind override.Kind,
	sourceRepo string,
) (*override.CodexHookSource, error) {
	if kind != override.KindCodexHookTrust {
		if sourceRepo != "" {
			return nil, fmt.Errorf("--codex-hook-source is valid only for %q", override.KindCodexHookTrust)
		}
		return nil, nil
	}
	if sourceRepo == "" {
		return nil, fmt.Errorf("--codex-hook-source is required for %q so approval is bound to reviewed bytes", kind)
	}
	identity, err := codexhooks.InspectSource(ctx, sourceRepo)
	if err != nil {
		return nil, fmt.Errorf("inspect Codex hook source before approval: %w", err)
	}
	return &override.CodexHookSource{
		Repository: identity.SourceRepo,
		Commit:     identity.SourceCommit,
		Digest:     identity.Digest,
	}, nil
}

func installConfirmedGrant(grant override.Grant) error {
	data, err := json.MarshalIndent(grant, "", "  ")
	if err != nil {
		return fmt.Errorf("encode override grant: %w", err)
	}
	if err := installOperatorGrant(append(data, '\n'), override.GrantPath(grant.Kind)); err != nil {
		return err
	}
	installed, err := override.LoadGrant(grant.Kind)
	if err != nil {
		return fmt.Errorf("verify installed override grant: %w", err)
	}
	if installed == nil || installed.Kind != grant.Kind ||
		!installed.ApprovedAtUTC.Equal(grant.ApprovedAtUTC) ||
		!installed.ExpiresUTC.Equal(grant.ExpiresUTC) ||
		installed.ApprovedBy != grant.ApprovedBy ||
		installed.Note != grant.Note ||
		!equalCodexHookSource(installed.CodexHooks, grant.CodexHooks) {
		return fmt.Errorf("verify installed override grant: installed content does not match the confirmed approval")
	}
	if err := installed.Active(grant.Kind, time.Now()); err != nil {
		return fmt.Errorf("verify installed override grant: %w", err)
	}
	return nil
}

func equalCodexHookSource(left, right *override.CodexHookSource) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func approvalActor() string {
	if user := os.Getenv("SUDO_USER"); user != "" && user != "root" {
		return user
	}
	return ops.OverrideActor()
}

func runOverrideRevoke(cmd *cobra.Command, args []string) error {
	kind, err := parseOverrideKind(args[0])
	if err != nil {
		return err
	}
	if err := removeOperatorGrant(override.GrantPath(kind)); err != nil {
		return err
	}
	if grant, err := override.LoadGrant(kind); err != nil {
		return fmt.Errorf("verify revoked override grant: %w", err)
	} else if grant != nil {
		return fmt.Errorf("verify revoked override grant: approval remains at %s", override.GrantPath(kind))
	}
	ui.PrintSuccess(fmt.Sprintf("Revoked approval for %s", kind))
	return nil
}

func runOverrideStatus(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	now := time.Now()
	fmt.Fprintln(out, "Approvals:")
	for _, kind := range override.Kinds() {
		grant, err := override.LoadGrant(kind)
		if err != nil {
			fmt.Fprintf(out, "  %-18s unreadable: %v\n", kind, err)
			continue
		}
		if grant == nil {
			fmt.Fprintf(out, "  %-18s not approved\n", kind)
			continue
		}
		if activeErr := grant.Active(kind, now); activeErr != nil {
			fmt.Fprintf(out, "  %-18s unavailable: %v\n", kind, activeErr)
			continue
		}
		fmt.Fprintf(out, "  %-18s approved by %s until %s", kind, grant.ApprovedBy, grant.ExpiresUTC.Format(time.RFC3339))
		if grant.CodexHooks != nil {
			fmt.Fprintf(out, " (%s @ %s, digest %s)",
				grant.CodexHooks.Repository, grant.CodexHooks.Commit, grant.CodexHooks.Digest)
		}
		fmt.Fprintln(out)
	}

	uses, err := override.LoadUses(now.Add(-7 * 24 * time.Hour))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\nUses in the last 7 days: %d\n", len(uses))
	for _, use := range lastUses(uses, 5) {
		fmt.Fprintf(out, "  %s  %-18s %s — %s\n",
			use.AtUTC.Format(time.RFC3339), use.Kind, use.Actor, use.Reason)
	}
	return nil
}

func lastUses(uses []override.Use, n int) []override.Use {
	if len(uses) <= n {
		return uses
	}
	return uses[len(uses)-n:]
}

func runOverrideAudit(cmd *cobra.Command, args []string) error {
	now := time.Now()
	uses, err := override.LoadUses(now.Add(-overrideAuditWindow))
	if err != nil {
		return err
	}
	report := override.Audit(uses, overrideAuditWindow, overrideAuditThreshold, now)

	out := cmd.OutOrStdout()
	if overrideAuditJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "Override audit — %d use(s) since %s (threshold %d per kind)\n\n",
			report.Total, report.Since.Format(time.RFC3339), report.Threshold)
		for _, kind := range override.Kinds() {
			fmt.Fprintf(out, "  %-18s %d\n", kind, report.ByKind[kind])
		}
		if len(report.ByReason) > 0 {
			fmt.Fprintln(out, "\nReasons, most repeated first:")
			for _, tally := range report.ByReason {
				fmt.Fprintf(out, "  %3d×  %-18s %s\n", tally.Count, tally.Kind, tally.Reason)
			}
			fmt.Fprintln(out, "\nA reason that repeats is a root cause to fix, not an override to renew.")
		}
	}

	if report.Breached {
		for _, breach := range report.Breaches {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"ALERT: %s used %d time(s), at or above the threshold of %d\n",
				breach.Kind, breach.Count, report.Threshold)
		}
		if err := notifyOverrideAuditBreach(report); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "ALERT DELIVERY FAILED: %v\n", err)
			cmd.SilenceUsage = true
			return fmt.Errorf("dangerous override threshold breached but alert delivery failed: %w", err)
		}
		// Signal the alert without Cobra reprinting usage for a healthy run.
		cmd.SilenceUsage = true
		os.Exit(overrideAuditBreachExit)
	}
	return nil
}

func notifyOverrideAuditBreach(report override.AuditReport) error {
	if !overrideAuditNotify {
		return nil
	}
	return deliverOverrideAuditNotification(overrideAuditAlertMessage(report))
}

func overrideAuditAlertMessage(report override.AuditReport) string {
	parts := make([]string, 0, len(report.Breaches))
	for _, breach := range report.Breaches {
		parts = append(parts, fmt.Sprintf("%s=%d", breach.Kind, breach.Count))
	}
	return fmt.Sprintf(
		"Dangerous override threshold %d breached: %s. Review %s.",
		report.Threshold, strings.Join(parts, ", "), override.LedgerPath(),
	)
}
