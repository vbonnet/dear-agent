# ADR-029: Dependabot Auto-Merge via GitHub Actions

**Status**: Accepted
**Date**: 2026-06-15
**Context**: Dependabot opens ~15 dependency-bump PRs per week (gomod + GitHub
Actions, see `.github/dependabot.yml`). Our normal merge path requires a Gemini
bot review before `safe-merge`/the merge loop will land a PR. Gemini never
reviews `dependabot[bot]`-authored PRs, so every bump stalls — blocked on a
review that will never arrive. The backlog of green-but-unmergeable dependency
PRs is pure carrying cost: each one rots, accumulates conflicts, and competes
for the serial merge slot.

---

## Decision

Land a dedicated GitHub Actions workflow,
`.github/workflows/dependabot-automerge.yml`, that auto-approves and enables
GitHub-native auto-merge for `dependabot[bot]` PRs — but **only** for patch and
minor bumps. This is a deliberately **hybrid** posture:

- **Dependabot PRs → GHA auto-merge.** Mechanical, low-risk, high-volume, and
  invisible to the human/Gemini review path. A platform-native workflow is the
  right tool: it reacts to every `pull_request` event with zero operator
  involvement and zero new infrastructure.
- **Feature PRs → merge loop / `safe-merge`.** Human- and Gemini-reviewed work
  keeps going through the existing governed path. We do **not** widen the GHA
  auto-merge to non-dependabot authors.

### How it stays safe

1. **Scoped to the bot.** `if: github.actor == 'dependabot[bot]'` gates the
   whole job; nothing else can trigger it.
2. **Major bumps excluded.** `dependabot/fetch-metadata@v2` classifies the
   update; the approve and merge steps are skipped when
   `update-type == 'version-update:semver-major'`. Majors fall back to the
   normal human-review path.
3. **`--auto`, not an immediate merge.** GitHub holds the merge until all
   required status checks pass. A red CI run blocks the bump exactly as it
   blocks any other PR — auto-merge does not bypass branch protection.
4. **`--squash`.** Preserves the linear history that branch protection
   requires (`requiresLinearHistory=true`).
5. **First-party token only.** Every step uses `secrets.GITHUB_TOKEN`. No
   third-party action gets repo write access; `fetch-metadata` is the single
   dependency and is a GitHub-published action.

---

## Consequences

**Positive**

- Patch/minor dependency PRs land the moment CI is green, with no operator and
  no Gemini review — closing the gap that left them stalled forever.
- The serial merge slot stops being clogged by mechanical bumps, so genuine
  feature PRs reach `main` faster.
- Major bumps still get a human in the loop, where the actual breaking-change
  risk lives.

**Negative**

- A patch/minor bump that is green but semantically broken can self-merge. Risk
  is bounded by CI coverage: if our tests pass, the bump is as trusted as any
  green change. Majors — the usual home of breaking changes — are excluded.
- Two merge mechanisms now coexist (GHA for dependabot, merge loop for
  everything else). The author split is crisp (`github.actor`), so the
  ambiguity is low, but it is a second path to understand.

**Trust boundary**

Auto-merge inherits the branch-protection gate verbatim — required checks must
still pass. We add the property "a green patch/minor dependabot PR merges
without review"; we do **not** weaken "every merge to main passed required
checks."

---

## Alternatives considered

- **Make Gemini review dependabot PRs.** Wrong layer: bumps don't need a
  semantic LLM review, and Gemini's quota is better spent on feature PRs. Would
  also keep them on the slow serial path.
- **A `dependabot` arm inside the existing merge loop.** Reuses our tooling but
  puts high-volume mechanical merges through the same serial, governed pipeline
  built for reviewed feature work — the merge loop would spend its budget on
  bumps. The platform-native event-driven workflow is simpler and free.
- **Blanket auto-merge including majors.** Rejected: majors carry the real
  breaking-change risk and warrant a human.
