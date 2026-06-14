locals {
  # Active (non-archived) personal repositories.
  # required_checks: exact GitHub Actions check-run names that must pass before merge.
  # Derive from: gh api /repos/vbonnet/<repo>/commits/main/check-runs | jq '.[].name'
  active_repos = {
    "dear-agent" = {
      visibility      = "public"
      default_branch  = "main"
      # ci.yml matrix: 2 jobs, both must pass.
      required_checks = [
        "Build & Test (ubuntu-latest)",
        "Build & Test (macos-latest)",
      ]
    }
    "engram" = {
      visibility      = "private"
      default_branch  = "main"
      # core.yml matrix: require the Linux leg as the canonical gate.
      required_checks = ["build-and-test (ubuntu-latest)"]
    }
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
    "infra-iac" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = []
    }
    "network-monitor" = {
      visibility      = "private"
      default_branch  = "main"
      required_checks = []
    }
  }

  # Repos with merge queue enabled. Start with dear-agent; expand by adding
  # entries here. The merge queue replaces manual "merge when ready" with a
  # serialized queue that tests PRs in groups before merging.
  merge_queue_repos = {
    "dear-agent" = {
      min_group_size        = 1
      max_group_size        = 5
      check_timeout_minutes = 30
    }
  }

  # Archived repositories: GitHub rejects mutations on archived repos, so
  # these are declared minimally with all changes ignored.
  archived_repos = {
    "ai-tools"                   = { visibility = "private" }
    "comp-520-peephole-compiler" = { visibility = "private" }
    "comp-520"                   = { visibility = "private" }
  }
}
