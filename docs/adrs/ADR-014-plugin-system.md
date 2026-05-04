# ADR-014: Plugin System for Composable Extensibility

**Status**: Proposed
**Date**: 2026-05-03
**Context**: dear-agent's value proposition is methodology-first: users
should be able to bring whatever models, harnesses, audit checks, and
workflow node types they prefer. Today's extensibility is *structurally
compose-out*: each extension surface (`pkg/workflow.Hooks`,
`pkg/audit.Registry`, `pkg/eventbus.Bus`) was added when its consumer
needed it, and they don't share discovery, manifesting, or activation
mechanics. A user who wants to ship a "PII scanner" that participates
in the workflow's `OnEnforce` hook *and* registers an audit check *and*
subscribes to telemetry has to wire each surface by hand against the
binary.

This ADR introduces `pkg/plugin` — a unifying composition layer over the
existing surfaces. It adds nothing the runtime could not already do
imperatively, but it gives a single name (`Plugin`) and a single discovery
mechanism (filesystem manifests + compiled-in registration) to the things
users will want to compose.

Builds on:
- [ADR-009: Work Item as First-Class Substrate](ADR-009-work-item-as-first-class-substrate.md)
- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md)

---

## Context

The existing extensibility surfaces in dear-agent are:

| Surface | What it extends | Discovery | Composition |
|---|---|---|---|
| `pkg/workflow.Hooks` (DEAR hooks) | Workflow lifecycle (Define/Enforce/Audit/Resolve) | None — caller wires `*Hooks` on the Runner | Single callback per phase; second hook overwrites first |
| `pkg/audit.Registry` (audit checks) | Audit catalogue | `init()`-time `Register()` calls | Idempotent registry; duplicates rejected |
| `pkg/eventbus.Bus` (events) | Telemetry/notification/audit/heartbeat events | Caller adds sinks/subscribers | Multi-subscriber via wildcard patterns |

Each works in isolation, but a third party who wants to ship a single
package containing all three must write three different wirings — one
that mutates `*workflow.Hooks`, one that calls `audit.Default.Register`,
and one that calls `bus.Subscribe`. There is no way to ask the system
"what extensions are loaded right now?" or "is this plugin enabled in
this repo?". Users who want to *configure* an extension differently in
two repos have nowhere to put that configuration.

Three concrete shortcomings drive this ADR:

1. **DEAR hooks are single-callback, not composable.** A `*Hooks` value
   has exactly one `OnEnforce func`. Two plugins that both want to
   participate in `OnEnforce` cannot coexist without a hand-written
   merge.
2. **Audit checks register at `init()` only.** The current registry is
   technically dynamic (`Register` is exported), but every existing
   caller registers from `init()`. There is no path for a manifest-driven
   plugin to declare "load these checks for this repo" without recompiling.
3. **No shared manifest or discovery surface.** A user who wants to know
   what extensions are active has nothing to read; a CI policy that wants
   to gate "no unapproved plugins" has nothing to enforce.

This ADR fills those gaps without replacing the underlying surfaces.

---

## Decision

Introduce a new package `pkg/plugin/` that provides:

### D1. The `Plugin` interface and capability sub-interfaces

```go
type Plugin interface {
    Manifest() Manifest
}

type HookProvider interface {
    Plugin
    Hooks() workflow.Hooks
}

type CheckProvider interface {
    Plugin
    Checks() []audit.Check
}
```

A plugin is a Go value that returns a `Manifest` describing itself and
optionally implements one or more capability interfaces. Capability
interfaces are detected via Go type assertions; a plugin that implements
none is *valid* (a metadata-only entry — useful for declaring intent
to load a future capability).

Phase 1 ships the two capability interfaces above. `EventSubscriber`,
`NodeKindProvider`, `SourceAdapter` are reserved names for later
phases — see "Out of scope" below.

### D2. `Manifest` is the discoverable contract

The manifest is the single source of truth for a plugin's identity,
capabilities, and permissions. It is the only field every plugin must
return:

```go
type Manifest struct {
    APIVersion   string         // "dear-agent.io/v1"
    Kind         string         // "Plugin"
    Name         string         // dot-separated identifier
    Version      string         // semver
    Description  string
    Author       string
    Capabilities []Capability   // declared subset of {hooks, checks, ...}
    Permissions  Permissions    // see D5
    Config       map[string]any // free-form per-plugin config
}
```

