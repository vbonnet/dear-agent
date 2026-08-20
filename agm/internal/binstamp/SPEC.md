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

## Requirements

### Stamping

**BINSTAMP-01** When asked to stamp a path, the system shall record the file's size, modification time, and inode.

**BINSTAMP-02** When asked to stamp the running executable, the system shall resolve the path through the operating system rather than from an argument, so the stamp cannot be pointed at a different file than the one executing.

**BINSTAMP-03** Where a stamp cannot be taken because the file is absent or unreadable, the system shall report the failure rather than returning a zero stamp that would compare equal to another failure.

### Replacement detection

**BINSTAMP-04** When the file at the watched path differs from the baseline in size, modification time, or inode, the system shall report the executable as replaced.

**BINSTAMP-05** Where an install replaces the file by writing a temporary file and renaming it over the target, the system shall report the executable as replaced, because the path continues to resolve and only the inode changes.

**BINSTAMP-06** Where an install rewrites the file in place without changing its size, the system shall report the executable as replaced, because the inode is unchanged and only the modification time moves.

**BINSTAMP-07** While the watched path cannot be stated, the system shall report the executable as not replaced, because an install is not atomic from the observer's side and exiting during that instant would restart the daemon onto a binary that has not yet been written.

**BINSTAMP-08** Where the baseline could not be taken when the watcher was created, the system shall report the executable as not replaced, so an unstampable binary leaves the daemon running rather than restarting it on every poll.

### Daemon integration

**BINSTAMP-09** When a watching daemon observes that its executable was replaced, the system shall exit cleanly so its supervisor restarts it on the new build.

**BINSTAMP-10** Where the daemon exits because its executable was replaced, the system shall emit a `binary_replaced` event naming the binary, so an operator reading the log can tell a redeploy restart from a crash restart.

**BINSTAMP-11** While a watching daemon is running, the system shall check for replacement before performing each cycle's work, so a redeploy takes effect on the next cycle rather than after another full round of superseded behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature`

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
