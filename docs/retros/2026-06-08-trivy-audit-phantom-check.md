# DEAR Findings: Trivy Supply-Chain Audit + Phantom `Trivy` Required Check

**Date:** 2026-06-08
**Author:** VROOM Worker
**Severity:** Medium (phantom required check is a latent merge blocker; Trivy
itself is clean)
**Status:** Audit complete — Trivy kept; one branch-protection fix pending
admin action (documented below, command blocked from agent execution by design)

## Define

Two questions, one cleanup:

1. Is the Trivy GitHub Action on a safe (non-compromised) version, pinned
   defensively?
2. Does Trivy earn its place given we already run govulncheck + CodeQL +
   SBOM, or is it redundant?
3. The `main` branch protection lists a required status check named
   **`Trivy`** that no workflow ever produces — a phantom that can never go
   green. Remove or correct it.

## Audit

### 1. Trivy version — CLEAN ✅

All five Trivy invocations live in `.github/workflows/sbom-scan.yml`, every
one pinned to a **full commit SHA**:

```
aquasecurity/trivy-action@ed142fd0673e97e23eac54620cfb913e5ce36c25 # v0.36.0
```

Verification against the upstream release:

```
$ gh api repos/aquasecurity/trivy-action/git/refs/tags/v0.36.0
  → annotated tag object a9c7b0f06e461e9d4b4d1711f154ee024b8d7ab8
$ gh api repos/aquasecurity/trivy-action/git/tags/a9c7b0f0...
  → commit ed142fd0673e97e23eac54620cfb913e5ce36c25   ← matches pin exactly
```

The pinned SHA is the genuine `v0.36.0` commit. This is the supply-chain-safe
pattern: a SHA pin cannot be moved by the action publisher, so a compromise of
the `trivy-action` repo (or a malicious retag) cannot inject code that runs
with our `GITHUB_TOKEN`. The prior `@master` exposure was already closed in
PR #96 (2026-05-10 retro). No remediation needed.

### 2. Cost/benefit — KEEP Trivy ✅

Trivy is **not** redundant with govulncheck + CodeQL. The other two scanners
are Go-only:

- **CodeQL** — `.github/workflows/codeql.yml` matrix is `language: ['go']`.
- **govulncheck** — Go-native, call-graph aware (only reachable vulns).

Trivy's `scan-type: fs` covers the dependency surface they cannot see:

| Surface | Covered by | Trivy-only? |
|---|---|---|
| Go module vulns (reachable) | govulncheck | no |
| Go static analysis | CodeQL | no |
| npm/JS deps | **Trivy** | **yes** |
| Container/IaC (Dockerfiles) | **Trivy** | **yes** |
| SBOM (CycloneDX + SPDX) | **Trivy** | **yes** |

The repo has real non-Go dependency surface Trivy is the only gate for:

```
wayfinder/review/package-lock.json
engram/mcp/package.json
.deepsec/package.json
agm/agm-plugin/channels/agm-bus/package-lock.json
agm/test/e2e/docker/Dockerfile  (+ agm/tests/e2e-install/Dockerfiles)
```

Trivy is also the **sole SBOM generator** (`sbom-cyclonedx.json`,
`sbom-spdx.json`, 90-day retention). govulncheck and CodeQL produce neither.

**Verdict:** keep Trivy. It is complementary, not overlapping.

### 3. Phantom `Trivy` required check — CONFIRMED, fix pending

Live branch protection on `main`:

```
$ gh api repos/vbonnet/dear-agent/branches/main/protection/required_status_checks
contexts:
  - Build & Test (ubuntu-latest)     [app 15368 GitHub Actions]
  - Build & Test (macos-latest)      [app 15368 GitHub Actions]
  - Analyze Go Code (go)             [app 15368 GitHub Actions]
  - govulncheck                      [app 15368 GitHub Actions]
  - Trivy                            [app 57789  ← NOT GitHub Actions]
  - Bash Script Size Check (20-line limit) [app 15368 GitHub Actions]
```

Actual check-runs produced on `main` HEAD (`a5666dd2`):

