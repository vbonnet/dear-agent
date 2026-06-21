package deploy

import (
	"strings"
	"testing"
)

const validManifest = `artifacts:
  - name: plist
    source: deploy/launchd/x.plist
    deployed: ~/Library/LaunchAgents/x.plist
    mode: "0644"
    tokens:
      __HOME__: ${HOME}
  - name: hook
    source: bin/hook
    deployed: ~/.config/hooks/hook
    mode: "0755"
    optional: true
    remediation: make build
`

func TestParseManifest_Valid(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if len(m.Artifacts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(m.Artifacts))
	}
	if m.Artifacts[1].Mode != "0755" || !m.Artifacts[1].Optional {
		t.Fatalf("hook artifact parsed wrong: %+v", m.Artifacts[1])
	}
}

func TestParseManifest_BinaryKind(t *testing.T) {
	data := "artifacts:\n  - name: mergeloop\n    kind: binary\n    source: cmd/mergeloop\n    deployed: ~/go/bin/mergeloop\n    remediation: make install-mergeloop\n"
	m, err := ParseManifest([]byte(data))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	a := m.Artifacts[0]
	if a.Kind != KindBinary || !a.IsBinary() {
		t.Fatalf("kind = %q, IsBinary = %v", a.Kind, a.IsBinary())
	}
}

func TestParseManifest_Errors(t *testing.T) {
	cases := map[string]string{
		"empty":          `artifacts: []`,
		"unknown field":  "artifacts:\n  - name: x\n    source: s\n    deployed: d\n    bogus: 1\n",
		"no name":        "artifacts:\n  - source: s\n    deployed: d\n",
		"no source":      "artifacts:\n  - name: x\n    deployed: d\n",
		"no deployed":    "artifacts:\n  - name: x\n    source: s\n",
		"duplicate name": "artifacts:\n  - name: x\n    source: s\n    deployed: d\n  - name: x\n    source: s2\n    deployed: d2\n",
		"bad mode":       "artifacts:\n  - name: x\n    source: s\n    deployed: d\n    mode: \"99\"\n",
		"unknown kind":   "artifacts:\n  - name: x\n    kind: widget\n    source: s\n    deployed: d\n",
	}
	for label, data := range cases {
		t.Run(label, func(t *testing.T) {
			if _, err := ParseManifest([]byte(data)); err == nil {
				t.Fatalf("expected error for %s", label)
			}
		})
	}
}

func TestFileMode_Default(t *testing.T) {
	a := Artifact{}
	m, err := a.FileMode()
	if err != nil {
		t.Fatalf("FileMode: %v", err)
	}
	if m.Perm() != 0o644 {
		t.Fatalf("default mode = %o, want 0644", m.Perm())
	}
}

func TestSelect(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	all, err := m.Select(nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("Select(nil) = %v, %v", all, err)
	}

	one, err := m.Select([]string{"hook"})
	if err != nil || len(one) != 1 || one[0].Name != "hook" {
		t.Fatalf("Select([hook]) = %v, %v", one, err)
	}

	if _, err := m.Select([]string{"nope"}); err == nil {
		t.Fatal("expected error for unknown artifact")
	}
}

func TestDeployedPath_Expansion(t *testing.T) {
	a := Artifact{Deployed: "~/Library/LaunchAgents/x.plist"}
	got := a.DeployedPath("/home/me")
	want := "/home/me/Library/LaunchAgents/x.plist"
	if got != want {
		t.Fatalf("DeployedPath = %q, want %q", got, want)
	}
	if !strings.HasPrefix(a.DeployedPath("/h"), "/h/") {
		t.Fatal("expected home prefix")
	}
}
