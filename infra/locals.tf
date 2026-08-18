# The repo inventory (active_repos / archived_repos) previously lived here as
# locals. Because this repo is PUBLIC, enumerating private repo names and their
# per-repo security posture here leaked them. The inventory now lives in the
# variables `var.active_repos` / `var.archived_repos` (see variables.tf),
# populated from the gitignored infra/repos.auto.tfvars (schema in
# repos.auto.tfvars.example). This file is intentionally left as a marker.
#
# Merge-queue inputs: merge queues require an ORGANIZATION account and are
# unavailable on a personal account (see rulesets.tf). If these repos ever move
# under an org, reintroduce a merge_queue_repos input.
#
# ARCHIVED-repo handling (why archived repos stay declared rather than removed):
# GitHub rejects ruleset/settings mutations on archived repos, and REMOVING a
# previously-managed repo would make `tofu apply` propose DESTROYING its
# github_repository resource — which deletes the repo on GitHub. So archived
# repos (e.g. engram, ai-tools, comp-520, infra-iac) are kept in
# var.archived_repos, declared minimally with all changes ignored.
#
# PENDING ONBOARDING (from the reconciliation guard added in this PR, ce-1onr):
# cat-pipeline, codebase-analyzer, feh-sandbox were flagged as live-but-unmanaged
# repos (created via `gh repo create` in cleanup sessions, never added to the
# fleet). Since the inventory moved to the gitignored repos.auto.tfvars, adding
# them here would leak private repo names again — they must be added to that
# private tfvars out of band instead (visibility "private", no CI workflows yet
# so required_checks = []).
