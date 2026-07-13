package agenttrace

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var (
	credentialAssignment = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|password|passwd|secret|client[_-]?secret|authorization|cookie|private[_-]?key)\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|(?:bearer\s+)?[^\s,;]+)`)
	bearerCredential     = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
	standaloneCredential = regexp.MustCompile(`\b(?:sk-(?:ant|proj)-[A-Za-z0-9_-]{12,}|github_pat_[A-Za-z0-9_]{12,}|gh[pousr]_[A-Za-z0-9]{12,}|AIza[A-Za-z0-9_-]{20,})\b`)
	keyNormalizer        = strings.NewReplacer("_", "", "-", "", ".", "")
)

func redactAttribute(value string) string {
	if value == "" {
		return value
	}
	if redacted, changed := redactJSON(value); changed {
		return redacted
	}
	redacted := credentialAssignment.ReplaceAllString(value, `$1$2`+redactedValue)
	redacted = bearerCredential.ReplaceAllString(redacted, redactedValue)
	return standaloneCredential.ReplaceAllString(redacted, redactedValue)
}

func redactJSON(value string) (string, bool) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return value, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return value, false
	}
	if !redactJSONValue(decoded) {
		return value, false
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(decoded); err != nil {
		return redactedValue, true
	}
	return strings.TrimSuffix(out.String(), "\n"), true
}

func redactJSONValue(value any) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if sensitiveJSONKey(key) {
				typed[key] = redactedValue
				changed = true
				continue
			}
			if redactJSONValue(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range typed {
			if redactJSONValue(child) {
				changed = true
			}
		}
	}
	return changed
}

func sensitiveJSONKey(key string) bool {
	normalized := keyNormalizer.Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "apikey", "accesstoken", "refreshtoken", "idtoken", "token",
		"password", "passwd", "secret", "clientsecret", "authorization",
		"credential", "credentials", "cookie", "setcookie", "privatekey":
		return true
	default:
		return false
	}
}
