package review

import (
	"fmt"
	"regexp"
	"time"
)

// PersonaType represents the different review personas
type PersonaType string

// Built-in persona type values.
const (
	// PersonaSpecCompliance is the Stage 1 persona: it verifies the
	// implementation satisfies the documented spec/requirements before any
	// code-quality review runs.
	PersonaSpecCompliance PersonaType = "spec_compliance"

	// Code-quality personas (Stage 2).
	PersonaSecurity        PersonaType = "security"
	PersonaPerformance     PersonaType = "performance"
	PersonaMaintainability PersonaType = "maintainability"
	PersonaUX              PersonaType = "ux"
	PersonaReliability     PersonaType = "reliability"
)

// ReviewStage identifies which stage of the two-stage review a persona belongs
// to. Stage 1 (spec compliance) runs before Stage 2 (code quality): an artifact
// that is well-written but does not satisfy the spec must still fail review.
type ReviewStage int

// Review stage values.
const (
	// StageSpecCompliance is Stage 1 — does the work satisfy the spec/requirements?
	StageSpecCompliance ReviewStage = 1
	// StageCodeQuality is Stage 2 — is the code secure, performant, maintainable, etc.?
	StageCodeQuality ReviewStage = 2
)

// PersonaStage reports which review stage a persona belongs to.
func PersonaStage(persona PersonaType) ReviewStage {
	if persona == PersonaSpecCompliance {
		return StageSpecCompliance
	}
	return StageCodeQuality
}

// antiSycophancyClause is prepended to every persona prompt. Research on
// multi-step agent review (superpowers/supermemory landscape, OSS dispatch
// ID-002) found that workers reviewing a prior step tend to validate it rather
// than scrutinize it. This directive instructs the reviewing worker to stay
// independent and flag disagreements instead of rubber-stamping the prior step.
const antiSycophancyClause = `ANTI-SYCOPHANCY DIRECTIVE (read this first):
You are an INDEPENDENT reviewer, not a collaborator. Your job is to find problems,
not to validate or agree with the prior step.
- Do NOT assume the work is correct just because a previous worker (or another
  reviewer) produced or approved it. Re-derive your own conclusion from the evidence.
- Do NOT soften, downplay, or omit findings to be agreeable, and do NOT add praise.
- If you DISAGREE with a prior decision, requirement interpretation, or claim, say so
  explicitly and explain why — flag the disagreement rather than rationalizing it away.
- Absence of evidence is not evidence of correctness: if you cannot verify a claim,
  report it as UNVERIFIED rather than assuming it holds.
- It is better to raise a concern that turns out minor than to stay silent to avoid friction.`

// composeReviewPrompt assembles the full worker prompt for a persona: the
// anti-sycophancy directive, the stage framing, then the persona-specific body.
func composeReviewPrompt(persona PersonaType, body string) string {
	stage := "STAGE 2 of 2 — CODE QUALITY"
	if PersonaStage(persona) == StageSpecCompliance {
		stage = "STAGE 1 of 2 — SPEC COMPLIANCE"
	}
	return antiSycophancyClause + "\n\nREVIEW STAGE: " + stage + "\n\n" + body
}

// Confidence represents the confidence level of a review result
type Confidence string

// Confidence level values.
const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// PersonaResult represents the result from a single persona's review
type PersonaResult struct {
	Persona    PersonaType   `json:"persona"`
	Timestamp  time.Time     `json:"timestamp"`
	Score      float64       `json:"score"` // 0-100
	Confidence Confidence    `json:"confidence"`
	Issues     []ReviewIssue `json:"issues"`
	Summary    string        `json:"summary"`
}

// PersonaConfig defines the configuration for a persona's review
type PersonaConfig struct {
	Name        string
	Description string
	FocusAreas  []string
	Patterns    map[string]IssueSeverity // Pattern -> Severity
	Prompt      string
}

