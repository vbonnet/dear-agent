---
name: research-pipeline
description: >
  Orchestrates the staged pipeline for turning an external source (a talk,
  video, article, paper) into verified, executable work — provider-routed
  ingestion, goal-oriented research, independent cross-model verification and
  planning, decomposition into sized beads, and dark-factory execution. Use
  when the user wants to ingest a source (especially a YouTube video or talk)
  and turn it into a concrete plan or beads for this codebase, wants a second
  model to fact-check research before acting on it, or says things like
  "research this and turn it into a plan", "verify this with an independent
  model and plan the application", or "decompose this into beads for Codex".
  Do NOT use for a single-shot research question with no downstream codebase
  action, or for grabbing a transcript with no further processing.
---

# Research Pipeline

This regular-file entrypoint supplies the shared AGENTS-compatible repository
discovery used by Codex and AGY.

## Workflow

1. Before taking any pipeline action, read
   `../../../research-pipeline/skills/research-pipeline/SKILL.md` completely.
2. Follow that canonical workflow without substituting this discovery
   entrypoint for any of its requirements or gates.

## Verification

Apply the canonical skill's verification and completion criteria. Do not claim
the pipeline is complete until every required canonical exit condition passes.
