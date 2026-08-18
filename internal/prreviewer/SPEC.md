# PR Reviewer Package Specification

<!-- Last audited at: 2026-08-17 -->

## Overview

`internal/prreviewer` detects pull requests whose head commit has not yet been
reviewed, asks a required primary provider and a best-effort secondary provider
for a review, and posts the combined result through the GitHub reviews API bound
to the inspected commit. Reviewed head SHAs are persisted so a repeated pass
never re-reviews an unchanged pull request. Every external process runs behind the `Runner` boundary so the review
pass is testable without GitHub or provider access.

## Contract Ownership

This SPEC is the single owner of the observable review-pass contract: detection,
provider dispatch, posting, and persistence. `cmd/external-pr-reviewer/SPEC.md`
owns only the operator-facing command surface — flag parsing, exit codes, and
loop control — and must not restate the requirements below. The split mirrors
`cmd/mergeloop` and `internal/mergeloop`.

## EARS Requirements

**PRV-01** When a review pass is configured without a repository, the system shall reject the configuration rather than inspecting any pull request.

**PRV-02** When a review pass is configured with an unsupported review event, the system shall reject the configuration before dispatching a provider.

**PRV-03** When optional configuration is omitted, the system shall apply the documented defaults for inspection limit, review event, provider commands, secondary attempts, provider timeout, state path, and clock.

**PRV-04** When pull requests are listed, the system shall exclude drafts in the GitHub query so the inspection limit is spent on reviewable pull requests, and shall skip any draft that still reaches the review path.

**PRV-05** When the recorded state already holds the inspected pull request head SHA, the system shall skip the pull request and report the already-reviewed skip reason.

**PRV-06** When the primary provider fails or returns an empty review, the system shall return a contextual error and shall not post a review.

**PRV-07** When the secondary provider fails, the system shall wait the configured retry delay, retry up to the configured attempt count, and then continue with a primary-only review body.

**PRV-08** When the secondary provider is unavailable, the system shall record the attempt count in the posted review body, shall withhold the provider error text from that body, and shall report the failure detail only on the operator output stream.

**PRV-09** When dry-run mode is enabled, the system shall report the review it would post and shall neither post a review nor persist state.

**PRV-10** When the pull request head or base revision no longer matches the inspected revisions, the system shall skip posting so a review is never attached to a patch it did not read.

**PRV-11** When a review is posted successfully, the system shall record the reviewed head SHA so a later pass skips the unchanged pull request.

**PRV-12** When a pull request or repository fails after an earlier review was posted, the system shall persist the already-posted results before returning the error.

**PRV-13** When a provider is invoked, the system shall bound the invocation with the configured provider timeout.

**PRV-14** When the state file does not exist, the system shall treat the recorded state as empty rather than failing the pass.

**PRV-15** When state is persisted, the system shall create the state directory with owner-only permissions and replace the state file atomically with owner-only permissions.

**PRV-16** When a review body is composed, the system shall include the repository, pull request number, review timestamp, and one section per provider.

**PRV-17** When a provider failure reports denied access or failed authentication, the system shall stop retrying that provider for the pull request.

**PRV-18** When a review provider is invoked through the isolated runner, the system shall remove the GitHub credential variables from its environment and run it outside the operator's checkout.

**PRV-19** When a review prompt is built, the system shall declare the pull request title and diff to be untrusted data that the provider must not act on.

**PRV-20** When a non-dry-run pass starts, the system shall claim an exclusive state lock, shall refuse to run while another pass holds it, and shall reclaim a lock older than the configured staleness window.

**PRV-21** When a review is submitted, the system shall bind it to the inspected commit SHA and shall send the review body outside the process argument vector.

**PRV-22** When one repository or pull request fails, the system shall continue with the remaining independent targets and shall report the collected failures once the pass ends.

**PRV-23** When an external command is terminated by pass cancellation, the system shall report the cancellation alongside the command failure.

**PRV-24** When a provider command declares the prompt placeholder, the system shall substitute the prompt into its argument vector instead of sending it on standard input.

**PRV-25** When a state lock is held, the system shall refresh its timestamp for the life of the pass so a long pass is not treated as stale.

**PRV-26** When the secondary provider is unavailable, the system shall report the number of attempts actually made rather than the configured maximum.

**PRV-27** When pull requests are listed, the system shall order the bounded query by most recent update so a moved pull request is not pinned outside the page.

**PRV-28** When a diff exceeds the configured prompt bound, the system shall truncate it and record where the truncation happened.

**PRV-29** When dry-run mode reports the review it would post, the system shall first apply the same head confirmation the posting path applies.

**PRV-30** When the reviewed diff was truncated and the configured event is an approval, the system shall downgrade the event to a comment and shall record why the approval was withheld.

**PRV-31** When a reviewed identity is recorded, the system shall include the base revision so a retargeted pull request is reviewed again, and shall key state by a case-insensitive repository name.

**PRV-32** When a review is posted, the system shall persist the reviewed identity before inspecting another target.

**PRV-33** When an external command produces output, the system shall bound what it buffers and shall record that the output was truncated.

## Residual Risk

`IsolatedRunner` removes the credential variables listed in the implementation
and moves the provider out of the checkout, but a review provider still runs as
the operator with the operator's filesystem access and its own provider
credentials. A provider that obeys injected instructions could therefore still
read operator-readable files. Closing that gap requires an OS-level sandbox for
provider execution, which is out of scope for this contract and is not claimed
by any requirement above.

A provider command that declares the prompt placeholder receives the pull
request title and bounded diff in its argument vector, where a process listing
on the same host can read them. That is a property of providers whose only
one-shot interface takes the prompt as an argument; commands that read standard
input keep the prompt off the command line.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `internal/prreviewer/*_test.go`
