package main

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
)

const (
	readmeSessionDomain   = "AGM session tools"
	readmeSchemaDomain    = "Schema tool"
	readmeDispatchDomain  = "Dispatch routing tools"
	readmeWayfinderDomain = "Wayfinder tools"
)

func TestREADMEInventoryMatchesCompiledMCPTools(t *testing.T) {
	markdown, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	documented, documentedDomains := parseREADMEToolInventory(t, string(markdown))

	tools := registeredMCPTools(t, registerMCPTools)
	compiledNames := make([]string, 0, len(tools))
	expectedDomains := make(map[string]bool)
	for _, tool := range tools {
		compiledNames = append(compiledNames, tool.Name)
		domain, ok := expectedREADMEToolDomain(tool.Name)
		if !ok {
			t.Fatalf("compiled tool %q has no README domain rule", tool.Name)
		}
		expectedDomains[domain] = true
		if got := documented[tool.Name]; got != domain {
			t.Errorf("README domain for %q = %q, want %q", tool.Name, got, domain)
		}
	}
	sort.Strings(compiledNames)

	documentedNames := make([]string, 0, len(documented))
	for name := range documented {
		documentedNames = append(documentedNames, name)
	}
	sort.Strings(documentedNames)
	if !slices.Equal(documentedNames, compiledNames) {
		t.Fatalf("README tools = %v, want compiled tools %v", documentedNames, compiledNames)
	}

	expectedDomainNames := make([]string, 0, len(expectedDomains))
	for domain := range expectedDomains {
		expectedDomainNames = append(expectedDomainNames, domain)
	}
	sort.Strings(expectedDomainNames)
	if !slices.Equal(documentedDomains, expectedDomainNames) {
		t.Fatalf("README domains = %v, want %v", documentedDomains, expectedDomainNames)
	}

	normalized := strings.Join(strings.Fields(string(markdown)), " ")
	claim := fmt.Sprintf("registers %d tools across %d domains.", len(compiledNames), len(expectedDomainNames))
	if !strings.Contains(normalized, claim) {
		t.Fatalf("README overview must contain %q", claim)
	}
}

func parseREADMEToolInventory(t *testing.T, markdown string) (map[string]string, []string) {
	t.Helper()
	tools := make(map[string]string)
	domains := make(map[string]bool)
	currentDomain := ""
	inTools := false
	for line := range strings.SplitSeq(markdown, "\n") {
		line = strings.TrimSpace(line)
		if line == "## Tools" {
			inTools = true
			continue
		}
		if !inTools {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			break
		}
		if domain, ok := strings.CutPrefix(line, "### "); ok {
			if domains[domain] {
				t.Fatalf("README repeats tool domain %q", domain)
			}
			domains[domain] = true
			currentDomain = domain
			continue
		}
		row, ok := strings.CutPrefix(line, "| `")
		if !ok {
			continue
		}
		name, _, ok := strings.Cut(row, "`")
		if !ok || name == "" {
			t.Fatalf("README has malformed tool row %q", line)
		}
		if currentDomain == "" {
			t.Fatalf("README tool %q has no domain heading", name)
		}
		if previous, duplicate := tools[name]; duplicate {
			t.Fatalf("README repeats tool %q under %q and %q", name, previous, currentDomain)
		}
		tools[name] = currentDomain
	}
	if !inTools {
		t.Fatal("README has no Tools section")
	}
	domainNames := make([]string, 0, len(domains))
	for domain := range domains {
		domainNames = append(domainNames, domain)
	}
	sort.Strings(domainNames)
	return tools, domainNames
}

func expectedREADMEToolDomain(name string) (string, bool) {
	switch {
	case strings.HasPrefix(name, "engram_"):
		return readmeWayfinderDomain, true
	case name == "agm_list_ops":
		return readmeSchemaDomain, true
	case name == "agm_get_quota_status", strings.Contains(name, "completion_relay_target"):
		return readmeDispatchDomain, true
	case strings.Contains(name, "_session"), strings.HasSuffix(name, "_message"):
		return readmeSessionDomain, true
	default:
		return "", false
	}
}
