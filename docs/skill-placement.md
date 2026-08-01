<!-- Last audited at: 2026-07-31 -->

# Skill Placement — which repo owns a skill, and how it reaches a session

Companion to `docs/skill-tiers.md` (model/effort contract) and
`docs/skill-verification-criteria.md` (exit-condition contract). Those two say
what a skill must *declare*. This one says where it must *live*.

Derived from a cross-model pass (synthesis + adversarial review) on a real
case: a `research-pipeline` orchestrator skill proposed in a private,
single-operator dotfiles repo.

---

## The principle

**A skill belongs to the repo that owns the rule it enforces — not the tools it
calls, and not the files it writes.**

Invoked tooling is a *dependency*. Storage layout is an *interface*. Neither
confers ownership. Ask instead: when this skill produces a bad outcome, whose
invariant was violated?

Why not the tempting alternatives:

| Candidate axis | Why it fails |
|---|---|
| "Whoever can lint it" | Only dear-agent has skill CI today, so it collapses to "everything → dear-agent." |
| "Whichever repo's commands it invokes" | Nearly every mesh skill invokes `bd`, `wayfinder`, or `safe-pr`. Same collapse. A `deploy-vbonnet-ai` skill calls `safe-pr` but plainly belongs to vbonnet.ai. |
| "Co-locate so breakage fails the same CI run" | **Not true today.** `pkg/skilllint` checks structure (metadata, workflow shape, verification section, length, duplicates). It does not validate referenced CLI flags or storage contracts, and nothing consumes `evals.json`. Co-location buys review proximity and `git grep` reach, not enforcement. Claim the benefit you actually have. |

## The checklist

Ordered. First decisive answer wins. Deliberately front-loads the questions
that are answerable in seconds; dependency archaeology is last, because it is
the slow one.

| # | Question | → |
|---|---|---|
| 1 | Is it a **mesh role identity** (what a supervisor *is*, not what anyone *does*)? | `cmd/vroom-dispatch/skills/` |
| 2 | Is the rule it enforces **personal taste, voice, or cadence** — would another operator reasonably want a different rule? | dotfiles (`dot_claude/skills/`) |
| 3 | Is the rule **generic craft** that no repo here owns (GitHub PR hygiene, transcript grabbing)? | dotfiles / user-level |
| 4 | Does one repo own the **invariant the skill protects**? | that repo |
| 5 | Does it mix a repo-owned mechanic with personal taste? | **split**: mechanic in the owning repo, thin taste wrapper in dotfiles that delegates |

If 1–4 are all "no", you have a skill with no owner. That is a signal the skill
is underspecified, not a signal to default it to dear-agent.

### Escape hatch

If the sole consumer is the human on this machine, and breakage is immediately
visible and costs minutes, dotfiles is fine regardless of the above. dear-agent's
required checks plus bot review are not free. Do not pay them for a cheat sheet.

### Trust boundary

dotfiles is private and single-operator. dear-agent is the shared product repo.
A skill that hard-codes machine-specific topology (a beads database path such
as `$BEADS_DIR`, or an absolute checkout path for a private knowledge-base
repo) publishes that topology when it moves. Either parameterize the paths or
accept the disclosure knowingly.

---

## Placement is not distribution

Placement decides which repo versions and reviews the source of truth.
Distribution decides which running sessions can load it. **They are separate
obligations**, and conflating them is a defect class we have already shipped:
the writing pipeline lives correctly in `~/.claude/skills/`, and Cowork Dispatch
sessions therefore draft prose with no Vale gate, because nothing there loads it.

A placement decision is incomplete until every consumer class has a named load
path — and **a named path is not proof**. Each one needs a smoke test that the
skill actually discovers and triggers.

### How skills actually reach sessions in this repo

| Surface | Reaches | Mechanism |
|---|---|---|
| `wayfinder/skills/`, `agm/agm-plugin/skills/` | Claude Code sessions with the plugins installed | `.claude-plugin/marketplace.json` → per-plugin `plugin.json` declaring its skills directory |
| `spec-governance/skills/` | Claude Code and Pi | canonical authored skills exported by the isolated `spec-governance` Claude plugin and loaded from `.pi/settings.json` |
| `agm/plugins/`, `wayfinder/skills/` | Pi | `.pi/settings.json` |
| `.agents/skills/` | Codex, AGY, and OpenCode fallback discovery | `.dear-agent/marketplace.json` declares `agents-md-skill-fallback`; authored skills remain canonical where they live, while `make lint-skills` checks any deterministic regular-file projections |
| `.claude/skills/` | Claude Code sessions cwd'd in this repo | holds a worked example today; no cross-repo reach |
| `cmd/vroom-dispatch/skills/` | VROOM supervisors | shipped with the dispatcher |
| Cowork / Desktop Dispatch | **undetermined** | no repository evidence establishes any of the above reaches Cowork. Verify with a live session before promising it. |

