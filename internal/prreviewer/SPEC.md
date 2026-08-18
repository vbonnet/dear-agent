# PR Reviewer Package Specification

<!-- Last audited at: 2026-08-17 -->

## Overview

`internal/prreviewer` detects pull requests whose head commit has not yet been
reviewed, asks a required primary provider and a best-effort secondary provider
for a review, and posts the combined result through `gh pr review`. Reviewed
head SHAs are persisted so a repeated pass never re-reviews an unchanged pull
request. Every external process runs behind the `Runner` boundary so the review
pass is testable without GitHub or provider access.

## EARS Requirements

**PRV-01** When a review pass is configured without a repository, the system shall reject the configuration rather than inspecting any pull request.

**PRV-02** When a review pass is configured with an unsupported review event, the system shall reject the configuration before dispatching a provider.

**PRV-03** When optional configuration is omitted, the system shall apply the documented defaults for inspection limit, review event, provider commands, secondary attempts, provider timeout, state path, and clock.

**PRV-04** When an inspected pull request is a draft, the system shall skip it and report the draft skip reason.

**PRV-05** When the recorded state already holds the inspected pull request head SHA, the system shall skip the pull request and report the already-reviewed skip reason.

**PRV-06** When the primary provider fails or returns an empty review, the system shall return a contextual error and shall not post a review.

**PRV-07** When the secondary provider fails, the system shall wait the configured retry delay, retry up to the configured attempt count, and then continue with a primary-only review body.

**PRV-08** When the secondary provider is unavailable, the system shall record the attempt count in the posted review body, shall withhold the provider error text from that body, and shall report the failure detail only on the operator output stream.

**PRV-09** When dry-run mode is enabled, the system shall report the review it would post and shall neither post a review nor persist state.

**PRV-10** When the pull request head no longer matches the inspected head SHA, the system shall skip posting so a review is never attached to a revision it did not read.

**PRV-11** When a review is posted successfully, the system shall record the reviewed head SHA so a later pass skips the unchanged pull request.

**PRV-12** When a pull request or repository fails after an earlier review was posted, the system shall persist the already-posted results before returning the error.

**PRV-13** When a provider is invoked, the system shall bound the invocation with the configured provider timeout.

**PRV-14** When the state file does not exist, the system shall treat the recorded state as empty rather than failing the pass.

**PRV-15** When state is persisted, the system shall create the state directory with owner-only permissions and replace the state file atomically with owner-only permissions.

**PRV-16** When a review body is composed, the system shall include the repository, pull request number, review timestamp, and one section per provider.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `internal/prreviewer/*_test.go`
