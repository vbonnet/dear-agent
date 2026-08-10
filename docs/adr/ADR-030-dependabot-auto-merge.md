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
neutral. Its protected-base classifier permits indirect requirement membership
changes and retained-requirement directness changes only alongside an existing
requirement version bump. Direct requirements may not be added or removed.
Membership of policy-annotated require blocks and non-tool-managed requirement
or require-block annotations remain fixed, and the classifier rejects
non-require module changes, special directives, extra files, stale branches,
and ambiguous evidence. The workflow then verifies the immutable Dependabot app-bot ID and
numeric repository identity, binds the REST commit response to the exact
reviewed head and current protected-base parent, and matches the canonical
Dependabot author and GitHub `web-flow` committer identities. An original head
must have no head-reference mutation event; a replaced head needs an exact-SHA
`head_ref_force_pushed` event by Dependabot or by an actor whose current GitHub
identity and permission response prove pre-existing `maintain` or `admin`
authority. API failures and ambiguous provenance return to ordinary fail-closed
review rather than crashing verdict publication. This avoids calling a
dependency version update a SPEC change without creating an ordinary-writer
bypass; the maintainer arm adds no authority beyond the audited revision
override maintainers already possess.

Rejected: making Gemini review bumps (wrong layer, wastes quota); a dependabot
arm inside the merge loop (puts mechanical bumps on the serial governed path);
blanket auto-merge including majors (that is where real breaking changes hide).
