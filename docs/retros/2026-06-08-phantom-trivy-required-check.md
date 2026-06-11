# Retro: the phantom "Trivy" required check that blocked PR cascade #93–#97

**Date:** 2026-06-08
**Bead:** ce-6as.63 (P1) · legacy BL-070 · recurring since 2026-05-11
**Author:** VROOM Worker
**Status of finding:** root cause confirmed; fix is an admin action (see Action items)

## Define

PRs #93, #94, #95, #97 (and #88, #96 ahead of them) could not land normally. A
required status check never reported, so the merge gate could never go green.
The cascade only unblocked when the in-front PRs were **admin-merged**
(`#88`, `#94`, `#96`, `#97` are now `MERGED`); **`#93` and `#95` are still
`OPEN` and still blocked today**, because the phantom check is still configured
on `main`.

## Execute (the diagnosis)

`main` branch protection requires six status check contexts:

| Required context                       | app_id  | App            | Produced by a workflow? |
|----------------------------------------|---------|----------------|-------------------------|
| `Build & Test (ubuntu-latest)`         | 15368   | GitHub Actions | ✅ `ci.yml` job `ci` (matrix `os`) |
| `Build & Test (macos-latest)`          | 15368   | GitHub Actions | ✅ `ci.yml` job `ci` (matrix `os`) |
| `Analyze Go Code (go)`                 | 15368   | GitHub Actions | ✅ `codeql.yml` job `analyze` (matrix `language: [go]`) |
| `govulncheck`                          | 15368   | GitHub Actions | ✅ `ci.yml` job `govulncheck` |
| `Bash Script Size Check (20-line limit)` | 15368 | GitHub Actions | ✅ `language-policy.yml` job `bash-line-limit` |
| **`Trivy`**                            | **57789** | **NOT GitHub Actions** | ❌ **never produced — phantom** |

### The mismatch

Five of the six required contexts are produced by GitHub Actions
(`app_id 15368`) and match a job `name:` exactly (matrix suffixes included).
The sixth, **`Trivy`, is bound to `app_id 57789`** — a *different* GitHub App
(a standalone Marketplace Trivy/Aqua app), not GitHub Actions.

That app is not installed / never runs on this repo. Trivy **is** executed in
CI, but as **steps inside Actions jobs** in `sbom-scan.yml`:

- job `sbom-generation` → check context **`Generate SBOM`**
- job `vulnerability-scan` → check context **`Vulnerability Scan`**

GitHub derives a check-run's context from the **job name**, never from the
tool a step happens to invoke. So Trivy's results post under `Generate SBOM`
and `Vulnerability Scan` — **never under a context named `Trivy`**.

### Confirmation

Actual check-run contexts produced on recent `main` commits:

```
Analyze Go Code (go)            Build & Test (macos-latest)
Bash Script Size Check (...)    Build & Test (ubuntu-latest)
Generate SBOM                   Go Vulnerability Check
Vulnerability Scan              govulncheck
AGM E2E Install (debian/ubuntu) check-ci-health
```

A context named `Trivy` appears on **0 of the last 20 main commits**. With
`strict: true` and `Trivy` required, the required-checks set is **unsatisfiable
by any normal PR** → every PR sits "Expected — Waiting for status to be
reported" forever, and only an admin merge gets past it.

## Audit (why it wasn't caught)

`branch-protection-audit.yml` only verifies that `required_status_checks`
**exists** (presence). It never cross-checks each required *context* against the
set of contexts any workflow actually **produces** (`check-runs` is referenced
0 times). A required context bound to an uninstalled app — or a typo'd context
name — passes the audit silently. That is the gap that let this phantom persist
across multiple cascades since 2026-05-11.

## Retro — Action items

1. **Fix the required-check set (admin action).** On `main` protection,
   replace the `Trivy` context with the contexts the workflow actually emits —
   **`Vulnerability Scan`** (the gating job that runs `trivy ... --exit-code 1`)
   and optionally **`Generate SBOM`**. Equivalent API call (admin only):

   ```bash
   gh api -X PATCH repos/vbonnet/dear-agent/branches/main/protection/required_status_checks \
     -f 'checks[][context]=Build & Test (ubuntu-latest)' \
     -f 'checks[][context]=Build & Test (macos-latest)' \
     -f 'checks[][context]=Analyze Go Code (go)' \
     -f 'checks[][context]=govulncheck' \
     -f 'checks[][context]=Vulnerability Scan' \
     -f 'checks[][context]=Bash Script Size Check (20-line limit)'
   ```

   (Alternative, if a context literally named `Trivy` is wanted: rename the
   `vulnerability-scan` job to `name: Trivy`. Renaming the required context is
   the lower-risk option.)

2. **Close the gap in `branch-protection-audit.yml`.** Extend the audit to
   flag any required context that has not been produced as a check-run on the
   default branch in the last N commits — i.e. detect *unproducible* required
   contexts, not just missing protection blocks. Tracked as a follow-up
   (own scoped agent; not bundled here per principle 1).

3. **Re-evaluate #93 and #95** once action item 1 lands — they should become
   mergeable through the normal gate, retiring the admin-merge workaround.

## Why this is a retro, not an inline fix

The required-check change touches `main` branch protection (admin-only,
consequential, shared state) — out of scope for a Worker to apply unilaterally.
This document is the durable artifact; the protection edit and the audit
hardening are escalated as scoped action items above.
