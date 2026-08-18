###############################################################################
# Archived repositories — declared but frozen.
#
# The active vbonnet/* fleet (github_repository + dependabot + branch-protection
# ruleset) is managed by the ./modules/managed-repo module — see
# managed_repos.tf. This file only declares ARCHIVED repos, which take a
# deliberately different shape: GitHub rejects mutations on archived repos, so
# they are declared minimally with all changes ignored. They are kept in state
# (rather than removed) so a full `tofu apply` never proposes DESTROYING them,
# which would delete the repo on GitHub.
###############################################################################

resource "github_repository" "archived" {
  for_each   = var.archived_repos
  name       = each.key
  visibility = each.value.visibility
  archived   = true

  lifecycle {
    ignore_changes = all
  }
}
