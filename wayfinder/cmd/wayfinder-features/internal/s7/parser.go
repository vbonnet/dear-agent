// Package s7 provides s7-related functionality.
package s7

import (
	"fmt"

	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// S7Feature represents a feature from the S7 plan
type S7Feature struct {
	ID           string
	Description  string
	Estimate     string
	Verification string
}

// ParseS7Plan parses the Feature Tracking table from S7 plan
func ParseS7Plan(path string) (string, []S7Feature, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read S7 plan: %w", err)
	}

	content := string(data)

	// Extract project name from header
	project := extractProjectName(content)

	// Find Feature Tracking section
	sectionRegex := regexp.MustCompile(`(?s)##\s+Feature Tracking.*?\n.*?\n\|\s*Feature ID\s*\|.*?\|\s*Estimate\s*\|.*?\|\s*\n\|[-:\s|]+\|\s*\n((?:\|[^|]+\|[^|]+\|[^|]+\|[^|]+\|\s*\n)+)`)
	matches := sectionRegex.FindStringSubmatch(content)
	if len(matches) < 2 {
		return "", nil, fmt.Errorf("Feature Tracking table not found in S7 plan\nEnsure your S7 plan includes:\n\n## Feature Tracking\n\n| Feature ID | Description | Estimate | Verification |\n|------------|-------------|----------|--------------|")
	}

	// Parse table rows
	rowRegex := regexp.MustCompile(`\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|`)
	rows := strings.Split(strings.TrimSpace(matches[1]), "\n")

	var features []S7Feature
	for _, row := range rows {
		if m := rowRegex.FindStringSubmatch(row); len(m) == 5 {
			featureID := strings.TrimSpace(m[1])
			// Skip example rows (feature-1, feature-2)
			if featureID == "feature-1" || featureID == "feature-2" {
				continue
			}
			features = append(features, S7Feature{
				ID:           featureID,
				Description:  strings.TrimSpace(m[2]),
				Estimate:     strings.TrimSpace(m[3]),
				Verification: strings.TrimSpace(m[4]),
			})
		}
	}

	if len(features) == 0 {
		return "", nil, fmt.Errorf("no features found in Feature Tracking table\nReplace example rows (feature-1, feature-2) with actual features")
	}

	return project, features, nil
}

// extractProjectName tries to extract project name from plan header
func extractProjectName(content string) string {
	// Try "# Implementation Plan: [Project Name]"
	headerRegex := regexp.MustCompile(`#\s+Implementation Plan:\s*(.+)`)
	if matches := headerRegex.FindStringSubmatch(content); len(matches) >= 2 {
		name := strings.TrimSpace(matches[1])
		if name != "[Project Name]" {
			return name
		}
	}

	// Try "**Project**: [Name]"
	projectRegex := regexp.MustCompile(`\*\*Project\*\*:\s*(.+)`)
	if matches := projectRegex.FindStringSubmatch(content); len(matches) >= 2 {
		name := strings.TrimSpace(matches[1])
		if name != "[Project Name]" {
			return name
		}
	}

	// Default
	return "unknown-project"
}

// FindS7Plan searches up the directory tree for S7 plan
func FindS7Plan() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Common plan filenames
	planNames := []string{
		"S7-plan.md",
		"plan.md",
		"S7-implementation-plan.md",
	}

	// Search current directory and up the tree
	dir := cwd
	for {
		for _, name := range planNames {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("S7 plan not found (searched for: %s)\nEnsure you're in a Wayfinder project directory", strings.Join(planNames, ", "))
}