```
Generate SBOM            [app 15368] success
Vulnerability Scan       [app 15368] success   ← this is the Trivy gate
Go Vulnerability Check   [app 15368] success
... (no check named "Trivy" exists anywhere)
```

**Root cause:** the required check `Trivy` is registered against app `57789`
(a non-GitHub-Actions app — likely a leftover from a directly-installed
Trivy/Aqua GitHub App that no longer runs). None of our workflows emit a
check named "Trivy"; the Trivy-based job in `sbom-scan.yml` is named
**`Vulnerability Scan`** (its final `Run Trivy with exit code` step fails the
job on CRITICAL/HIGH, `ignore-unfixed: true`). So the required `Trivy` check
never reports and is unsatisfiable for any non-admin PR flow.

Why it has not blocked everything: `enforce_admins: false` (deliberate
solo-dev lockout-recovery choice, see 2026-05-10 retro) lets admin pushes
bypass it. But for the normal PR path it is a latent, permanent blocker —
exactly the kind of cosmetic-protection drift the 2026-05-10 retro warned
the required-checks list would accumulate ("Required-status-checks list will
drift as workflows are added / renamed").

This is also a second drift from that retro's documented 7-check intent:
`Language Policy Enforcement` is no longer in the live list (now 6 contexts).
That is a *separate* discrepancy and is **out of scope** for this task —
flagged here only so it is not lost. File it separately rather than bundling.

## Fix (requires admin — agent execution intentionally blocked)

The fix is **branch-protection state, not source** — no workflow file change
is needed (Trivy stays, on its clean pin). Replace the phantom `Trivy` with
the real Trivy gate `Vulnerability Scan` so merge-blocking intent is
preserved rather than silently dropped:

```bash
cat > /tmp/da-required-checks.json <<'JSON'
{
  "strict": true,
  "checks": [
    {"context": "Build & Test (ubuntu-latest)", "app_id": 15368},
    {"context": "Build & Test (macos-latest)", "app_id": 15368},
    {"context": "Analyze Go Code (go)", "app_id": 15368},
    {"context": "govulncheck", "app_id": 15368},
    {"context": "Vulnerability Scan", "app_id": 15368},
    {"context": "Bash Script Size Check (20-line limit)", "app_id": 15368}
  ]
}
JSON

gh api -X PATCH \
  repos/vbonnet/dear-agent/branches/main/protection/required_status_checks \
  --input /tmp/da-required-checks.json
```

`app_id: 15368` is GitHub Actions — scoping each context to that app prevents
a third-party app from spoofing a required check. `Vulnerability Scan` runs on
every PR (`sbom-scan.yml` `on: pull_request`), so it is a valid gate.

When this Worker attempted the PATCH, the Claude Code auto-mode classifier
denied it (high-severity shared security config; agent-inferred `app_id`).
That denial is correct per the JIT/least-privilege model — branch-protection
edits belong to the human admin. No workaround was attempted. **Action item:
user runs the command above.**

Verify afterward:

```bash
gh api repos/vbonnet/dear-agent/branches/main/protection/required_status_checks \
  --jq '.contexts'
# expect: no "Trivy"; "Vulnerability Scan" present
```

## Retro

- **What was healthy:** Trivy is pinned correctly and pulls real weight
  (npm + Docker + SBOM the Go-only scanners miss). No supply-chain action
  needed.
- **What drifted:** the required-checks list outlived the check names that
  back it — the exact failure mode the 2026-05-10 retro predicted but did
  not mechanize a guard for. A check named for a *tool* (`Trivy`) rather
  than the *job that runs it* (`Vulnerability Scan`) is brittle: the job
  name is the contract, the tool name is not.
- **Follow-up (separate scope, do not bundle):**
  1. `Language Policy Enforcement` missing from required checks vs. the
     2026-05-10 documented intent — confirm whether intentional.
  2. The retro's own open item — "a small Go tool that diffs
     `.github/workflows/*.yml` job names against the live protection and
     reports mismatches" — would have caught this `Trivy` phantom
     automatically. Worth building now that it has recurred.
