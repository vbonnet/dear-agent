package specguard

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/vbonnet/dear-agent/internal/repoinventory"

	gherkin "github.com/cucumber/gherkin/go/v26"
	"github.com/cucumber/messages/go/v21"
	"github.com/vbonnet/dear-agent/internal/earslint"
)

type issueCategory string

const (
	issueEARS issueCategory = "ears"
	issueBDD  issueCategory = "bdd"
)

type parseIssue struct {
	category issueCategory
	code     string
	line     int
	message  string
}

const (
	maxDocumentIssues = 1024
	maxDocumentLinks  = 4096
	maxDocumentIDs    = 4096
	// MaxSpecOwnerBytes bounds the complete SPEC.owner structural edge.
	MaxSpecOwnerBytes = 4096
)

func (issue parseIssue) finding(filePath string) Finding {
	return Finding{Code: issue.code, Path: filePath, Line: issue.line, Message: issue.message}
}

type specDocument struct {
	features []string
	issues   []parseIssue
}

func (document *specDocument) addIssue(issue parseIssue) {
	document.issues = appendBoundedIssue(document.issues, issue)
}

type featureDocument struct {
	primary             []string
	related             []string
	executableScenarios int
	issues              []parseIssue
}

func (document *featureDocument) addIssue(issue parseIssue) {
	document.issues = appendBoundedIssue(document.issues, issue)
}

func appendBoundedIssue(issues []parseIssue, issue parseIssue) []parseIssue {
	if len(issues) < maxDocumentIssues-1 {
		return append(issues, issue)
	}
	if len(issues) == maxDocumentIssues-1 {
		return append(issues, parseIssue{category: issueBDD, code: "document-issue-limit", message: "document diagnostics exceeded the safety limit"})
	}
	return issues
}

func (document featureDocument) relatedSpecs() []string {
	result := make([]string, 0, len(document.primary)+len(document.related))
	result = append(result, document.primary...)
	result = append(result, document.related...)
	sort.Strings(result)
	return result
}

var (
	requirementIDPattern = regexp.MustCompile(`^([A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*-[A-Z0-9]*\d+)\s+`)
	headingPattern       = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
	bddEntryPattern      = regexp.MustCompile("^- (?:(?:([^`:]+): )?)`([^`]+\\.feature)`(?:\\s+([^`]*))?$")
	canonicalLinter      = mustCanonicalLinter()
	traceabilityHeadings = map[string]bool{
		"bdd traceability":          true,
		"package test traceability": true,
		"test traceability":         true,
		"traceability":              true,
	}
	traceabilityLabels = map[string]bool{
		"":                        true,
		"bdd":                     true,
		"bdd feature":             true,
		"bdd tests":               true,
		"command bdd":             true,
		"cross-surface bdd":       true,
		"cross-surface contracts": true,
		"feature":                 true,
		"related feature":         true,
		"status bdd":              true,
		"strict-spec linkage":     true,
		"strictness bdd":          true,
	}
)

func mustCanonicalLinter() *earslint.Linter {
	linter, err := earslint.New(earslint.DefaultConfig())
	if err != nil {
		panic(err)
	}
	return linter
}

