# Workflow & Scheduling Engine Comparison

**Status:** Draft
**Date:** 2026-05-04
**Author:** workflow-engine-comparison session
**Related:**
[ADR-010 — Workflow Engine as Substrate-Quality Work-Item Layer](../adrs/ADR-010-workflow-engine-architecture.md),
[Workflow Engine Operator Guide](../workflow-engine.md)

## Problem

dear-agent has shipped three phases of an in-tree workflow engine
(`pkg/workflow`, ~2700 LOC) backed by SQLite + WAL, with audit emission,
roles, budget metering, HITL, exit gates, and structured outputs. ADR-010
already decided to evolve this engine rather than adopt Temporal or
LangGraph, but the rejection rationales were one-line entries in an
"alternatives considered" table.

This doc is the long-form version: a side-by-side comparison of the
candidates against the constraints we actually run under, written as a
durable reference so future "should we switch?" questions can be answered
without redoing the analysis.

The framing also clarifies a category confusion that recurs in casual
discussion: **DAG orchestration** (run these nodes in this order, with
retries and audit) and **time-based scheduling** (fire this thing at 03:00
daily) are different layers. Most of the candidates listed in the prompt
solve exactly one of them. The comparison below makes that explicit.

## Constraints

| C | Constraint | Source |
|---|------------|--------|
| C1 | Runs **identically** on macOS and Linux. No "works on my box, fails on the server." | Hard requirement. |
| C2 | No long-running daemon a user has to install, configure, and babysit. | dear-agent is personal-dev tooling, not a platform team's product. |
| C3 | State is local-first, file-on-disk, inspectable with normal CLI tools. | ADR-010 D2; existing `runs.db` is `sqlite3`-debuggable. |
| C4 | Must have an audit trail per state transition (substrate property). | [ADR-009](../adrs/ADR-009-work-item-as-first-class-substrate.md), ADR-010 D3. |
| C5 | Must be embeddable as a Go library — workflows are authored in YAML, but the runner is `cmd/workflow-run`, not a service. | Project language policy; ADR-010 D1. |
| C6 | "Good analytics" — the operator can answer *what ran, when, with what cost, why did it fail?* without log-grep archaeology. | Stated requirement (Valentin). |
| C7 | Right-sized for personal dev automation: nightly audits, periodic signal collection, the occasional multi-step research pipeline. Not 10K-tenant enterprise. | Project scope; see [.dear-agent.yml `audits.schedule`](../../.dear-agent.yml). |

C1 (Mac + Linux parity) is the disqualifier the prompt highlights. C2–C7
are independently stringent enough that several candidates fail before
C1 even applies.

## Candidates

### 1. dear-agent built-in workflow engine

**What it is.** A Go library + thin CLIs. YAML defines a DAG of nodes
(`bash`, `ai`, `loop`, soon `spawn`). State persists in SQLite with WAL.
Every transition writes to `audit_events`. AI nodes resolve through a role
registry (`roles.yaml`), with budget metering and per-node permissions.
HITL is a first-class state, not a callback.

| Criterion | Verdict |
|---|---|
| Mac + Linux parity (C1) | ✅ Pure Go + `modernc.org/sqlite` (cgo-free). Same binary, same `runs.db`. |
| Operational burden (C2) | ✅ Zero daemons. `workflow-run` is a one-shot process; `runs.db` is a file. |
| Local-first state (C3) | ✅ Single SQLite file. `sqlite3 runs.db` is the debugger. |
| Audit trail (C4) | ✅ Every transition is a row in `audit_events`. ADR-010 §5. |
| Embeddable (C5) | ✅ `pkg/workflow` is the API; CLIs are thin shells over it. |
| Analytics (C6) | ✅ SQL is the query language. Cost, latency, failure-class, model-mix breakdowns are all `SELECT` queries. FTS5 covers free-text. |
| Right-sized (C7) | ✅ Designed for this use case. SQLite ceiling (~10 concurrent writers) is well past personal-dev demand. |
| Maintenance cost | ⚠️ It's our code. ~2700 LOC plus the in-flight Phase 4–5 work. Bug surface is ours. |
| Time-based scheduling | ❌ The engine runs DAGs; it does not fire them on a clock. Pair with cron / `at` / a tiny ticker. |

**When this is the right answer.** Anytime the unit of work is a DAG of
nodes whose state we want to inspect later. Which is most of dear-agent.

### 2. Temporal.io

**What it is.** A distributed, durable workflow orchestration platform.
Workers (your code) connect to a Temporal server, which persists workflow
history and replays it on failure to provide deterministic execution.

