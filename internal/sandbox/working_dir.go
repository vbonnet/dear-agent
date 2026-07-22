package sandbox

import (
	"fmt"
	"path/filepath"
)

// WorkingDirMatch identifies the most specific lower directory containing a
// requested host working directory. Providers use the index and relative path
// to map that directory into their own workspace topology.
type WorkingDirMatch struct {
	LowerDir    string
	LowerIndex  int
	RelativeDir string
}

// MatchWorkingDir resolves workingDir and lowerDirs through symlinks, then
// returns the most specific containing lower directory. Selecting the most
// specific match keeps nested configured repositories deterministic.
func MatchWorkingDir(workingDir string, lowerDirs []string) (WorkingDirMatch, error) {
	if workingDir == "" {
		return WorkingDirMatch{}, NewInvalidConfigError("WorkingDir", "must not be empty")
	}
	resolvedWorkingDir, err := canonicalExistingPath(workingDir)
	if err != nil {
		return WorkingDirMatch{}, NewInvalidConfigError("WorkingDir", fmt.Sprintf("cannot resolve %q: %v", workingDir, err))
	}

	best := WorkingDirMatch{LowerIndex: -1}
	bestSpecificity := -1
	for index, lowerDir := range lowerDirs {
		resolvedLowerDir, resolveErr := canonicalExistingPath(lowerDir)
		if resolveErr != nil {
			continue
		}
		relativeDir, relativeErr := filepath.Rel(resolvedLowerDir, resolvedWorkingDir)
		if relativeErr != nil || relativeDir == ".." || filepath.IsAbs(relativeDir) || startsWithParent(relativeDir) {
			continue
		}
		if len(resolvedLowerDir) <= bestSpecificity {
			continue
		}
		best = WorkingDirMatch{
			LowerDir:    lowerDir,
			LowerIndex:  index,
			RelativeDir: relativeDir,
		}
		bestSpecificity = len(resolvedLowerDir)
	}
	if best.LowerIndex < 0 {
		return WorkingDirMatch{}, NewInvalidConfigError(
			"WorkingDir",
			fmt.Sprintf("%q is not contained by any configured lower directory", workingDir),
		)
	}
	return best, nil
}

// MapFlatWorkingDir maps a requested host directory into providers that expose
// one selected lower directory at the merged root. The matched lower directory
// is returned so worktree-backed adapters materialize the same repository they
// map into.
func MapFlatWorkingDir(workingDir string, lowerDirs []string, mergedRoot string) (string, string, error) {
	if workingDir == "" {
		return mergedRoot, "", nil
	}
	match, err := MatchWorkingDir(workingDir, lowerDirs)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(mergedRoot, match.RelativeDir), match.LowerDir, nil
}

// PrioritizeLowerDir returns a copy with selected first while retaining the
// relative order of every other lower directory. Flat union providers use this
// after MapFlatWorkingDir so the mapped project is also the visible repository
// when lower directories contain colliding paths.
func PrioritizeLowerDir(lowerDirs []string, selected string) []string {
	ordered := append([]string(nil), lowerDirs...)
	if selected == "" {
		return ordered
	}
	for index, lowerDir := range ordered {
		if lowerDir != selected || index == 0 {
			continue
		}
		copy(ordered[1:index+1], ordered[0:index])
		ordered[0] = selected
		break
	}
	return ordered
}

func canonicalExistingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func startsWithParent(relative string) bool {
	return len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator)
}
