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

**DEP-02** When a deploy write fails before an artifact is activated, the system shall leave the previously installed artifact untouched.

**DEP-03** When `AtomicInstall` cannot resolve the intended source ref in the repo, the system shall fail loud and install nothing.

**DEP-04** When `AtomicInstall` builds a binary, the system shall write it to a temporary file in the target's own directory so the final replace is an atomic rename on the same filesystem.

**DEP-05** When a build invoked by `AtomicInstall` fails, the system shall return an error and leave the live binary untouched.

**DEP-06** When a freshly built binary carries no `vcs.revision` stamp, the system shall refuse to install it.

**DEP-07** When a freshly built binary's revision is neither the source ref nor an ancestor of it, the system shall refuse to install it.

**DEP-08** Where the ancestry gate passes, the system shall install the binary by renaming the temporary file over the target.

**DEP-09** When any `AtomicInstall` step fails before the rename, the system shall remove the temporary file it created.

**DEP-10** When a clean linked-worktree build succeeds without a `vcs.revision`, the system shall rebuild in a temporary standalone clone and apply the same provenance gate before installation. The fallback clone and its detached checkout shall use the finite clone-fallback deadline.

**DEP-11** When `AtomicInstall` invokes `go build`, the system shall use the finite ten-minute cold-build deadline and leave the live binary untouched if that deadline expires.

**DEP-12** When an operator installs a root-owned hook helper, the system shall require exact artifact and fixed-bootstrap digest confirmation, reject a passwordless probe, and stage, verify, and atomically activate the helper beneath verified root-owned non-writable non-symlink ancestry in one non-caching privileged command.

**DEP-13** When publication of required-pulse state is interrupted or partially fails, the system shall preserve any required pulse already activated and shall restore one consistent state before applying a later merge, or fail without discarding operator state.

**DEP-14** When status or dry-run cannot observe one stable required-pulse state, the system shall fail without writing and shall not report the deployment clean.

**DEP-15** When concurrent deployments publish required pulses to an initially absent registry, the system shall retain every required pulse activated by either completed deployment, and a later sync shall still install any required pulse that neither deployment activated.

**DEP-16** When required-pulse state is already current, the system shall leave that required-pulse state unchanged during a repeated sync.

**DEP-17** When the pulse registry is a valid leaf symlink, the system shall preserve the symlink and its target mode while merging; when that symlink is dangling, the system shall fail without modifying the link, its target, or required-pulse history.

**DEP-18** When deployment cannot establish that the selected recovery-job registry and projected live pulse registry are runtime-loadable and that every declared job pulse is validly defined with current required-pulse state, the system shall fail without activating the recovery-job registry.

**DEP-19** When an absent-only recovery-job registry exists on the host, the system shall derive required pulses from that live operator-owned registry and validate them against the projected live pulse registry rather than repository defaults alone.

**DEP-20** When sync or install emits JSON, the system shall preserve the top-level array of artifact results used by existing automation.

**DEP-21** When a required-pulse merge rewrites an existing registry, the system shall preserve the exact values of fields in operator-owned pulse entries.

**DEP-22** When a pulse artifact is not declared absent-only, the system shall validate and atomically deploy its exact rendered source instead of applying additive pulse-merge semantics.

**DEP-23** When the explicit pulse-merge command selects a pulse artifact that is not declared absent-only, the system shall fail without modifying the deployed artifact.

**DEP-24** When status or dry-run reports nested pulse state, the system shall keep every result name equal to a manifest-declared artifact selector and shall carry pulse-level context separately.

**DEP-26** When a pulse registry is selected without its normal recovery-job registry, the system shall derive required pulses from the deployed job registry, fall back to rendered source only when that deployed registry does not exist, and fail without changing deployed pulse bytes when the live registry cannot be observed or its requirements cannot be satisfied.

**DEP-27** When an existing pulse registry needs only first-time offer-ledger adoption, the system shall report pulse-artifact work without writing during status and dry-run, report an update during a real sync, and report the state current during later previews and syncs.

**DEP-28** When required-pulse transaction state is renamed into place or its pending marker is removed, the system shall sync the directory that owns that namespace update before advancing to the next transaction phase or reporting success.

**DEP-29** When normal pulse and recovery-job registries are selected together, the system shall validate prospective pulse bytes against the exact rendered prospective jobs and every runtime-loadable deployed job before either registry changes, permit those valid prospective snapshots to repair a readable deployed job registry rejected by the runtime parser, publish the same rendered job bytes used in validation, and keep every runtime-loadable deployed-job pulse present if job publication later fails.

**DEP-30** When a mutating selection includes a normal pulse registry or its recovery-job registry, the system shall hold the persistent logical pulse-registry lock from before observing live registry state through every selected publication attempt.

**DEP-31** When `install-recovery-loop-launchagent` preserves an existing recovery-job registry or would seed a missing one, the system shall complete the locked required-pulse merge from the live registry, or its rendered source when absent, before publishing a missing job registry or reporting staging success.

**DEP-32** When a selected normal pulse registry is optional, its source is absent, and no recovery-job registry is selected, the system shall preserve skipped-artifact semantics without creating publication sidecars after resolving the applicable existing-job requirements; when a recovery-job registry is also selected, the system shall refuse its publication without the pulse source.
