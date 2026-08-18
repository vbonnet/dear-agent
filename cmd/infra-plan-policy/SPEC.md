# Infra Plan Policy Command Specification

<!-- Last audited at: 2026-08-18 -->

## Overview

`cmd/infra-plan-policy` is the transport adapter around
`internal/infraattest`. CI holds the private material — the encrypted saved
plan, the plaintext plan JSON streamed out of it, the pinned OpenTofu and
provider binaries, the lock files, and the five evidence documents — and this
command is the only thing allowed to look at all of it and speak about it in
public.

It exposes four subcommands. `authorize` turns private evidence into canonical
public authorization claims on stdout. `verify-authorization` re-checks those
claims against the bindings the caller currently believes. `receipt` turns
post-apply observations into canonical receipt claims. `verify-receipt`
re-checks a receipt the same way.

The command owns no policy of its own. Every classification decision lives in
`internal/infraattest`; this layer is responsible for the properties the
library cannot enforce from behind an `io.Reader`: that all inputs are present,
that the plan-JSON reader is a stream rather than a file left on disk, that
timestamps and nonces are canonically encoded, that the HMAC key is wiped after
use, and that nothing but a rejection code ever reaches stderr.

The plan-JSON input is required to be a non-regular file — a pipe or process
substitution — because a regular file would mean the decrypted plaintext plan
had been written to the filesystem, which is exactly what the encrypted-plan
requirement exists to prevent.

Failure is uniform and quiet. Every rejection exits 3 with a single stable code
from the library's catalog, so a caller cannot distinguish "your plan creates a
resource" from "your provider hash is wrong" by inspecting output, and a log
scrape cannot recover private evidence from an error message.

## EARS Requirements

**IPP-01** When invoked with no arguments, an unrecognised subcommand, or a failure to parse the subcommand's flags, the command shall print usage and exit 2.

**IPP-02** When invoked with `help`, `-h`, or `--help`, the command shall print usage to stdout and exit 0.

**IPP-03** If any required flag for the selected subcommand is empty, or any positional argument is supplied, the command shall reject the invocation as `invalid-input` and exit 3.

**IPP-04** When `authorize` runs, the command shall open the encrypted plan, plan JSON, OpenTofu binary, provider binary, dependency lockfile, toolchain manifest, all five evidence documents, and the baseline receipt, and shall fail the whole invocation if any one of them cannot be opened.

**IPP-05** If the `--plan-json` input resolves to a regular file, the command shall reject the invocation rather than evaluate it, because a regular file means the decrypted plan was persisted to disk.

**IPP-06** When file inputs are opened, the command shall close every descriptor it opened, including on the rejection path.

**IPP-07** When a private context document is loaded, the command shall canonicalise it through the same bounded unique-key parser the library uses, and shall reject duplicated keys, trailing bytes, unknown fields, and documents larger than the context byte bound.

**IPP-08** When a timestamp is parsed from a context document, the command shall require RFC 3339 in UTC with zero nanoseconds, and shall reject any other encoding.

**IPP-09** When a nonce is parsed from a context document, the command shall require unpadded base64url that decodes to exactly the required nonce length and re-encodes to the identical string, so a non-canonical encoding of a valid nonce is rejected.

**IPP-10** When the HMAC key is loaded, the command shall require unpadded base64url decoding to at least the library's minimum key length, and shall zero both the encoded and decoded key material before returning.

**IPP-11** When `authorize` succeeds, the command shall write the canonical authorization claims to stdout with no additional framing, and shall exit 0.

**IPP-12** When `verify-authorization` succeeds, the command shall write exactly `{"status":"authorized"}` to stdout and exit 0.

**IPP-13** When `receipt` succeeds, the command shall write the canonical receipt claims to stdout with no additional framing, and shall exit 0.

**IPP-14** When `verify-receipt` succeeds, the command shall write exactly `{"status":"reconciled"}` to stdout and exit 0.

**IPP-15** When claims are read for verification, the command shall bound the read to the library's maximum claims size and reject anything larger.

**IPP-16** If any evaluation fails, the command shall print a single stable rejection code from the library's public-safe catalog to stderr and exit 3.

**IPP-17** The command shall not print plan, inventory, backend, state, migration, provider, key, or lineage content to stdout, to stderr, or to any log.

**IPP-18** When the current time is used for freshness evaluation, the command shall canonicalise it to whole UTC seconds before passing it to the library, so a sub-second host clock cannot produce a non-canonical freshness window.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Governing contract: `infra/SPEC.md` (`INFRA-ATTEST-01` .. `INFRA-ATTEST-21`)
- Library specification: `internal/infraattest/SPEC.md`
- Package tests: `cmd/infra-plan-policy/*_test.go`
