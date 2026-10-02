package tmux

import "testing"

// trustDialogPane is the real pane text Claude Code v2.1.273 renders in an AGM
// sandbox. It is recorded verbatim because the regression this guards is
// exactly a wording/layout drift: the dialog lost its "1."/"2." numbering and
// now defaults to the negative option, which is what silently broke the older
// numbered matchers.
const trustDialogPane = ` Accessing workspace:
 /Users/v/.agm/sandboxes/1f3eb17b-1cd0-4bc9-bd63-b5c3920d483b/upper/repo0
 Quick safety check: Is this a project you created or one you trust? (Like your
 own code, a well-known open source project, or work from your team).
 Claude Code'll be able to read, edit, and execute files here.
 ⚠ This folder pre-approves 21 tool permissions in .claude/settings.json
 These will apply without asking. Only proceed if you trust this configuration.
 Security guide
 ❯ No, exit
   Yes, I trust this folder
 Enter to confirm · Esc to cancel`

const mcpDialogPane = `  2 new MCP servers found in this project
  Select any you wish to enable.
  ❯ [✔] gopls
    [✔] dear-agent-recommendations
       Enable selected
 Space to select · Esc to reject all`

const readyPane = ` ▐▛███▛█   Claude Code v2.1.273
▝▜██████▀  Sonnet 4.6 (1M context) · Claude API
❯ `

func TestClassifyStartupBlock(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		content  string
		wantKind string
		wantOK   bool
	}{
		{"unnumbered trust dialog", trustDialogPane, "workspace-trust", true},
		{"legacy trust wording", "Do you trust the files in this folder?", "workspace-trust", true},
		{"mcp approval dialog", mcpDialogPane, "mcp-approval", true},
		{"unauthenticated", "OAuth access token revoked, please run /login", "authentication", true},
		{"ready prompt is not a block", readyPane, "", false},
		{"empty capture", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block, ok := classifyStartupBlock(tc.content)
			if ok != tc.wantOK {
				t.Fatalf("classifyStartupBlock ok = %v, want %v", ok, tc.wantOK)
			}
			if block.Kind != tc.wantKind {
				t.Fatalf("classifyStartupBlock kind = %q, want %q", block.Kind, tc.wantKind)
			}
			if ok && (block.Summary == "" || block.Remedy == "") {
				t.Fatalf("block %q must carry both a summary and a remedy", block.Kind)
			}
		})
	}
}

// The trust dialog gates the MCP dialog, so a pane carrying traces of both must
// report the gate the operator has to clear first.
func TestClassifyStartupBlockPrefersTheEarlierGate(t *testing.T) {
	t.Parallel()
	block, ok := classifyStartupBlock(trustDialogPane + "\n" + mcpDialogPane)
	if !ok {
		t.Fatal("expected a startup block for combined pane text")
	}
	if block.Kind != "workspace-trust" {
		t.Fatalf("kind = %q, want workspace-trust", block.Kind)
	}
}

// ANSI attributes are present in real captures and must not defeat matching.
func TestClassifyStartupBlockStripsANSI(t *testing.T) {
	t.Parallel()
	block, ok := classifyStartupBlock("\x1b[1m Is this a project you created or one you trust?\x1b[0m")
	if !ok || block.Kind != "workspace-trust" {
		t.Fatalf("classifyStartupBlock = (%+v, %v), want workspace-trust", block, ok)
	}
}
