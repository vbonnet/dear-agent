// Package codeintel provides language detection and tiered verification support.
// It implements a registry of language specifications used to determine which
// verification checks are available for a given project.
package codeintel

// LanguageSpec describes a programming language's detection patterns, grep-based
// checks (Tier 0), AST analysis support (Tier 1), and semantic tool commands (Tier 2).
type LanguageSpec struct {
	Name string `json:"name"`

	// Detection: files/patterns that indicate this language is used.
	ManifestFiles []string `json:"manifest_files"`
	SourceGlobs   []string `json:"source_globs"`

	// Tier 0: grep patterns (always available).
	DebugPatterns   []string `json:"debug_patterns"`
	TestFileGlobs   []string `json:"test_file_globs"`
	ImportPattern   string   `json:"import_pattern,omitempty"`
	FunctionPattern string   `json:"function_pattern,omitempty"`

	// Tier 1: ast-grep (if installed).
	ASTGrepLang     string `json:"ast_grep_lang,omitempty"`
	ASTGrepRulesDir string `json:"ast_grep_rules_dir,omitempty"`

	// Tier 2: semantic tools (if installed).
	BuildCmd    []string `json:"build_cmd,omitempty"`
	TestCmd     []string `json:"test_cmd,omitempty"`
	DeadcodeCmd []string `json:"deadcode_cmd,omitempty"`
	LintCmd     []string `json:"lint_cmd,omitempty"`
}

// Tier constants for verification levels.
const (
	Tier0 = 0 // Universal: grep, git, file patterns
	Tier1 = 1 // Structural: ast-grep AST-aware matching
	Tier2 = 2 // Semantic: language-specific tool analysis
)

// UnknownLanguage is a minimal spec for languages not in the registry.
// It supports Tier 0 grep checks only.
var UnknownLanguage = LanguageSpec{
	Name: "unknown",
}

// BuiltinSpecs contains the default language specifications.
var BuiltinSpecs = map[string]LanguageSpec{
	"go": {
		Name:            "go",
		ManifestFiles:   []string{"go.mod"},
		SourceGlobs:     []string{"**/*.go"},
		DebugPatterns:   []string{`fmt\.Print`, `log\.Print`},
		TestFileGlobs:   []string{"**/*_test.go"},
		ImportPattern:   `^import\s+[("]\s*"([^"]+)"`,
		FunctionPattern: `func\s+(?:\([^)]+\)\s+)?(\w+)`,
		ASTGrepLang:     "go",
		ASTGrepRulesDir: "rules/go",
		BuildCmd:        []string{"go", "build", "./..."},
		TestCmd:         []string{"go", "test", "./..."},
		DeadcodeCmd:     []string{"deadcode", "-test", "./..."},
		LintCmd:         []string{"staticcheck", "./..."},
	},
	"python": {
		Name:            "python",
		ManifestFiles:   []string{"pyproject.toml", "setup.py", "requirements.txt"},
		SourceGlobs:     []string{"**/*.py"},
		DebugPatterns:   []string{`print\(`, `pdb\.set_trace`, `breakpoint\(\)`},
		TestFileGlobs:   []string{"**/test_*.py", "**/*_test.py"},
		ImportPattern:   `^(?:from|import)\s+(\S+)`,
		FunctionPattern: `def\s+(\w+)`,
		ASTGrepLang:     "python",
		ASTGrepRulesDir: "rules/python",
		TestCmd:         []string{"pytest", "--tb=short", "-q"},
		DeadcodeCmd:     []string{"vulture", "."},
		LintCmd:         []string{"ruff", "check", "."},
	},
	"typescript": {
		Name:            "typescript",
		ManifestFiles:   []string{"package.json", "tsconfig.json"},
		SourceGlobs:     []string{"**/*.ts", "**/*.tsx"},
		DebugPatterns:   []string{`console\.log`, `debugger`},
		TestFileGlobs:   []string{"**/*.test.ts", "**/*.spec.ts"},
		ImportPattern:   `^import\s+.*from\s+['"]([^'"]+)`,
		FunctionPattern: `(?:function\s+(\w+)|export\s+(?:const|function)\s+(\w+))`,
		ASTGrepLang:     "typescript",
		ASTGrepRulesDir: "rules/typescript",
		BuildCmd:        []string{"tsc", "--noEmit"},
		TestCmd:         []string{"npm", "test"},
		DeadcodeCmd:     []string{"ts-prune"},
		LintCmd:         []string{"eslint", "."},
	},
	"rust": {
		Name:            "rust",
		ManifestFiles:   []string{"Cargo.toml"},
		SourceGlobs:     []string{"**/*.rs"},
		DebugPatterns:   []string{`println!`, `dbg!`},
		TestFileGlobs:   []string{"**/*.rs"}, // tests are inline in Rust
		ImportPattern:   `^use\s+(\S+)`,
		FunctionPattern: `fn\s+(\w+)`,
		ASTGrepLang:     "rust",
		ASTGrepRulesDir: "rules/rust",
		BuildCmd:        []string{"cargo", "build"},
		TestCmd:         []string{"cargo", "test"},
		LintCmd:         []string{"cargo", "clippy"},
	},
	"java": {
		Name:            "java",
		ManifestFiles:   []string{"pom.xml", "build.gradle", "build.gradle.kts"},
		SourceGlobs:     []string{"**/*.java"},
		DebugPatterns:   []string{`System\.out\.print`, `System\.err\.print`},
		TestFileGlobs:   []string{"**/*Test.java", "**/*Tests.java"},
		ImportPattern:   `^import\s+(\S+);`,
		FunctionPattern: `(?:public|private|protected|static|\s)+[\w<>\[\]]+\s+(\w+)\s*\(`,
		ASTGrepLang:     "java",
		ASTGrepRulesDir: "rules/java",
		BuildCmd:        []string{"mvn", "compile"},
		TestCmd:         []string{"mvn", "test"},
		LintCmd:         []string{"checkstyle"},
	},
	"c": {
		Name:            "c",
		ManifestFiles:   []string{"CMakeLists.txt", "Makefile", "meson.build"},
		SourceGlobs:     []string{"**/*.c", "**/*.h", "**/*.cpp", "**/*.hpp", "**/*.cc"},
		DebugPatterns:   []string{`printf\(`, `fprintf\(stderr`},
		TestFileGlobs:   []string{"**/*_test.c", "**/*_test.cpp", "**/test_*.c", "**/test_*.cpp"},
		ImportPattern:   `^#include\s+[<"]([^>"]+)`,
		FunctionPattern: `(?:[\w*]+\s+)+(\w+)\s*\([^)]*\)\s*\{`,
		ASTGrepLang:     "c",
		ASTGrepRulesDir: "rules/c",
		BuildCmd:        []string{"make"},
		TestCmd:         []string{"make", "test"},
	},
	"shell": {
		Name:            "shell",
		ManifestFiles:   []string{},
		SourceGlobs:     []string{"**/*.sh", "**/*.bash", "**/*.zsh"},
		DebugPatterns:   []string{`set\s+-x`, `echo\s+["']?DEBUG`},
		TestFileGlobs:   []string{"**/*_test.sh", "**/test_*.sh"},
		ASTGrepLang:     "bash",
		ASTGrepRulesDir: "rules/shell",
		LintCmd:         []string{"shellcheck"},
	},
}
