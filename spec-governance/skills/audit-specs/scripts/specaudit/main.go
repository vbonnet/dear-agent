// Command specaudit inventories and renders evidence for a read-only SPEC audit.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/earslint"
)

const schemaVersion = "spec-audit/v1"

var (
	requirementPattern  = regexp.MustCompile(`^\s*\*\*([A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*-[A-Z0-9]*\d+)\*\*\s+(.+?)\s*$`)
	featurePattern      = regexp.MustCompile(`[A-Za-z0-9_./-]+\.feature`)
	shaPattern          = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	canonicalEARSLinter = mustEARSLinter()
)

func mustEARSLinter() *earslint.Linter {
	linter, err := earslint.New(earslint.DefaultConfig())
	if err != nil {
		panic(err)
	}
	return linter
}

type report struct {
	SchemaVersion string        `json:"schema_version"`
	Snapshot      snapshot      `json:"snapshot"`
	Scope         scope         `json:"scope"`
	Summary       summary       `json:"summary"`
	Methodology   methodology   `json:"methodology"`
	Inventory     []specFile    `json:"inventory,omitempty"`
	Features      []featureFile `json:"features,omitempty"`
	Seeds         []seed        `json:"seeds,omitempty"`
	Candidates    []finding     `json:"candidates"`
	NonCandidates []finding     `json:"non_candidates"`
	Limitations   []string      `json:"limitations"`
}

type snapshot struct {
	Repository          string `json:"repository"`
	Revision            string `json:"revision"`
	ComparisonRevision  string `json:"comparison_revision,omitempty"`
	RevisionCommittedAt string `json:"revision_committed_at"`
	GeneratedAt         string `json:"generated_at,omitempty"`
}

type scope struct {
	Roots         []string    `json:"roots"`
	Excluded      []exclusion `json:"excluded"`
	ActiveMembers []string    `json:"active_members"`
}

type exclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type summary struct {
	SpecFiles      int            `json:"spec_files"`
	Requirements   int            `json:"requirements"`
	Diagnostics    int            `json:"diagnostics"`
	CandidateCount int            `json:"candidate_count"`
	ByVerdict      map[string]int `json:"by_verdict"`
}

type methodology struct {
	Collector      string   `json:"collector"`
	SeedKinds      []string `json:"seed_kinds"`
	SemanticReview string   `json:"semantic_review"`
	Reproduce      []string `json:"reproduce"`
}

type specFile struct {
	Path         string        `json:"path"`
	SHA256       string        `json:"sha256"`
	Requirements []requirement `json:"requirements"`
	BDDFeatures  []bddRef      `json:"bdd_features"`
	Diagnostics  []diagnostic  `json:"diagnostics"`
}

type diagnostic struct {
	Line    int    `json:"line"`
	Kind    string `json:"kind"`
	Excerpt string `json:"excerpt"`
}

type featureFile struct {
	Path         string   `json:"path"`
	SHA256       string   `json:"sha256"`
	RelatedSpecs []string `json:"related_specs"`
}

type requirement struct {
	ID      string `json:"id"`
	Line    int    `json:"line"`
	Body    string `json:"body"`
	Excerpt string `json:"excerpt"`
}

type ownerClaim struct {
	Path      string `json:"path"`
	Rationale string `json:"rationale"`
}

type proposedOwnerClaim struct {
	Path      string `json:"path"`
	State     string `json:"state"`
	Rationale string `json:"rationale"`
}

type bddRef struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

type seed struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Key      string     `json:"key"`
	Evidence []evidence `json:"evidence"`
}

type finding struct {
	ID                     string              `json:"id"`
	Rank                   int                 `json:"rank,omitempty"`
	Title                  string              `json:"title"`
	Verdict                string              `json:"verdict"`
	Relationship           string              `json:"relationship"`
	Classification         string              `json:"classification"`
	Confidence             string              `json:"confidence"`
	Strength               string              `json:"strength"`
	CurrentOwners          []ownerClaim        `json:"current_owners"`
	OwnershipCompleteness  string              `json:"ownership_completeness,omitempty"`
	ProposedOwner          *proposedOwnerClaim `json:"proposed_owner,omitempty"`
	SharedOutcome          string              `json:"shared_outcome"`
	MaterialDifferences    []string            `json:"material_differences"`
	Evidence               []evidence          `json:"evidence"`
	ApplicabilityBasis     string              `json:"applicability_basis,omitempty"`
	ApplicabilityRationale string              `json:"applicability_rationale,omitempty"`
	Applicability          []applicability     `json:"applicability"`
	BDD                    bddImpact           `json:"bdd"`
	Recommendation         []string            `json:"recommendation"`
	Risk                   string              `json:"risk"`
	Limitations            []string            `json:"limitations"`
	Decision               string              `json:"decision"`
	Boundary               string              `json:"boundary,omitempty"`
}

type evidence struct {
	Path          string `json:"path"`
	Line          int    `json:"line"`
	RequirementID string `json:"requirement_id,omitempty"`
	Excerpt       string `json:"excerpt"`
}

type applicability struct {
	Member      string     `json:"member"`
	Disposition string     `json:"disposition"`
	Evidence    []evidence `json:"evidence"`
}

