package reflection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleRetro builds a minimal DEAR retro document for tests.
func sampleRetro(title, severity, rootCause, remediation string) string {
	return strings.Join([]string{
		"# " + title,
		"",
		"- **Date:** 2026-06-17",
		"- **Severity:** " + severity,
		"",
		"## Audit",
		"",
		"- " + rootCause,
		"",
		"## Retro",
		"",
		"- " + remediation,
		"",
	}, "\n")
}

func TestParseRetroArtifact(t *testing.T) {
	content := sampleRetro(
		"Mergeloop never resolved review threads",
		"P1",
		"the mergeloop never resolved gemini review threads blocking merge",
		"wire automatic gemini thread resolution into the mergeloop",
	)
	a := ParseRetroArtifact("/tmp/retro-mergeloop.md", content)

	if a.Title != "Mergeloop never resolved review threads" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Severity != "P1" {
		t.Errorf("Severity = %q, want P1", a.Severity)
	}
	if len(a.RootCauses) != 1 {
		t.Fatalf("RootCauses = %v, want 1", a.RootCauses)
	}
	if !strings.Contains(a.RootCauses[0], "review threads") {
		t.Errorf("RootCauses[0] = %q", a.RootCauses[0])
	}
	if len(a.Remediations) != 1 {
		t.Fatalf("Remediations = %v, want 1", a.Remediations)
	}
	if !strings.Contains(a.Remediations[0], "thread resolution") {
		t.Errorf("Remediations[0] = %q", a.Remediations[0])
	}
}

func TestParseRetroArtifactInlineRootCause(t *testing.T) {
	content := strings.Join([]string{
		"# Inline root cause retro",
		"## Audit",
		"**Root cause:** the dispatch loop spawned duplicate workers concurrently",
		"",
	}, "\n")
	a := ParseRetroArtifact("/tmp/inline.md", content)
	if len(a.RootCauses) != 1 {
		t.Fatalf("RootCauses = %v, want 1", a.RootCauses)
	}
	if !strings.Contains(a.RootCauses[0], "dispatch loop spawned duplicate workers") {
		t.Errorf("RootCauses[0] = %q", a.RootCauses[0])
	}
}

// corpus builds a small set of artifacts where two share a recurring theme
// (review-thread merge stalls) and one is a standalone one-off.
func corpus() []*RetroArtifact {
	return []*RetroArtifact{
		ParseRetroArtifact("/r/a.md", sampleRetro(
			"PR merge stall A", "P1",
			"unresolved gemini review threads blocked the mergeloop from merging",
			"resolve gemini review threads automatically in the mergeloop",
		)),
		ParseRetroArtifact("/r/b.md", sampleRetro(
			"PR merge stall B", "P1",
			"the mergeloop merge attempt blocked on unresolved review threads",
			"add thread resolution step before merge",
		)),
		ParseRetroArtifact("/r/c.md", sampleRetro(
			"Gopls fd leak", "P2",
			"gopls subprocess leaked file descriptors until the host exhausted them",
			"add periodic gopls restart to reclaim descriptors",
		)),
	}
}

func TestSupervisorSynthesizeRecurrenceAndClassification(t *testing.T) {
	report := NewSupervisor().Synthesize(corpus())

	if report.ArtifactCount != 3 {
		t.Fatalf("ArtifactCount = %d, want 3", report.ArtifactCount)
	}
	if len(report.Patterns) == 0 {
		t.Fatal("no patterns synthesized")
	}

	// The two merge-stall retros share keywords (mergeloop, review, threads,
	// merge) and must cluster into one recurring, systemic pattern.
	var mergePattern *SynthesizedPattern
	for i := range report.Patterns {
		p := &report.Patterns[i]
		if strings.Contains(p.Label, "review threads") || strings.Contains(p.Label, "mergeloop") {
			if p.Recurrence >= 2 {
				mergePattern = p
				break
			}
		}
	}
	if mergePattern == nil {
		t.Fatalf("expected a recurring merge-stall pattern; got %+v", report.Patterns)
	}
	if mergePattern.Recurrence != 2 {
		t.Errorf("merge pattern Recurrence = %d, want 2", mergePattern.Recurrence)
	}
	if mergePattern.Classification != ClassSystemic {
		t.Errorf("merge pattern Classification = %q, want systemic", mergePattern.Classification)
	}
	// Remediations from both contributing artifacts should be aggregated.
	if len(mergePattern.Remediations) < 2 {
		t.Errorf("merge pattern Remediations = %v, want >= 2", mergePattern.Remediations)
	}

	// Systemic patterns must sort before one-off patterns.
	seenOneOff := false
	for _, p := range report.Patterns {
		if p.Classification == ClassOneOff {
			seenOneOff = true
		} else if seenOneOff {
			t.Errorf("systemic pattern %q sorted after a one-off pattern", p.Label)
		}
	}
}

