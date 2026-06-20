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
├── repos.tf                  # github_repository + dependabot resources
├── branch_protection.tf      # github_branch_protection (vbonnet/* repos)
├── rulesets.tf               # github_repository_ruleset (vbonnet/* repos, active)
├── dear_labs.tf              # github_organization_ruleset (dear-labs org)
├── import.sh                 # Import existing state before first apply
├── terraform.tfvars.example  # Copy → terraform.tfvars to override defaults
└── .gitignore                # .terraform/, *.tfstate, credentials
```

## Authentication

The GitHub token is supplied **only** through `GITHUB_TOKEN`. It is never a
Terraform variable, never in `*.tfvars`, never in state.

```bash
export GITHUB_TOKEN="$(gh auth token)"
```

The token must have:
- `repo` scope — to manage `vbonnet/*` repositories and branch protection
- `admin:org` scope — to create/manage `dear-labs` org rulesets

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

### vbonnet/* personal repos (`repos.tf` + `branch_protection.tf`)

| Setting | Value |
|---|---|
| Allow squash merge | ✅ |
| Allow rebase merge | ✅ |
| Allow merge commits | ❌ (linear history) |
| Allow auto-merge | ✅ |
| Delete branch on merge | ✅ |
| Wiki / Projects | ❌ |
| Dependabot vulnerability alerts | ✅ |
| Dependabot security updates | ✅ |
| Secret scanning + push protection | ✅ public repos only (private = Advanced Security required) |

Branch protection on the default branch of every active repo:

| Rule | Value |
|---|---|
| Required linear history | ✅ |
| No force pushes | ✅ |
| No branch deletion | ✅ |
| Require conversation resolution | ✅ |
| Enforce admins | ❌ (solo-maintainer workflow) |
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
- **`enforce_admins = false`.** Solo-maintainer workflow where the owner must
  be able to merge their own PRs. Raise per-repo for shared repos.
- **dear-labs runs in active mode.** `evaluate` (audit-only) mode is
  Enterprise-only and 422s on non-Enterprise accounts. The org has no repos
  yet, so active enforcement blocks nothing until repos are added.
- **Archived repos are frozen.** `ai-tools`, `comp-520`, `comp-520-peephole-compiler`
  are declared with `ignore_changes = all` because GitHub rejects mutations.
  `engram` is likewise archived and is therefore **excluded** from
  `active_repos` — a ruleset cannot be applied to an archived repo.
- **Personal Pro account, no merge queues.** Repository rulesets
  (`rulesets.tf`) work on GitHub Pro and run in `active` enforcement —
  `evaluate` mode is Enterprise-only. Merge-queue rulesets require an
  **organization** account, so none is defined; `vbonnet/*` keeps the existing
  `github_branch_protection` resources alongside the rulesets until the
  rulesets are validated.

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
