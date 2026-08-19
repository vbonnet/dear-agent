terraform {
  required_version = "= 1.12.5"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "= 6.13.0"
    }
  }
}

# Personal account provider (vbonnet/*).
# Authentication: export GITHUB_TOKEN="$(gh auth token)"
# The token is never stored in variables, tfvars, or state.
provider "github" {
  owner = var.personal_owner
}

# dear-labs org provider.
# The same GITHUB_TOKEN must have org:admin (or equivalent) scope on dear-labs.
provider "github" {
  alias = "dearlabs"
  owner = var.org_name
}
