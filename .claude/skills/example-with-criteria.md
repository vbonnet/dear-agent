---
name: run-preflight
description: Run the project's fast local CI-parity gate (vet + build + lint) and report pass/fail. Use when asked to "run preflight", "make preflight", or before pushing to verify the build is green.
model: haiku
effort: low
verification_criteria:
  - "`make preflight` exits 0"
  - "No `FAIL` lines appear in stdout"
  - "Stdout contains at least one `ok` line (confirms packages were linted/checked)"
---

# Run Preflight

Demonstrates the `## Verification Criteria` convention from
`docs/skill-verification-criteria.md`. The frontmatter above declares machine-readable
criteria; the section below is the human-readable mirror for code review.

## Quick start

```bash
make -C <repo-root> preflight
```

Runs vet + build + lint (~25 s). All three must pass for the gate to be green.

## Workflow

1. Identify the repo root (directory containing the `Makefile` with a `preflight` target).
2. Run: `make -C <root> preflight 2>&1 | tee /tmp/preflight-out.txt`
3. Check exit code.
   - **0** — report "preflight passed" and show the summary of `ok` lines.
   - **non-0** — show the first failing line with ±5 lines of context; suggest
     checking `docs/ERROR_GUIDE.md` for remediation steps.
4. Clean up: `rm -f /tmp/preflight-out.txt`

## Verification Criteria

The DEAR Auditor checks the following after this skill runs:

- [ ] `make preflight` exits 0
- [ ] No `FAIL` lines appear in stdout (any `FAIL` means a test or lint check failed)
- [ ] Stdout contains at least one `ok` line (confirms packages were actually checked,
  not skipped due to a misconfigured `GOFLAGS` or missing source)

These criteria mirror the `verification_criteria:` frontmatter above — both must
agree. The frontmatter is machine-readable; this section is visible during review.
