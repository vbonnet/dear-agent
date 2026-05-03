# dear-agent

> An autonomous software development agent with lifecycle intelligence.

# Goal

## Product Vision

An autonomous software development agent with lifecycle intelligence.

Two entry points: greenfield ("build me X") and brownfield ("improve this existing codebase"). Three modes: active development, ROI plateau detection, and maintenance. The product runs on a heartbeat, improving codebases toward goals, asking for clarification when needed.

## Vision

AI Tools aims to be the state-of-the-art meta-harness for AI coding agents —
a unified management layer that sits above individual AI CLIs and provides
the session lifecycle, isolation, orchestration, and memory that
production-grade AI-assisted development requires.

The core insight: AI coding agents are powerful but ephemeral. They lack
persistent state, workspace isolation, and coordination primitives. AI Tools
fills that gap without replacing the agents themselves.

## Principles

### Configurable with sensible defaults

Every behavior should be configurable, but the out-of-box experience should
work well. Configuration cascades from CLI flags → environment variables →
config file → smart defaults, so users only configure what they need to change.

### Staff+ engineering quality bar

Code and documentation must meet a Staff+ software engineering standard.
This means: clear abstractions, comprehensive test isolation, structured error
handling with actionable suggestions, and architecture decisions recorded in
ADRs. No shortcuts that create maintenance burden.

### Security first

Agents operate in sandboxed, copy-on-write filesystems by default. Advisory
file reservations prevent destructive concurrent edits. Test infrastructure is
blocked from touching production workspaces at the infrastructure level, not
by convention. Secrets and PII are never stored in session metadata.

### Harness-agnostic by design

AGM treats AI CLIs as interchangeable backends behind the adapter pattern.
No product is privileged over another. Adding a new harness means implementing
one interface — no changes to core logic.

### Observe, don't interfere

AGM monitors agent state (READY, THINKING, COMPACTING, etc.) through
non-invasive detection: hooks, pane inspection, event streams. It delivers
messages only when agents are idle. The system provides visibility and
coordination, not control.

### Be the substrate, not the wrapper

Agents need durable state outside the context window: records with stable IDs,
explicit ownership, a state machine, structural verbs, and queryable history.
The systems that already provide those properties — issue trackers, source
control, CRMs, calendars — are the substrate that agents end up running on,
whether or not they were built for AI.

dear-agent is built to be that substrate for AI coding work, and to integrate
cleanly with the substrates that already exist for everything else. Every
component should be able to answer five diagnostic questions: does it have
records (not just content)? a state machine (not just labels)? explicit
ownership (not inferred from conversation)? structural verbs (not
conversational)? queryable history (not just visible)? See
[research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md](research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md)
and [docs/design/substrate-diagnostic.md](docs/design/substrate-diagnostic.md).

## What We're Building

- **Session lifecycle management** — Create, resume, archive, and monitor
  AI agent sessions with persistent metadata and state tracking.
- **Sandbox isolation** — Copy-on-write filesystem environments so agents
  work safely without affecting the host or each other.
- **Multi-agent orchestration** — Async message delivery, state-aware routing,
  and advisory file reservations for parallel agent workflows.
- **Persistent memory** — Cue-based retrieval so AI sessions retain context
  across conversations and sessions.
- **Structured workflows** — Phased SDLC lifecycle with validation gates,
  so complex work follows a repeatable process.
- **Unified API surface** — CLI, MCP server, and Skills all share the same
  operations layer, ensuring consistent behavior.

## What We're NOT Building

- **A replacement for AI CLIs** — AGM manages sessions, it doesn't replace
  Claude Code, Gemini CLI, or any other agent. The agents do the work.
- **A prompt engineering framework** — We don't optimize prompts or manage
  prompt libraries. That's the agent's domain.
- **A cloud platform** — AI Tools runs locally. There is no hosted service,
  no account management, no billing.
- **An LLM inference layer** — We don't proxy API calls or manage API keys
  for the underlying models. The harness handles that.
- **A general-purpose task runner** — AGM orchestrates AI agent sessions,
  not arbitrary shell scripts or CI pipelines.

## Harness Parity Roadmap

dear-agent must achieve full feature parity across all supported harnesses (Claude Code, Gemini CLI, Codex CLI, OpenCode) running all 3 model families (Anthropic, Google, OpenAI).

