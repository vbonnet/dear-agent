# deepsec — incremental security scanning

[deepsec](https://github.com/vercel-labs/deepsec) is an AI-powered
vulnerability scanner. We run it **incrementally** — only on files
changed since `origin/main` — because full-repo scans are minutes-to-hours
of agent time, while typical PRs touch a handful of files.

This page covers the automation. For the underlying scanner config (custom
matchers, repo context, threat model), see `.deepsec/README.md` and
`.deepsec/data/deepsec-scan/INFO.md`.

## Cost model

| Surface           | Cost                                                                     |
| ----------------- | ------------------------------------------------------------------------ |
| Local (developer) | $0 — auto-detects your `claude` CLI subscription and uses your quota.    |
| CI (GitHub PR)    | Billed against `ANTHROPIC_API_KEY`. Workflow no-ops if the secret is absent. |

Local runs require either `claude` or `codex` logged in. If neither is
present, set `ANTHROPIC_API_KEY` (or `OPENAI_API_KEY`) in
`.deepsec/.env.local`.

## Quick start

```bash
# One-shot scan of files changed since origin/main:
make deepsec-incremental

# Scan only what you have staged (manual pre-commit gate):
make deepsec-staged

# Install a pre-push hook (soft-fail — warns but does not block):
make install-deepsec-hook

# Strict mode — pre-push refuses to push on findings:
STRICT=1 make install-deepsec-hook

# Bypass the hook for one push:
DEEPSEC_SKIP=1 git push

# Uninstall:
make uninstall-deepsec-hook
```

## How the incremental mode works

`deepsec process --diff <ref>` investigates only the files changed between
`<ref>` and `HEAD`. The project (path resolution, custom matchers, INFO.md
context) is auto-created if it doesn't exist — so the scanner works in any
checkout that has `.deepsec/`. Other modes the wrapper exposes:

| Wrapper flag        | Underlying deepsec flag | When to use                       |
| ------------------- | ----------------------- | --------------------------------- |
| `--since <ref>`     | `--diff <ref>`          | Push gate / PR scan (default).    |
| `--staged`          | `--diff-staged`         | Manual pre-commit check.          |
| `--working`         | `--diff-working`        | Scan uncommitted + untracked.     |
| `--comment-out <p>` | `--comment-out <p>`     | Write PR-shaped markdown summary. |

Exit codes follow deepsec: `0` means no findings, `1` means at least one.
Pass `--soft` to convert `1` → `0` (the pre-push hook uses this).

## Pre-push hook

`scripts/install-deepsec-hook.sh` writes a block bounded by sentinel
markers into `.git/hooks/pre-push`. Re-running rewrites the block, so it
coexists with the `prepush-act-validator` hook installed by
`make install-hooks`. Bypass for a single push with `DEEPSEC_SKIP=1`.

The hook is **soft-fail by default**. Rationale: pre-push is the wrong
window to discover an RCE — developers will just `--no-verify`. We want
the hook to inform, and CI to gate. Install with `STRICT=1` if you'd
rather block locally.

## CI (`deepsec.yml`)

<<<<<<< Updated upstream
Runs for same-repository PRs against `main` that carry the `full-ci` label. It
evaluates the PR when it is opened, synchronized, reopened, or when `full-ci`
is added; adding an unrelated label does not start another paid scan. The
workflow scans the exact current PR head, posts candidate findings as a PR
comment, and is **currently non-blocking**. Promote it to required once
signal-to-noise stabilises
(`.ci-policy.yaml > branch_policies.main.required_workflows`).
=======
Runs on every PR against `main`, scans the PR diff, and posts findings as
a PR comment. **Currently non-blocking** — the workflow always reports
success so it doesn't gate merges. To promote it to required once
signal-to-noise stabilises, add its check context to
`.github/rulesets/main.json` and re-apply the ruleset (the source of truth for
required checks — see `docs/branch-protection.md`), and mirror the change in
`.ci-policy.yaml > branch_policies.main.required_workflows` so `internal/ci`
tooling stays in agreement.
>>>>>>> Stashed changes

Setup steps to enable in CI:

1. Add an `ANTHROPIC_API_KEY` secret at the repo level (Settings →
   Secrets → Actions).
2. Add `full-ci` to the PR. If the label is already present and the head is
   unchanged, remove and re-add it to request a rerun. Use the GitHub UI, a
   GitHub App, or a PAT-backed automation: a label written by an Actions
   workflow's default `GITHUB_TOKEN` does not recursively start this workflow.

Without the secret, an eligible run logs an explicit notice and leaves the
scan job visibly skipped; it does not claim that the head was scanned.

Fork PRs are rejected before the credential probe, even if they carry
`full-ci`.

## Where findings live

Generated artifacts (per-file analysis, run metadata, exported markdown,
the `findings/` tree) are gitignored — see `.deepsec/.gitignore`. To
generate a browsable export:

```bash
cd .deepsec && pnpm install   # first time only
pnpm deepsec scan
pnpm deepsec process     --concurrency 5
pnpm deepsec revalidate  --concurrency 5         # cuts false-positive rate
pnpm deepsec export      --format md-dir --out ./findings
```

## Triage workflow for new findings

1. PR comment lands → reviewer clicks into the linked file/line.
2. False positive? Run `deepsec revalidate --force` to re-check, or open
   the markdown finding file and document why (deepsec uses prior
   verdicts to avoid re-flagging on subsequent scans).
3. True positive that needs a fix? File an issue or fix inline; add a
   custom matcher in `.deepsec/` if the pattern is worth catching by
   regex going forward (see `node_modules/deepsec/dist/docs/writing-matchers.md`).

## Known gaps

- The PR-based gate above is the primary deepsec signal. The
  `weekly-security-audit` scheduled task writes to a sandbox path Cowork
  can't access; fixing that task is tracked in bead `ce-0tp87`.
- Output routing: deepsec writes findings under `.deepsec/findings/` in
  this repo. That's tooling output, not research analysis, so it
  doesn't violate `.dear-agent.yml > forbidden-paths`.
