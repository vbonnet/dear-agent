// Package vroomgate holds the single canonical list of human-gated beads: work
// that must never be handed to an autonomous VROOM worker.
//
// It exists because the list used to be duplicated per binary. vroom-dispatch-direct
// and vroom-prompt-gen each carried their own copy, and the copies drifted — a bead
// gated in the dispatcher was still materialised as a prompt file by the generator,
// so gating one entry point left the other wide open. A gate that only some of the
// pipeline honours is not a gate. Everything on the VROOM dispatch path consults
// this package instead of a local map.
package vroomgate

import "sort"

// gated lists beads that must never be auto-dispatched to a worker: they require
// a human in the loop (credential rotation, backups, repointing live skills,
// destructive ops) or are otherwise gated by operator decision.
var gated = map[string]bool{
	"ce-pqha":    true,
	"ce-8qi":     true,
	"ce-kgd":     true,
	"ce-9uo":     true,
	"ce-xulg.14": true,
	"ce-126c":    true,
	"ce-cd14":    true,
	"ce-cd14.2":  true,
	"ce-cd14.1":  true,
	"ce-rrry":    true,
	"ce-clm6":    true,
	"ce-6as.10":  true, // interactive Gmail OAuth re-consent — HUMAN-ACTION, needs a human at the browser

	// Gated for the 2026-07-16 overnight unattended dispatch run (operator
	// decision): each is either core mesh/spawn/install infra whose breakage
	// would take dispatch itself down, or touches a security-sensitive surface
	// (fsguard, credentials, write-guard, OAuth) that needs a human review pass
	// rather than an unattended merge. Revisit and lift individually once a
	// human has reviewed the fix, rather than clearing the whole batch at once.
	"ce-nq2r":    true, // dispatch-owner crash detection — architectural, being handled directly tonight
	"ce-zp4c":    true, // non-atomic install cp — core install tooling
	"ce-24f1":    true, // find/guard go/bin wipe — core toolchain safety investigation
	"ce-bz3w":    true, // go/bin wipe incident — same core toolchain surface as ce-24f1
	"ce-cknn":    true, // mesh OAuth apiKeyHelper auto-refresh — auth/architectural
	"ce-fmxv":    true, // agm session new hang / spawn path — core spawn infra
	"ce-7ep9":    true, // agm.sock deletion under running tmux — core spawn infra
	"ce-3knl.10": true, // fail-closed AI review merge gate — governance/security
	"ce-3knl.3":  true, // FSGUARD gaps — security
	"ce-3knl.2":  true, // credential handling in logs/panes — security
	"ce-3knl.1":  true, // global git hooks / worktree sweep isolation — host-wide safety surface
	"ce-3knl":    true, // epic wrapper for the above
	"ce-5iv2":    true, // nightly go-install clobbers agm-mcp-server — core toolchain
	"ce-w77v":    true, // non-atomic in-place go install SIGKILLs agm — core toolchain
	"ce-2n5j":    true, // mesh recovery harness/provider selection — architectural
	"ce-ychx":    true, // control-plane single-provider dependency — architectural
	"ce-mazv":    true, // post-merge deploy-verify hook — deploy pipeline safety surface
	"ce-i5ru":    true, // spawn preflight VerifyAncestry — core spawn infra
	"ce-uxju":    true, // sandbox-dir GC auto-reap — host stability surface (2.3T leak history)
	"ce-q172":    true, // write-guard vroom carveout — security
	"ce-m80x":    true, // FD exhaustion crashing Dolt — host stability surface
	"ce-93lw.3":  true, // fsguard tokenizer fail-open — security
	"ce-93lw.1":  true, // raw shell-string interpolation — security
	"ce-93lw":    true, // epic wrapper for the above
	"ce-wcmz":    true, // safety-guard false positives — core safety-guard logic
	"ce-de4v":    true, // OAuth token hot-reload — auth
	"ce-77ip":    true, // unified OAuth lifecycle epic — architectural
	"ce-py3x":    true, // human_typing pane-state guard invert — core safety-guard logic
}

// IsHumanGated reports whether the bead id must be driven by a human and never
// handed to an autonomous worker. An unknown id is not gated: the gate is an
// explicit deny list layered on top of the normal eligibility filters.
func IsHumanGated(id string) bool { return gated[id] }

// IDs returns every human-gated bead id, sorted, so callers (tests, audits,
// operator reports) can enumerate the gate without duplicating its contents.
func IDs() []string {
	out := make([]string, 0, len(gated))
	for id := range gated {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
