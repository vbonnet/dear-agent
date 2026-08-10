# ADR-030: Auto-merge Dependabot patch/minor PRs via GitHub Actions

Status: Accepted (2026-06-15)

Dependabot opens ~15 dependency-bump PRs/week. Our merge path requires a Gemini
bot review before a PR can land, but Gemini never reviews `dependabot[bot]`
PRs — so every bump stalls forever on a review that never arrives, rotting and
accumulating conflicts while it competes for the serial merge slot.

`.github/workflows/dependabot-automerge.yml` auto-approves and enables
GitHub-native auto-merge for `dependabot[bot]` PRs, scoped to **patch and minor**
bumps only: `dependabot/fetch-metadata@v3` classifies the update and the approve
+ merge steps skip when it is `version-update:semver-major`. Majors keep the
human-review path, where breaking-change risk lives; feature PRs are untouched
and still go through the merge loop / `safe-merge`.

It stays safe because auto-merge inherits branch protection verbatim: `--auto`
holds the merge until required checks pass, `--squash` preserves the required
linear history, the job is gated on `github.actor == 'dependabot[bot]'`, and
every step uses `secrets.GITHUB_TOKEN` (the only third-party action is GitHub's
own `fetch-metadata`). The property we add is "a green patch/minor dependabot PR
merges without review"; we do not weaken "every merge to main passed required
checks."

The trusted SPEC-review workflow independently treats an authenticated
same-repository Dependabot dependency-version-led `go.mod`/`go.sum` delta as
neutral. Its protected-base classifier permits parsed require-graph updates only
alongside an existing requirement version bump, while rejecting non-require
module changes, special directives, extra files, stale branches, and ambiguous
evidence; the workflow then verifies the immutable Dependabot app-bot ID and
numeric repository identity from GitHub's trusted event context. This avoids
calling a dependency version update a SPEC change without creating a
contributor-controlled bypass.

Rejected: making Gemini review bumps (wrong layer, wastes quota); a dependabot
arm inside the merge loop (puts mechanical bumps on the serial governed path);
blanket auto-merge including majors (that is where real breaking changes hide).
