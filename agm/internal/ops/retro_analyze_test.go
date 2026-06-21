package ops

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleRetro = `# DEAR Retro: 14-Day CI Red Streak on main

**Date:** 2026-05-09
**Severity:** High

---

## Define

` + "`main`" + ` SHOULD be green. CI is a regression-detection signal; when main
stays red, the team loses its automated answer to "did my change break anything?"

## Enforce

Root cause — compounding failures:

### gh pr merge --auto merges immediately on this repo

There are no required status checks gating merges. Every PR with red CI was
merged into main anyway, again and again.

### New stdlib CVEs against the pinned Go version

govulncheck started reporting advisories; this is a recurring policy gap.

## Refine

| # | Action | Owner | Status |
| - | ------ | ----- | ------ |
| 1 | Add required status checks to protect main (ce-abc1) | vb | open |
| 2 | Bump Go toolchain in PR #67 | vb | done |

- [ ] Wire branch protection so structural breakage cannot recur (ce-def2)
- [x] Document the merge-by-default footgun

Prior incident: ce-xyz9 covered the same root cause last month.
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "RETRO.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestAnalyzeRetro_AllLenses(t *testing.T) {
	p := writeTemp(t, sampleRetro)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, AllLenses: true})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}

	if res.Title != "DEAR Retro: 14-Day CI Red Streak on main" {
		t.Errorf("title = %q", res.Title)
	}

	// Root cause: chain comes from the ### sub-headings under Enforce.
	if res.Lenses.RootCause == nil || len(res.Lenses.RootCause.Chain) != 2 {
		t.Fatalf("root cause chain = %+v", res.Lenses.RootCause)
	}
	if res.Lenses.RootCause.Summary == "" {
		t.Errorf("expected non-empty root cause summary")
	}

	// Recurrence: strong recurrence language + multiple prior bead refs → score 2.
	rec := res.Lenses.Recurrence
	if rec == nil || rec.Score != 2 {
		t.Fatalf("recurrence = %+v", rec)
	}
	if rec.Label != "recurring" {
		t.Errorf("recurrence label = %q", rec.Label)
	}
	if len(rec.PriorInstances) < 2 {
		t.Errorf("prior instances = %v", rec.PriorInstances)
	}

	// Remediation: should capture bead-tagged actions, the table rows, and checklist items.
	rem := res.Lenses.Remediation
	if rem == nil || len(rem.Actions) == 0 {
		t.Fatalf("remediation = %+v", rem)
	}
	var sawBead, sawPR bool
	for _, a := range rem.Actions {
		if a.Bead == "ce-abc1" {
			sawBead = true
		}
		if a.PR == "#67" {
			sawPR = true
		}
	}
	if !sawBead {
		t.Errorf("expected a remediation action tagged ce-abc1, got %+v", rem.Actions)
	}
	if !sawPR {
		t.Errorf("expected a remediation action tagged PR #67, got %+v", rem.Actions)
	}

	// Systemic: policy/structural language should dominate.
	sys := res.Lenses.Systemic
	if sys == nil || sys.Classification != "systemic" {
		t.Fatalf("systemic = %+v", sys)
	}
}

func TestAnalyzeRetro_SingleLens(t *testing.T) {
	p := writeTemp(t, sampleRetro)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, Lens: "root-cause"})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}
	if res.Lenses.RootCause == nil {
		t.Fatal("expected root cause lens")
	}
	if res.Lenses.Recurrence != nil || res.Lenses.Remediation != nil || res.Lenses.Systemic != nil {
		t.Errorf("expected only root-cause lens populated, got %+v", res.Lenses)
	}
}

func TestAnalyzeRetro_LensAliases(t *testing.T) {
	p := writeTemp(t, sampleRetro)
	for _, alias := range []string{"recur", "frequency", "recurrence"} {
		res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, Lens: alias})
		if err != nil {
			t.Fatalf("alias %q: %v", alias, err)
		}
		if res.Lenses.Recurrence == nil {
			t.Errorf("alias %q did not resolve to recurrence lens", alias)
		}
	}
}

func TestAnalyzeRetro_NovelRecurrence(t *testing.T) {
	novel := "# One-off typo fix\n\n## Define\n\nA single config typo broke startup. Isolated, context-specific manual error.\n"
	p := writeTemp(t, novel)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, AllLenses: true})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}
	if res.Lenses.Recurrence.Score != 0 {
		t.Errorf("expected novel score 0, got %d", res.Lenses.Recurrence.Score)
	}
	if res.Lenses.Systemic.Classification != "one-off" {
		t.Errorf("expected one-off classification, got %q", res.Lenses.Systemic.Classification)
	}
}

func TestAnalyzeRetro_Errors(t *testing.T) {
	if _, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: ""}); err == nil {
		t.Error("expected error for empty input path")
	}
	if _, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: "/nonexistent/RETRO.md", AllLenses: true}); err == nil {
		t.Error("expected error for missing file")
	}
	p := writeTemp(t, sampleRetro)
	if _, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, Lens: "bogus"}); err == nil {
		t.Error("expected error for unknown lens")
	}
}
