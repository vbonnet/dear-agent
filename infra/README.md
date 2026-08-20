# infra/

OpenTofu Infrastructure-as-Code for GitHub repositories under `vbonnet/` and
the `dear-labs` organization. Manages security defaults, branch protection, and
org-level rulesets.

> **Supersedes `vbonnet/infra-iac`** — that standalone repo was the initial
> attempt; this directory is the canonical home going forward. Archive
> `infra-iac` once you've run `./import.sh` and `tofu apply` here.

## Layout

```
infra/
├── providers.tf              # Two GitHub providers: personal + dear-labs org
├── variables.tf              # personal_owner, org_name (token via env only)
├── locals.tf                 # Repo inventory with per-repo required CI checks
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
module and instantiated once via `for_each` over `local.active_repos` in
`managed_repos.tf`. Fleet-wide policy changes live in the module; the per-repo
inventory lives in `locals.tf`. The module is provider-agnostic, so the same
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
to the remote backend once:

```bash
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID=...  AWS_SECRET_ACCESS_KEY=...
tofu init -backend-config=backend.hcl -migrate-state   # prompts y/n to copy state up
tofu plan                                              # expect 0 changes
```

If no local state exists (e.g. the previous apply ran in an ephemeral sandbox
and the state was lost), skip the migrate and follow **First-time setup** below —
`import.sh` rebuilds state from the live GitHub resources straight into R2.

## First-time setup

```bash
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID=...  AWS_SECRET_ACCESS_KEY=...   # R2 token (see Remote state)
cp backend.hcl.example backend.hcl                        # edit ACCOUNT_ID + bucket
tofu init -backend-config=backend.hcl                     # initializes the R2 backend
chmod +x import.sh
./import.sh        # import existing repos + branch protection into state
tofu plan          # review: expect ~0 changes for existing resources
tofu apply         # only after a human reviews the plan
```

## Day-to-day usage

```bash
export GITHUB_TOKEN="$(gh auth token)"
tofu plan          # always review before applying
tofu apply
```

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

Required CI checks are configured per repo. Repos with no CI (`required_checks = []`)
still get the PR-required + no-force-push rules — they just have no check gate.

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
- **Archived repos are frozen.** `ai-tools`, `comp-520`, `comp-520-peephole-compiler`
  are declared with `ignore_changes = all` because GitHub rejects mutations.
  `engram` is likewise archived and is therefore **excluded** from
  `active_repos` — a ruleset cannot be applied to an archived repo.
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

1. Add an entry to `local.active_repos` in `locals.tf`.
2. Run `./import.sh` (or manually `tofu import`) to import the existing repo.
3. `tofu plan` → review → `tofu apply`.

## Adding CI checks to a repo

Update `required_checks` in `locals.tf`. Derive the exact check names from:

```bash
gh api /repos/vbonnet/<repo>/commits/main/check-runs \
  --jq '[.check_runs[].name] | unique[]'
```
