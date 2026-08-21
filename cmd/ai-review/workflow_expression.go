package main

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

func nodeContainsCustomSecretReference(node *yaml.Node) bool {
	if node.Kind == yaml.ScalarNode && scalarContainsCustomSecretReference(node.Value) {
		return true
	}
	return slices.ContainsFunc(node.Content, nodeContainsCustomSecretReference)
}

func scalarContainsCustomSecretReference(value string) bool {
	return classifyWorkflowSecretReferences(value).custom
}

type workflowSecretReferences struct {
	defaultToken bool
	githubToken  bool
	custom       bool
}

// classifyWorkflowSecretReferences follows the Actions expression delimiter
// and quoting rules closely enough to avoid treating quoted }} or the word
// "secrets" inside a single-quoted string as executable authority. Unclosed or
// dynamic accesses fail closed.
func classifyWorkflowSecretReferences(value string) workflowSecretReferences {
	var references workflowSecretReferences
	for offset := 0; offset < len(value); {
		relativeStart := strings.Index(value[offset:], "${{")
		if relativeStart < 0 {
			break
		}
		start := offset + relativeStart + len("${{")
		end, ok := workflowExpressionEnd(value, start)
		if !ok {
			references.custom = true
			break
		}
		classifyWorkflowExpressionSecrets(value[start:end], &references)
		offset = end + len("}}")
	}
	return references
}

func workflowExpressionEnd(value string, start int) (int, bool) {
	inString := false
	for i := start; i < len(value); i++ {
		if value[i] == '\'' {
			if inString && i+1 < len(value) && value[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
			continue
		}
		if !inString && i+1 < len(value) && value[i] == '}' && value[i+1] == '}' {
			return i, true
		}
	}
	return 0, false
}

func classifyWorkflowExpressionSecrets(expression string, references *workflowSecretReferences) {
	inString := false
	for i := 0; i < len(expression); i++ {
		if expression[i] == '\'' {
			if inString && i+1 < len(expression) && expression[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if asciiRootIdentifierAt(expression, i, "secrets") {
			next, defaultToken := defaultContextMemberAccessEnd(expression, i+len("secrets"), "github_token")
			if !defaultToken {
				references.custom = true
				return
			}
			references.defaultToken = true
			i = next - 1
			continue
		}
		if asciiRootIdentifierAt(expression, i, "github") {
			next, defaultToken := defaultContextMemberAccessEnd(expression, i+len("github"), "token")
			if defaultToken {
				references.githubToken = true
				i = next - 1
			}
		}
	}
}

func asciiRootIdentifierAt(value string, start int, identifier string) bool {
	if start < 0 || start+len(identifier) > len(value) ||
		asciiLower(value[start:start+len(identifier)]) != identifier {
		return false
	}
	if start > 0 && workflowIdentifierByte(value[start-1]) {
		return false
	}
	previous := start - 1
	for previous >= 0 {
		switch value[previous] {
		case ' ', '\t', '\r', '\n':
			previous--
		default:
			if value[previous] == '.' {
				return false
			}
			previous = -1
		}
	}
	return start+len(identifier) == len(value) || !workflowIdentifierByte(value[start+len(identifier)])
}

func workflowIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_'
}

func skipWorkflowSpace(value string, start int) int {
	for start < len(value) {
		switch value[start] {
		case ' ', '\t', '\r', '\n':
			start++
		default:
			return start
		}
	}
	return start
}

func defaultContextMemberAccessEnd(expression string, start int, member string) (int, bool) {
	i := skipWorkflowSpace(expression, start)
	if i >= len(expression) {
		return i, false
	}
	var ok bool
	switch expression[i] {
	case '.':
		i, ok = defaultDotMemberAccessEnd(expression, i+1, member)
	case '[':
		i, ok = defaultBracketMemberAccessEnd(expression, i+1, member)
	default:
		return i, false
	}
	if !ok {
		return i, false
	}
	return terminalContextMemberAccessEnd(expression, i)
}

func defaultDotMemberAccessEnd(expression string, start int, member string) (int, bool) {
	i := skipWorkflowSpace(expression, start)
	nameStart := i
	for i < len(expression) && workflowIdentifierByte(expression[i]) {
		i++
	}
	return i, asciiLower(expression[nameStart:i]) == member
}

func defaultBracketMemberAccessEnd(expression string, start int, member string) (int, bool) {
	i := skipWorkflowSpace(expression, start)
	if i >= len(expression) || expression[i] != '\'' {
		return i, false
	}
	name, next, ok := workflowSingleQuotedMember(expression, i+1)
	if !ok || asciiLower(name) != member {
		return next, false
	}
	next = skipWorkflowSpace(expression, next)
	if next >= len(expression) || expression[next] != ']' {
		return next, false
	}
	return next + 1, true
}

func workflowSingleQuotedMember(expression string, start int) (string, int, bool) {
	var name strings.Builder
	for i := start; i < len(expression); i++ {
		if expression[i] != '\'' {
			name.WriteByte(expression[i])
			continue
		}
		if i+1 < len(expression) && expression[i+1] == '\'' {
			name.WriteByte('\'')
			i++
			continue
		}
		return name.String(), i + 1, true
	}
	return name.String(), len(expression), false
}

func terminalContextMemberAccessEnd(expression string, end int) (int, bool) {
	continuation := skipWorkflowSpace(expression, end)
	if continuation < len(expression) && (expression[continuation] == '.' || expression[continuation] == '[') {
		return continuation, false
	}
	return end, true
}

func scalarIsExactDefaultTokenReference(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "${{") {
		return false
	}
	start := len("${{")
	end, ok := workflowExpressionEnd(value, start)
	if !ok || strings.TrimSpace(value[end+len("}}"):]) != "" {
		return false
	}
	expression := strings.TrimSpace(value[start:end])
	for _, allowed := range []struct {
		context string
		member  string
	}{
		{context: "secrets", member: "github_token"},
		{context: "github", member: "token"},
	} {
		if !asciiRootIdentifierAt(expression, 0, allowed.context) {
			continue
		}
		next, exact := defaultContextMemberAccessEnd(expression, len(allowed.context), allowed.member)
		if exact && strings.TrimSpace(expression[next:]) == "" {
			return true
		}
	}
	return false
}
