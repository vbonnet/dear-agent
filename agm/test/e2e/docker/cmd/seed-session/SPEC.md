# Seed Session Fixture Command Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Living
**Scope:** `seed-session`, the reaper E2E lifecycle-record fixture.

## Overview

`seed-session` creates the AGM lifecycle record a reaper end-to-end test needs
before `agm-reaper` can act on it. The reaper resolves its target through the
lifecycle store — `ops.ArchiveSession` looks the session up by identifier, and
the reaper's archive preflight runs before the pane is touched — so a fixture
that writes only a `manifest.yaml` fails preflight with `AGM-001` and never
reaps anything.

It writes through the same storage API the reaper's own unit tests use, rather
than hand-written SQL, so the fixture cannot drift from the schema. It is a
test fixture and is deliberately excluded from `build-agm` / `install-agm`.

It deliberately does not delegate to `agm session new`: that spawns a real
harness and runs workspace detection and the spawn circuit breaker, none of
which the reaper tests are about.

## EARS Requirements

**SEED-SESSION-01** When `--session-id` or `--name` is absent, the system shall return an error without writing to the lifecycle store.

**SEED-SESSION-02** When `AGM_DB_PATH` is unset, the system shall return an error rather than writing to the production lifecycle store.

**SEED-SESSION-03** When invoked with a session id and name, the system shall create a lifecycle record readable by identifier from the store named by `AGM_DB_PATH`.

**SEED-SESSION-04** When `--tmux-session` is absent, the system shall record the session name as the tmux session name, so the reaper stops the pane it was pointed at.

**SEED-SESSION-05** When `--harness` is provided, the system shall record it, so the reaper selects the matching graceful-exit command.

**SEED-SESSION-06** When the store cannot be opened or migrated, the system shall return a wrapped error naming the database path.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Package tests: `agm/test/e2e/docker/cmd/seed-session/main_test.go`
- Consumed by: `agm/test/e2e/docker/scripts/test_reaper_*.sh`
