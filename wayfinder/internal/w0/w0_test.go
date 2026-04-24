package w0

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectW0NeedDisabled(t *testing.T) {
	cfg := DetectionConfig{Enabled: false}
	result := DetectW0Need("test", "/tmp", cfg)
	if result.Trigger {
		t.Error("should not trigger when disabled")
	}
	if result.Reason != "disabled" {
		t.Errorf("expected reason 'disabled', got %q", result.Reason)
	}
}

func TestDetectW0NeedExistingCharter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "W0-project-charter.md"), []byte("test"), 0o644)

	result := DetectW0Need("test", dir)
	if result.Trigger {
		t.Error("should not trigger when charter exists")
	}
	if result.Reason != "w0_exists" {
		t.Errorf("expected reason 'w0_exists', got %q", result.Reason)
	}
}

func TestDetectW0NeedUserSkip(t *testing.T) {
	result := DetectW0Need("please /minimal start", t.TempDir())
	if result.Trigger {
		t.Error("should not trigger with /minimal")
	}
	if result.Reason != "user_skip" {
		t.Errorf("expected reason 'user_skip', got %q", result.Reason)
	}
}

func TestDetectW0NeedVagueRequest(t *testing.T) {
	result := DetectW0Need("make it better", t.TempDir())
	if !result.Trigger {
		t.Error("should trigger for vague request")
	}
	if result.Reason != "vague_request" {
		t.Errorf("expected reason 'vague_request', got %q", result.Reason)
	}
}

func TestDetectW0NeedDetailedRequest(t *testing.T) {
	detailed := "We have a problem because the authentication system is broken. " +
		"Users cannot login with SSO. The issue is that JWT tokens expire too quickly. " +
		"We must fix this because it affects 40% of enterprise customers. " +
		"The requirement is to extend token lifetime to 24 hours. " +
		"The constraint is that we cannot change the auth middleware API."

	result := DetectW0Need(detailed, t.TempDir())
	if result.Trigger {
		t.Error("should not trigger for detailed request")
	}
}

func TestSaveAndReadCharter(t *testing.T) {
	dir := t.TempDir()
	charter := "# W0 Project Charter: Test\n\n## Problem Statement\nTest problem"

	result := SaveCharter(dir, charter)
	if !result.Success {
		t.Fatalf("save failed: %s", result.Error)
	}

	content, meta, err := ReadCharter(dir)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(content, "Test problem") {
		t.Error("charter content not preserved")
	}
	if meta.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", meta.Status)
	}
}

func TestSaveCharterValidation(t *testing.T) {
	result := SaveCharter("", "content")
	if result.Success {
		t.Error("should fail with empty path")
	}

	result = SaveCharter("/tmp", "")
	if result.Success {
		t.Error("should fail with empty charter")
	}
}

func TestCharterExists(t *testing.T) {
	dir := t.TempDir()
	if CharterExists(dir) {
		t.Error("charter should not exist yet")
	}

	os.WriteFile(filepath.Join(dir, "W0-project-charter.md"), []byte("test"), 0o644)
	if !CharterExists(dir) {
		t.Error("charter should exist")
	}
}

func TestDeleteCharter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "W0-project-charter.md"), []byte("test"), 0o644)

	result := DeleteCharter(dir)
	if !result.Success {
		t.Fatalf("delete failed: %s", result.Error)
	}
	if CharterExists(dir) {
		t.Error("charter should be deleted")
	}
}

func TestParseCharterWithFrontmatter(t *testing.T) {
	content := "---\ncreated: 2024-01-15\nstatus: approved\nversion: 1.0\n---\n\n# Charter\nContent here"
	charter, meta := ParseCharterWithFrontmatter(content)

	if meta.Created != "2024-01-15" {
		t.Errorf("expected created '2024-01-15', got %q", meta.Created)
	}
	if meta.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", meta.Status)
	}
	if meta.Version != "1.0" {
		t.Errorf("expected version '1.0', got %q", meta.Version)
	}
	if !strings.Contains(charter, "Content here") {
		t.Error("charter content not extracted correctly")
	}
}

func TestValidateCharter(t *testing.T) {
	valid := "## Problem Statement\ntest\n## Proposed Solution\ntest\n## Success Criteria\ntest\n## Scope\ntest\nTBD"
	result := ValidateCharter(valid)
	if !result.Valid {
		t.Errorf("expected valid charter, got error: %s", result.Error)
	}

	invalid := "## Problem Statement\ntest\n## Success Criteria\ntest"
	result = ValidateCharter(invalid)
	if result.Valid {
		t.Error("expected invalid charter (missing sections)")
	}
	if len(result.MissingSection) == 0 {
		t.Error("expected missing sections to be listed")
	}
}

