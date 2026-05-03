# The Substrate Diagnostic

A five-question architectural lint for dear-agent components. Every component
that holds work-related state — sessions, manifests, beads, phase artifacts,
reservations, the message queue, agent cards, anything that another agent or
session has to reason about later — should be able to answer all five.

This is not a checklist of features to add. It is a way to surface design
debt: components that score poorly accumulate it, and the work shows up later
as agent reliability problems.

The framing comes from the substrate hypothesis: agents need durable state
outside the context window, and the tools that already provide durable state
to humans (issue trackers, source control, CRMs) are the ones agents end up
running on. See
[../../research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md](../../research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md)
for the full argument.

---

## The Five Questions

### 1. Does this component have records, or does it mostly have content?

A **record** is an addressable unit of state with a stable ID. You can
reference it, link to it, append to its history, and it will still be there
tomorrow. A pile of **content** (a transcript, a chat thread, a free-form
document) might contain the same information, but you cannot reliably point
agents at "the third paragraph of yesterday's Slack discussion" and expect
deterministic behavior.

**Pass:** the component exposes objects with stable identifiers that survive
restarts, agent handoffs, and process death.
**Fail:** the relevant state lives only in a log file, a context window, or a
free-text field.

### 2. Does it have a state machine, or does it just have labels?

A **state machine** has a defined set of states, defined transitions, and
rules about which transitions are legal from where. It can answer the
question *"what is this allowed to do next?"*. **Labels** are tags applied
freely; they describe but do not constrain. A label of `done` means whatever
the writer thought it meant.

**Pass:** transitions are defined, illegal transitions are rejected (or at
least flagged), and the current state is queryable.
**Fail:** state is inferred from string fields, presence/absence of files, or
"whatever the last hook wrote."

### 3. Is ownership an explicit field, or is it inferred from conversation?

**Explicit ownership** means there is one place to look to find out who is
responsible for this thing right now. Not "whoever last commented", not
"whoever the project lead probably is", not "the agent currently in the
sandbox". A field. With a value. That changes via a defined verb when
ownership transfers.

**Pass:** ownership is a single field with a defined transfer verb (assign,
hand off, claim, release).
**Fail:** ownership is inferred from session ID, message author, file
locking behavior, or human memory.

### 4. Are the verbs structural, or are they conversational?

**Structural verbs** change the state of the system in a defined way:
*create*, *assign*, *resolve*, *block*, *unblock*, *request review*,
*archive*. Each one has a name, a precondition, an effect, and shows up in
the audit trail. **Conversational verbs** look like *comment*, *reply*,
*mention*, *send a message* — they add content but not structure. Both have
their place. The trap is when conversational verbs are doing the work that
structural verbs should do.

**Pass:** the operations that change ownership, state, or scope have named
verbs and are visible in the audit log as discrete events.
**Fail:** a comment that contains the words "I'll take this" is what
actually transfers ownership.

### 5. Is the history queryable, or is it just visible?

**Queryable** means another agent can ask the system "what changed between
T1 and T2?" or "who last advanced this past gate G?" and get a structured
answer. **Visible** means a human can scroll through and see, but
programmatic access is brittle (parse a YAML file, scrape a tmux pane,
diff two snapshots).

**Pass:** history is exposed via a typed API, with stable event names and
deterministic ordering.
**Fail:** history is reconstructable only by reading log lines or rendered UI.

---

## Worked Examples — dear-agent Components

| Component | Records | State machine | Explicit ownership | Structural verbs | Queryable history |
|---|---|---|---|---|---|
| **AGM session** | ✅ Manifest + Dolt row, stable UUID | ✅ `READY/THINKING/COMPACTING/OFFLINE` | ✅ Harness + Claude UUID binding | ✅ `new/resume/archive/kill/associate/send/compact` | ✅ Dolt versioned SQL + event bus |
| **AGM work item** (the *job* the session is doing, distinct from the session itself) | ⚠️ Implicit; lives in goals, beads, phase artifacts | ⚠️ Not a single state machine — split across Wayfinder + session lifecycle | ❌ No first-class field; inferred from "which session is active" | ⚠️ Verbs at session level, not work-item level | ⚠️ Reconstructable from beads + commits + phase artifacts, not as one stream |
| **Engram memory** | ✅ Cue-tagged entries, frontmatter schema | ⚠️ Memories don't transition; that's correct for a knowledge store | ⚠️ Authorship is captured, ownership-of-action isn't the model | ✅ `create-bead`, ecphory queries, retrieval cues | ✅ Hippocampus index + validators |
| **Wayfinder phase** | ✅ Phase artifacts | ✅✅ This *is* a 9-state machine with gates — the model component | ⚠️ Phase ownership = active session, implicitly | ✅ Phase advance, gate check, prep, review | ⚠️ Transitions logged; cross-phase work timeline less structured |
| **File reservations** | ✅ Reservation records | ⚠️ Held / released; advisory not enforced | ✅ Holder is recorded | ✅ `reserve` / `release` | ✅ Reservation log |
| **Pending messages** | ✅ Files in `~/.agm/pending/{session}/` | ✅ Pending → delivered (or expired) | ✅ Target session | ✅ `send`, deliver-on-READY | ⚠️ Delivery is logged; reasoning about why a message was queued is not |
| **Beads** | ✅ Frontmatter + body | ⚠️ Created / archived; not a workflow state | ✅ Author | ✅ `create-bead` | ✅ Engram retrieval |

### What this surfaces

- **AGM work item** is the only red row. dear-agent's *session* substrate is
  strong; its *work-item* substrate is weak, because work items aren't yet a
  first-class object. They live as the implicit subject of a session, the
  goal field of a manifest, the topic of a bead, or the artifact of a
  Wayfinder phase. This is the substrate gap the
  [research analysis](../../research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md)
  identifies.
- **Wayfinder** is the strongest existing state machine in the system and the
  closest analogue to an issue tracker. If dear-agent were to make a single
  move to close the work-item gap, it would be to expose Wayfinder phase
  state as a queryable board.

## How to Use This Diagnostic

- **At design time.** When proposing a new component (ADR, design doc, RFC),
  answer all five questions in a section titled *Substrate Diagnostic*. If
  any answer is "fail" or "we're not sure", call that out and justify it.
- **At review time.** When reviewing a PR that adds or changes a stateful
  component, ask the five questions. Failing answers are not a blocker — they
  are signal that future agent operations against this component will be
  unreliable, and a deliberate decision should be recorded.
- **At audit time.** Periodic architecture review walks through the table
  above. Components whose answers worsen over time (e.g., ownership becoming
  more inferred as features accumulate) get refactor priority.

## What This Diagnostic Is *Not*

- It is not a substitute for product judgment. A "fail" answer can be the
  right answer for a deliberately lightweight component.
- It is not a feature checklist. The point is the *property*, not a specific
  implementation. A YAML file with a stable schema can be a record; a SQL
  table without one isn't.
- It is not a replacement for DEAR. DEAR governs *how* you maintain the
  substrate (Define → Enforce → Audit → Resolve). The diagnostic governs
  *whether the substrate is structured well enough to maintain at all.*
