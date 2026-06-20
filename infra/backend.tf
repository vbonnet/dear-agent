# Remote state backend (ce-6f1b).
#
# State lives in a Cloudflare R2 bucket via OpenTofu's S3-compatible backend.
# R2 is chosen over AWS S3 because this account already runs on Cloudflare
# (wrangler is the installed cloud CLI; there is no AWS CLI / AWS account).
#
# Locking uses OpenTofu's NATIVE state lock (`use_lockfile = true`, OpenTofu
# >= 1.10) which writes a `<key>.tflock` object with an S3 conditional PUT.
# That removes the classic DynamoDB lock-table dependency entirely — R2
# supports the conditional writes the lockfile needs.
#
# This is a PARTIAL backend configuration: the account-specific `bucket` and
# R2 `endpoints.s3` (which embeds the account ID) are supplied at init time
# from a gitignored `backend.hcl` (see backend.hcl.example). Credentials are
# never stored here — they come from the environment, mirroring how the
# GitHub provider reads GITHUB_TOKEN (see providers.tf / variables.tf):
#
#   export AWS_ACCESS_KEY_ID="<R2 token Access Key ID>"
#   export AWS_SECRET_ACCESS_KEY="<R2 token Secret Access Key>"
#
#   tofu init -backend-config=backend.hcl
#
# See "Remote state" in README.md for first-time setup and migration.

terraform {
  backend "s3" {
    key          = "dear-agent/infra/terraform.tfstate"
    region       = "auto" # R2 ignores region; the S3 backend still requires a value.
    use_lockfile = true   # Native S3 locking — no DynamoDB table needed.

    # R2 is S3-compatible but not AWS. These skips stop the backend from
    # calling AWS-only metadata/STS/account endpoints that R2 does not serve.
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true # R2 rejects the default AWS checksum trailers.

    # `bucket` and `endpoints = { s3 = "https://<accountid>.r2.cloudflarestorage.com" }`
    # are provided via -backend-config=backend.hcl (gitignored, env-specific).
  }
}
