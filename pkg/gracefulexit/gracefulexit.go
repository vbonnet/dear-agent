// Package gracefulexit holds the framework-level "no-overfit" guardrail
// that every dear-agent task inherits unless explicitly opted out.
//
// # The pattern
//
// Agents asked to search, research, or pattern-match tend to overshoot:
// faced with weak evidence, they manufacture a finding rather than
// report "nothing fits". Inflated results poison downstream consumers
// — a low-confidence "match" handed off to another agent is read as a
// high-confidence one, and the chain of stretched judgments compounds.
//
// The fix is to make "nothing fits" a first-class valid outcome and
// to publish that contract at the framework level, not per-prompt.
// Tasks should not need to remember to opt into honesty; they should
// have to opt out of it (and explain why) when they really do require
// a non-empty result.
//
// This package is the single source of truth for the guardrail text
// and the configuration that toggles it. Three call sites consume it:
//
//   - `agm new` prints Banner() at session start so every worker sees
//     the guardrail before composing its first response.
//   - The acceptance package recognises "graceful-exit" as a typed
//     criterion, so the DEAR Define phase can record the exit
//     condition alongside tests-pass and lint-clean.
//   - The audit subsystem can later add a check that flags inflated
//     findings (ADR-018, deferred Enforcement tier).
//
// # Why a default, not a per-prompt instruction
//
// The two repeated failure modes were:
//
//  1. The user (or upstream prompt) forgets to say "report nothing if
//     nothing fits", so the agent defaults to "find something".
//  2. The user does say it, but a downstream sub-agent invoked by the
//     first one doesn't inherit the instruction.
//
// Both are fixed by making the guardrail a framework-level default
// that travels with the session rather than the prompt. The
// .dear-agent.yml > framework-defaults > graceful-exit knob exists
// for the rare repo that really wants the opposite (e.g. a synthetic
// data generator that must always emit a row).
package gracefulexit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Guardrail is the canonical text published to every worker. Keep it
// short; this string is read by humans in a terminal banner and is
// also embedded verbatim in acceptance-criteria output, so it must
// fit on a few lines without wrapping awkwardly.
const Guardrail = "If nothing fits the criteria, report that. " +
	"\"Nothing found\" is a first-class valid outcome — " +
	"saying so is better than inflating a weak match to meet an imagined quota."

// Rationale is the longer-form explanation shown when a worker asks
// "why is this rule here?". It is NOT printed by default — Banner()
// uses Guardrail alone to keep session start uncluttered.
const Rationale = "Inflated findings poison downstream consumers: a low-confidence " +
	"match handed off to another agent is read as high-confidence, and the chain " +
	"of stretched judgments compounds. A graceful exit (\"nothing fits\") preserves " +
	"handoff signal."

// Applies enumerates the task kinds the guardrail is most relevant
// to. The list is informational — the framework applies the
// guardrail to ALL tasks by default — but a worker that wants to
// self-check "should I be especially careful here?" can consult it.
var Applies = []string{
	"search",         // codebase / web / corpus search
	"research",       // analysis, literature review, synthesis
	"pattern-match",  // grep-like discovery, dead code, duplicates
	"code-review",    // findings enumeration
	"backfill",       // ingestion, retrofit pipelines
	"recommendation", // suggestion / recommendation surfaces
}

// Config is the .dear-agent.yml > framework-defaults > graceful-exit
// section. Zero value means "use the default" (enabled).
//
// Disabling is a deliberate, scoped opt-out: the WHY field is
// required so future readers can tell whether the repo really needs
// the override or whether it was left over from a one-off experiment.
type Config struct {
	// Disabled turns the guardrail off entirely. The default (zero
	// value) is false, i.e. the guardrail is on.
	Disabled bool `yaml:"disabled,omitempty"`
	// Why explains the opt-out. Required when Disabled is true.
	Why string `yaml:"why,omitempty"`
}

// Enabled reports whether the guardrail applies in this repo.
func (c Config) Enabled() bool { return !c.Disabled }

// Validate checks the config for shape errors.
func (c Config) Validate() error {
	if c.Disabled && strings.TrimSpace(c.Why) == "" {
		return errors.New("graceful-exit: disabling the framework default requires a non-empty `why` field")
	}
	return nil
}

// file mirrors the slice of .dear-agent.yml the loader cares about.
// yaml.v3 silently ignores unknown keys, so this struct can stay
// narrow alongside the acceptance and audit loaders.
type file struct {
	FrameworkDefaults frameworkDefaults `yaml:"framework-defaults,omitempty"`
}

type frameworkDefaults struct {
	GracefulExit *Config `yaml:"graceful-exit,omitempty"`
}

// Load reads .dear-agent.yml from repoRoot and returns the
// graceful-exit configuration. A missing file or missing section is
// not an error — the caller gets the zero value, which is "enabled".
func Load(repoRoot string) (Config, error) {
	path := filepath.Join(repoRoot, ".dear-agent.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("gracefulexit: read %s: %w", path, err)
	}
	return ParseBytes(data)
}

// ParseBytes parses raw YAML bytes. Exposed for tests and hooks that
// already have the file content in memory.
func ParseBytes(data []byte) (Config, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Config{}, fmt.Errorf("gracefulexit: parse: %w", err)
	}
	cfg := Config{}
	if f.FrameworkDefaults.GracefulExit != nil {
		cfg = *f.FrameworkDefaults.GracefulExit
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Banner returns the multi-line banner printed at session start. The
// returned string is empty when the guardrail is disabled in this
// repo, so callers can unconditionally print it and rely on the
// loader's decision.
//
// The banner intentionally does not include Rationale — a worker who
// wants more can read this package or the design doc. The terminal
// banner is for the rule, not the essay.
func Banner(cfg Config) string {
	if !cfg.Enabled() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nFramework guardrail — graceful exit (no-overfit):\n")
	b.WriteString("  " + Guardrail + "\n")
	b.WriteString("  Applies to: " + strings.Join(Applies, ", ") + ".\n")
	b.WriteString("  To opt out for this repo, set framework-defaults.graceful-exit.disabled: true in .dear-agent.yml (a `why:` is required).\n")
	return b.String()
}
