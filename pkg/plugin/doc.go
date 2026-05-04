// Package plugin is dear-agent's composable extensibility layer.
//
// A Plugin is a Go value that returns a Manifest describing itself and
// optionally implements one or more capability interfaces (HookProvider,
// CheckProvider). The Registry composes plugins into the runtime values
// the rest of the system already consumes:
//
//   - Registry.Hooks() returns a workflow.Hooks whose four DEAR
//     callbacks fan out to every registered HookProvider.
//   - Registry.ApplyChecks(*audit.Registry) walks every CheckProvider
//     and registers each audit.Check via the existing audit registry.
//
// Plugins reach the Registry via two activation paths:
//
//   - Compiled-in: the host binary calls registry.Register(p) with a
//     constructed Go value. This is the path first-party plugins take
//     and the path third parties distributing as Go modules take.
//   - Manifest-discovered: a FilesystemLoader scans a plugin directory
//     (~/.config/dear-agent/plugins, per-repo .dear-agent/plugins, or
//     $DEAR_AGENT_PLUGIN_DIR) for plugin.yaml files and *enables*
//     previously-registered Go values whose Manifest().Name matches.
//     The manifest never grants trust on its own — only code already
//     linked into the binary can run.
//
// Phase 1 ships HookProvider and CheckProvider. EventSubscriber,
// NodeKindProvider, and SourceAdapter are reserved names for later
// phases; see ADR-014 §D7.
//
// See docs/adrs/ADR-014-plugin-system.md for the full design.
package plugin
