package specpackage

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"
)

const authenticatedSpecauditPath = "<distribution-root>/bin/specaudit"
const maxMarkdownLinks = 4096
const maxMarkdownCodeBlocks = 128
const maxMarkdownSpecauditReferences = 256

type portableShellCommands uint8

const (
	portableShellInventory portableShellCommands = 1 << iota
	portableShellValidate
	portableShellRender
	portableShellAll = portableShellInventory | portableShellValidate | portableShellRender
)

type markdownApproval struct {
	sha256        string
	shellCommands portableShellCommands
}

// markdownApprovals is deliberately review-maintained rather than derived at
// runtime. It approves exact source bytes without copying their normative prose;
// the compact capability bitset selects the generic shell grammar below.
var markdownApprovals = map[string]markdownApproval{
	"skills/audit-specs/SKILL.md": {
		sha256:        "737e803ec6b7a1f2834581a35544befaed93bd02261060de54e285149e6826a8",
		shellCommands: portableShellAll,
	},
	"skills/audit-specs/references/audit-verdicts.md": {
		sha256: "1dd3342ba761da099439e9734b28d368588862835f21933a570ce8f6416eb38e",
	},
	"skills/audit-specs/references/report-schema.md": {
		sha256:        "870998db344ad96bc8613d4b8d26ec5ce50661403d8d63f516684951821b0b48",
		shellCommands: portableShellAll,
	},
	"skills/write-spec/SKILL.md": {
		sha256: "5a32042aaac1995940136d9e1544bf995d1b74820ef199e89787f8c404680bac",
	},
	"skills/write-spec/references/contract-model.md": {
		sha256: "772165307e8529c79da2d0860abec87c14a94d5a7941ca93991ba8d3bc3d5790",
	},
	"skills/write-spec/references/ears-and-bdd.md": {
		sha256: "9bfc20f0059501a96ac1704480795394c8fcbbaad9ab8aa8897863198b7e0d06",
	},
}

func validateMarkdownClosure(files map[string][]byte) error {
	markdownCount := 0
	for _, entry := range payloadLayout {
		if entry.role != "skill" && entry.role != "reference" {
			continue
		}
		markdownCount++
		content, ok := files[entry.packagePath]
		if !ok {
			return fmt.Errorf("missing Markdown payload %q", entry.packagePath)
		}
		if !utf8.Valid(content) {
			return fmt.Errorf("markdown payload %q is not valid UTF-8", entry.packagePath)
		}
		if entry.skillName != "" {
			if err := validateSkillName(entry.packagePath, content, entry.skillName); err != nil {
				return err
			}
		}
		if err := validateMarkdownLinks(entry.packagePath, content); err != nil {
			return err
		}
		if err := validatePortableMarkdownCommands(entry.packagePath, content); err != nil {
			return err
		}
		if err := validateApprovedMarkdownContent(entry.packagePath, content); err != nil {
			return err
		}
	}
	if len(markdownApprovals) != markdownCount {
		return fmt.Errorf("approved Markdown digest registry has %d entries, want %d", len(markdownApprovals), markdownCount)
	}
	return nil
}

func validateApprovedMarkdownContent(packagePath string, content []byte) error {
	approval, declared := markdownApprovals[packagePath]
	if !declared {
		return fmt.Errorf("markdown payload %q has no approved content digest", packagePath)
	}
	digest := sha256.Sum256(content)
	if fmt.Sprintf("%x", digest) != approval.sha256 {
		return fmt.Errorf("markdown payload %q does not match its approved content digest", packagePath)
	}
	return nil
}

func codeSpanText(span *ast.CodeSpan, content []byte) string {
	var result strings.Builder
	for child := span.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		result.Write(textNode.Segment.Value(content))
	}
	return result.String()
}

