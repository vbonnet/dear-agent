package validator

import "strings"

func contains(text, substring string) bool {
	return strings.Contains(text, substring)
}
