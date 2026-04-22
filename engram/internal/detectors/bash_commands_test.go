package detectors

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

func TestBashCommandPatternDetector_DetectCdAnd(t *testing.T) {
	detector := NewBashCommandPatternDetector()

	input := DetectorInput{
		Content: "cd /path && ls",
		Metadata: map[string]string{
			"agent": "claude-code",
		},
	}

	violations, err := detector.Detect(context.Background(), input)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if len(violations) != 1 {
		t.Fatalf("Expected 1 violation, got %d", len(violations))
	}

	v := violations[0]
	if v.InstructionRule != "never_use_cd_and" {
		t.Errorf("InstructionRule = %q, want %q", v.InstructionRule, "never_use_cd_and")
	}
	if v.Confidence != telemetry.ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q", v.Confidence, telemetry.ConfidenceHigh)
	}
}

func TestBashCommandPatternDetector_DetectCat(t *testing.T) {
	detector := NewBashCommandPatternDetector()

	input := DetectorInput{
		Content: "cat file.txt",
		Metadata: map[string]string{
			"agent": "claude-code",
		},
	}

	violations, err := detector.Detect(context.Background(), input)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if len(violations) != 1 {
		t.Fatalf("Expected 1 violation, got %d", len(violations))
	}

	v := violations[0]
	if v.InstructionRule != "never_use_cat" {
		t.Errorf("InstructionRule = %q, want %q", v.InstructionRule, "never_use_cat")
	}
}

func TestBashCommandPatternDetector_DetectGrep(t *testing.T) {
	detector := NewBashCommandPatternDetector()

	input := DetectorInput{
		Content: "grep pattern file.txt",
		Metadata: map[string]string{
			"agent": "claude-code",
		},
	}

	violations, err := detector.Detect(context.Background(), input)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if len(violations) != 1 {
		t.Fatalf("Expected 1 violation, got %d", len(violations))
	}

	v := violations[0]
	if v.InstructionRule != "never_use_grep" {
		t.Errorf("InstructionRule = %q, want %q", v.InstructionRule, "never_use_grep")
	}
}

func TestBashCommandPatternDetector_DetectForLoop(t *testing.T) {
	detector := NewBashCommandPatternDetector()

	input := DetectorInput{
		Content: "for i in *.md; do echo $i; done",
		Metadata: map[string]string{
			"agent": "claude-code",
		},
	}

	violations, err := detector.Detect(context.Background(), input)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if len(violations) != 1 {
		t.Fatalf("Expected 1 violation, got %d", len(violations))
	}

	v := violations[0]
	if v.InstructionRule != "never_use_for_loop" {
		t.Errorf("InstructionRule = %q, want %q", v.InstructionRule, "never_use_for_loop")
	}
}

func TestBashCommandPatternDetector_NoViolations(t *testing.T) {
	detector := NewBashCommandPatternDetector()

	input := DetectorInput{
		Content: "git -C /path status",
		Metadata: map[string]string{
			"agent": "claude-code",
		},
	}

	violations, err := detector.Detect(context.Background(), input)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if len(violations) != 0 {
		t.Fatalf("Expected 0 violations, got %d", len(violations))
	}
}

func TestBashCommandPatternDetector_MultipleViolations(t *testing.T) {
	detector := NewBashCommandPatternDetector()

	input := DetectorInput{
		Content: "cd /path && cat file.txt",
		Metadata: map[string]string{
			"agent": "claude-code",
		},
	}

	violations, err := detector.Detect(context.Background(), input)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	// Should detect both cd && and cat
	if len(violations) != 2 {
		t.Fatalf("Expected 2 violations, got %d", len(violations))
	}
}
