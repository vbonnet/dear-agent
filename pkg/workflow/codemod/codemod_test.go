package codemod

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func TestUpgradeV01ToV02_NoOpOnAlreadyUpgraded(t *testing.T) {
	in := []byte(`schema_version: "1"
name: already-v02
version: 0.1.0
nodes:
  - id: n1
    kind: ai
    ai:
      role: research
      prompt: hello
`)
	r, err := UpgradeV01ToV02(in, UpgradeOptions{})
	if err != nil {
		t.Fatalf("UpgradeV01ToV02: %v", err)
	}
	if r.Changed() {
		t.Fatalf("expected no-op, got changes: %v", r.Changes)
	}
	if !bytes.Equal(r.Output, in) {
		t.Fatalf("no-op should preserve bytes verbatim")
	}
}

func TestUpgradeV01ToV02_AddsSchemaVersionAndPromotesModel(t *testing.T) {
	in := []byte(`name: legacy
version: 0.1.0
nodes:
  - id: research
    kind: ai
    ai:
      model: claude-opus-4-7
      prompt: dig into the corpus
  - id: code
    kind: ai
    ai:
      model: claude-sonnet-4-6
      prompt: implement the spec
`)
	r, err := UpgradeV01ToV02(in, UpgradeOptions{})
	if err != nil {
		t.Fatalf("UpgradeV01ToV02: %v", err)
	}
	if !r.Changed() {
		t.Fatalf("expected changes, got none")
	}
	out := string(r.Output)
	if !strings.Contains(out, "schema_version: \"1\"") {
		t.Errorf("schema_version not added:\n%s", out)
	}
	if !strings.Contains(out, "role: research") {
		t.Errorf("role: research not added:\n%s", out)
	}
	if !strings.Contains(out, "role: implementer") {
		t.Errorf("role: implementer not added:\n%s", out)
	}
	// Model retained by default.
	if !strings.Contains(out, "model: claude-opus-4-7") {
		t.Errorf("model field unexpectedly removed:\n%s", out)
	}
	// And the upgraded YAML is loadable by the workflow loader.
	if _, err := workflow.LoadBytes(r.Output); err != nil {
		t.Fatalf("upgraded YAML doesn't load: %v\noutput:\n%s", err, out)
	}
}

func TestUpgradeV01ToV02_DropModelOnRolePromotion(t *testing.T) {
	in := []byte(`name: legacy
version: 0.1.0
nodes:
  - id: research
    kind: ai
    ai:
      model: claude-opus-4-7
      prompt: dig
`)
	r, err := UpgradeV01ToV02(in, UpgradeOptions{DropModelOnRolePromotion: true})
	if err != nil {
		t.Fatalf("UpgradeV01ToV02: %v", err)
	}
	out := string(r.Output)
	if strings.Contains(out, "model: claude-opus-4-7") {
		t.Errorf("model: should have been dropped:\n%s", out)
	}
	if !strings.Contains(out, "role: research") {
		t.Errorf("role: research not added:\n%s", out)
	}
}

func TestUpgradeV01ToV02_AddsDefaultBudgetWhenRequested(t *testing.T) {
	in := []byte(`name: legacy
version: 0.1.0
nodes:
  - id: research
    kind: ai
    ai:
      model: claude-opus-4-7
      prompt: dig
`)
	r, err := UpgradeV01ToV02(in, UpgradeOptions{AddDefaultBudget: true})
	if err != nil {
		t.Fatalf("UpgradeV01ToV02: %v", err)
	}
	out := string(r.Output)
	if !strings.Contains(out, "budget:") {
		t.Errorf("expected budget block:\n%s", out)
	}
	if !strings.Contains(out, "max_tokens: 50000") {
		t.Errorf("expected default max_tokens:\n%s", out)
	}
	if !strings.Contains(out, "on_overrun: fail") {
		t.Errorf("expected default on_overrun:\n%s", out)
	}
	// Loader accepts.
	if _, err := workflow.LoadBytes(r.Output); err != nil {
		t.Fatalf("upgraded YAML doesn't load: %v", err)
	}
}

func TestUpgradeV01ToV02_LeavesUnknownModelAlone(t *testing.T) {
	in := []byte(`name: legacy
version: 0.1.0
nodes:
  - id: n1
    kind: ai
    ai:
      model: some-unknown-model
      prompt: dig
`)
	r, err := UpgradeV01ToV02(in, UpgradeOptions{})
	if err != nil {
		t.Fatalf("UpgradeV01ToV02: %v", err)
	}
	out := string(r.Output)
	if strings.Contains(out, "role:") {
		t.Errorf("unknown model should not be promoted to a role:\n%s", out)
	}
}

func TestUpgradeV01ToV02_RecursesIntoLoops(t *testing.T) {
	in := []byte(`name: with-loop
version: 0.1.0
nodes:
  - id: outer
    kind: loop
    loop:
      until: 'Outputs.inner == "done"'
      max_iters: 3
      nodes:
        - id: inner
          kind: ai
          ai:
            model: claude-sonnet-4-6
            prompt: keep going
`)
	r, err := UpgradeV01ToV02(in, UpgradeOptions{})
	if err != nil {
		t.Fatalf("UpgradeV01ToV02: %v", err)
	}
	out := string(r.Output)
	if !strings.Contains(out, "role: implementer") {
		t.Errorf("loop child not promoted:\n%s", out)
	}
}

func TestUpgradeV01ToV02_RejectsNonMappingRoot(t *testing.T) {
	in := []byte("- not\n- a\n- mapping\n")
	if _, err := UpgradeV01ToV02(in, UpgradeOptions{}); err == nil {
		t.Fatal("expected error for non-mapping root")
	}
}

func TestUpgradeV01ToV02_CustomMappingOverridesBuiltin(t *testing.T) {
	in := []byte(`name: custom
version: 0.1.0
nodes:
  - id: n1
    kind: ai
    ai:
      model: my-private-model
      prompt: dig
`)
	r, err := UpgradeV01ToV02(in, UpgradeOptions{
		ModelToRole: map[string]string{"my-private-model": "research"},
	})
	if err != nil {
		t.Fatalf("UpgradeV01ToV02: %v", err)
	}
	out := string(r.Output)
	if !strings.Contains(out, "role: research") {
		t.Errorf("custom mapping not honoured:\n%s", out)
	}
}

func TestWriteResult_NoOp(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResult(&buf, Result{Path: "x.yaml"}); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "no-op") {
		t.Errorf("expected no-op prefix, got %q", buf.String())
	}
}

func TestWriteResult_Changed(t *testing.T) {
	var buf bytes.Buffer
	r := Result{Path: "x.yaml", Changes: []string{"first", "second"}}
	if err := WriteResult(&buf, r); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}
	if !strings.Contains(buf.String(), "changed x.yaml (2 transformations)") {
		t.Errorf("missing summary line:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "  - first") || !strings.Contains(buf.String(), "  - second") {
		t.Errorf("missing change lines:\n%s", buf.String())
	}
}
