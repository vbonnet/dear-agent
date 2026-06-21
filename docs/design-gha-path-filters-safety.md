# Design Spike: Safe GHA Path Filters (with skipped-check safety)

- **Bead:** ce-xulg.1 (spike phase)
- **Status:** Proposed
- **Date:** 2026-06-20
- **Scope:** Design only. No workflow YAML is changed by this document.

## Context

Every PR push currently fans out the full CI matrix plus several LLM-backed
review workflows. A docs-only PR (e.g. editing `*.md`) pays for the entire Go
build/test matrix and ~5 Claude review invocations even though no code changed.
Adding `paths:` / `paths-ignore:` filters would let those PRs skip the expensive
work.

The hazard: **a path-filtered workflow that does not run never reports its
status check.** If that check is *required* by branch protection, GitHub leaves
it `pending` forever and the PR can never merge. (A job *skipped via `if:`* is
reported as `skipped` and **does** satisfy required checks — only the
workflow-level `paths:` trigger filter is dangerous for required checks.)

So path filters are only free for **non-required** workflows. Required ones need
the shim pattern below.

## 1. Workflows firing on every PR push

Workflows with a `pull_request:` trigger targeting `main`:

`agm-e2e-install`, `branch-protection-audit`, `ci`, `codeql`,
`dependabot-automerge`, `deepsec`, `doc-proximity`, `gemini-quota-fallback`,
`language-policy`, `pr-review-agent`, `review`, `routing-enforcement`,
`sbom-scan`, `security-audit`, `shell-matrix`, `structural-health`.

## 2. Required-check audit

`gh api repos/vbonnet/dear-agent/branches/main/protection` →
`required_status_checks.contexts` (`strict: false`):

| Required context | Workflow | Job |
|---|---|---|
| `Build & Test (ubuntu-latest)` | `ci.yml` | `ci` (matrix) |
| `Build & Test (macos-latest)` | `ci.yml` | `ci` (matrix) |
| `Analyze Go Code (go)` | `codeql.yml` | `analyze` |
| `govulncheck` | `ci.yml` | `govulncheck` |
| `Bash Script Size Check (20-line limit)` | `language-policy.yml` | `bash-line-limit` |
| `Vulnerability Scan` | `sbom-scan.yml` | `vulnerability-scan` |

Only **four** workflows hold required checks: `ci`, `codeql`, `language-policy`,
`sbom-scan`. Crucially, the heavy LLM review workflows — `review` (5-dimension,
~5 Claude calls), `pr-review-agent`, and `deepsec` — are **not** required. They
are informational/`continue-on-error` today.

## 3. Workflow-safety table

| Workflow | Required check? | Safe to path-filter? | Shim needed? |
|---|---|---|---|
| `review` (5-dim AI) | No | ✅ Yes | No |
| `pr-review-agent` | No | ✅ Yes | No |
| `deepsec` | No | ✅ Yes | No |
| `ci` (Go matrix + govulncheck) | **Yes** | ⚠️ Only with shim | **Yes** |
| `codeql` | **Yes** | ⚠️ Only with shim | **Yes** |
| `sbom-scan` (Vulnerability Scan) | **Yes** | ⚠️ Only with shim | **Yes** |
| `language-policy` (bash limit) | **Yes** | ⚠️ Only with shim | **Yes** |

The three review workflows are the cheap, zero-risk win: a plain
`paths-ignore:` removes the ~5 Claude calls on docs-only PRs with no branch-
protection interaction.

## 4. The skipped=success workaround

Two viable patterns. **Pattern A is recommended** — it needs no branch-
protection change and survives the matrix-check gotcha.

### Pattern A — twin-workflow shim (recommended)

Split each required workflow into a *real* file (runs on code paths) and a
*shim* file (runs on the inverse paths). **Both must emit the identical workflow
name and job check name** so exactly one reports the required context green per
PR. The mutually-exclusive path filters guarantee exactly one fires.