// GetPersonaConfig returns the configuration for a specific persona. The
// returned Prompt is the full worker prompt: anti-sycophancy directive + stage
// framing + persona-specific body.
func GetPersonaConfig(persona PersonaType) PersonaConfig {
	configs := map[PersonaType]PersonaConfig{
		PersonaSpecCompliance:  specCompliancePersonaConfig(),
		PersonaSecurity:        securityPersonaConfig(),
		PersonaPerformance:     performancePersonaConfig(),
		PersonaMaintainability: maintainabilityPersonaConfig(),
		PersonaUX:              uxPersonaConfig(),
		PersonaReliability:     reliabilityPersonaConfig(),
	}

	cfg := configs[persona]
	// Unknown personas have no body; don't fabricate a prompt for them.
	if cfg.Name == "" {
		return cfg
	}
	cfg.Prompt = composeReviewPrompt(persona, cfg.Prompt)
	return cfg
}

// specCompliancePersonaConfig defines the Stage 1 Spec Compliance Persona. It
// checks whether the implementation actually satisfies the task's spec and
// requirements — independent of code quality, which is Stage 2.
func specCompliancePersonaConfig() PersonaConfig {
	return PersonaConfig{
		Name:        "Spec Compliance Reviewer",
		Description: "Stage 1: verifies the implementation satisfies the documented spec and requirements",
		FocusAreas: []string{
			"Acceptance-criteria coverage",
			"Requirement-to-implementation traceability",
			"Unimplemented or stubbed requirements",
			"Behavior that silently diverges from the spec",
			"Out-of-scope changes / scope creep",
			"Undocumented behavior changes",
			"Spec contradictions",
		},
		Patterns: map[string]IssueSeverity{
			// P1 - Requirement clearly not met
			`(?i)panic\(\s*["'][^"']*not\s+implemented`: SeverityP1, // not-implemented panic
			`(?i)\bnot\s+implemented\b`:                 SeverityP1, // not-implemented marker
			`(?i)\bunimplemented\b`:                     SeverityP1, // unimplemented marker

			// P2 - Spec gap that needs follow-up
			`(?i)TODO[^\n]*(spec|requirement|acceptance)`:  SeverityP2, // spec TODOs
			`(?i)FIXME[^\n]*(spec|requirement|acceptance)`: SeverityP2, // spec FIXMEs
			`(?i)out[\s-]?of[\s-]?scope`:                   SeverityP2, // self-declared scope creep
		},
		Prompt: `You are a Spec Compliance Reviewer performing STAGE 1 of a two-stage review.
This stage runs BEFORE any code-quality review. Code that is well-written but does
NOT satisfy the spec MUST still fail this stage — getting the right thing built comes
before building the thing right.

Check the implementation against the task's spec / requirements / acceptance criteria:
1. Is every stated requirement actually implemented, not just stubbed or mocked?
2. Does the observable behavior match the spec, or does it silently diverge?
3. Are the acceptance criteria demonstrably met (by tests or reproducible behavior)?
4. Are there out-of-scope changes or scope creep not called for by the spec?
5. Are spec-mandated edge cases handled?
6. Does the implementation contradict the spec anywhere?

Do NOT evaluate code style, performance, or security here — that is Stage 2. If the
spec itself is ambiguous or you cannot confirm a requirement is met, flag it as
unverified rather than assuming compliance.

Classify findings as:
- P0 (Critical): A core requirement is unmet, or behavior contradicts the spec
- P1 (High): A requirement is only partially met, or a stated acceptance criterion is unverifiable
- P2 (Medium): Minor spec gaps, ambiguous coverage, or undocumented deviations
- P3 (Low): Cosmetic spec/traceability notes`,
	}
}

