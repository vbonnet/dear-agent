# Constructs the expected zero-bypass branch-protection ruleset for a
# non-dear-agent managed repository from its private inventory entry (the
# input document). dear-agent itself uses its own checked-in
# .github/rulesets/main.json instead (see
# .github/workflows/branch-protection-audit.yml); this projects the same
# fleet policy the managed-repo OpenTofu module applies
# (infra/managed_repos.tf) so the two can be compared, after normalize.jq,
# against the live ruleset.
#
# strict_required_status_checks_policy is unconditionally true:
# infra/managed_repos.tf sets it to true for every repository regardless of
# name (PR #1271 made every fleet repository inherit dear-agent's up-to-date
# requirement). A hardcoded false here would make every correctly
# reconciled repository compare as drifted against its own live ruleset.
. as $cfg
| (if ($cfg | has("required_check_identities")) and $cfg.required_check_identities != null then
   $cfg.required_check_identities
 else
   (if ($cfg | has("required_checks")) and $cfg.required_checks != null then
      $cfg.required_checks
    else []
    end) as $legacy_checks
   | [$legacy_checks[] | {context: ., integration_id: null}]
 end)
| map({context, integration_id}) as $checks
| {
    name: "branch-protection",
    target: "branch",
    enforcement: "active",
    bypass_actors: [],
    conditions: {ref_name: {include: ["~DEFAULT_BRANCH"], exclude: []}},
    rules: ([
      {type: "deletion"},
      {type: "non_fast_forward"},
      {type: "required_linear_history"},
      {type: "pull_request", parameters: {
        required_approving_review_count: 0,
        dismiss_stale_reviews_on_push: true,
        require_code_owner_review: false,
        require_last_push_approval: false,
        required_review_thread_resolution: true,
        required_reviewers: [],
        allowed_merge_methods: ["squash"]
      }}
    ] + if ($checks | length) > 0 then [{
      type: "required_status_checks",
      parameters: {
        strict_required_status_checks_policy: true,
        do_not_enforce_on_create: false,
        required_status_checks: $checks
      }
    }] else [] end)
  }
