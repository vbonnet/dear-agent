package buildauthority

import (
	"strings"
	"testing"
)

var _ func(string, entryKind, repositoryObjectFormat) sourceAdministrativeClass = classifySourceAdministrativePath

func TestClassifySourceAdministrativePathClasses(t *testing.T) {
	sha1 := strings.Repeat("a", 40)
	tests := []struct {
		name string
		path string
		kind entryKind
		want sourceAdministrativeClass
	}{
		{name: "git root", path: ".git", kind: entryDirectory, want: sourceAuthorityAndManifest},
		{name: "config", path: ".git/config", kind: entryRegular, want: sourceAuthorityAndManifest},
		{name: "packed refs", path: ".git/packed-refs", kind: entryRegular, want: sourceAuthorityAndManifest},
		{name: "objects root", path: ".git/objects", kind: entryDirectory, want: sourceAuthorityAndManifest},
		{name: "loose fanout", path: ".git/objects/ab", kind: entryDirectory, want: sourceAuthorityAndManifest},
		{name: "loose object", path: ".git/objects/ab/" + strings.Repeat("c", 38), kind: entryRegular, want: sourceAuthorityAndManifest},
		{name: "pack", path: ".git/objects/pack/pack-" + sha1 + ".pack", kind: entryRegular, want: sourceAuthorityAndManifest},
		{name: "pack index", path: ".git/objects/pack/pack-" + sha1 + ".idx", kind: entryRegular, want: sourceAuthorityAndManifest},
		{name: "commit graph chain", path: ".git/objects/info/commit-graphs/commit-graph-chain", kind: entryRegular, want: sourceAuthorityAndManifest},
		{name: "incremental midx", path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + sha1 + ".midx", kind: entryRegular, want: sourceAuthorityAndManifest},
		{name: "config worktree", path: ".git/config.worktree", kind: entryRegular, want: sourceForbidden},
		{name: "commondir", path: ".git/commondir", kind: entryRegular, want: sourceForbidden},
		{name: "gitdir", path: ".git/gitdir", kind: entryRegular, want: sourceForbidden},
		{name: "shallow", path: ".git/shallow", kind: entryRegular, want: sourceForbidden},
		{name: "grafts", path: ".git/info/grafts", kind: entryRegular, want: sourceForbidden},
		{name: "alternates", path: ".git/objects/info/alternates", kind: entryRegular, want: sourceForbidden},
		{name: "http alternates", path: ".git/objects/info/http-alternates", kind: entryRegular, want: sourceForbidden},
		{name: "replace root", path: ".git/refs/replace", kind: entryDirectory, want: sourceForbidden},
		{name: "replace descendant", path: ".git/refs/replace/" + sha1, kind: entryRegular, want: sourceForbidden},
		{name: "promisor", path: ".git/objects/pack/pack-" + sha1 + ".promisor", kind: entryRegular, want: sourceForbidden},
		{name: "temporary object", path: ".git/objects/ab/tmp_obj", kind: entryRegular, want: sourceForbidden},
		{name: "unknown object", path: ".git/objects/mystery", kind: entryDirectory, want: sourceForbidden},
		{name: "inert hook", path: ".git/hooks/pre-commit", kind: entryRegular, want: sourceInertAdministration},
		{name: "inert ref", path: ".git/refs/heads/main", kind: entryRegular, want: sourceInertAdministration},
		{name: "inert directory", path: ".git/logs", kind: entryDirectory, want: sourceInertAdministration},
		{name: "authority with wrong kind", path: ".git/config", kind: entryDirectory, want: sourceForbidden},
		{name: "object directory with wrong kind", path: ".git/objects/pack", kind: entryRegular, want: sourceForbidden},
		{name: "object file with wrong kind", path: ".git/objects/info/packs", kind: entryDirectory, want: sourceForbidden},
		{name: "symlink", path: ".git/hooks/pre-commit", kind: entrySymlink, want: sourceForbidden},
		{name: "special", path: ".git/hooks/pre-commit", kind: entryKind(255), want: sourceForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifySourceAdministrativePath(test.path, test.kind, objectFormatSHA1); got != test.want {
				t.Fatalf("classifySourceAdministrativePath(%q, %v, sha1) = %v, want %v", test.path, test.kind, got, test.want)
			}
		})
	}
}

