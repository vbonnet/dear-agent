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

There is no jq gate yet. When the first `.jq` program lands, it gets a fixture
directory of input and expected-output cases run from CI, and this section
gains its description.
