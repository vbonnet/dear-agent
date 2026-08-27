package specpackage

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	manifestPath          = "package-manifest.json"
	manifestLogicalMode   = "0444"
	maxMarkdownBytes      = 1 << 20
	maxExecutableBytes    = 128 << 20
	maxManifestBytes      = 64 << 10
	maxPackageBytes       = maxExecutableBytes + 8*maxMarkdownBytes
	maxPackageTreeEntries = 32
	maxPackagePathBytes   = 512
	maxPackagePathDepth   = 16
)

type layoutEntry struct {
	packagePath string
	sourcePath  string
	role        string
	mode        fs.FileMode
	maxBytes    int64
	skillName   string
}

var payloadLayout = []layoutEntry{
	{
		packagePath: "bin/specaudit",
		role:        "executable",
		mode:        0o555,
		maxBytes:    maxExecutableBytes,
	},
	{
		packagePath: "skills/audit-specs/SKILL.md",
		sourcePath:  "spec-governance/skills/audit-specs/SKILL.md",
		role:        "skill",
		mode:        0o444,
		maxBytes:    maxMarkdownBytes,
		skillName:   "audit-specs",
	},
	{
		packagePath: "skills/audit-specs/references/audit-verdicts.md",
		sourcePath:  "spec-governance/skills/audit-specs/references/audit-verdicts.md",
		role:        "reference",
		mode:        0o444,
		maxBytes:    maxMarkdownBytes,
	},
	{
		packagePath: "skills/audit-specs/references/report-schema.md",
		sourcePath:  "spec-governance/skills/audit-specs/references/report-schema.md",
		role:        "reference",
		mode:        0o444,
		maxBytes:    maxMarkdownBytes,
	},
	{
		packagePath: "skills/write-spec/SKILL.md",
		sourcePath:  "spec-governance/skills/write-spec/SKILL.md",
		role:        "skill",
		mode:        0o444,
		maxBytes:    maxMarkdownBytes,
		skillName:   "write-spec",
	},
	{
		packagePath: "skills/write-spec/references/contract-model.md",
		sourcePath:  "spec-governance/skills/write-spec/references/contract-model.md",
		role:        "reference",
		mode:        0o444,
		maxBytes:    maxMarkdownBytes,
	},
	{
		packagePath: "skills/write-spec/references/ears-and-bdd.md",
		sourcePath:  "spec-governance/skills/write-spec/references/ears-and-bdd.md",
		role:        "reference",
		mode:        0o444,
		maxBytes:    maxMarkdownBytes,
	},
}

var expectedDirectories = []string{
	".",
	"bin",
	"skills",
	"skills/audit-specs",
	"skills/audit-specs/references",
	"skills/write-spec",
	"skills/write-spec/references",
}

func entryByPackagePath(packagePath string) (layoutEntry, bool) {
	index := sort.Search(len(payloadLayout), func(i int) bool {
		return payloadLayout[i].packagePath >= packagePath
	})
	if index == len(payloadLayout) || payloadLayout[index].packagePath != packagePath {
		return layoutEntry{}, false
	}
	return payloadLayout[index], true
}

func validatePackagePath(packagePath string) error {
	if !utf8.ValidString(packagePath) {
		return fmt.Errorf("package path is not valid UTF-8")
	}
	if len(packagePath) == 0 || len(packagePath) > maxPackagePathBytes {
		return fmt.Errorf("package path length is outside the supported bound")
	}
	if strings.ContainsRune(packagePath, '\x00') || strings.Contains(packagePath, `\`) {
		return fmt.Errorf("package path contains a forbidden character")
	}
	if packagePath != "." && !fs.ValidPath(packagePath) {
		return fmt.Errorf("package path is not a canonical slash-relative path")
	}
	if packagePath != "." && len(strings.Split(packagePath, "/")) > maxPackagePathDepth {
		return fmt.Errorf("package path exceeds the supported depth")
	}
	return nil
}

func pathAliasKey(packagePath string) string {
	return cases.Fold().String(norm.NFC.String(packagePath))
}

func validateNoPathAliases(paths []string) error {
	seenBytes := make(map[string]struct{}, len(paths))
	seenAliases := make(map[string]string, len(paths))
	for _, packagePath := range paths {
		if err := validatePackagePath(packagePath); err != nil {
			return fmt.Errorf("invalid package path %q: %w", packagePath, err)
		}
		if _, duplicate := seenBytes[packagePath]; duplicate {
			return fmt.Errorf("duplicate package path %q", packagePath)
		}
		seenBytes[packagePath] = struct{}{}
		key := pathAliasKey(packagePath)
		if previous, duplicate := seenAliases[key]; duplicate {
			return fmt.Errorf("package paths %q and %q are case or Unicode aliases", previous, packagePath)
		}
		seenAliases[key] = packagePath
	}
	return nil
}

func expectedPackagePaths() ([]string, []string) {
	directories := append([]string(nil), expectedDirectories...)
	files := make([]string, 0, len(payloadLayout)+1)
	for _, entry := range payloadLayout {
		files = append(files, entry.packagePath)
	}
	files = append(files, manifestPath)
	sort.Strings(directories)
	sort.Strings(files)
	return directories, files
}

func expectedSourceSkillPaths() ([]string, []string) {
	directories := []string{
		"spec-governance/skills",
		"spec-governance/skills/audit-specs",
		"spec-governance/skills/audit-specs/references",
		"spec-governance/skills/write-spec",
		"spec-governance/skills/write-spec/references",
	}
	files := make([]string, 0, len(payloadLayout)-1)
	for _, entry := range payloadLayout {
		if entry.sourcePath != "" {
			files = append(files, entry.sourcePath)
		}
	}
	sort.Strings(directories)
	sort.Strings(files)
	return directories, files
}