func validatePortableMarkdownCommands(packagePath string, content []byte) error {
	approval, declared := markdownApprovals[packagePath]
	if !declared {
		return fmt.Errorf("markdown payload %q has no approved content digest", packagePath)
	}
	seenShellCommands := portableShellCommands(0)
	document := goldmark.DefaultParser().Parse(text.NewReader(content))
	blockCount := 0
	specauditReferenceCount := 0
	prose, err := markdownProseText(document, content)
	if err != nil {
		return fmt.Errorf("read visible prose from markdown payload %q: %w", packagePath, err)
	}
	if err := validateProseSpecauditReferences(packagePath, prose, &specauditReferenceCount); err != nil {
		return err
	}
	err = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.CodeSpan:
			value := strings.TrimSpace(codeSpanText(typed, content))
			if err := validateInlineSpecauditReference(packagePath, value, &specauditReferenceCount); err != nil {
				return ast.WalkStop, err
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			blockCount++
			if blockCount > maxMarkdownCodeBlocks {
				return ast.WalkStop, fmt.Errorf("markdown payload %q exceeds the %d-code-block bound", packagePath, maxMarkdownCodeBlocks)
			}
			if insideBlockquote(typed) {
				return ast.WalkStop, fmt.Errorf("markdown payload %q contains an ambiguous blockquoted code block", packagePath)
			}
			language := strings.ToLower(strings.TrimSpace(string(typed.Language(content))))
			info := ""
			if typed.Info != nil {
				info = string(typed.Info.Text(content))
			}
			if info != language {
				return ast.WalkStop, fmt.Errorf("markdown payload %q contains noncanonical code block info %q", packagePath, info)
			}
			body := string(typed.Text(content))
			switch language {
			case "sh", "shell", "bash", "zsh", "console", "terminal":
				if language != "sh" {
					return ast.WalkStop, fmt.Errorf("markdown payload %q contains noncanonical shell block language %q", packagePath, language)
				}
				commands, err := validatePortableShellBlock(packagePath, body, &specauditReferenceCount)
				if err != nil {
					return ast.WalkStop, err
				}
				if seenShellCommands&commands != 0 {
					return ast.WalkStop, fmt.Errorf("markdown payload %q repeats an approved packaged shell command", packagePath)
				}
				seenShellCommands |= commands
			case "text":
				if _, err := visitSpecauditReferences(packagePath, body, &specauditReferenceCount, func(int) error {
					return fmt.Errorf("markdown payload %q text block is not an approved packaged data template", packagePath)
				}); err != nil {
					return ast.WalkStop, err
				}
			case "json":
				if err := validateJSONSpecauditReferences(packagePath, body, &specauditReferenceCount); err != nil {
					return ast.WalkStop, err
				}
			case "":
				return ast.WalkStop, fmt.Errorf("markdown payload %q contains an ambiguous untyped code block", packagePath)
			default:
				return ast.WalkStop, fmt.Errorf("markdown payload %q contains unsupported code block language %q", packagePath, language)
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			blockCount++
			if blockCount > maxMarkdownCodeBlocks {
				return ast.WalkStop, fmt.Errorf("markdown payload %q exceeds the %d-code-block bound", packagePath, maxMarkdownCodeBlocks)
			}
			return ast.WalkStop, fmt.Errorf("markdown payload %q contains an ambiguous indented code block", packagePath)
		default:
			return ast.WalkContinue, nil
		}
	})
	if err != nil {
		return err
	}
	if seenShellCommands != approval.shellCommands {
		return fmt.Errorf("markdown payload %q has approved shell command set %d, want %d", packagePath, seenShellCommands, approval.shellCommands)
	}
	return nil
}

func validatePortableShellBlock(packagePath, body string, count *int) (portableShellCommands, error) {
	if _, err := visitSpecauditReferences(packagePath, body, count, func(int) error { return nil }); err != nil {
		return 0, err
	}

	file, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(body), packagePath)
	if err != nil {
		return 0, fmt.Errorf("markdown payload %q shell block is not valid POSIX shell: %w", packagePath, err)
	}
	if len(file.Stmts) == 0 {
		return 0, fmt.Errorf("markdown payload %q contains an empty shell block", packagePath)
	}

	commands := portableShellCommands(0)
	for _, statement := range file.Stmts {
		command, err := validatePortableShellStatement(packagePath, statement)
		if err != nil {
			return 0, err
		}
		if commands&command != 0 {
			return 0, fmt.Errorf("markdown payload %q repeats an approved packaged shell command", packagePath)
		}
		commands |= command
	}
	return commands, nil
}