func TestExtractCharterTitle(t *testing.T) {
	charter := "# W0 Project Charter: Google OAuth SSO\n\ncontent"
	title := ExtractCharterTitle(charter)
	if title != "Google OAuth SSO" {
		t.Errorf("expected 'Google OAuth SSO', got %q", title)
	}

	noTitle := "## Section\ncontent"
	if ExtractCharterTitle(noTitle) != "" {
		t.Error("expected empty title for no match")
	}
}

func TestValidateResponse(t *testing.T) {
	q := Q1Problem

	// Valid response with selection and text
	resp := QuestionResponse{
		QuestionID:              "q1-problem",
		MultipleChoiceSelection: []string{"bug"},
		FreeTextResponse:        "The login system is broken for enterprise users",
	}
	result := ValidateResponse(q, resp)
	if !result.Valid {
		t.Errorf("expected valid response, got errors: %v", result.Errors)
	}

	// Missing selection
	resp2 := QuestionResponse{
		QuestionID:       "q1-problem",
		FreeTextResponse: "The login system is broken for enterprise users",
	}
	result2 := ValidateResponse(q, resp2)
	if result2.Valid {
		t.Error("expected invalid response (missing selection)")
	}
}

func TestValidateUserResponse(t *testing.T) {
	result := ValidateUserResponse("", QuestionProblem)
	if result.Valid {
		t.Error("empty response should be invalid")
	}

	result = ValidateUserResponse("ok", QuestionProblem)
	if !result.NeedsElaboration {
		t.Error("short response should need elaboration")
	}

	result = ValidateUserResponse("I'm not sure what the problem is", QuestionProblem)
	if !result.Uncertainty {
		t.Error("uncertain response should be flagged")
	}

	result = ValidateUserResponse(
		"The authentication system is broken because JWT tokens expire after 5 minutes, "+
			"causing 40% of enterprise users to lose their sessions.",
		QuestionProblem)
	if !result.Valid {
		t.Error("detailed response should be valid")
	}
	if result.ClarityScore <= 0 {
		t.Error("detailed response should have positive clarity score")
	}
}

func TestDetectTechnicalTerms(t *testing.T) {
	terms := DetectTechnicalTerms("We need OAuth SSO with JWT support for our REST API")
	if len(terms) == 0 {
		t.Error("should detect technical terms")
	}

	found := make(map[string]bool)
	for _, term := range terms {
		found[term] = true
	}
	if !found["OAuth"] {
		t.Error("should detect OAuth")
	}
	if !found["SSO"] {
		t.Error("should detect SSO")
	}
}

func TestGetQuestionByID(t *testing.T) {
	q := GetQuestionByID("q1-problem")
	if q == nil {
		t.Fatal("expected question, got nil")
	}
	if q.Type != QuestionProblem {
		t.Errorf("expected problem type, got %s", q.Type)
	}

	if GetQuestionByID("nonexistent") != nil {
		t.Error("expected nil for nonexistent question")
	}
}

func TestGenerateSynthesisPrompt(t *testing.T) {
	input := SynthesisInput{
		Q1Answer: "Users can't login",
		Q2Answer: "40% abandon signup",
		Q3Answer: "Add Google OAuth",
		Q4Answer: "Must integrate with JWT",
		Date:     "2024-01-15",
	}

	prompt := GenerateSynthesisPrompt(input)
	if !strings.Contains(prompt, "Users can't login") {
		t.Error("prompt should contain Q1 answer")
	}
	if !strings.Contains(prompt, "Step 1") {
		t.Error("prompt should contain step-by-step process")
	}
}

func TestExtractCharterSections(t *testing.T) {
	charter := `## Problem Statement
The problem is X.

## Proposed Solution
We will do Y.

## Success Criteria
- Metric 1
- Metric 2

## Scope
In scope: A, B

## Constraints
Must do C
`
	sections := ExtractCharterSections(charter)
	if sections.ProblemStatement == "" {
		t.Error("should extract problem statement")
	}
	if sections.ProposedSolution == "" {
		t.Error("should extract proposed solution")
	}
	if sections.Constraints == "" {
		t.Error("should extract constraints")
	}
}
