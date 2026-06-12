# D3 — Chosen Approach

**Defense in depth across four tiers, with the wrapper as the policy core:**

```
        ┌──────────────────────────────────────────────────────┐
 instruct │ CLAUDE.md/AGENTS.md rules + exit-2 teaching messages │
        ├──────────────────────────────────────────────────────┤
 enforce  │ T1 server: zero-bypass rulesets (public repos, $0)   │
          │ T2 wrapper: safe-merge predicate → atomic merge      │
          │ T3 agent:  fsguard checkGh + deny rules + allow-list │
        ├──────────────────────────────────────────────────────┤
 verify   │ T4 audit: merge-audit cron + post-merge Action       │
        └──────────────────────────────────────────────────────┘
```

This instantiates the existing three-tier philosophy (instruct → enforce → verify) from AGENTS.md, and maps each user requirement to at least two independent layers (see D4 traceability).

Key approach decisions:

1. **The wrapper owns the predicate** (not GitHub): because on free private repos GitHub enforces nothing and on public repos required-check lists drift (phantom-Trivy retro). The wrapper gates on **all** checks present on the head SHA, not just required ones, and treats `UNKNOWN`/pending as *wait*, never pass.
2. **Waiting is a feature:** `safe-merge` blocks-and-watches (checks, expected bot reviewers) by default with a hard timeout — the compliant path requires zero extra keystrokes versus the old bypass path.
3. **Merge is TOCTOU-safe:** predicate verified against a captured `headRefOid`, merge executed with `expectedHeadOid`; any racing push → server 409 → re-verify.
4. **Break-glass is human-only and slow:** interactive TTY + typed reason + audit trail; not reachable by agents, not a flag on the normal path.
5. **Go, in dear-agent** (principle 4), reusing `internal/safegit` patterns and `cmd/resolve-review-threads` GraphQL primitives; policy in `internal/safemerge` so hooks and wrapper share one implementation.
6. **Deployment split resolved deliberately:** Go binaries → `~/go/bin` via Makefile (never into chezmoi-managed dirs); hook *registration* + deny rules → chezmoi source via the REVIEW.md strict gate.
