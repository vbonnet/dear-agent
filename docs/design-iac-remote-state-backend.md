# Design Spike: IaC Remote State Backend

- **Bead:** ce-6f1b (parent ce-kf6j — IaC standardization epic)
- **Status:** Proposed (design spike — no Terraform changes in this PR)
- **Date:** 2026-06-20
- **Decision driver:** `infra/` Terraform/OpenTofu state is local-only on one
  machine. No durability, no locking, no shared visibility — the likely reason
  repo-settings drift (ce-1onr) went uncorrected and invisible. Move state to a
  remote backend while keeping the **token-out-of-state invariant** intact.

## Context

`infra/` manages GitHub repository settings, branch protection, repository
rulesets (`vbonnet/*`), and an organization ruleset (`dear-labs`) through the
`integrations/github` provider. Authentication is environment-only: the provider
reads `GITHUB_TOKEN` (`export GITHUB_TOKEN="$(gh auth token)"`), and there is
deliberately **no `github_token` Terraform variable** (`variables.tf`) — so the
token can never land in `*.tfvars` or state.

Today `providers.tf` declares no `backend` block, so OpenTofu defaults to a
**local** `terraform.tfstate`. `.gitignore` excludes `*.tfstate*`, so that state
is never committed. The practical consequences:

- **No durability.** Lose or wipe the working machine (or run in an ephemeral
  sandbox, as ce-1onr did) and the entire state is gone — every resource must be
  re-imported via `import.sh`.
- **No locking.** Two concurrent `tofu apply` runs can race and corrupt state.
- **No shared visibility.** State cannot be audited or shared; drift is
  invisible until a `plan` is run on the one machine that holds state.

### What is in state scope

`tofu state` for `infra/` tracks GitHub-API resources only — there are no
compute, network, or secret resources:

| Resource | File | Notes |
|---|---|---|
| `github_repository.active` (map) | `repos.tf` | settings for each `vbonnet/*` repo |
| `github_repository.archived` (map) | `repos.tf` | frozen, `ignore_changes = all` |
| `github_repository_dependabot_security_updates.active` | `repos.tf` | per-repo |
| `github_branch_protection.active` (map) | `branch_protection.tf` | default-branch rules |
| `github_repository_ruleset.branch_protection` (map) | `rulesets.tf` | active rulesets |
| `github_organization_ruleset.baseline` | `dear_labs.tf` | dear-labs org baseline |

**Secret-in-state surface.** None of these resources stores a credential as an
attribute. The only secret in the workflow is `GITHUB_TOKEN`, which is a
provider-config value read from the environment, not a resource attribute — so
it is **not** serialized into state. This is the invariant we must preserve when
choosing and configuring a backend: nothing the backend stores should ever
contain the token.

## Backend options compared

Scored on **cost**, **complexity** (setup + ongoing ops), **secret-in-state
risk** (does the backend itself encourage or require a secret in state?), and
**locking** (does it provide concurrency protection without bolt-on infra?).

| Option | Cost | Complexity | Secret-in-state risk | Locking | Verdict |
|---|---|---|---|---|---|
| **S3-compatible (Cloudflare R2)** | Free tier ample for a few-KB state object | Low — stock `s3` backend + endpoint + `skip_*` flags | None — state object is opaque; creds via env | **Native** (`use_lockfile`, OpenTofu ≥ 1.10) — `.tflock` via conditional PUT, no DynamoDB | **Chosen** |
| S3-compatible (MinIO / Backblaze B2) | B2 free tier / MinIO self-host | Low–Medium — same backend, but MinIO is infra to run; B2 conditional-write support varies | None | Native lockfile depends on conditional-PUT support (B2 ok, older MinIO not) | Viable, worse fit |
| GitHub Releases artifact | Free | High — no native TF backend; needs a wrapper to pull/push the artifact around each command | Medium — easy to fumble state into git or a public asset | **None** — no locking primitive at all | Rejected |
| Terraform Cloud free tier | Free up to resource cap | Medium — HCP account, org/workspace, `cloud {}` block, API token | Low — but adds a third-party token to manage | Native (managed) | Rejected — new vendor + token |
| Local + git-crypt | Free | Medium — git-crypt keys, must remember to encrypt | **High** — state lands in git history; one missed encrypt leaks it forever | None | Rejected |

