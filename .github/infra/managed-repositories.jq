include "identity-segment";

if type != "object"
    or ((keys_unsorted | sort) != (["active", "archived", "owner"] | sort))
    or (.owner | is_identity_segment | not)
    or (.active | type) != "object"
    or (.archived | type) != "object"
    or (all(.active[]; type == "object") | not)
    or (all(.archived[]; type == "object") | not)
    or (all(.active | keys[]; is_identity_segment) | not)
    or (all(.archived | keys[]; is_identity_segment) | not)
    or (.active | has("dear-agent") | not) then
  error("managed repository inventory is malformed or omits dear-agent")
else
  (.owner | ascii_downcase) as $owner
  | (.active | keys) as $active
  | (.archived | keys) as $archived
  | ($active | map(ascii_downcase)) as $active_normalized
  | ($archived | map(ascii_downcase)) as $archived_normalized
  | if ($active_normalized | length) != ($active_normalized | unique | length)
      or ($archived_normalized | length) != ($archived_normalized | unique | length)
      or any($active_normalized[]; . as $repo | ($archived_normalized | index($repo)) != null) then
      error("managed repository inventory contains duplicate or overlapping case-insensitive identities")
    else
      [($active_normalized + $archived_normalized)[] | ($owner + "/" + .)]
      | sort
      | join("\n")
    end
end