### Incremental Phases
1. **Simple query + basic agm-assoc/exit** — Any harness can start a session, associate with AGM, and exit cleanly
2. **Research tasks** — Any harness can perform research via AGM sessions.
   First slice landed: `deep-research youtube <url>` runs a YouTube research
   pipeline (transcript + structured extraction + engram-research diary + optional
   multi-provider deep research). The structured-extraction step can be driven
   by an AGM-spawned `gemini-cli` session via `--use-agm`, with automatic
   fallback to a direct `gemini -p` call when AGM is unavailable. See
   [research/cmd/deep-research/youtube/](research/cmd/deep-research/youtube/)
   and the `youtube` subcommand in
   [research/cmd/deep-research/cmd/youtube.go](research/cmd/deep-research/cmd/youtube.go).
3. **Code review + Wayfinder** — Any harness can run Wayfinder phases and code review workflows
4. **Orchestrator in any harness** — The VROOM mesh can run in any harness
5. **Full parity** — All skills, hooks, plugins work identically across all harnesses

### Implementation
- AGENTS.md is the cross-harness convergence point (all harnesses read it)
- Thin per-harness wrappers: CLAUDE.md, GEMINI.md (with @import AGENTS.md)
- Hook abstraction layer for harness-specific event formats
- Skill adaptation for harness-specific tool names

## dear-agent CLI UX Design

### Core Commands

```
dear-agent target ~/src/repos/myapp --goal "Add authentication"
dear-agent status                    # What's being worked on
dear-agent ask "Should I use JWT?"   # Human answers agent question
dear-agent diary                     # View dear-diary session log
dear-agent stop                      # Graceful shutdown
dear-agent resume                    # Resume from last state
```

### Greenfield

```
dear-agent new ~/src/repos/newapp --goal "Build REST API for todo app"
```

Create a new project from scratch. dear-agent scaffolds the repo, sets up the
initial structure, and begins active development toward the stated goal.

### Brownfield

```
dear-agent target ~/src/repos/existing --goal "Improve test coverage to 80%"
```

Point dear-agent at an existing codebase. It analyzes the current state,
plans incremental improvements, and works toward the goal while respecting
existing architecture and conventions.

### Lifecycle Modes

- **Active** — dear-agent is actively building or improving code toward the
  stated goal. This is the default mode after `new` or `target`.
- **Plateau** — dear-agent detects diminishing returns (e.g., test coverage
  asymptoting, refactoring yield dropping). It pauses and asks the human for
  a new direction or goal refinement.
- **Maintenance** — dear-agent watches for dependency updates, security
  patches, and drift from quality baselines. It applies fixes autonomously
  within configured guardrails and escalates anything beyond them.

### Configuration

- `.dear-agent.yaml` in repo root — per-project goals, constraints, review
  gates, and lifecycle thresholds.
- `~/.config/dear-agent/config.yaml` — global settings, API keys, model
  preferences, and default behaviors.

## Bead Analysis Pipeline

Beads capture session outcomes. Analyze them: what tasks succeed/fail, what prompts work best, where is time wasted. Pipeline: collect → aggregate → analyze → propose improvements → test → deploy. This IS recursive self-improvement.

## Session Cost Tracking

Every session tracks: token count, API calls, wall clock time, commits produced, cost-per-commit ratio. Waste detection: 0-commit sessions = 100% waste. High token + few commits = low efficiency.

## Automated Verification

Worker outputs checked against acceptance criteria by a verifier agent. Steps: check git log for commits → if 0 = false completion → if commits exist check criteria → run tests → report pass/fail.

## Build Optimization

Shared Go caches (GOCACHE, GOMODCACHE). GOMAXPROCS=2 per worker. nice -n 19 for builds. Break up large jobs (specific packages, not ./...). Small jobs get priority.

## Event-Driven Heartbeat

Replace polling loops with event-driven wake. Events: new A2A message, worker state change, human directive file, cron job, resource alert. Fallback: 5-minute heartbeat. Benefits: zero waste when idle, instant response when work arrives.

## Single Orchestration Layer

Current 3 layers (meta-orch → orchestrator → worker) collapse to 1 smart orchestrator at current scale. Merged responsibilities: strategic direction + tactical coordination + monitoring. Split back when 10+ concurrent workers or multi-harness operation needed.

## Benchmark-Driven Self-Improvement

Set up SWE-bench-lite and TerminalBench-2.0-lite. Karpathy autoresearch loop: benchmark → find inefficiencies → fix → re-benchmark. This IS the self-improvement flywheel measured externally.

