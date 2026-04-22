package harnesseffort

import (
	"strings"
	"testing"
)

func testConfig(t *testing.T) HarnessEffortConfig {
	t.Helper()
	cfg, err := LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	return ResolveAliases(cfg)
}

func TestGenerateCodex_NewFile(t *testing.T) {
	cfg := testConfig(t)
	out, err := GenerateCodex(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCodex: %v", err)
	}
	if !strings.Contains(out, "[profiles.lookup]") {
		t.Errorf("expected [profiles.lookup] in output, got:\n%s", out)
	}
	if !strings.Contains(out, "reasoning_effort") {
		t.Errorf("expected reasoning_effort field in output, got:\n%s", out)
	}
	if !strings.Contains(out, codexSentinelBegin) {
		t.Errorf("expected sentinel begin in output")
	}
	if !strings.Contains(out, codexSentinelEnd) {
		t.Errorf("expected sentinel end in output")
	}
}

func TestGenerateCodex_AppendToExisting(t *testing.T) {
	cfg := testConfig(t)
	existing := "[model]\nname = \"default\"\n"
	out, err := GenerateCodex(cfg, existing)
	if err != nil {
		t.Fatalf("GenerateCodex: %v", err)
	}
	// Original content must be preserved
	if !strings.Contains(out, "[model]") {
		t.Errorf("expected original content preserved, got:\n%s", out)
	}
	// Managed block must be appended
	if !strings.Contains(out, codexSentinelBegin) {
		t.Errorf("expected sentinel begin appended")
	}
	if !strings.Contains(out, "[profiles.lookup]") {
		t.Errorf("expected [profiles.lookup] appended")
	}
	// Original content must appear before the managed block
	sentinelPos := strings.Index(out, codexSentinelBegin)
	modelPos := strings.Index(out, "[model]")
	if modelPos > sentinelPos {
		t.Errorf("expected original content before managed block")
	}
}

func TestGenerateCodex_ReplaceBlock(t *testing.T) {
	cfg := testConfig(t)
	existing := "[model]\nname = \"default\"\n\n" +
		codexSentinelBegin + "\n" +
		"[profiles.lookup]\nmodel = \"old-model\"\n" +
		codexSentinelEnd + "\n\n" +
		"[other]\nkey = \"value\"\n"

	out, err := GenerateCodex(cfg, existing)
	if err != nil {
		t.Fatalf("GenerateCodex: %v", err)
	}
	// Content before sentinels preserved
	if !strings.Contains(out, "[model]") {
		t.Errorf("expected content before sentinel preserved")
	}
	// Content after sentinels preserved
	if !strings.Contains(out, "[other]") {
		t.Errorf("expected content after sentinel preserved")
	}
	// Old model value must be gone
	if strings.Contains(out, "old-model") {
		t.Errorf("expected old model value replaced, still found in:\n%s", out)
	}
	// New managed block present
	if !strings.Contains(out, codexSentinelBegin) {
		t.Errorf("expected sentinel begin in result")
	}
	if !strings.Contains(out, codexSentinelEnd) {
		t.Errorf("expected sentinel end in result")
	}
	// Only one occurrence of each sentinel
	if strings.Count(out, codexSentinelBegin) != 1 {
		t.Errorf("expected exactly one sentinel begin, got:\n%s", out)
	}
	if strings.Count(out, codexSentinelEnd) != 1 {
		t.Errorf("expected exactly one sentinel end, got:\n%s", out)
	}
}

func TestGenerateCodex_Idempotent(t *testing.T) {
	cfg := testConfig(t)
	first, err := GenerateCodex(cfg, "")
	if err != nil {
		t.Fatalf("first GenerateCodex: %v", err)
	}
	second, err := GenerateCodex(cfg, first)
	if err != nil {
		t.Fatalf("second GenerateCodex: %v", err)
	}
	if first != second {
		t.Errorf("GenerateCodex is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestGenerateCodex_CorrectFieldName(t *testing.T) {
	cfg := testConfig(t)
	out, err := GenerateCodex(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCodex: %v", err)
	}
	if !strings.Contains(out, "reasoning_effort") {
		t.Errorf("expected reasoning_effort in output")
	}
	if strings.Contains(out, "model_reasoning_effort") {
		t.Errorf("output must NOT contain model_reasoning_effort, got:\n%s", out)
	}
}
