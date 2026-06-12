# S4 Research — Local Enforcement Stack (this machine)

> Produced by an exploration subagent, 2026-06-11. Paths verified on disk.

## 1. Prior art: `safe-push`

`cmd/safe-push/main.go` + `internal/safegit/push.go` (+tests). Thin main (arg parsing only); all policy in the testable `internal/safegit` library (`chezmoi-deploy` already imports it). Enforces by construction: rejects `-f/--force/--force-with-lease/--force-if-includes`; resets the credential-helper chain to gh only; `GIT_TERMINAL_PROMPT=0` + 30s timeout; no shell (`exec.Command` argv). Install: `make install-safe-push` → `~/go/bin/safe-push`. `cmd/chezmoi-deploy` is the second instance of the same Principle-9 pattern.

## 2. PreToolUse guard binaries

`cmd/pretool-bash-write-guard`, `cmd/pretool-fs-write-guard`, policy in `internal/fsguard/` (bash.go tokenizer, policy.go, SPEC.md).

- Contract: PreToolUse JSON on stdin; **exit 0 = allow, exit 2 = block** with positive-guidance text on stderr. Both **fail open** on parse errors (settings.json deny rules are the backstop).
- The bash parser handles quotes/escapes, control-operator splitting, env-assignment and runner stripping (`sudo`, `env`, ...), recursive `bash -c`/`eval` to depth 8, and **tracks `cd` across `&&` chains** — so `cd repo && git ...` is attributed correctly (unlike the prefix-matched deny rules).
- **Critical gap: `internal/fsguard` has no handling of `gh` at all** — `gh pr merge --admin` sails through today. `checkGit` is the natural template for a `checkGh`.
- **Deployment conflict:** deployed hooks at `~/.config/claude-code/hooks/` are currently **Python scripts managed by chezmoi**; `make install-write-guards` copies Go binaries into the same dir, which the next `chezmoi apply` clobbers. See `docs/retros/2026-05-28-broken-go-hook-binaries.md`. A safe-merge hook must pick its deployment home deliberately.
- Precedent for "deny raw form, point at wrapper": the deployed Python bash guard blocks `chezmoi apply` everywhere and points at `chezmoi-deploy`, ending with a `PERMISSION_ESCALATION:` JIT instruction.

## 3. Claude Code permission config

Live `~/.claude/settings.json`; source of truth `~/.local/share/chezmoi/dot_claude/private_settings.json.tmpl`.

- **allow** includes `Bash(gh api:*)` ⚠️, `Bash(gh pr comment/create/list/view ...)`, `Bash(git push *)`, `Bash(git -C * push *)`, `Bash(git -C ~/worktrees/* merge *)`.
- **deny** includes `Bash(gh pr close *)`, force-push spellings, `~/src` git-write spellings.
- **`gh pr merge` appears in NO rule** — only the auto-mode permission classifier (a non-configured heuristic, noted in memory as "load-bearing") blocks it today.
- **`Bash(gh api:*)` is a wholesale bypass route:** it allow-lists `gh api -X PUT repos/<o>/<r>/pulls/<n>/merge` and GraphQL `mergePullRequest`. Blocking only `gh pr merge` leaves this open. (Also uses the `:*` syntax REVIEW.md lists as auto-FAIL — pre-existing debt.)
- Prefix-match gotchas observed: trailing-` *` patterns miss bare commands; `cd x && git ...` bypasses `git -C`-shaped denies.
- Registered PreToolUse hooks: askuserquestion-block, gmail-mcp-block, bash-write-guard, beads-protection, config-guard, fs-write-guard, validate-paired-files, article-pipeline-guard. Hook homes `~/.claude/hooks/` and `~/.config/claude-code/hooks/`, both chezmoi-managed.
- Any settings/hook change in dotfiles triggers the REVIEW.md §3 strict gate (security + governance personas + LLM judge).

## 4. `resolve-review-threads`

`cmd/resolve-review-threads/main.go`: `list` (unresolved threads, JSON), `list-all`, `resolve <threadId>`, `unresolve`, `resolve-all`. GraphQL-only (`resolveReviewThread` mutation; thread IDs `PRRT_...` from `reviewThreads`, paginated 100/page, via `gh api graphql` argv). `listThreads()`/`filterThreads()` are directly reusable as safe-merge's "all conversations resolved" verification primitive. No Makefile build/install target yet.

## 5. Makefile

`preflight`/`preflight-tests`/`preflight-full` (CI parity); `build/install-safe-push`; `build/install-write-guards` (`HOOKS_DIR ?= ~/.config/claude-code/hooks`, chezmoi-clobber caveat); git-hook installers are no-ops due to global `core.hooksPath=~/.config/git/hooks` (chezmoi-managed).

## 6. Related retros

- `docs/retros/2026-05-10-ci-red-and-unguarded-merges.md` — branch protection was cosmetic; established required checks.
- `docs/retros/2026-05-29-lint-red-direct-pushed-to-main.md` — admin direct pushes skip checks.
- `docs/retros/2026-05-28-broken-go-hook-binaries.md` — orphaned hook binaries; spec for Go hooks.
- `docs/retros/2026-06-08-phantom-trivy-required-check.md` — required-check config drift.
- No existing ADR covers merge gating.

## 7. REVIEW.md strict gate (dotfiles)

Sensitive = Claude settings/hooks, permission lists, always-loaded instructions, write guards/security boundaries. Three parallel verdicts required (security persona, governance persona, LLM judge JSON PASS|FAIL|BLOCK). Auto-FAIL: broadening allow / narrowing deny without justification, weakening a PreToolUse guard without replacement, `:*` wildcards, editing deployed targets instead of chezmoi source.

## 8. No existing `gh` shim

`which -a gh` → `/opt/homebrew/bin/gh` only. **PATH caveat: `/opt/homebrew/bin` is FIRST**, before `~/bin`/`~/go/bin`/`~/.local/bin` — a user-dir shim would not shadow gh without a PATH reorder, and never binds absolute-path invocations.

## Interception points summary

| Layer | Exists? | Notes for `gh pr merge` |
|---|---|---|
| PreToolUse Bash hook (exit 2) | Live | Strongest layer; survives `cd`/quoting/`bash -c`. Needs `checkGh`. Fails open → pair with deny rules. |
| settings.json deny rules | Live | Add `gh pr merge` spellings; must also narrow/guard `Bash(gh api:*)`. Sensitive change → REVIEW.md gate. |
| Auto-mode classifier | Implicit | Currently the only block on `gh pr merge`. Don't break; don't rely on. |
| Vetted wrapper (ALLOW'd) | Pattern proven | safe-push/chezmoi-deploy precedent; Principle 9 charter. |
| PATH shim for `gh` | Absent | Weak (PATH order, absolute paths). |
| Shell alias via chezmoi | Absent | Human shells only; `command gh` bypasses. |
| Server-side protection | Partial | dear-agent: checks+conversation-resolution+linear history but `enforce_admins:false`; private repos: nothing. |
| Git hooks (core.hooksPath) | Live | Irrelevant to API merges; useful only for direct-push variant. |
| Post-hoc audit | Manual today | The "verify" tier; should become cron. |
