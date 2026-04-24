package status

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PhaseFile represents a phase file and its validation status
type PhaseFile struct {
	Name        string     // e.g., "D1", "S7", "W0"
	Path        string     // full path to file
	Validated   bool       // has validation signature?
	ValidatedAt *time.Time // when was it validated?
	Checksum    string     // SHA256 hash from signature
	Version     string     // validator version from signature
}

// ValidationSignature represents the YAML frontmatter signature added by validator
type ValidationSignature struct {
	Validated        bool      `yaml:"validated"`
	ValidatedAt      time.Time `yaml:"validated_at"`
	ValidatorVersion string    `yaml:"validator_version"`
	Checksum         string    `yaml:"checksum"`
}

// phaseFilePattern matches v1 phase files: W0.md, D1.md, S4.md, etc.
var phaseFilePatternV1 = regexp.MustCompile(`^([WDS])(\d+)\.md$`)

// phaseFilePatternV2 matches v2 phase files: discovery-problem.md, build-implement.md, definition.md, etc.
var phaseFilePatternV2 = regexp.MustCompile(`^([a-z]+(-[a-z]+)*)\.md$`)

// ScanPhaseFiles scans directory for phase files (v1 or v2 format)
// Returns slice of PhaseFile with validation status
func ScanPhaseFiles(dir string) ([]PhaseFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var phaseFiles []PhaseFile
	wayfinderVersion := detectWayfinderVersion(dir)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		var phaseName string

		// Try v1 pattern first (W0.md, D1.md, S4.md)
		if matches := phaseFilePatternV1.FindStringSubmatch(entry.Name()); matches != nil {
			phaseName = matches[1] + matches[2] // e.g., "D1", "S7"
		} else if phaseFilePatternV2.MatchString(entry.Name()) {
			// Try v2 pattern (discovery-problem.md, definition.md)
			phaseName = FileNameToPhase(entry.Name())
			if phaseName == "" {
				continue // Invalid format
			}
		} else {
			continue // Not a phase file
		}

		// Build full path
		fullPath := filepath.Join(dir, entry.Name())

		// Check for validation signature
		validated, validatedAt, checksum, version := checkSignature(fullPath)

		phaseFiles = append(phaseFiles, PhaseFile{
			Name:        phaseName,
			Path:        fullPath,
			Validated:   validated,
			ValidatedAt: validatedAt,
			Checksum:    checksum,
			Version:     version,
		})
	}

	// Sort by phase sequence using version-aware index
	sort.Slice(phaseFiles, func(i, j int) bool {
		return phaseIndex(phaseFiles[i].Name, wayfinderVersion) < phaseIndex(phaseFiles[j].Name, wayfinderVersion)
	})

	return phaseFiles, nil
}

// DetectFromFilesystem creates Status by scanning phase files on disk
// This is the stateless alternative to ReadFrom()
func DetectFromFilesystem(dir string) (*Status, error) {
	// Scan for phase files
	phaseFiles, err := ScanPhaseFiles(dir)
	if err != nil {
		return nil, err
	}

	// Create status object
	status := &Status{
		SchemaVersion: SchemaVersion,
		ProjectPath:   dir,
		Status:        StatusInProgress,
		Phases:        make([]Phase, 0),
	}

	// Build Phases array from phase files
	for _, pf := range phaseFiles {
		var phaseStatus string
		var completedAt *time.Time
		var outcome string

		if pf.Validated {
			phaseStatus = PhaseStatusCompleted
			completedAt = pf.ValidatedAt
			outcome = OutcomeSuccess
		} else {
			// File exists but no signature = in progress
			phaseStatus = PhaseStatusInProgress
		}

		status.Phases = append(status.Phases, Phase{
			Name:        pf.Name,
			Status:      phaseStatus,
			CompletedAt: completedAt,
			Outcome:     outcome,
		})
	}

	// Determine CurrentPhase
	status.CurrentPhase = determineCurrentPhase(phaseFiles)

	// Try to read SessionID from STATUS file if it exists
	// (fallback for backward compatibility)
	if statusFilePath := filepath.Join(dir, StatusFilename); fileExists(statusFilePath) {
		if existingStatus, err := ReadFrom(dir); err == nil {
			status.SessionID = existingStatus.SessionID
			status.StartedAt = existingStatus.StartedAt
			status.EndedAt = existingStatus.EndedAt
		}
	}

	// If still no SessionID, leave empty (will be generated when needed)
	if status.SessionID == "" {
		// Don't generate UUID here - let caller decide
		status.SessionID = ""
	}

	// Set StartedAt if not set
	if status.StartedAt.IsZero() && len(phaseFiles) > 0 {
		// Use earliest phase file as start time
		info, err := os.Stat(phaseFiles[0].Path)
		if err == nil {
			status.StartedAt = info.ModTime()
		}
	}

	return status, nil
}

