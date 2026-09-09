package buildauthority

import "strings"

// sourceAdministrativeClass is deliberately only a path-policy result. It
// carries no content, handle, or evidence with which a caller could seal a
// source snapshot.
type sourceAdministrativeClass uint8

const (
	sourceAuthorityAndManifest sourceAdministrativeClass = iota + 1
	sourceForbidden
	sourceInertAdministration
)

// classifySourceAdministrativePath classifies one already-normalized,
// repository-relative path. Object relationship closure (for example, the
// pack/index pair and chain membership) belongs to the later inventory pass.
func classifySourceAdministrativePath(relativePath string, kind entryKind, objectFormat repositoryObjectFormat) sourceAdministrativeClass {
	components, ok := splitNormalizedSourcePath(relativePath)
	if !ok || components[0] != ".git" {
		return sourceForbidden
	}
	if kind != entryDirectory && kind != entryRegular {
		return sourceForbidden
	}
	for _, component := range components {
		if strings.HasSuffix(component, ".lock") {
			return sourceForbidden
		}
	}

	switch relativePath {
	case ".git":
		return requireSourceAuthorityKind(kind, entryDirectory)
	case ".git/config", ".git/packed-refs":
		return requireSourceAuthorityKind(kind, entryRegular)
	case ".git/config.worktree", ".git/commondir", ".git/gitdir", ".git/shallow", ".git/info/grafts",
		".git/objects/info/alternates", ".git/objects/info/http-alternates":
		return sourceForbidden
	}
	if relativePath == ".git/refs/replace" || strings.HasPrefix(relativePath, ".git/refs/replace/") {
		return sourceForbidden
	}
	if relativePath == ".git/objects" || strings.HasPrefix(relativePath, ".git/objects/") {
		return classifySourceObjectPath(components[2:], kind, objectFormat)
	}

	return sourceInertAdministration
}

func splitNormalizedSourcePath(relativePath string) ([]string, bool) {
	if relativePath == "" || strings.IndexByte(relativePath, 0) >= 0 {
		return nil, false
	}
	components := strings.Split(relativePath, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, false
		}
	}
	return components, true
}

func requireSourceAuthorityKind(got, want entryKind) sourceAdministrativeClass {
	if got != want {
		return sourceForbidden
	}
	return sourceAuthorityAndManifest
}

func classifySourceObjectPath(components []string, kind entryKind, objectFormat repositoryObjectFormat) sourceAdministrativeClass {
	if len(components) == 0 {
		return requireSourceAuthorityKind(kind, entryDirectory)
	}

	if isRecognizedSourceObjectDirectory(components) {
		return requireSourceAuthorityKind(kind, entryDirectory)
	}
	if isRecognizedSourceObjectFile(components, objectFormat) {
		return requireSourceAuthorityKind(kind, entryRegular)
	}
	return sourceForbidden
}

func isRecognizedSourceObjectDirectory(components []string) bool {
	switch len(components) {
	case 1:
		return components[0] == "info" || components[0] == "pack" || isLowerHexOfWidth(components[0], 2)
	case 2:
		return (components[0] == "info" && components[1] == "commit-graphs") ||
			(components[0] == "pack" && components[1] == "multi-pack-index.d")
	default:
		return false
	}
}

func isRecognizedSourceObjectFile(components []string, objectFormat repositoryObjectFormat) bool {
	switch len(components) {
	case 2:
		switch components[0] {
		case "info":
			return isRecognizedSourceObjectInfoFile(components[1])
		case "pack":
			return isRecognizedSourceObjectPackFile(components[1], objectFormat)
		default:
			return isLowerHexOfWidth(components[0], 2) &&
				isLowerHexOfWidth(components[1], objectFormat.hexWidth()-2)
		}
	case 3:
		return isRecognizedSourceObjectChainFile(components, objectFormat)
	}
	return false
}

func isRecognizedSourceObjectInfoFile(name string) bool {
	return name == "commit-graph" || name == "packs"
}

func isRecognizedSourceObjectPackFile(name string, objectFormat repositoryObjectFormat) bool {
	if name == "multi-pack-index" {
		return true
	}
	for _, suffix := range [...]string{".pack", ".idx", ".rev", ".bitmap", ".keep", ".mtimes"} {
		if matchesSourceObjectHashFile(name, "pack-", suffix, objectFormat) {
			return true
		}
	}
	for _, suffix := range [...]string{".rev", ".bitmap"} {
		if matchesSourceObjectHashFile(name, "multi-pack-index-", suffix, objectFormat) {
			return true
		}
	}
	return false
}

func isRecognizedSourceObjectChainFile(components []string, objectFormat repositoryObjectFormat) bool {
	if components[0] == "info" && components[1] == "commit-graphs" {
		return components[2] == "commit-graph-chain" ||
			matchesSourceObjectHashFile(components[2], "graph-", ".graph", objectFormat)
	}
	if components[0] != "pack" || components[1] != "multi-pack-index.d" {
		return false
	}
	if components[2] == "multi-pack-index-chain" {
		return true
	}
	for _, suffix := range [...]string{".midx", ".rev", ".bitmap"} {
		if matchesSourceObjectHashFile(components[2], "multi-pack-index-", suffix, objectFormat) {
			return true
		}
	}
	return false
}

func matchesSourceObjectHashFile(name, prefix, suffix string, objectFormat repositoryObjectFormat) bool {
	if len(name) < len(prefix)+len(suffix) || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	hash := name[len(prefix) : len(name)-len(suffix)]
	return isLowerHexOfWidth(hash, objectFormat.hexWidth())
}

func isLowerHexOfWidth(value string, width int) bool {
	if width <= 0 || len(value) != width {
		return false
	}
	for i := 0; i < len(value); i++ {
		if (value[i] < '0' || value[i] > '9') && (value[i] < 'a' || value[i] > 'f') {
			return false
		}
	}
	return true
}