func TestClassificationOneOffP2(t *testing.T) {
	// A single P2 retro with a unique root cause is a one-off.
	single := []*RetroArtifact{
		ParseRetroArtifact("/r/solo.md", sampleRetro(
			"Solo flake", "P2",
			"a flaky integration test failed intermittently on macos runners",
			"quarantine the flaky test pending investigation",
		)),
	}
	report := NewSupervisor().Synthesize(single)
	if len(report.Patterns) != 1 {
		t.Fatalf("Patterns = %d, want 1", len(report.Patterns))
	}
	p := report.Patterns[0]
	if p.Recurrence != 1 {
		t.Errorf("Recurrence = %d, want 1", p.Recurrence)
	}
	if p.Classification != ClassOneOff {
		t.Errorf("Classification = %q, want one_off", p.Classification)
	}
}

func TestClassificationP1SingleIsSystemic(t *testing.T) {
	// A single high-severity (P1) retro is treated as systemic despite recurrence 1.
	single := []*RetroArtifact{
		ParseRetroArtifact("/r/p1.md", sampleRetro(
			"Sev one incident", "P1",
			"the overnight supervisor cascade failed and halted all workers",
			"add supervisor failover",
		)),
	}
	report := NewSupervisor().Synthesize(single)
	if len(report.Patterns) != 1 {
		t.Fatalf("Patterns = %d, want 1", len(report.Patterns))
	}
	if report.Patterns[0].Classification != ClassSystemic {
		t.Errorf("Classification = %q, want systemic for P1", report.Patterns[0].Classification)
	}
}

func TestAllFourLensesProduceFindings(t *testing.T) {
	clusters := buildClusters(corpus())
	if len(clusters) == 0 {
		t.Fatal("no clusters built")
	}
	lenses := []LensAnalyzer{
		&rootCauseLens{}, &recurrenceLens{}, &remediationLens{}, &classificationLens{},
	}
	wantLens := map[Lens]bool{
		LensRootCause: true, LensRecurrence: true, LensRemediation: true, LensClassification: true,
	}
	for _, l := range lenses {
		findings := l.Analyze(clusters)
		if len(findings) != len(clusters) {
			t.Errorf("lens %s produced %d findings, want %d", l.Lens(), len(findings), len(clusters))
		}
		if !wantLens[l.Lens()] {
			t.Errorf("unexpected lens %s", l.Lens())
		}
		for _, f := range findings {
			if f.Lens != l.Lens() {
				t.Errorf("finding lens = %s, want %s", f.Lens, l.Lens())
			}
			if f.Pattern == "" {
				t.Errorf("lens %s produced empty pattern key", l.Lens())
			}
		}
	}
}

func TestSynthesizeDeterministic(t *testing.T) {
	c := corpus()
	r1 := NewSupervisor().Synthesize(c)
	r2 := NewSupervisor().Synthesize(c)
	if r1.RenderMarkdown() != r2.RenderMarkdown() {
		t.Error("Synthesize is not deterministic across runs")
	}
}

func TestRenderMarkdownContainsLensSummary(t *testing.T) {
	report := NewSupervisor().Synthesize(corpus())
	md := report.RenderMarkdown()
	for _, want := range []string{
		"# Multi-Lens Retro Synthesis",
		"4 lenses",
		"Classification:",
		"Recurrence:",
		"Recommended remediations:",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestLoadCorpus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.md"), sampleRetro(
		"First", "P1", "first root cause about the dispatch substrate", "fix one"))
	writeFile(t, filepath.Join(dir, "two.md"), sampleRetro(
		"Second", "P2", "second root cause about the merge pipeline", "fix two"))
	writeFile(t, filepath.Join(dir, "ignore.txt"), "not markdown")

	artifacts, err := LoadCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("LoadCorpus returned %d artifacts, want 2 (txt ignored)", len(artifacts))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
