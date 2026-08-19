###############################################################################
# State migration: inline resources → ./modules/managed-repo (ce-32o5).
#
# These moved blocks rename the existing state addresses to their new homes
# inside the managed_repos module so `tofu plan` reports ZERO changes — this is
# a pure refactor, NOT a destroy/recreate (which would delete and re-create
# every repo on GitHub).
#
# Instance keys (repo names) are preserved automatically: both the source
# resources and the module use the same for_each over var.active_repos, so
# OpenTofu matches source instance ["X"] to module.managed_repos["X"] by key.
#
# Apply workflow (per README): save a plan and CONFIRM it shows only moves and
# 0 resource changes before applying that exact plan file. Once a clean apply
# has run against every environment and state no longer references the old
# addresses, these blocks can be deleted.
###############################################################################

moved {
  from = github_repository.active
  to   = module.managed_repos.github_repository.this
}

moved {
  from = github_repository_dependabot_security_updates.active
  to   = module.managed_repos.github_repository_dependabot_security_updates.this
}

moved {
  from = github_repository_ruleset.branch_protection
  to   = module.managed_repos.github_repository_ruleset.branch_protection
}