The neutral catalog declares a Pi fallback surface, while `.pi/settings.json`
lists Pi's native skill roots. The
[neutral marketplace contract](../.dear-agent/SPEC.md) and
[Pi adapter contract](../.pi/SPEC.md) describe these complementary surfaces.
This placement guide records the observed topology without choosing a
cross-harness catalog owner while that audit decision remains pending.

### The canonical packaging pattern

Use one authored workflow body with the narrowest projection that every target
loader actually discovers. `wayfinder` is the worked symlink example:
`wayfinder/SKILL.md` is canonical, `wayfinder/skills/wayfinder/SKILL.md` is a
tracked symlink, the Claude plugin manifest exports the directory, and both
marketplace catalogs register the plugin.

`spec-governance` is the worked regular-projection example. Claude's isolated
plugin root authenticates only `spec-governance` and its manifest exports only
the two canonical skill directories; this keeps the collector, module metadata,
and canonical EARS library in one installed boundary without exposing the root
repository's agents, hooks, MCP servers, or language servers. Pi loads the
canonical tree directly. Codex, AGY, and OpenCode load generated regular
`.agents/skills/{write-spec,audit-specs}/SKILL.md` delegators because the active
runtimes did not all discover symlinks consistently. Those files contain no
second workflow; `cmd/sync-skill-projections` derives them from canonical
metadata, creates only missing targets without clobbering, and never replaces
or deletes an existing entry automatically. Stale and obsolete paths must be
inspected and explicitly removed by a human; `make lint-skills` blocks drift in
preflight and CI.

Do not blindly copy either filesystem mechanism. Verify each claimed loader,
prefer a symlink when all consumers support it, and otherwise generate a
minimal delegator with deterministic equality checks. Never maintain two
handwritten workflow copies.

### Version skew is real

Source checkout, installed Go binaries, and the installed plugin snapshot are
independent deployment states. The repository catalogs and plugin manifests
agree on their declared versions, but that does not prove any local plugin
cache is current. Merging a skill to `main` does not put it in a session. The
distribution step includes reinstalling the plugin and restarting the harness,
followed by a discovery smoke test in each claimed consumer.

---

## Worked verdicts

| Skill | Rule it enforces | Owner |
|---|---|---|
| `research-pipeline` (a candidate skill proposed in a private dotfiles repo) | cross-model verification, human gate before execution, beads sized for one run — DEAR process discipline | **dear-agent** |
| writing pipeline, `linkedin-cross-post` | Valentin's voice and cadence | dotfiles (+ a Cowork distribution gap to close) |
| `github-thread-resolver` | verify the fix landed before resolving — generic PR hygiene | dotfiles |
| a hypothetical `deploy-vbonnet-ai` | vbonnet.ai's release policy (even though it calls `safe-pr`) | vbonnet.ai |

### Research-pipeline workflow boundary

Placement in dear-agent does not make Wayfinder the skill's decomposition
engine. The research pipeline owns its decomposition contract and creates every
execution unit through the configured canonical Beads store. Wayfinder's PLAN
phase records a project plan; its Beads adapter creates project tracking
identity and does not turn that plan into sized, dependency-linked tasks.

Wayfinder is an optional downstream workflow for an execution unit whose scope
justifies the full SDLC. When used, its status and history artifacts follow the
repository routing policy into the configured Engram research store; they do
not make the temporal research or run record living dear-agent documentation.

---

## Known limits

1. **The checklist is provisional whenever ownership or reachability is
   unknown.** Q4 can require repo archaeology; when it does, the 30-second answer
   is a hypothesis, not a verdict.
2. **No compatibility enforcement exists.** Until skill-lint validates invoked
   command surfaces (or `evals.json` is actually executed in CI), a flag rename
   in dear-agent can silently invalidate a co-located skill. That gap is the
   single highest-value follow-up this doc implies.
3. **Cowork reach is unproven** for every surface listed above. Any claim that a
   placement fixes a Cowork gap must be backed by a live session test.
