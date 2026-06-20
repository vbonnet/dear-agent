###############################################################################
# vbonnet/* personal repositories — security & merge hygiene defaults.
###############################################################################

resource "github_repository" "active" {
  for_each = local.active_repos

  name       = each.key
  visibility = each.value.visibility

  # Merge hygiene: squash-only; no rebase or merge commits (linear history);
  # auto-merge and branch-delete-after-merge enabled.
  allow_squash_merge     = true
  allow_rebase_merge     = false
  allow_merge_commit     = false
  allow_auto_merge       = true
  delete_branch_on_merge = true

  has_wiki     = false
  has_projects = false
  has_issues   = true

  # Deprecated attribute but the only supported path to vulnerability alerts;
  # keep until the provider ships a dedicated resource.
  vulnerability_alerts = true

  archived = false

  # Secret scanning and push protection are free for public repos.
  # Private repos require GitHub Advanced Security — do NOT enable there.
  dynamic "security_and_analysis" {
    for_each = each.value.visibility == "public" ? [1] : []
    content {
      secret_scanning {
        status = "enabled"
      }
      secret_scanning_push_protection {
        status = "enabled"
      }
    }
  }

  # Project metadata (description, topics, homepage) is owned by each repo,
  # not by this IaC. Ignore it so the plan stays focused on security/merge
  # settings and never wipes repo metadata.
  lifecycle {
    ignore_changes = [
      description,
      homepage_url,
      topics,
      pages,
    ]
  }
}

resource "github_repository_dependabot_security_updates" "active" {
  for_each   = local.active_repos
  repository = github_repository.active[each.key].name
  enabled    = true
}

###############################################################################
# Archived repositories — declared but frozen
###############################################################################

resource "github_repository" "archived" {
  for_each   = local.archived_repos
  name       = each.key
  visibility = each.value.visibility
  archived   = true

  lifecycle {
    ignore_changes = all
  }
}
