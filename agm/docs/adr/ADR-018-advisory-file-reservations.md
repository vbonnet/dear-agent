# ADR-018: Advisory (Not Enforced) File Reservations

**Status:** Accepted
**Date:** 2026-03-24
**Context:** Parallel agent safety during swarm operations

## Problem

When multiple agents work in parallel (swarm mode), they can destructively interfere by editing the same files simultaneously. This leads to merge conflicts, lost changes, or corrupted state. Agents need a way to declare intent to edit files so others can avoid conflicts.

## Decision

Implement an advisory file reservation system that provides visibility into which agent is editing which files, without hard enforcement:

1. **Reserve**: `Reserve(sessionID, patterns, ttl)` declares intent to edit files matching glob patterns
2. **Check**: `Check(sessionID, filePath)` reports whether another agent holds a conflicting reservation
3. **Release**: `Release(sessionID)` frees all reservations for a session
4. **Auto-expire**: Reservations have a configurable TTL (default 2 hours) and are cleaned up automatically

### Key design choices:

1. **Advisory, not enforced**: Agents can proceed despite conflicts. The system provides information, not locks. This avoids deadlocks and keeps the system simple.
2. **JSON file storage**: Single file at `~/.agm/reservations.json` with atomic writes. No database dependency.
3. **Glob pattern matching**: Agents reserve patterns (e.g., `pkg/dag/*.go`) rather than individual files, reducing reservation churn.
4. **TTL-based expiry**: Prevents abandoned reservations from blocking other agents indefinitely. Default 2 hours covers most task durations.

## Alternatives Considered

1. **Hard file locking (flock/lockfile)**: Rejected -- deadlock risk with AI agents that don't reliably release locks; agents can't reason about lock contention
2. **Git branch-per-agent**: Rejected -- merge overhead grows with agent count; doesn't prevent concurrent edits to the same logical file
3. **Database-backed locks (Dolt)**: Rejected -- adds Dolt dependency for a simple coordination primitive; overkill for advisory semantics
4. **No coordination**: Rejected -- observed real conflicts during swarm operations; even advisory warnings significantly reduce wasted work

## Consequences

- Agents must opt-in to checking reservations (no enforcement means non-participating agents can still cause conflicts)
- Single JSON file may become a bottleneck with many agents (acceptable for current swarm sizes of 3-8 agents)
- TTL-based expiry means stale reservations may briefly block checks after an agent crashes (2-hour window)
- Cross-harness compatible: any process with filesystem access can participate via the `agm` CLI
