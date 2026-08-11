# REVIEW.md — Multi-agent PR review protocol

- **Status:** authoritative
- **Last updated:** 2026-08-10

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
- **Pre/post-tool hooks** — any change to hook scripts or hook registration in `settings.json`. Canonical `SPEC.md` contracts under protected hook owners do not escalate solely because of that ownership and continue through the ordinary semantic-governance rules, as do exact lowercase `_test.go` files under authenticated Go-package hook owners; noncanonical aliases, production sources, scripts, registrations, and ambiguous tree evidence remain protected.
- **Security boundaries** — write guards, deny rules, `~/src` enforcement, PII manifests.
- **Protected review policy, production enforcement, and provider rules** — this policy, the trusted AI-review workflow, production reviewer implementation, and ruleset declarations. Go files with the exact lowercase ASCII `_test.go` suffix under `cmd/ai-review/` stay in the automated review loop because those files cannot enter the production reviewer binary; a rename crossing that boundary still escalates through its production-side path.
- **Privileged workflow authority** — executable changes to a workflow whose authenticated base or head can consume custom secrets, mint OIDC, write source/review/gate/deployment/signing state, use a privileged default-branch trigger, target an environment, delegate unresolved reusable-workflow or runner authority, or run on self-hosted infrastructure. Unclassifiable workflow evidence also fails closed.
- **Infrastructure that is expensive to reverse** — database schema changes, infrastructure-as-code, launchd plist installs, and systemd service installs.
- **Explicit `HUMAN REVIEW REQUIRED` label** in the PR description or commit message.

Escalation is not a failure state — it is a correct outcome that preserves
human authority over irreversible decisions.

The triggers above are enforced **deterministically in code** (`cmd/ai-review`
inspects the changed paths, the PR body, and the commit messages), not left to
the synthesis agent's judgement — §3 says escalation is mandatory "regardless
of finding severity", so those must not depend on a nondeterministic model
call. A diff that trips any of them is forced to `needs-human-review` even if
all five dimensions report clean.

**Size and scope are flagged separately, and are advisory, not a §3 trigger:**
a deterministic GitHub Action (`.github/workflows/pr-size-scope.yml`) computes
changed lines, changed files, and top-level areas on every push and posts a
split-suggestion comment once a PR crosses 1,000 changed lines, 50 changed
files, or 4 top-level areas. It comments only — it does not call into
`cmd/ai-review` and does not force `needs-human-review`. Reviewers should
still treat a flagged PR as needing a split before line-by-line review, but an
oversized diff does not by itself force escalation the way the triggers above
do.

**Stacked PRs and this protocol:** the five-dimension review workflow only
triggers for `branches: [main]`, and additionally validates that
`.base.ref == "main"`. A stacked PR (base ≠ `main`) does not get this review
at all.

