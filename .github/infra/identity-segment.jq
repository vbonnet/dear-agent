# The one grammar for a repository identity segment, shared by every inventory
# validator (.github/infra/managed-repositories.jq and infra/import.sh).
#
# GitHub repository and owner names are limited to ASCII alphanumerics, ".",
# "-", and "_". Anchoring to that grammar rejects the segments that break the
# consumers rather than merely looking odd: an embedded newline splits one
# `jq -r` record into two repositories, "/" produces a bogus owner/name slug,
# and a quote produces an invalid OpenTofu state address or provider import ID.
def is_identity_segment:
  type == "string" and test("^[A-Za-z0-9][A-Za-z0-9._-]*$");
