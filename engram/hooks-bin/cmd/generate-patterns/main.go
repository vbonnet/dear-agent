// Command generate-patterns reads the unified YAML patterns file and generates
// the Go patterns.go file used by the bash-blocker validator.
//
// Usage:
//
//	go run ./cmd/generate-patterns -yaml ../../patterns/bash-anti-patterns.yaml -output patterns.go
//	go run ./cmd/generate-patterns -yaml ../../patterns/bash-anti-patterns.yaml --lint
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"go.yaml.in/yaml/v3"
)

// PatternDB is the top-level YAML structure.
type PatternDB struct {
	Patterns []Pattern `yaml:"patterns"`
}

// Pattern represents a single entry in the YAML patterns database.
type Pattern struct {
	ID               string   `yaml:"id"`
	Order            int      `yaml:"order"`
	RE2Regex         string   `yaml:"re2_regex"`
	PatternName      string   `yaml:"pattern_name"`
	Remediation      string   `yaml:"remediation"`
	Regex            string   `yaml:"regex"`
	Reason           string   `yaml:"reason"`
	Alternative      string   `yaml:"alternative"`
	Examples         []string `yaml:"examples"`
	ShouldNotMatch   []string `yaml:"should_not_match"`
	Severity         string   `yaml:"severity"`
	Tier2Validation  bool     `yaml:"tier2_validation"`
	Tier1Example     string   `yaml:"tier1_example"`
	Relaxed          bool     `yaml:"relaxed"`
	RelaxedDate      string   `yaml:"relaxed_date"`
	RelaxedReason    string   `yaml:"relaxed_reason"`
	ConsolidatedInto string   `yaml:"consolidated_into"`
}

func main() {
	yamlPath := flag.String("yaml", "../../patterns/bash-anti-patterns.yaml", "path to YAML patterns file")
	outputPath := flag.String("output", "", "output file path (default: stdout)")
	lint := flag.Bool("lint", false, "validate patterns and examples, then exit")
	flag.Parse()

	data, err := os.ReadFile(*yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading YAML: %v\n", err)
		os.Exit(1)
	}

	var db PatternDB
	if err := yaml.Unmarshal(data, &db); err != nil {
		fmt.Fprintf(os.Stderr, "error: parsing YAML: %v\n", err)
		os.Exit(1)
	}

	// Filter: only patterns with non-empty re2_regex AND relaxed is not true.
	var active []Pattern
	for _, p := range db.Patterns {
		if p.RE2Regex != "" && !p.Relaxed {
			active = append(active, p)
		}
	}

	// Sort by order ascending.
	sort.Slice(active, func(i, j int) bool {
		return active[i].Order < active[j].Order
	})

	// Validate.
	errors := validate(active)
	if len(errors) > 0 {
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "error: %s\n", e)
		}
		os.Exit(1)
	}

	if *lint {
		lintErrors := lintExamples(active)
		if len(lintErrors) > 0 {
			for _, e := range lintErrors {
				fmt.Fprintf(os.Stderr, "lint: %s\n", e)
			}
			fmt.Fprintf(os.Stderr, "\n%d lint error(s)\n", len(lintErrors))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "ok: %d patterns validated, all examples pass\n", len(active))
		os.Exit(0)
	}

	// Generate output.
	var w io.Writer = os.Stdout
	if *outputPath != "" {
		f, err := os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: creating output: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	if err := generate(w, active); err != nil {
		// fmt.Fprintf goes to stderr; close any open file before exit so the deferred close does run
		fmt.Fprintf(os.Stderr, "error: generating output: %v\n", err)
		if f, ok := w.(*os.File); ok && f != os.Stdout {
			f.Close()
		}
		os.Exit(1)
	}
}

// validate checks that all patterns compile, have required fields, and have
// unique order values.
func validate(patterns []Pattern) []string {
	var errs []string
	seen := make(map[int]string) // order -> id

	for _, p := range patterns {
		// Required fields.
		if p.PatternName == "" {
			errs = append(errs, fmt.Sprintf("pattern %q: missing pattern_name", p.ID))
		}
		if p.Remediation == "" {
			errs = append(errs, fmt.Sprintf("pattern %q: missing remediation", p.ID))
		}

		// RE2 compilation.
		if _, err := regexp.Compile(p.RE2Regex); err != nil {
			errs = append(errs, fmt.Sprintf("pattern %q: re2_regex does not compile: %v", p.ID, err))
		}

		// Duplicate order.
		if prev, ok := seen[p.Order]; ok {
			errs = append(errs, fmt.Sprintf("pattern %q: duplicate order %d (also used by %q)", p.ID, p.Order, prev))
		}
		seen[p.Order] = p.ID
	}

	return errs
}

// lintExamples checks that examples match and should_not_match entries do not.
func lintExamples(patterns []Pattern) []string {
	var errs []string

	for _, p := range patterns {
		re, err := regexp.Compile(p.RE2Regex)
		if err != nil {
			// Already reported in validate.
			continue
		}

		for _, ex := range p.Examples {
			if !re.MatchString(ex) {
				errs = append(errs, fmt.Sprintf("pattern %q (order %d): example should match but doesn't: %q", p.ID, p.Order, ex))
			}
		}

		for _, ex := range p.ShouldNotMatch {
			if re.MatchString(ex) {
				errs = append(errs, fmt.Sprintf("pattern %q (order %d): should_not_match but does: %q", p.ID, p.Order, ex))
			}
		}
	}

	return errs
}

// regexLiteral returns a Go source literal for the regex string.
// Most regexes use backtick quoting. If the regex contains a backtick,
// we use "\x60" escaping instead.
func regexLiteral(re string) string {
	if strings.Contains(re, "`") {
		// Escape for double-quoted string.
		escaped := strings.ReplaceAll(re, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, "`", `\x60`)
		return `"` + escaped + `"`
	}
	return "`" + re + "`"
}

// goStringLiteral returns a Go double-quoted string literal, properly escaped.
func goStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	// Preserve literal newlines as \n in the Go source.
	s = strings.ReplaceAll(s, "\n", `\n`)
	return `"` + s + `"`
}

const tmplSource = `// Code generated by generate-patterns; DO NOT EDIT.
// Source: patterns/bash-anti-patterns.yaml

package validator

import "regexp"

// Forbidden patterns compiled at package init (zero per-request overhead)
// IMPORTANT: Pattern order matters - first match wins!
// More specific patterns should come before more general ones.
var forbiddenPatterns = []*regexp.Regexp{
{{- range $i, $p := .Patterns }}
	// {{ $i }}. {{ $p.PatternName }}
	regexp.MustCompile({{ regexLiteral $p.RE2Regex }}),
{{- end }}
}

var patternNames = []string{
{{- range .Patterns }}
	{{ goStringLiteral .PatternName }},
{{- end }}
}

// remediations provides actionable fix instructions for each forbidden pattern.
// Index matches forbiddenPatterns/patternNames arrays.
var remediations = []string{
{{- range .Patterns }}
	{{ goStringLiteral .Remediation }},
{{- end }}
}
`

type templateData struct {
	Patterns []Pattern
}

func generate(w io.Writer, patterns []Pattern) error {
	funcMap := template.FuncMap{
		"regexLiteral":    regexLiteral,
		"goStringLiteral": goStringLiteral,
	}

	tmpl, err := template.New("patterns.go").Funcs(funcMap).Parse(tmplSource)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	return tmpl.Execute(w, templateData{Patterns: patterns})
}
