// Package buildauthority stages the fixed AGM and disk-watchdog pair from
// explicitly authenticated tools, source, dependencies, and private state.
//
// The package stops before installation or activation. Its single public
// staging transaction returns either a sealed verified pair or a typed refusal;
// it never returns a partial artifact capability.
package buildauthority
