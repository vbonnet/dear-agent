# Security Pipeline Evaluation — Bumblebee + DeepSec

**Date:** 2026-05-23
**Author:** research pass (no changes yet)
**Status:** §4.1 accepted — see [ADR-027](../adr/ADR-027-bumblebee-endpoint-scanner.md); §4.2 (don't add DeepSec to scheduled task) accepted; §4.3 (audit-task sandbox host-access fix) and §3 (DeepSec cost levers) remain open follow-ups
**Scope:** assess Perplexity's Bumblebee and check our DeepSec integration; recommend
whether/how to fold them into the existing scheduled audit pipeline.

---

## TL;DR

| Question | Answer |
|---|---|
| Is DeepSec already integrated? | **Yes — extensively.** `~/src/dear-agent/.deepsec/` is provisioned, `.github/workflows/deepsec.yml` runs incrementally on every PR, a pre-push hook is installable via Makefile, and `pkg.json` pins `deepsec@^2.0.8`. A point-in-time full scan was bootstrapped in `~/worktrees/dear-agent/deepsec-scan/`. |
| Can subsequent DeepSec runs be cheaper? | **Already incremental.** `deepsec process --diff origin/main` is the steady-state mode. The remaining cost lever is `revalidate` (prior-verdict reuse) and limiting full scans to dedicated worktrees on demand. |
| Should we add Bumblebee? | **Yes — different layer.** DeepSec audits source code; Bumblebee inventories what's *installed on the developer endpoint* (packages, MCP configs, browser/editor extensions). They are complementary, not overlapping. |
| Add either to `weekly-security-audit`? | **Bumblebee: yes**, if we fix the sandbox-host-access gap; otherwise it has to run via a separate launchd/cron entry. **DeepSec: no** — the existing CI gate is the right surface for it. |
| Cost implications | Bumblebee: $0 (no LLM, single Go binary). DeepSec incremental: scales with PR diff size; full scans documented as "thousands to tens of thousands of dollars" for large codebases. |

---

## 1. Current state

### 1.1 DeepSec integration (already heavy)

DeepSec is wired into dear-agent at multiple layers:

| Surface | File | What it does |
|---|---|---|
| Workspace | `.deepsec/` | Provisioned project root with `INFO.md`, custom matchers, lockfile |
| CI | `.github/workflows/deepsec.yml` | Runs on every PR vs `main`, scans only changed files, posts findings as a marker-deduplicated PR comment. **Non-blocking** (always reports success). Gated on `ANTHROPIC_API_KEY` secret — no-ops cleanly if absent or for fork PRs. |
| Local diff scan | `make deepsec-incremental` → `scripts/deepsec-incremental.sh` | One-shot scan of files changed since `origin/main`. Wraps `deepsec process --diff <ref>` |
| Local staged scan | `make deepsec-staged` | Pre-commit-style check on staged changes (`--diff-staged`) |
| Pre-push hook | `make install-deepsec-hook` (+ `STRICT=1` variant; `DEEPSEC_SKIP=1` bypass) | Sentinel-bounded block in `.git/hooks/pre-push`; coexists with `prepush-act-validator`. **Soft-fail by default** |
| Worktree carry-forward | 58 dear-agent worktrees, 1 brain-v2 worktree | `.deepsec/` travels with feature branches |
| Docs | `docs/deepsec.md` | Cost model, modes, triage flow |

**Cost model (per `docs/deepsec.md`):**

- Local: $0 — auto-detects `claude` CLI subscription
- CI: billed against `ANTHROPIC_API_KEY` repo secret; workflow no-ops if absent
- Fork PRs skipped (no secret access)

### 1.2 `weekly-security-audit` scheduled task

- **Task ID:** `weekly-security-audit`
- **Schedule:** `0 3 * * *` — **daily at 03:00 UTC** (the name is misleading)
- **Model:** `claude-opus-4-7`
- **Last run:** 2026-05-23T10:02:24.745Z
- **SKILL:** `~/Documents/Claude/Scheduled/weekly-security-audit/SKILL.md`
- **Config:** `~/Library/Application Support/Claude/local-agent-mode-sessions/294f5b58-…/8efeacfe-…/scheduled-tasks.json`

**What it actually does:** _web research only_. No `userSelectedFolders` → zero host
filesystem access. The SKILL is a strict-citation news scan over the last 7 days
across: npm/PyPI supply-chain, Node/Python/macOS, common frameworks, browsers, dev
tools. Output is candidate-findings markdown stamped **UNVERIFIED** with a
ready-to-paste verification prompt for the host. By design the audit and the
verification are decoupled to prevent hallucinated "your machine has X" claims.

**Last accessible run output:** `…/local_6dfc4ece-…/outputs/security-audit-candidate-findings-2026-04-28.md`
(npm `pgserve` 1.1.11-1.1.13 credential-harvester; PyPI `elementary-data` 0.23.3
`.pth`-based exfiltration). Both UNVERIFIED. The sandbox can't reach NVD/MITRE/CISA
which causes legitimate incidents to be dropped on the two-source rule, not on
plausibility — a known gap of this design.

### 1.3 Other security tooling already present

| Tool | Surface | Status |
|---|---|---|
| Trivy (SBOM) | `.github/workflows/sbom-scan.yml` (pinned by commit hash to v0.36.0; SARIF → CodeQL) | Active |
| CodeQL | `.github/workflows/codeql.yml` (Go, TS/JS) + `wayfinder/review/.github/workflows/security.yml` | Active |
| Gitleaks | Referenced in `docs/features/term-denylist.md` | Available, not configured |
| Semgrep | Referenced in ADR-011 | Not active |

---

## 2. Bumblebee — what it is and how it would fit

### 2.1 What it does

Bumblebee (Perplexity, just open-sourced, Apache 2.0, v0.1.1) is a **read-only
inventory collector** for developer endpoints, written in Go with **zero non-stdlib
dependencies**. Single static binary, one-shot scanner — scheduling is operator's
job (cron, launchd, MDM).

**Sources:** [marktechpost coverage](https://www.marktechpost.com/2026/05/23/perplexity-open-sources-bumblebee-a-read-only-supply-chain-scanner-for-developer-endpoints/) · [github.com/perplexityai/bumblebee](https://github.com/perplexityai/bumblebee) · [Perplexity blog](https://www.perplexity.ai/hub/blog/perplexity-is-open-sourcing-bumblebee)

**Scans:**

- Package ecosystems: npm, pnpm, Yarn, Bun, PyPI, Go modules, RubyGems, Composer —
  by reading lockfiles and on-disk `*.dist-info/METADATA`, **never invoking the
  package manager** (so scanning cannot trigger install-time attacks)
- MCP / AI tool configs: `mcp.json`, `claude_desktop_config.json`, etc.
- Editor extensions: VS Code, Cursor, Windsurf, VSCodium
- Browser extensions: Chrome, Comet, Edge, Brave, Arc, Firefox

**Scan profiles:** `baseline` (common roots), `project` (dev dirs), `deep`
(operator-specified roots).

**Output:** NDJSON — hostname, OS/arch, ecosystem, package name+version, source
file, confidence (high/med/low), severity, catalog ID. Matches are against
**operator-supplied exposure catalogs** — Bumblebee itself is the collector; the
catalog is what tells it which versions are "bad."

**Doesn't do:**

- Execute lifecycle hooks or invoke npm/pip
- Read application source (so no overlap with DeepSec)
- Process or network monitoring (not an EDR)

**Install:** `go install github.com/perplexityai/bumblebee/cmd/bumblebee@latest`
(needs Go 1.25+). macOS + Linux only.

### 2.2 Why it complements what we already have

|  | DeepSec | Bumblebee | Trivy | weekly-security-audit |
|---|---|---|---|---|
| Layer | Source code | **Developer endpoint** | Container/SBOM | Web research |
| Reads source files | yes | **no** | dep manifests | n/a |
| Reads endpoint installs | no | **yes (lockfiles + installed metadata)** | no | no |
| Scans MCP/editor/browser configs | no | **yes** | no | no |
| LLM cost | yes (per-file agent) | **none** | none | yes (research) |
| Runtime | minutes-hours / PR | seconds | seconds | minutes |

The gap Bumblebee fills uniquely: **what is actually installed on this laptop**,
including MCP configs and IDE/browser extensions — neither DeepSec nor Trivy nor
the audit task look there. Given how many MCP servers and IDE extensions we run
(see system reminders this session: `claude-in-chrome`, `Claude_Preview`,
`computer-use`, `mcp-registry`, several `mcp__plugin_*`…), this is a real blind
spot.

### 2.3 Catalog question — the open issue

The Bumblebee binary is only as useful as the **exposure catalog** it's matched
against. Two options:

1. **Maintain our own** (small, tracks incidents we care about — e.g. the
   `pgserve` / `elementary-data` findings the audit task surfaces). Tight loop:
   audit task → catalog entry → next Bumblebee run flags installed instances.
2. **Wait for community catalogs** to coalesce around the project. Premature today
   (v0.1.1, days old).

Recommendation: option 1, scoped narrow — catalog only contains things our
audit task or a published incident actually flagged. Catalog lives in this repo
(it's operational config, not research).

---

## 3. DeepSec — cost-reduction opportunities

The user's intuition that "subsequent runs can be cheaper" is correct but the
mechanism is already in place at the CI/local layer:

| Lever | Status | Notes |
|---|---|---|
| `--diff origin/main` (incremental) | **Already default** for `make deepsec-incremental` and CI | Scans only PR-changed files |
| `--diff-staged` | Available via `make deepsec-staged` | Pre-commit gate |
| `revalidate` (prior-verdict reuse) | Documented; manual invocation | Cuts false-positive rate without re-running full agent investigation. Worth promoting in the triage workflow. |
| Resume on failure | Built-in | "Re-run the same command — deepsec picks up where it left off" |
| Sandbox mode (`sandbox <cmd>` with `--sandboxes N --concurrency`) | Available | Distributed execution on Vercel Sandbox; not currently used. Worth piloting **only** if we start running full-repo scans on a schedule. |
| Promote CI to required check | Pending signal-to-noise stabilisation | `.ci-policy.yaml > branch_policies.main.required_workflows` |

**Where there is room to lower cost further:**

1. **Skip the pre-push hook on docs-only changes.** The wrapper script doesn't
   gate on file type. If the diff is only `*.md`/`docs/**`, exit 0 without
   calling the agent. Saves quota for the long tail of doc PRs.
2. **Make `revalidate` part of the soft-fail loop**: when the pre-push hook
   would warn, first run revalidate on prior findings touching the changed
   files. Cheaper than full investigation, fewer false-positive nags.
3. **Don't carry `.deepsec/` into every worktree.** 58 worktrees × NPM workspace
   is a real disk-and-`pnpm install` tax. Consider a shared workspace at
   `~/.local/share/deepsec/` keyed by repo, or `.gitignore`-ing the workspace
   below the project boundary. (Caveat: needs verification that
   `deepsec process --diff` still resolves project context correctly without a
   per-checkout `.deepsec/`.)

None of these need to happen for Bumblebee adoption — they're independent.

---

## 4. Recommendations

### 4.1 Add Bumblebee (recommended)

**Why:** fills a real gap — installed-package and MCP/extension inventory on this
machine — that nothing else in the stack covers, with $0 ongoing cost.

**How (proposed, not implemented):**

1. Add a thin wrapper at `scripts/bumblebee-scan.sh` that:
   - Runs `bumblebee` with `project` profile against `~/src/` and `~/worktrees/`
   - Emits NDJSON to `~/Library/Application Support/dear-agent/bumblebee/<date>.ndjson`
   - Diffs against the previous run; flags only new/changed lines
2. Schedule via **launchd** (not the Cowork scheduled-task system — that has the
   zero-host-access problem). Daily at 04:00 local. Output a one-line summary
   to a file the next `weekly-security-audit` run reads (when the
   sandbox-host-access gap is fixed; see §4.3).
3. Curate `~/Library/Application Support/dear-agent/bumblebee/catalog.json` —
   seeded from recent audit-task findings (`pgserve`, `elementary-data`, etc.).
   One PR per added incident.
4. Document at `docs/bumblebee.md` mirroring `docs/deepsec.md`'s structure.

**Out of scope for v1:** no CI integration (Bumblebee is endpoint-focused, not
repo-focused — running it in CI would inventory the runner, which is meaningless).

### 4.2 Don't add DeepSec to the scheduled task

DeepSec is already at the right surface: every PR, incremental, with a documented
path to promote to required. Adding it to the daily audit task would either
duplicate or burn quota on no-PR days. Leave it where it is.

The one improvement worth filing: **add docs-only short-circuit** to
`scripts/deepsec-incremental.sh` (§3, lever 1).

### 4.3 Fix the `weekly-security-audit` sandbox gap (separately)

The audit task drops verifiable findings because the sandbox can't reach NVD /
MITRE / CISA. `docs/deepsec.md:122-125` already flags this as a known
weekly-audit bug. Two options, neither implemented:

1. **Allowlist** NVD/GHSA/Socket/CISA hosts in the Cowork sandbox config (if
   exposed).
2. **Demote the sandboxed task to a feed-only pass**, and add a separate
   host-side task (launchd) that does the verification against installed
   packages — which is exactly what Bumblebee would feed.

§4.1 + this make a coherent pair: Bumblebee answers "what's installed?",
the audit task answers "what new issues were disclosed?", and they intersect in
a host-side verification step that produces the actually-useful "this machine is
exposed to X" alert.

### 4.4 Don't merge anything yet

This is a research doc. Concrete next steps if accepted:

- [ ] Spawn issue: "Add Bumblebee endpoint scanner (launchd, daily, $0 cost)"
- [ ] Spawn issue: "DeepSec incremental: docs-only short-circuit"
- [ ] Spawn issue: "weekly-security-audit sandbox can't reach NVD/MITRE/CISA — propose verification split"
- [ ] Decide on Bumblebee catalog ownership (in-repo vs. side repo) — small enough that in-repo wins on simplicity

---

## 5. Risks / things this report could be wrong about

- **Bumblebee is v0.1.1, days old.** API may change; catalog format isn't
  community-standard yet. Adopting now means writing our own catalog and re-doing
  it when one emerges.
- **The 58-worktree DeepSec workspace footprint** is asserted by the Explore
  agent, not directly counted in this pass. If lower, the §3 "shared workspace"
  point is less urgent.
- **DeepSec `revalidate` cost savings are claimed by docs**, not measured here.
  Worth a quick benchmark before promoting it in the triage workflow.
- **Sandbox host-access fix (§4.3) depends on Cowork internals** I haven't
  inspected. The two-task split is a fallback if the allowlist option isn't
  possible.

---

## 6. Sources

- [MarkTechPost — Bumblebee announcement](https://www.marktechpost.com/2026/05/23/perplexity-open-sources-bumblebee-a-read-only-supply-chain-scanner-for-developer-endpoints/)
- [github.com/perplexityai/bumblebee](https://github.com/perplexityai/bumblebee)
- [Perplexity blog — Open-sourcing Bumblebee](https://www.perplexity.ai/hub/blog/perplexity-is-open-sourcing-bumblebee)
- [github.com/vercel-labs/deepsec](https://github.com/vercel-labs/deepsec)
- Local: `docs/deepsec.md`, `.github/workflows/deepsec.yml`,
  `~/Documents/Claude/Scheduled/weekly-security-audit/SKILL.md`,
  the 2026-04-28 audit output in the Cowork session dir.
