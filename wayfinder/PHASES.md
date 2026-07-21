# Wayfinder phases

Wayfinder has one active phase sequence.

| # | Phase | Purpose | Typical artifact |
|---:|---|---|---|
| 1 | CHARTER | Bound the objective, scope, constraints, and success conditions | `CHARTER-charter.md` |
| 2 | PROBLEM | Establish evidence, users, impact, and the problem boundary | `PROBLEM-statement.md` |
| 3 | RESEARCH | Search existing solutions and decide what to reuse or build | `RESEARCH-existing-solutions.md` |
| 4 | DESIGN | Define architecture, seams, invariants, and trade-offs | `DESIGN-architecture.md` |
| 5 | SPEC | State observable requirements and acceptance criteria | `SPEC-solution-requirements.md` |
| 6 | PLAN | Order implementation and verification work | `PLAN-design.md` |
| 7 | SETUP | Prepare the workspace and task breakdown | `SETUP-plan.md` |
| 8 | BUILD | Implement and record reproducible validation and delivery evidence | `BUILD-evidence.md` |
| 9 | RETRO | Record outcomes, deviations, lessons, and remaining work | `RETRO-retrospective.md` |

The generic deliverable rule is `<PHASE>-*.md`. Some gates require the exact
names above. `SETUP` may be skipped only when the session was started with
`--skip-roadmap`.

The phase order is declared in
`wayfinder/cmd/wayfinder-session/internal/status/types_v2.go` and exposed by
`status.AllPhases()`. Normal commands reject missing or unsupported schema
versions; there is no runtime compatibility path.