Naming uses dot-separated lowercase (matching `audit.CheckMeta.ID`):
`plugin.example.pii-scanner`, not `MyPiiScanner`. Validate rejects
empty `Name`, malformed `APIVersion`, unknown `Capability` values,
and control characters in identifiers — same rules `audit.CheckMeta.Validate`
applies to check IDs.

### D3. `Registry` composes plugins into runtime values

The registry is the single hub:

```go
type Registry struct { /* ... */ }

func (r *Registry) Register(p Plugin) error
func (r *Registry) Plugins() []Plugin
func (r *Registry) Hooks() workflow.Hooks       // composes all HookProviders
func (r *Registry) ApplyChecks(*audit.Registry) error
```

`Hooks()` returns a `workflow.Hooks` whose four callbacks fan out to
every registered `HookProvider` in registration order. Each provider's
return is captured: an `OnEnforce` error from any provider fails the
node (matching today's single-callback semantics); audit-hook errors
are accumulated and returned as a multi-error so the runner can log
all of them but never blocks the run. This preserves the
[ADR-010 §D3 substrate guarantee](ADR-010-workflow-engine-architecture.md):
audit emission is unconditional.

`ApplyChecks` is the bridge to `pkg/audit.Registry`: it walks every
`CheckProvider`, registers each `audit.Check` through the existing
`Registry.Register` API, and returns the first conflict it finds.
Duplicate IDs are rejected by `audit.Registry` already; this just
threads the plugin's name through the error so operators can tell which
plugin's check collided.

### D4. Two activation paths: compiled-in and manifest-discovered

```
        ┌─────────────────────────────────────────┐
        │             *plugin.Registry            │
        └──────────┬──────────────────┬───────────┘
                   │                  │
       Register(p) │                  │ LoadFromDir(path)
       (built-in)  │                  │ (filesystem)
                   │                  │
                   ▼                  ▼
            Go-value plugin    Manifest matched against
            (compiled in)      registered factories;
                               unknown name → error
```

**Compiled-in.** A built-in plugin is a Go value the host binary
constructs and passes to `registry.Register`. This is the path
first-party plugins use (the bundled audit checks, the future HITL
backends, etc.), and the path shared `dear-agent` integrations use
when distributed as Go modules. Trust = same as any other linked code.

**Manifest-discovered.** The `FilesystemLoader` scans a directory
(`~/.config/dear-agent/plugins/`, per-repo `.dear-agent/plugins/`,
or `$DEAR_AGENT_PLUGIN_DIR`) for `plugin.yaml` files. For each, it
loads the manifest and *enables* a previously-registered Go value
whose `Manifest().Name` matches. This is the same pattern Claude Code
uses for MCP server config: the manifest declares "use this plugin in
this repo, with this config", but the executable code must already be
known to the binary. Phase 1 deliberately does not load arbitrary `.so`
files, WASM modules, or subprocess RPC — see D6.

The split is the security boundary: only code the operator already
trusted enough to compile in can run; the manifest only governs *which
of those* runs in *which repo* and *with what config*.

### D5. Permissions are advertised in the manifest, enforced by the substrate

Every plugin's manifest declares the permissions it expects:

```yaml
permissions:
  fs_read:  ["docs/**"]
  fs_write: []
  network:  false
  tools:    ["audit.govulncheck"]
```

These piggy-back on the [ADR-010 §D5 permissions model](ADR-010-workflow-engine-architecture.md):
when a plugin participates in node execution (e.g. via `OnEnforce`),
its declared permissions are unioned into the node's permissions for
the duration of the call. The existing `PermissionEnforcer` is the
enforcer; `pkg/plugin` is just the declaration site.

Permissions in v1 are advisory for plugins that *only* register checks
(audit checks already run with the audit runner's permissions). They
become load-bearing once a plugin participates in workflow node
execution, which the hook surface allows today.

### D6. Phase 1 does not load untrusted code

Out of scope for v1:

- **Subprocess plugins** (RPC over a Unix socket / stdin-stdout JSON).
  Defensible later, but the protocol is its own ADR.
- **WASM plugins.** Sandboxing story is appealing but the host-call
  surface (`workflow.AuditEvent`, `audit.Result`) is not stable enough
  yet to commit to a WASM ABI.
- **`.so` / `plugin.Open`.** Go's `plugin` package is famously
  ABI-fragile (only works against the exact same binary build); we
  reject it explicitly to avoid the support tax.

The Phase 1 trust model is: "code is trusted iff it is linked into the
binary." Manifests can disable, configure, or scope plugins — they
cannot grant trust.

### D7. Surfaces in Phase 1: HookProvider + CheckProvider

Phase 1 ships exactly two capability interfaces:

- `HookProvider` — composes into `workflow.Hooks`.
- `CheckProvider` — registers into `audit.Registry`.

These are the two surfaces with the highest pent-up demand and the
clearest existing primitives. They cover the "PII scanner that hooks
enforce + adds an audit check" use case the context section calls out.

Reserved (not implemented):

- `EventSubscriber` — wraps `eventbus.Bus.Subscribe`. Trivial to add
  in Phase 2 once we have a real plugin in tree wanting it.
- `NodeKindProvider` — registers a new `Kind` value into the runner's
  dispatch. Requires refactoring the hardcoded `switch` in
  `pkg/workflow/runner.go`; that is its own (small) ADR.
- `SourceAdapter` — extends `pkg/source` from
  [ADR-010 §D9](ADR-010-workflow-engine-architecture.md). Phase 3 of
  the workflow engine plan already covers `pkg/source`; the plugin
  surface there will land alongside it.

### D8. Naming and config conventions

- Package: `pkg/plugin/` (singular). The `Plugin` *type* is the
  composition surface; *each* registered plugin is a `Plugin`.
- Manifest filename: `plugin.yaml`. One per directory.
- Config search order (first found wins, mirrors
  [ADR-010 §D4 roles.yaml lookup](ADR-010-workflow-engine-architecture.md)):
  1. `$DEAR_AGENT_PLUGIN_DIR` (env var path)
  2. `./.dear-agent/plugins/` (per-repo)
  3. `~/.config/dear-agent/plugins/` (per-user)
- Plugin name namespace: dot-separated lowercase. First-party plugins
  use the `dear-agent.<area>.<name>` namespace
  (`dear-agent.audit.license-header`); third-party plugins use a
  reverse-DNS or org-scoped namespace
  (`com.example.pii-scanner`, `acme.guardrail`).

---

## Architecture diagram

```
                    Caller (binary main)
                              │
                              │ 1. construct compiled-in plugins
                              │    registry.Register(p)
                              ▼
                  ┌──────────────────────────┐
                  │     plugin.Registry      │
                  │                          │
                  │   ┌──────────────────┐   │   2. read filesystem manifests
                  │   │ FilesystemLoader │◀──┼─── ~/.config/dear-agent/plugins/
                  │   └──────────────────┘   │       per-repo .dear-agent/plugins/
                  │                          │
                  │   compose hook providers │
                  │   collect check providers│
                  └──────────┬───────┬───────┘
                             │       │
            registry.Hooks() │       │ registry.ApplyChecks(audit.Default)
                             ▼       ▼
              ┌───────────────────┐ ┌────────────────────┐
              │ workflow.Runner   │ │  audit.Registry    │
              │  *workflow.Hooks  │ │  + plugin checks   │
              └───────────────────┘ └────────────────────┘
                       │                       │
                       ▼                       ▼
                 (executes nodes)       (runs audit checks)
                 calls hooks per         per CheckMeta
                 ADR-010 D3 audit         cadence per ADR-011
```

---

## What changes

| File | Change |
|---|---|
| `pkg/plugin/doc.go` | New: package doc, ADR cross-reference. |
| `pkg/plugin/plugin.go` | New: `Plugin`, `HookProvider`, `CheckProvider` interfaces. |
| `pkg/plugin/manifest.go` | New: `Manifest`, `Capability`, `Permissions` types + `LoadManifest(path)`. |
| `pkg/plugin/registry.go` | New: `Registry`, `Register`, `Plugins`, `Hooks`, `ApplyChecks`. |
| `pkg/plugin/loader.go` | New: `FilesystemLoader.LoadFromDir(path) ([]Manifest, error)`. |
| `pkg/plugin/*_test.go` | New: tests for every public surface. |
| `docs/adrs/ADR-014-plugin-system.md` | This ADR. |

## What does **not** change

- `pkg/workflow/hooks.go` — same `Hooks` struct, same payloads. The
  registry returns a `workflow.Hooks` value the runner cannot tell
  apart from a hand-built one.
- `pkg/audit/registry.go` — same `Registry`, same `Check` interface.
  The plugin registry calls the existing `Register`.
- `pkg/eventbus/` — untouched in Phase 1.
- `pkg/workflow/runner.go` — untouched. The wiring point is the caller
  (whoever constructs `*Runner`); the runner itself does not import
  `pkg/plugin`.

---

## Storage and persistence

None. Plugins are runtime composition over already-persistent state
(audit findings → SQLite via existing checks; workflow events → SQLite
via existing audit sinks). The plugin system itself stores nothing.

The `plugin.yaml` files on disk are configuration, not state.

---

## Consequences

### Positive

- **Composable hooks.** Two plugins can each register an `OnEnforce`
  handler; both run on each node. Today's single-callback model
  becomes a special case (single `HookProvider`).
- **One declarative surface for "what's loaded?"** A `dear-agent
  plugins list` command (Phase 2) reads the registry and prints names,
  versions, and capabilities. CI/audit policies that want to gate
  unapproved plugins have a manifest to inspect.
- **Per-repo plugin scoping.** A `.dear-agent/plugins/` dir lets a
  repo enable a stricter plugin set than the user's global config,
  matching the existing `.dear-agent.yml` per-repo override pattern.
- **Cheap to build on.** New capability interfaces are additive: a
  plugin that gains a new method gets the new behaviour; old plugins
  keep working.
- **No new persistent surface.** No schema migration; no compatibility
  story to maintain across versions of the engine state.

### Negative / costs

- **Two activation paths is more concept than one.** "Compiled-in" vs.
  "manifest-enabled" is a real distinction users have to learn.
  Mitigated by the fact that first-party plugins are always both
  (registered at startup; manifest in `embed.FS`).
- **No third-party code execution in Phase 1.** A user who wants to
  drop in a binary must wait for Phase 2's subprocess protocol. The
  argument for this restriction is in D6; the cost is that the Phase 1
  story is "you can ship a Go module" rather than "drop a binary in".
- **Multi-error from `OnAudit` may surprise callers** who expect a
  single error. Mitigated by `errors.Is` / `errors.Join` standard-library
  semantics; the multi-error renders sensibly under `%w`.
- **Manifest validation is a new failure mode.** A bad `plugin.yaml`
  in a discovery directory fails the loader; we mitigate by making
  `FilesystemLoader.LoadFromDir` continue on individual errors and
  return them as a slice, so one bad plugin does not disable the rest.

### Neutral

- **`Manifest.Config` is `map[string]any`.** Loose typing on purpose
  for v1 — plugins type-assert / decode into their own config struct.
  We may add a typed-config helper later (`UnmarshalConfig(target any)`)
  but the v1 surface stays minimal.
- **Discovery directories are *additive*, not overlay.** A plugin
  manifest in a per-repo dir activates that plugin for the run; it
  does not override or shadow a global manifest of the same name.
  Duplicate names across discovery dirs are an error.

---

## Bets, ranked by stakes

**High stakes:**

1. **The HookProvider + CheckProvider pair covers most demand.** If
   the next plugin built wants something else (a new node kind, a
   new sink), Phase 1 is undersized. Hedged by reserving the names
   in D7 and accepting that adding a third capability interface is
   small.
2. **Manifests as YAML.** Mirrors workflow YAML and `roles.yaml`. If
   the project pivots to TOML or HCL, this is rewrite churn.

**Medium stakes:**

3. **Trust = compiled-in only.** Pragmatic Phase 1 cut; the cost is
   third parties have to wait for Phase 2 to ship.
4. **Multi-error semantics on hook fan-out.** Hedged by `errors.Join`
   being stdlib; consumers pattern-match unchanged.

**Low stakes:** the exact discovery directory order; whether `Capability`
is a string or an enum; the namespace convention for plugin names.

---

## Alternatives considered

| Alternative | Why rejected |
|---|---|
| **Extend `workflow.Hooks` to a `[]Hook` slice in place.** | Touches the substrate-critical runner contract; breaks every existing caller; conflates "the engine accepts hooks" with "the system has plugins". |
| **Make `pkg/audit.Registry` the plugin registry.** | Audit is one capability, not the universe. Coupling plugin discovery to the audit package would force every future capability to depend on audit. |
| **Subprocess RPC plugins in Phase 1.** | Protocol design + sandbox design is its own ADR-sized work; would block Phase 1. |
| **Go `plugin.Open` (.so).** | ABI brittleness, build-flag fragility, no cross-compile story. The known-bad option. |
| **HermesAgent-style learning-loop plugins as the first surface.** | Rejected as Phase 1: the substrate primitives (audit replay, beads write-back) are still being built. The learning-loop *uses* the plugin system; it isn't the plugin system. |
| **No registry — plugins call `audit.Default.Register` and mutate `*workflow.Hooks` themselves.** | Status quo. Has the three problems in the Context section. |

---

## Open questions

These are deferred to later ADRs or later phase reviews:

1. **Subprocess RPC protocol.** Likely JSON-RPC over Unix sockets,
   modelled on MCP. Belongs to its own ADR once we have a Phase 2
   plugin in tree that needs it.
2. **Plugin lifecycle hooks** (`OnLoad`, `OnUnload`, `OnReload`). Phase 1
   has no need; if hot-reload becomes a Phase 4 deliverable, the
   surface lives here.
3. **Plugin dependency graph.** Two plugins where one depends on
   another's check existing. Punt: Phase 1 rejects ordering across
   plugins; an `OnEnforce` chain is "registration order".
4. **Manifest schema versioning policy.** When `Manifest` gains a
   field, do we bump `apiVersion` to `v2` or treat it as additive?
   The "Dolt-compatible additive" bias from
   [ADR-010 §Open question 5](ADR-010-workflow-engine-architecture.md)
   suggests additive; written down here when the first migration hits.
5. **Per-plugin telemetry.** Should `Registry.Hooks()` annotate every
   audit event with the originating plugin? Useful for "which plugin
   wrote this finding" debugging, but mixes plugin metadata into
   substrate data. Punted.

---

## Implementation plan

Phase 1 (this PR):

1. `pkg/plugin/plugin.go` — `Plugin`, `HookProvider`, `CheckProvider`
   interfaces.
2. `pkg/plugin/manifest.go` — `Manifest`, `Permissions`, `Capability`,
   `Validate`, YAML un/marshal.
3. `pkg/plugin/registry.go` — `Registry.Register`, `.Plugins`,
   `.Hooks`, `.ApplyChecks`.
4. `pkg/plugin/loader.go` — `FilesystemLoader.LoadFromDir` returning
   manifests + per-file errors.
5. Tests for every public function: registration idempotency, hook
   fan-out (including error accumulation), check duplication
   detection, manifest validation, loader's continue-on-error
   semantics.

Phase 2 (separate PR):

- `EventSubscriber` capability.
- `dear-agent plugins list` CLI subcommand.
- First in-tree plugin migrated onto the surface (candidate: license
  header check from `pkg/audit/checks/`).

Phase 3+ (separate ADRs):

- Subprocess RPC plugins.
- `NodeKindProvider` (post-runner-dispatch refactor).
- `SourceAdapter` (alongside Phase 3 of the workflow engine roadmap).

---

## Status

This ADR is **Proposed**, not Accepted.

Acceptance authorizes the Phase 1 work in this PR (the five files above
plus tests) and reserves the capability names in D7. Adding a third
capability interface in Phase 2 does not require a new ADR; replacing
the discovery model in D4 does.

---

## References

- [ADR-009 — Work Item as First-Class Substrate](ADR-009-work-item-as-first-class-substrate.md)
- [ADR-010 — Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
- [ADR-011 — DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md)
- `pkg/workflow/hooks.go` — single-callback DEAR hooks (Phase 1 input)
- `pkg/audit/registry.go` — idempotent check registry (Phase 1 bridge)