//nolint:gocyclo // One pass keeps Markdown fence, EARS, and traceability line numbers aligned.
func parseSpecDocument(filePath string, body []byte) specDocument {
	document := specDocument{features: []string{}, issues: []parseIssue{}}
	if !utf8.Valid(body) {
		document.addIssue(parseIssue{category: issueEARS, code: "invalid-utf8", message: "SPEC content must be valid UTF-8"})
		return document
	}
	inTraceability := false
	seenFeatures := make(map[string]bool)
	seenIDs := make(map[string]bool)
	validRequirements := 0
	fence := markdownFence{}
	for index, rawLine := range strings.Split(string(body), "\n") {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(rawLine)
		if fence.skip(trimmed) {
			continue
		}
		if heading := headingPattern.FindStringSubmatch(trimmed); len(heading) != 0 && len(heading[1]) <= 2 {
			inTraceability = heading[1] == "##" && traceabilityHeadings[strings.ToLower(strings.TrimSpace(heading[2]))]
		}
		linted, err := canonicalLinter.Lint(filePath, strings.NewReader(rawLine+"\n"))
		if err != nil {
			document.addIssue(parseIssue{category: issueEARS, code: "ears-read", line: lineNumber, message: "EARS requirement could not be read"})
		} else if linted.TotalRequirements != 0 {
			normalized := earslint.NormalizeRequirementLine(rawLine)
			match := requirementIDPattern.FindStringSubmatch(normalized)
			switch {
			case linted.TotalRequirements != 1 || linted.ValidRequirements != 1:
				document.addIssue(parseIssue{category: issueEARS, code: "invalid-ears", line: lineNumber, message: "requirement must match exactly one canonical EARS form"})
			case len(match) == 0:
				document.addIssue(parseIssue{category: issueEARS, code: "missing-requirement-id", line: lineNumber, message: "EARS requirement must begin with a stable requirement ID"})
			case seenIDs[match[1]]:
				document.addIssue(parseIssue{category: issueEARS, code: "duplicate-requirement-id", line: lineNumber, message: fmt.Sprintf("requirement ID %q is duplicated in this SPEC", match[1])})
			case len(seenIDs) >= maxDocumentIDs:
				document.addIssue(parseIssue{category: issueEARS, code: "requirement-count-limit", line: lineNumber, message: "identified requirement count exceeded the safety limit"})
			default:
				seenIDs[match[1]] = true
				validRequirements++
			}
		}
		if !inTraceability {
			continue
		}
		match := bddEntryPattern.FindStringSubmatch(trimmed)
		if len(match) == 0 {
			if strings.HasPrefix(trimmed, "-") && strings.Contains(trimmed, ".feature") {
				document.addIssue(parseIssue{category: issueBDD, code: "malformed-bdd-reference", line: lineNumber, message: "BDD traceability entry must contain one repository-relative .feature path in backticks"})
			}
			continue
		}
		if !traceabilityLabels[strings.ToLower(strings.TrimSpace(match[1]))] {
			document.addIssue(parseIssue{category: issueBDD, code: "malformed-bdd-reference", line: lineNumber, message: "BDD traceability entry uses an unsupported label"})
			continue
		}
		featurePath := match[2]
		if failure := validateGitPath(featurePath, defaultMaxPathBytes); failure != nil || !isFeaturePath(featurePath) {
			document.addIssue(parseIssue{category: issueBDD, code: "malformed-bdd-reference", line: lineNumber, message: "BDD traceability path is not a safe repository-relative .feature path"})
			continue
		}
		if seenFeatures[featurePath] {
			document.addIssue(parseIssue{category: issueBDD, code: "duplicate-bdd-reference", line: lineNumber, message: fmt.Sprintf("BDD feature %q is referenced more than once", featurePath)})
			continue
		}
		if len(document.features) >= maxDocumentLinks {
			document.addIssue(parseIssue{category: issueBDD, code: "bdd-link-limit", line: lineNumber, message: "BDD feature link count exceeded the safety limit"})
			continue
		}
		seenFeatures[featurePath] = true
		document.features = append(document.features, featurePath)
	}
	if fence.length != 0 {
		document.addIssue(parseIssue{category: issueBDD, code: "unterminated-markdown-fence", message: "SPEC contains an unterminated Markdown fence"})
	}
	if validRequirements == 0 {
		document.addIssue(parseIssue{category: issueEARS, code: "missing-ears-requirement", message: "changed SPEC must contain at least one valid, identified EARS requirement"})
	}
	sort.Strings(document.features)
	return document
}

func parseFeatureDocument(body []byte) featureDocument {
	document := featureDocument{primary: []string{}, related: []string{}, issues: []parseIssue{}}
	if !utf8.Valid(body) {
		document.addIssue(parseIssue{category: issueBDD, code: "invalid-utf8", message: "BDD feature content must be valid UTF-8"})
		return document
	}
	identifier := 0
	gherkinDocument, err := gherkin.ParseGherkinDocument(bytes.NewReader(body), func() string {
		identifier++
		return fmt.Sprintf("specguard-%d", identifier)
	})
	if err != nil || gherkinDocument == nil || gherkinDocument.Feature == nil || gherkinDocument.Feature.Location == nil {
		document.addIssue(parseIssue{category: issueBDD, code: "invalid-gherkin", message: "BDD feature must parse as a valid Gherkin document"})
		return document
	}

	featureLine := int(gherkinDocument.Feature.Location.Line)
	parseFeatureOwnership(&document, gherkinDocument.Comments, featureLine)
	assessFeatureChildren(&document, gherkinDocument.Feature.Children)
	sort.Strings(document.primary)
	sort.Strings(document.related)
	return document
}

