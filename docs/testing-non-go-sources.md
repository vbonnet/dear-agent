# Testing non-Go sources

Go code in this repository is covered by `go test`, `golangci-lint`, and the
`make preflight*` targets. Shell, jq, and OpenTofu sources historically were
not. This document is the canonical description of the gate for each, so a
change adding one of those file types has a pattern to follow instead of
inventing a new one.

The shared principle: the decision logic belongs in a tested Go command, and
the workflow only supplies inputs. A gate whose verdict lives in an untestable
`run:` block cannot itself be reviewed.

## Shell

Two tools, two different jobs.

**`shellcheck`** is the static gate, in
[`.github/workflows/shell-lint.yml`](../.github/workflows/shell-lint.yml).

It is scoped to changed lines, not changed files and not the whole repository.
The repository carries 105 lower-severity ShellCheck findings across 104
tracked scripts; a repository-wide gate above `-S error` could never be turned
on, and a whole-file gate would fail a one-line edit to a legacy script for
findings its author did not write. This is the same rule `.golangci.yml`
already applies to Go through `new-from-merge-base: origin/main`.

The scoping decision is made by [`cmd/shellcheck-diff`](../cmd/shellcheck-diff),
a unit-tested Go command that takes a `git diff -U0` patch and a ShellCheck
JSON1 document and reports only the findings on lines the change introduced. It
fails closed: an unreadable or empty findings document is an error, never a
"clean" result.

Three levels run:

| Job | Scope | Severity | Blocking |
|---|---|---|---|
| `changed-scripts` | lines the change touched | `style` (everything) | yes |
| `baseline` | every tracked `*.sh` | `error` | yes |
| `nightly-sweep` | every tracked `*.sh` | `style` | no, reports only |

To silence a finding, use ShellCheck's own `# shellcheck disable=SCxxxx`
directive next to the line. It lands in the diff and is reviewable, unlike a
side-channel waiver.

**`bats`** is the behavioral gate, in
[`.github/workflows/shell-matrix.yml`](../.github/workflows/shell-matrix.yml),
which already runs every `tests/bats/*.bats` file across bash 4, bash 5, zsh,
dash, and ash. A new script gets a `tests/bats/<name>.bats` file next to the
existing ones; no workflow change is needed.

Both gates sit behind the 20-line limit in
[`.github/workflows/language-policy.yml`](../.github/workflows/language-policy.yml).
A script that wants to grow past it should move its logic into a Go command
instead of taking a waiver.

## OpenTofu and Terraform

[`.github/workflows/terraform-lint.yml`](../.github/workflows/terraform-lint.yml)
runs three credential-free gates over `infra/`:

- `tofu fmt -check -recursive -diff`
- `tofu init -backend=false` then `tofu validate`
- `tflint --recursive`, configured by [`infra/.tflint.hcl`](../infra/.tflint.hcl)
  and pinned to an explicit version so new rules arrive in a reviewable commit

None of them contacts the state backend or the GitHub API, so the workflow is
safe on a pull request from any source. Planning needs provider credentials and
the private repository inventory, so it stays in the workflow that owns that
boundary.

`fmt` and `validate` are also reachable locally through
[`infra/terraform_test.go`](../infra/terraform_test.go), so `make test-affected`
catches a regression before CI does. That file is the pattern for any future
OpenTofu root: shell out to the real tool, skip cleanly when it is absent, and
assert on its exit status rather than reimplementing its rules.

Terratest is not used. Its value is assertions over a real plan or apply, which
needs the credentials and private inventory this public repository deliberately
does not carry. Assertions over a saved plan belong with the fixture that
supplies an inventory. Checkov is likewise not used: its Terraform policies
target cloud resources, and this root manages only GitHub repositories and
rulesets, so it would add an unpinned dependency for no coverage.

## jq

The jq programs under `.github/` encode policy decisions: which ruleset
documents are well formed, which required checks are missing, which repository
inventories are safe to act on. They run inside workflows and inside
`infra/import.sh`, where a silent behavior change surfaces as a mis-audited
ruleset rather than as a test failure.

[`tests/jq`](../tests/jq) is the gate. It is a Go test rather than a shell
runner, so it needs no new CI wiring, adds no shell under the 20-line policy,
and turns a malformed fixture into a loud failure instead of a skipped case.
[`.github/workflows/jq-lint.yml`](../.github/workflows/jq-lint.yml) runs it on
changes to the programs and records the jq version the fixtures were replayed
against.

A case is a directory under `tests/jq/testdata/<suite>/<name>/`:

| File | Purpose |
|---|---|
| `case.json` | which program to run, its `--arg`/`--argjson` values, `-L` include paths, and a required `description` |
| `input.json` | the document piped to jq |
| `expected.json` | expected output, compared as JSON so key order does not matter |
| `expected.txt` | expected output, compared as raw text (for `-r` programs) |
| `expected-error.txt` | a substring jq's stderr must contain |

Exactly one `expected-*` file is present. The runner discovers suites by
walking the tree, so adding a program means adding a directory, not editing Go.

Two properties make the gate hard to hollow out:

- Every checked-in `.jq` file must have at least one case. A new program with
  no fixture fails the run.
- `case.json` must name a program that exists and carry a non-empty
  `description`. A typo would otherwise exempt a real file from the coverage
  check while its own case still passed.

A library of bare `def`s cannot be run with `jq -f`, since it has no final
expression. Such a case sets `filter` to an inline expression that `include`s
the library, and still names the library in `program` so it counts for
coverage.
