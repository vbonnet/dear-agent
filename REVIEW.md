# REVIEW.md — Multi-agent PR review protocol

**Status:** authoritative · **Last updated:** 2026-06-11

Every PR against this repo goes through the review protocol below before
merging. The protocol is designed for a dark-factory loop: an LLM review agent
handles everything except decisions that are genuinely novel or irreversible.

---

## 1. Four-state outcome model

| Outcome | Meaning | Action |
|---|---|---|
| **approved** | No blocking findings; PR is ready to merge. | Merge immediately (squash). |
| **needs-work** | Fixable findings exist (incorrect logic, missing tests, style violations). | Author addresses findings; re-run review after. |
| **rejected** | Fundamental design problem that the current approach cannot resolve. | Close PR; open a new one with a different approach. |
| **needs-human-review** | Novel decision the protocol can't resolve: new security boundary, irreversible infrastructure change, explicit escalation trigger. | Pause automated loop; escalate to human. |

The review agent must output exactly one of these four states — never a
hedged "mostly approved" or a blend. Ambiguous findings always resolve
*down* (needs-work beats approved; needs-human-review beats needs-work).

---

## 2. Parallel review dimensions

Five sub-agents run concurrently against every PR diff. Each is independent —
no sub-agent reads another's output before forming its own findings. This
prevents anchoring and surfaces genuinely independent signals.

| Dimension | Agent prompt seed | Auto-fail triggers |
|---|---|---|
| **bugs** | "Find logic errors, off-by-one errors, nil dereferences, incorrect error handling, and any code path that panics or corrupts state." | Crashes, data loss, incorrect return values. |
| **security** | "Find vulnerabilities: injection, path traversal, privilege escalation, secret exposure, insecure defaults. Assume the diff will be deployed to production." | Any OWASP top-10 instance, hardcoded secrets. |
| **perf** | "Find algorithmic regressions (O(n²) where O(n) was expected), unnecessary allocations in hot paths, unbounded goroutine spawns, or missing context cancellation." | Goroutine leaks, O(n²) in request path. |
| **style** | "Check against the project's conventions: Go idioms, error wrapping, package naming, comment coverage on exported symbols, and golangci-lint rules." | Exported symbols without doc comment; unused parameters. |
| **regression** | "Does this diff break any existing tests, remove test coverage of critical paths, or introduce a change whose behaviour is inconsistent with existing tests?" | Deleting tests without replacement; changed behaviour with no test update. |

### 2.1 Synthesis

After all five sub-agents report, a synthesis agent collects their findings
and determines the overall outcome per §1:

1. Any auto-fail trigger from any dimension → **needs-human-review** (security) or **rejected** (data-loss) or **needs-work** (everything else).
2. No auto-fail triggers, no unresolved findings → **approved**.
3. Fixable findings only → **needs-work** (list each finding with the owning dimension and a concrete suggested fix).
4. Synthesis cannot determine whether a decision is safe → **needs-human-review**.

---

## 3. Escalation triggers (→ needs-human-review)

Regardless of finding severity, the synthesis agent must escalate when the
diff touches any of the following:

- **Agent permissions** — any edit to `permissions.allow`, `permissions.ask`, or `permissions.deny`.
- **Pre-tool hooks** — any change to hook scripts or hook registration in `settings.json`.
- **Security boundaries** — write guards, deny rules, `~/src` enforcement, PII manifests.
- **Infrastructure that is expensive to reverse** — database schema changes, launchd plist installs, CI/CD pipeline edits.
- **Explicit `HUMAN REVIEW REQUIRED` label** in the PR description or commit message.

Escalation is not a failure state — it is a correct outcome that preserves
human authority over irreversible decisions.

These triggers are enforced **deterministically in code** (`cmd/ai-review`
inspects the changed paths, the PR body, and the commit messages), not left to
the synthesis agent's judgement — §3 says escalation is mandatory "regardless of
finding severity", so it must not depend on a nondeterministic model call. A
diff that trips any trigger is forced to `needs-human-review` even if all five
dimensions report clean.

---

## 4. Review agent invocation

```
/code-review [--dimension bugs|security|perf|style|regression|all] [--effort low|medium|high]
```

- Default: `--dimension all --effort medium`
- High-effort runs use larger context windows and take ~3× longer; use for
  security-sensitive or architecture-changing PRs.
- `--comment` posts findings as inline GitHub PR comments.
- `--fix` applies fixable findings to the working tree (style, trivial bugs).

The review protocol is wired into CI via `.github/workflows/review.yml`, which
invokes the `cmd/ai-review` Go command. That check is **fail-closed and
required** (registered in `.github/rulesets/main.json`): the command maps the
§1 outcome to a process exit code, and only `approved` — or the audited human
override below — lets the check pass. `needs-work`, `rejected`, and
`needs-human-review`, as well as a missing API key, a fork PR, a per-dimension
API failure, synthesis failure, an unparseable outcome, or an oversize diff, all
**block the merge**.

### Human override (the verified fallback)

A maintainer who has consciously reviewed the change can apply the
`ai-review:override` label. The gate then posts its comment and passes on human
authority. The override is recorded on the PR (auditable) and requires a human
action (verified) — it is the sanctioned path to merge a fork PR, a
`needs-human-review` escalation, or a change the automated review could not
process.

