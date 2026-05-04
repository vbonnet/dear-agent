package w0

import (
	"fmt"
	"strings"
)

// QuestionType categorizes W0 questions.
type QuestionType string

// W0 QuestionType values.
const (
	QuestionProblem     QuestionType = "problem"
	QuestionImpact      QuestionType = "impact"
	QuestionApproach    QuestionType = "approach"
	QuestionConstraints QuestionType = "constraints"
)

// MultipleChoiceOption represents a multiple choice option.
type MultipleChoiceOption struct {
	Value       string
	Label       string
	Description string
}

// MultipleChoice holds multiple choice configuration.
type MultipleChoice struct {
	Prompt        string
	Options       []MultipleChoiceOption
	AllowMultiple bool
}

// FreeText holds free text configuration.
type FreeText struct {
	Prompt      string
	Placeholder string
	Required    bool
	MinWords    int
}

// Question represents a W0 question template.
type Question struct {
	ID             string
	Type           QuestionType
	Round          int
	MultipleChoice *MultipleChoice
	FreeText       *FreeText
	Examples       []string
	HelpText       string
}

// QuestionResponse holds a user's response to a W0 question.
type QuestionResponse struct {
	QuestionID              string
	MultipleChoiceSelection []string
	FreeTextResponse        string
	Timestamp               int64
}

// Q1Problem is the problem type question.
var Q1Problem = Question{
	ID:    "q1-problem",
	Type:  QuestionProblem,
	Round: 1,
	MultipleChoice: &MultipleChoice{
		Prompt:        "What type of problem are we solving?",
		AllowMultiple: false,
		Options: []MultipleChoiceOption{
			{Value: "new_feature", Label: "New feature needed", Description: "Something is missing that users want"},
			{Value: "bug", Label: "Bug or issue", Description: "Something is broken that needs fixing"},
			{Value: "performance", Label: "Performance problem", Description: "System is slow or inefficient"},
			{Value: "security", Label: "Security concern", Description: "Vulnerability or compliance issue"},
			{Value: "refactoring", Label: "Refactoring needed", Description: "Code quality or architecture improvement"},
			{Value: "other", Label: "Other", Description: "Please describe in the text below"},
		},
	},
	FreeText: &FreeText{
		Prompt:      "Describe the specific problem in 1-2 sentences.",
		Placeholder: "e.g., Users can't login easily, causing high abandonment",
		Required:    true,
		MinWords:    5,
	},
	Examples: []string{
		"Users can't login easily, causing high abandonment",
		"API response times are 3x slower than last month",
		"Auth system doesn't support SSO, limiting enterprise adoption",
	},
	HelpText: "Think about: What's broken, missing, or painful? Who is affected?",
}

// Q2Impact is the impact question.
var Q2Impact = Question{
	ID:    "q2-impact",
	Type:  QuestionImpact,
	Round: 1,
	FreeText: &FreeText{
		Prompt:      "Why does this problem matter?\n\nHelp us understand the urgency and importance:\n- Who is affected? (all users, specific group, internal team)\n- How often does this happen? (constantly, daily, occasionally)\n- What's the cost of not solving it? (lost revenue, user churn, team time)",
		Placeholder: "e.g., 40% of new users abandon signup, losing ~200 customers/month...",
		Required:    true,
		MinWords:    10,
	},
	Examples: []string{
		"40% of new users abandon signup, losing ~200 customers/month. This directly impacts our Q4 growth targets.",
	},
	HelpText: "Think about: What happens if we ignore this problem?",
}

// Q3Approach is the approach question.
var Q3Approach = Question{
	ID:    "q3-approach",
	Type:  QuestionApproach,
	Round: 1,
	MultipleChoice: &MultipleChoice{
		Prompt:        "What type of solution are you thinking?",
		AllowMultiple: false,
		Options: []MultipleChoiceOption{
			{Value: "build_new", Label: "Build new", Description: "Create something from scratch"},
			{Value: "enhance_existing", Label: "Enhance existing", Description: "Add to what we have"},
			{Value: "fix_broken", Label: "Fix broken", Description: "Repair something that's not working"},
			{Value: "refactor", Label: "Refactor", Description: "Restructure existing code/system"},
			{Value: "research_first", Label: "Research first", Description: "Need to investigate before deciding"},
			{Value: "other", Label: "Other", Description: "Please describe in the text below"},
		},
	},
	FreeText: &FreeText{
		Prompt:      "Describe your high-level approach in 1-2 sentences.\n(Don't worry about technical details - that comes later!)",
		Placeholder: "e.g., Add Google OAuth SSO to authentication",
		Required:    true,
		MinWords:    5,
	},
	Examples: []string{
		"Add Google OAuth SSO to authentication",
		"Profile the system to find bottlenecks, then optimize top 3",
		"Refactor auth module to support multiple identity providers",
	},
	HelpText: "It's okay to say \"Not sure yet, need to explore in D1-D2\"",
}

