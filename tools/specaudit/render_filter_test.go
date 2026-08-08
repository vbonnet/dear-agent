package main

import (
	"strings"
	"testing"
)

func TestRenderFiltersCandidateAndBoundaryFindingCards(t *testing.T) {
	report := validReport()
	report.Candidates = []finding{{
		ID: "SPEC-CLUSTER-001", Title: "candidate finding", Verdict: "merge-now",
	}}
	report.NonCandidates = []finding{{
		ID: "SPEC-CLUSTER-002", Title: "boundary finding", Verdict: "keep-separate",
	}}
	report.Summary.ByVerdict = map[string]int{"keep-separate": 1, "merge-now": 1}

	output := renderHTML(report, nil)
	for _, fragment := range []string{
		`<option value="keep-separate">keep-separate</option>`,
		`<option value="merge-now">merge-now</option>`,
		`<article class="finding" id="SPEC-CLUSTER-001" data-verdict="merge-now">`,
		`<article class="finding" id="SPEC-CLUSTER-002" data-verdict="keep-separate">`,
		"document.querySelectorAll('#findings .finding,#boundaries .finding')",
		"card.hidden=(q&&!card.innerText.toLowerCase().includes(q))||(v&&card.dataset.verdict!==v)",
		"query.addEventListener('input',applyFilters)",
		"verdict.addEventListener('change',applyFilters)",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("renderer omitted finding-filter contract fragment %q", fragment)
		}
	}
}
