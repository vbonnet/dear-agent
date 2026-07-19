// Command coverage-ratchet runs the packages in a versioned coverage policy
// and fails when statement coverage falls below any package's recorded floor.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Minute

type policy struct {
	Version  int      `json:"version"`
	Packages []target `json:"packages"`
}

type target struct {
	Name              string  `json:"name"`
	Package           string  `json:"package"`
	MinimumStatements float64 `json:"minimum_statements"`
}

type coverage struct {
	Covered int
	Total   int
}

func (c coverage) percent() float64 {
	if c.Total == 0 {
		return 0
	}
	return float64(c.Covered) * 100 / float64(c.Total)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("coverage-ratchet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	policyPath := fs.String("policy", "agm/test/coverage/critical-lifecycle.json", "coverage policy JSON")
	race := fs.Bool("race", false, "run package tests with the race detector")
	timeout := fs.Duration("timeout", defaultTimeout, "timeout for each package")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *timeout <= 0 {
		fmt.Fprintln(stderr, "coverage-ratchet: policy accepts no positional arguments and timeout must be positive")
		return 2
	}

	p, err := loadPolicy(*policyPath)
	if err != nil {
		fmt.Fprintf(stderr, "coverage-ratchet: %v\n", err)
		return 2
	}

	tempDir, err := os.MkdirTemp("", "coverage-ratchet-")
	if err != nil {
		fmt.Fprintf(stderr, "coverage-ratchet: create temporary directory: %v\n", err)
		return 2
	}
	defer os.RemoveAll(tempDir)

	failed := false
	for i, item := range p.Packages {
		profile := filepath.Join(tempDir, fmt.Sprintf("%02d.cover", i))
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		output, runErr := runCoverage(ctx, item.Package, profile, *race)
		cancel()
		if len(output) > 0 {
			_, _ = stdout.Write(output)
		}
		if runErr != nil {
			fmt.Fprintf(stderr, "coverage-ratchet: test %s: %v\n", item.Package, runErr)
			failed = true
			continue
		}

		profileFile, openErr := os.Open(profile)
		if openErr != nil {
			fmt.Fprintf(stderr, "coverage-ratchet: open %s profile: %v\n", item.Name, openErr)
			failed = true
			continue
		}
		measured, parseErr := parseProfile(profileFile)
		closeErr := profileFile.Close()
		if parseErr != nil {
			fmt.Fprintf(stderr, "coverage-ratchet: parse %s profile: %v\n", item.Name, parseErr)
			failed = true
			continue
		}
		if closeErr != nil {
			fmt.Fprintf(stderr, "coverage-ratchet: close %s profile: %v\n", item.Name, closeErr)
			failed = true
			continue
		}

		actual := measured.percent()
		status := "PASS"
		if actual+0.000001 < item.MinimumStatements {
			status = "FAIL"
			failed = true
		}
		fmt.Fprintf(stdout, "%s %s: %.1f%% statements (minimum %.1f%%)\n", status, item.Name, actual, item.MinimumStatements)
	}
	if failed {
		return 1
	}
	return 0
}

func loadPolicy(path string) (policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return policy{}, fmt.Errorf("read policy: %w", err)
	}
	var p policy
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return policy{}, fmt.Errorf("decode policy: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return policy{}, errors.New("decode policy: trailing JSON value")
	}
	if err := validatePolicy(p); err != nil {
		return policy{}, err
	}
	return p, nil
}

func validatePolicy(p policy) error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported policy version %d", p.Version)
	}
	if len(p.Packages) == 0 {
		return errors.New("policy has no packages")
	}
	seen := make(map[string]struct{}, len(p.Packages))
	for _, item := range p.Packages {
		if item.Name == "" || item.Package == "" {
			return errors.New("policy package requires name and package")
		}
		if item.MinimumStatements < 0 || item.MinimumStatements > 100 {
			return fmt.Errorf("package %s has invalid minimum %.1f", item.Name, item.MinimumStatements)
		}
		if _, ok := seen[item.Package]; ok {
			return fmt.Errorf("duplicate package %s", item.Package)
		}
		seen[item.Package] = struct{}{}
	}
	return nil
}

func runCoverage(ctx context.Context, packagePath, profile string, race bool) ([]byte, error) {
	args := []string{"test", "-count=1"}
	if race {
		args = append(args, "-race")
	}
	args = append(args, "-coverprofile="+profile, packagePath)
	cmd := exec.CommandContext(ctx, "go", args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return output, fmt.Errorf("timed out: %w", ctx.Err())
	}
	return output, err
}

func parseProfile(r io.Reader) (coverage, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return coverage{}, errors.New("empty coverage profile")
	}
	if !strings.HasPrefix(scanner.Text(), "mode: ") {
		return coverage{}, errors.New("missing coverage mode header")
	}

	var result coverage
	for line := 2; scanner.Scan(); line++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return coverage{}, fmt.Errorf("line %d: want 3 fields", line)
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil || statements < 0 {
			return coverage{}, fmt.Errorf("line %d: invalid statement count %q", line, fields[1])
		}
		count, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || count < 0 {
			return coverage{}, fmt.Errorf("line %d: invalid execution count %q", line, fields[2])
		}
		result.Total += statements
		if count > 0 {
			result.Covered += statements
		}
	}
	if err := scanner.Err(); err != nil {
		return coverage{}, err
	}
	if result.Total == 0 {
		return coverage{}, errors.New("coverage profile has no statements")
	}
	return result, nil
}