func parseFeatureOwnership(document *featureDocument, comments []*messages.Comment, featureLine int) {
	seen := make(map[string]bool)
	for _, comment := range comments {
		if comment == nil || comment.Location == nil {
			continue
		}
		lineNumber := int(comment.Location.Line)
		trimmed := strings.TrimSpace(comment.Text)
		if value, primary, marker := featureSpecMarker(trimmed); marker {
			if lineNumber >= featureLine {
				document.addIssue(parseIssue{category: issueBDD, code: "late-spec-marker", line: lineNumber, message: "SPEC ownership markers must precede the Feature header"})
				continue
			}
			if failure := validateGitPath(value, defaultMaxPathBytes); failure != nil || !isSpecPath(value) {
				document.addIssue(parseIssue{category: issueBDD, code: "malformed-feature-spec-reference", line: lineNumber, message: "feature SPEC marker must name a safe repository-relative SPEC.md path"})
				continue
			}
			if seen[value] {
				document.addIssue(parseIssue{category: issueBDD, code: "duplicate-feature-spec-reference", line: lineNumber, message: fmt.Sprintf("SPEC %q is linked more than once", value)})
				continue
			}
			if len(document.primary)+len(document.related) >= maxDocumentLinks {
				document.addIssue(parseIssue{category: issueBDD, code: "spec-link-limit", line: lineNumber, message: "feature SPEC link count exceeded the safety limit"})
				continue
			}
			seen[value] = true
			if primary {
				document.primary = append(document.primary, value)
			} else {
				document.related = append(document.related, value)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "# SPEC ") || strings.HasPrefix(trimmed, "# RELATED-SPEC ") {
			document.addIssue(parseIssue{category: issueBDD, code: "malformed-feature-spec-reference", line: lineNumber, message: "feature SPEC marker must use an exact colon-delimited form"})
		}
	}
}

func assessFeatureChildren(document *featureDocument, children []*messages.FeatureChild) {
	for _, child := range children {
		if child == nil {
			continue
		}
		if child.Scenario != nil {
			assessRunnableScenario(document, child.Scenario)
		}
		if child.Rule == nil {
			continue
		}
		for _, ruleChild := range child.Rule.Children {
			if ruleChild != nil && ruleChild.Scenario != nil {
				assessRunnableScenario(document, ruleChild.Scenario)
			}
		}
	}
}

func assessRunnableScenario(document *featureDocument, scenario *messages.Scenario) {
	line := 0
	if scenario.Location != nil {
		line = int(scenario.Location.Line)
	}
	keyword := strings.TrimSpace(scenario.Keyword)
	isOutline := keyword == "Scenario Outline"
	if keyword != "Scenario" && !isOutline {
		document.addIssue(parseIssue{category: issueBDD, code: "unsupported-scenario-kind", line: line, message: "BDD execution requires a concrete Scenario or Scenario Outline"})
		return
	}

	hasAssertion := scenarioHasThenAssertion(scenario.Steps)
	if !hasAssertion {
		document.addIssue(parseIssue{category: issueBDD, code: "scenario-without-assertion", line: line, message: "BDD scenario must contain a Then assertion; only And or But steps following Then continue the assertion phase"})
	}

	hasExamples := !isOutline
	if isOutline {
		hasExamples = scenarioHasNonemptyExamples(scenario.Examples)
		if !hasExamples {
			document.addIssue(parseIssue{category: issueBDD, code: "outline-without-examples", line: line, message: "BDD Scenario Outline must contain a nonempty Examples table"})
		}
	}
	if hasAssertion && hasExamples {
		document.executableScenarios++
	}
}

func scenarioHasThenAssertion(steps []*messages.Step) bool {
	assertionSteps := 0
	afterThen := false
	for _, step := range steps {
		if step == nil {
			continue
		}
		switch step.KeywordType {
		case messages.StepKeywordType_OUTCOME:
			assertionSteps++
			afterThen = true
		case messages.StepKeywordType_CONJUNCTION:
			conjunction := strings.TrimSpace(step.Keyword)
			if afterThen && (conjunction == "And" || conjunction == "But") {
				assertionSteps++
			} else {
				afterThen = false
			}
		case messages.StepKeywordType_UNKNOWN, messages.StepKeywordType_CONTEXT, messages.StepKeywordType_ACTION:
			afterThen = false
		}
	}
	return assertionSteps != 0
}

func scenarioHasNonemptyExamples(examplesList []*messages.Examples) bool {
	for _, examples := range examplesList {
		if examples != nil && examples.TableHeader != nil && len(examples.TableBody) != 0 {
			return true
		}
	}
	return false
}

func featureSpecMarker(line string) (value string, primary, ok bool) {
	if value, ok := strings.CutPrefix(line, "# SPEC:"); ok {
		return strings.TrimSpace(value), true, true
	}
	if value, ok := strings.CutPrefix(line, "# RELATED-SPEC:"); ok {
		return strings.TrimSpace(value), false, true
	}
	return "", false, false
}

type markdownFence struct {
	marker byte
	length int
}

// skip reports whether a Markdown fence delimiter or fenced line should be
// ignored. Closers must use the opening marker and at least its run length.
func (fence *markdownFence) skip(line string) bool {
	if fence.length == 0 {
		marker, length := fencePrefix(line)
		if length < 3 {
			return false
		}
		fence.marker = marker
		fence.length = length
		return true
	}
	marker, length := fencePrefix(line)
	if marker == fence.marker && length >= fence.length && strings.TrimSpace(line[length:]) == "" {
		fence.marker = 0
		fence.length = 0
	}
	return true
}

func fencePrefix(line string) (byte, int) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	marker := line[0]
	length := 0
	for length < len(line) && line[length] == marker {
		length++
	}
	return marker, length
}

