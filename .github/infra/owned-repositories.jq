if type == "array"
    and all(.[]; type == "array")
    and all(flatten(1)[];
      type == "object"
      and (.full_name | type) == "string"
      and (.full_name | length) > 0
      and (.fork | type) == "boolean"
      and (.owner | type) == "object"
      and (.owner.login | type) == "string"
      and (.full_name | ascii_downcase | startswith(($owner | ascii_downcase) + "/")))
    and all(flatten(1)[]; (.owner.login | ascii_downcase) == ($owner | ascii_downcase)) then
  [flatten(1)[] | select(.fork | not) | (.full_name | ascii_downcase)]
  | unique
  | if length > 0 then join("\n") else error("owned repository inventory is empty") end
else
  error("unexpected paginated repository response")
end
