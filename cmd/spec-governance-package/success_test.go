//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vbonnet/dear-agent/internal/specpackage"
)

func TestRunStagesAndValidatesPackageJSON(t *testing.T) {
	source := commandTestRepositoryRoot(t)
	artifact := filepath.Join(t.TempDir(), "specaudit")
	if err := os.WriteFile(artifact, []byte("portable test artifact\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stagingParent := t.TempDir()
	var stageOutput bytes.Buffer
	var stageError bytes.Buffer
	code := run(context.Background(), []string{
		"stage",
		"-source", source,
		"-artifact", artifact,
		"-staging-parent", stagingParent,
	}, &stageOutput, &stageError)
	if code != 0 {
		t.Fatalf("stage exit = %d, stderr = %q", code, stageError.String())
	}
	var staged specpackage.StagedPackage
	if err := json.Unmarshal(stageOutput.Bytes(), &staged); err != nil {
		t.Fatalf("decode staged package: %v", err)
	}
	if staged.Root == "" || staged.Receipt.ManifestSHA256 == "" {
		t.Fatalf("stage output = %#v, want root and manifest digest", staged)
	}

	var validateOutput bytes.Buffer
	var validateError bytes.Buffer
	code = run(context.Background(), []string{"validate", "-root", staged.Root}, &validateOutput, &validateError)
	if code != 0 {
		t.Fatalf("validate exit = %d, stderr = %q", code, validateError.String())
	}
	var validated specpackage.Receipt
	if err := json.Unmarshal(validateOutput.Bytes(), &validated); err != nil {
		t.Fatalf("decode validation receipt: %v", err)
	}
	if validated.ManifestSHA256 != staged.Receipt.ManifestSHA256 {
		t.Fatalf("validate digest = %q, stage digest = %q", validated.ManifestSHA256, staged.Receipt.ManifestSHA256)
	}
}

func commandTestRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve command test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
