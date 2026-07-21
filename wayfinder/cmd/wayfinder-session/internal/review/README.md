# Wayfinder review adapter

The review package selects risk-appropriate personas and parses structured
review results for phases that request provider-backed document review.

Deterministic phase gates remain authoritative. A provider review cannot make
an invalid transition valid, and unavailable or malformed review output must
not be converted into a passing result.

Key entry points:

- `DetectPersonas`: select relevant review perspectives;
- `GetTierConfig`: resolve risk and phase policy;
- `NewMultiPersonaGate`: execute and aggregate configured reviews;
- `ParseReviewResult`: validate structured output.

Normative behavior is in [SPEC.md](SPEC.md). Package tests cover persona
selection, risk adaptation, parsing, and failure behavior.
