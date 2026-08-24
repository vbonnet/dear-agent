package craplens

import (
	"fmt"
	"strings"
)

// maxListed bounds each list in the rendered comment. A signal nobody finishes
// reading is a signal nobody acts on, and the worst entries sort first.
const maxListed = 10

// Render returns the Markdown body for the advisory comment, or the empty
// string when nothing was flagged.
//
// The wording asks for tests or a simplification and never asserts that the
// diff is wrong, because this signal cannot tell an untested function from one
// whose tests live behind an integration harness it could not run.
func (r Report) Render() string {
	if !r.Flagged() {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Code health signal (advisory)\n\n")
	b.WriteString("CRAP joins cyclomatic complexity with test coverage on the functions this PR changed: ")
	b.WriteString("`complexity^2 * (1 - coverage)^3 + complexity`. ")
	b.WriteString("At full coverage it equals the complexity, so a high score means branches no test reaches.\n\n")

	r.renderUntested(&b)
	r.renderOver(&b)

	if r.Scored > 0 {
		fmt.Fprintf(&b, "%d of %d scored changed functions are at or under %.0f, the target for agent-written code.\n\n",
			r.WithinAgentTarget, r.Scored, AgentTarget)
	}

	b.WriteString("Either add tests for the uncovered branches or split the function until each part is simple enough to be obviously right. ")
	b.WriteString("If the code is exercised by an integration suite this signal cannot run, say so and move on.\n")

	if len(r.Unknown) > 0 {
		unknown := truncate(r.Unknown, maxListed)
		encoded := make([]string, len(unknown))
		for i, path := range unknown {
			encoded[i] = mdCode(path)
		}
		fmt.Fprintf(&b, "\nCoverage could not be collected for %d touched package(s), which are excluded rather than scored as untested: %s",
			len(r.Unknown), strings.Join(encoded, ", "))
		if len(r.Unknown) > maxListed {
			fmt.Fprintf(&b, ", and %d more", len(r.Unknown)-maxListed)
		}
		b.WriteString(".\n")
	}

	if r.Unmeasured > 0 {
		fmt.Fprintf(&b, "\n%d changed function(s) could not be measured on this platform, typically build-tagged files excluded from the profile, and were not scored.\n", r.Unmeasured)
	}

	b.WriteString("\nThis verdict is advisory: it posts a comment and never fails a check. The separate gocyclo parity verification is a hard gate. ")
	b.WriteString("Discarded error returns and raw complexity are already hard-gated by `errcheck` and `gocyclo` in `.golangci.yml`; this signal deliberately does not duplicate them.\n")

	return b.String()
}

// renderUntested lists the touched packages measured at zero coverage, marking
// the ones that are wholly new in this diff.
func (r Report) renderUntested(b *strings.Builder) {
	if len(r.Untested) == 0 {
		return
	}
	fmt.Fprintf(b, "**Touched packages measured at 0%% coverage (%d):**\n\n", len(r.Untested))
	for i, p := range r.Untested {
		if i == maxListed {
			fmt.Fprintf(b, "- ...and %d more\n", len(r.Untested)-maxListed)
			break
		}
		label := "existing package"
		if p.New {
			label = "**new package**"
		}
		fmt.Fprintf(b, "- %s (%s)\n", mdCode(p.ImportPath), label)
	}
	b.WriteString("\n")
}

// renderOver tabulates the changed functions above the threshold, worst first.
func (r Report) renderOver(b *strings.Builder) {
	if len(r.Over) == 0 {
		return
	}
	fmt.Fprintf(b, "**Changed functions scoring over %.0f (%d):**\n\n", r.Threshold, len(r.Over))
	b.WriteString("| CRAP | Complexity | Coverage | Function |\n|---:|---:|---:|---|\n")
	for i, f := range r.Over {
		if i == maxListed {
			fmt.Fprintf(b, "\n...and %d more.\n", len(r.Over)-maxListed)
			break
		}
		// One decimal, not zero. A score of 30.4 is correctly selected by the
		// strict `> threshold` test but rounds to "30" at %.0f, so the row
		// would appear not to exceed the threshold it is listed under.
		fmt.Fprintf(b, "| %.1f | %d | %.1f%% | %s %s |\n",
			f.CRAP(), f.Complexity, f.Coverage*100,
			mdCode(fmt.Sprintf("%s:%d", f.File, f.Line)), mdCode(f.Name))
	}
	b.WriteString("\n")
}

// mdCode renders text as an inline code span that survives a Markdown table.
//
// A path is attacker-adjacent input as far as rendering goes: a filename may
// legally contain a backtick, which would terminate the span, or a pipe, which
// would add a column and break the table. The fence is widened past the
// longest backtick run in the text, per CommonMark, and a pipe is escaped
// because a table cell needs that even inside code.
func mdCode(text string) string {
	// A literal newline survives a widened backtick fence: Markdown's
	// block-level parsing (a heading, a list) can still trigger on content
	// after a line break even inside what reads as one inline code span,
	// so a repository-controlled path with an embedded newline followed by
	// e.g. "## " could inject a heading into the rendered comment. Collapse
	// line breaks before fencing rather than relying on the fence alone.
	text = strings.ReplaceAll(text, "\n", " ")
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
			continue
		}
		run = 0
	}
	fence := strings.Repeat("`", longest+1)

	padded := text
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		padded = " " + text + " "
	}
	return fence + strings.ReplaceAll(padded, "|", "\\|") + fence
}

// truncate bounds a list to n entries.
func truncate(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}
