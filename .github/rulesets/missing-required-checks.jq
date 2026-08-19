def validate_check($label):
  if type != "object"
      or ((keys_unsorted | sort) != (["context", "integration_id"] | sort))
      or (.context | type) != "string"
      or (.context | length) == 0
      or (($label == "produced") and .integration_id == null)
      or ((.integration_id != null)
        and ((.integration_id | type) != "number"
          or .integration_id != (.integration_id | floor)
          or .integration_id <= 0)) then
    error($label + " contains malformed check identity evidence")
  else
    .
  end;

if type != "object"
    or ((keys_unsorted | sort) != (["expected", "produced"] | sort))
    or (.expected | type) != "array"
    or (.produced | type) != "array" then
  error("required-check evidence must contain expected and produced arrays")
else
  .expected |= map(validate_check("expected"))
  | .produced |= map(validate_check("produced"))
  | . as $evidence
  | [$evidence.expected[] as $required
      | select(($evidence.produced
          | any(.[];
              .context == $required.context
              and ($required.integration_id == null or .integration_id == $required.integration_id)))
        | not)
      | $required]
end
