# Spike: DEAR Retro Generation Investigation (ce-h9q)

**Status:** Complete
**Date:** 2026-06-21
**Bead:** ce-h9q (P1, FLY)
**Author:** research worker ce-h9q

> **Note on path:** the bead template asked for
> `engram-research/spikes/dear-retro-gen-investigation.md`, but
> `engram-research/` is a separate sibling repo and is **not** tracked inside
> `dear-agent`. This spike is filed at the repo's established convention,
> `docs/spike-*.md`, instead. The template also referenced
> `engram-research/retrospectives/` and a `2026-06-17-vroom-overnight-pr-merge-stall-dear-retro.md`
> baseline — neither exists in this repo (see §3).

---

## TL;DR

The 2026-06-11 root causes are **resolved**, but **retros still aren't being
generated**. The real gap is *downstream* of OTel:

1. **OTel/Jaeger is running end-to-end** — spans land on disk as expected.
2. **agm-bus was restarted** (Jun 13, 8-day uptime) — the 2026-06-11 "not
   restarted" finding no longer holds.
3. **But the consumer that turns spans into retros (`cmd/retro-audit`) is an
   orphan** — never built, never scheduled, invoked by nothing. `RETRO_LOG.md`
   has never been created.
4. **DEAR narrative retros are a *manual* practice that lapsed** — newest retro
   is `2026-06-08`, none in the ~13 days since.
5. **The `stop-retrospect` hook is built but not wired** into the Stop hook.

So "DEAR retros not generated" has **two independent causes**: an automated
pipeline that was authored but never connected, and a manual cadence that
stopped. Fixing one does not fix the other.

---

## 1. Current state of each pipeline component

### 1a. Telemetry collection — ✅ WORKING

| Component | State | Evidence |
| --- | --- | --- |
| `CLAUDE_CODE_ENABLE_TELEMETRY` | ✅ `1` | env |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | ✅ `http://localhost:4317` | env |
| `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA` | ✅ `1` | env |
| Jaeger collector | ✅ running | PID 1203, launchd `com.jaegertracing.jaeger`; ports 4317/4318/16686 all `LISTEN` |
| Span sink (`pkg/otelsetup`) | ✅ writing | `~/.engram/traces/<session>/spans.jsonl`, fresh data dated today (2026-06-21) from `service_name` `agm`, `token-refresher`, … |
| `agm-bus` | ✅ running, **restarted** | PID 1198, `agm-bus serve`, started **Sat Jun 13 14:20** (8-day uptime) |
| `vroom-orchestrator` | ✅ running | PID 42223, `agm watch-stalled --orchestrator vroom-orchestrator` |

The span on-disk format matches what the consumer expects (`trace_id`,
`span_id`, `name`, `start_time`, `duration_ms`, `status_code`,
`status_message` — extra fields like `attributes`/`kind`/`end_time` are
ignored on unmarshal). The traces directory layout
(`~/.engram/traces/<session-uuid>/spans.jsonl`) exactly matches the consumer's
glob `filepath.Join(tracesDir, "*", "spans.jsonl")`.

**Minor / non-blocking:** `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_METRICS_EXPORTER`,
and `OTEL_LOGS_EXPORTER` are empty. Claude Code defaults to gRPC for a `:4317`
endpoint, and `retro-audit` only consumes trace spans, so this does not block
retro generation. Worth setting `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` explicitly
for clarity.

### 1b. Automated span-audit retro (`cmd/retro-audit`) — ❌ ORPHANED

`cmd/retro-audit/main.go` reads `~/.engram/traces/*/spans.jsonl`, classifies
findings (status=`Error` spans → errors; `duration_ms > 5000` → slow), and
**appends** a markdown report to `docs/retros/RETRO_LOG.md`
(`appendToFile` does `MkdirAll` + `O_CREATE`, so it self-creates the file).

It would work if run — but:

