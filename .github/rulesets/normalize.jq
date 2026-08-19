def required_object($label; $allowed; $required):
  if type != "object" then
    error($label + " must be an object")
  else
    . as $value
    | (($value | keys_unsorted) - $allowed | sort) as $unknown
    | ($required - ($value | keys_unsorted) | sort) as $missing
    | if ($unknown | length) > 0 then
        error($label + " contains unsupported fields: " + ($unknown | join(", ")))
      elif ($missing | length) > 0 then
        error($label + " omits required fields: " + ($missing | join(", ")))
      else
        $value
      end
  end;

def nonempty_string($label):
  if type == "string" and test("\\S") then .
  else error($label + " must be a non-empty string")
  end;

def boolean($label):
  if type == "boolean" then .
  else error($label + " must be a boolean")
  end;

def integer_at_least($label; $minimum):
  if type == "number" and . == floor and . >= $minimum then .
  else error($label + " must be an integer at least " + ($minimum | tostring))
  end;

def string_array($label; $nonempty):
  if type != "array" then
    error($label + " must be an array")
  elif $nonempty and length == 0 then
    error($label + " must not be empty")
  elif all(.[]; type == "string" and test("\\S")) | not then
    error($label + " must contain only non-empty strings")
  else
    sort
  end;

def check:
  required_object("required status check"; ["context", "integration_id"]; ["context"])
  | (.context | nonempty_string("required status check context")) as $context
  | (if has("integration_id") and .integration_id != null then
       .integration_id | integer_at_least("required status check integration_id"; 1)
     else null
     end) as $integration_id
  | {context: $context, integration_id: $integration_id};

def reviewer_identity:
  required_object("required reviewer identity"; ["id", "type"]; ["id", "type"])
  | {
      id: (.id | integer_at_least("required reviewer identity id"; 1)),
      type: (.type | nonempty_string("required reviewer identity type"))
    };

def reviewer:
  required_object(
    "required reviewer";
    ["file_patterns", "minimum_approvals", "reviewer"];
    ["file_patterns", "minimum_approvals", "reviewer"]
  )
  | {
      file_patterns: (.file_patterns | string_array("required reviewer file_patterns"; false)),
      minimum_approvals: (.minimum_approvals | integer_at_least("required reviewer minimum_approvals"; 0)),
      reviewer: (.reviewer | reviewer_identity)
    };

def reviewers:
  if type != "array" then
    error("pull_request required_reviewers must be an array")
  else
    map(reviewer)
    | if length != (map([.reviewer.id, .reviewer.type, .minimum_approvals, .file_patterns] | @json) | unique | length) then
        error("pull_request required_reviewers contains duplicate identities")
      else
        sort_by(.reviewer.id, .reviewer.type, .minimum_approvals, .file_patterns)
      end
  end;

def pull_request_parameters:
  [
    "required_approving_review_count", "dismiss_stale_reviews_on_push",
    "require_code_owner_review", "require_last_push_approval",
    "required_review_thread_resolution", "required_reviewers",
    "allowed_merge_methods"
  ] as $fields
  | required_object("pull_request parameters"; $fields; $fields)
  | {
      required_approving_review_count: (.required_approving_review_count | integer_at_least("pull_request required_approving_review_count"; 0)),
      dismiss_stale_reviews_on_push: (.dismiss_stale_reviews_on_push | boolean("pull_request dismiss_stale_reviews_on_push")),
      require_code_owner_review: (.require_code_owner_review | boolean("pull_request require_code_owner_review")),
      require_last_push_approval: (.require_last_push_approval | boolean("pull_request require_last_push_approval")),
      required_review_thread_resolution: (.required_review_thread_resolution | boolean("pull_request required_review_thread_resolution")),
      required_reviewers: (.required_reviewers | reviewers),
      allowed_merge_methods: (.allowed_merge_methods | string_array("pull_request allowed_merge_methods"; true))
    };

def status_check_parameters:
  ["strict_required_status_checks_policy", "do_not_enforce_on_create", "required_status_checks"] as $fields
  | required_object("required_status_checks parameters"; $fields; $fields)
  | {
      strict_required_status_checks_policy: (.strict_required_status_checks_policy | boolean("required_status_checks strict policy")),
      do_not_enforce_on_create: (.do_not_enforce_on_create | boolean("required_status_checks do_not_enforce_on_create")),
      required_status_checks: (
        if (.required_status_checks | type) != "array" or (.required_status_checks | length) == 0 then
          error("required_status_checks required_status_checks must be a non-empty array")
        else
          .required_status_checks | map(check)
          | if length != (map([.context, .integration_id] | @json) | unique | length) then
              error("required_status_checks contains duplicate check identities")
            else
              sort_by(.context, .integration_id)
            end
        end
      )
    };

def rule:
  required_object("ruleset rule"; ["type", "parameters"]; ["type"])
  | (.type | nonempty_string("ruleset rule type")) as $type
  | if $type == "pull_request" then
      {type: $type, parameters: (.parameters | pull_request_parameters)}
    elif $type == "required_status_checks" then
      {type: $type, parameters: (.parameters | status_check_parameters)}
    elif ($type == "deletion" or $type == "non_fast_forward" or $type == "required_linear_history") then
      if has("parameters") then error($type + " rule has unsupported parameters")
      else {type: $type}
      end
    else
      error("unsupported ruleset rule: " + $type)
    end;

def rules:
  if type != "array" or length == 0 then
    error("ruleset rules must be a non-empty array")
  else
    map(rule)
    | if length != (map(.type) | unique | length) then
        error("ruleset contains duplicate rule types")
      else
        sort_by(.type)
      end
  end;

def bypass_actor:
  required_object("bypass actor"; ["actor_id", "actor_type", "bypass_mode"]; ["actor_id", "actor_type", "bypass_mode"])
  | {
      actor_id: (.actor_id | integer_at_least("bypass actor actor_id"; 1)),
      actor_type: (.actor_type | nonempty_string("bypass actor actor_type")),
      bypass_mode: (.bypass_mode | nonempty_string("bypass actor bypass_mode"))
    };

def bypass_actors:
  if type != "array" then error("ruleset bypass_actors must be an array")
  else map(bypass_actor) | sort_by(.actor_type, .actor_id, .bypass_mode)
  end;

def conditions:
  required_object("ruleset conditions"; ["ref_name"]; ["ref_name"])
  | {
      ref_name: (
        .ref_name
        | required_object("ref_name conditions"; ["include", "exclude"]; ["include", "exclude"])
        | {
            include: (.include | string_array("ref_name include"; true)),
            exclude: (.exclude | string_array("ref_name exclude"; false))
          }
      )
    };

required_object("ruleset"; [
  "name", "target", "enforcement", "bypass_actors", "conditions", "rules",
  "id", "node_id", "source", "source_type", "_links", "created_at", "updated_at",
  "current_user_can_bypass"
]; ["name", "target", "enforcement", "bypass_actors", "conditions", "rules"])
| {
    name: (.name | nonempty_string("ruleset name")),
    target: (.target | nonempty_string("ruleset target")),
    enforcement: (.enforcement | nonempty_string("ruleset enforcement")),
    bypass_actors: (.bypass_actors | bypass_actors),
    conditions: (.conditions | conditions),
    rules: (.rules | rules)
  }