// Q4Constraints is the constraints question.
var Q4Constraints = Question{
	ID:    "q4-constraints",
	Type:  QuestionConstraints,
	Round: 1,
	MultipleChoice: &MultipleChoice{
		Prompt:        "Are there any must-haves or can't-dos?",
		AllowMultiple: true,
		Options: []MultipleChoiceOption{
			{Value: "timeline", Label: "Timeline", Description: "Need it by specific date"},
			{Value: "budget", Label: "Budget", Description: "Limited funds available"},
			{Value: "technical_compatibility", Label: "Technical compatibility", Description: "Must work with existing systems"},
			{Value: "compliance", Label: "Compliance", Description: "Regulatory requirements (HIPAA, SOC2, etc.)"},
			{Value: "performance", Label: "Performance", Description: "Speed/latency requirements"},
			{Value: "no_breaking_changes", Label: "No breaking changes", Description: "Can't disrupt existing features"},
			{Value: "none", Label: "None", Description: "No specific constraints"},
		},
	},
	FreeText: &FreeText{
		Prompt:      "Anything else we should know about limitations or requirements? (Optional)",
		Placeholder: "e.g., Must integrate with existing JWT sessions without breaking current logins",
		Required:    false,
	},
	Examples: []string{
		"Must integrate with existing JWT sessions without breaking current logins",
		"Need it by Q1 for enterprise deal, budget approved",
		"HIPAA compliance required, no PHI in logs",
	},
	HelpText: "Think about: What CAN'T we do? What MUST we do?",
}

// AllQuestions returns all W0 questions in order.
var AllQuestions = []Question{Q1Problem, Q2Impact, Q3Approach, Q4Constraints}

// GetQuestionByID returns a question by its ID.
func GetQuestionByID(id string) *Question {
	for _, q := range AllQuestions {
		if q.ID == id {
			return &q
		}
	}
	return nil
}

// GetQuestionsByRound returns questions for a specific round.
func GetQuestionsByRound(round int) []Question {
	var result []Question
	for _, q := range AllQuestions {
		if q.Round == round {
			result = append(result, q)
		}
	}
	return result
}

// ResponseValidation holds the result of response validation.
type ResponseValidation struct {
	Valid  bool
	Errors []string
}

// ValidateResponse validates a question response.
func ValidateResponse(question Question, response QuestionResponse) ResponseValidation {
	var errors []string

	if question.MultipleChoice != nil {
		if len(response.MultipleChoiceSelection) == 0 {
			errors = append(errors, "Please select at least one option")
		} else {
			validValues := make(map[string]bool)
			for _, opt := range question.MultipleChoice.Options {
				validValues[opt.Value] = true
			}

			for _, sel := range response.MultipleChoiceSelection {
				if !validValues[sel] {
					errors = append(errors, "Invalid selection: "+sel)
				}
			}

			if !question.MultipleChoice.AllowMultiple && len(response.MultipleChoiceSelection) > 1 {
				errors = append(errors, "Please select only one option")
			}
		}
	}

	if question.FreeText != nil && question.FreeText.Required {
		trimmed := strings.TrimSpace(response.FreeTextResponse)
		if trimmed == "" {
			errors = append(errors, "Please provide a text response")
		} else if question.FreeText.MinWords > 0 {
			wc := countWords(trimmed)
			if wc < question.FreeText.MinWords {
				errors = append(errors, fmt.Sprintf("Please provide at least %d words (currently: %d)",
					question.FreeText.MinWords, wc))
			}
		}
	}

	return ResponseValidation{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}