Landing the stack's root does **not** fix this on its own: merging the root
changes no descendant's base, so every descendant stays outside the protocol
until it is explicitly rebased onto `main` and retargeted there (see
[CONTRIBUTING.md](CONTRIBUTING.md) — a squash landing means retargeting alone
would leave the descendant re-carrying its predecessor's diff). Treat each
stack member as unreviewed by this protocol until it has itself run with
`main` as its base.

Even then, a `main` base is necessary but not sufficient — `review.yml` takes
a neutral path when its plan reports `review_relevant=false`, which publishes
a green-looking check that is not a review.

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
invokes the `cmd/ai-review` Go command. That check is **fail-closed** when it
runs: the command maps the §1 outcome to a process exit code, and only
`approved` — or the audited human override below — lets the check pass.
`needs-work`, `rejected`, and `needs-human-review`, as well as a fork PR, a
per-dimension API failure, synthesis failure, an unparseable outcome, or an
oversize diff, all fail the workflow check. They block a merge only when the
provider has separately made that exact check required.

**Keyless exception (skip-with-warning).** While `ANTHROPIC_API_KEY` is not
configured as a repository secret, a same-repository, non-override run whose
*sole* blocker is the absent credential — an otherwise-reviewable changed
SPEC, a §3 escalation whose accompanying model review cannot run, or a
bounded-wire proof that no admissible prompt or complete verdict for the diff
fits the reviewer's own budgets — exits
with the distinct code 78 ("review cannot run"), and the workflow publishes
that one disposition as a **neutral-with-warning** check instead of a
failure: no approval is claimed, the command still posts its
`needs-human-review` evidence on the PR, and the neutral comment states that
human review is recommended before merge. Conclusive SPEC-governance verdicts
that need no model (ownership edges, reviewer-dependency changes,
traceability failures, stale-base evidence) stay blocking even keyless, as do
any verdict mixing such a finding with a bounded-wire proof, fork PRs, plan
build errors, expired deadlines, override audit failures, and
any failure while a key *is* configured. A bound proof is not a conclusive
verdict: it reports that no verdict was reachable, its remedy is ours rather
than the contributor's, and its size tracks the repository's SPEC tree rather
than the diff. Configuring the secret makes exit 78
unreachable and restores the full fail-closed contract above with no further
change.

> [!NOTE]
> **Provider state verified 2026-07-31**: `ANTHROPIC_API_KEY` is unset and the
> active GitHub ruleset has no AI-review required context. The workflow source
> therefore does not prove provider-required enforcement or that an LLM ran.
> `cmd/ai-review` remains fail-closed whenever it is invoked; a credential,
> unique authoritative check name, and exact-head canary are prerequisites for
> a later reviewed Terraform/ruleset enforcement rollout.

### Known residual risk: workflow-definition trust

The reviewer binary and workflow definition are loaded from the protected base
revision; the PR revision is only ever diffed, never executed.

Changes to `REVIEW.md`, `.github/workflows/review.yml`, `.github/rulesets/`, or
production and governance files under `cmd/ai-review/` are deterministic §3
escalation triggers, so the candidate revision cannot approve changes to its
own review authority. Protected owner and dependency paths use normalized
Unicode case-fold identities and symmetric ancestor/descendant matching, so a
file/directory alias, a slashless protected directory such as `vendor`, or a
changed ancestor cannot shadow a canonical trust root. One bounded authenticated
base/head tree index supplies the raw spelling, mode, type, and complete
directory ancestry for all of these decisions. Tree-only directory aliases and
one unique non-tree peer per revision may remain automated; a tree/non-tree
conflict, multiple non-tree peers, or a cross-revision tree/non-tree transition
fails closed. Protected static leaves and SPEC controls retain their stricter
canonical spelling rules.
Changes confined to Go files with the exact lowercase ASCII `_test.go` suffix
under that package stay automated only when the same tree evidence proves a
regular-blob test leaf and compatible directory ancestry. Unique
case/normalization-fold test-to-test renames and regular/executable mode changes
remain automated because both exact lowercase-suffix paths stay outside the
production package.
Changed-path extraction
disables rename detection and retains both sides, so a rename crossing into or
out of production still escalates. Other workflow changes stay in the automated
review loop. A bounded authenticated comparison
of every changed workflow identity inspects all normalized Unicode case-fold
peers in both the merge base and head, so removing privilege or adding a shadow
alias cannot evade classification. Proven read-only or low-blast workflows, and
comment/format/key-order-only edits whose executable YAML is unchanged remain
automated; critical authority or ambiguous evidence escalates.
Matrix axis order is retained because Actions uses that order to determine job
creation order; swapping those keys is therefore an executable change.

The protected OpenCode, Pi, AGM Git-hook, AGM internal-hook, and Codex-hook
owners apply the same distinction: their executable hook surfaces remain
human-review triggers, while an exact canonical regular `SPEC.md` is left to
the ordinary semantic contract rules; independent ownership validation still
applies, as do independent permission, migration, and infrastructure triggers.
Exact lowercase `_test.go` changes are additionally
automated only beneath enumerated Go-package hook owners. Both carveouts depend
on authenticated canonical directory ancestry and regular Git blobs;
simultaneous same-revision normalized peers, owner aliases,
file/directory conflicts, symlinks, gitlinks, and noncanonical suffixes or
basenames fail closed.

This classifier is deliberately scoped to changed workflow YAML. It does not
yet compute transitive authority through separately changed scripts, reusable
workflow targets, local actions, or workflow-to-workflow event, artifact, and
cache producer/consumer edges. Direct reusable-call jobs fail closed as
delegated authority; separately changed dependencies and upstream workflows
remain outside this classifier unless another deterministic path rule covers
them. Authenticated dependency-closure enforcement—including ambiguous or
dynamic workflow, artifact, and cache names—is tracked in `ce-juhj.22.3`.

The finite standard GitHub-hosted runner allowlist proves only the workflow's
single declared label, matched byte-exactly apart from ASCII case; quoted
leading or trailing whitespace is label data and fails closed. Event names,
permission scopes, and permission levels follow the same rule, because YAML
syntax whitespace is already removed while quoted whitespace remains semantic
data. Multi-label sequences fail closed because Actions
requires one runner to satisfy every label, and a self-hosted runner can claim
the whole conjunction even when each spelling is a standard hosted label. The
static allowlist cannot authenticate provider-side runner registration or a
self-hosted/larger-runner label collision; that inventory boundary is tracked
in `ce-juhj.22.4`.

Changed `SPEC.md` files have an additional authenticated contract review. The
same fail-closed boundary inventories complete normalized Unicode case-fold
path identities in both the authenticated merge base and head. It rejects
noncanonical basenames, multiple non-tree control peers, nonregular control
leaves, tree/non-tree ancestor conflicts, and control-leaf spelling drift across
revisions. Addition-only file/directory shadows and case-only or
normalization-only control renames therefore cannot evade review, while
tree-only directory peers and one stable unique regular control leaf remain in
the automated contract-review path. A directory merely named `SPEC.md` or
`SPEC.owner` is not itself a contract unless an authenticated non-tree control
exists at that identity. The
trusted base supplies both the canonical authoring policy and the exact
package-level `activeHarnesses` registry. Additions or modifications under a
registered dotted root, plugin root, or explicit harness grouping are rejected
as local normative owners, including when nested beneath `internal/` or `cmd/`.
Those root, plugin, grouping, and member identities are normalized so a
checkout alias cannot create a harness-local contract owner.
Those package seams remain valid locations when they own a logical product or
domain contract without an intrinsic registration root. Every current promise
in an added or modified SPEC must receive a
final `supported`, `adapted`, `unsupported`, or `not-applicable` disposition
for every active member. Native differences belong in applicability-scoped
requirements under the shared owner, not in a peer harness SPEC. An owner
search that would be truncated escalates to a maintainer instead of presenting
partial evidence as complete.

The semantic reviewer must keep uncertain ownership separate from confirmed
defects: incomplete or low-confidence semantic evidence is
`needs-human-review`, not an invented canonical owner or a blocking conclusion.

### Authenticated dependency automation

A dependency version bump does not change a SPEC contract. The trusted review
plan therefore identifies a narrow automation candidate when Git proves that a
current-base change modifies only `go.mod` (optionally with `go.sum`), preserves
the module, Go, toolchain, and all non-require directives, changes at least one
existing requirement version, and has no other review or escalation evidence.
The parsed require graph may add or remove only requirements marked indirect,
or reclassify retained requirements between direct and indirect, as part of
that authenticated update. Direct requirements may not be added or removed.
Membership of any policy-annotated require block and every non-tool-managed
requirement or require-block annotation remain fixed. The workflow may publish
a neutral, non-model verdict only after GitHub's trusted API resolves current
open-PR and protected-main snapshots and every diff, identity, body, label,
base, and publication decision is bound to their exact object IDs rather than
the event's possibly stale payload, embedded base, or a mutable pull-request
ref. Override authority exists only in an exact-current-head labeled event by
an actor whose current `maintain` or `admin` permission is verified. Synchronize
and every other event ignore retained labels and bot-authored markers without
mutating the cosmetic label. The trusted APIs must
also match Dependabot's immutable app-bot ID and numeric repository identity,
bind the exact current head to the canonical Dependabot/GitHub commit identities
and the current protected-base parent, and show either an unmodified original
head or an exact-head force push by Dependabot or an existing maintain/admin
principal.

This is not a contributor-facing bypass. A fork, a claimed bot login, a branch
name, a label, a graph-only change with no existing version bump, any `replace`,
non-require directive change, extra path, stale-base evidence, or parsing
failure keeps the ordinary fail-closed path. Patch/minor merge
eligibility remains owned separately by `.github/workflows/dependabot-automerge.yml`
and all provider-required checks must still pass.

An organization-level required workflow remains defence in depth against a
malicious maintainer with push access.

### Human override (the verified fallback)

A repository maintainer or administrator who has consciously reviewed the
current revision can apply the `ai-review:override` label. The trusted workflow
activates the override only in that exact `labeled` event when the event head
matches the current API-resolved head and GitHub currently reports the event
actor has `maintain` or `admin` permission. A persistent label or bot-authored
comment carries no authority on a synchronize or any other event. A later push
therefore invalidates the override. The workflow deliberately leaves the now-
cosmetic label in place to avoid a delayed synchronize run racing a newer label
application; to approve the same or a later revision after another event, remove
the stale label and apply it again. The override is therefore auditable,
revision-bound, and requires a fresh maintainer action — it is the sanctioned path to merge a fork PR, a
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
`blocking` or higher. Advisory findings (style, minor perf) may be deferred to
a follow-up bead. Every pushed revision is re-reviewed, so the check that
reports on the current head SHA reflects that SHA — not an earlier draft.

> [!WARNING]
> This is **policy, not currently provider-required machine enforcement**.
> The active ruleset has no AI-review context, so no source workflow result is
> proof that a merge was blocked or that an LLM reviewed the revision. A future
> reviewed infrastructure rollout must use a unique authoritative check and an
> exact-head canary before changing that state.

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
- `.github/workflows/review.yml` + `cmd/ai-review/` — trusted workflow source;
  provider-required status must be verified separately.
- Live GitHub ruleset and its Terraform owner — provider-required check truth;
  do not infer it from `.github/rulesets/main.json`.
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
