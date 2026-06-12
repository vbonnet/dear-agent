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

## First-time setup

```bash
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
tofu init
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

A baseline `github_organization_ruleset` in **evaluate mode** (audit-only, merges
are not blocked). Covers every repo's default branch as soon as repos are added
to the org. Flip `enforcement = "active"` to enforce once the team is comfortable.

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
- **dear-labs starts in evaluate mode.** The org has no repos yet; evaluate
  lets rules be validated before they block anyone.
- **Archived repos are frozen.** `ai-tools`, `comp-520`, `comp-520-peephole-compiler`
  are declared with `ignore_changes = all` because GitHub rejects mutations.
- **Personal account, not an org.** `vbonnet/*` uses `github_branch_protection`,
  not rulesets — rulesets require an org account or GitHub Enterprise for the
  org-level resource.

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