// securityPersonaConfig defines the Security Persona
func securityPersonaConfig() PersonaConfig {
	return PersonaConfig{
		Name:        "Security Engineer",
		Description: "Reviews code for security vulnerabilities and threats",
		FocusAreas: []string{
			"SQL injection risks",
			"Command injection vulnerabilities",
			"Path traversal issues",
			"Hardcoded credentials",
			"Unsafe deserialization",
			"Missing authentication/authorization",
			"XSS vulnerabilities",
			"CSRF protection",
			"Cryptographic weaknesses",
			"Input validation",
		},
		Patterns: map[string]IssueSeverity{
			// P0 - Critical Security Issues
			`password\s*:?=\s*["']`:    SeverityP0, // Hardcoded passwords
			`api[_-]?key\s*:?=\s*["']`: SeverityP0, // Hardcoded API keys
			`secret\s*:?=\s*["']`:      SeverityP0, // Hardcoded secrets
			`(?i)eval\s*\(`:            SeverityP0, // eval usage (case insensitive)
			`(?i)exec\s*\(`:            SeverityP0, // exec usage (case insensitive)
			`os\.[Ss]ystem\s*\(`:       SeverityP0, // Shell command execution

			// P1 - High Security Issues
			`sql\.Exec\s*\([^,]*\+`:   SeverityP1, // SQL concatenation
			`fmt\.Sprintf.*SELECT`:    SeverityP1, // SQL string formatting
			`innerHTML\s*=`:           SeverityP1, // XSS risk
			`dangerouslySetInnerHTML`: SeverityP1, // React XSS risk
			`pickle\.loads`:           SeverityP1, // Unsafe deserialization
			`yaml\.load\(`:            SeverityP1, // Unsafe YAML load

			// P2 - Medium Security Issues
			`http://`:         SeverityP2, // Insecure HTTP
			`TODO.*security`:  SeverityP2, // Security TODOs
			`FIXME.*security`: SeverityP2, // Security FIXMEs
		},
		Prompt: `You are a Security Engineer reviewing code for vulnerabilities.
Focus on:
1. Authentication and authorization flaws
2. Input validation and sanitization
3. SQL injection and command injection risks
4. XSS and CSRF vulnerabilities
5. Hardcoded secrets and credentials
6. Unsafe deserialization
7. Cryptographic weaknesses
8. Path traversal issues

Classify findings as:
- P0 (Critical): Security vulnerabilities that could lead to data breaches, RCE, or privilege escalation
- P1 (High): Security issues that should be fixed before deployment
- P2 (Medium): Security concerns that should be addressed
- P3 (Low): Security best practices and minor improvements`,
	}
}

// performancePersonaConfig defines the Performance Persona
func performancePersonaConfig() PersonaConfig {
	return PersonaConfig{
		Name:        "Performance Engineer",
		Description: "Reviews code for performance bottlenecks and resource usage",
		FocusAreas: []string{
			"N+1 query patterns",
			"Missing database indexes",
			"Inefficient algorithms",
			"Memory leaks",
			"Unoptimized loops",
			"Excessive allocations",
			"Blocking operations",
			"Cache misses",
		},
		Patterns: map[string]IssueSeverity{
			// P1 - High Performance Issues
			`for.*range.*\{[\s\S]*?\.Query\(`: SeverityP1, // N+1 query pattern
			`time\.Sleep\(.*Second`:           SeverityP1, // Long sleep in code
			`.*\.Query\(.*\+.*\)`:             SeverityP1, // String concatenation in queries

			// P2 - Medium Performance Issues
			`defer.*\.Close\(\).*for.*range`: SeverityP2, // Defer in loop
			`append\(.*\).*for.*range`:       SeverityP2, // Append without capacity
			`json\.Marshal.*for.*range`:      SeverityP2, // Repeated marshaling
			`regexp\.MustCompile.*\)`:        SeverityP2, // Regex compilation in hot path

			// P3 - Low Performance Issues
			`fmt\.Sprint`: SeverityP3, // Inefficient string formatting
		},
		Prompt: `You are a Performance Engineer reviewing code for bottlenecks.
Focus on:
1. N+1 query patterns
2. Missing database indexes
3. Inefficient algorithms (O(n²) or worse)
4. Memory leaks and excessive allocations
5. Blocking operations on critical paths
6. Unoptimized loops
7. Cache utilization
8. Resource pooling

Classify findings as:
- P0 (Critical): Performance issues causing system crashes or extreme slowdowns
- P1 (High): Performance bottlenecks that significantly impact user experience
- P2 (Medium): Performance improvements that would benefit users
- P3 (Low): Minor optimizations and best practices`,
	}
}

