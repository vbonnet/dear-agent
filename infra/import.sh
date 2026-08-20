#!/usr/bin/env bash
# Import existing GitHub state so `tofu plan` shows drift, not creation.
#
#   export GITHUB_TOKEN="$(gh auth token)"
#   ./import.sh
#   tofu plan
#
# Every decision this needs — validating the evaluated inventory, choosing
# which ruleset is safe to import, proving an existing state address is bound
# to the object the plan expects, deciding whether a provider failure means
# "absent" or "broken" — lives in cmd/tofu-import-plan, under unit test in
# internal/tofuimport. This script only collects evidence and executes the
# resulting plan.
set -euo pipefail

owner="${PERSONAL_OWNER:-vbonnet}"
here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
planner="${TOFU_IMPORT_PLAN:-tofu-import-plan}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

[[ -n "${GITHUB_TOKEN:-}" ]] || {
  # shellcheck disable=SC2016  # the command is shown to the reader verbatim
  echo 'GITHUB_TOKEN is not set. Run: export GITHUB_TOKEN="$(gh auth token)"' >&2
  exit 1
}

# Evidence, gathered before anything is decided. The inventory comes from the
# same evaluated OpenTofu variables as the module graph, so there is no second
# hard-coded fleet list to drift.
tofu console -compact-warnings \
  <<< 'jsonencode({active = sort(keys(var.active_repos)), archived = sort(keys(var.archived_repos))})' \
  | jq -r 'fromjson' > "$work/inventory.json"
tofu show -json > "$work/state.json" 2>/dev/null || : > "$work/state.json"
mkdir -p "$work/rulesets"
while IFS= read -r repo; do
  gh api --paginate --slurp "/repos/$owner/$repo/rulesets?per_page=100" > "$work/rulesets/$repo.json"
done < <("$planner" repos --inventory "$work/inventory.json")

# Plan. Every identity is resolved here, before any state is mutated, so an
# ambiguous or unreadable listing cannot surface halfway through and leave a
# partially imported state behind.
"$planner" plan \
  --inventory "$work/inventory.json" \
  --state "$work/state.json" \
  --canonical-ruleset "$here/../.github/rulesets/main.json" \
  --rulesets-dir "$work/rulesets" > "$work/plan.tsv"

# Execute.
while IFS=$'\x1f' read -r verb address import_id reason; do
  case "$verb" in
    skip | create) echo "$verb: $address ($reason)" ;;
    import)
      # -input=false so a provider that would prompt fails instead of hanging
      # a CI job until its timeout, which reports nothing at all.
      if tofu import -input=false "$address" "$import_id" > "$work/import.log" 2>&1; then
        echo "imported: $address"
      elif "$planner" classify --provider-output "$work/import.log" > /dev/null; then
        echo "not imported (will be CREATED by plan): $address"
      else
        echo "Error: failed to import $address" >&2
        cat "$work/import.log" >&2
        exit 1
      fi
      ;;
  esac
done < "$work/plan.tsv"

echo "Import complete. Next: tofu plan"