- **Not built:** no `bin/retro-audit`, not on `PATH`.
- **No Makefile target** referencing it.
- **Not scheduled:** no launchd plist, no crontab entry, no agm-bus subscriber.
- **Invoked by nothing:** the only repo references to `retro-audit` are its own
  source/test files. (`safe-merge` and `vroom-dispatch` mention `otel-local`,
  not `retro-audit`.)
- **`docs/retros/RETRO_LOG.md` does not exist** → definitive proof the command
  has never run in this checkout.

Provenance: added in `32c57c16d6 feat(otel): ce-1xi minimal audit job + PR
applier (#294)` as a "minimal audit job" — authored, never wired up.

### 1c. DEAR narrative retros (`docs/retros/`) — ❌ CADENCE LAPSED

Per `docs/process-bead-retro-pairing.md`, DEAR retros are **agent/supervisor
authored** markdown docs PR'd into `docs/retros/` via `safe-pr --emergency`,
each paired with a `dear-retro`-labelled bead. They are *not* auto-generated;
they depend on a supervisor noticing an incident and writing one.

- Newest DEAR retro in the repo: **`2026-06-08`** (committed `2026-06-09`).
- **None since** — ~13 days as of 2026-06-21.
- Docs the process file cites as existing
  (`2026-06-15-dogfooding-audit-retro.md`, `2026-06-20-ci-cascade.md`) and the
  bead's baseline (`2026-06-17-vroom-overnight-pr-merge-stall-dear-retro.md`)
  **do not exist** in the repo.

### 1d. `stop-retrospect` hook — ⚠️ BUILT BUT NOT WIRED

`engram/hooks/cmd/stop-retrospect` mines conversation history for undone work /
broken promises / unretried tool errors. It is **advisory only** (it `Warn`s;
it does not write a retro file). More importantly, it is **not wired into the
Stop hook** — `.claude/settings.json` runs `stop-guardrail-feedback` on both
`Stop` and `SubagentStop`, never `stop-retrospect`. So even the end-of-session
signal that might *prompt* a human/supervisor retro never fires.

### 1e. `agm retro analyze` — ℹ️ CONSUMER, NOT GENERATOR

`agm retro analyze` (`agm/cmd/agm/retro.go` → `agm/internal/ops/retro_analyze.go`)
is a **static** analyzer: it parses an *existing* retro markdown file across
four lenses (root-cause / recurrence / remediation / systemic-vs-oneoff). It
**cannot generate** retros — it needs input files the upstream pipeline isn't
producing. Not part of the generation gap, but worth noting it is starved of
input by the gaps above.

---

## 2. What is broken / missing / unconfigured

| # | Gap | Severity | Effect |
| --- | --- | --- | --- |
| G1 | `retro-audit` not built / not scheduled / not invoked | **High** | No automated span→retro output; `RETRO_LOG.md` never created |
| G2 | DEAR narrative-retro cadence stopped after 2026-06-08 | **High** | No human/supervisor retros for ~13 days |
| G3 | `stop-retrospect` hook not wired into Stop hook | Medium | End-of-session "undone work" signal never fires |
| G4 | `.claude/settings.json` has unresolved merge-conflict markers (invalid JSON, `UU` status) | Medium | Breaks project-level hook loading for interactive in-repo sessions (headless runs use `--setting-sources=user`, so unaffected — but still a repo-health bug) |
| G5 | `OTEL_EXPORTER_OTLP_PROTOCOL` unset | Low | Relies on default; harmless for traces |

**Reconciliation with 2026-06-11:** that investigation blamed "OTel not running
end-to-end + agm-bus not restarted." Both are now **false** — OTel writes spans
to disk and agm-bus has run since Jun 13. The 2026-06-11 fix addressed the
*collection* layer; nobody wired the *consumption* layer (G1) or restored the
*manual* layer (G2). That is why retros are still absent.

> ⚠️ **G4 detail:** `~/src/dear-agent/.claude/settings.json` currently shows
> conflict markers `<<<<<<< Updated upstream` (L31), `=======` (L43),
> `>>>>>>> Stashed changes` (L57) and fails `json.load`. This is in the golden
> checkout's working tree (`git status` = `UU`). It should be resolved
> independently of retro work.