// determineCurrentPhase finds the current phase based on file states
// Returns the first incomplete phase, or the last completed phase if all done
func determineCurrentPhase(phaseFiles []PhaseFile) string {
	if len(phaseFiles) == 0 {
		return ""
	}

	// Find first non-validated phase
	for _, pf := range phaseFiles {
		if !pf.Validated {
			return pf.Name
		}
	}

	// All phases validated - return last one
	return phaseFiles[len(phaseFiles)-1].Name
}

// checkSignature reads file frontmatter and checks for validation signature
// Returns (validated, validatedAt, checksum, version)
func checkSignature(filePath string) (bool, *time.Time, string, string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, nil, "", ""
	}

	// Extract frontmatter
	yamlContent, err := extractFrontmatterFromContent(string(content))
	if err != nil {
		return false, nil, "", ""
	}

	// Parse as generic map to check for signature fields
	var frontmatter map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &frontmatter); err != nil {
		return false, nil, "", ""
	}

	// Check for validated field
	validated, ok := frontmatter["validated"].(bool)
	if !ok || !validated {
		return false, nil, "", ""
	}

	// Extract validation timestamp
	var validatedAt *time.Time
	if ts, ok := frontmatter["validated_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			validatedAt = &parsed
		}
	} else if ts, ok := frontmatter["validated_at"].(time.Time); ok {
		validatedAt = &ts
	}

	// Extract checksum and version
	checksum, _ := frontmatter["checksum"].(string)
	version, _ := frontmatter["validator_version"].(string)

	return true, validatedAt, checksum, version
}

// extractFrontmatterFromContent extracts YAML between --- delimiters from content string
func extractFrontmatterFromContent(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return "", fmt.Errorf("invalid frontmatter: must start with ---")
	}

	var yamlLines []string
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return strings.Join(yamlLines, "\n"), nil
		}
		yamlLines = append(yamlLines, lines[i])
	}

	return "", fmt.Errorf("invalid frontmatter: missing closing ---")
}

// detectWayfinderVersion reads WAYFINDER-STATUS.md to determine version
// Returns v1 if file doesn't exist or version field is missing (backward compat)
func detectWayfinderVersion(dir string) string {
	statusPath := filepath.Join(dir, StatusFilename)
	if !fileExists(statusPath) {
		return WayfinderV1 // Default to v1
	}

	// Try to read status file
	st, err := ReadFrom(dir)
	if err != nil {
		return WayfinderV1 // Default to v1
	}

	return st.GetVersion()
}

// phaseIndex returns sort index for a phase name using version-aware AllPhases()
// V1: W0=0, D1=1, D2=2, D3=3, D4=4, S4=5, S5=6, ..., S11=12
// V2: discovery.problem=0, discovery.solutions=1, ..., retrospective=16
func phaseIndex(phaseName, version string) int {
	allPhases := AllPhases(version)
	for i, phase := range allPhases {
		if phase == phaseName {
			return i
		}
	}
	return 999 // Unknown phase sorts to end
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
