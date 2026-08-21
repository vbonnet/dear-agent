package main

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

// The scheduled refresh must publish to the exact path an interactive
// consumer resolves. launchd's EnvironmentVariables carries only PATH and
// HOME (not XDG_STATE_HOME), so quota-meter's own default resolution
// inside the job would silently diverge from $XDG_STATE_HOME for an
// operator who has it set. The installer instead bakes in the path it
// resolves in its own environment via a --state-file argument (codex
// review on #1218).
func TestQuotaRefreshPlistTemplateBakesInAStateFileArgument(t *testing.T) {
	raw, err := schedulesFS.ReadFile(quotaPlistFile)
	if err != nil {
		t.Fatalf("read embedded plist template: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, "<string>--state-file</string>") {
		t.Fatal("template ProgramArguments is missing --state-file")
	}
	if !strings.Contains(content, "<string>__STATE_FILE__</string>") {
		t.Fatal("template ProgramArguments is missing the __STATE_FILE__ placeholder")
	}

	// --state-file must immediately follow --refresh in ProgramArguments
	// (order matters for a flag argument), and __STATE_FILE__ must be the
	// very next element after --state-file.
	const refreshArg = "<string>--refresh</string>"
	_, rest, found := strings.Cut(content, refreshArg)
	if !found {
		t.Fatal("template is missing --refresh")
	}
	rest = strings.TrimLeft(rest, "\n\t ")
	if !strings.HasPrefix(rest, "<string>--state-file</string>") {
		t.Errorf("--state-file does not immediately follow --refresh: %.80q", rest)
	}
}

// Mirrors the substitution runInstallQuotaSchedule performs, proving the
// rendered plist ends up with the resolved path rather than the literal
// placeholder — the same regression class as __USER_HOME__/__AGM_BINARY__.
func TestQuotaRefreshPlistTemplateSubstitutesStateFilePath(t *testing.T) {
	raw, err := schedulesFS.ReadFile(quotaPlistFile)
	if err != nil {
		t.Fatalf("read embedded plist template: %v", err)
	}

	const statePath = "/Users/test/.local/state/dear-agent/quota/latest.json"
	content := strings.ReplaceAll(string(raw), "__USER_HOME__", "/Users/test")
	content = strings.ReplaceAll(content, "__AGM_BINARY__", "/Users/test/go/bin/agm")
	content = strings.ReplaceAll(content, "__STATE_FILE__", statePath)

	if strings.Contains(content, "__STATE_FILE__") {
		t.Error("__STATE_FILE__ placeholder was not fully substituted")
	}
	if !strings.Contains(content, "<string>"+statePath+"</string>") {
		t.Errorf("rendered plist does not carry the resolved state path %q", statePath)
	}
}

// A home directory, binary path, or resolved state path can legitimately
// contain an XML metacharacter (an "R&D" directory, say). Substituting it
// raw would produce a malformed plist that launchctl load silently fails
// to parse; xmlEscapeText must keep every substituted <string> element
// well-formed (codex review on #1218).
//
// This checks the substituted elements directly rather than parsing the
// whole document: the template's own descriptive header comment is free
// text, not installer-controlled data, and Go's strict XML parser rejects
// even a literal "--" inside a comment — a rule real plist tooling
// (CFPropertyList/launchctl) does not enforce and this fix has no bearing
// on.
func TestQuotaRefreshPlistTemplateEscapesXMLMetacharactersInSubstitutions(t *testing.T) {
	raw, err := schedulesFS.ReadFile(quotaPlistFile)
	if err != nil {
		t.Fatalf("read embedded plist template: %v", err)
	}

	const home = `/Users/A&D <ops>/test`
	const bin = `/Users/A&D <ops>/go/bin/agm`
	const statePath = `/Users/A&D <ops>/.local/state/dear-agent/quota/latest.json`

	content := strings.ReplaceAll(string(raw), "__USER_HOME__", xmlEscapeText(home))
	content = strings.ReplaceAll(content, "__AGM_BINARY__", xmlEscapeText(bin))
	content = strings.ReplaceAll(content, "__STATE_FILE__", xmlEscapeText(statePath))

	if strings.Contains(content, home) {
		t.Error("the unescaped home path leaked into the rendered plist")
	}
	wantEscaped := xml.CharData(bin)
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, wantEscaped); err != nil {
		t.Fatalf("xml.EscapeText: %v", err)
	}
	if !strings.Contains(content, "<string>"+escaped.String()+"</string>") {
		t.Errorf("rendered plist does not carry the escaped binary path as a well-formed <string> element:\n%s", content)
	}
}

func TestXMLEscapeTextEscapesMetacharacters(t *testing.T) {
	got := xmlEscapeText(`A&D <ops> "quoted"`)
	for _, bad := range []string{"&D", "<ops>", `"quoted"`} {
		if strings.Contains(got, bad) {
			t.Errorf("xmlEscapeText result still contains unescaped %q: %s", bad, got)
		}
	}
}
