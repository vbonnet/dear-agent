// Package lintcontext reads linting configuration files from a project directory
// and produces concise summaries suitable for injection into BUILD-phase agent prompts.
package lintcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LintConfig holds parsed lint rules from config files.
type LintConfig struct {
	Language string
	Tool     string
	Linters  []string
	Rules    []string
	Settings map[string]string
}

// Summarize reads lint config files from projectDir and returns a concise
// markdown bullet-list summary of actionable rules. Returns empty string
// (not error) when no config files are found.
func Summarize(projectDir string) (string, error) {
	var configs []LintConfig

	goConfig, err := parseGolangCI(projectDir)
	if err != nil {
		return "", fmt.Errorf("parsing golangci config: %w", err)
	}
	if goConfig != nil {
		configs = append(configs, *goConfig)
	}

	pyConfig, err := parsePyproject(projectDir)
	if err != nil {
		return "", fmt.Errorf("parsing pyproject config: %w", err)
	}
	if pyConfig != nil {
		configs = append(configs, *pyConfig)
	}

	esConfig, err := parseESLint(projectDir)
	if err != nil {
		return "", fmt.Errorf("parsing eslint config: %w", err)
	}
	if esConfig != nil {
		configs = append(configs, *esConfig)
	}

	if len(configs) == 0 {
		return "", nil
	}

	return formatSummary(configs), nil
}

// golangCIConfig is a minimal representation of .golangci.yml for YAML parsing.
type golangCIConfig struct {
	Linters struct {
		Enable   []string               `yaml:"enable"`
		Settings map[string]interface{} `yaml:"settings"`
	} `yaml:"linters"`
}

func parseGolangCI(projectDir string) (*LintConfig, error) {
	path := filepath.Join(projectDir, ".golangci.yml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil //nolint:nilnil // nil,nil means "not found" by design
	}
	if err != nil {
		return nil, err
	}

	var cfg golangCIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal golangci config: %w", err)
	}

	config := &LintConfig{
		Language: "Go",
		Tool:     "golangci-lint",
		Linters:  cfg.Linters.Enable,
		Settings: make(map[string]string),
	}

	// Extract key settings as human-readable strings.
	for linter, settingsRaw := range cfg.Linters.Settings {
		settingsMap, ok := settingsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		for key, val := range settingsMap {
			config.Settings[linter+"."+key] = fmt.Sprintf("%v", val)
		}
	}

	return config, nil
}

// pyprojectConfig is a minimal representation of pyproject.toml relevant sections.
// We parse it as TOML-like key extraction rather than full TOML to avoid
// adding a TOML dependency. We look for [tool.ruff] and [tool.pyright] sections.
type pyprojectConfig struct {
	RuffSelect  []string
	RuffIgnore  []string
	PyrightMode string
}

func parsePyproject(projectDir string) (*LintConfig, error) {
	path := filepath.Join(projectDir, "pyproject.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil //nolint:nilnil // nil,nil means "not found" by design
	}
	if err != nil {
		return nil, err
	}

	content := string(data)
	cfg := extractPyprojectConfig(content)

	if len(cfg.RuffSelect) == 0 && cfg.PyrightMode == "" {
		return nil, nil //nolint:nilnil // no relevant config found
	}

	config := &LintConfig{
		Language: "Python",
		Tool:     "ruff/pyright",
		Settings: make(map[string]string),
	}

	if len(cfg.RuffSelect) > 0 {
		config.Linters = append(config.Linters, "ruff")
		config.Rules = append(config.Rules, "select: "+strings.Join(cfg.RuffSelect, ", "))
	}
	if len(cfg.RuffIgnore) > 0 {
		config.Rules = append(config.Rules, "ignore: "+strings.Join(cfg.RuffIgnore, ", "))
	}
	if cfg.PyrightMode != "" {
		config.Linters = append(config.Linters, "pyright")
		config.Settings["pyright.typeCheckingMode"] = cfg.PyrightMode
	}

	return config, nil
}

func extractPyprojectConfig(content string) pyprojectConfig {
	var cfg pyprojectConfig

	// Extract ruff select rules: look for select = ["X", "Y"]
	cfg.RuffSelect = extractTOMLStringArray(content, "select")
	cfg.RuffIgnore = extractTOMLStringArray(content, "ignore")

	// Extract pyright mode: typeCheckingMode = "strict"
	cfg.PyrightMode = extractTOMLString(content, "typeCheckingMode")

	return cfg
}

