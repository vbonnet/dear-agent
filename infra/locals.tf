locals {
  # Active (non-archived) personal repositories.
  # required_checks: exact GitHub Actions check-run names that must pass before merge.
  # Derive from: gh api /repos/vbonnet/<repo>/commits/main/check-runs | jq '.[].name'
  active_repos = {
    "dear-agent" = {
      visibility     = "public"
      default_branch = "main"
      # ci.yml matrix: 2 jobs, both must pass.
      required_checks = [
        "Build & Test (ubuntu-latest)",
        "Build & Test (macos-latest)",
      ]
    }
    # NOTE: "engram" is intentionally NOT here — it is an ARCHIVED repo and
    # cannot take a ruleset (GitHub rejects mutations on archived repos). It is
    # declared under archived_repos below (frozen, ignore_changes = all) rather
    # than removed entirely: removing a previously-managed repo would make a
    # full `tofu apply` propose DESTROYING module.managed_repos["engram"], which
    # deletes the repo on GitHub. archived_repos keeps it safely managed.
    "brain-v2" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = ["go test -race"]
    }
    "engram-research" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = ["Build & Test (engram/hooks/cmd)"]
    }
    "vbonnet.ai" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = ["deploy"]
    }
    "gdoc-sync" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = ["test"]
    }
    "ai-conversation-logs" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = ["Scan for sensitive terms"]
    }
    # Repos below have no CI workflows; branch protection still applies.
    "dotfiles" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = []
    }
    "engram-kb" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = []
    }
    "beads-context-engine" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = []
    }
    "vbonnet" = {
      visibility      = "public"
      default_branch  = "main"
      required_checks = []
    }
    "ai-sdlc" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = []
    }
    # NOTE: "infra-iac" is intentionally NOT here — it was the original
    # standalone IaC repo that this infra/ directory supersedes (see README).
    # It has been ARCHIVED on GitHub and moved to archived_repos below
    # (frozen, ignore_changes = all). Same rationale as "engram" above:
    # removing it outright would make a full `tofu apply` propose DESTROYING
    # the github_repository resource, which deletes the repo on GitHub.
    "network-monitor" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = []
    }
  }

  # Merge-queue inputs removed: merge queues require an ORGANIZATION account
  # and are unavailable on a personal account (see rulesets.tf). Reintroduce a
  # merge_queue_repos local here if these repos ever move under an org.

  # Archived repositories: GitHub rejects mutations on archived repos, so
  # these are declared minimally with all changes ignored.
  archived_repos = {
    "engram"                     = { visibility = "private" }
    "ai-tools"                   = { visibility = "private" }
    "comp-520-peephole-compiler" = { visibility = "private" }
    "comp-520"                   = { visibility = "private" }
    "infra-iac"                  = { visibility = "private" }
  }
}
