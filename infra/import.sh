#!/usr/bin/env bash
# Import existing GitHub state so `tofu plan` shows drift, not creation.
#
# Run once after `tofu init`:
#   export GITHUB_TOKEN="$(gh auth token)"
#   ./import.sh
#   tofu plan
#
# A failed import is non-fatal: the resource doesn't exist yet and tofu plan
# will propose creating it. That is the correct drift, not an error.
set -euo pipefail

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN is not set. Run: export GITHUB_TOKEN=\"\$(gh auth token)\"" >&2
  exit 1
fi

# Personal account owner. Repository rulesets are imported by numeric id, which
# must be looked up per repo (see the ruleset import in the ACTIVE_REPOS loop).
OWNER="${PERSONAL_OWNER:-vbonnet}"

ACTIVE_REPOS=(
  dear-agent
  brain-v2
  engram-research
  vbonnet.ai
  gdoc-sync
  ai-conversation-logs
  dotfiles
  engram-kb
  beads-context-engine
  vbonnet
  ai-sdlc
  infra-iac
  network-monitor
)

ARCHIVED_REPOS=(
  engram
  ai-tools
  comp-520-peephole-compiler
  comp-520
)

imp() {
  local addr="$1" id="$2"
  if tofu state show "$addr" >/dev/null 2>&1; then
    echo "skip (already imported): $addr"
    return 0
  fi

  local err
  if err=$(tofu import "$addr" "$id" 2>&1 >/dev/null); then
    echo "imported: $addr"
  else
    # Non-fatal "nothing to import" cases — the resource does not exist yet, so
    # `tofu plan` will correctly propose creating it. Each matched phrase is a
    # real provider message for a missing remote object:
    #   - "not found" / "404"  : repo or dependabot config absent
    #   - "associated"         : no security config associated
    #   - "could not find a branch protection rule with the pattern '<branch>'"
    #                            : repo has no branch protection yet (most repos
    #                            here are pre-rulesets, so this is the common case)
    if [[ "$err" == *"not found"* ||
          "$err" == *"404"* ||
          "$err" == *"associated"* ||
          "$err" == *"could not find a branch protection rule"* ]]; then
      echo "not imported (will be CREATED by plan): $addr"
    else
      echo "Error: Failed to import $addr" >&2
      echo "$err" >&2
      exit 1
    fi
  fi
}

for r in "${ACTIVE_REPOS[@]}"; do
  imp "github_repository.active[\"$r\"]" "$r"
  imp "github_branch_protection.active[\"$r\"]" "$r:main"
  imp "github_repository_dependabot_security_updates.active[\"$r\"]" "$r"

  # Import the existing "branch-protection" repository ruleset if one exists.
  # Rulesets were applied in an earlier pass. Without importing them, `tofu plan`
  # proposes CREATING a second ruleset of the same name — GitHub permits
  # duplicate ruleset names, so a blind apply DOUBLES enforcement instead of
  # reconciling it. Rulesets import by numeric id (<repo>:<id>), so look it up.
  rs_id=$(gh api "/repos/${OWNER}/$r/rulesets" \
            --jq 'map(select(.name=="branch-protection")) | .[0].id // empty' 2>/dev/null || true)
  if [[ -n "$rs_id" ]]; then
    imp "github_repository_ruleset.branch_protection[\"$r\"]" "$r:$rs_id"
  else
    echo "not imported (will be CREATED by plan): github_repository_ruleset.branch_protection[\"$r\"]"
  fi
done

for r in "${ARCHIVED_REPOS[@]}"; do
  imp "github_repository.archived[\"$r\"]" "$r"
done

# dear-labs org ruleset (created fresh — no prior state to import).
echo ""
echo "Note: github_organization_ruleset.baseline is new — tofu plan will propose creating it."
echo "Import complete. Next: tofu plan"
