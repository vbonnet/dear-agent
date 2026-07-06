# dear-deploy Command Specification

<!-- Last audited at: 2026-07-06 -->

## Purpose

`cmd/dear-deploy` is the CLI over the `deploy` package. It lists deployable host
artifacts, reports deployed state versus the manifest, and reconciles drift.
`build-install` is the crash-safe replacement for the post-merge hook's
`go install`: it builds a Go binary and atomically installs it only when the
built revision is proven to be on `origin/main`.

## EARS Requirements

**DD-01** When invoked with no subcommand, the system shall print usage and exit non-zero.

**DD-02** When invoked with an unknown subcommand, the system shall report it and print usage.

**DD-03** When `status` finds a deployed artifact that has drifted or a required artifact that is missing, the system shall exit with code 2.

**DD-04** When `build-install` is invoked without a `--pkg` argument, the system shall report the missing flag and exit non-zero.

**DD-05** Where `--target` is omitted, the system shall default the install path to `~/go/bin/<pkg basename>`.

**DD-06** Where `--repo-root` is omitted, the system shall resolve the repo root from the git toplevel of the working directory.

**DD-07** When `build-install` completes, the system shall report the installed target and the revision it was gated against.

**DD-08** When `build-install` fails any gate, the system shall report the failure on stderr and exit non-zero.