type bddImpact struct {
	Features    []string `json:"features"`
	Consequence string   `json:"consequence"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "inventory":
		return runInventory(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "specaudit: unknown subcommand %q\n", args[0])
		usage(stderr)
		return 2
	}
}

func usage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  specaudit inventory -repo <path> -repository <owner/name> -revision <commit>")
	fmt.Fprintln(out, "  specaudit validate -input <report.json> -inventory <inventory.json> -repo <path>")
	fmt.Fprintln(out, "  specaudit render -input <report.json> -inventory <inventory.json> -repo <path>")
	fmt.Fprintln(out, "Inventory JSON and rendered HTML are emitted only to stdout; use an authorized shell redirection to store them.")
}

func runInventory(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inventory", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoPath := fs.String("repo", "", "Git repository root or descendant")
	repository := fs.String("repository", "", "stable repository identity, for example owner/name")
	revision := fs.String("revision", "", "pinned Git commit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *repoPath == "" || *repository == "" || *revision == "" || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	report, err := inventory(*repoPath, *repository, *revision)
	if err != nil {
		fmt.Fprintf(stderr, "specaudit inventory: %v\n", err)
		return 2
	}
	if err := validateReport(report); err != nil {
		fmt.Fprintf(stderr, "specaudit inventory: generated invalid report: %v\n", err)
		return 1
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "specaudit inventory: encode: %v\n", err)
		return 2
	}
	data = append(data, '\n')
	if _, err := stdout.Write(data); err != nil {
		fmt.Fprintf(stderr, "specaudit inventory: %v\n", err)
		return 2
	}
	return 0
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "report JSON")
	inventoryPath := fs.String("inventory", "", "pinned inventory JSON used to verify source evidence")
	repoPath := fs.String("repo", "", "repository path used to recompute and authenticate the supplied inventory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *input == "" || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if *inventoryPath == "" || *repoPath == "" {
		fmt.Fprintln(stderr, "specaudit validate: -inventory and -repo are required")
		return 2
	}
	auditReport, err := readReport(*input)
	if err != nil {
		fmt.Fprintf(stderr, "specaudit validate: %v\n", err)
		return 2
	}
	if err := validateReport(auditReport); err != nil {
		fmt.Fprintf(stderr, "specaudit validate: %v\n", err)
		return 1
	}
	{
		inventoryReport, inventoryErr := readReport(*inventoryPath)
		if inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit validate: read inventory: %v\n", inventoryErr)
			return 2
		}
		if inventoryErr = validateAgainstInventory(auditReport, inventoryReport); inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit validate: %v\n", inventoryErr)
			return 1
		}
		if *repoPath != "" {
			if inventoryErr = validateInventoryAgainstRepo(inventoryReport, *repoPath); inventoryErr != nil {
				fmt.Fprintf(stderr, "specaudit validate: %v\n", inventoryErr)
				return 1
			}
		}
	}
	fmt.Fprintln(stdout, "specaudit: valid spec-audit/v1 report")
	return 0
}

func runRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "report JSON")
	inventoryPath := fs.String("inventory", "", "pinned inventory JSON used to verify source evidence and summarize seeds")
	repoPath := fs.String("repo", "", "repository path used to recompute and authenticate the supplied inventory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *input == "" || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if *inventoryPath == "" || *repoPath == "" {
		fmt.Fprintln(stderr, "specaudit render: -inventory and -repo are required")
		return 2
	}
	auditReport, err := readReport(*input)
	if err != nil {
		fmt.Fprintf(stderr, "specaudit render: %v\n", err)
		return 2
	}
	if err := validateReport(auditReport); err != nil {
		fmt.Fprintf(stderr, "specaudit render: %v\n", err)
		return 1
	}
	var inventoryReport *report
	{
		decodedInventory, inventoryErr := readReport(*inventoryPath)
		if inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit render: read inventory: %v\n", inventoryErr)
			return 2
		}
		if inventoryErr = validateAgainstInventory(auditReport, decodedInventory); inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit render: %v\n", inventoryErr)
			return 1
		}
		if *repoPath != "" {
			if inventoryErr = validateInventoryAgainstRepo(decodedInventory, *repoPath); inventoryErr != nil {
				fmt.Fprintf(stderr, "specaudit render: %v\n", inventoryErr)
				return 1
			}
		}
		inventoryReport = &decodedInventory
	}
	htmlOutput := renderHTML(auditReport, inventoryReport)
	if _, err := fmt.Fprint(stdout, htmlOutput); err != nil {
		fmt.Fprintf(stderr, "specaudit render: %v\n", err)
		return 2
	}
	return 0
}

//nolint:gocyclo // Linear pinned-object collection keeps inventory and reciprocal-diagnostic construction auditable.
func inventory(repoPath, repository, revision string) (report, error) {
	root, err := git(repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return report{}, err
	}
	root = strings.TrimSpace(root)
	commit, err := git(root, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		return report{}, fmt.Errorf("resolve pinned revision %q: %w", revision, err)
	}
	commit = strings.TrimSpace(commit)
	pathsRaw, err := git(root, "ls-tree", "-r", "-z", "--name-only", commit)
	if err != nil {
		return report{}, fmt.Errorf("list %s: %w", commit, err)
	}
	paths := strings.Split(pathsRaw, "\x00")
	files := make([]specFile, 0)
	features := make([]featureFile, 0)
	for _, path := range paths {
		if path == "" {
			continue
		}
		switch {
		case filepath.Base(path) == "SPEC.md":
			body, showErr := git(root, "show", commit+":"+path)
			if showErr != nil {
				return report{}, fmt.Errorf("read %s at %s: %w", path, commit, showErr)
			}
			files = append(files, parseSpec(path, body))
		case strings.HasSuffix(path, ".feature"):
			body, showErr := git(root, "show", commit+":"+path)
			if showErr != nil {
				return report{}, fmt.Errorf("read %s at %s: %w", path, commit, showErr)
			}
			features = append(features, parseFeature(path, body))
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sort.Slice(features, func(i, j int) bool { return features[i].Path < features[j].Path })
	featureIndex := map[string]featureFile{}
	for _, feature := range features {
		featureIndex[feature.Path] = feature
	}
	for fileIndex := range files {
		for _, ref := range files[fileIndex].BDDFeatures {
			feature, ok := featureIndex[ref.Path]
			switch {
			case !ok:
				files[fileIndex].Diagnostics = append(files[fileIndex].Diagnostics, diagnostic{Line: ref.Line, Kind: "missing-bdd-feature", Excerpt: ref.Path})
			case !containsString(feature.RelatedSpecs, files[fileIndex].Path):
				files[fileIndex].Diagnostics = append(files[fileIndex].Diagnostics, diagnostic{Line: ref.Line, Kind: "nonreciprocal-bdd-feature", Excerpt: ref.Path})
			}
		}
	}

	commitTime, err := git(root, "show", "-s", "--format=%cI", commit)
	if err != nil {
		return report{}, fmt.Errorf("read commit timestamp: %w", err)
	}
	revisionCommittedAt := strings.TrimSpace(commitTime)
	active, activeLimitations := activeMembers(root, commit)
	requirementCount := 0
	diagnosticCount := 0
	for _, file := range files {
		requirementCount += len(file.Requirements)
		diagnosticCount += len(file.Diagnostics)
	}
	return report{
		SchemaVersion: schemaVersion,
		Snapshot:      snapshot{Repository: strings.TrimSpace(repository), Revision: commit, RevisionCommittedAt: revisionCommittedAt},
		Scope:         scope{Roots: []string{"."}, Excluded: []exclusion{}, ActiveMembers: active},
		Summary:       summary{SpecFiles: len(files), Requirements: requirementCount, Diagnostics: diagnosticCount, CandidateCount: 0, ByVerdict: map[string]int{}},
		Methodology: methodology{
			Collector:      "specaudit inventory",
			SeedKinds:      []string{"exact-body", "duplicate-id", "shared-bdd", "identical-file"},
			SemanticReview: "Seeds are lexical leads; source and BDD review determine every finding verdict.",
			Reproduce:      []string{fmt.Sprintf("specaudit inventory -repo . -repository %s -revision %s", strings.TrimSpace(repository), commit)},
		},
		Inventory:     files,
		Features:      features,
		Seeds:         collectSeeds(files),
		Candidates:    []finding{},
		NonCandidates: []finding{},
		Limitations:   activeLimitations,
	}, nil
}

func parseSpec(path, body string) specFile {
	digest := sha256.Sum256([]byte(body))
	file := specFile{Path: filepath.ToSlash(path), SHA256: fmt.Sprintf("%x", digest), Requirements: []requirement{}, BDDFeatures: []bddRef{}, Diagnostics: []diagnostic{}}
	inFence := false
	inBDDTraceability := false
	seenFeatures := map[string]bool{}
	for index, line := range strings.Split(body, "\n") {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if heading, ok := strings.CutPrefix(trimmed, "## "); ok {
			inBDDTraceability = strings.EqualFold(strings.TrimSpace(heading), "BDD Traceability")
		}
		match := requirementPattern.FindStringSubmatch(line)
		totalRequirements, validRequirements := lintRequirementLine(line)
		if len(match) > 0 && validRequirements == 1 && totalRequirements == 1 {
			file.Requirements = append(file.Requirements, requirement{ID: match[1], Line: lineNumber, Body: normalize(match[2]), Excerpt: strings.TrimSpace(line)})
		} else if totalRequirements > 0 {
			kind := "nonconforming-requirement"
			if validRequirements == 1 {
				kind = "anonymous-requirement"
			}
			file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: kind, Excerpt: strings.TrimSpace(line)})
		}
		if inBDDTraceability {
			for _, feature := range featurePattern.FindAllString(line, -1) {
				feature = filepath.ToSlash(feature)
				if validFeaturePath(feature) && !seenFeatures[feature] {
					file.BDDFeatures = append(file.BDDFeatures, bddRef{Path: feature, Line: lineNumber})
					seenFeatures[feature] = true
				}
			}
		}
	}
	return file
}

func lintRequirementLine(line string) (int, int) {
	result, err := canonicalEARSLinter.Lint("requirement", strings.NewReader(line+"\n"))
	if err != nil {
		return 0, 0
	}
	return result.TotalRequirements, result.ValidRequirements
}

func parseFeature(path, body string) featureFile {
	digest := sha256.Sum256([]byte(body))
	related := make([]string, 0)
	seen := map[string]bool{}
	inFence := false
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, marker := range []string{"# SPEC:", "# RELATED-SPEC:"} {
			value, ok := strings.CutPrefix(trimmed, marker)
			if !ok {
				continue
			}
			value = filepath.ToSlash(strings.TrimSpace(value))
			if isSpecPath(value) && !seen[value] {
				related = append(related, value)
				seen[value] = true
			}
		}
	}
	sort.Strings(related)
	return featureFile{Path: filepath.ToSlash(path), SHA256: fmt.Sprintf("%x", digest), RelatedSpecs: related}
}

func collectSeeds(files []specFile) []seed {
	type bodyEntry struct {
		key      string
		evidence evidence
	}
	bodies := map[string][]bodyEntry{}
	ids := map[string][]evidence{}
	features := map[string][]evidence{}
	identical := map[string][]evidence{}
	for _, file := range files {
		identical[file.SHA256] = append(identical[file.SHA256], evidence{Path: file.Path, Line: 1, Excerpt: "identical full SPEC body"})
		for _, req := range file.Requirements {
			e := evidence{Path: file.Path, Line: req.Line, RequirementID: req.ID, Excerpt: req.Excerpt}
			bodies[req.Body] = append(bodies[req.Body], bodyEntry{key: req.Body, evidence: e})
			ids[req.ID] = append(ids[req.ID], e)
		}
		for _, ref := range file.BDDFeatures {
			features[ref.Path] = append(features[ref.Path], evidence{Path: file.Path, Line: ref.Line, Excerpt: ref.Path})
		}
	}
	var seeds []seed
	appendSeed := func(kind, key string, evidence []evidence) {
		if distinctPaths(evidence) < 2 {
			return
		}
		sortEvidence(evidence)
		seeds = append(seeds, seed{ID: fmt.Sprintf("SEED-%s-%03d", strings.ToUpper(strings.ReplaceAll(kind, "-", "_")), len(seeds)+1), Kind: kind, Key: key, Evidence: evidence})
	}
	bodyKeys := sortedKeys(bodies)
	for _, key := range bodyKeys {
		evidence := make([]evidence, 0, len(bodies[key]))
		for _, entry := range bodies[key] {
			evidence = append(evidence, entry.evidence)
		}
		appendSeed("exact-body", key, evidence)
	}
	for _, id := range sortedKeys(ids) {
		appendSeed("duplicate-id", id, ids[id])
	}
	for _, feature := range sortedKeys(features) {
		appendSeed("shared-bdd", feature, features[feature])
	}
	for _, digest := range sortedKeys(identical) {
		appendSeed("identical-file", digest, identical[digest])
	}
	sort.Slice(seeds, func(i, j int) bool {
		if seeds[i].Kind == seeds[j].Kind {
			return seeds[i].Key < seeds[j].Key
		}
		return seeds[i].Kind < seeds[j].Kind
	})
	for index := range seeds {
		seeds[index].ID = fmt.Sprintf("SEED-%03d", index+1)
	}
	return seeds
}

func activeMembers(root, commit string) ([]string, []string) {
	body, err := git(root, "show", commit+":agm/internal/agent/harnesses.go")
	if err != nil {
		return []string{}, []string{"Active harness inventory was unavailable at the pinned revision."}
	}
	match := regexp.MustCompile(`activeHarnesses\s*=\s*\[\]string\{([^}]*)\}`).FindStringSubmatch(body)
	if match == nil {
		return []string{}, []string{"Active harness inventory could not be parsed at the pinned revision."}
	}
	quoted := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(match[1], -1)
	active := make([]string, 0, len(quoted))
	for _, item := range quoted {
		active = append(active, item[1])
	}
	sort.Strings(active)
	return active, nil
}

func git(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"--no-replace-objects", "-C", root}, args...)...)
	command.Env = cleanGitEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func cleanGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			environment = append(environment, entry)
		}
	}
	return append(environment,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_TERMINAL_PROMPT=0",
	)
}

func readReport(path string) (report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return report{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded report
	if err := decoder.Decode(&decoded); err != nil {
		return report{}, fmt.Errorf("decode %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return report{}, fmt.Errorf("decode %s: contains multiple JSON values", path)
	}
	return decoded, nil
}

//nolint:gocyclo // Exhaustive fail-closed schema validation is intentionally kept as one ordered guard sequence.
func validateReport(report report) error {
	if report.SchemaVersion != schemaVersion {
		return fmt.Errorf("schema_version must be %q", schemaVersion)
	}
	if strings.TrimSpace(report.Snapshot.Repository) == "" || !shaPattern.MatchString(report.Snapshot.Revision) {
		return errors.New("snapshot requires a repository and 40-hex revision")
	}
	if report.Snapshot.ComparisonRevision != "" && !shaPattern.MatchString(report.Snapshot.ComparisonRevision) {
		return errors.New("snapshot comparison_revision must be 40-hex when present")
	}
	if _, err := time.Parse(time.RFC3339, report.Snapshot.RevisionCommittedAt); err != nil {
		return fmt.Errorf("snapshot revision_committed_at must be RFC 3339: %w", err)
	}
	if report.Snapshot.GeneratedAt != "" {
		if _, err := time.Parse(time.RFC3339, report.Snapshot.GeneratedAt); err != nil {
			return fmt.Errorf("snapshot generated_at must be RFC 3339 when present: %w", err)
		}
	}
	if len(report.Scope.Roots) == 0 || !uniqueRelativePaths(report.Scope.Roots, true) {
		return errors.New("scope roots must be nonempty unique repository-relative paths")
	}
	if !validExclusions(report.Scope.Excluded) || !uniqueStrings(report.Scope.ActiveMembers) {
		return errors.New("scope exclusions and active_members must be nonempty, unique values")
	}
	if strings.TrimSpace(report.Methodology.Collector) == "" || strings.TrimSpace(report.Methodology.SemanticReview) == "" || len(report.Methodology.Reproduce) == 0 {
		return errors.New("methodology requires collector, semantic_review, and reproduce commands")
	}
	if !validEnumSlice(report.Methodology.SeedKinds, seedKinds) || !uniqueStrings(report.Methodology.SeedKinds) {
		return errors.New("methodology seed_kinds must be unique supported values")
	}
	if err := validateInventory(report.Inventory); err != nil {
		return err
	}
	if err := validateFeatures(report.Features); err != nil {
		return err
	}
	if report.Inventory != nil {
		requirementCount := 0
		diagnosticCount := 0
		for _, file := range report.Inventory {
			requirementCount += len(file.Requirements)
			diagnosticCount += len(file.Diagnostics)
		}
		if report.Summary.SpecFiles != len(report.Inventory) || report.Summary.Requirements != requirementCount || report.Summary.Diagnostics != diagnosticCount {
			return errors.New("summary spec_files, requirements, or diagnostics does not match inventory")
		}
	}
	if err := validateSeeds(report.Seeds); err != nil {
		return err
	}
	ids := map[string]bool{}
	ranks := map[int]bool{}
	active := stringSet(report.Scope.ActiveMembers)
	for _, finding := range report.Candidates {
		if err := validateFinding(finding, false, active); err != nil {
			return err
		}
		if ids[finding.ID] {
			return fmt.Errorf("finding ID %q is duplicated", finding.ID)
		}
		ids[finding.ID] = true
		if finding.Rank < 1 || finding.Rank > len(report.Candidates) || ranks[finding.Rank] {
			return fmt.Errorf("candidate %q has invalid or duplicate rank %d", finding.ID, finding.Rank)
		}
		ranks[finding.Rank] = true
	}
	for _, finding := range report.NonCandidates {
		if err := validateFinding(finding, true, active); err != nil {
			return err
		}
		if ids[finding.ID] {
			return fmt.Errorf("finding ID %q is duplicated", finding.ID)
		}
		ids[finding.ID] = true
	}
	if report.Summary.CandidateCount != len(report.Candidates) {
		return fmt.Errorf("summary candidate_count=%d, want %d", report.Summary.CandidateCount, len(report.Candidates))
	}
	if report.Summary.SpecFiles < 0 || report.Summary.Requirements < 0 || report.Summary.Diagnostics < 0 || !equalHistogram(report.Summary.ByVerdict, verdictHistogram(report)) {
		return errors.New("summary counts or by_verdict do not match report findings")
	}
	return nil
}

//nolint:gocyclo // Sequential authenticated cross-checks keep every trust transition visible and fail closed.
func validateAgainstInventory(semantic, inventory report) error {
	if err := validateReport(inventory); err != nil {
		return fmt.Errorf("inventory report is invalid: %w", err)
	}
	if inventory.Inventory == nil {
		return errors.New("inventory report must include its inventory")
	}
	if semantic.Snapshot.Repository != inventory.Snapshot.Repository || semantic.Snapshot.Revision != inventory.Snapshot.Revision || semantic.Snapshot.RevisionCommittedAt != inventory.Snapshot.RevisionCommittedAt {
		return errors.New("semantic report and inventory must name the same repository, pinned revision, and revision timestamp")
	}
	if semantic.Summary.SpecFiles != inventory.Summary.SpecFiles || semantic.Summary.Requirements != inventory.Summary.Requirements || semantic.Summary.Diagnostics != inventory.Summary.Diagnostics {
		return errors.New("semantic report corpus counts do not match the pinned inventory")
	}
	if !reflect.DeepEqual(semantic.Scope.Roots, inventory.Scope.Roots) || !reflect.DeepEqual(semantic.Scope.Excluded, inventory.Scope.Excluded) {
		return errors.New("semantic report scope roots or exclusions do not match the pinned inventory")
	}
	if !sameStringSet(semantic.Scope.ActiveMembers, inventory.Scope.ActiveMembers) {
		return errors.New("semantic report active members do not match the pinned inventory")
	}
	for _, limitation := range inventory.Limitations {
		if !containsString(semantic.Limitations, limitation) {
			return fmt.Errorf("semantic report omits inventory limitation %q", limitation)
		}
	}

	type requirementKey struct {
		line int
		id   string
	}
	requirements := map[string]map[requirementKey]string{}
	features := map[string]featureFile{}
	files := map[string]bool{}
	specFeatures := map[string]map[string]bool{}
	for _, file := range inventory.Inventory {
		files[file.Path] = true
		requirements[file.Path] = map[requirementKey]string{}
		specFeatures[file.Path] = map[string]bool{}
		for _, requirement := range file.Requirements {
			requirements[file.Path][requirementKey{line: requirement.Line, id: requirement.ID}] = requirement.Excerpt
		}
		for _, ref := range file.BDDFeatures {
			specFeatures[file.Path][ref.Path] = true
		}
	}
	for _, feature := range inventory.Features {
		features[feature.Path] = feature
	}

	for _, finding := range allFindings(semantic) {
		for _, owner := range finding.CurrentOwners {
			if !files[owner.Path] {
				return fmt.Errorf("finding %q names current owner %q outside the pinned inventory", finding.ID, owner.Path)
			}
		}
		evidenceOwners := map[string]bool{}
		for _, item := range finding.Evidence {
			if !files[item.Path] {
				return fmt.Errorf("finding %q evidence path %q is outside the pinned inventory", finding.ID, item.Path)
			}
			if item.RequirementID == "" {
				return fmt.Errorf("finding %q evidence %s:%d must name a requirement ID", finding.ID, item.Path, item.Line)
			}
			excerpt, ok := requirements[item.Path][requirementKey{line: item.Line, id: item.RequirementID}]
			if !ok {
				return fmt.Errorf("finding %q evidence %s:%d does not match requirement %q in the pinned inventory", finding.ID, item.Path, item.Line, item.RequirementID)
			}
			if strings.TrimSpace(item.Excerpt) != strings.TrimSpace(excerpt) {
				return fmt.Errorf("finding %q evidence %s:%d excerpt does not exactly match the pinned inventory", finding.ID, item.Path, item.Line)
			}
			evidenceOwners[item.Path] = true
		}
		for _, owner := range finding.CurrentOwners {
			if !evidenceOwners[owner.Path] {
				return fmt.Errorf("finding %q has no requirement evidence for current owner %q", finding.ID, owner.Path)
			}
		}
		if !sameStringSet(ownerPaths(finding.CurrentOwners), mapKeys(evidenceOwners)) {
			return fmt.Errorf("finding %q current owners must exactly match its source-evidence paths", finding.ID)
		}
		for _, applicabilityEntry := range finding.Applicability {
			for _, item := range applicabilityEntry.Evidence {
				if !files[item.Path] || item.RequirementID == "" {
					return fmt.Errorf("finding %q applicability for %q has evidence outside the pinned requirement inventory", finding.ID, applicabilityEntry.Member)
				}
				excerpt, ok := requirements[item.Path][requirementKey{line: item.Line, id: item.RequirementID}]
				if !ok || strings.TrimSpace(item.Excerpt) != strings.TrimSpace(excerpt) {
					return fmt.Errorf("finding %q applicability for %q does not exactly match pinned evidence", finding.ID, applicabilityEntry.Member)
				}
			}
		}
		if finding.ProposedOwner != nil && finding.ProposedOwner.State == "existing" && !files[finding.ProposedOwner.Path] {
			return fmt.Errorf("finding %q marks proposed owner %q existing, but it is absent from the pinned inventory", finding.ID, finding.ProposedOwner.Path)
		}
		if finding.ProposedOwner != nil && finding.ProposedOwner.State == "new" && files[finding.ProposedOwner.Path] {
			return fmt.Errorf("finding %q marks proposed owner %q new, but it already exists in the pinned inventory", finding.ID, finding.ProposedOwner.Path)
		}
		coveredOwners := map[string]bool{}
		for _, featurePath := range finding.BDD.Features {
			feature, ok := features[featurePath]
			if !ok {
				return fmt.Errorf("finding %q BDD feature %q is absent from the pinned feature inventory", finding.ID, featurePath)
			}
			featureOwners := map[string]bool{}
			for _, related := range feature.RelatedSpecs {
				featureOwners[related] = true
			}
			coveredFeature := false
			for _, owner := range finding.CurrentOwners {
				if featureOwners[owner.Path] && specFeatures[owner.Path][featurePath] {
					coveredOwners[owner.Path] = true
					coveredFeature = true
				}
			}
			if !coveredFeature {
				return fmt.Errorf("finding %q BDD feature %q does not reciprocally name any current owner", finding.ID, featurePath)
			}
		}
		for _, owner := range finding.CurrentOwners {
			if len(finding.BDD.Features) > 0 && !coveredOwners[owner.Path] {
				return fmt.Errorf("finding %q BDD features do not reciprocally name current owner %q", finding.ID, owner.Path)
			}
		}
	}
	return nil
}

func validateInventoryAgainstRepo(supplied report, repoPath string) error {
	recomputed, err := inventory(repoPath, supplied.Snapshot.Repository, supplied.Snapshot.Revision)
	if err != nil {
		return fmt.Errorf("recompute pinned inventory: %w", err)
	}
	if !reflect.DeepEqual(supplied.Snapshot, recomputed.Snapshot) ||
		!reflect.DeepEqual(supplied.Scope, recomputed.Scope) ||
		!reflect.DeepEqual(supplied.Summary, recomputed.Summary) ||
		!reflect.DeepEqual(supplied.Methodology, recomputed.Methodology) ||
		!reflect.DeepEqual(supplied.Inventory, recomputed.Inventory) ||
		!reflect.DeepEqual(supplied.Features, recomputed.Features) ||
		!reflect.DeepEqual(supplied.Seeds, recomputed.Seeds) ||
		!reflect.DeepEqual(supplied.Limitations, recomputed.Limitations) {
		return errors.New("supplied inventory does not match a fresh Git-object inventory at the pinned revision")
	}
	return nil
}

func allFindings(report report) []finding {
	result := make([]finding, 0, len(report.Candidates)+len(report.NonCandidates))
	result = append(result, report.Candidates...)
	result = append(result, report.NonCandidates...)
	return result
}

//nolint:gocyclo // Exhaustive inventory-shape validation is clearer as one ordered fail-closed pass.
func validateInventory(files []specFile) error {
	seen := map[string]bool{}
	for _, file := range files {
		if !validPath(file.Path) || filepath.Base(file.Path) != "SPEC.md" || !digestPattern.MatchString(file.SHA256) || seen[file.Path] {
			return fmt.Errorf("inventory has invalid or duplicate SPEC path %q", file.Path)
		}
		seen[file.Path] = true
		for _, req := range file.Requirements {
			if strings.TrimSpace(req.ID) == "" || req.Line < 1 || strings.TrimSpace(req.Body) == "" || strings.TrimSpace(req.Excerpt) == "" {
				return fmt.Errorf("inventory requirement in %s is incomplete", file.Path)
			}
		}
		for _, ref := range file.BDDFeatures {
			if !validFeaturePath(ref.Path) || ref.Line < 1 {
				return fmt.Errorf("inventory BDD reference in %s is invalid", file.Path)
			}
		}
		for _, diagnostic := range file.Diagnostics {
			if diagnostic.Line < 1 || !diagnosticKinds[diagnostic.Kind] || strings.TrimSpace(diagnostic.Excerpt) == "" {
				return fmt.Errorf("inventory diagnostic in %s is invalid", file.Path)
			}
		}
	}
	return nil
}

func validateFeatures(features []featureFile) error {
	seen := map[string]bool{}
	for _, feature := range features {
		if !validFeaturePath(feature.Path) || !digestPattern.MatchString(feature.SHA256) || seen[feature.Path] || !uniqueSpecPaths(feature.RelatedSpecs) {
			return fmt.Errorf("feature inventory has invalid or duplicate path %q", feature.Path)
		}
		seen[feature.Path] = true
	}
	return nil
}

func validateSeeds(seeds []seed) error {
	seen := map[string]bool{}
	for _, seed := range seeds {
		if strings.TrimSpace(seed.ID) == "" || seen[seed.ID] || !seedKinds[seed.Kind] || strings.TrimSpace(seed.Key) == "" || len(seed.Evidence) < 2 || distinctPaths(seed.Evidence) < 2 {
			return fmt.Errorf("seed %q is invalid", seed.ID)
		}
		seen[seed.ID] = true
		if err := validateEvidence(seed.Evidence); err != nil {
			return fmt.Errorf("seed %q: %w", seed.ID, err)
		}
	}
	return nil
}

//nolint:gocyclo // Exhaustive verdict and ownership invariants are intentionally validated in one fail-closed sequence.
func validateFinding(f finding, nonCandidate bool, active map[string]bool) error {
	if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.Title) == "" || !verdicts[f.Verdict] || !relationships[f.Relationship] || !classifications[f.Classification] || !confidences[f.Confidence] || !strengths[f.Strength] {
		return fmt.Errorf("finding %q has invalid required enumerations", f.ID)
	}
	if len(f.CurrentOwners) == 0 || !validOwnerClaims(f.CurrentOwners) || strings.TrimSpace(f.SharedOutcome) == "" || strings.TrimSpace(f.Risk) == "" {
		return fmt.Errorf("finding %q has incomplete ownership or outcome", f.ID)
	}
	if len(f.MaterialDifferences) == 0 || !uniqueNonemptyStrings(f.MaterialDifferences) || len(f.Recommendation) == 0 || !uniqueNonemptyStrings(f.Recommendation) || strings.TrimSpace(f.Decision) == "" {
		return fmt.Errorf("finding %q requires material differences, recommendations, and a maintainer decision", f.ID)
	}
	if f.Strength == "strong" && f.Confidence != "confirmed" {
		return fmt.Errorf("finding %q cannot be strong without confirmed evidence", f.ID)
	}
	if f.Verdict == "merge-now" && f.Relationship != "same-observable" {
		return fmt.Errorf("merge-now finding %q must describe the same observable", f.ID)
	}
	if f.Verdict == "resolve-product-divergence" && f.Relationship != "contradictory-observables" {
		return fmt.Errorf("resolve-product-divergence finding %q must describe contradictory observables", f.ID)
	}
	if err := validateEvidence(f.Evidence); err != nil {
		return fmt.Errorf("finding %q: %w", f.ID, err)
	}
	if !sameStringSet(ownerPaths(f.CurrentOwners), evidencePaths(f.Evidence)) {
		return fmt.Errorf("finding %q current owners must exactly match its source-evidence paths", f.ID)
	}
	positive := f.Verdict == "merge-now" || f.Verdict == "extract-neutral-contract"
	if !bddConsequences[f.BDD.Consequence] || !uniqueRelativeFeaturePaths(f.BDD.Features) {
		return fmt.Errorf("finding %q has invalid BDD impact", f.ID)
	}
	if len(f.BDD.Features) == 0 {
		if positive || f.BDD.Consequence != "none" {
			return fmt.Errorf("finding %q without BDD features must be non-positive with consequence none", f.ID)
		}
	}
	seenMembers := map[string]bool{}
	for _, entry := range f.Applicability {
		if strings.TrimSpace(entry.Member) == "" || !dispositions[entry.Disposition] || len(entry.Evidence) == 0 || seenMembers[entry.Member] {
			return fmt.Errorf("finding %q has invalid applicability", f.ID)
		}
		if err := validateEvidence(entry.Evidence); err != nil {
			return fmt.Errorf("finding %q applicability for %q: %w", f.ID, entry.Member, err)
		}
		if len(active) > 0 && !active[entry.Member] {
			return fmt.Errorf("finding %q names unknown active member %q", f.ID, entry.Member)
		}
		seenMembers[entry.Member] = true
	}
	if positive {
		if len(f.CurrentOwners) < 2 || distinctProductSpecPaths(f.Evidence) < 2 || f.ProposedOwner == nil || !isSpecPath(f.ProposedOwner.Path) || !ownerStates[f.ProposedOwner.State] || len(f.BDD.Features) == 0 {
			return fmt.Errorf("positive finding %q requires two SPEC owners, proposed owner state, and BDD features", f.ID)
		}
		if strings.TrimSpace(f.OwnershipCompleteness) == "" || strings.TrimSpace(f.ProposedOwner.Rationale) == "" {
			return fmt.Errorf("positive finding %q requires owner-completeness and proposed-owner rationale", f.ID)
		}
		if f.ProposedOwner.State == "existing" {
			if !containsString(ownerPaths(f.CurrentOwners), f.ProposedOwner.Path) {
				return fmt.Errorf("positive finding %q must select an existing proposed owner from its authenticated current-owner set", f.ID)
			}
		} else if containsString(ownerPaths(f.CurrentOwners), f.ProposedOwner.Path) {
			return fmt.Errorf("positive finding %q cannot mark a current owner as new", f.ID)
		}
		if f.Confidence != "confirmed" || f.Strength == "exploratory" {
			return fmt.Errorf("positive finding %q requires confirmed evidence and non-exploratory strength", f.ID)
		}
		if f.Classification == "fixture" || f.Relationship == "fixture-or-generated-copy" {
			return fmt.Errorf("positive finding %q cannot treat a fixture or generated copy as a normative owner", f.ID)
		}
		if !applicabilityBases[f.ApplicabilityBasis] {
			return fmt.Errorf("positive finding %q requires an applicability basis", f.ID)
		}
		if strings.TrimSpace(f.ApplicabilityRationale) == "" {
			return fmt.Errorf("positive finding %q requires applicability rationale", f.ID)
		}
		for _, entry := range f.Applicability {
			if entry.Disposition == "unknown" {
				return fmt.Errorf("positive finding %q has unresolved applicability for active member %q", f.ID, entry.Member)
			}
		}
		switch f.ApplicabilityBasis {
		case "active-members":
			if len(active) == 0 || len(seenMembers) != len(active) {
				return fmt.Errorf("positive finding %q must cover every pinned active member", f.ID)
			}
		case "implementation-only":
			if hasHarnessSurfaceOwner(ownerPaths(f.CurrentOwners)) {
				return fmt.Errorf("implementation-only finding %q includes a harness configuration owner", f.ID)
			}
			if len(seenMembers) != 0 {
				if len(active) == 0 || len(seenMembers) != len(active) {
					return fmt.Errorf("implementation-only finding %q must omit harness rows or mark every active member", f.ID)
				}
				for _, entry := range f.Applicability {
					if entry.Disposition != "not-applicable" {
						return fmt.Errorf("implementation-only finding %q has non-applicable harness disposition %q", f.ID, entry.Disposition)
					}
				}
			}
		}
	}
	if f.Verdict == "resolve-product-divergence" || f.Verdict == "insufficient-evidence" {
		if strings.TrimSpace(f.Decision) == "" || f.Strength == "strong" {
			return fmt.Errorf("finding %q requires a decision and non-strong strength", f.ID)
		}
	}
	if !positive && f.ProposedOwner != nil {
		return fmt.Errorf("non-positive finding %q cannot select a canonical owner", f.ID)
	}
	if nonCandidate {
		if f.Rank != 0 || f.Verdict != "keep-separate" || strings.TrimSpace(f.Boundary) == "" {
			return fmt.Errorf("non-candidate %q must be keep-separate with a boundary", f.ID)
		}
	} else if f.Verdict == "keep-separate" {
		return fmt.Errorf("keep-separate finding %q belongs in non_candidates", f.ID)
	}
	return nil
}

func hasHarnessSurfaceOwner(paths []string) bool {
	for _, path := range paths {
		for _, prefix := range []string{".agents/", ".claude/", ".codex/", ".gemini/", ".opencode/", ".pi/"} {
			if strings.HasPrefix(filepath.ToSlash(path), prefix) {
				return true
			}
		}
	}
	return false
}

func validateEvidence(items []evidence) error {
	if len(items) == 0 {
		return errors.New("evidence is required")
	}
	for _, item := range items {
		if !validPath(item.Path) || item.Line < 1 || strings.TrimSpace(item.Excerpt) == "" {
			return errors.New("evidence requires a relative path, positive line, and excerpt")
		}
	}
	return nil
}

func validOwnerClaims(claims []ownerClaim) bool {
	seen := map[string]bool{}
	for _, claim := range claims {
		if !isSpecPath(claim.Path) || strings.TrimSpace(claim.Rationale) == "" || seen[claim.Path] {
			return false
		}
		seen[claim.Path] = true
	}
	return true
}

func ownerPaths(claims []ownerClaim) []string {
	paths := make([]string, 0, len(claims))
	for _, claim := range claims {
		paths = append(paths, claim.Path)
	}
	return paths
}

func evidencePaths(items []evidence) []string {
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Path] = true
	}
	return mapKeys(seen)
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

var (
	verdicts           = map[string]bool{"merge-now": true, "extract-neutral-contract": true, "keep-separate": true, "resolve-product-divergence": true, "insufficient-evidence": true}
	relationships      = map[string]bool{"same-observable": true, "overlapping-observables": true, "contradictory-observables": true, "same-vocabulary-only": true, "fixture-or-generated-copy": true}
	classifications    = map[string]bool{"shared-contract": true, "capability-variation": true, "native-adapter": true, "wrapper": true, "fixture": true, "implementation-detail": true}
	confidences        = map[string]bool{"confirmed": true, "likely": true, "tentative": true}
	strengths          = map[string]bool{"strong": true, "moderate": true, "exploratory": true}
	ownerStates        = map[string]bool{"existing": true, "new": true}
	applicabilityBases = map[string]bool{"active-members": true, "implementation-only": true}
	dispositions       = map[string]bool{"supported": true, "adapted": true, "unsupported": true, "not-applicable": true, "unknown": true}
	bddConsequences    = map[string]bool{"merge": true, "add-matrix": true, "adapter-only": true, "none": true, "resolve": true}
	seedKinds          = map[string]bool{"exact-body": true, "duplicate-id": true, "shared-bdd": true, "identical-file": true}
	diagnosticKinds    = map[string]bool{"anonymous-requirement": true, "nonconforming-requirement": true, "missing-bdd-feature": true, "nonreciprocal-bdd-feature": true}
)

func validExclusions(items []exclusion) bool {
	seen := map[string]bool{}
	for _, item := range items {
		if !validPath(item.Path) || strings.TrimSpace(item.Reason) == "" || seen[item.Path] {
			return false
		}
		seen[item.Path] = true
	}
	return true
}

func validEnumSlice(items []string, allowed map[string]bool) bool {
	for _, item := range items {
		if !allowed[item] {
			return false
		}
	}
	return len(items) > 0
}

func validPath(path string) bool {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validFeaturePath(path string) bool {
	return validPath(path) && strings.HasSuffix(path, ".feature")
}

func uniqueRelativePaths(paths []string, allowDot bool) bool {
	seen := map[string]bool{}
	for _, path := range paths {
		if allowDot && path == "." {
			if seen[path] {
				return false
			}
			seen[path] = true
			continue
		}
		if !validPath(path) || seen[path] {
			return false
		}
		seen[path] = true
	}
	return true
}

func uniqueRelativeFeaturePaths(paths []string) bool {
	seen := map[string]bool{}
	for _, path := range paths {
		if !validFeaturePath(path) || seen[path] {
			return false
		}
		seen[path] = true
	}
	return true
}

func uniqueStrings(items []string) bool {
	seen := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item) == "" || seen[item] {
			return false
		}
		seen[item] = true
	}
	return true
}

func uniqueNonemptyStrings(items []string) bool {
	return uniqueStrings(items)
}

func uniqueSpecPaths(paths []string) bool {
	if !uniqueRelativePaths(paths, false) {
		return false
	}
	for _, path := range paths {
		if !isSpecPath(path) {
			return false
		}
	}
	return true
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	rightSet := stringSet(right)
	for _, item := range left {
		if !rightSet[item] {
			return false
		}
	}
	return true
}

func containsString(items []string, want string) bool {
	return slices.Contains(items, want)
}

func stringSet(items []string) map[string]bool {
	result := map[string]bool{}
	for _, item := range items {
		result[item] = true
	}
	return result
}

func verdictHistogram(report report) map[string]int {
	histogram := map[string]int{}
	for _, finding := range append(append([]finding{}, report.Candidates...), report.NonCandidates...) {
		histogram[finding.Verdict]++
	}
	return histogram
}

func equalHistogram(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func distinctPaths(items []evidence) int {
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Path] = true
	}
	return len(seen)
}

func distinctProductSpecPaths(items []evidence) int {
	seen := map[string]bool{}
	for _, item := range items {
		if isProductSpecPath(item.Path) {
			seen[item.Path] = true
		}
	}
	return len(seen)
}

func isProductSpecPath(path string) bool {
	if !isSpecPath(path) {
		return false
	}
	for segment := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		switch segment {
		case "testdata", "migrations", "vendor", "node_modules":
			return false
		}
	}
	return true
}

func isSpecPath(path string) bool {
	return validPath(path) && filepath.Base(filepath.ToSlash(path)) == "SPEC.md"
}
func sortEvidence(items []evidence) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].Line < items[j].Line
		}
		return items[i].Path < items[j].Path
	})
}
func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func normalize(text string) string { return strings.Join(strings.Fields(text), " ") }

//nolint:gocyclo // Linear self-contained report assembly keeps escaping and section order directly auditable.
func renderHTML(audit report, inventory *report) string {
	candidates := append([]finding{}, audit.Candidates...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Rank < candidates[j].Rank })
	seeds := audit.Seeds
	if inventory != nil {
		seeds = inventory.Seeds
	}

	var out strings.Builder
	out.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	fmt.Fprintf(&out, "<title>SPEC governance audit · %s</title>", esc(audit.Snapshot.Repository))
	out.WriteString(`<style>