//nolint:gocyclo // Explicit path rejection rules are a security admission gate.
func validateGitPath(filePath string, limit int) *guardFailure {
	if filePath == "" || len(filePath) > limit || !utf8.ValidString(filePath) ||
		strings.HasPrefix(filePath, "/") || strings.Contains(filePath, "\\") ||
		path.Clean(filePath) != filePath || filePath == "." {
		return fail("invalid-git-path", filePath, "Git path must be a bounded, normalized repository-relative path")
	}
	for part := range strings.SplitSeq(filePath, "/") {
		if part == "" || part == "." || part == ".." || part == ".git" {
			return fail("path-escape", filePath, "Git path contains a forbidden path component")
		}
	}
	for _, value := range filePath {
		if value < 0x20 || value == 0x7f {
			return fail("invalid-git-path", filePath, "Git path contains a control character")
		}
	}
	return nil
}

func isGovernedPath(filePath string) bool {
	return repoinventory.IsGovernedPath(filePath) && (isSpecPath(filePath) || isSpecOwnerPath(filePath) || isFeaturePath(filePath))
}
func isSpecPath(filePath string) bool      { return path.Base(filePath) == "SPEC.md" }
func isSpecOwnerPath(filePath string) bool { return path.Base(filePath) == "SPEC.owner" }
func isFeaturePath(filePath string) bool   { return strings.HasSuffix(filePath, ".feature") }

// ParseSpecOwner validates the language-neutral structural edge from one
// implementation directory to its canonical shared contract.
func ParseSpecOwner(data []byte, localSpecPath string) (string, error) {
	value, err := parseSpecOwnerValue(data)
	if err != nil {
		return "", err
	}
	if err := validateSpecOwnerTarget(value, localSpecPath); err != nil {
		return "", err
	}
	return value, nil
}

func parseSpecOwnerValue(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxSpecOwnerBytes || !utf8.Valid(data) {
		return "", fmt.Errorf("SPEC.owner must contain one bounded UTF-8 repository-relative SPEC.md path")
	}
	value := string(data)
	if trimmed, ok := strings.CutSuffix(value, "\n"); ok {
		value = trimmed
	}
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\\:\x00\r\n*?[]{}#~") {
		return "", fmt.Errorf("SPEC.owner must contain exactly one canonical repository-relative SPEC.md path")
	}
	return value, nil
}

func validateSpecOwnerTarget(value, localSpecPath string) error {
	if !fs.ValidPath(value) || path.Clean(value) != value || path.Base(value) != "SPEC.md" {
		return fmt.Errorf("SPEC.owner path %q is not a canonical repository-relative SPEC.md path", value)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("SPEC.owner path contains a control character")
		}
	}
	if value == localSpecPath {
		return fmt.Errorf("SPEC.owner must point to a shared owner outside the implementation directory")
	}
	if IsHarnessRegistrationOwner(value) {
		return fmt.Errorf("SPEC.owner path %q is harness-owned rather than a neutral product or domain owner", value)
	}
	return nil
}