---

## 5. Canonical PR merge workflow

```
1. Open PR (feature branch → main).
2. CI runs preflight (lint + build + vet) — MUST pass.
3. Review agent runs all 5 dimensions in parallel.
4. Synthesis produces outcome (§1).
   - approved       → merge (squash, delete branch)
   - needs-work     → author fixes → loop from step 2
   - rejected       → close PR; design new approach
   - needs-human    → human reviews + overrides or closes
5. After merge: delete branch.
```

No PR merges while any dimension has an unresolved finding with severity
`blocking` or higher. This is enforced, not aspirational: the required
`5-Dimension AI Review` check fails closed on every non-`approved` outcome (see
§4). Advisory findings (style, minor perf) may be deferred to a follow-up bead.
Every pushed revision is re-reviewed, so the check that gates the current head
SHA reflects that SHA — not an earlier draft.

---

## 6. Reviewer persona prompts

These are the system-prompt seeds injected into each dimension agent. Keep
them here so they version-control alongside the protocol.

### 6.1 Bugs agent

> You are a meticulous Go engineer reviewing a diff for logic errors. Your
> default assumption is that the code is wrong and you are trying to find
> the evidence. Focus on: incorrect error handling (swallowed errors,
> missing checks), nil pointer dereferences, off-by-one errors, incorrect
> use of goroutines and channels, and any code path that could panic or
> silently corrupt state. Report every finding with: file, line range,
> severity (blocking | advisory), and a one-sentence fix suggestion.
> If you find no issues, say so explicitly — do not invent findings.

### 6.2 Security agent

> You are a paranoid security engineer. Assume the diff will be deployed to
> production and that an attacker will read this review. Focus on: injection
> vectors (command, SQL, template), path traversal, privilege escalation,
> secret or credential exposure (including in logs), insecure defaults, and
> anything that weakens an existing security boundary. For each finding:
> state the attack scenario, the vulnerable line(s), the severity
> (blocking | advisory), and the minimal fix. Your default is to flag and
> escalate, not to approve quietly.

### 6.3 Performance agent

> You are a performance-focused Go engineer. Focus on: algorithmic
> regressions (e.g. O(n²) in a previously O(n) path), unnecessary heap
> allocations in hot paths, goroutine leaks (launched without a cancel path),
> missing context propagation, and unbounded slices or maps. For each
> finding: name the hot path, describe the regression, estimate the impact
> (blocking | advisory), and suggest the fix.

### 6.4 Style agent

> You are enforcing the project's Go style conventions. Check: exported
> symbols have doc comments; error strings are lowercase and do not end with
> punctuation; packages follow standard naming; golangci-lint rules are
> satisfied; and the diff does not introduce dead code or unused imports.
> Advisory-only: you cannot block a merge on style alone, but you must list
> all violations. Suggest the lint command or exact rewrite for each.

### 6.5 Regression agent

> You are checking whether this diff is consistent with the existing test
> suite. Look for: tests deleted without replacement, behaviour changes with
> no corresponding test update, and any changed exported symbol whose
> callers in the test suite now receive different semantics. If a changed
> function is untested, flag it. Report each gap with file, affected symbol,
> and whether it is blocking (deleted test coverage) or advisory (missing
> new coverage).

---

## 7. References

- [Autonomous merge policy](docs/policies/autonomous-merge.ai.md) — merge boundaries after review.
- `vbonnet/engram-research` `retrospectives/` — past incidents that shaped this protocol.
- `.github/workflows/review.yml` + `cmd/ai-review/` — the fail-closed CI gate.
- `.github/rulesets/main.json` — registers `5-Dimension AI Review` as a required check.
- Chezmoi `docs/REVIEW.md` — the *dotfiles* review protocol (different bar,
  same philosophy).

---

## 8. Reviewer standards quick reference

### Severity calibration

**Important** (blocking): findings that would break behavior, leak data, cause a
security vulnerability, or break CI/CD. Logic errors, unscoped operations, PII
exposure, and backward-incompatible changes qualify. Style, naming, and
refactoring suggestions are **Nit** at most.

**Cap nits:** report at most five Nits per review. If more found, say "plus N
similar items" in the summary.

### Do not report

- Anything CI already enforces: lint (golangci-lint), formatting (gofmt), type errors.
- Generated files, `go.sum` changes, vendor directory.
- Test-only code that intentionally violates production rules (test helpers, mocks).

### Always check

- New exported functions have godoc comments.
- Error handling: errors wrapped with context (`fmt.Errorf` with `%w`), not silently dropped.
- Concurrency: goroutines have `recover()`, channels properly closed, mutexes not held across I/O.
- No PII in log statements or error messages.
- File operations use atomic writes (`internal/fileutil`) not direct `os.WriteFile`.
- New CLI commands registered in the Makefile install targets.

### Repo-specific rules

- Go is the default language. Python/JS only with strong justification.
- All work happens in worktrees, never in `~/src/`.
- Force push is blocked at the settings level.
- OTel spans use `gen_ai.*` attribute naming conventions where applicable.