// maintainabilityPersonaConfig defines the Maintainability Persona
func maintainabilityPersonaConfig() PersonaConfig {
	return PersonaConfig{
		Name:        "Tech Lead",
		Description: "Reviews code for quality, maintainability, and complexity",
		FocusAreas: []string{
			"Code complexity",
			"Function length",
			"Duplicate code",
			"Poor naming",
			"Missing documentation",
			"Tight coupling",
			"Code smells",
			"Test coverage",
		},
		Patterns: map[string]IssueSeverity{
			// P1 - High Maintainability Issues
			`func.*\{[\s\S]{3000,}\}`: SeverityP1, // Very long functions (>100 lines approx)

			// P2 - Medium Maintainability Issues
			`TODO`:  SeverityP2, // TODOs
			`FIXME`: SeverityP2, // FIXMEs
			`HACK`:  SeverityP2, // HACKs
			`XXX`:   SeverityP2, // XXX markers

			// P3 - Low Maintainability Issues
			`^\s*//.*$`:      SeverityP3, // Commented code (needs analysis)
			`var\s+[a-z]\s+`: SeverityP3, // Single letter variables
		},
		Prompt: `You are a Tech Lead reviewing code for maintainability.
Focus on:
1. Cyclomatic complexity (flag if >15)
2. Function length (flag if >100 lines)
3. Duplicate code (DRY violations)
4. Poor naming conventions
5. Missing documentation and comments
6. Tight coupling and lack of abstraction
7. Code smells (god objects, long parameter lists)
8. Test coverage for critical code

Classify findings as:
- P0 (Critical): Code that is unmaintainable or will cause major issues
- P1 (High): Complexity or quality issues that make maintenance difficult
- P2 (Medium): Code quality improvements that enhance maintainability
- P3 (Low): Style issues, minor improvements, documentation gaps`,
	}
}

// uxPersonaConfig defines the UX Persona
func uxPersonaConfig() PersonaConfig {
	return PersonaConfig{
		Name:        "UX Designer",
		Description: "Reviews code for user experience and accessibility",
		FocusAreas: []string{
			"Error messages clarity",
			"User feedback",
			"Accessibility",
			"Loading states",
			"Error handling",
			"Internationalization",
			"Responsive design",
			"User flow",
		},
		Patterns: map[string]IssueSeverity{
			// P1 - High UX Issues
			`panic\(`:             SeverityP1, // Panics (poor error handling)
			`fmt\.Println.*error`: SeverityP1, // Printing errors to console

			// P2 - Medium UX Issues
			`return.*nil.*error`: SeverityP2, // Silent failures
			`log\.Fatal`:         SeverityP2, // Fatal logs (abrupt termination)

			// P3 - Low UX Issues
			`TODO.*ux`:   SeverityP3, // UX TODOs
			`TODO.*user`: SeverityP3, // User-related TODOs
		},
		Prompt: `You are a UX Designer reviewing code for user experience.
Focus on:
1. Clear and helpful error messages
2. User feedback and loading states
3. Accessibility compliance (WCAG)
4. Error handling that guides users
5. Internationalization support
6. Responsive design considerations
7. User flow and navigation
8. Form validation and input feedback

Classify findings as:
- P0 (Critical): UX issues that break user flows or cause confusion
- P1 (High): UX problems that significantly impact usability
- P2 (Medium): UX improvements that enhance user experience
- P3 (Low): Minor UX enhancements and polish`,
	}
}

