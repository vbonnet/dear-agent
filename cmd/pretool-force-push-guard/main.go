// Command pretool-force-push-guard is a Claude Code PreToolUse hook that
// enforces one rule: force-push is allowed on feature and PR branches, and
// refused on main, master, and the repository's default branch.
//
// The rule was already the policy in internal/safegit, and safe-push, the
// filesystem write guard, and the GitHub ruleset on the default branch all
// implement it. The shell hook that ran ahead of them did not: it matched the
// text of a command instead of parsing it, so it refused force-pushes to
// feature branches whenever the surrounding command mentioned main, and
// refused commands that merely quoted a push. Sessions read those refusals as
// "force-push is denied on this host" and abandoned rebases they were entitled
// to finish.
//
// This adapter reads the PreToolUse envelope from stdin, extracts every push
// invocation with fsguard.ScanPushes (both `git push` and the safe-push
// wrapper, per simple command, with `cd` and -C tracked), and judges each one
// with safegit.ForcePushViolation, the same function every other layer calls.
// One policy, one answer.
//
// Exit 0 allows; exit 2 blocks with positive guidance on stderr and a
// permission decision on stdout. The hook fails open on anything it cannot
// read or parse: a guard that blocks what it does not understand wedges the
// session for reasons unrelated to the policy, and the settings.json deny
// rules plus the GitHub ruleset remain the backstop.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/internal/fsguard"
	"github.com/vbonnet/dear-agent/internal/override"
	"github.com/vbonnet/dear-agent/internal/safegit"
)

// approvalEnv carries the operator's typed justification for the one sanctioned
// force-push to a protected branch: a history scrub a human has decided to run.
// It is deliberately not a bare boolean. The reason is judged for substance and
// appended to the override audit ledger, so the bypass leaves a record naming
// who did what and why.
//
// Unlocking this layer is not the same as unlocking the push. The GitHub
// ruleset on the default branch carries non_fast_forward independently, and
// refuses the rewrite regardless of what this hook decides.
const approvalEnv = "FORCE_PUSH_PROTECTED_APPROVAL"

type envelope struct {
	ToolName  string `json:"tool_name"`
	CWD       string `json:"cwd"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// hookDecision is the structured half of a block, read by Claude Code.
type hookDecision struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
	PermissionDecision string `json:"permissionDecision"`
	DenialReason       string `json:"denialReason"`
}

func main() { os.Exit(run(context.Background(), os.Stdin, os.Stdout, os.Stderr)) }

func run(ctx context.Context, in io.Reader, out, errOut io.Writer) int {
	var env envelope
	if err := json.NewDecoder(in).Decode(&env); err != nil {
		return 0 // unparseable envelope -> fail open
	}
	if env.ToolName != "Bash" || strings.TrimSpace(env.ToolInput.Command) == "" {
		return 0
	}
	pushes, ok := fsguard.New().ScanPushes(env.ToolInput.Command, env.CWD)
	if !ok || len(pushes) == 0 {
		return 0
	}
	for _, p := range pushes {
		target, blocked := violation(p)
		if !blocked {
			continue
		}
		if approved(ctx, target) {
			continue
		}
		msg := refusal(target)
		emit(out, errOut, msg)
		fmt.Fprintln(errOut, msg)
		return 2
	}
	return 0
}

// violation judges one push against every directory it may really run in. A
// conditional `cd` leaves more than one candidate, and a push that would be
// refused in any of them is refused: the alternative is to act on a guess about
// which directory the shell was in.
func violation(p fsguard.PushInvocation) (string, bool) {
	for _, dir := range append([]string{p.RepoDir}, p.AlsoDirs...) {
		if target, blocked := safegit.ForcePushViolation(dir, "", p.Args); blocked {
			return target, true
		}
	}
	return "", false
}

// approved reports whether the operator has supplied a justification good
// enough to unlock this layer, recording the attempt either way.
func approved(ctx context.Context, target string) bool {
	reason := strings.TrimSpace(os.Getenv(approvalEnv))
	if reason == "" {
		return false
	}
	err := override.Require(ctx, override.Guard{
		Tool: "git push",
		Flag: "--force",
		Gate: "the force-push guard on protected branch " + target,
		Risk: override.RiskP1,
		// The deterministic judge is chosen explicitly rather than taken from
		// the risk default. A PreToolUse hook runs inside a few seconds on
		// every Bash call, and a model round-trip there would make the guard
		// the slowest thing in the session. The high-friction half of this
		// bypass is the audited justification plus the GitHub ruleset that
		// still refuses the rewrite, not a second opinion gathered here.
		Judge: override.DefaultJudge{},
	}, reason)
	return err == nil
}

func refusal(target string) string {
	return fmt.Sprintf(`BLOCKED: force-push would reach %q, a protected branch.

Force-push is allowed on feature and PR branches and refused on main, master,
and the repository default. This push resolves to %q, so it is refused.

What to do instead:
  - Rebasing a PR branch? Force-push the branch itself, which is allowed:
      safe-push --force-with-lease origin <feature-branch>
  - Landing work on the default branch? Use the PR flow:
      safe-pr create ...   then   safe-merge --pr <num> --watch
  - Named a branch but got this anyway? The destination could not be resolved
    from the command line (a wildcard refspec, --mirror/--all/--tags, a
    configured remote push refspec, or push.default=matching). Name the branch
    explicitly: safe-push --force-with-lease origin <branch>

The one sanctioned exception is a history scrub a human has decided to run. It
is gated on a typed justification, which is judged for substance and appended
to the override audit ledger:
  %s="<why this rewrite of %s is warranted right now>" safe-push ...
Note that the GitHub ruleset on the default branch carries non_fast_forward
independently, so it refuses the rewrite whatever this hook decides.`,
		target, target, approvalEnv, target)
}

func emit(out, errOut io.Writer, msg string) {
	var d hookDecision
	d.HookSpecificOutput.HookEventName = "PreToolUse"
	d.HookSpecificOutput.AdditionalContext = msg
	d.PermissionDecision = "deny"
	d.DenialReason = msg
	// A failed encode costs only the structured half of the decision: the exit
	// code and the stderr guidance still block the command, so this must not
	// change the verdict. Report it rather than discarding it, so a broken
	// stdout is visible instead of silently degrading every refusal.
	if err := json.NewEncoder(out).Encode(d); err != nil {
		fmt.Fprintf(errOut, "force-push guard: could not write the structured decision: %v\n", err)
	}
}