`ci.yml` (real):
```yaml
on:
  pull_request:
    branches: [main, develop]
    paths: ['**.go', 'go.mod', 'go.sum', '.github/workflows/ci.yml']
jobs:
  ci:
    name: Build & Test (${{ matrix.os }})
    # ...unchanged...
```

`ci-skip.yml` (shim — same names, inverse filter, always passes):
```yaml
name: CI
on:
  pull_request:
    branches: [main, develop]
    paths-ignore: ['**.go', 'go.mod', 'go.sum', '.github/workflows/ci.yml']
jobs:
  ci:
    name: Build & Test (${{ matrix.os }})
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ubuntu-latest
    steps:
      - run: echo "Docs-only PR — Go build/test skipped, reporting success."
```

### Pattern B — in-workflow `if:` + `always()` aggregator gate

Keep one workflow (no top-level `paths:`); detect changes, gate heavy jobs, and
make a single `always()` gate job the required check instead of the matrix jobs.

```yaml
jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      code: ${{ steps.f.outputs.code }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: f
        with:
          filters: |
            code: ['**.go', 'go.mod', 'go.sum']
  build:
    needs: changes
    if: needs.changes.outputs.code == 'true'
    # ...heavy matrix...
  ci-gate:                      # <-- this becomes the required check
    name: CI Gate
    needs: [changes, build]
    if: always()               # runs even when build is skipped
    runs-on: ubuntu-latest
    steps:
      - run: |
          r="${{ needs.build.result }}"
          if [ "$r" = "failure" ] || [ "$r" = "cancelled" ]; then exit 1; fi
          echo "ok (build was $r)"
```

`if: always()` forces the gate to run regardless of upstream skips; it fails
only on real failure, passes on success *or* skip. Trade-off: it requires an
admin to swap the required contexts (matrix names → `CI Gate`), so it is a
larger one-time change than Pattern A.

## 5. Path categories for dear-agent

```yaml
# docs-only — safe to skip code/security workflows
docs-only:
  - '**/*.md'
  - 'docs/**'
  - 'LICENSE'
  - '.gitignore'

# go-code — must trigger full CI/security
go-code:
  - '**/*.go'
  - 'go.mod'
  - 'go.sum'
  - '.github/workflows/**'   # never skip when CI config itself changes
```

Express skips as `paths-ignore: [docs-only]` (fail-safe: any unlisted path runs
the full pipeline). Always treat `.github/workflows/**` as code so a workflow
edit is never self-skipped.

## 6. Recommended implementation order (safest first)

1. **W0 — non-required review workflows.** Add `paths-ignore` (docs-only) to
   `review.yml`, `pr-review-agent.yml`, `deepsec.yml`. Zero branch-protection
   risk; captures the ~5-Claude-call savings immediately.
2. **W1 — `sbom-scan` + `language-policy`** via Pattern A twin-shim. Lower blast
   radius than CI; validates the shim mechanics on a required check.
3. **W2 — `codeql`** via Pattern A.
4. **W3 — `ci`** via Pattern A (matrix), last because it is the highest-value
   and highest-risk required check.

Each step is independently revertable. Verify on a throwaway docs-only PR that
the required contexts go green (via the shim) before promoting the next.

## 7. W0 requirements (first safe PR)

Add to the top-level `pull_request:` block of `review.yml`,
`pr-review-agent.yml`, and `deepsec.yml`:

```yaml
on:
  pull_request:
    branches: [main]
    paths-ignore:
      - '**/*.md'
      - 'docs/**'
      - 'LICENSE'
      - '.gitignore'
```

Because none of these three are required checks, a skipped run leaves no pending
context and the PR merges normally. No shim, no branch-protection change.

**Acceptance:** open a docs-only test PR → confirm `review`, `pr-review-agent`,
`deepsec` show *no* runs while the four required checks still pass → confirm a
Go-touching PR still triggers all three.