func validatePortableShellStatement(packagePath string, statement *syntax.Stmt) (portableShellCommands, error) {
	if statement.Negated || statement.Background || statement.Coprocess || statement.Disown || statement.Semicolon.IsValid() {
		return 0, fmt.Errorf("markdown payload %q shell block is not an approved packaged command template: statement control syntax", packagePath)
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 {
		return 0, fmt.Errorf("markdown payload %q shell block is not an approved static command", packagePath)
	}
	arguments := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		literal, err := staticShellWord(word)
		if err != nil {
			return 0, fmt.Errorf("markdown payload %q shell block contains a dynamic word: %w", packagePath, err)
		}
		arguments = append(arguments, literal)
	}

	var command portableShellCommands
	var expectedArguments []string
	var expectedOutput string
	if len(arguments) >= 2 {
		switch arguments[1] {
		case "inventory":
			command = portableShellInventory
			expectedArguments = []string{
				authenticatedSpecauditPath, "inventory",
				"-repo", "<repository-path>",
				"-repository", "<owner/name>",
				"-revision", "<40-hex-sha>",
			}
			expectedOutput = "inventory.json"
		case "validate":
			command = portableShellValidate
			expectedArguments = []string{
				authenticatedSpecauditPath, "validate",
				"-input", "findings.json",
				"-inventory", "inventory.json",
				"-repo", "<repository-path>",
			}
		case "render":
			command = portableShellRender
			expectedArguments = []string{
				authenticatedSpecauditPath, "render",
				"-input", "findings.json",
				"-inventory", "inventory.json",
				"-repo", "<repository-path>",
			}
			expectedOutput = "report.html"
		}
	}
	if command == 0 || !equalStrings(arguments, expectedArguments) {
		return 0, fmt.Errorf("markdown payload %q shell block is not an approved packaged command template", packagePath)
	}
	if err := validatePortableShellRedirection(packagePath, statement.Redirs, expectedOutput); err != nil {
		return 0, err
	}
	return command, nil
}

func validatePortableShellRedirection(packagePath string, redirections []*syntax.Redirect, expectedOutput string) error {
	if expectedOutput == "" {
		if len(redirections) != 0 {
			return fmt.Errorf("markdown payload %q shell block contains an unapproved redirection", packagePath)
		}
		return nil
	}
	if len(redirections) != 1 {
		return fmt.Errorf("markdown payload %q shell block does not have its one approved output redirection", packagePath)
	}
	redirection := redirections[0]
	if redirection.Op != syntax.RdrOut || redirection.N != nil || redirection.Hdoc != nil {
		return fmt.Errorf("markdown payload %q shell block contains an unapproved redirection", packagePath)
	}
	output, err := staticShellWord(redirection.Word)
	if err != nil || output != expectedOutput {
		return fmt.Errorf("markdown payload %q shell block does not redirect to its approved output", packagePath)
	}
	return nil
}

func staticShellWord(word *syntax.Word) (string, error) {
	if word == nil {
		return "", fmt.Errorf("missing shell word")
	}
	var value strings.Builder
	for _, part := range word.Parts {
		switch typed := part.(type) {
		case *syntax.Lit:
			value.WriteString(typed.Value)
		case *syntax.SglQuoted:
			if typed.Dollar {
				return "", fmt.Errorf("locale-translated single quote")
			}
			value.WriteString(typed.Value)
		case *syntax.DblQuoted:
			if typed.Dollar {
				return "", fmt.Errorf("locale-translated double quote")
			}
			for _, quotedPart := range typed.Parts {
				literal, ok := quotedPart.(*syntax.Lit)
				if !ok {
					return "", fmt.Errorf("nonliteral double-quoted word")
				}
				value.WriteString(literal.Value)
			}
		default:
			return "", fmt.Errorf("nonliteral shell word")
		}
	}
	return value.String(), nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func markdownProseText(document ast.Node, content []byte) (string, error) {
	var prose strings.Builder
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if node.Type() == ast.TypeBlock {
				prose.WriteByte('\n')
			}
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.CodeSpan, *ast.FencedCodeBlock, *ast.CodeBlock:
			prose.WriteByte('\n')
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			if isHTTPReference(string(typed.Destination)) {
				prose.WriteByte('\n')
				prose.Write(typed.Destination)
				prose.WriteByte('\n')
				return ast.WalkSkipChildren, nil
			}
		case *ast.Image:
			if isHTTPReference(string(typed.Destination)) {
				prose.WriteByte('\n')
				prose.Write(typed.Destination)
				prose.WriteByte('\n')
				return ast.WalkSkipChildren, nil
			}
		case *ast.AutoLink:
			prose.WriteByte('\n')
			prose.Write(typed.URL(content))
			prose.WriteByte('\n')
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			prose.Write(typed.Segment.Value(content))
			if typed.SoftLineBreak() || typed.HardLineBreak() {
				prose.WriteByte('\n')
			}
		case *ast.String:
			prose.Write(typed.Value)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	decoded := util.ResolveNumericReferences([]byte(prose.String()))
	decoded = util.ResolveEntityNames(decoded)
	return string(decoded), nil
}

func insideBlockquote(node ast.Node) bool {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if _, ok := parent.(*ast.Blockquote); ok {
			return true
		}
	}
	return false
}

