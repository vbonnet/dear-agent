# infra/

OpenTofu Infrastructure-as-Code for GitHub repositories under `vbonnet/` and
the `dear-labs` organization. Manages security defaults, branch protection, and
org-level rulesets.

> **Supersedes the standalone IaC repository** — that repository was the
> initial attempt; this directory is the canonical home going forward. Archive
> the superseded repository only after completing the independently verified saved-plan
> workflow here.

## Layout

```
infra/
├── providers.tf              # Two GitHub providers: personal + dear-labs org
├── variables.tf              # personal_owner, org_name (token via env only)
├── locals.tf                 # Canonical dear-agent ruleset projection + safety checks
├── managed_repos.tf          # for_each instantiation of ./modules/managed-repo (active repos)
├── repos.tf                  # github_repository.archived (frozen, ignore_changes=all)
├── moved.tf                  # state migration: inline resources → module (ce-32o5)
├── dear_labs.tf              # github_organization_ruleset (dear-labs org)
├── modules/
│   └── managed-repo/         # Reusable standard fleet policy (repo + dependabot + ruleset)
├── backend.tf                # Remote state backend (Cloudflare R2, S3-compatible)
├── import.sh                 # Import existing state before first apply
├── terraform.tfvars.example  # Copy → terraform.tfvars to override defaults
└── .gitignore                # .terraform/, *.tfstate, credentials
```

