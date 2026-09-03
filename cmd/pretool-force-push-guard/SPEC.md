# Force-Push Guard Specification

<!-- Last audited at: 2026-09-02 -->

## Overview

`cmd/pretool-force-push-guard` is the PreToolUse adapter for one rule:
force-push is allowed on feature and PR branches, and refused on `main`,
`master`, and the repository's default branch.

The rule is not new. `internal/safegit.ForcePushViolation` has implemented it
for every Go caller, `safe-push` enforces it on the push itself, and the
GitHub ruleset carries `non_fast_forward` on the default branch. What was
missing was a hook that asked the same question. The shell guard that ran
first matched the *text* of a command rather than parsing it, so it refused
force-pushes to feature branches whenever an unrelated command in the same
chain mentioned `main`, and refused commands that merely quoted a push.
Sessions read those refusals as a host-wide force-push ban and abandoned
rebases the policy allows.

This command closes that gap by delegating both halves to code that already
exists: `fsguard.ScanPushes` for parsing, `safegit.ForcePushViolation` for the
decision. Shell and Go cannot drift into two answers because there is only one.

## EARS Requirements

**FPG-01** When a valid Bash tool envelope is received, the command shall extract every push invocation in it through `internal/fsguard.ScanPushes`, covering both the `git push` and `safe-push` front-ends.

**FPG-02** When a push invocation's destination resolves to a non-default branch, the command shall allow it, including under `--force`, `-f`, and `--force-with-lease`.

**FPG-03** When a push invocation's destination resolves to `main`, `master`, the repository default branch, or a repository-configured protected branch, the command shall return exit code 2 with guidance naming the allowed path.

**FPG-04** When a push invocation's destination cannot be positively enumerated, the command shall refuse it rather than assume a non-protected destination.

**FPG-05** When a command mentions a force-push without invoking one, the command shall allow it.

**FPG-06** When a compound command contains several pushes, the command shall judge each one against its own operands and working directory.

**FPG-07** When a conditional `cd` leaves the working directory ambiguous, the command shall judge the push against every candidate directory and refuse if any is protected.

**FPG-08** When `FORCE_PUSH_PROTECTED_APPROVAL` carries a justification that passes `override.DefaultJudge`, the command shall allow a protected-branch force-push and append the bypass to the override audit ledger.

**FPG-09** When `FORCE_PUSH_PROTECTED_APPROVAL` is absent, empty, or fails the reason-quality check, the command shall refuse the protected-branch force-push.

**FPG-10** When the envelope is malformed, targets another tool, or the command cannot be tokenized, the command shall fail open, leaving the `settings.json` deny rules and the GitHub ruleset as the backstop.

## Non-goals

Unlocking the local layer is not unlocking the push. `non_fast_forward` on the
default branch is enforced by GitHub and is unaffected by anything here.

## BDD Traceability

- Package tests: `cmd/pretool-force-push-guard/*_test.go`
- Scanner tests: `internal/fsguard/pushscan_test.go`
- Policy tests: `internal/safegit/forcepolicy_test.go`
