# D1 — Charter / Pre-planning

**Problem statement (W0):** [../../docs/w0-speed-merging.md](../../docs/w0-speed-merging.md)

## Scope

- A `safe-merge` wrapper that becomes the only sanctioned way to merge
  PRs in any of vbonnet's repos, enforcing: checks complete+green,
  expected bot reviewers finished, all review threads resolved (with
  substantive replies), no admin/force flags.
- Blocking of the raw paths (`gh pr merge`, REST/GraphQL merge calls,
  GitHub web UI where feasible) for agents and, best-effort, for the
  human shell.
- A break-glass path that is slower than compliance and leaves an audit
  record.
- Server-side hardening to the extent free plans allow (rulesets on
  public repos, defense-in-depth detection elsewhere).
- Regression monitoring: recurring audit of merged PRs.

## Non-goals

- Paid GitHub plans as a prerequisite.
- Defending against a deliberately malicious local root user.
- Changing what CI/Gemini/REVIEW.md check — only enforcing that their
  verdicts are consumed.
- Implementation in this Wayfinder pass — this project ends at the
  reviewed design (S6/S7 plan); implementation is follow-up beads.

## Constraints

See W0 §Constraints: free-plan private repos (no server-side
protection), solo owner (no second admin), agents as dominant merge
actors, prefix-match porosity of deny rules, existing prior art
(safe-push, pretool guards, resolve-review-threads).

## Stakeholders

- vbonnet (owner, sole human).
- All Claude Code / AGM / VROOM agent sessions (the regulated actors).
- Gemini Code Assist + CI (the gate-producing systems).

## Definition of Done (this Wayfinder project)

- [ ] W0 committed (docs/w0-speed-merging.md)
- [ ] Research findings recorded (S4)
- [ ] Design doc committed (docs/design-safe-merge.md) with phased
      implementation plan
- [ ] Beads filed per implementation phase
- [ ] All work committed to feat/safe-merge branch
