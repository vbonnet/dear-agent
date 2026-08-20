# Binary Stamp Specification

<!-- Last audited at: 2026-08-20 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/binstamp` — how a long-lived AGM daemon notices that its own executable was reinstalled

## Overview

`agm/internal/binstamp` answers one question on demand: is the executable
this process is running still the one on disk?

A `KeepAlive` LaunchAgent restarts a daemon that crashes. It does not
restart one whose binary was merely reinstalled, because nothing crashed:
the running process holds the old inode and keeps serving the old
behavior for as long as it lives. The failure this package prevents is
therefore silent by construction. `main` carries a fix, `~/go/bin/agm`
carries the fix, every audit that inspects code or binaries reports the
fix as deployed, and the daemon actually handling events does not have
it.

That is not hypothetical. `agm watch-stalled` started on 2026-08-11
15:34 and was still serving that build on 2026-08-20, across the merges
of #1212, #1224 and #1226 — every one of them a fix to the completion
notification path it was running. Completions kept failing with the
pre-fix error the whole time.

Consumers: `agm/cmd/agm/watch_stalled.go`.

## EARS Requirements

### Stamping

- WHEN asked to stamp a path, the system SHALL record the file's size, modification time, and inode.
- WHEN asked to stamp the running executable, the system SHALL resolve the path via the operating system rather than from an argument, so the stamp cannot be pointed at a different file than the one executing.
- WHERE a stamp cannot be taken because the file is absent or unreadable, the system SHALL report the failure rather than returning a zero stamp that would compare equal to another failure.

### Replacement detection

- WHEN the file at the watched path differs from the baseline in size, modification time, or inode, the system SHALL report the executable as replaced.
- WHERE an install replaces the file by writing a temporary file and renaming it over the target, the system SHALL report the executable as replaced, because the path continues to resolve and only the inode changes.
- WHERE an install rewrites the file in place without changing its size, the system SHALL report the executable as replaced, because the inode is unchanged and only the modification time moves.
- WHILE the watched path cannot be stated, the system SHALL report the executable as not replaced, because an install is not atomic from the observer's side and exiting during that instant would restart the daemon onto a binary that has not yet been written.
- WHERE the baseline could not be taken when the watcher was created, the system SHALL always report the executable as not replaced, so an unstampable binary leaves the daemon running rather than restarting it on every poll.

### Daemon integration

- WHEN a watching daemon observes that its executable was replaced, the system SHALL exit cleanly so its supervisor restarts it on the new build.
- WHERE the daemon exits because its executable was replaced, the system SHALL emit a `binary_replaced` event naming the binary, so an operator reading the log can tell a redeploy restart from a crash restart.
- WHILE a watching daemon is running, the system SHALL check for replacement before performing each cycle's work, so a redeploy takes effect on the next cycle rather than after another full round of superseded behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature` (changed-package SPEC coverage gate)

## Non-goals

- **Verifying what the new build contains.** This package reports that the
  bytes changed, not that they changed for the better. A bad deploy
  restarts the daemon just as a good one does.
- **Performing the restart.** Exiting is the whole action; bringing the
  process back is the supervisor's job (`KeepAlive` under launchd), whose
  throttle is what keeps a persistently failing binary from hot-looping.
- **Watching anything but the executable.** Config reloads are a separate
  concern with different semantics: a changed config does not require a
  new process image.