func validateInlineSpecauditReference(packagePath, value string, count *int) error {
	found, err := visitSpecauditReferences(packagePath, value, count, func(int) error { return nil })
	if err != nil || found == 0 {
		return err
	}
	switch value {
	case "specaudit", authenticatedSpecauditPath:
		return nil
	}
	return fmt.Errorf("markdown payload %q contains nonportable inline specaudit reference %q", packagePath, value)
}

func validateJSONSpecauditReferences(packagePath, value string, count *int) error {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return fmt.Errorf("markdown payload %q contains an invalid JSON data block: %w", packagePath, err)
	}
	return walkJSONStrings(decoded, func(candidate string) error {
		found, err := visitSpecauditReferences(packagePath, candidate, count, func(int) error { return nil })
		if err != nil || found == 0 {
			return err
		}
		if candidate == "specaudit inventory" || isHTTPReference(candidate) {
			return nil
		}
		return fmt.Errorf("markdown payload %q contains an unapproved specaudit reference in a JSON data block", packagePath)
	})
}

func walkJSONStrings(value any, visit func(string) error) error {
	switch typed := value.(type) {
	case string:
		return visit(typed)
	case []any:
		for _, element := range typed {
			if err := walkJSONStrings(element, visit); err != nil {
				return err
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := visit(key); err != nil {
				return err
			}
			if err := walkJSONStrings(typed[key], visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProseSpecauditReferences(packagePath, value string, count *int) error {
	_, err := visitSpecauditReferences(packagePath, value, count, func(index int) error {
		if isHTTPReferenceAt(value, index) {
			return nil
		}
		return fmt.Errorf("markdown payload %q contains specaudit outside a declared non-executable context in %q", packagePath, value)
	})
	return err
}

func visitSpecauditReferences(packagePath, value string, count *int, visit func(int) error) (int, error) {
	found := 0
	for offset := 0; offset+len("specaudit") <= len(value); {
		relative := indexASCIIFold(value[offset:], "specaudit")
		if relative < 0 {
			break
		}
		index := offset + relative
		found++
		(*count)++
		if *count > maxMarkdownSpecauditReferences {
			return found, fmt.Errorf("markdown payload %q exceeds the %d-specaudit-reference bound", packagePath, maxMarkdownSpecauditReferences)
		}
		if err := visit(index); err != nil {
			return found, err
		}
		offset = index + len("specaudit")
	}
	return found, nil
}

func indexASCIIFold(value, target string) int {
	for start := 0; start+len(target) <= len(value); start++ {
		matched := true
		for offset := range len(target) {
			character := value[start+offset]
			if character >= 'A' && character <= 'Z' {
				character += 'a' - 'A'
			}
			if character != target[offset] {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func isHTTPReferenceAt(value string, offset int) bool {
	start := offset
	for start > 0 && !referenceDelimiter(value[start-1]) {
		start--
	}
	end := offset + len("specaudit")
	for end < len(value) && !referenceDelimiter(value[end]) {
		end++
	}
	return isHTTPReference(strings.TrimRight(value[start:end], ".,;:!?"))
}

func referenceDelimiter(value byte) bool {
	return value <= ' ' || strings.ContainsRune(`()[]{}<>"'`, rune(value))
}

func isHTTPReference(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (strings.EqualFold(parsed.Scheme, "https") || strings.EqualFold(parsed.Scheme, "http"))
}

func validateSkillName(packagePath string, content []byte, expected string) error {
	const opening = "---\n"
	if !bytes.HasPrefix(content, []byte(opening)) {
		return fmt.Errorf("skill %q is missing YAML frontmatter", packagePath)
	}
	remainder := content[len(opening):]
	frontmatter, _, found := bytes.Cut(remainder, []byte("\n---\n"))
	if !found {
		return fmt.Errorf("skill %q has unterminated YAML frontmatter", packagePath)
	}
	var metadata struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return fmt.Errorf("parse skill %q frontmatter: %w", packagePath, err)
	}
	if metadata.Name != expected {
		return fmt.Errorf("skill %q has name %q, want %q", packagePath, metadata.Name, expected)
	}
	return nil
}

func validateMarkdownLinks(packagePath string, content []byte) error {
	document := goldmark.DefaultParser().Parse(text.NewReader(content))
	linkCount := 0
	return ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination []byte
		switch typed := node.(type) {
		case *ast.Link:
			destination = typed.Destination
		case *ast.Image:
			destination = typed.Destination
		case *ast.AutoLink:
			destination = typed.URL(content)
			if typed.AutoLinkType == ast.AutoLinkEmail &&
				!bytes.HasPrefix(bytes.ToLower(destination), []byte("mailto:")) {
				destination = append([]byte("mailto:"), destination...)
			}
		case *ast.RawHTML, *ast.HTMLBlock:
			return ast.WalkStop, fmt.Errorf("markdown payload %q contains unsupported raw HTML", packagePath)
		default:
			return ast.WalkContinue, nil
		}
		linkCount++
		if linkCount > maxMarkdownLinks {
			return ast.WalkStop, fmt.Errorf("markdown payload %q exceeds the %d-link bound", packagePath, maxMarkdownLinks)
		}
		if err := validateMarkdownDestination(packagePath, string(destination)); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	})
}

func validateMarkdownDestination(sourcePath, raw string) error {
	if err := validateRawMarkdownDestination(sourcePath, raw); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("markdown payload %q contains malformed link target %q: %w", sourcePath, raw, err)
	}
	if parsed.Scheme != "" {
		return validateExternalMarkdownDestination(sourcePath, raw, parsed)
	}
	return validateLocalMarkdownDestination(sourcePath, raw, parsed)
}

func validateRawMarkdownDestination(sourcePath, raw string) error {
	if raw == "" || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return fmt.Errorf("markdown payload %q contains an empty or invalid link target", sourcePath)
	}
	if strings.Contains(raw, `\`) || strings.HasPrefix(raw, "//") {
		return fmt.Errorf("markdown payload %q contains forbidden link target %q", sourcePath, raw)
	}
	return nil
}

func validateExternalMarkdownDestination(sourcePath, raw string, parsed *url.URL) error {
	switch strings.ToLower(parsed.Scheme) {
	case "https", "http", "mailto":
		return nil
	default:
		return fmt.Errorf("markdown payload %q contains forbidden link scheme in %q", sourcePath, raw)
	}
}

func validateLocalMarkdownDestination(sourcePath, raw string, parsed *url.URL) error {
	if parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" {
		return fmt.Errorf("markdown payload %q contains nonlocal link target %q", sourcePath, raw)
	}
	if parsed.Path == "" {
		if parsed.Fragment != "" && parsed.RawQuery == "" {
			return nil
		}
		return fmt.Errorf("markdown payload %q contains unresolved link target %q", sourcePath, raw)
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("markdown payload %q contains query-bearing local link %q", sourcePath, raw)
	}
	decoded, err := decodeLocalMarkdownPath(sourcePath, raw, parsed)
	if err != nil {
		return err
	}
	return resolveLocalMarkdownPath(sourcePath, raw, decoded)
}

func decodeLocalMarkdownPath(sourcePath, raw string, parsed *url.URL) (string, error) {
	decoded, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", fmt.Errorf("markdown payload %q contains malformed encoded path %q: %w", sourcePath, raw, err)
	}
	if decoded == "" || !utf8.ValidString(decoded) || strings.ContainsAny(decoded, "\\\x00") || strings.Contains(decoded, "%") {
		return "", fmt.Errorf("markdown payload %q contains invalid encoded path %q", sourcePath, raw)
	}
	if path.IsAbs(decoded) || decoded != path.Clean(decoded) {
		return "", fmt.Errorf("markdown payload %q contains noncanonical local path %q", sourcePath, raw)
	}
	return decoded, nil
}

func resolveLocalMarkdownPath(sourcePath, raw, decoded string) error {
	resolved := path.Clean(path.Join(path.Dir(sourcePath), decoded))
	if resolved == ".." || strings.HasPrefix(resolved, "../") || !strings.HasPrefix(resolved, "skills/") {
		return fmt.Errorf("markdown payload %q contains escaping local path %q", sourcePath, raw)
	}
	if err := validatePackagePath(resolved); err != nil {
		return fmt.Errorf("markdown payload %q contains invalid local path %q: %w", sourcePath, raw, err)
	}
	entry, ok := entryByPackagePath(resolved)
	if !ok || (entry.role != "skill" && entry.role != "reference") {
		return fmt.Errorf("markdown payload %q contains unresolved package reference %q", sourcePath, raw)
	}
	return nil
}
