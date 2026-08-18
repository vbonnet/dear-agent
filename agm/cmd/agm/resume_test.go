package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestResumeCommandFlags(t *testing.T) {
	for _, name := range []string{"detached", "force-parent", "prompt", "prompt-file", "delete-prompt-file"} {
		if resumeCmd.Flags().Lookup(name) == nil {
			t.Fatalf("resume flag %q is not registered", name)
		}
	}
}

func TestReadResumePromptFileValidatesBeforeExplicitDeletion(t *testing.T) {
	oversized := filepath.Join(t.TempDir(), "oversized.txt")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", 10*1024+1)), 0o600); err != nil {
		t.Fatalf("write oversized prompt: %v", err)
	}
	if _, err := readResumePromptFile(oversized, true); err == nil {
		t.Fatal("readResumePromptFile() accepted an oversized prompt")
	}
	if _, err := os.Stat(oversized); err != nil {
		t.Fatalf("rejected prompt file was removed: %v", err)
	}

	accepted := filepath.Join(t.TempDir(), "accepted.txt")
	if err := os.WriteFile(accepted, []byte("continue safely"), 0o600); err != nil {
		t.Fatalf("write accepted prompt: %v", err)
	}
	got, err := readResumePromptFile(accepted, true)
	if err != nil {
		t.Fatalf("readResumePromptFile() error: %v", err)
	}
	if got != "continue safely" {
		t.Fatalf("readResumePromptFile() = %q", got)
	}
	if _, err := os.Stat(accepted); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("accepted disposable prompt still exists: %v", err)
	}
}

type attachmentTestTmux struct {
	*session.MockTmux
	attached  []string
	attachErr error
}

func (t *attachmentTestTmux) AttachSession(name string) error {
	t.attached = append(t.attached, name)
	if t.attachErr != nil {
		return t.attachErr
	}
	return t.MockTmux.AttachSession(name)
}

func TestFinishResumeAttachmentUsesOnlyOperationResult(t *testing.T) {
	oldDetached := resumeDetached
	resumeDetached = false
	t.Cleanup(func() { resumeDetached = oldDetached })

	tmuxAdapter := &attachmentTestTmux{MockTmux: session.NewMockTmux()}
	result := &ops.ResumeSessionResult{
		SessionID:       "stable-id",
		TmuxSessionName: "canonical-name",
		StartedHarness:  true,
	}
	if err := finishResumeAttachment(context.Background(), tmuxAdapter, result); err != nil {
		t.Fatalf("finishResumeAttachment() error: %v", err)
	}
	if len(tmuxAdapter.attached) != 1 || tmuxAdapter.attached[0] != result.TmuxSessionName {
		t.Fatalf("attach calls = %v, want operation result name", tmuxAdapter.attached)
	}
}

func TestFinishResumeAttachmentReturnsAttachFailure(t *testing.T) {
	oldDetached := resumeDetached
	resumeDetached = false
	t.Cleanup(func() { resumeDetached = oldDetached })

	wantErr := errors.New("attach failed")
	tmuxAdapter := &attachmentTestTmux{
		MockTmux:  session.NewMockTmux(),
		attachErr: wantErr,
	}
	result := &ops.ResumeSessionResult{
		SessionID:       "stable-id",
		TmuxSessionName: "existing-runtime",
	}
	err := finishResumeAttachment(t.Context(), tmuxAdapter, result)
	if !errors.Is(err, wantErr) {
		t.Fatalf("finishResumeAttachment() error = %v, want %v", err, wantErr)
	}
}

func TestResumeSourceDelegatesLifecycleToOperation(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "resume.go", nil, 0)
	if err != nil {
		t.Fatalf("parse resume.go: %v", err)
	}
	operationCalls := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "ResumeSession" {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && pkg.Name == "ops" {
			operationCalls++
		}
		return true
	})
	if operationCalls != 1 {
		t.Fatalf("ops.ResumeSession calls in resume.go = %d, want 1", operationCalls)
	}
	source, err := os.ReadFile("resume.go")
	if err != nil {
		t.Fatalf("read resume.go: %v", err)
	}
	for _, retired := range []string{
		"resumeSessionRuntime",
		"resumeSessionTransactionWithRuntime",
		"rollbackCreatedResumeTmux",
		"dispatchResumeCommand",
		"waitForResumedHarness",
	} {
		if strings.Contains(string(source), retired) {
			t.Fatalf("resume.go still contains retired lifecycle symbol %q", retired)
		}
	}
}

func TestArchitectureUsesPreparedClaudeResumeBoundary(t *testing.T) {
	architecture, err := os.ReadFile("ARCHITECTURE.md")
	if err != nil {
		t.Fatalf("read CLI architecture: %v", err)
	}
	text := string(architecture)
	for _, want := range []string{"ops.ResumeSession", "ops.ResumeSessionRequest", "finishResumeAttachment"} {
		if !strings.Contains(text, want) {
			t.Errorf("CLI architecture shared resume example missing %q", want)
		}
	}
	for _, retired := range []string{"prepareClaudeResumeCommand", `tmux.SendKeys(sessionName, "claude --resume`} {
		if strings.Contains(text, retired) {
			t.Fatalf("CLI architecture still teaches retired resume boundary %q", retired)
		}
	}

	operationSource, err := os.ReadFile(filepath.Join("..", "..", "internal", "ops", "session_resume.go"))
	if err != nil {
		t.Fatalf("read shared resume operation: %v", err)
	}
	if !strings.Contains(string(operationSource), "harnessexec.PrepareClaudeCommand") {
		t.Fatal("shared resume operation bypasses the prepared Claude executor boundary")
	}
}
