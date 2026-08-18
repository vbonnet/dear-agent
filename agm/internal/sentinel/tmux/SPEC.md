# Sentinel Tmux Inspection Specification

<!-- Last audited at: 2026-07-21 -->

## Overview

`agm/internal/sentinel/tmux` captures tmux pane state and classifies permission,
completion, idle, waiting, and stuck indicators for sentinel recovery.

## Requirements

**STM-01** When a named-session operation resolves its tmux socket, the system shall use the configured client timeout and return probe failures with per-socket context instead of collapsing execution failures into an unqualified missing-session result.

**STM-02** When pane content contains a permission prompt, the system shall classify the prompt before weaker idle or completion indicators.

**STM-03** When pane content contains a pending user question, the system shall exempt that pane from generic stuck classification.

**STM-04** When completion, idle, waiting, or stuck patterns overlap, the system shall apply the maintained specificity and precedence rules.

**STM-05** When pane information is captured, the system shall return the recent content, last command, and derived status indicators for the requested session.

**STM-06** When `CI_SKIP_TMUX=true`, the test suite shall skip real sentinel tmux integration tests without suppressing pure sentinel classification, client-configuration, or injected command-routing tests.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/sentinel/tmux/*_test.go`
