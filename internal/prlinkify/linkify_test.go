package prlinkify

import (
	"testing"
)

var defaultCfg = Config{DefaultRepo: "vbonnet/dear-agent"}

func link(num string) string {
	return "[PR #" + num + "](https://github.com/vbonnet/dear-agent/pull/" + num + ")"
}

func TestLinkifySingular(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"basic", "merged PR #464", "merged " + link("464")},
		{"start of line", "PR #100 is ready", link("100") + " is ready"},
		{"trailing punct", "see PR #42.", "see " + link("42") + "."},
		{"in parens", "(PR #99)", "(" + link("99") + ")"},
		{"multiple separate", "PR #1 and PR #2", link("1") + " and " + link("2")},
		{"no match bare number", "#464 is an issue", "#464 is an issue"},
		{"no match word prefix", "QPR #5 thing", "QPR #5 thing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Linkify(tt.input, defaultCfg)
			if got != tt.want {
				t.Errorf("Linkify(%q)\n got  %q\n want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLinkifyPlural(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"comma list",
			"PRs #412, #419, #423",
			link("412") + ", " + link("419") + ", " + link("423"),
		},
		{
			"two items",
			"PRs #10, #20",
			link("10") + ", " + link("20"),
		},
		{
			"and separator",
			"PRs #5 and #6",
			link("5") + ", " + link("6"),
		},
		{
			"comma and",
			"PRs #1, #2, and #3",
			link("1") + ", " + link("2") + ", " + link("3"),
		},
		{
			"with context",
			"merged PRs #100, #200 today",
			"merged " + link("100") + ", " + link("200") + " today",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Linkify(tt.input, defaultCfg)
			if got != tt.want {
				t.Errorf("Linkify(%q)\n got  %q\n want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSkipAlreadyLinked(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"full link", "[PR #464](https://github.com/vbonnet/dear-agent/pull/464)"},
		{"short link", "[#464](https://github.com/vbonnet/dear-agent/pull/464)"},
		{"custom text", "[PR #464](https://example.com)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Linkify(tt.input, defaultCfg)
			if got != tt.input {
				t.Errorf("should not modify already-linked:\n input %q\n got   %q", tt.input, got)
			}
		})
	}
}

func TestSkipCodeBlocks(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"inline code", "run `PR #464` in terminal"},
		{"fenced block", "```\nPR #464\n```"},
		{"fenced with lang", "```bash\ngh pr view #464\n```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Linkify(tt.input, defaultCfg)
			if got != tt.input {
				t.Errorf("should not modify code:\n input %q\n got   %q", tt.input, got)
			}
		})
	}
}

func TestMixedContent(t *testing.T) {
	input := "merged PR #464. See `PR #100` for context. Also PR #200."
	want := "merged " + link("464") + ". See `PR #100` for context. Also " + link("200") + "."
	got := Linkify(input, defaultCfg)
	if got != want {
		t.Errorf("mixed:\n got  %q\n want %q", got, want)
	}
}

func TestCustomRepo(t *testing.T) {
	cfg := Config{DefaultRepo: "dear-labs/dear-agent"}
	got := Linkify("PR #42", cfg)
	want := "[PR #42](https://github.com/dear-labs/dear-agent/pull/42)"
	if got != want {
		t.Errorf("custom repo:\n got  %q\n want %q", got, want)
	}
}

func TestEmptyAndNoMatch(t *testing.T) {
	for _, input := range []string{"", "no refs here", "#464 without PR prefix", "issue #123"} {
		got := Linkify(input, defaultCfg)
		if got != input {
			t.Errorf("should not change %q, got %q", input, got)
		}
	}
}

func TestFindRefs(t *testing.T) {
	refs := FindRefs("merged PR #464 and PR #200. See `PR #100`.", defaultCfg)
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d: %+v", len(refs), refs)
	}
	if refs[0].Number != "464" || refs[1].Number != "200" {
		t.Errorf("wrong refs: %+v", refs)
	}
}

func TestFindRefsDedups(t *testing.T) {
	refs := FindRefs("PR #464 and PR #464 again", defaultCfg)
	if len(refs) != 1 {
		t.Fatalf("want 1 deduped ref, got %d", len(refs))
	}
}

func TestPluralThenSingularNoDoubleLink(t *testing.T) {
	input := "PRs #1, #2 and PR #3"
	got := Linkify(input, defaultCfg)
	want := link("1") + ", " + link("2") + " and " + link("3")
	if got != want {
		t.Errorf("plural+singular:\n got  %q\n want %q", got, want)
	}
}

func TestAlreadyLinkedAmongBare(t *testing.T) {
	input := "[PR #464](https://github.com/vbonnet/dear-agent/pull/464) and PR #200"
	want := "[PR #464](https://github.com/vbonnet/dear-agent/pull/464) and " + link("200")
	got := Linkify(input, defaultCfg)
	if got != want {
		t.Errorf("mixed linked/bare:\n got  %q\n want %q", got, want)
	}
}

func TestMultiline(t *testing.T) {
	input := "Summary:\n- PR #464\n- PR #200\n- `PR #100`\n"
	want := "Summary:\n- " + link("464") + "\n- " + link("200") + "\n- `PR #100`\n"
	got := Linkify(input, defaultCfg)
	if got != want {
		t.Errorf("multiline:\n got  %q\n want %q", got, want)
	}
}
