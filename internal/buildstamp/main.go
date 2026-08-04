// Package main provides non-shipped GOFLAGS admission and default Git
// provenance helpers for governed root-Makefile builds.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	rawGOFLAGSKey = "_BUILD_STAMP_RAW_GOFLAGS"
	rawGOENVKey   = "_BUILD_STAMP_RAW_GOENV"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "git-commit" {
		fmt.Fprintln(os.Stdout, defaultGitCommit(".", runGit))
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: buildstamp [git-commit]")
		os.Exit(2)
	}
	if err := guard(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func guard() error {
	goflags, err := effectiveGOFLAGS()
	if err != nil {
		return fmt.Errorf("build-stamp GOFLAGS guard: %w", err)
	}

	fields, err := splitGOFLAGS(goflags)
	if err != nil {
		return fmt.Errorf("GOFLAGS uses invalid Go quoted-field syntax: %w", err)
	}
	if hasLinkerFlag(fields) {
		return errors.New("GOFLAGS must not contain -ldflags or --ldflags for governed builds; use EXTRA_GO_LDFLAGS")
	}
	return nil
}

func effectiveGOFLAGS() (string, error) {
	if raw := os.Getenv(rawGOFLAGSKey); raw != "" {
		return raw, nil
	}

	cmd := exec.Command("go", "env", "-json", "GOFLAGS")
	cmd.Env = goEnvQueryEnvironment(os.Environ(), os.Getenv(rawGOENVKey))
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("could not resolve persisted GOFLAGS with go env")
	}

	var values map[string]string
	if err := json.Unmarshal(output, &values); err != nil {
		return "", errors.New("go env returned invalid GOFLAGS JSON")
	}
	goflags, ok := values["GOFLAGS"]
	if !ok {
		return "", errors.New("go env did not return GOFLAGS")
	}
	return goflags, nil
}

func goEnvQueryEnvironment(current []string, rawGOENV string) []string {
	env := make([]string, 0, len(current)+1)
	for _, entry := range current {
		key, _, found := strings.Cut(entry, "=")
		if found && (key == "GOFLAGS" || key == "GOENV") {
			continue
		}
		env = append(env, entry)
	}
	if rawGOENV != "" {
		env = append(env, "GOENV="+rawGOENV)
	}
	return env
}

// splitGOFLAGS mirrors the quoted-field grammar used by Go 1.26.5 in
// cmd/internal/quoted.Split. That package is internal to the Go distribution,
// so the build-stamp guard keeps this small compatibility implementation local.
func splitGOFLAGS(input string) ([]string, error) {
	var fields []string
	for cursor := 0; cursor < len(input); {
		for cursor < len(input) && isGOFlagSpace(input[cursor]) {
			cursor++
		}
		if cursor == len(input) {
			break
		}

		if input[cursor] == '\'' || input[cursor] == '"' {
			quote := input[cursor]
			start := cursor + 1
			end := start
			for end < len(input) && input[end] != quote {
				end++
			}
			if end == len(input) {
				return nil, fmt.Errorf("unterminated %c string", quote)
			}
			fields = append(fields, input[start:end])
			cursor = end + 1
			continue
		}

		start := cursor
		for cursor < len(input) && !isGOFlagSpace(input[cursor]) {
			cursor++
		}
		fields = append(fields, input[start:cursor])
	}
	return fields, nil
}

func isGOFlagSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func hasLinkerFlag(fields []string) bool {
	for _, field := range fields {
		name, _, _ := strings.Cut(field, "=")
		if name == "-ldflags" || name == "--ldflags" {
			return true
		}
	}
	return false
}