| Criterion | Verdict |
|---|---|
| Mac + Linux parity (C1) | ✅ Server runs on both; Go SDK is first-class. |
| Operational burden (C2) | ❌ Requires a running Temporal server (or Temporal Cloud). Even `temporal server start-dev` is a process you have to keep alive between runs, with its own port, DB, and lifecycle. |
| Local-first state (C3) | ❌ State lives in Temporal's backend (Cassandra / Postgres / SQLite-via-server). Not a file you can `cat`. |
| Audit trail (C4) | ✅ Event history is the entire model — Temporal's strongest property. |
| Embeddable (C5) | ⚠️ The SDK is a library, but the runtime is a service. You don't run "a Temporal" inline. |
| Analytics (C6) | ✅ Temporal Web + tctl. Excellent — arguably best-in-class for workflow analytics. |
| Right-sized (C7) | ❌ Built for "long-running orchestration at scale." For a nightly DAG of five nodes, the operational overhead dwarfs the work. |
| Determinism contract | ⚠️ Workflows must be deterministic for replay. AI nodes are non-deterministic. ADR-010 D10 explicitly rejects this contract. |
| Migration cost | ❌ ~2700 LOC of `pkg/workflow` plus the YAML schema, role registry, exit gates, and audit sinks would all be reframed against Temporal's primitives. Months of work. |

**Verdict.** Disqualified by C2 (daemon) and C7 (overkill), with the
determinism contract as the architectural deal-breaker. Already rejected
in ADR-010; this row documents *why* in detail.

**When Temporal would be the right answer.** A team running multi-tenant
workflows at scale, where deterministic replay is the product
requirement, and a platform team owns the cluster. Not us.

### 3. launchd (macOS only)

**What it is.** Apple's native job scheduler. Plist files in
`~/Library/LaunchAgents` describe what to run and when (calendar interval,
keep-alive, on-demand).

| Criterion | Verdict |
|---|---|
| Mac + Linux parity (C1) | ❌ macOS only. **Disqualified.** |