func TestClassifySourceAdministrativePathObjectIDWidthIsFormatSensitive(t *testing.T) {
	sha1 := strings.Repeat("a", 40)
	sha256 := strings.Repeat("b", 64)
	tests := []struct {
		name   string
		path   string
		format repositoryObjectFormat
		want   sourceAdministrativeClass
	}{
		{name: "sha1 loose", path: ".git/objects/aa/" + strings.Repeat("b", 38), format: objectFormatSHA1, want: sourceAuthorityAndManifest},
		{name: "sha1 loose under sha256", path: ".git/objects/aa/" + strings.Repeat("b", 38), format: objectFormatSHA256, want: sourceForbidden},
		{name: "sha256 loose", path: ".git/objects/aa/" + strings.Repeat("b", 62), format: objectFormatSHA256, want: sourceAuthorityAndManifest},
		{name: "sha256 loose under sha1", path: ".git/objects/aa/" + strings.Repeat("b", 62), format: objectFormatSHA1, want: sourceForbidden},
		{name: "sha1 pack", path: ".git/objects/pack/pack-" + sha1 + ".pack", format: objectFormatSHA1, want: sourceAuthorityAndManifest},
		{name: "sha1 pack under sha256", path: ".git/objects/pack/pack-" + sha1 + ".pack", format: objectFormatSHA256, want: sourceForbidden},
		{name: "sha256 pack", path: ".git/objects/pack/pack-" + sha256 + ".idx", format: objectFormatSHA256, want: sourceAuthorityAndManifest},
		{name: "sha256 graph", path: ".git/objects/info/commit-graphs/graph-" + sha256 + ".graph", format: objectFormatSHA256, want: sourceAuthorityAndManifest},
		{name: "unknown format hash", path: ".git/objects/pack/pack-" + sha1 + ".idx", format: objectFormatUnknown, want: sourceForbidden},
		{name: "missing hash", path: ".git/objects/pack/pack-.idx", format: objectFormatSHA1, want: sourceForbidden},
		{name: "uppercase hash", path: ".git/objects/pack/pack-" + strings.Repeat("A", 40) + ".idx", format: objectFormatSHA1, want: sourceForbidden},
		{name: "non hex hash", path: ".git/objects/pack/pack-" + strings.Repeat("g", 40) + ".idx", format: objectFormatSHA1, want: sourceForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifySourceAdministrativePath(test.path, entryRegular, test.format); got != test.want {
				t.Fatalf("classifySourceAdministrativePath(%q, regular, %v) = %v, want %v", test.path, test.format, got, test.want)
			}
		})
	}
}

func TestClassifySourceAdministrativePathGlobalLockPredicate(t *testing.T) {
	for _, path := range []string{
		".git/.lock",
		".git/config.lock",
		".git/hooks/state.lock/child",
		".git/objects/info/packs.lock",
	} {
		if got := classifySourceAdministrativePath(path, entryRegular, objectFormatSHA1); got != sourceForbidden {
			t.Errorf("classifySourceAdministrativePath(%q) = %v, want forbidden", path, got)
		}
	}

	for _, path := range []string{
		".git/hooks/clock",
		".git/hooks/.locked",
		".git/hooks/state.LOCK",
	} {
		if got := classifySourceAdministrativePath(path, entryRegular, objectFormatSHA1); got != sourceInertAdministration {
			t.Errorf("classifySourceAdministrativePath(%q) = %v, want inert administration", path, got)
		}
	}
}

func TestClassifySourceAdministrativePathUsesRawNormalizedBytes(t *testing.T) {
	for _, path := range []string{
		"",
		"/.git/config",
		".git/",
		".git//config",
		".git/./config",
		".git/../config",
		".git/config\x00tail",
		"outside/.git/config",
		".GIT/config",
	} {
		if got := classifySourceAdministrativePath(path, entryRegular, objectFormatSHA1); got != sourceForbidden {
			t.Errorf("classifySourceAdministrativePath(%q) = %v, want forbidden", path, got)
		}
	}

	for _, path := range []string{
		".git/hooks/name\\with-backslash",
		".git/hooks/name\nwith-newline",
		string([]byte{'.', 'g', 'i', 't', '/', 'h', 'o', 'o', 'k', 's', '/', 0xff}),
		".git/refs/replacement/value",
	} {
		if got := classifySourceAdministrativePath(path, entryRegular, objectFormatSHA1); got != sourceInertAdministration {
			t.Errorf("classifySourceAdministrativePath(%q) = %v, want inert administration", path, got)
		}
	}
}