func extractTOMLStringArray(content, key string) []string {
	// Look for key = ["val1", "val2"]
	idx := strings.Index(content, key+" = [")
	if idx == -1 {
		idx = strings.Index(content, key+"= [")
	}
	if idx == -1 {
		idx = strings.Index(content, key+" =[")
	}
	if idx == -1 {
		return nil
	}

	// Find the bracket range.
	start := strings.Index(content[idx:], "[")
	if start == -1 {
		return nil
	}
	start += idx
	end := strings.Index(content[start:], "]")
	if end == -1 {
		return nil
	}
	end += start

	arrayContent := content[start+1 : end]
	var result []string
	for _, part := range strings.Split(arrayContent, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"'")
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func extractTOMLString(content, key string) string {
	idx := strings.Index(content, key+" = ")
	if idx == -1 {
		idx = strings.Index(content, key+"= ")
	}
	if idx == -1 {
		idx = strings.Index(content, key+" =")
	}
	if idx == -1 {
		return ""
	}

	// Find the value after =
	eqIdx := strings.Index(content[idx:], "=")
	if eqIdx == -1 {
		return ""
	}
	rest := content[idx+eqIdx+1:]

	// Get the rest of the line.
	newline := strings.Index(rest, "\n")
	if newline != -1 {
		rest = rest[:newline]
	}
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, "\"'")
	return rest
}

func parseESLint(projectDir string) (*LintConfig, error) {
	// Check for eslint config files in priority order.
	candidates := []string{
		"eslint.config.js",
		"eslint.config.mjs",
		"eslint.config.cjs",
		"eslint.config.ts",
		".eslintrc.json",
		".eslintrc.js",
		".eslintrc.yml",
		".eslintrc.yaml",
	}

	var foundPath string
	for _, candidate := range candidates {
		p := filepath.Join(projectDir, candidate)
		if _, err := os.Stat(p); err == nil {
			foundPath = p
			break
		}
	}

	if foundPath == "" {
		return nil, nil //nolint:nilnil // nil,nil means "not found" by design
	}

	// We detect the presence of ESLint config but do not deep-parse JS/TS configs.
	// For JSON/YAML configs, we extract what we can.
	config := &LintConfig{
		Language: "TypeScript/JavaScript",
		Tool:     "eslint",
		Linters:  []string{"eslint"},
		Settings: make(map[string]string),
	}

	configFile := filepath.Base(foundPath)
	config.Settings["config-file"] = configFile

	// Try to extract rules from JSON config.
	if strings.HasSuffix(foundPath, ".json") {
		data, err := os.ReadFile(foundPath)
		if err != nil {
			return nil, err
		}
		rules := extractESLintRulesFromJSON(string(data))
		config.Rules = rules
	}

	return config, nil
}

func extractESLintRulesFromJSON(content string) []string {
	// Simple extraction: look for "extends" array values.
	var rules []string

	extendsArr := extractJSONStringArray(content, "extends")
	if len(extendsArr) > 0 {
		rules = append(rules, "extends: "+strings.Join(extendsArr, ", "))
	}

	return rules
}

func extractJSONStringArray(content, key string) []string {
	// Look for "key": ["val1", "val2"]
	searchKey := "\"" + key + "\""
	idx := strings.Index(content, searchKey)
	if idx == -1 {
		return nil
	}

	rest := content[idx+len(searchKey):]
	// Skip whitespace and colon.
	rest = strings.TrimLeft(rest, " \t\n\r:")

	if len(rest) == 0 || rest[0] != '[' {
		return nil
	}

	end := strings.Index(rest, "]")
	if end == -1 {
		return nil
	}

	arrayContent := rest[1:end]
	var result []string
	for _, part := range strings.Split(arrayContent, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"'")
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func formatSummary(configs []LintConfig) string {
	var b strings.Builder
	b.WriteString("## Lint Rules Summary\n\n")

	for _, cfg := range configs {
		b.WriteString("### ")
		b.WriteString(cfg.Language)
		b.WriteString(" (")
		b.WriteString(cfg.Tool)
		b.WriteString(")\n\n")

		if len(cfg.Linters) > 0 {
			b.WriteString("- **Enabled linters**: ")
			b.WriteString(strings.Join(cfg.Linters, ", "))
			b.WriteString("\n")
		}

		for _, rule := range cfg.Rules {
			b.WriteString("- ")
			b.WriteString(rule)
			b.WriteString("\n")
		}

		for key, val := range cfg.Settings {
			b.WriteString("- **")
			b.WriteString(key)
			b.WriteString("**: ")
			b.WriteString(val)
			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