### Why R2 wins for this account

- **Already on Cloudflare.** `wrangler` is the installed cloud CLI; there is no
  AWS account or AWS CLI. R2 is S3-compatible, so the stock `s3` backend works
  with a custom endpoint plus a few `skip_*` flags — no new vendor relationship.
- **Locking with zero extra infra.** OpenTofu ≥ 1.10 supports a native state
  lock (`use_lockfile = true`) that writes a `<key>.tflock` object using an S3
  conditional PUT. R2 supports those conditional writes, so we get mutual
  exclusion **without** the classic DynamoDB lock table.
- **Cost is effectively zero.** State is a few-KB JSON object; R2's free tier
  covers it with no egress concerns for this access pattern.
- **Keeps the secret model.** R2 credentials are supplied as
  `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` in the environment — exactly
  mirroring how `GITHUB_TOKEN` is already handled. No credential is written to
  HCL, tfvars, or state.

## Recommended backend: Cloudflare R2 (S3-compatible)

### Token-out-of-state invariant — how it is preserved

The invariant has two halves: keep the **GitHub** token out of state, and keep
the **R2** credentials out of state and out of git.

1. **No secret resource attributes.** The managed resources are GitHub settings
   objects; none has a secret attribute. We do **not** add resources whose
   outputs are secrets (e.g. `github_actions_secret` with a plaintext
   `plaintext_value`), which would serialize the secret into state. If such a
   resource is ever needed, its value must come from a write-only/ephemeral
   source, and any exposed output must be marked `sensitive = true` so it is
   redacted from CLI/log output (note: `sensitive` redacts *display*, it does
   **not** remove the value from the state file — so the real rule is "don't put
   secrets in state at all").
2. **Provider token stays in the environment.** `GITHUB_TOKEN` is read by the
   provider at runtime; there is no `github_token` variable, so it cannot reach
   tfvars or state. Unchanged by this design.
3. **R2 credentials stay in the environment.** `AWS_ACCESS_KEY_ID` /
   `AWS_SECRET_ACCESS_KEY` are exported, never written to `backend.tf` or the
   partial config file.
4. **Account-specific, non-secret backend values stay out of git.** The
   `bucket` name and R2 `endpoints.s3` (which embeds the account ID) are
   supplied at init time from a **partial backend config** (`backend.hcl`,
   gitignored), with a tracked `backend.hcl.example`. These are not secrets, but
   keeping them out of the committed HCL avoids leaking account identifiers.

Net: the state object in R2 contains only GitHub-settings attributes; no token
(GitHub or R2) is ever serialized.

### Backend configuration shape (partial config)

`backend.tf` declares the backend with only the non-account-specific, stable
fields. The `bucket` and `endpoints` come from `-backend-config=backend.hcl`:

```hcl
terraform {
  backend "s3" {
    key          = "dear-agent/infra/terraform.tfstate"
    region       = "auto" # R2 ignores region; the s3 backend still requires a value.
    use_lockfile = true   # Native S3 locking — no DynamoDB table.

    # R2 is S3-compatible but not AWS — skip AWS-only metadata/STS/account calls.
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true # R2 rejects default AWS checksum trailers.

    # bucket + endpoints.s3 supplied via -backend-config=backend.hcl (gitignored).
  }
}
```

`backend.hcl` (gitignored; `backend.hcl.example` tracked):

```hcl
bucket = "vbonnet-tofu-state"
endpoints = {
  s3 = "https://ACCOUNT_ID.r2.cloudflarestorage.com"
}
```

## Locking

OpenTofu's native S3 state lock (`use_lockfile = true`, OpenTofu ≥ 1.10):

- Before mutating state, OpenTofu writes a `<key>.tflock` object using an **S3
  conditional PUT** (`If-None-Match: *`) — the write only succeeds if the lock
  object does not already exist. R2 honors the conditional, so a second
  concurrent `apply` fails to acquire the lock instead of racing.
- On completion the lock object is deleted. A crashed run can leave a stale
  lock; `tofu force-unlock <LOCK_ID>` clears it after confirming no apply is
  actually running.
- **No DynamoDB table** (the legacy AWS approach) is needed — that dependency is
  eliminated entirely.

## Migration path: local state → R2

Two cases.

**A. A local `terraform.tfstate` exists** (an earlier apply ran on this
machine):

```bash
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID="<R2 Access Key ID>"
export AWS_SECRET_ACCESS_KEY="<R2 Secret Access Key>"
cp backend.hcl.example backend.hcl          # edit ACCOUNT_ID + bucket

tofu init -backend-config=backend.hcl -migrate-state   # prompts to copy state up
tofu plan                                              # expect 0 changes
```

`-migrate-state` detects the local→remote transition and offers to copy the
existing state into R2. Optionally `tofu state pull > backup.tfstate` first as a
safety copy (delete it after — it contains full state).

**B. No local state** (the previous apply ran in an ephemeral sandbox and state
was lost — the ce-1onr situation). Skip the migrate; rebuild state from the live
GitHub resources straight into R2:

```bash
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID=...  AWS_SECRET_ACCESS_KEY=...
cp backend.hcl.example backend.hcl          # edit ACCOUNT_ID + bucket

tofu init -backend-config=backend.hcl       # initialize the R2 backend (empty state)
./import.sh                                  # import existing repos + protection
tofu plan                                    # expect ~0 changes
```

`import.sh` is idempotent — importing already-imported addresses is a no-op — so
both paths converge on a remote state that matches the live GitHub config.

## W0 requirements (exact handoff for implementation)

The implementing work (ce-6f1b W0) must land:

1. **`infra/backend.tf`** — the `terraform { backend "s3" { … } }` block exactly
   as in "Backend configuration shape" above (`key`, `region = "auto"`,
   `use_lockfile = true`, the five `skip_*` flags).
2. **`infra/backend.hcl.example`** — tracked template with `bucket` and
   `endpoints.s3` placeholders; real `backend.hcl` gitignored.
3. **`.gitignore`** — add `backend.hcl` (the live partial config) alongside the
   existing `*.tfstate*` excludes.
4. **`README.md`** — a "Remote state" section documenting one-time bucket
   provisioning, backend config, and both migration paths.

### One-time human steps (run once, not in CI)

```bash
# 1. Create the state bucket (idempotent).
wrangler r2 bucket create vbonnet-tofu-state

# 2. Create an R2 API token: Cloudflare dashboard → R2 → Manage API Tokens,
#    "Object Read & Write", scoped to the bucket. Record Access Key ID + Secret.

# 3. Initialize / migrate (see Migration path A or B above):
cd infra/
export GITHUB_TOKEN="$(gh auth token)"
export AWS_ACCESS_KEY_ID=...  AWS_SECRET_ACCESS_KEY=...
cp backend.hcl.example backend.hcl          # edit ACCOUNT_ID + bucket
tofu init -backend-config=backend.hcl [-migrate-state]
tofu plan                                    # expect ~0 changes before any apply
```

## Deliberate decisions

- **R2 over AWS S3.** No AWS account exists; the account already runs on
  Cloudflare. R2's S3 compatibility lets the stock backend work unchanged.
- **Native lockfile over DynamoDB.** OpenTofu ≥ 1.10 `use_lockfile` removes the
  lock-table dependency; R2's conditional PUT support is sufficient.
- **Partial backend config over hardcoded bucket/endpoint.** Keeps the
  account-ID-bearing endpoint out of committed HCL without making it a secret.
- **Credentials via environment, never in files.** R2 keys mirror the existing
  `GITHUB_TOKEN` model — no credential in HCL, tfvars, or state.
- **No new resources that serialize secrets.** The token-out-of-state invariant
  is preserved by not introducing secret-valued resources; `sensitive = true`
  redacts display but does not remove a value from state, so the rule is "keep
  secrets out of state entirely," not "mark them sensitive."

## Scope / non-goals

- No Terraform/HCL changes in this PR — design only. Implementation lands under
  ce-6f1b W0.
- State encryption-at-rest beyond R2's server-side encryption is out of scope;
  state holds no secrets, so client-side encryption adds ops cost without a
  threat to mitigate here.
- Multi-environment/workspace state layout is out of scope — a single `infra/`
  state with one `key` is sufficient for the current footprint.