func TestClassifySourceAdministrativePathDefersObjectRelationships(t *testing.T) {
	sha1 := strings.Repeat("d", 40)
	for _, path := range []string{
		".git/objects/pack/pack-" + sha1 + ".pack",
		".git/objects/pack/pack-" + sha1 + ".idx",
		".git/objects/pack/pack-" + sha1 + ".rev",
		".git/objects/pack/multi-pack-index-" + sha1 + ".bitmap",
		".git/objects/pack/multi-pack-index.d/multi-pack-index-" + sha1 + ".rev",
	} {
		if got := classifySourceAdministrativePath(path, entryRegular, objectFormatSHA1); got != sourceAuthorityAndManifest {
			t.Errorf("structurally recognized path %q = %v, want authority and manifest", path, got)
		}
	}
}

func TestClassifySourceAdministrativePathClosedObjectGrammar(t *testing.T) {
	formats := []struct {
		name   string
		format repositoryObjectFormat
		hash   string
	}{
		{name: "sha1", format: objectFormatSHA1, hash: strings.Repeat("a", 40)},
		{name: "sha256", format: objectFormatSHA256, hash: strings.Repeat("b", 64)},
	}
	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			authorityDirectories := []string{
				".git/objects/info",
				".git/objects/pack",
				".git/objects/0f",
				".git/objects/info/commit-graphs",
				".git/objects/pack/multi-pack-index.d",
			}
			for _, path := range authorityDirectories {
				if got := classifySourceAdministrativePath(path, entryDirectory, format.format); got != sourceAuthorityAndManifest {
					t.Errorf("directory %q = %v, want authority and manifest", path, got)
				}
			}

			authorityFiles := []string{
				".git/objects/0f/" + format.hash[2:],
				".git/objects/info/commit-graph",
				".git/objects/info/packs",
				".git/objects/pack/pack-" + format.hash + ".pack",
				".git/objects/pack/pack-" + format.hash + ".idx",
				".git/objects/pack/pack-" + format.hash + ".rev",
				".git/objects/pack/pack-" + format.hash + ".bitmap",
				".git/objects/pack/pack-" + format.hash + ".keep",
				".git/objects/pack/pack-" + format.hash + ".mtimes",
				".git/objects/pack/multi-pack-index",
				".git/objects/pack/multi-pack-index-" + format.hash + ".rev",
				".git/objects/pack/multi-pack-index-" + format.hash + ".bitmap",
				".git/objects/info/commit-graphs/commit-graph-chain",
				".git/objects/info/commit-graphs/graph-" + format.hash + ".graph",
				".git/objects/pack/multi-pack-index.d/multi-pack-index-chain",
				".git/objects/pack/multi-pack-index.d/multi-pack-index-" + format.hash + ".midx",
				".git/objects/pack/multi-pack-index.d/multi-pack-index-" + format.hash + ".rev",
				".git/objects/pack/multi-pack-index.d/multi-pack-index-" + format.hash + ".bitmap",
			}
			for _, path := range authorityFiles {
				if got := classifySourceAdministrativePath(path, entryRegular, format.format); got != sourceAuthorityAndManifest {
					t.Errorf("file %q = %v, want authority and manifest", path, got)
				}
			}

			wrongLocations := []string{
				".git/objects/info/pack-" + format.hash + ".pack",
				".git/objects/pack/graph-" + format.hash + ".graph",
				".git/objects/commit-graph",
				".git/objects/multi-pack-index",
				".git/objects/info/commit-graphs/multi-pack-index-" + format.hash + ".midx",
				".git/objects/pack/multi-pack-index.d/graph-" + format.hash + ".graph",
				".git/objects/pack/multi-pack-index-" + format.hash + ".midx",
				".git/objects/pack/pack-" + format.hash + ".promisor",
			}
			for _, path := range wrongLocations {
				if got := classifySourceAdministrativePath(path, entryRegular, format.format); got != sourceForbidden {
					t.Errorf("wrong-location file %q = %v, want forbidden", path, got)
				}
			}
		})
	}
}

func TestClassifySourceAdministrativePathRootKindsAreClosed(t *testing.T) {
	tests := []struct {
		path string
		kind entryKind
	}{
		{path: ".git", kind: entryRegular},
		{path: ".git/config", kind: entryDirectory},
		{path: ".git/packed-refs", kind: entryDirectory},
		{path: ".git/objects", kind: entryRegular},
	}
	for _, test := range tests {
		if got := classifySourceAdministrativePath(test.path, test.kind, objectFormatSHA1); got != sourceForbidden {
			t.Errorf("classifySourceAdministrativePath(%q, %v) = %v, want forbidden", test.path, test.kind, got)
		}
	}
}
