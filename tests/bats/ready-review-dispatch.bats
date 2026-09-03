#!/usr/bin/env bats
#
# Contract: a draft->ready transition must dispatch an agentic review that can
# actually publish what it found, and must leave a revision-bound marker a
# merge gate can require.
#
# These are workflow-source assertions because the observable behaviour lives
# in GitHub's dispatcher and in the Actions token's scopes, neither of which
# the repository test harness can execute. Each assertion below corresponds to
# a defect measured on real run history:
#
#   * `Claude Code Review` held `pull-requests: read`, so the reviewer ran,
#     spent budget and concluded `success` having posted nothing. Across
#     PRs #1396-#1433 no Claude identity authored a single comment or review.
#   * `PR Review Agent` required a `full-ci` label that no pull request has
#     ever carried, so 59 of its last 60 pull_request runs were `skipped` —
#     and `ready_for_review` is its only pull_request trigger.
#   * Nothing recorded that a review had run for a given head SHA, so no gate
#     could require one.

bats_require_minimum_version 1.7.0

# A bare `!` does not fail a Bats test (SC2314): the negation is discarded and
# the case passes regardless. Negative assertions therefore go through this
# helper, which returns non-zero on a match.
refute_scope() {
  if printf '%s\n' "$1" | grep -qF "$2"; then
    printf 'unexpected %s still present:\n%s\n' "$2" "$1" >&2
    return 1
  fi
}

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  OAUTH_REVIEW="${REPO_ROOT}/.github/workflows/claude-code-review.yml"
  HEALTH_REVIEW="${REPO_ROOT}/.github/workflows/pr-review-agent.yml"
}

@test "OAuth reviewer dispatches on the draft->ready transition" {
  grep -qF 'types: [opened, synchronize, ready_for_review, reopened]' "${OAUTH_REVIEW}"
}

@test "OAuth reviewer can publish its findings" {
  # The whole point of the job. Read-only scopes make it a silent no-op.
  # Comment lines are stripped: this block explains the read-only defect it
  # replaced, and the explanation must not satisfy the assertion.
  permissions="$(sed -n '/^    permissions:/,/^$/p' "${OAUTH_REVIEW}" | grep -v '^ *#')"
  echo "${permissions}" | grep -qF 'pull-requests: write'
  echo "${permissions}" | grep -qF 'issues: write'
  refute_scope "${permissions}" 'pull-requests: read'
  refute_scope "${permissions}" 'issues: read'
}

@test "OAuth reviewer still refuses fork pull requests" {
  # Write scopes are only defensible while untrusted heads cannot reach them.
  grep -qF 'github.event.pull_request.head.repo.full_name == github.repository' "${OAUTH_REVIEW}"
}

@test "review status is published against the reviewed head SHA" {
  step="$(sed -n '/- name: Publish agentic-review status/,$p' "${OAUTH_REVIEW}")"
  echo "${step}" | grep -qF "name='Agentic Review Posted'"
  # shellcheck disable=SC2016  # matching the workflow's literal text, not expanding
  echo "${step}" | grep -qF 'head_sha="$HEAD_SHA"'
  # A absent context reads as pending to branch protection, so a failed or
  # cancelled review must still publish a conclusion.
  echo "${step}" | grep -qF 'if: always()'
  echo "${step}" | grep -qF 'conclusion=failure'
  grep -qF 'checks: write' "${OAUTH_REVIEW}"
}

@test "review status refuses a malformed head SHA" {
  step="$(sed -n '/- name: Publish agentic-review status/,$p' "${OAUTH_REVIEW}")"
  echo "${step}" | grep -qF '^[0-9a-f]{40}$'
}

@test "codebase-health review runs on ready without an opt-in label" {
  grep -qF 'types: [ready_for_review]' "${HEALTH_REVIEW}"
  condition="$(sed -n '/^  review:/,/^    runs-on:/p' "${HEALTH_REVIEW}" | grep -v '^ *#')"
  refute_scope "${condition}" "'full-ci'"
  echo "${condition}" | grep -qF 'github.event.pull_request.head.repo.full_name == github.repository'
}
