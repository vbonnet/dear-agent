# AGM State Detector Specification

<!-- Last audited at: 2026-07-21 -->

## Overview

`agm/internal/state` detects interactive harness state from tmux pane output.
It recognizes Claude Code, Codex, Antigravity, and managed Pi ready prompts plus blocked,
active, overlay, waiting, looping, stuck, and unknown states.

## EARS Requirements

**AGM-STATE-01** When a Claude Code permission prompt is visible, the system shall report `blocked_permission` before considering ready-prompt patterns.

**AGM-STATE-02** When the Claude Code background tasks overlay is visible, the system shall report `background_tasks_view` and classify it as an overlay.

**AGM-STATE-03** When a Claude prompt, a complete Codex composer (the initial header with its model-change hint and empty `›` or `»` input cursor, or an empty post-turn `›` or `»` input cursor paired with the structured model footer), an Antigravity bare prompt, or the latest managed Pi ready status is visible, the system shall report `ready` with high confidence; standalone Codex model text, a working-status footer, a typed draft, an unsubmitted paste chip, and Pi working or permission status shall not report ready.

**AGM-STATE-04** When a spinner appears with agent markers, the system shall report `waiting_agent`.

**AGM-STATE-05** When a spinner appears with loop or monitoring markers, the system shall report `looping`.

**AGM-STATE-06** When only a spinner is detected, the system shall report `thinking`.

**AGM-STATE-07** When authentication or structured user-choice prompts are visible without a ready prompt, the system shall report the corresponding blocked state.

**AGM-STATE-08** When no recognizable pattern appears and the last output is older than the stuck threshold, the system shall report `stuck`.

**AGM-STATE-09** When no state pattern or stuck condition applies, the system shall report `unknown` with low confidence.

**AGM-STATE-10** When input readiness is checked, the system shall return no for permission dialogs, overlay for background-task overlays, yes for complete Claude/Codex/Antigravity ready prompts or the latest managed Pi ready status, and queue for standalone Codex model text, a working-status footer, Pi working or permission status, or any other non-ready output.

**AGM-STATE-11** When state helper methods are used, the system shall classify blocked, overlay, active, idle, and waiting states according to the exported state constants.

**AGM-STATE-12** When AGY pane history contains a feedback-survey marker, the system shall classify it as an active overlay only when no bare AGY composer appears after the final marker; a later composer shall be ready and directly sendable, while a prompt that precedes the marker shall remain blocked by the survey.

**AGM-STATE-13** When Pi pane history contains multiple managed status lines, the system shall classify the latest status and shall not let a stale ready line override a newer working or permission state.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