// reliabilityPersonaConfig defines the Reliability Persona
func reliabilityPersonaConfig() PersonaConfig {
	return PersonaConfig{
		Name:        "SRE",
		Description: "Reviews code for reliability, error handling, and edge cases",
		FocusAreas: []string{
			"Error handling",
			"Edge case coverage",
			"Null/nil checks",
			"Resource cleanup",
			"Retry logic",
			"Circuit breakers",
			"Timeout handling",
			"Graceful degradation",
		},
		Patterns: map[string]IssueSeverity{
			// P0 - Critical Reliability Issues
			`/\s*0`:                    SeverityP0, // Division by zero risk
			`\[\w+\].*without.*bounds`: SeverityP0, // Array access without bounds check

			// P1 - High Reliability Issues
			`if\s+err\s*!=\s*nil\s*\{[\s\S]*?\}[\s\S]*?return[\s\S]*?nil`: SeverityP1, // Swallowing errors
			`_\s*=.*error`:                SeverityP1, // Ignoring errors
			`defer.*Close\(\).*err\s*:?=`: SeverityP1, // Not checking Close errors

			// P2 - Medium Reliability Issues
			`for\s*\{[\s\S]*?\}.*without.*break`:      SeverityP2, // Infinite loop risk
			`select\s*\{[\s\S]*?\}.*without.*default`: SeverityP2, // Select without default
			`time\.After\(.*Hour`:                     SeverityP2, // Very long timeouts

			// P3 - Low Reliability Issues
			`TODO.*error`: SeverityP3, // Error handling TODOs
			`FIXME.*edge`: SeverityP3, // Edge case FIXMEs
		},
		Prompt: `You are an SRE reviewing code for reliability and resilience.
Focus on:
1. Comprehensive error handling
2. Edge case coverage (nil, empty, boundary conditions)
3. Resource cleanup (connections, files, locks)
4. Retry logic and exponential backoff
5. Circuit breakers and fallbacks
6. Timeout handling
7. Graceful degradation
8. Monitoring and observability hooks

Classify findings as:
- P0 (Critical): Reliability issues that could cause crashes or data loss
- P1 (High): Error handling gaps that impact system stability
- P2 (Medium): Reliability improvements that increase robustness
- P3 (Low): Minor reliability enhancements and edge cases`,
	}
}

// checkPattern checks if content matches a pattern and returns an issue
func checkPattern(content, pattern string, severity IssueSeverity, filePath string, persona PersonaType) (bool, ReviewIssue) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, ReviewIssue{}
	}

	if re.MatchString(content) {
		return true, ReviewIssue{
			Persona:  persona,
			Severity: severity,
			Category: "pattern_match",
			Message:  fmt.Sprintf("Detected pattern: %s", pattern),
			FilePath: filePath,
		}
	}

	return false, ReviewIssue{}
}

// AllPersonas returns all available persona types, in review order: the Stage 1
// spec-compliance persona first, then the Stage 2 code-quality personas.
func AllPersonas() []PersonaType {
	return append([]PersonaType{PersonaSpecCompliance}, CodeQualityPersonas()...)
}

// CodeQualityPersonas returns the Stage 2 (code-quality) personas.
func CodeQualityPersonas() []PersonaType {
	return []PersonaType{
		PersonaSecurity,
		PersonaPerformance,
		PersonaMaintainability,
		PersonaUX,
		PersonaReliability,
	}
}

// GetPersonaName returns the human-readable name for a persona
func GetPersonaName(persona PersonaType) string {
	config := GetPersonaConfig(persona)
	return config.Name
}

// GetPersonaDescription returns the description for a persona
func GetPersonaDescription(persona PersonaType) string {
	config := GetPersonaConfig(persona)
	return config.Description
}