:root{--ink:#132238;--muted:#56657a;--paper:#f4f1ea;--card:#fffdf8;--line:#d9d2c5;--accent:#b5412b;--teal:#176b67;--gold:#a66b17;--soft:#ece7dc;--code:#f1eee6}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;color:var(--ink);background:var(--paper);font:16px/1.55 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}a{color:var(--teal)}code{font:0.9em/1.45 ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--code);padding:.08rem .3rem;border-radius:4px;overflow-wrap:anywhere}.shell{width:min(1180px,calc(100% - 2rem));margin:auto}.hero{padding:4.5rem 0 2.3rem;border-bottom:1px solid var(--line);background:linear-gradient(135deg,#fffaf0 0%,#e5efec 100%)}.eyebrow{margin:0 0 .5rem;color:var(--accent);font-weight:800;letter-spacing:.14em;text-transform:uppercase;font-size:.76rem}.hero h1{max-width:850px;margin:.1rem 0 .8rem;font:700 clamp(2.25rem,6vw,4.6rem)/.98 ui-serif,Georgia,serif;letter-spacing:-.04em}.lede{max-width:780px;color:var(--muted);font-size:1.08rem}.snapshot{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:.75rem;margin:1.5rem 0}.snapshot div{padding:.8rem 0;border-top:1px solid var(--line)}.snapshot dt{color:var(--muted);font-size:.75rem;text-transform:uppercase;letter-spacing:.08em}.snapshot dd{margin:.2rem 0 0}.warning{margin:1.2rem 0 0;padding:1rem 1.1rem;border:1px solid #e3b6a5;border-left:5px solid var(--accent);background:#fff5ef}.toc{position:sticky;top:0;z-index:5;background:rgba(244,241,234,.96);border-bottom:1px solid var(--line);backdrop-filter:blur(8px)}.toc .shell{display:flex;gap:1.1rem;overflow-x:auto;padding:.72rem 0}.toc a{white-space:nowrap;text-decoration:none;font-weight:700;font-size:.86rem}main{padding:2.2rem 0 5rem}section{scroll-margin-top:4rem;margin:0 0 3.5rem}h2{font:700 clamp(1.6rem,3vw,2.35rem)/1.05 ui-serif,Georgia,serif;letter-spacing:-.02em;margin:0 0 1rem}h3{font-size:1.25rem;line-height:1.25;margin:.15rem 0}.section-intro{max-width:780px;color:var(--muted)}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:.8rem}.metric{padding:1.05rem;border:1px solid var(--line);background:var(--card);box-shadow:0 3px 0 rgba(19,34,56,.04)}.metric strong{display:block;font:700 2rem/1 ui-serif,Georgia,serif}.metric span{color:var(--muted);font-size:.82rem}.scope-grid,.two-col{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.panel{padding:1.15rem;border:1px solid var(--line);background:var(--card)}.pill,.tag{display:inline-flex;align-items:center;margin:.18rem .25rem .18rem 0;padding:.23rem .55rem;border-radius:999px;background:var(--soft);font-size:.78rem;font-weight:800}.tag.verdict{background:#d9ebe8;color:#0d5753}.tag.warn{background:#f6dfd5;color:#7a2716}.top-pick{padding:1.35rem;border:1px solid #b9cfc9;border-left:6px solid var(--teal);background:#f7fcfa}.top-pick .rank{color:var(--teal);font-weight:900}.topology{display:grid;grid-template-columns:minmax(0,1fr) auto minmax(0,1fr);align-items:center;gap:.8rem;margin:1rem 0}.owner-box{height:100%;padding:.8rem;border:1px solid var(--line);background:var(--card)}.arrow{font-size:1.6rem;color:var(--teal)}.toolbar{display:grid;grid-template-columns:2fr 1fr;gap:.7rem;margin:1rem 0}.toolbar input,.toolbar select{width:100%;padding:.75rem;border:1px solid var(--line);border-radius:0;background:#fff;font:inherit}.finding{margin:1rem 0;padding:1.3rem;border:1px solid var(--line);background:var(--card);box-shadow:0 7px 18px rgba(19,34,56,.05)}.finding-head{display:flex;justify-content:space-between;gap:1rem;align-items:flex-start}.finding-id{color:var(--muted);font:700 .78rem/1.2 ui-monospace,SFMono-Regular,Menlo,monospace}.outcome{font-size:1.04rem}.label{display:block;margin:1rem 0 .35rem;color:var(--muted);font-size:.73rem;font-weight:900;letter-spacing:.09em;text-transform:uppercase}.compact{margin:.35rem 0;padding-left:1.2rem}.compact li{margin:.25rem 0}.matrix{width:100%;border-collapse:collapse;font-size:.88rem}.matrix th,.matrix td{text-align:left;vertical-align:top;padding:.45rem;border-bottom:1px solid var(--line)}.matrix th{color:var(--muted)}.evidence{list-style:none;padding:0}.evidence li{margin:.45rem 0;padding:.7rem;border-left:3px solid #aebdb8;background:#f4f7f4}.decision{padding:.85rem;border-left:4px solid var(--gold);background:#fff8e8}.risk{padding:.8rem;background:#fff3ee}.empty{color:var(--muted);font-style:italic}.seed-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:.7rem}.seed{padding:.85rem;border:1px solid var(--line);background:var(--card)}details{margin-top:.8rem}summary{cursor:pointer;font-weight:800}.source-note{padding:1rem;border:1px dashed var(--teal);background:#f4fbf9}.footer{padding:1.2rem 0 3rem;color:var(--muted);border-top:1px solid var(--line)}[hidden]{display:none!important}
html,body,.toc{overflow-x:hidden}
@media(max-width:720px){.hero{padding-top:3rem}.scope-grid,.two-col,.topology,.toolbar{grid-template-columns:1fr}.arrow{transform:rotate(90deg);justify-self:center}.finding-head{display:block}.shell{width:min(100% - 1rem,1180px)}.matrix{display:block;overflow-x:auto}}
@media print{body{background:#fff}.toc,.toolbar{display:none}.shell{width:100%}.hero{padding:1rem 0;background:#fff}.finding,.panel,.metric{box-shadow:none;break-inside:avoid}details>summary{display:none}details>*{display:block!important}}
</style></head><body>`)
	out.WriteString("<header class=\"hero\"><div class=\"shell\"><p class=\"eyebrow\">Read-only decision evidence</p><h1>SPEC governance audit</h1><p class=\"lede\">Where observable contracts are repeated across harnesses or implementations, where a neutral owner should replace copies, and which similar-looking specifications must stay separate.</p><dl class=\"snapshot\">")
	renderSnapshotItem(&out, "Repository", audit.Snapshot.Repository, false)
	renderSnapshotItem(&out, "Pinned revision", audit.Snapshot.Revision, true)
	renderSnapshotItem(&out, "Revision committed", audit.Snapshot.RevisionCommittedAt, false)
	if audit.Snapshot.GeneratedAt != "" {
		renderSnapshotItem(&out, "Report generated", audit.Snapshot.GeneratedAt, false)
	}
	if audit.Snapshot.ComparisonRevision != "" {
		renderSnapshotItem(&out, "Comparison only", audit.Snapshot.ComparisonRevision, true)
	}
	out.WriteString("</dl><p class=\"warning\"><strong>Selection precedes change.</strong> This report records evidence and maintainer decisions; it does not authorize automatic consolidation, deletion, or product mutation.</p></div></header>")
	out.WriteString("<nav class=\"toc\" aria-label=\"Report sections\"><div class=\"shell\"><a href=\"#summary\">Corpus</a><a href=\"#top\">Top decision</a><a href=\"#candidates\">Candidates</a><a href=\"#boundaries\">Keep separate</a><a href=\"#method\">Method</a><a href=\"#limits\">Limits</a></div></nav><main class=\"shell\">")

	out.WriteString("<section id=\"summary\"><h2>Corpus and scope</h2><div class=\"metrics\">")
	renderMetric(&out, audit.Summary.SpecFiles, "tracked SPEC files")
	renderMetric(&out, audit.Summary.Requirements, "identified EARS requirements")
	renderMetric(&out, audit.Summary.Diagnostics, "collector diagnostics")
	renderMetric(&out, audit.Summary.CandidateCount, "consolidation candidates")
	for _, verdict := range sortedKeys(audit.Summary.ByVerdict) {
		renderMetric(&out, audit.Summary.ByVerdict[verdict], verdict)
	}
	out.WriteString("</div><div class=\"scope-grid\" style=\"margin-top:1rem\"><div class=\"panel\"><span class=\"label\">Roots</span>")
	for _, root := range audit.Scope.Roots {
		fmt.Fprintf(&out, "<code>%s</code> ", esc(root))
	}
	out.WriteString("<span class=\"label\">Pinned active members</span>")
	for _, member := range audit.Scope.ActiveMembers {
		fmt.Fprintf(&out, "<span class=\"pill\">%s</span>", esc(member))
	}
	out.WriteString("</div><div class=\"panel\"><span class=\"label\">Exclusions</span>")
	if len(audit.Scope.Excluded) == 0 {
		out.WriteString("<p class=\"empty\">No paths were excluded.</p>")
	} else {
		out.WriteString("<ul class=\"compact\">")
		for _, exclusion := range audit.Scope.Excluded {
			fmt.Fprintf(&out, "<li><code>%s</code> — %s</li>", esc(exclusion.Path), esc(exclusion.Reason))
		}
		out.WriteString("</ul>")
	}
	out.WriteString("</div></div></section>")

	out.WriteString("<section id=\"top\"><h2>Top maintainer decision</h2>")
	if len(candidates) == 0 {
		out.WriteString("<p class=\"empty\">No supported consolidation candidate was selected.</p>")
	} else {
		top := candidates[0]
		fmt.Fprintf(&out, "<div class=\"top-pick\"><span class=\"rank\">Rank %d · %s</span><h3><a href=\"#%s\">%s</a></h3><p>%s</p>", top.Rank, esc(top.ID), esc(top.ID), esc(top.Title), esc(top.SharedOutcome))
		renderTopology(&out, top)
		fmt.Fprintf(&out, "<p class=\"decision\"><strong>Decision requested:</strong> %s</p></div>", esc(top.Decision))
	}
	out.WriteString("</section>")

	out.WriteString("<section id=\"candidates\"><h2>Ranked consolidation candidates</h2><p class=\"section-intro\">Lexical seeds are not verdicts. Every card below preserves ownership, applicability, exact source evidence, BDD impact, risks, and the maintainer choice.</p><div class=\"toolbar\"><input id=\"filter\" type=\"search\" aria-label=\"Filter findings\" placeholder=\"Filter by path, requirement, owner, or outcome\"><select id=\"verdict\" aria-label=\"Filter by verdict\"><option value=\"\">All verdicts</option>")
	for _, verdict := range sortedKeys(audit.Summary.ByVerdict) {
		fmt.Fprintf(&out, "<option value=\"%s\">%s</option>", esc(verdict), esc(verdict))
	}
	out.WriteString("</select></div><div id=\"findings\">")
	for _, finding := range candidates {
		renderFinding(&out, finding)
	}
	out.WriteString("</div></section>")

	out.WriteString("<section id=\"boundaries\"><h2>Keep-separate controls</h2><p class=\"section-intro\">These near misses constrain the audit: shared vocabulary, identifiers, or lifecycle shape do not erase adapter, platform, or domain boundaries.</p>")
	if len(audit.NonCandidates) == 0 {
		out.WriteString("<p class=\"empty\">None recorded.</p>")
	}
	for _, finding := range audit.NonCandidates {
		renderFinding(&out, finding)
	}
	out.WriteString("</section>")

	out.WriteString("<section id=\"method\"><h2>Method and reproducibility</h2><div class=\"two-col\"><div class=\"panel\"><span class=\"label\">Collector</span><p><code>" + esc(audit.Methodology.Collector) + "</code></p><span class=\"label\">Semantic review</span><p>" + esc(audit.Methodology.SemanticReview) + "</p><span class=\"label\">Seed kinds</span>")
	for _, kind := range audit.Methodology.SeedKinds {
		fmt.Fprintf(&out, "<span class=\"pill\">%s</span>", esc(kind))
	}
	out.WriteString("</div><div class=\"panel\"><span class=\"label\">Reproduce</span><ol class=\"compact\">")
	for _, command := range audit.Methodology.Reproduce {
		fmt.Fprintf(&out, "<li><code>%s</code></li>", esc(command))
	}
	out.WriteString("</ol></div></div>")
	renderSeedSummary(&out, seeds, inventory)
	proof := "Schema validation was performed without a repository-authenticated inventory."
	if inventory != nil {
		proof = "The supplied inventory was recomputed from Git objects at the pinned revision before evidence and HTML rendering were accepted."
	}
	fmt.Fprintf(&out, "<p class=\"source-note\"><strong>Source disclosure.</strong> %s Structured JSON remains the findings source; this HTML is a complete view, not a second classification store.</p></section>", esc(proof))

	out.WriteString("<section id=\"limits\"><h2>Limitations</h2>")
	if len(audit.Limitations) == 0 {
		out.WriteString("<p class=\"empty\">No limitations recorded.</p>")
	} else {
		renderStringList(&out, audit.Limitations, false)
	}
	out.WriteString("</section></main><footer class=\"footer\"><div class=\"shell\">Self-contained offline report · no external scripts, fonts, styles, or data sources.</div></footer>")
	out.WriteString(`<script>
const query=document.getElementById('filter');const verdict=document.getElementById('verdict');
function applyFilters(){const q=query.value.trim().toLowerCase();const v=verdict.value;for(const card of document.querySelectorAll('#findings .finding')){card.hidden=(q&&!card.innerText.toLowerCase().includes(q))||(v&&card.dataset.verdict!==v)}}
query.addEventListener('input',applyFilters);verdict.addEventListener('change',applyFilters);
</script></body></html>
`)
	return out.String()
}

func renderSnapshotItem(out *strings.Builder, label, value string, code bool) {
	fmt.Fprintf(out, "<div><dt>%s</dt><dd>", esc(label))
	if code {
		fmt.Fprintf(out, "<code>%s</code>", esc(value))
	} else {
		out.WriteString(esc(value))
	}
	out.WriteString("</dd></div>")
}

func renderMetric(out *strings.Builder, value int, label string) {
	fmt.Fprintf(out, "<div class=\"metric\"><strong>%d</strong><span>%s</span></div>", value, esc(label))
}

func renderTopology(out *strings.Builder, finding finding) {
	out.WriteString("<div class=\"topology\"><div class=\"owner-box\"><span class=\"label\">Current owners</span><ul class=\"compact\">")
	for _, owner := range finding.CurrentOwners {
		fmt.Fprintf(out, "<li><code>%s</code><br>%s</li>", esc(owner.Path), esc(owner.Rationale))
	}
	out.WriteString("</ul></div><div class=\"arrow\" aria-hidden=\"true\">→</div><div class=\"owner-box\"><span class=\"label\">Proposed canonical owner</span>")
	if finding.ProposedOwner == nil {
		out.WriteString("<p class=\"empty\">No consolidation owner.</p>")
	} else {
		fmt.Fprintf(out, "<p><code>%s</code></p><span class=\"pill\">%s owner</span><p>%s</p>", esc(finding.ProposedOwner.Path), esc(finding.ProposedOwner.State), esc(finding.ProposedOwner.Rationale))
	}
	out.WriteString("</div></div>")
}

func renderFinding(out *strings.Builder, finding finding) {
	fmt.Fprintf(out, "<article class=\"finding\" id=\"%s\" data-verdict=\"%s\"><header class=\"finding-head\"><div><div class=\"finding-id\">%s", esc(finding.ID), esc(finding.Verdict), esc(finding.ID))
	if finding.Rank > 0 {
		fmt.Fprintf(out, " · rank %d", finding.Rank)
	}
	fmt.Fprintf(out, "</div><h3>%s</h3></div><div><span class=\"tag verdict\">%s</span><span class=\"tag\">%s</span><span class=\"tag\">%s</span></div></header>", esc(finding.Title), esc(finding.Verdict), esc(finding.Confidence), esc(finding.Strength))
	fmt.Fprintf(out, "<p><span class=\"tag\">%s</span><span class=\"tag\">%s</span></p><span class=\"label\">Shared outcome or apparent overlap</span><p class=\"outcome\">%s</p>", esc(finding.Relationship), esc(finding.Classification), esc(finding.SharedOutcome))
	renderTopology(out, finding)
	if finding.OwnershipCompleteness != "" {
		fmt.Fprintf(out, "<p class=\"source-note\"><strong>Owner-set completeness:</strong> %s</p>", esc(finding.OwnershipCompleteness))
	}
	if finding.Boundary != "" {
		fmt.Fprintf(out, "<p class=\"warning\"><strong>Keep-separate boundary:</strong> %s</p>", esc(finding.Boundary))
	}
	out.WriteString("<div class=\"two-col\"><div><span class=\"label\">Material differences</span>")
	renderStringList(out, finding.MaterialDifferences, false)
	out.WriteString("</div><div><span class=\"label\">Applicability</span>")
	if finding.ApplicabilityBasis != "" {
		fmt.Fprintf(out, "<p><span class=\"pill\">%s</span> %s</p>", esc(finding.ApplicabilityBasis), esc(finding.ApplicabilityRationale))
	}
	if len(finding.Applicability) == 0 {
		out.WriteString("<p class=\"empty\">No active-member parity claim.</p>")
	} else {
		out.WriteString("<table class=\"matrix\"><thead><tr><th>Member</th><th>Disposition</th><th>Evidence</th></tr></thead><tbody>")
		for _, entry := range finding.Applicability {
			fmt.Fprintf(out, "<tr><td><code>%s</code></td><td>%s</td><td>", esc(entry.Member), esc(entry.Disposition))
			for index, item := range entry.Evidence {
				if index > 0 {
					out.WriteString("<br>")
				}
				fmt.Fprintf(out, "<code>%s:%d %s</code>", esc(item.Path), item.Line, esc(item.RequirementID))
			}
			out.WriteString("</td></tr>")
		}
		out.WriteString("</tbody></table>")
	}
	out.WriteString("</div></div><details open><summary>Exact source evidence</summary><ul class=\"evidence\">")
	for _, item := range finding.Evidence {
		fmt.Fprintf(out, "<li><code>%s:%d</code>", esc(item.Path), item.Line)
		if item.RequirementID != "" {
			fmt.Fprintf(out, " <span class=\"pill\">%s</span>", esc(item.RequirementID))
		}
		fmt.Fprintf(out, "<br>%s</li>", esc(item.Excerpt))
	}
	out.WriteString("</ul></details><div class=\"two-col\"><div><span class=\"label\">BDD consequence</span>")
	fmt.Fprintf(out, "<p><span class=\"tag\">%s</span></p>", esc(finding.BDD.Consequence))
	if len(finding.BDD.Features) == 0 {
		out.WriteString("<p class=\"empty\">No feature selected.</p>")
	} else {
		out.WriteString("<ul class=\"compact\">")
		for _, feature := range finding.BDD.Features {
			fmt.Fprintf(out, "<li><code>%s</code></li>", esc(feature))
		}
		out.WriteString("</ul>")
	}
	out.WriteString("</div><div><span class=\"label\">Ordered recommendation</span>")
	renderStringList(out, finding.Recommendation, true)
	out.WriteString("</div></div>")
	fmt.Fprintf(out, "<p class=\"risk\"><strong>Risk:</strong> %s</p><p class=\"decision\"><strong>Maintainer decision:</strong> %s</p>", esc(finding.Risk), esc(finding.Decision))
	out.WriteString("<details><summary>Finding limitations</summary>")
	if len(finding.Limitations) == 0 {
		out.WriteString("<p class=\"empty\">None recorded.</p>")
	} else {
		renderStringList(out, finding.Limitations, false)
	}
	out.WriteString("</details></article>")
}

func renderStringList(out *strings.Builder, items []string, ordered bool) {
	tag := "ul"
	if ordered {
		tag = "ol"
	}
	fmt.Fprintf(out, "<%s class=\"compact\">", tag)
	for _, item := range items {
		fmt.Fprintf(out, "<li>%s</li>", esc(item))
	}
	fmt.Fprintf(out, "</%s>", tag)
}

func renderSeedSummary(out *strings.Builder, seeds []seed, inventory *report) {
	counts := map[string]int{}
	for _, seed := range seeds {
		counts[seed.Kind]++
	}
	out.WriteString("<h3 style=\"margin-top:1.5rem\">Deterministic seed and diagnostic summary</h3><div class=\"seed-grid\">")
	for _, kind := range sortedKeys(counts) {
		fmt.Fprintf(out, "<div class=\"seed\"><strong>%d</strong><br>%s seeds</div>", counts[kind], esc(kind))
	}
	if inventory != nil {
		diagnostics := map[string]int{}
		for _, file := range inventory.Inventory {
			for _, diagnostic := range file.Diagnostics {
				diagnostics[diagnostic.Kind]++
			}
		}
		for _, kind := range sortedKeys(diagnostics) {
			fmt.Fprintf(out, "<div class=\"seed\"><strong>%d</strong><br>%s diagnostics</div>", diagnostics[kind], esc(kind))
		}
	}
	if len(counts) == 0 && (inventory == nil || inventory.Summary.Diagnostics == 0) {
		out.WriteString("<div class=\"seed empty\">No deterministic seeds or diagnostics were supplied.</div>")
	}
	out.WriteString("</div><p class=\"section-intro\">Seeds identify review leads only. The authenticated inventory JSON retains every seed and diagnostic record; the semantic ledger records the bounded verdicts.</p>")
}

func esc(text string) string { return html.EscapeString(text) }
