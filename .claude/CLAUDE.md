# dear-agent — Project Instructions

## Output Routing — Where Artifacts Belong (MANDATORY)

This repo holds **code**, not research. Research artifacts (analysis docs,
transcripts, literature reviews, findings) belong in `engram-research`.
Conversation logs belong in `ai-conversation-logs`. Routing is governed by
`.dear-agent.yml` at the repo root — read it once at the start of any
session that produces artifacts.

**Forbidden in dear-agent** (declared by `.dear-agent.yml > forbidden-paths`):
- New `*.md` or `*.txt` files under `research/`. dear-agent does not
  currently have a `research/` tree, and any such file should be redirected
  to `~/src/engram-research`.

**Where things go:**

| Artifact kind                                              | Destination                  |
|------------------------------------------------------------|------------------------------|
| Source code, ADRs (`docs/adrs/`), design docs (`docs/design/`) | this repo                |
| Research analysis (substrate/architecture studies, etc.)   | `~/src/engram-research`      |
| Source transcripts (YouTube, podcasts, interviews)         | `~/src/engram-research`      |
| Conversation/session logs                                  | `~/src/ai-conversation-logs` |

**Decision procedure** when writing a new file:
1. If it is code, build config, ADR, or design doc that constrains code in
   this repo → write here.
2. Otherwise check `.dear-agent.yml > output-dirs` for the matching kind and
   write there instead.
3. If unsure, ask the user — do **not** default to `research/` in this repo.

This rule exists because research artifacts were committed to the predecessor
code repo (ai-tools) in error multiple times, polluting code-repo history and
stranding work away from the corpus where it belongs. Treat the redirect as
authoritative.

See [AGENTS.why.md](../AGENTS.why.md) for the rationale behind the two-tier
(instruction + configuration) routing model.
