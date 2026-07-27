# Deploy Package Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-06 -->

## Purpose

Package `deploy` is the write side of host-artifact management. It compares each
declared artifact against its deployed copy and reconciles the difference. File
artifacts (launchd plists, compiled hooks) are deployed by byte content with an
atomic stage-verify-activate write. Binary artifacts are status-only: their
deployed copy is compared by the `vcs.revision` embedded at build time.

`AtomicInstall` is the crash-safe, stale-proof way to (re)install a Go binary.
It replaces `go install`, whose in-place overwrite of a running Mach-O made
macOS SIGKILL every live instance with "Code Signature Invalid" (the 2026-07-06
crash loop) and which silently reinstalled stale code when the build env was
broken.

## EARS Requirements

**DEP-01** When a file artifact's deployed copy differs from its source, the system shall stage the new content, verify it, and activate it with an atomic replace.

**DEP-02** When any deploy write fails, the system shall leave the previously installed artifact untouched.

**DEP-03** When `AtomicInstall` cannot resolve the intended source ref in the repo, the system shall fail loud and install nothing.

**DEP-04** When `AtomicInstall` builds a binary, the system shall write it to a temporary file in the target's own directory so the final replace is an atomic rename on the same filesystem.

**DEP-05** When a build invoked by `AtomicInstall` fails, the system shall return an error and leave the live binary untouched.

**DEP-06** When a freshly built binary carries no `vcs.revision` stamp, the system shall refuse to install it.

**DEP-07** When a freshly built binary's revision is neither the source ref nor an ancestor of it, the system shall refuse to install it.

**DEP-08** Where the ancestry gate passes, the system shall install the binary by renaming the temporary file over the target.

**DEP-09** When any `AtomicInstall` step fails before the rename, the system shall remove the temporary file it created.

**DEP-10** When a clean linked-worktree build succeeds without a `vcs.revision`, the system shall rebuild in a temporary standalone clone and apply the same provenance gate before installation. The fallback clone and its detached checkout shall use the finite clone-fallback deadline.

**DEP-11** When `AtomicInstall` invokes `go build`, the system shall use the finite ten-minute cold-build deadline and leave the live binary untouched if that deadline expires.