**Verdict.** The prompt flagged this; it's correct. Listed here for
completeness because the question recurs ("isn't there something native
on macOS?"). Yes, but it's non-portable, so it cannot be *the* answer for
dear-agent.

**Where launchd could still appear.** As a *user-side* convenience for a
Mac developer who wants their daily audit fired by their OS scheduler
rather than cron. The dear-agent engine itself must not depend on it.

### 4. cron / systemd timers

**What it is.** Two different things grouped together because they
solve the same problem: fire a command on a schedule.

- **cron** — a 50-year-old line-per-job scheduler. Available on macOS
  (BSD cron) and every Linux distro. `crontab -e` and you're done.
- **systemd timers** — Linux's modern replacement. `.timer` units pair
  with `.service` units. Better logging (journalctl), dependency graph,
  randomized delays. **Not available on macOS.**

| Criterion | cron verdict | systemd timers verdict |
|---|---|---|
| Mac + Linux parity (C1) | ✅ Both platforms have cron. Syntax is portable for the common subset. | ❌ Linux only. |
| Operational burden (C2) | ✅ Already running. Zero install. | ⚠️ Linux-already-running, but adds a Mac fallback. |
| Local-first state (C3) | N/A — cron has no state model; it just fires commands. | N/A — same. |
| Audit trail (C4) | ❌ Mail spool or a redirected log file. Not queryable. | ⚠️ journalctl is structured, but Linux-only. |
| Analytics (C6) | ❌ None. Whatever the command emits to stdout is what you get. | ⚠️ journalctl filters. |
| Time-based scheduling | ✅ This is what they do. | ✅ This is what they do. |
| DAG orchestration | ❌ Not the model. Cron fires one command. | ❌ Same. |

**Verdict.** Cron is the right answer for the *trigger* layer, paired
with the dear-agent engine for the *execution* layer. Systemd timers
fail C1 on their own but make sense as a Linux-side optimization if the
trigger layer ever needs more than cron offers.

**Concrete recommendation.** Use cron (or the user's OS-equivalent — they
can pick launchd on Mac, systemd timers on Linux) to invoke
`workflow-run <some.yaml>` at the desired cadence. The engine handles
the rest. This is the model `.dear-agent.yml > audits.schedule` already
implies: dear-agent declares *what* runs daily/weekly/monthly; the
operator's OS scheduler fires it.

### 5. Apache Airflow

**What it is.** Python-based DAG orchestrator with a web UI. Industry
standard for ETL pipelines.

| Criterion | Verdict |
|---|---|
| Mac + Linux parity (C1) | ✅ Both run Python. |
| Operational burden (C2) | ❌ Webserver + scheduler + worker(s) + a metadata DB (Postgres in any non-toy config). For one user. |
| Embeddable (C5) | ❌ Python; dear-agent is Go. Cross-language IPC for every node would dominate latency. |
| Right-sized (C7) | ❌ Built for "data engineering at a company." A one-user Airflow install is roughly 10× the binary surface of dear-agent itself. |

**Verdict.** Wrong scale, wrong language. Not a serious candidate.

### 6. Prefect

**What it is.** Modernized Python-native workflow tool, born partly as a
reaction to Airflow's operational weight. Has a hosted "Cloud" mode and a
self-hosted server.

| Criterion | Verdict |
|---|---|
| Mac + Linux parity (C1) | ✅ Both run Python. |
| Operational burden (C2) | ⚠️ Local-only mode is workable; full mode wants Prefect Cloud or a self-hosted server. |
| Embeddable (C5) | ❌ Python. Same cross-language problem as Airflow. |
| Analytics (C6) | ✅ Good UI in Prefect Cloud. |
| Right-sized (C7) | ⚠️ Smaller than Airflow but still designed for team workflows; analytics live in Prefect Cloud. |

**Verdict.** Better than Airflow on every axis except the one that
matters: it's Python, so it cannot be embedded in dear-agent's Go
binaries. The operator-facing UI doesn't compensate for the language
boundary.

### 7. Dagger

**What it is.** A programmable CI/CD engine that models pipelines as DAGs
of containerized steps. Has a Go SDK. Backed by BuildKit.

| Criterion | Verdict |
|---|---|
| Mac + Linux parity (C1) | ✅ Cross-platform. |
| Operational burden (C2) | ❌ Requires a Docker / container runtime on the host. macOS = Docker Desktop or OrbStack. That's a heavy dependency to demand of every dear-agent user. |
| Embeddable (C5) | ⚠️ Go SDK exists, but the engine is the BuildKit daemon, not a library. |
| Right-sized (C7) | ❌ Designed for CI pipelines (build, test, deploy). Our DAGs are "fetch context → run AI node → commit output," not "build a multi-stage container." |
| Audit trail (C4) | ⚠️ Has run logs and a UI, but the model is "step output," not "state transitions." |

**Verdict.** Strong fit for *containerized CI*, weak fit for our use
case. The Docker dependency alone makes it unviable as a default; the
model mismatch means we'd be working against the grain of the tool.

### 8. Other notable mentions (rejected briefly)

| Tool | Why it's not a candidate |
|---|---|
| **LangGraph** | Python-only; in-memory by default; substrate properties are user-built. Already rejected in ADR-010. |
| **Argo Workflows** | Kubernetes-native. Disqualified by C2 — we are not running a K8s cluster for personal-dev automation. |
| **Step Functions / Cloud Workflows** | AWS-/GCP-specific. Disqualified by C1 (cloud lock-in is worse than OS lock-in: it removes local-only operation entirely). |
| **n8n / Node-RED / Zapier** | UI-first low-code automation. Wrong audience; can't be authored in a PR. |
| **Make / Just / Taskfile** | These are *build tools*, not workflow engines. No state, no audit, no DAG-with-retries. Useful for build orchestration; orthogonal to this question. |
| **GitHub Actions (locally via `act`)** | YAML DAGs, decent ecosystem, but the runtime model assumes containers (`act` uses Docker) and the analytics live in github.com. Disqualified by C2 + C3. |

## Side-by-side summary

Legend: ✅ pass · ⚠️ partial · ❌ fail · — not applicable

| Candidate | C1 Mac+Linux | C2 No daemon | C3 Local-first | C4 Audit | C5 Embeddable Go | C6 Analytics | C7 Right-sized | Layer |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|---|
| **dear-agent engine** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | DAG |
| Temporal | ✅ | ❌ | ❌ | ✅ | ⚠️ | ✅ | ❌ | DAG |
| launchd | ❌ | ✅ | ✅ | ❌ | — | ❌ | ✅ | Schedule |
| cron | ✅ | ✅ | — | ❌ | — | ❌ | ✅ | Schedule |
| systemd timers | ❌ | ✅ | — | ⚠️ | — | ⚠️ | ✅ | Schedule |
| Airflow | ✅ | ❌ | ❌ | ✅ | ❌ | ✅ | ❌ | DAG |
| Prefect | ✅ | ⚠️ | ⚠️ | ✅ | ❌ | ✅ | ⚠️ | DAG |
| Dagger | ✅ | ❌ | ⚠️ | ⚠️ | ⚠️ | ⚠️ | ❌ | DAG (CI) |

The "Layer" column is the most important takeaway: launchd, cron, and
systemd timers are not in the same row of the comparison as Temporal or
Airflow. Mixing them in one ranked list invites a category error. The
real choice is **DAG engine** × **trigger layer** = two independent
picks.

## Recommendation

| Layer | Pick | Reasoning |
|---|---|---|
| **DAG orchestration** | dear-agent built-in engine | Passes every constraint. Embedded, file-state, SQL-queryable analytics, no daemon. The 2700 LOC is the cost; nothing else passes C2 and C5 without forcing us to either run a service or change languages. |
| **Time-based trigger** | cron, with the operator free to substitute (launchd on Mac, systemd timer on Linux, GitHub Actions schedule in CI) | Cron is the lowest-common-denominator portable trigger. `.dear-agent.yml > audits.schedule` already declares *what* runs at *what cadence*; cron just fires `workflow-run` against those declarations. |

In other words: **don't switch.** The built-in engine is the right pick
for the orchestration layer for the same reasons ADR-010 already
identified, and the scheduling-tool question turns out to be a separate
concern that's already solved by the OS layer.

### When to revisit

Switch *away* from the built-in engine if any of these become true:

1. **Concurrency > ~10 simultaneous writers.** SQLite WAL stops being
   comfortable beyond that. ADR-010 D2 already plans a Postgres adapter
   behind the same `State` interface; that's the first move, not "adopt
   Temporal."
2. **Multi-tenant or shared-team operation becomes a goal.** The current
   design is one-user-one-`runs.db`. ADR-010 open question §2.
3. **Deterministic replay becomes a product requirement.** Currently
   ADR-010 D10 explicitly disclaims it. If a use case forces
   determinism (compliance? regulated workloads?), Temporal becomes a
   serious candidate again.

None of these are visible on the roadmap. Recheck this doc when one of
them shows up; until then the recommendation stands.

## Decisions

| Decision | Rationale |
|---|---|
| Keep the built-in engine for DAG orchestration. | Only candidate that passes all of C1–C7 simultaneously. |
| Use cron (or its OS-equivalent) for the trigger layer. | DAG engines and time schedulers are different layers; cron is the portable common denominator. |
| Preserve the `State` interface as the future-proofing seam. | The realistic upgrade path under load is "swap SQLite for Postgres," not "swap dear-agent for Temporal." Already in ADR-010 D2. |
| Do not promise deterministic replay. | Reaffirms ADR-010 D10. Audit completeness is the substitute. |
| Do not adopt a Python-based engine (Airflow, Prefect, LangGraph). | Cross-language boundary on every node is unjustifiable for personal-dev tooling. |
| Do not adopt a container-based engine (Dagger, Argo, `act`). | Forces a Docker/K8s dependency on every user. Violates C2. |

## Open questions

1. **Should `.dear-agent.yml > audits.schedule` grow a literal cron
   expression?** Currently it declares cadence categorically (`daily`,
   `weekly`, `monthly`). The operator picks the actual minute. Promoting
   to cron syntax would be more precise but couples the config to a
   specific scheduler dialect. Recommendation: keep categorical, document
   the canonical cron lines in the operator guide.

2. **Should we ship a `workflow-cron` helper that translates schedule
   declarations into per-platform timer files (cron line, launchd plist,
   systemd unit)?** Would solve the "operator has to know three dialects"
   problem. Probably yes, post-MVS. Out of scope for this doc.

3. **Does the analytics story need a richer surface than SQL?** The
   built-in engine's "good analytics" answer today is "every question is
   a `SELECT`." That's powerful but unfriendly to non-SQL users. A
   read-only web inspector is mentioned in ADR-010 Phase 5. Worth
   prototyping when the data volume justifies it; not a reason to switch
   engines.

## References

- [ADR-010 — Workflow Engine as Substrate-Quality Work-Item Layer](../adrs/ADR-010-workflow-engine-architecture.md)
- [ADR-009 — Work Item as First-Class Substrate](../adrs/ADR-009-work-item-as-first-class-substrate.md)
- [Workflow Engine Operator Guide](../workflow-engine.md)
- [Substrate Diagnostic](substrate-diagnostic.md)
- [.dear-agent.yml — `audits.schedule`](../../.dear-agent.yml)