The active vbonnet/* fleet — `github_repository`, Dependabot security updates,
and the branch-protection ruleset — is encapsulated in the **`modules/managed-repo`**
module and instantiated once via `for_each` over `var.active_repos` in
`managed_repos.tf`. Fleet-wide policy changes live in the module; the per-repo
inventory is supplied privately through `var.active_repos` and
`var.archived_repos`. The module is provider-agnostic, so the same
policy can be applied to dear-labs org repos via the `github.dearlabs` provider
alias once they exist (see `modules/managed-repo/README.md`).

## Authentication

The GitHub token is supplied **only** through `GITHUB_TOKEN`. It is never a
Terraform variable, never in `*.tfvars`, never in state.

```bash
export GITHUB_TOKEN="$(gh auth token)"
```

The token must have:
- `repo` scope — to manage `vbonnet/*` repositories and branch protection
- `admin:org` scope — to create/manage `dear-labs` org rulesets

## Plan encryption

The checked-in OpenTofu configuration refuses to create a plan unless a plan
encryption method is configured. For an interactive plan that is not saved,
create a fresh process-local method before running `tofu plan` or `tofu apply`:

```bash
plan_passphrase="$(openssl rand -hex 32)"
export TF_ENCRYPTION="$(printf '%s\n' \
  'key_provider "pbkdf2" "interactive" {' \
  "  passphrase = \"${plan_passphrase}\"" \
  '}' \
  'method "aes_gcm" "interactive" {' \
  '  keys = key_provider.pbkdf2.interactive' \
  '}' \
  'plan {' \
  '  method = method.aes_gcm.interactive' \
  '}')"
unset plan_passphrase
```

Do not print or persist `TF_ENCRYPTION`. A saved `-out` plan must retain the
exact method long enough for authorized decryption and must never upload its
plaintext `tofu show -json` representation. The automated production path owns
that transport and key lifetime; ad hoc workflows must not invent a stable key
from a run ID, commit, branch, or other public value.

## Remote state

State is **not** local. It lives in a Cloudflare R2 bucket via OpenTofu's
S3-compatible backend (`backend.tf`), so losing the working machine no longer
means re-importing every resource. Locking uses OpenTofu's native state lock
(`use_lockfile = true`, OpenTofu ≥ 1.10) — a `<key>.tflock` object written with
a conditional PUT — so there is **no DynamoDB table** to run.

> Why R2 and not S3? This account already runs on Cloudflare (`wrangler` is the
> installed cloud CLI; there is no AWS account). R2 is S3-compatible, so the
> stock `s3` backend works with a custom endpoint and a few `skip_*` flags.

### One-time bucket provisioning (human, run once)

```bash
# Create the state bucket (idempotent; safe to re-run).
wrangler r2 bucket create vbonnet-tofu-state

# Create an R2 API token (Cloudflare dashboard → R2 → Manage API Tokens,
# "Object Read & Write", scoped to this bucket). Note the Access Key ID/Secret.
```

### Backend config

The account-specific `bucket` and R2 endpoint are kept out of git via a
**partial backend config**. Copy the example and fill it in:

```bash
cd infra/
cp backend.hcl.example backend.hcl     # gitignored; edit ACCOUNT_ID + bucket
```

`backend.hcl` needs your Cloudflare account ID (`wrangler whoami`). Credentials
come from the environment, never from a file:

```bash
export AWS_ACCESS_KEY_ID="<R2 token Access Key ID>"
export AWS_SECRET_ACCESS_KEY="<R2 token Secret Access Key>"
```

### Migrating existing local state → R2

If you already have a local `terraform.tfstate` from an earlier apply, push it
to the remote backend once. Before any production import, plan, or apply,
provision the complete gitignored `repos.auto.tfvars` (or both inventory
`TF_VAR_*` values), `backend.hcl`, the real managed OAuth value, the R2
credentials, and a GitHub token with both `repo` and `admin:org` scopes. Stop
before state mutation if any prerequisite is unavailable.

```bash
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID=...  AWS_SECRET_ACCESS_KEY=...
export TF_VAR_claude_code_oauth_token=...               # real managed value
test -s backend.hcl
if [ ! -s repos.auto.tfvars ]; then
  : "${TF_VAR_active_repos:?set active inventory}"
  : "${TF_VAR_archived_repos:?set archived inventory}"
fi
tofu init -backend-config=backend.hcl -migrate-state   # prompts y/n to copy state up
# Subshell, so the cleanup trap fires when this block finishes rather
# than when the interactive shell it was pasted into eventually exits.
(
umask 077
plan_dir="$(mktemp -d "${TMPDIR:-/tmp}/dear-agent-infra.XXXXXX")"
plan_file="$plan_dir/infra.tfplan"
trap 'rm -f -- "$plan_file"; rmdir "$plan_dir" 2>/dev/null || true' EXIT
tofu plan -out="$plan_file"                            # expect 0 changes
printf 'plan_sha256=%s\n' "$(shasum -a 256 "$plan_file" | awk '{print $1}')"
tofu show -no-color "$plan_file"                       # independent verifier attests exact plan + digest
: "${ATTESTED_PLAN_SHA256:?independent exact-plan attestation required}"
test "$(shasum -a 256 "$plan_file" | awk '{print $1}')" = "$ATTESTED_PLAN_SHA256"
tofu apply "$plan_file"
)
```

If no local state exists (e.g. the previous apply ran in an ephemeral sandbox
and the state was lost), skip the migrate and follow **First-time setup** below —
`import.sh` rebuilds state from the live GitHub resources straight into R2.

## First-time setup

```bash
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID=...  AWS_SECRET_ACCESS_KEY=...   # R2 token (see Remote state)
export TF_VAR_claude_code_oauth_token=...                  # real managed value
cp backend.hcl.example backend.hcl                        # edit ACCOUNT_ID + bucket
test -s backend.hcl
if [ ! -s repos.auto.tfvars ]; then
  : "${TF_VAR_active_repos:?set active inventory}"
  : "${TF_VAR_archived_repos:?set archived inventory}"
fi
tofu init -backend-config=backend.hcl                     # initializes the R2 backend
chmod +x import.sh
./import.sh                                             # prove/import existing bindings
# Subshell, so the cleanup trap fires when this block finishes rather
# than when the interactive shell it was pasted into eventually exits.
(
umask 077
plan_dir="$(mktemp -d "${TMPDIR:-/tmp}/dear-agent-infra.XXXXXX")"
plan_file="$plan_dir/infra.tfplan"
trap 'rm -f -- "$plan_file"; rmdir "$plan_dir" 2>/dev/null || true' EXIT
tofu plan -out="$plan_file"
printf 'plan_sha256=%s\n' "$(shasum -a 256 "$plan_file" | awk '{print $1}')"
tofu show -no-color "$plan_file"                       # independent verifier attests exact plan + digest
: "${ATTESTED_PLAN_SHA256:?independent exact-plan attestation required}"
test "$(shasum -a 256 "$plan_file" | awk '{print $1}')" = "$ATTESTED_PLAN_SHA256"
tofu apply "$plan_file"                                # applies the attested artifact
)
```

The importer fails closed when GitHub cannot list rulesets or when more than
one ruleset matches. For dear-agent it additionally requires existing ruleset
ID `18061003`, accepting either its legacy `branch-protection` name or the
current name declared in `.github/rulesets/main.json`; it never infers that a
second ruleset is safe to create. If that ruleset address already exists in
OpenTofu state, the importer also proves that its repository and immutable ID
resolve to `dear-agent:18061003`; a stale binding aborts recovery.

## Day-to-day usage

```bash
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID=...  AWS_SECRET_ACCESS_KEY=...
export TF_VAR_claude_code_oauth_token=...               # real managed value
test -s backend.hcl
if [ ! -s repos.auto.tfvars ]; then
  : "${TF_VAR_active_repos:?set active inventory}"
  : "${TF_VAR_archived_repos:?set archived inventory}"
fi
tofu init -reconfigure -backend-config=backend.hcl
# Subshell, so the cleanup trap fires when this block finishes rather
# than when the interactive shell it was pasted into eventually exits.
(
umask 077
plan_dir="$(mktemp -d "${TMPDIR:-/tmp}/dear-agent-infra.XXXXXX")"
plan_file="$plan_dir/infra.tfplan"
trap 'rm -f -- "$plan_file"; rmdir "$plan_dir" 2>/dev/null || true' EXIT
tofu plan -out="$plan_file"
printf 'plan_sha256=%s\n' "$(shasum -a 256 "$plan_file" | awk '{print $1}')"
tofu show -no-color "$plan_file"                       # independent verifier attests exact plan + digest
: "${ATTESTED_PLAN_SHA256:?independent exact-plan attestation required}"
test "$(shasum -a 256 "$plan_file" | awk '{print $1}')" = "$ATTESTED_PLAN_SHA256"
tofu apply "$plan_file"                                # applies the attested artifact
)
```

Production reconciliation requires the complete gitignored `repos.auto.tfvars` (or both
`TF_VAR_active_repos` and `TF_VAR_archived_repos`). The GitHub token needs
`repo` for the personal fleet and `admin:org` for the full root's dear-labs
organization ruleset. A placeholder OAuth value is safe only in the
credential-free pull-request fixture plan; using it in production would
overwrite managed repository secrets. Saved plans can contain sensitive values:
keep them outside the repository with restrictive permissions and remove them
after the apply and verification are complete.

Production plans follow a dark-factory two-process path: the planning process
emits a saved artifact and SHA-256 digest, and an independent verifier attests
that exact digest against the merged declaration, complete private inventory,
and expected provider delta. Routine additive or in-place, reversible,
unambiguous plans proceed without human approval and the apply must consume the
attested artifact unchanged. Stop for human authorization only when the exact
plan contains a destroy, replacement, state migration, irreversible change, or
ambiguous effect. A changed digest invalidates the attestation and requires a
fresh independent verification.

The secret-bearing drift and inventory audits run only from trusted default-
branch code. Scheduled runs happen automatically; an operator can request an
immediate default-branch run with a repository dispatch (a manual branch/ref
selector is intentionally unavailable):

```bash
gh api --method POST repos/vbonnet/dear-agent/dispatches -f event_type=tofu-drift
gh api --method POST repos/vbonnet/dear-agent/dispatches -f event_type=branch-protection-audit
gh api --method POST repos/vbonnet/dear-agent/dispatches -f event_type=infra-repo-reconcile
```

## Post-apply verification

Run every check below after each production apply. These commands are
assertions: any non-zero exit means the deployment is incomplete and must not
be reported as reconciled.

```bash
set -euo pipefail
ruleset_address='module.managed_repos["dear-agent"].github_repository_ruleset.branch_protection'
state="$(tofu state show -no-color "$ruleset_address")"
printf '%s\n' "$state" | grep -Eq '^[[:space:]]*repository[[:space:]]*=[[:space:]]*"dear-agent"$'
# Assert both identifiers. `ruleset_id` is the provider's GitHub ID attribute
# and `id` is the resource address the module output reads, so asserting only
# one would let a rename or rebind of the other pass unnoticed.
printf '%s\n' "$state" | grep -Eq '^[[:space:]]*ruleset_id[[:space:]]*=[[:space:]]*18061003$'
printf '%s\n' "$state" | grep -Eq '^[[:space:]]*id[[:space:]]*=[[:space:]]*"18061003"$'

# Exit 0 means state, configuration, and refreshed provider state agree.
tofu plan -detailed-exitcode -no-color

cd ..
go run ./cmd/merge-audit \
  --repos vbonnet/dear-agent \
  --ruleset .github/rulesets/main.json \
  --days 1 \
  --dry-run \
  --json \
  | jq -e '[.[] | select(.type == "ruleset-drift")] | length == 0'

gh api repos/vbonnet/dear-agent/rules/branches/main \
  | jq -e '
      [.[] | select(.ruleset_id == 18061003)] as $rules
      | ($rules | map(.ruleset_id) | unique) == [18061003]
        and (($rules | map(.type) | sort) == [
          "deletion",
          "non_fast_forward",
          "pull_request",
          "required_linear_history",
          "required_status_checks"
        ])
    '
```

Use the next fresh pull request as an enforcement canary. The commands below
prove every canonical context was emitted and every required check reached a
mergeable result; GitHub returns non-zero while a required context is pending or
failed.

```bash
set -euo pipefail
canary_pr="<fresh-pr-number>"
gh pr checks "$canary_pr" --repo vbonnet/dear-agent --required --watch
missing_contexts="$(comm -23 \
  <(jq -r '.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[].context' .github/rulesets/main.json | sort) \
  <(gh pr view "$canary_pr" --repo vbonnet/dear-agent --json statusCheckRollup --jq '.statusCheckRollup[].name' | sort -u))"
test -z "$missing_contexts"
```

Also confirm strict up-to-date enforcement blocks an intentionally behind
canary branch and that the normal squash merge succeeds only after the branch
is current and all eight required contexts are green.

## What this manages

### vbonnet/* personal repos (`managed_repos.tf` → `modules/managed-repo`)

| Setting | Value |
|---|---|
| Allow squash merge | ✅ |
| Allow rebase merge | ❌ (squash-only) |
| Allow merge commits | ❌ (linear history) |
| Allow auto-merge | ✅ |
| Delete branch on merge | ✅ |
| Wiki / Projects | ❌ |
| Dependabot vulnerability alerts | ✅ |
| Dependabot security updates | ✅ |
| Secret scanning + push protection | ✅ public repos only (private = Advanced Security required) |

Branch protection on the default branch of every active repo is enforced by a
single `branch-protection` **repository ruleset** (defined in
`modules/managed-repo`, `active`):

| Rule | Value |
|---|---|
| Required linear history | ✅ |
| No force pushes | ✅ (`non_fast_forward`) |
| No branch deletion | ✅ (`deletion`) |
| Require conversation resolution | ✅ |
| Allowed merge methods | squash only |
| Bypass actors | none (ruleset applies to everyone, incl. the owner) |
| Dismiss stale reviews | ✅ |
| Required approvers | 0 (PR required, solo reviewer sufficient) |
| Required CI checks | per-repo (see `locals.tf`) |

For `dear-agent`, [`.github/rulesets/main.json`](../.github/rulesets/main.json)
is the canonical desired ruleset: `infra/locals.tf` decodes that committed JSON
and the managed-repo module preserves its name, strictness, zero-bypass policy,
and check context plus optional GitHub App integration ID. OpenTofu does not
become a second policy source. An independent verifier must attest the saved
plan's exact digest and scope, and the apply must consume that unchanged plan
file, before the declaration becomes provider-visible.

Other managed repositories retain their inventory-defined policy until each has
its own committed canonical declaration. Existing private inventory and
`TF_VAR_active_repos` values may keep `required_checks = ["Build & Test"]`.
Migrate without a fleet-wide type break by adding
`required_check_identities = [{ context = "Build & Test", integration_id = 15368 }]`;
when present, that structured list is authoritative and preserves GitHub's full
check identity. Repos with no CI (`required_checks = []`) still get the
PR-required + no-force-push rules — they just have no check gate.

### dear-labs org (`dear_labs.tf`)

A baseline `github_organization_ruleset` in **active mode**. (`evaluate`/
audit-only mode is GitHub Enterprise-only and 422s on non-Enterprise accounts,
so the ruleset enforces directly.) The org has no repos yet, so it blocks
nothing until repos are added, at which point it immediately protects their
default branch. Applying this resource needs a token with `admin:org` on
dear-labs — see the note in `dear_labs.tf`; the personal-repo rulesets apply
independently of it.

Rules:
- No branch deletion
- No force push
- Required linear history
- Required PR review (1 approver, dismiss stale, resolve threads)
- No bypass actors (admins cannot bypass)

Required status checks are omitted at the org level — check contexts are
repo-specific. Add `github_repository_ruleset` resources per repo when CI
names are known.

## Deliberate decisions

- **Visibility is preserved, not changed.** Flipping visibility is destructive
  and out of scope.
- **Secret scanning is public-repo-only.** Private repos require GitHub Advanced
  Security; enabling it there would fail on apply.
- **No required status checks for repos with no CI.** An empty `contexts` list
  makes the rule a no-op; we simply omit the block.
- **No bypass actors.** The ruleset applies to everyone including the owner.
  `required_approving_review_count = 0` keeps the solo-maintainer workflow
  viable (a PR is required, but the owner's own merge suffices). Raise the
  approver count per-repo for shared repos.
- **dear-labs runs in active mode.** `evaluate` (audit-only) mode is
  Enterprise-only and 422s on non-Enterprise accounts. The org has no repos
  yet, so active enforcement blocks nothing until repos are added.
- **Archived repos are frozen.** They are declared through the private
  `archived_repos` inventory with `ignore_changes = all` because GitHub rejects
  mutations. They are excluded from `active_repos`; a ruleset cannot be applied
  to an archived repository.
- **Personal Pro account, no merge queues.** The repository ruleset defined in
  `modules/managed-repo` works on GitHub Pro and runs in `active` enforcement —
  `evaluate` mode is Enterprise-only. Merge-queue rulesets require an
  **organization** account, so none is defined.
- **Squash-only + auto-merge is the full merge contract.** See
  [ADR-034](../docs/adr/ADR-034-squash-only-merge-contract.md) for the
  supervisor-facing contract and merge-velocity health thresholds.
- **Standard policy is a module.** The active-repo policy (`github_repository` +
  Dependabot + branch-protection ruleset) lives in `modules/managed-repo` and is
  instantiated via `for_each` in `managed_repos.tf` — one place to change fleet
  policy, reusable across the personal account and the dear-labs org alias.
- **Rulesets are the sole branch-protection mechanism.** The legacy
  `github_branch_protection` resources were validated as fully covered by the
  rulesets and removed in ce-yg6b, eliminating the divergent-mechanism trap
  (classic branch protection had only ever materialized on `dear-agent`, where
  it drifted to 6 stale required checks vs the ruleset's 2).

## Adding a new repo

1. Add an entry to the private `active_repos` inventory in
   `repos.auto.tfvars` (or its secure `TF_VAR_active_repos` equivalent).
2. Run the fail-closed `./import.sh` to prove and import existing bindings.
3. Save a `tofu plan`, obtain independent exact-digest verification, then apply
   that same saved plan.

## Adding CI checks to a repo

Keep existing `required_checks` context names, then add the optional structured
identity field one repository at a time. Preserve `integration_id` when GitHub
returns it:

```bash
gh api /repos/vbonnet/<repo>/commits/main/check-runs \
  --jq '[.check_runs[] | {context: .name, integration_id: .app.id}] | unique'
```

Before removing legacy names, run a read-only plan with the real private
`repos.auto.tfvars` (or the production `TF_VAR_active_repos` secret) and have an
independent verifier confirm that every ruleset update is in-place. The public
example cannot prove that private migration; do not apply until that exact plan
and digest have been independently attested.
