# tmux Monitor Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/monitor/tmux` captures tmux pane content and session metadata for
AGM monitoring surfaces. It provides small, typed wrappers around tmux commands
so higher-level harness monitors can distinguish missing sessions, unavailable
tmux servers, and permission failures.

## EARS Requirements

**TMON-01** When pane content is requested, the system shall reject an empty session name before invoking tmux.

**TMON-02** When tmux is unavailable, the system shall return the tmux-not-running error instead of shelling out to capture panes.

**TMON-03** When a target session or pane cannot be found, the system shall return the session-not-found error.

**TMON-04** When pane history is requested with a line limit, the system shall pass the corresponding scrollback bound to `tmux capture-pane`.

**TMON-05** When session information is requested, the system shall parse tmux session name, window count, creation time, and attached state from formatted session output.

**TMON-06** When executing any tmux subprocess command, the system shall use a timeout-bounded context to prevent the process from hanging indefinitely.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