---

## 3. Concrete fix recommendations

### Fix G1 — wire `retro-audit` into a schedule (highest leverage)

1. **Build it.** Add a build target to the `Makefile` and produce `bin/retro-audit`.
2. **Schedule it.** Add a launchd plist mirroring the existing
   `com.dear-agent.*` agents (e.g. `com.dear-agent.retro-audit.plist`) running
   daily:
   ```sh
   retro-audit --lookback 24h --output docs/retros/RETRO_LOG.md
   ```
   Alternatively (or additionally) invoke it from a **post-merge hook** or as an
   **agm-bus subscriber** so it runs on real activity rather than a fixed clock.
3. **Seed `docs/retros/RETRO_LOG.md`** with a header so intent is documented
   (the appender will create it regardless, but a seeded file signals the
   pipeline is live).

### Fix G2 — restore the DEAR narrative-retro cadence

DEAR retros are a manual mandate, not pipeline output; `retro-audit` complements
but does not replace them. There is already a `daily-ops-audit` scheduled agent
(seen in the session's allow-listed `Documents/Claude/Scheduled/daily-ops-audit`
path). Recommend it (or a dedicated scheduled agent) **author a DEAR retro when
it detects an incident**, following `docs/process-bead-retro-pairing.md`
(bead first → `docs/retros/<date>-<slug>.md` → link PR). This closes the
"retros stopped after 06-08" gap.

### Fix G3 — wire `stop-retrospect`

Add `stop-retrospect` to the `Stop` (and/or `SessionEnd`) hook array in
`.claude/settings.json` alongside `stop-guardrail-feedback`, so undone-work
signals surface at session end and can trigger a retro.

### Fix G4 — resolve the settings.json conflict

Resolve the merge-conflict markers in `~/src/dear-agent/.claude/settings.json`
and re-validate as JSON (`python3 -c 'import json,sys; json.load(open(sys.argv[1]))'`).
Until then, project-level hooks won't load for interactive sessions in the repo.

### Fix G5 — minor env hardening

Set `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` explicitly (and decide whether
metrics/logs exporters are wanted) for clarity and future-proofing.

---

## 4. Verification commands (re-runnable)

```sh
# Collection layer (should all be green)
echo "$CLAUDE_CODE_ENABLE_TELEMETRY $OTEL_EXPORTER_OTLP_ENDPOINT"
lsof -iTCP -sTCP:LISTEN -P -n | grep -E ':4317|:4318|:16686'
ls ~/.engram/traces/*/spans.jsonl | head            # spans on disk
launchctl list | grep -E 'agm-bus|jaeger'           # collectors up

# Consumption layer (the gap)
ls docs/retros/RETRO_LOG.md                          # MISSING → retro-audit never ran
grep -rl retro-audit --include='*.plist' ~/Library/LaunchAgents  # empty → unscheduled
ls -t docs/retros/ | grep '^20' | tail -1            # newest DEAR retro = 2026-06-08
```

---

## 5. Summary

| Layer | Component | State |
| --- | --- | --- |
| Collection | OTel env, Jaeger, span sink, agm-bus, vroom | ✅ working (2026-06-11 causes resolved) |
| Consumption (auto) | `cmd/retro-audit` → `RETRO_LOG.md` | ❌ orphaned — never built/scheduled/invoked |
| Consumption (manual) | DEAR narrative retros in `docs/retros/` | ❌ lapsed since 2026-06-08 |
| Signal | `stop-retrospect` hook | ⚠️ built, not wired |
| Analysis | `agm retro analyze` | ℹ️ works, but starved of input |
| Config health | `.claude/settings.json` | ⚠️ unresolved merge conflict (invalid JSON) |

**Bottom line:** the telemetry plumbing is healthy; the retro *generation* step
was authored (`retro-audit`) but never connected, and the manual DEAR cadence
stopped. Wiring G1 + restoring G2 are the two changes that actually resume retro
generation.
