package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func TestCommandIdentityDoesNotDependOnSourceCheckout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help exit code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("help stderr = %q, want empty", stderr.String())
	}
	for _, command := range []string{
		"specaudit guard",
		"specaudit inventory",
		"specaudit validate",
		"specaudit render",
	} {
		if !strings.Contains(stdout.String(), command) {
			t.Errorf("help omitted logical command identity %q", command)
		}
	}
	assertPortableCommandIdentity(t, "help", stdout.String())

	_, inventoryReport, _ := auditFixture(t)
	if got := inventoryReport.Methodology.Collector; got != "specaudit inventory" {
		t.Fatalf("collector identity = %q, want %q", got, "specaudit inventory")
	}
	wantReproduce := fmt.Sprintf(
		"%s inventory -repo %s -repository %s -revision %s",
		authenticatedSpecauditCommand,
		reproducibleRepositoryPath,
		inventoryReport.Snapshot.Repository,
		inventoryReport.Snapshot.Revision,
	)
	if got := inventoryReport.Methodology.Reproduce; len(got) != 1 || got[0] != wantReproduce {
		t.Fatalf("reproduction identity = %#v, want %q", got, wantReproduce)
	}
	assertPortableCommandIdentity(t, "inventory methodology", strings.Join(append(
		[]string{inventoryReport.Methodology.Collector},
		inventoryReport.Methodology.Reproduce...,
	), "\n"))
}

func TestReproductionRepositoryArgumentCannotChangeCommandShape(t *testing.T) {
	const revision = "0123456789012345678901234567890123456789"
	for _, repository := range []string{
		"owner/repo",
		"owner/repo with spaces",
		"owner/repo; touch sentinel",
		"owner/repo'; touch sentinel; '",
		"owner/$(touch sentinel)",
		"owner/`touch sentinel`",
	} {
		t.Run(repository, func(t *testing.T) {
			quoted, err := quoteReproductionArgument(repository)
			if err != nil {
				t.Fatalf("quote repository: %v", err)
			}
			command := fmt.Sprintf("%s inventory -repo %s -repository %s -revision %s", authenticatedSpecauditCommand, reproducibleRepositoryPath, quoted, revision)
			parsed, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(command), "reproduce")
			if err != nil {
				t.Fatalf("parse reproduction command: %v", err)
			}
			if len(parsed.Stmts) != 1 || parsed.Stmts[0].Negated || parsed.Stmts[0].Background || len(parsed.Stmts[0].Redirs) != 0 {
				t.Fatalf("reproduction command changed statement shape: %#v", parsed.Stmts)
			}
			call, ok := parsed.Stmts[0].Cmd.(*syntax.CallExpr)
			if !ok || len(call.Assigns) != 0 || len(call.Args) != 8 {
				t.Fatalf("reproduction command is not one static eight-argument call: %#v", parsed.Stmts[0].Cmd)
			}
			want := []string{"<distribution-root>/bin/specaudit", "inventory", "-repo", "<repository-path>", "-repository", repository, "-revision", revision}
			for index, word := range call.Args {
				got, err := expand.Literal(nil, word)
				if err != nil {
					t.Fatalf("expand argument %d: %v", index, err)
				}
				if got != want[index] {
					t.Fatalf("argument %d = %q, want %q", index, got, want[index])
				}
			}
		})
	}
}

func TestInventoryRejectsUnquotableRepositoryLabelsBeforeGit(t *testing.T) {
	for _, repository := range []string{"owner/repo\nnext", "owner/repo\tnext", "owner/repo\x00next", "owner/repo\x01next", string([]byte("owner/repo\xff"))} {
		_, err := inventoryWithLimits("does-not-need-to-exist", repository, strings.Repeat("0", 40), inventoryLimits{
			corpusBytes: 1,
			corpusFiles: 1,
			wallTime:    1,
		})
		if err == nil || !strings.Contains(err.Error(), "quote repository label") {
			t.Fatalf("inventoryWithLimits(%q) error = %v, want pre-Git quote rejection", repository, err)
		}
	}
	for _, repository := range []string{"", " owner/repo", "owner/repo ", "\nowner/repo"} {
		_, err := inventoryWithLimits("does-not-need-to-exist", repository, strings.Repeat("0", 40), inventoryLimits{
			corpusBytes: 1,
			corpusFiles: 1,
			wallTime:    1,
		})
		if err == nil || !strings.Contains(err.Error(), "without surrounding whitespace") {
			t.Fatalf("inventoryWithLimits(%q) error = %v, want pre-Git surrounding-whitespace rejection", repository, err)
		}
	}
}

func assertPortableCommandIdentity(t *testing.T, field, value string) {
	t.Helper()
	for _, checkoutRelative := range []string{"go run", "./tools/specaudit", "tools/specaudit"} {
		if strings.Contains(value, checkoutRelative) {
			t.Errorf("%s contains checkout-relative command identity %q: %q", field, checkoutRelative, value)
		}
	}
}
