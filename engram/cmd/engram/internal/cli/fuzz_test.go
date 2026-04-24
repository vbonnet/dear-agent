package cli

import (
	"testing"
)

// FuzzValidateNoShellMetacharacters feeds random strings to the shell metacharacter validator.
// This is security-critical: it prevents command injection attacks.
func FuzzValidateNoShellMetacharacters(f *testing.F) {
	f.Add("safe input")
	f.Add("test; rm -rf /")
	f.Add("test | cat /etc/passwd")
	f.Add("test`whoami`")
	f.Add("test$(whoami)")
	f.Add("")
	f.Add(string([]byte{0x00}))
	f.Add("test\nrm -rf /")
	f.Add("test\r\nevil")
	f.Add("test && evil || more")
	f.Add("a]b[c{d}e")

	f.Fuzz(func(t *testing.T, value string) {
		// Must never panic
		_ = ValidateNoShellMetacharacters("field", value)
	})
}

// FuzzValidateNoTraversal feeds random paths to the path traversal detector.
func FuzzValidateNoTraversal(f *testing.F) {
	f.Add("safe/path/file.txt")
	f.Add("../../../etc/passwd")
	f.Add("")
	f.Add("..")
	f.Add("a/../b/../c")
	f.Add(string([]byte{0x2e, 0x2e, 0x2f})) // ../
	f.Add("....//....//etc/passwd")

	f.Fuzz(func(t *testing.T, path string) {
		// Must never panic
		_ = ValidateNoTraversal("field", path)
	})
}

// FuzzValidateMaxLength feeds random strings and lengths to the length validator.
func FuzzValidateMaxLength(f *testing.F) {
	f.Add("short", 100)
	f.Add("", 0)
	f.Add("test", 4)
	f.Add("test", 3)

	f.Fuzz(func(t *testing.T, value string, maxLen int) {
		// Must never panic
		_ = ValidateMaxLength("field", value, maxLen)
	})
}

// FuzzValidateAlphanumeric feeds random strings to the alphanumeric validator.
func FuzzValidateAlphanumeric(f *testing.F) {
	f.Add("abc123", true)
	f.Add("test-name_v2", true)
	f.Add("test@evil", false)
	f.Add("", false)
	f.Add(string([]byte{0xff, 0xfe}), false)

	f.Fuzz(func(t *testing.T, value string, allowHyphens bool) {
		// Must never panic
		_ = ValidateAlphanumeric("field", value, allowHyphens)
	})
}

// FuzzValidateNamespaceComponents feeds random namespace strings to the namespace validator.
func FuzzValidateNamespaceComponents(f *testing.F) {
	f.Add("user,alice,project", 10, 50)
	f.Add("", 10, 50)
	f.Add("a,b,c,d,e,f,g,h,i,j,k", 10, 50)
	f.Add("user,,project", 10, 50)
	f.Add(",,,", 5, 5)

	f.Fuzz(func(t *testing.T, namespace string, maxComponents, maxCompLen int) {
		// Must never panic
		_ = ValidateNamespaceComponents(namespace, maxComponents, maxCompLen)
	})
}

// FuzzValidateFileExtension feeds random paths and extension lists to the extension validator.
func FuzzValidateFileExtension(f *testing.F) {
	f.Add("test.md", ".md,.json,.yaml")
	f.Add("test.exe", ".md,.json")
	f.Add("", ".md")
	f.Add("noext", ".md")

	f.Fuzz(func(t *testing.T, path, allowedExtsStr string) {
		// Split the comma-separated extensions
		var exts []string
		if allowedExtsStr != "" {
			start := 0
			for i := 0; i <= len(allowedExtsStr); i++ {
				if i == len(allowedExtsStr) || allowedExtsStr[i] == ',' {
					exts = append(exts, allowedExtsStr[start:i])
					start = i + 1
				}
			}
		}
		// Must never panic
		_ = ValidateFileExtension("field", path, exts)
	})
}
