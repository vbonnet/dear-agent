# ADR-014: Plugin System for Composable Extensibility

Status: Accepted (2026-05-03; verified 2026-07-17)

Today's extensibility surfaces — `workflow.Hooks`, `audit.Registry`,
`eventbus.Bus` — were each added when their consumer needed them and do not
share discovery, manifesting, or activation. A third-party "PII scanner"
that wants an `OnEnforce` hook *and* an audit check has to wire two
surfaces by hand against the binary. `Hooks.OnEnforce` is also a single
callback, so two plugins cannot coexist without a hand-merge.

Introduce `pkg/plugin/` as a unifying composition layer over the existing
surfaces. The runtime gains no new capability; the system gains one name
(`Plugin`) and one discovery mechanism (filesystem manifests + compiled-in
registration).

- **`Plugin` is `Manifest()` plus optional capability interfaces.** Phase 1
  ships `HookProvider` and `CheckProvider`; `EventSubscriber`,
  `NodeKindProvider`, `SourceAdapter` are reserved names that slot in
  additively.
- **`Registry.Hooks()` composes every `HookProvider` into one `workflow.Hooks`.**
  Audit-hook errors accumulate via `errors.Join`; enforce errors fail the
  node, matching today's single-callback semantics. `ApplyChecks` threads
  the plugin name through `audit.Registry.Register` so a duplicate-ID error
  names the colliding plugin.
- **Two activation paths: compiled-in and manifest-discovered.** A
  `plugin.yaml` in `~/.config/dear-agent/plugins/` or per-repo
  `.dear-agent/plugins/` only *enables* and *configures* code the binary
  already has. The split is the security boundary — the manifest does not
  grant trust.
- **No untrusted code in v1.** No `.so` (Go's `plugin.Open` is ABI-fragile),
  no WASM (host-call surface is not stable enough yet), no subprocess RPC.
  Phase 1 trust model: "code is trusted iff linked into the binary."
  Subprocess RPC is its own ADR.
- **Permissions are advertised in the manifest, enforced by the substrate**
  (the bounded-execution decision in [ADR-010](ADR-010-workflow-engine-architecture.md)). The plugin
  package is the declaration site; `PermissionEnforcer` is the enforcer.

Builds on [ADR-010](ADR-010-workflow-engine-architecture.md) and
[ADR-011](ADR-011-dear-audit-subsystem.md). The bet: HookProvider +
CheckProvider cover most demand; reserving the capability names in the
manifest schema (`pkg/plugin/manifest.go`) means adding a third capability
is small.
