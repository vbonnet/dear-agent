# Tmux Manager Backend Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/manager/tmuxbackend`.

## Overview

`tmuxbackend` adapts the existing AGM tmux runtime to the `manager.Backend`
interface. It is the interactive session backend for terminal harnesses, so it
must preserve attach support, terminal-scraping state detection, send-key
delivery, interrupt safety, and tmux health checks behind the manager contract.

## EARS Requirements

**TMUXBACKEND-01** When the tmux backend reports its name, the system shall return `tmux`.

**TMUXBACKEND-02** When the tmux backend reports capabilities, the system shall advertise attach and interrupt support without structured I/O support.

**TMUXBACKEND-03** When creating a session without a name, the system shall reject the request.

**TMUXBACKEND-04** When creating a session without a working directory, the system shall use the current directory.

**TMUXBACKEND-05** When listing sessions, the system shall adapt tmux session metadata into manager session metadata.

**TMUXBACKEND-06** When a name filter is provided while listing sessions, the system shall return only matching sessions.

**TMUXBACKEND-07** When a list limit is provided, the system shall return no more than that number of sessions.

**TMUXBACKEND-08** When fetching a session that tmux does not contain, the system shall return a not-found error.

**TMUXBACKEND-09** When sending a message, the system shall deliver the message through tmux send-command semantics.

**TMUXBACKEND-10** When reading output with a non-positive line count, the system shall default to thirty captured lines.

**TMUXBACKEND-11** When interrupting a session, the system shall verify capture-pane succeeds before sending Ctrl-C.

**TMUXBACKEND-12** When tmux state detection cannot capture pane output, the system shall return an idle state with reduced confidence.

**TMUXBACKEND-13** When delivery readiness is checked for a missing session, the system shall return not-found delivery status without error.

**TMUXBACKEND-14** When tmux is unavailable or unresponsive, the system shall fail the health check.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/internal/manager/tmuxbackend`
