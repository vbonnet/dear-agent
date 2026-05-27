# DEAR Retro: <title>

**Date:** YYYY-MM-DD
**Severity:** Low | Medium | High (one-line rationale)
**Status:** Resolved | Partially resolved | Open — one-line state

This is the **Audit + Retro** half of the DEAR loop for "<task / incident name>".
The **Define / Execute** halves are [link to ADR or design doc, if applicable].

> **Template note (delete this block in real retros).**
>
> This template is **dear-agent's** DEAR retro template — DEAR's four phases
> (Define / Execute / Audit / Retro) are the canonical, framework-level
> structure; the four subsection headings marked *(dear-agent addition)*
> below are project-specific requirements layered on top by
> [ADR-024](../adr/ADR-024-push-bike-and-dear-retro-extensions.md).
>
> Other repos that adopt DEAR are not bound by these additions (per
> [ADR-024 § D4 Per-project extension mechanism](../adr/ADR-024-push-bike-and-dear-retro-extensions.md)).
> Inside *this* repo they are MANDATORY — skipping one is a retro smell;
> fill it with "n/a: <reason>" rather than removing the heading.
>
> **Core design constraint:** *push-bike, not training wheels.* Prefer
> fixes that teach the system the right reflex at target scale, not
> bolted-on caps that have to be removed before the system can ride for
> real. See [/CONTEXT.md § Push-bike, not training wheels — design constraint for DEAR retros (dear-agent)](../../CONTEXT.md#push-bike-not-training-wheels--design-constraint-for-dear-retros-dear-agent).

---

## Define

State the task and its exit conditions (acceptance criteria).

### Scaling model *(dear-agent addition — MANDATORY)*

- **Current scale:** <today's numbers — agents, sessions, repos,
  requests/min, whatever the relevant denominator is>
- **Target scale:** <where we credibly expect to be in 12 months>
- **Scaling model:** <linear / sub-linear / super-linear in what dimension —
  e.g. "linear in concurrent workers; super-linear in worker × repo
  because every worker scans every repo">
- **Binding constraint at target:** <what runs out first — file
  descriptors, tokens/min, lock contention, human review bandwidth,
  SQLite write throughput>

If any of these are unknown, write "unknown — needs measurement" and add a
Retro action item to measure it. Sizing a fix to unmeasured load is the
exact failure mode this section exists to prevent.

---

## Execute

Do the work. Keep this section *narrative*: a numbered list of changes,
in order, each with a one-line outcome. Cross-reference PRs / commits /
files. Every Execute item must appear in the Scale-test table below.

### Ideal-first solution *(dear-agent addition — MANDATORY)*

Two passes, in this order. Do not reorder them.

**Ideal scalable solution (write this first).** Describe the shape the
system would take if we were rebuilding it today with the target scale in
mind. **No reference to what currently exists.** No reference to what we
can ship this week. This is the destination.

**Path-compatible minimum (write this second).** Describe the smallest
change we can ship now that is **on the path to the ideal above** — i.e.
nothing in it has to be ripped out to get there. If the minimum has to be
ripped out, it is rejected: go back to the ideal and find a different
minimum.

If you cannot articulate the ideal without referencing current code, the
ideal is not actually scoped — restart it from a blank page.

---

## Audit

Verify the runnable exit conditions actually hold. Standard DEAR Audit:
what was checked, what passed, what failed.

### Scale-test *(dear-agent addition — MANDATORY)*

Score every proposed fix against three axes. Each axis is **scales**,
**neutral**, or **caps**.

| Proposed fix      | Agents (10×) | Machines (10×) | Users (10×) | Rip-out tax (if any `caps`) |
|-------------------|--------------|----------------|-------------|------------------------------|
| <fix 1>           | scales       | scales         | neutral     | —                            |
| <fix 2>           | caps         | neutral        | neutral     | `path/to/thing.why.md`       |

**Rules:**

- A fix scoring **scales** or **neutral** on all three axes ships without
  further ceremony.
- A fix scoring **caps** on any axis MUST carry a co-located `.why.md`
  declaring the rip-out tax (see template below). The `.why.md` ships in
  the same PR as the cap.
- A fix that nobody can score because the scaling model (Define) is
  unknown cannot proceed past Scale-test until the model is filled in.

### Rip-out tax declarations *(dear-agent addition — MANDATORY when any `caps` above)*

For every `caps`-scored fix in the table above, link or inline the
co-located `.why.md`:

```markdown
# <thing>.why.md — temporary cap

**Type:** cap (rip-out tax)
**Date:** YYYY-MM-DD
**Why this is here:** <the load it relieves, the regression it prevents>
**What it costs to remove:**
  - Code touched: <files / packages / call sites>
  - Migration shape: <what has to change to delete this cleanly>
**Removal trigger:** <the observable signal that says "now safe to remove">
**Related retro:** <link to this DEAR retro>
```

---

## Retro

Capture what was learned. Two parts:

### What went well

Brief — typically one or two patterns worth repeating.

### What didn't

Brief — typically one or two patterns to avoid next time. Be specific:
"X did Y because Z" beats "we should be more careful".

### Action items

| # | Action | Owner | Status |
|---|--------|-------|--------|
| 1 | <e.g. measure the unknown in Define's scaling model> | TBD | open |
| 2 | <e.g. remove the cap when `removal trigger` observed>  | TBD | open |
| 3 | <e.g. follow-up ADR to formalize the ideal-shape above> | TBD | open |

Findings flow to the backlog/roadmap via the Meta-Orchestrator
([CONTEXT.md § VROOM](../../CONTEXT.md)).

---

## References

- [ADR-024: Push-Bike Design Constraint and dear-agent's DEAR Retro Extensions](../adr/ADR-024-push-bike-and-dear-retro-extensions.md)
- [/CONTEXT.md § DEAR](../../CONTEXT.md)
- [/CONTEXT.md § Push-bike, not training wheels — design constraint for DEAR retros (dear-agent)](../../CONTEXT.md#push-bike-not-training-wheels--design-constraint-for-dear-retros-dear-agent)
- [AGENTS.why.md § Why the `.why.md` pattern is also the rip-out-tax tracker](../../AGENTS.why.md)
- <any prior retro this one follows from>
