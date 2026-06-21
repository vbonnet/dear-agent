// Package safegit flakevalve.go implements the P4 flake valve: checks listed
// under flaky_checks in .safe-merge.yml get a sanctioned number of reruns before
// their failure is treated as real. The valve is deliberately narrow:
//
//   - A failing check that is NOT named flaky is a real block, immediately.
//   - A failing flaky check with retries remaining is rerun (gh run rerun
//     --failed) and reported as retryable; watch mode waits for the rerun.
//   - A flaky check that has already used its retries is a real block — a second
//     (or Nth) failure is signal, not noise.
//
// Retry counts are tracked per check name across watch attempts so a check is
// never rerun more than its max_retries.
package safegit

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// runIDFromLink extracts the numeric Actions run ID from a check's details link,
// e.g. https://github.com/o/r/actions/runs/123456789/job/987 → "123456789".
var runIDFromLink = regexp.MustCompile(`/actions/runs/(\d+)`)

// failedCheck is a CI check that did not pass, with the Actions run it belongs
// to (when resolvable) so the valve can rerun it.
type failedCheck struct {
	Name  string
	State string
	RunID string
}

// ghCheck mirrors the gh pr checks --json fields we need.
type ghCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Link  string `json:"link"`
}

// rerunFunc reruns the failed jobs of an Actions run. Injected so the flake
// valve's decision logic is unit-testable without invoking gh.
type rerunFunc func(runID, repo string) error

// checkFlakeValve fetches the PR's failing checks and applies the flake valve.
// state maps check name → reruns already spent; it is mutated in place so watch
// mode honours max_retries across attempts.
func checkFlakeValve(prNum int, repo string, cfg *Config, state map[string]int) error {
	out, err := runCommand(exec.Command("gh", "pr", "checks",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "name,state,link",
	))
	if err != nil {
		// gh pr checks exits non-zero when checks are failing; runCommand still
		// returns the captured stdout on some shells, but to be safe we surface
		// the error rather than silently passing.
		return fmt.Errorf("gh pr checks failed: %w", err)
	}
	failing, err := parseFailingChecks(out)
	if err != nil {
		return err
	}
	return evaluateFlakeValve(failing, cfg, repo, state, RerunFailed)
}

// parseFailingChecks extracts the failing checks (and their run IDs) from the
// gh pr checks --json output. Pending checks are not failures and are ignored
// here — the CI gate already handles them. Exported for testing.
func parseFailingChecks(data []byte) ([]failedCheck, error) {
	var checks []ghCheck
	if err := json.Unmarshal(data, &checks); err != nil {
		return nil, fmt.Errorf("parsing check output: %w", err)
	}
	var failing []failedCheck
	for _, c := range checks {
		switch strings.ToLower(c.State) {
		case "fail", "failure", "error", "cancelled", "canceled", "timed_out", "action_required", "startup_failure":
			fc := failedCheck{Name: c.Name, State: c.State}
			if m := runIDFromLink.FindStringSubmatch(c.Link); m != nil {
				fc.RunID = m[1]
			}
			failing = append(failing, fc)
		}
	}
	return failing, nil
}

// evaluateFlakeValve decides what to do about the failing checks. It returns nil
// only when there are no failing checks. Any non-flaky failure, or a flaky
// failure that has exhausted its retries, is a real block. A flaky failure with
// retries left is rerun and reported as retryable (an error, so a one-shot run
// blocks and watch mode waits for the rerun). Exported for testing via a mocked
// rerun.
func evaluateFlakeValve(failing []failedCheck, cfg *Config, repo string, state map[string]int, rerun rerunFunc) error {
	if len(failing) == 0 {
		return nil
	}
	if state == nil {
		// Defensive: callers pass a real map, but never panic on a nil one.
		state = map[string]int{}
	}

	var reran []string
	var realBlocks []string
	for _, fc := range failing {
		flaky := matchFlaky(cfg, fc.Name)
		if flaky == nil {
			realBlocks = append(realBlocks, fmt.Sprintf("%s (%s)", fc.Name, fc.State))
			continue
		}

		spent := state[fc.Name]
		if spent >= flaky.MaxRetries {
			realBlocks = append(realBlocks, fmt.Sprintf(
				"%s (%s) — failed again after %d sanctioned retry(ies); real failure",
				fc.Name, fc.State, spent))
			continue
		}

		if fc.RunID == "" {
			realBlocks = append(realBlocks, fmt.Sprintf(
				"%s (%s) — flaky but no Actions run ID resolved; cannot rerun",
				fc.Name, fc.State))
			continue
		}

		state[fc.Name] = spent + 1
		if err := rerun(fc.RunID, repo); err != nil {
			realBlocks = append(realBlocks, fmt.Sprintf(
				"%s — rerun failed: %v", fc.Name, err))
			continue
		}
		reran = append(reran, fmt.Sprintf("%s (attempt %d/%d, run %s)",
			fc.Name, state[fc.Name], flaky.MaxRetries, fc.RunID))
	}

	if len(realBlocks) > 0 {
		return fmt.Errorf("CI failure(s) block merge:\n  %s", strings.Join(realBlocks, "\n  "))
	}
	if len(reran) > 0 {
		return fmt.Errorf("flaky check(s) reran; awaiting result:\n  %s", strings.Join(reran, "\n  "))
	}
	return nil
}

// matchFlaky returns the FlakyCheck whose Name is a substring of the failing
// check name, or nil if none matches.
func matchFlaky(cfg *Config, checkName string) *FlakyCheck {
	if cfg == nil {
		return nil
	}
	for i := range cfg.FlakyChecks {
		if cfg.FlakyChecks[i].Name != "" && strings.Contains(checkName, cfg.FlakyChecks[i].Name) {
			return &cfg.FlakyChecks[i]
		}
	}
	return nil
}

// RerunFailed reruns the failed jobs of an Actions run via gh. It is the default
// rerunFunc; tests inject their own.
func RerunFailed(runID, repo string) error {
	args := BuildRerunArgs(runID, repo)
	if _, err := runCommand(exec.Command(args[0], args[1:]...)); err != nil {
		return fmt.Errorf("gh run rerun --failed %s: %w", runID, err)
	}
	return nil
}

// BuildRerunArgs returns the argv for rerunning only the failed jobs of an
// Actions run. Exported so tests can verify the --failed flag is always present
// (a full rerun would waste minutes re-running already-green jobs). Panics on an
// empty runID — there is nothing to rerun without one.
func BuildRerunArgs(runID, repo string) []string {
	if runID == "" {
		panic("BuildRerunArgs: runID must not be empty")
	}
	return []string{"gh", "run", "rerun", runID, "--failed", "--repo", repo}
}
