package scanners

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// TestConversationScanner_DetectReact tests React detection from conversation
func TestConversationScanner_DetectReact(t *testing.T) {
	scanner := NewConversationScanner()
	ctx := context.Background()

	req := &metacontext.AnalyzeRequest{
		WorkingDir: "/tmp/test",
		Conversation: []string{
			"I'm working on a React app",
			"Need help with React hooks",
		},
	}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect React
	hasReact := false
	for _, sig := range signals {
		if sig.Name == "React" && sig.Source == "conversation" {
			hasReact = true
			// Should have confidence > 0 (mentioned twice)
			if sig.Confidence <= 0 {
				t.Errorf("Expected confidence > 0, got %f", sig.Confidence)
			}
		}
	}

	if !hasReact {
		t.Error("ConversationScanner should detect React from conversation")
	}
}

// TestConversationScanner_MultipleFrameworks tests detection of multiple frameworks
func TestConversationScanner_MultipleFrameworks(t *testing.T) {
	scanner := NewConversationScanner()
	ctx := context.Background()

	req := &metacontext.AnalyzeRequest{
		WorkingDir: "/tmp/test",
		Conversation: []string{
			"Working on Django backend",
			"React frontend with GraphQL API",
			"Using PostgreSQL database",
		},
	}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect Django, React, GraphQL, PostgreSQL
	frameworks := map[string]bool{
		"Django":     false,
		"React":      false,
		"GraphQL":    false,
		"PostgreSQL": false,
	}

	for _, sig := range signals {
		if _, ok := frameworks[sig.Name]; ok {
			frameworks[sig.Name] = true
		}
	}

	for framework, detected := range frameworks {
		if !detected {
			t.Errorf("ConversationScanner should detect %s from conversation", framework)
		}
	}
}

// TestConversationScanner_EmptyConversation tests handling of empty conversation
func TestConversationScanner_EmptyConversation(t *testing.T) {
	scanner := NewConversationScanner()
	ctx := context.Background()

	req := &metacontext.AnalyzeRequest{
		WorkingDir:   "/tmp/test",
		Conversation: []string{},
	}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should return empty signals
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals for empty conversation, got %d", len(signals))
	}
}

// TestConversationScanner_Recent5Turns tests that only recent 5 turns are used
func TestConversationScanner_Recent5Turns(t *testing.T) {
	scanner := NewConversationScanner()
	ctx := context.Background()

	// Create conversation with 10 turns
	// First 5 mention "Django", last 5 mention "React"
	conversation := []string{
		"Django project 1",
		"Django project 2",
		"Django project 3",
		"Django project 4",
		"Django project 5",
		"React app 1",
		"React app 2",
		"React app 3",
		"React app 4",
		"React app 5",
	}

	req := &metacontext.AnalyzeRequest{
		WorkingDir:   "/tmp/test",
		Conversation: conversation,
	}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect React (recent 5 turns) but not Django (old turns)
	hasReact := false
	hasDjango := false

	for _, sig := range signals {
		if sig.Name == "React" {
			hasReact = true
		}
		if sig.Name == "Django" {
			hasDjango = true
		}
	}

	if !hasReact {
		t.Error("ConversationScanner should detect React from recent turns")
	}

	if hasDjango {
		t.Error("ConversationScanner should NOT detect Django from old turns (>5 turns ago)")
	}
}

// TestConversationScanner_ConfidenceScoring tests confidence based on mention frequency
func TestConversationScanner_ConfidenceScoring(t *testing.T) {
	scanner := NewConversationScanner()
	ctx := context.Background()

	// Single mention
	req1 := &metacontext.AnalyzeRequest{
		WorkingDir:   "/tmp/test",
		Conversation: []string{"Using React"},
	}

	signals1, err := scanner.Scan(ctx, req1)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Multiple mentions
	req2 := &metacontext.AnalyzeRequest{
		WorkingDir: "/tmp/test",
		Conversation: []string{
			"React hooks React components React state React props React context",
		},
	}

	signals2, err := scanner.Scan(ctx, req2)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Find React signal in both results
	var conf1, conf2 float64
	for _, sig := range signals1 {
		if sig.Name == "React" {
			conf1 = sig.Confidence
		}
	}
	for _, sig := range signals2 {
		if sig.Name == "React" {
			conf2 = sig.Confidence
		}
	}

	// Multiple mentions should have higher confidence
	if conf2 <= conf1 {
		t.Errorf("Expected higher confidence for multiple mentions, got %f (1 mention) vs %f (5 mentions)", conf1, conf2)
	}
}

// TestConversationScanner_CaseInsensitive tests case-insensitive matching
func TestConversationScanner_CaseInsensitive(t *testing.T) {
	scanner := NewConversationScanner()
	ctx := context.Background()

	req := &metacontext.AnalyzeRequest{
		WorkingDir: "/tmp/test",
		Conversation: []string{
			"REACT application",
			"react hooks",
			"React components",
		},
	}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect React regardless of case
	hasReact := false
	for _, sig := range signals {
		if sig.Name == "React" {
			hasReact = true
		}
	}

	if !hasReact {
		t.Error("ConversationScanner should detect React case-insensitively")
	}
}

// TestConversationScanner_Name tests Name() method
func TestConversationScanner_Name(t *testing.T) {
	scanner := NewConversationScanner()
	if scanner.Name() != "conversation" {
		t.Errorf("Expected name 'conversation', got '%s'", scanner.Name())
	}
}

// TestConversationScanner_Priority tests Priority() method
func TestConversationScanner_Priority(t *testing.T) {
	scanner := NewConversationScanner()
	if scanner.Priority() != 50 {
		t.Errorf("Expected priority 50, got %d", scanner.Priority())
	}
}
