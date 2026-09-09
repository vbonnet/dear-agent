---
phase: RESEARCH
phase_name: RESEARCH
wayfinder_session_id: feedback-loop-event-architecture
created_at: 2026-09-02T23:33:00-07:00
---

# Research

Two research tracks, run manually following the discipline of this repo's own
`research-pipeline/skills/research-pipeline/SKILL.md` (not invoked as a packaged skill,
since it was not in this session's enabled-skill list). Both ran on Haiku 4.5, the
practical stand-in for Gemini 3.8 Flash: that model is real, per this repo's own
in-flight pricing work on `feat/gemini-3.8-flash-and-fable-5.1`, but this session had
no Gemini API access at all.

- **Internal codebase grounding**: read `pkg/eventbus`, `pkg/trigger`, `pkg/vroom/vroom`,
  `pkg/vroom/decisiontrail`, `pkg/vroom/escalation`, `internal/mergeloop`,
  `wayfinder/internal/analytics`, `pkg/absencealarm` (unmerged worktree),
  `cmd/agm-webhook-receiver`, `agm/internal/bus`, `agm/workflowbus`, `pkg/workflow`,
  and the relevant ADRs (002, 010, 029, 030, 031, 032), citing exact file:line for
  every claim.
- **External prior art**: C4 model and Mermaid C4 diagram syntax, Factorio Logistics
  Train Network mechanics (provider/requester stations, dispatcher matching, network
  starvation), pipes-and-filters architecture, and real-world precedent for
  trigger-to-benchmark-to-conditional-merge pipelines (Argo Events, CML, Flagger,
  Dependabot).

Findings and citations are folded into
[docs/architecture/feedback-loop-pipelines.md](docs/architecture/feedback-loop-pipelines.md)
directly rather than kept as a separate research artifact, since every claim there
carries its own citation.