## Automated Verification — Implementation Plan

### Verifier Agent Design
Lightweight Sonnet session launched after each worker completion:
- Input: worker name, branch, acceptance criteria from prompt
- Check 1: git log on branch — any new commits? If 0 → FALSE COMPLETION
- Check 2: acceptance criteria grep — expected patterns present?
- Check 3: go test (if code changed)
- Output: PASS/FAIL with evidence

### Integration with Orchestrator
Orchestrator scan loop adds verification step:
1. Worker reports DONE
2. Launch verifier (Sonnet, disposable, 5-min TTL)
3. Verifier checks criteria
4. If PASS → merge branch to main
5. If FAIL → relaunch worker with failure context

## Refactor-First Philosophy

### Prep Phase in Wayfinder
Before implementing any feature, run a Prep phase:
- Clean up the area you're about to work in
- Refactor for clarity before adding complexity
- Ensure tests exist for the code you'll modify
- This is the "leave it better than you found it" principle

### Hygiene Sentinel
A dedicated lightweight check (astrocyte-level, not LLM) that runs before each implementation:
- Are there TODO comments in the affected files?
- Is test coverage above minimum for the package?
- Are imports clean? Dead code present?
- If hygiene score is below threshold: clean first, implement second

### Water-Carrier Capacity
Reserve 15-20% of work capacity for maintenance tasks:
- Dependency updates
- Stale branch cleanup
- Documentation freshness checks
- Test flakiness investigation
- This prevents debt accumulation during feature sprints

## Wayfinder Prep Phase

Before implementing any feature, run a preparation phase:
1. Read and understand the affected code
2. Clean up: fix TODOs, remove dead code, improve naming
3. Add missing tests for code you'll modify
4. Refactor for clarity before adding complexity
5. Only THEN start the implementation

This embeds the refactor-first philosophy into the structured workflow. The prep phase prevents debt accumulation by requiring cleanup before new work.

## Commit Discipline: Structural vs Behavioral

Never mix structural changes (renames, moves, refactors) with behavioral changes (new features, bug fixes) in the same commit.

### Why:
- Structural commits are easy to review (no logic changes)
- Behavioral commits are meaningful (logic changes only)
- Mixed commits hide bugs in noise
- Reverts are clean when commits are focused

### Rules:
1. Refactor in a separate commit BEFORE the feature commit
2. Each commit does ONE thing
3. Commit message prefix: "refactor:" for structural, "feat:"/"fix:" for behavioral
4. If a commit needs both, split it into two commits

## Karpathy Principles for Agent Development

Four principles from Andrej Karpathy's approach to AI-assisted development:

1. **Think before coding** — Research and plan before implementation. The Wayfinder Prep phase embodies this: understand the problem space before touching code.

2. **Simplicity first** — Prefer the simplest solution that works. Complexity is a cost, not a feature. Three lines of clear code > one clever abstraction.

3. **Surgical changes** — Make the smallest change that solves the problem. Large diffs are hard to review, hard to revert, and hide bugs.

4. **Goal-driven development** — Every change must connect to a stated goal. If you can't explain why a change moves toward the goal, don't make it.

## Water-Carrier Capacity (15-20%)

Reserve 15-20% of total work capacity for maintenance tasks that prevent debt accumulation:
- Dependency updates (monthly)
- Stale branch cleanup (weekly)
- Documentation freshness audit (weekly)
- Test flakiness investigation (on occurrence)
- Session GC / sandbox cleanup (daily)
- Hook health verification (daily)

This is NOT optional overhead — it's preventive maintenance. Skipping it causes compound debt that costs 3-5x more to fix later.

## Auto-Compaction for Supervisor Loops

Supervisor sessions (orchestrator, meta-orchestrator, overseer) must auto-compact before context exhaustion:
- Monitor context usage every 10th cycle
- When context > 80% full: trigger /compact with preservation instructions
- Preservation template: identity, active workers, pending work, critical rules
- Post-compaction: verify state recovered, restart loop
- This prevents the "silent death" where a supervisor fills context and stops responding

## Shift-Left Testing

Tests are written BEFORE or DURING implementation, not after:
- Worker prompts include acceptance criteria with test commands
- Test files created alongside source files (same commit)
- No feature commit without corresponding test commit
- Integration tests for every failure mode we've experienced
- Test ratio target: 1:3 (test lines : code lines minimum)

This is a DEAR Definition: "untested code is undefined behavior."
