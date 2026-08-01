// Command specaudit inventories and renders evidence for a read-only SPEC audit.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/vbonnet/dear-agent/internal/earslint"
)

const schemaVersion = "spec-audit/v2"

const gitEvidenceTrustDisclosure = "The collector trusts the PATH-selected Git executable, repository Git metadata, the common object store, and configured object alternates. It disables replacement objects and lazy fetching and resolves evidence from the pinned commit through Git; it does not independently authenticate source provenance or object-store integrity."

const (
	canonicalActiveHarnessRegistryPath = "agm/internal/harnessregistry/registry.go"
	legacyActiveHarnessRegistryPath    = "agm/internal/agent/harnesses.go"
	maxGitOutputBytes                  = 16 * 1024 * 1024
	maxGitInputBytes                   = 1 * 1024 * 1024
	maxReportInputBytes                = 32 * 1024 * 1024
	maxCorpusBytes                     = 64 * 1024 * 1024
	maxArtifactOutputBytes             = 64 * 1024 * 1024
	maxGitCommandDuration              = 30 * time.Second
	maxGitWaitDelay                    = 250 * time.Millisecond
	maxInventoryDuration               = 60 * time.Second
	maxInventoryFiles                  = 10_000
	maxJSONDepth                       = 128
	maxBatchHeaderBytes                = 128
	maxGitExecutableIdentityBytes      = 128 * 1024 * 1024
	maxAlternateConfigBytes            = 1024 * 1024
	maxAlternateObjectRoutes           = 1024
)

type corpusBudget struct {
	byteLimit int64
	fileLimit int
	usedBytes int64
	usedFiles int
}

func (budget *corpusBudget) consumeFile(label string, size int64) error {
	if budget.byteLimit <= 0 {
		return errors.New("SPEC audit corpus limit must be positive")
	}
	if budget.fileLimit <= 0 {
		return errors.New("SPEC audit corpus file limit must be positive")
	}
	if budget.usedFiles >= budget.fileLimit {
		return fmt.Errorf("SPEC audit corpus exceeds %d files while selecting %q", budget.fileLimit, label)
	}
	if size < 0 || size > budget.byteLimit-budget.usedBytes {
		return fmt.Errorf("SPEC audit corpus exceeds %d bytes while reading %q", budget.byteLimit, label)
	}
	budget.usedFiles++
	budget.usedBytes += size
	return nil
}

type inventoryLimits struct {
	corpusBytes int64
	corpusFiles int
	wallTime    time.Duration
}

type pinnedBlob struct {
	path string
	oid  string
	size int64
}

type gitExecutable struct {
	path string
}

func (executable gitExecutable) Path() string { return executable.path }

var errGitOutputLimit = errors.New("git command output exceeded its safety limit")

var (
	requirementPattern       = regexp.MustCompile(`^\s*([A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*-[A-Z0-9]*\d+)\s+(.+?)\s*$`)
	bddFeatureEntryPattern   = regexp.MustCompile("^- (?:(?:([^`:]+): )?)`([^`]+\\.feature)`(?:\\s+([^`]*))?$")
	markdownHeadingPattern   = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
	shaPattern               = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern            = regexp.MustCompile(`^[0-9a-f]{64}$`)
	identityPattern          = regexp.MustCompile(`^(?:sha256|path-sha256):[0-9a-f]{64}$`)
	canonicalEARSLinter      = mustEARSLinter()
	specTraceabilityHeadings = map[string]bool{
		"bdd traceability":          true,
		"package test traceability": true,
		"test traceability":         true,
		"traceability":              true,
	}
	specTraceabilityLabels = map[string]bool{
		"":                        true,
		"bdd":                     true,
		"bdd feature":             true,
		"bdd tests":               true,
		"command bdd":             true,
		"cross-surface bdd":       true,
		"cross-surface contracts": true,
		"feature":                 true,
		"related feature":         true,
		"status bdd":              true,
		"strict-spec linkage":     true,
		"strictness bdd":          true,
	}
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

	// inventoryPayloadPresent preserves whether a decoded semantic document
	// embedded an inventory field as null. Non-nil fields are caught directly.
	inventoryPayloadPresent bool
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
	Collector        string         `json:"collector"`
	SeedKinds        []string       `json:"seed_kinds"`
	SemanticReview   string         `json:"semantic_review"`
	GitEvidenceTrust string         `json:"git_evidence_trust"`
	GitTrustInputs   gitTrustInputs `json:"git_trust_inputs"`
	Reproduce        []string       `json:"reproduce"`
}

// gitTrustInputs records privacy-preserving identities for every Git input
// that can route pinned-object resolution. They are recomputed before a
// supplied inventory is accepted, rather than treated as source attestation.
type gitTrustInputs struct {
	Executable          string   `json:"executable"`
	WorkTreeRoot        string   `json:"worktree_root"`
	GitDir              string   `json:"git_dir"`
	CommonDir           string   `json:"common_dir"`
	ObjectDir           string   `json:"object_dir"`
	AlternateObjectDirs []string `json:"alternate_object_dirs"`
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
	Path         string       `json:"path"`
	SHA256       string       `json:"sha256"`
	RelatedSpecs []string     `json:"related_specs"`
	Diagnostics  []diagnostic `json:"diagnostics,omitempty"`
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
	Path                string `json:"path"`
	State               string `json:"state"`
	Rationale           string `json:"rationale"`
	NeutralityRationale string `json:"neutrality_rationale"`
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
	OwnershipPlan          *ownershipPlan      `json:"ownership_plan,omitempty"`
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
	Features              []string `json:"features"`
	SharedContractFeature string   `json:"shared_contract_feature,omitempty"`
	Consequence           string   `json:"consequence"`
}

// ownershipPlan records a migration proposal for maintainer review. It is
// deliberately not an authorization to edit or remove a current owner.
type ownershipPlan struct {
	Approval      string               `json:"approval"`
	CurrentOwners []ownershipPlanOwner `json:"current_owners"`
}

type ownershipPlanOwner struct {
	Path         string            `json:"path"`
	Action       string            `json:"action"`
	Rationale    string            `json:"rationale"`
	Preservation *preservationPlan `json:"preservation,omitempty"`
}

type preservationPlan struct {
	Requirements       []requirementPreservation `json:"requirements"`
	BDD                []bddPreservation         `json:"bdd"`
	ApplicabilityBasis string                    `json:"applicability_basis"`
	Applicability      []applicability           `json:"applicability"`
}

type requirementPreservation struct {
	Source      evidence `json:"source"`
	TargetID    string   `json:"target_id"`
	TargetState string   `json:"target_state"`
	Strategy    string   `json:"strategy"`
}

type bddPreservation struct {
	Feature     string `json:"feature"`
	SourceOwner string `json:"source_owner"`
	TargetOwner string `json:"target_owner"`
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
	fmt.Fprintln(out, "  go run ./tools/specaudit inventory -repo <path> -repository <owner/name> -revision <commit>")
	fmt.Fprintln(out, "  go run ./tools/specaudit validate -input <report.json> -inventory <inventory.json> -repo <path>")
	fmt.Fprintln(out, "  go run ./tools/specaudit render -input <report.json> -inventory <inventory.json> -repo <path>")
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
	data, err := marshalReportWithLimit(report, maxArtifactOutputBytes)
	if err != nil {
		fmt.Fprintf(stderr, "specaudit inventory: encode: %v\n", err)
		return 2
	}
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
	repoPath := fs.String("repo", "", "repository path used to recompute and compare the supplied pinned inventory")
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
	if err := validateSemanticReport(auditReport); err != nil {
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
	fmt.Fprintln(stdout, "specaudit: valid spec-audit/v2 report")
	return 0
}

func runRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "report JSON")
	inventoryPath := fs.String("inventory", "", "pinned inventory JSON used to verify source evidence and summarize seeds")
	repoPath := fs.String("repo", "", "repository path used to recompute and compare the supplied pinned inventory")
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
	if err := validateSemanticReport(auditReport); err != nil {
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
	htmlOutput, err := renderHTMLWithLimit(auditReport, inventoryReport, maxArtifactOutputBytes)
	if err != nil {
		fmt.Fprintf(stderr, "specaudit render: %v\n", err)
		return 2
	}
	if _, err := fmt.Fprint(stdout, htmlOutput); err != nil {
		fmt.Fprintf(stderr, "specaudit render: %v\n", err)
		return 2
	}
	return 0
}

func inventory(repoPath, repository, revision string) (report, error) {
	return inventoryWithLimits(repoPath, repository, revision, inventoryLimits{
		corpusBytes: maxCorpusBytes,
		corpusFiles: maxInventoryFiles,
		wallTime:    maxInventoryDuration,
	})
}

func inventoryWithCorpusLimit(repoPath, repository, revision string, corpusLimit int64) (report, error) {
	return inventoryWithLimits(repoPath, repository, revision, inventoryLimits{
		corpusBytes: corpusLimit,
		corpusFiles: maxInventoryFiles,
		wallTime:    maxInventoryDuration,
	})
}

//nolint:gocyclo // Linear pinned-object collection keeps inventory and reciprocal-diagnostic construction auditable.
func inventoryWithLimits(repoPath, repository, revision string, limits inventoryLimits) (report, error) {
	if limits.corpusBytes <= 0 {
		return report{}, errors.New("SPEC audit corpus limit must be positive")
	}
	if limits.corpusFiles <= 0 {
		return report{}, errors.New("SPEC audit corpus file limit must be positive")
	}
	if limits.wallTime <= 0 {
		return report{}, errors.New("SPEC audit inventory wall-time limit must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), limits.wallTime)
	defer cancel()
	executable, err := trustedGitExecutable()
	if err != nil {
		return report{}, err
	}
	runGit := func(root string, args ...string) ([]byte, error) {
		commandCtx, commandCancel := context.WithTimeout(ctx, maxGitCommandDuration)
		defer commandCancel()
		output, commandErr := gitBytesWithContext(commandCtx, executable, root, maxGitOutputBytes, nil, args...)
		if errors.Is(commandErr, context.DeadlineExceeded) {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("SPEC audit inventory exceeded %s global wall-time limit", limits.wallTime)
			}
			return nil, fmt.Errorf("git %s exceeded %s wall-time limit", strings.Join(args, " "), maxGitCommandDuration)
		}
		return output, commandErr
	}
	rootOutput, err := runGit(repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return report{}, err
	}
	root := strings.TrimSpace(string(rootOutput))
	gitTrustInputs, err := collectGitTrustInputs(executable, root, runGit)
	if err != nil {
		return report{}, fmt.Errorf("resolve Git trust inputs: %w", err)
	}
	commitOutput, err := runGit(root, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		return report{}, fmt.Errorf("resolve pinned revision %q: %w", revision, err)
	}
	commit := strings.TrimSpace(string(commitOutput))
	treeOutput, err := runGit(root, "ls-tree", "-r", "-z", "--long", commit)
	if err != nil {
		return report{}, fmt.Errorf("list %s: %w", commit, err)
	}
	budget := corpusBudget{byteLimit: limits.corpusBytes, fileLimit: limits.corpusFiles}
	blobs, err := selectPinnedBlobs(treeOutput, &budget)
	if err != nil {
		return report{}, err
	}
	bodies, err := readPinnedBlobBodies(ctx, executable, root, blobs, limits.corpusBytes, limits.wallTime)
	if err != nil {
		return report{}, err
	}
	files := make([]specFile, 0)
	features := make([]featureFile, 0)
	featureSpecRefs := map[string][]bddRef{}
	for _, blob := range blobs {
		if err := inventoryContextError(ctx, limits.wallTime); err != nil {
			return report{}, err
		}
		body := string(bodies[blob.oid])
		path := blob.path
		switch {
		case filepath.Base(path) == "SPEC.md":
			files = append(files, parseSpec(path, body))
		case strings.HasSuffix(path, ".feature"):
			feature, refs := parseFeature(path, body)
			features = append(features, feature)
			featureSpecRefs[feature.Path] = refs
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
	specIndex := map[string]specFile{}
	for _, file := range files {
		specIndex[file.Path] = file
	}
	for featureIndex := range features {
		feature := &features[featureIndex]
		for _, ref := range featureSpecRefs[feature.Path] {
			spec, ok := specIndex[ref.Path]
			switch {
			case !ok:
				feature.Diagnostics = append(feature.Diagnostics, diagnostic{Line: ref.Line, Kind: "missing-feature-spec", Excerpt: ref.Path})
			case !containsBDDRef(spec.BDDFeatures, feature.Path):
				feature.Diagnostics = append(feature.Diagnostics, diagnostic{Line: ref.Line, Kind: "nonreciprocal-feature-spec", Excerpt: ref.Path})
			}
		}
	}

	commitTime, err := runGit(root, "show", "-s", "--format=%cI", commit)
	if err != nil {
		return report{}, fmt.Errorf("read commit timestamp: %w", err)
	}
	revisionCommittedAt := strings.TrimSpace(string(commitTime))
	active, activeLimitations := activeMembersFromPinnedBodies(blobs, bodies)
	requirementCount := 0
	diagnosticCount := 0
	for _, file := range files {
		requirementCount += len(file.Requirements)
		diagnosticCount += len(file.Diagnostics)
	}
	for _, feature := range features {
		diagnosticCount += len(feature.Diagnostics)
	}
	if err := inventoryContextError(ctx, limits.wallTime); err != nil {
		return report{}, err
	}
	return report{
		SchemaVersion: schemaVersion,
		Snapshot:      snapshot{Repository: strings.TrimSpace(repository), Revision: commit, RevisionCommittedAt: revisionCommittedAt},
		Scope:         scope{Roots: []string{"."}, Excluded: []exclusion{}, ActiveMembers: active},
		Summary:       summary{SpecFiles: len(files), Requirements: requirementCount, Diagnostics: diagnosticCount, CandidateCount: 0, ByVerdict: map[string]int{}},
		Methodology: methodology{
			Collector:        "go run ./tools/specaudit inventory",
			SeedKinds:        []string{"exact-body", "duplicate-id", "shared-bdd", "identical-file", "harness-terminology"},
			SemanticReview:   "Seeds are bounded lexical leads; harness terminology records at most one matching requirement per SPEC path, and source plus BDD review determines every finding verdict.",
			GitEvidenceTrust: gitEvidenceTrustDisclosure,
			GitTrustInputs:   gitTrustInputs,
			Reproduce:        []string{fmt.Sprintf("go run ./tools/specaudit inventory -repo . -repository %s -revision %s", strings.TrimSpace(repository), commit)},
		},
		Inventory:     files,
		Features:      features,
		Seeds:         collectSeeds(files, active),
		Candidates:    []finding{},
		NonCandidates: []finding{},
		Limitations:   activeLimitations,
	}, nil
}

func selectPinnedBlobs(treeOutput []byte, budget *corpusBudget) ([]pinnedBlob, error) {
	blobs := make([]pinnedBlob, 0)
	for rawEntry := range bytes.SplitSeq(treeOutput, []byte{0}) {
		if len(rawEntry) == 0 {
			continue
		}
		metadata, rawPath, found := bytes.Cut(rawEntry, []byte{'\t'})
		if !found {
			return nil, fmt.Errorf("parse pinned tree entry: missing path separator")
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 4 {
			return nil, fmt.Errorf("parse pinned tree entry %q: invalid metadata", rawPath)
		}
		path := string(rawPath)
		selected := filepath.Base(path) == "SPEC.md" ||
			strings.HasSuffix(path, ".feature") ||
			path == canonicalActiveHarnessRegistryPath ||
			path == legacyActiveHarnessRegistryPath
		if !selected {
			continue
		}
		size, err := pinnedGitBlobSize(fields, path)
		if err != nil {
			return nil, err
		}
		if err := budget.consumeFile(path, size); err != nil {
			return nil, err
		}
		blobs = append(blobs, pinnedBlob{path: filepath.ToSlash(path), oid: fields[2], size: size})
	}
	return blobs, nil
}

func pinnedGitBlobSize(fields []string, path string) (int64, error) {
	if fields[1] != "blob" || !shaPattern.MatchString(fields[2]) {
		return 0, fmt.Errorf("pinned inventory object %q is not a Git blob with a 40-hex object ID", path)
	}
	if fields[3] == "-" || fields[3] == "BAD" {
		return 0, fmt.Errorf("pinned inventory object %s for %q is a missing object; lazy fetching is disabled", fields[2], path)
	}
	size, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || size < 0 {
		return 0, fmt.Errorf("parse pinned inventory object size for %q", path)
	}
	return size, nil
}

func readPinnedBlobBodies(ctx context.Context, executable gitExecutable, root string, blobs []pinnedBlob, corpusLimit int64, inventoryWallTime time.Duration) (map[string][]byte, error) {
	if corpusLimit <= 0 {
		return nil, errors.New("pinned Git batch corpus limit must be positive")
	}
	unique := make([]pinnedBlob, 0, len(blobs))
	seen := map[string]bool{}
	var input bytes.Buffer
	var expectedBytes int64
	for _, blob := range blobs {
		if seen[blob.oid] {
			continue
		}
		seen[blob.oid] = true
		unique = append(unique, blob)
		input.WriteString(blob.oid)
		input.WriteByte('\n')
		if blob.size > corpusLimit-expectedBytes {
			return nil, fmt.Errorf("pinned Git batch exceeds %d-byte corpus limit", corpusLimit)
		}
		expectedBytes += blob.size
	}
	if len(unique) == 0 {
		return map[string][]byte{}, nil
	}
	if input.Len() > maxGitInputBytes {
		return nil, fmt.Errorf("pinned Git batch input exceeds %d bytes", maxGitInputBytes)
	}
	headerAllowance := int64(len(unique)) * maxBatchHeaderBytes
	if headerAllowance < 0 || expectedBytes > corpusLimit || headerAllowance > int64(^uint64(0)>>1)-expectedBytes {
		return nil, errors.New("pinned Git batch metadata exceeds supported output ceiling")
	}
	commandCtx, cancel := context.WithTimeout(ctx, maxGitCommandDuration)
	defer cancel()
	output, err := gitBytesWithContext(commandCtx, executable, root, expectedBytes+headerAllowance, input.Bytes(), "cat-file", "--batch")
	if errors.Is(err, context.DeadlineExceeded) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("SPEC audit inventory exceeded %s global wall-time limit", inventoryWallTime)
		}
		return nil, fmt.Errorf("git cat-file --batch exceeded %s wall-time limit", maxGitCommandDuration)
	}
	if err != nil {
		return nil, fmt.Errorf("read pinned object batch: %w", err)
	}
	return parsePinnedBlobBatch(output, unique)
}

func parsePinnedBlobBatch(output []byte, expected []pinnedBlob) (map[string][]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(output))
	bodies := make(map[string][]byte, len(expected))
	for _, blob := range expected {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("parse pinned object %s header: %w", blob.oid, err)
		}
		fields := strings.Fields(strings.TrimSuffix(header, "\n"))
		if len(fields) != 3 || fields[0] != blob.oid || fields[1] != "blob" {
			return nil, fmt.Errorf("pinned object %s returned unexpected Git batch metadata", blob.oid)
		}
		size, sizeErr := strconv.ParseInt(fields[2], 10, 64)
		if sizeErr != nil || size != blob.size || size < 0 || size > int64(int(^uint(0)>>1)) {
			return nil, fmt.Errorf("pinned object %s returned an unexpected size", blob.oid)
		}
		body := make([]byte, int(size))
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, fmt.Errorf("read pinned object %s body: %w", blob.oid, err)
		}
		terminator, err := reader.ReadByte()
		if err != nil || terminator != '\n' {
			return nil, fmt.Errorf("pinned object %s has an invalid batch terminator", blob.oid)
		}
		bodies[blob.oid] = body
	}
	if trailing, err := reader.ReadByte(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("pinned object batch contains trailing byte %q", trailing)
		}
		return nil, fmt.Errorf("read pinned object batch trailer: %w", err)
	}
	return bodies, nil
}

func pinnedBodyForPath(blobs []pinnedBlob, bodies map[string][]byte, path string) (string, bool) {
	for _, blob := range blobs {
		if blob.path == path {
			body, ok := bodies[blob.oid]
			return string(body), ok
		}
	}
	return "", false
}

func activeMembersFromPinnedBodies(blobs []pinnedBlob, bodies map[string][]byte) ([]string, []string) {
	if body, ok := pinnedBodyForPath(blobs, bodies, canonicalActiveHarnessRegistryPath); ok {
		return activeMembersFromBody(body, true)
	}
	if body, ok := pinnedBodyForPath(blobs, bodies, legacyActiveHarnessRegistryPath); ok {
		return activeMembersFromBody(body, true)
	}
	return activeMembersFromBody("", false)
}

func inventoryContextError(ctx context.Context, wallTime time.Duration) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("SPEC audit inventory exceeded %s global wall-time limit", wallTime)
}

func parseSpec(path, body string) specFile {
	digest := sha256.Sum256([]byte(body))
	file := specFile{Path: filepath.ToSlash(path), SHA256: fmt.Sprintf("%x", digest), Requirements: []requirement{}, BDDFeatures: []bddRef{}, Diagnostics: []diagnostic{}}
	inFence := false
	inBDDTraceability := false
	seenTraceabilitySections := map[string]bool{}
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
		if heading := markdownHeadingPattern.FindStringSubmatch(trimmed); len(heading) != 0 && len(heading[1]) <= 2 {
			inBDDTraceability = false
			traceabilityHeading := strings.ToLower(strings.TrimSpace(heading[2]))
			if heading[1] == "##" && specTraceabilityHeadings[traceabilityHeading] {
				inBDDTraceability = true
				if seenTraceabilitySections[traceabilityHeading] {
					file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "ambiguous-bdd-traceability-section", Excerpt: trimmed})
				}
				seenTraceabilitySections[traceabilityHeading] = true
			}
		}
		normalizedLine := earslint.NormalizeRequirementLine(line)
		match := requirementPattern.FindStringSubmatch(normalizedLine)
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
		collectSpecBDDTraceability(&file, trimmed, lineNumber, inBDDTraceability, seenFeatures)
	}
	return file
}

func collectSpecBDDTraceability(file *specFile, line string, lineNumber int, inTraceability bool, seen map[string]bool) {
	if !inTraceability {
		return
	}
	match := bddFeatureEntryPattern.FindStringSubmatch(line)
	if len(match) == 0 || !specTraceabilityLabels[strings.ToLower(strings.TrimSpace(match[1]))] {
		looksLikeEntry := strings.HasPrefix(line, "-") &&
			(strings.Contains(strings.ToLower(line), "feature:") || strings.Contains(line, ".feature"))
		if looksLikeEntry {
			file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "malformed-bdd-feature-reference", Excerpt: line})
		}
		return
	}
	feature := filepath.ToSlash(match[2])
	if feature != filepath.ToSlash(filepath.Clean(feature)) || !validFeaturePath(feature) {
		file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "malformed-bdd-feature-reference", Excerpt: line})
		return
	}
	if seen[feature] {
		file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "duplicate-bdd-feature-reference", Excerpt: line})
		return
	}
	file.BDDFeatures = append(file.BDDFeatures, bddRef{Path: feature, Line: lineNumber})
	seen[feature] = true
}

func lintRequirementLine(line string) (int, int) {
	result, err := canonicalEARSLinter.Lint("requirement", strings.NewReader(line+"\n"))
	if err != nil {
		return 0, 0
	}
	return result.TotalRequirements, result.ValidRequirements
}

func parseFeature(path, body string) (featureFile, []bddRef) {
	digest := sha256.Sum256([]byte(body))
	file := featureFile{Path: filepath.ToSlash(path), SHA256: fmt.Sprintf("%x", digest), RelatedSpecs: []string{}}
	related := make([]string, 0)
	refs := make([]bddRef, 0)
	seen := map[string]bool{}
	inFence := false
	primaryMarkers := 0
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
		value, primary, marker := featureSpecMarker(trimmed)
		if !marker {
			if malformedPrimary, malformed := malformedFeatureSpecMarker(trimmed); malformed {
				file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "malformed-feature-spec-reference", Excerpt: trimmed})
				if malformedPrimary {
					primaryMarkers++
					if primaryMarkers > 1 {
						file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "ambiguous-feature-spec-reference", Excerpt: trimmed})
					}
				}
			}
			continue
		}
		ambiguousPrimary := false
		if primary {
			primaryMarkers++
			if primaryMarkers > 1 {
				file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "ambiguous-feature-spec-reference", Excerpt: trimmed})
				ambiguousPrimary = true
			}
		}
		value = filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
		if !isSpecPath(value) {
			file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "malformed-feature-spec-reference", Excerpt: trimmed})
			continue
		}
		if seen[value] {
			if !ambiguousPrimary {
				file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "ambiguous-feature-spec-reference", Excerpt: trimmed})
			}
			continue
		}
		related = append(related, value)
		refs = append(refs, bddRef{Path: value, Line: lineNumber})
		seen[value] = true
	}
	if primaryMarkers == 0 {
		file.Diagnostics = append(file.Diagnostics, diagnostic{Line: 1, Kind: "missing-feature-spec-reference", Excerpt: file.Path})
	}
	sort.Strings(related)
	file.RelatedSpecs = related
	return file, refs
}

func featureSpecMarker(line string) (value string, primary, ok bool) {
	if value, ok := strings.CutPrefix(line, "# SPEC:"); ok {
		return value, true, true
	}
	if value, ok := strings.CutPrefix(line, "# RELATED-SPEC:"); ok {
		return value, false, true
	}
	return "", false, false
}

func malformedFeatureSpecMarker(line string) (primary, ok bool) {
	if strings.HasPrefix(line, "# SPEC ") {
		return true, true
	}
	return false, strings.HasPrefix(line, "# RELATED-SPEC ")
}

func containsBDDRef(refs []bddRef, path string) bool {
	for _, ref := range refs {
		if ref.Path == path {
			return true
		}
	}
	return false
}

func collectSeeds(files []specFile, activeHarnesses []string) []seed {
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
	for _, harness := range activeHarnesses {
		appendSeed("harness-terminology", harness, harnessTerminologyEvidence(files, harness))
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

func harnessTerminologyEvidence(files []specFile, harness string) []evidence {
	items := make([]evidence, 0)
	for _, file := range files {
		for _, requirement := range file.Requirements {
			if !containsHarnessTerminology(requirement.Excerpt, harness) {
				continue
			}
			items = append(items, evidenceForRequirement(file.Path, requirement))
			break
		}
	}
	return items
}

func evidenceForRequirement(path string, requirement requirement) evidence {
	return evidence{Path: path, Line: requirement.Line, RequirementID: requirement.ID, Excerpt: requirement.Excerpt}
}

func containsHarnessTerminology(text, harness string) bool {
	haystack := strings.ToLower(text)
	needle := strings.ToLower(strings.TrimSpace(harness))
	if needle == "" {
		return false
	}
	for offset := 0; offset <= len(haystack)-len(needle); {
		relative := strings.Index(haystack[offset:], needle)
		if relative < 0 {
			return false
		}
		start := offset + relative
		end := start + len(needle)
		beforeBoundary := start == 0 || !isHarnessIdentifierByte(haystack[start-1])
		afterBoundary := end == len(haystack) || !isHarnessIdentifierByte(haystack[end])
		if beforeBoundary && afterBoundary {
			return true
		}
		offset = start + 1
	}
	return false
}

func isHarnessIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_' || value == '-'
}

func activeMembersFromBody(body string, available bool) ([]string, []string) {
	if !available {
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

func gitWithOutputLimit(root string, limit int64, args ...string) (string, error) {
	return gitWithLimits(root, limit, maxGitCommandDuration, args...)
}

func gitWithLimits(root string, outputLimit int64, wallTime time.Duration, args ...string) (string, error) {
	if outputLimit <= 0 {
		return "", errors.New("git output limit must be positive")
	}
	if wallTime <= 0 {
		return "", errors.New("git wall-time limit must be positive")
	}
	executable, err := trustedGitExecutable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), wallTime)
	defer cancel()
	output, err := gitBytesWithContext(ctx, executable, root, outputLimit, nil, args...)
	if errors.Is(err, context.DeadlineExceeded) {
		return "", fmt.Errorf("git %s exceeded %s wall-time limit", strings.Join(args, " "), wallTime)
	}
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var (
	trustedGitOnce       sync.Once
	trustedGitResolved   gitExecutable
	trustedGitResolveErr error
)

func trustedGitExecutable() (gitExecutable, error) {
	trustedGitOnce.Do(func() {
		resolved, err := exec.LookPath("git")
		if err != nil {
			trustedGitResolveErr = fmt.Errorf("resolve PATH-selected Git toolchain input: %w", err)
			return
		}
		trustedGitResolved, trustedGitResolveErr = resolveAbsoluteGitExecutable(resolved)
	})
	return trustedGitResolved, trustedGitResolveErr
}

func resolveAbsoluteGitExecutable(path string) (gitExecutable, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return gitExecutable{}, fmt.Errorf("resolve Git executable path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return gitExecutable{}, fmt.Errorf("resolve Git executable symlinks: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return gitExecutable{}, fmt.Errorf("inspect Git executable: %w", err)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return gitExecutable{}, errors.New("PATH-selected Git toolchain input must be an executable regular file")
	}
	return gitExecutable{path: canonical}, nil
}

func collectGitTrustInputs(executable gitExecutable, root string, runGit func(string, ...string) ([]byte, error)) (gitTrustInputs, error) {
	executableIdentity, err := fileSHA256Identity(executable.Path(), maxGitExecutableIdentityBytes)
	if err != nil {
		return gitTrustInputs{}, fmt.Errorf("identify canonical Git executable: %w", err)
	}
	workTreeRoot, err := directoryPathIdentity(root)
	if err != nil {
		return gitTrustInputs{}, fmt.Errorf("identify Git-resolved worktree root: %w", err)
	}
	resolveDir := func(label string, args ...string) (string, string, error) {
		output, commandErr := runGit(root, args...)
		if commandErr != nil {
			return "", "", fmt.Errorf("resolve %s: %w", label, commandErr)
		}
		path := strings.TrimSpace(string(output))
		identity, identityErr := directoryPathIdentity(path)
		if identityErr != nil {
			return "", "", fmt.Errorf("resolve %s: %w", label, identityErr)
		}
		return path, identity, nil
	}
	_, gitDir, err := resolveDir("repository Git directory", "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return gitTrustInputs{}, err
	}
	_, commonDir, err := resolveDir("common Git directory", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return gitTrustInputs{}, err
	}
	objectDirPath, objectDir, err := resolveDir("object directory", "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return gitTrustInputs{}, err
	}
	alternates, err := alternateObjectDirIdentities(objectDirPath)
	if err != nil {
		return gitTrustInputs{}, err
	}
	return gitTrustInputs{
		Executable:          executableIdentity,
		WorkTreeRoot:        workTreeRoot,
		GitDir:              gitDir,
		CommonDir:           commonDir,
		ObjectDir:           objectDir,
		AlternateObjectDirs: alternates,
	}, nil
}

func fileSHA256Identity(path string, limit int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return "", fmt.Errorf("file %q is not a regular file within the %d-byte identity limit", path, limit)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	bytesRead, err := io.Copy(digest, io.LimitReader(file, limit+1))
	if err != nil {
		return "", err
	}
	if bytesRead > limit {
		return "", fmt.Errorf("file %q changed beyond the %d-byte identity limit", path, limit)
	}
	return "sha256:" + fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func directoryPathIdentity(path string) (string, error) {
	canonical, err := canonicalDirectoryPath(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	return "path-sha256:" + fmt.Sprintf("%x", digest), nil
}

func canonicalDirectoryPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return "", errors.New("git returned an empty or invalid directory path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", canonical)
	}
	return filepath.Clean(canonical), nil
}

func alternateObjectDirIdentities(objectDir string) ([]string, error) {
	canonicalObjectDir, err := canonicalDirectoryPath(objectDir)
	if err != nil {
		return nil, err
	}
	identities := make([]string, 0)
	seen := map[string]bool{canonicalObjectDir: true}
	var visit func(string) error
	visit = func(current string) error {
		entries, entriesErr := configuredAlternateObjectDirs(current)
		if entriesErr != nil {
			return entriesErr
		}
		for lineNumber, entry := range entries {
			path := entry
			if !filepath.IsAbs(path) {
				path = filepath.Join(current, path)
			}
			canonical, canonicalErr := canonicalDirectoryPath(path)
			if canonicalErr != nil {
				return fmt.Errorf("resolve configured object alternate on line %d: %w", lineNumber+1, canonicalErr)
			}
			if seen[canonical] {
				return fmt.Errorf("configured object alternate on line %d creates a duplicate or cycle", lineNumber+1)
			}
			if len(identities) >= maxAlternateObjectRoutes {
				return fmt.Errorf("configured object alternates exceed %d routed directories", maxAlternateObjectRoutes)
			}
			seen[canonical] = true
			identity, identityErr := directoryPathIdentity(canonical)
			if identityErr != nil {
				return fmt.Errorf("identify configured object alternate on line %d: %w", lineNumber+1, identityErr)
			}
			identities = append(identities, identity)
			if visitErr := visit(canonical); visitErr != nil {
				return visitErr
			}
		}
		return nil
	}
	if err := visit(canonicalObjectDir); err != nil {
		return nil, err
	}
	return identities, nil
}

func configuredAlternateObjectDirs(objectDir string) ([]string, error) {
	configPath := filepath.Join(objectDir, "info", "alternates")
	data, err := readOptionalBoundedRegularFile(configPath, maxAlternateConfigBytes)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read configured object alternates: %w", err)
	}
	entries := make([]string, 0)
	for lineNumber, entry := range strings.Split(string(data), "\n") {
		entry = strings.TrimSuffix(entry, "\r")
		if entry == "" {
			continue
		}
		if strings.ContainsRune(entry, 0) {
			return nil, fmt.Errorf("configured object alternate on line %d is invalid", lineNumber+1)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func readOptionalBoundedRegularFile(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("file limit must be positive")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%q exceeds %d bytes", path, limit)
	}
	return data, nil
}

type boundedCommandBuffer struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int64
	exceeded bool
	cancel   context.CancelFunc
}

func (buffer *boundedCommandBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if buffer.exceeded {
		return 0, errGitOutputLimit
	}
	remaining := buffer.limit - int64(buffer.buffer.Len())
	if int64(len(data)) <= remaining {
		return buffer.buffer.Write(data)
	}
	written := 0
	if remaining > 0 {
		written, _ = buffer.buffer.Write(data[:int(remaining)])
	}
	buffer.exceeded = true
	buffer.cancel()
	return written, errGitOutputLimit
}

func (buffer *boundedCommandBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return bytes.Clone(buffer.buffer.Bytes())
}

func (buffer *boundedCommandBuffer) Exceeded() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.exceeded
}

func gitBytesWithContext(ctx context.Context, executable gitExecutable, root string, outputLimit int64, input []byte, args ...string) ([]byte, error) {
	if outputLimit <= 0 {
		return nil, errors.New("git output limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fullArgs := append([]string{"--no-replace-objects", "--no-lazy-fetch", "-C", root}, args...)
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &boundedCommandBuffer{limit: outputLimit, cancel: cancel}
	command := exec.CommandContext(commandCtx, executable.Path(), fullArgs...)
	command.Dir = root
	command.Env = cleanGitEnvironment(executable.Path())
	command.Stdin = bytes.NewReader(input)
	command.Stdout = output
	if input == nil {
		command.Stderr = output
	} else {
		command.Stderr = io.Discard
	}
	command.WaitDelay = maxGitWaitDelay
	err := command.Run()
	result := output.Bytes()
	if output.Exceeded() {
		return nil, fmt.Errorf("git %s output exceeds %d bytes: %w", strings.Join(args, " "), outputLimit, errGitOutputLimit)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(result)))
	}
	return result, nil
}

func readReport(path string) (report, error) {
	return readReportWithLimit(path, maxReportInputBytes)
}

func readReportWithLimit(path string, limit int64) (report, error) {
	data, err := readStableBoundedFile(path, limit)
	if err != nil {
		return report{}, err
	}
	if err := validateUniqueJSONDocument(data, maxJSONDepth); err != nil {
		return report{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := validateExactJSONFieldNames(data, reflect.TypeFor[report]()); err != nil {
		return report{}, fmt.Errorf("decode %s: %w", path, err)
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
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(data, &topLevel); err != nil {
		return report{}, fmt.Errorf("decode %s: inspect inventory payload fields: %w", path, err)
	}
	for _, key := range []string{"inventory", "features", "seeds"} {
		if _, ok := topLevel[key]; ok {
			decoded.inventoryPayloadPresent = true
			break
		}
	}
	return decoded, nil
}

// validateExactJSONFieldNames closes encoding/json's case-insensitive struct
// field matching. DisallowUnknownFields remains the type/schema decoder; this
// guard ensures every declared field name is written exactly as its JSON tag.
func validateExactJSONFieldNames(data []byte, valueType reflect.Type) error {
	return validateExactJSONValue(json.RawMessage(data), valueType)
}

//nolint:gocyclo // Recursive schema walking handles each composite JSON shape explicitly and rejects unknown object keys.
func validateExactJSONValue(raw json.RawMessage, valueType reflect.Type) error {
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	// Primitive kinds have no nested field names to validate. Pointers are
	// dereferenced above; the report schema contains no interface fields.
	switch valueType.Kind() { //nolint:exhaustive // Primitive kinds contain no nested JSON field names.
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return fmt.Errorf("JSON object for %s: %w", valueType, err)
		}
		fields := jsonFieldTypes(valueType)
		for key, child := range object {
			fieldType, ok := fields[key]
			if !ok {
				return fmt.Errorf("non-exact or unknown JSON object key %q", key)
			}
			if err := validateExactJSONValue(child, fieldType); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return fmt.Errorf("JSON array for %s: %w", valueType, err)
		}
		for _, child := range values {
			if err := validateExactJSONValue(child, valueType.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map:
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("JSON object for %s: %w", valueType, err)
		}
		for _, child := range entries {
			if err := validateExactJSONValue(child, valueType.Elem()); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	return nil
}

func jsonFieldTypes(valueType reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, valueType.NumField())
	for field := range valueType.Fields() {
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

func readStableBoundedFile(path string, limit int64) ([]byte, error) {
	return readStableBoundedFileWithHook(path, limit, nil)
}

//nolint:gocyclo // Ordered fail-closed input authentication keeps every identity and content check explicit.
func readStableBoundedFileWithHook(path string, limit int64, betweenReads func() error) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("report input limit must be positive")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	originalParent := filepath.Dir(absolute)
	parentRoot, err := openDescriptorWalkedRoot(originalParent)
	if err != nil {
		return nil, fmt.Errorf("authenticate report input ancestors %q: %w", path, err)
	}
	defer parentRoot.Close()
	name := filepath.Base(absolute)
	pathInfo, err := parentRoot.Lstat(name)
	if err != nil {
		return nil, err
	}
	if err := validateSingleLinkReportFile(path, pathInfo, limit); err != nil {
		return nil, err
	}
	file, err := openReportFile(parentRoot, name)
	if err != nil {
		return nil, fmt.Errorf("open report input %q without following links: %w", path, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := validateSingleLinkReportFile(path, openedInfo, limit); err != nil {
		return nil, err
	}
	if !os.SameFile(pathInfo, openedInfo) {
		return nil, fmt.Errorf("report input %q changed before it was opened", path)
	}
	first, err := readBoundedFileFromStart(file, path, limit)
	if err != nil {
		return nil, err
	}
	middleInfo, err := file.Stat()
	if err != nil || !stableReportRead(openedInfo, middleInfo, pathInfo, len(first)) {
		return nil, fmt.Errorf("report input %q changed during its first authenticated read", path)
	}
	if betweenReads != nil {
		if err := betweenReads(); err != nil {
			return nil, fmt.Errorf("report input %q read authentication hook: %w", path, err)
		}
	}
	second, err := readBoundedFileFromStart(file, path, limit)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, fmt.Errorf("report input %q failed double-read content authentication", path)
	}
	finalOpenedInfo, err := file.Stat()
	if err != nil || !stableReportRead(openedInfo, finalOpenedInfo, pathInfo, len(first)) {
		return nil, fmt.Errorf("report input %q changed while it was read", path)
	}
	freshParentRoot, err := openDescriptorWalkedRoot(originalParent)
	if err != nil {
		return nil, fmt.Errorf("reauthenticate report input ancestors %q: %w", path, err)
	}
	defer freshParentRoot.Close()
	finalPathInfo, err := freshParentRoot.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("reauthenticate report input %q: %w", path, err)
	}
	if err := validateSingleLinkReportFile(path, finalPathInfo, limit); err != nil {
		return nil, err
	}
	reopened, err := openReportFile(freshParentRoot, name)
	if err != nil {
		return nil, fmt.Errorf("reopen report input %q without following links: %w", path, err)
	}
	reopenedInfo, statErr := reopened.Stat()
	closeErr := reopened.Close()
	if statErr != nil {
		return nil, statErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if !stableReportRead(openedInfo, reopenedInfo, finalPathInfo, len(first)) {
		return nil, fmt.Errorf("report input %q changed before path reauthentication", path)
	}
	return first, nil
}

func stableReportRead(openedBefore, openedAfter, pathAfter os.FileInfo, bytesRead int) bool {
	return os.SameFile(openedBefore, openedAfter) &&
		os.SameFile(openedAfter, pathAfter) &&
		openedBefore.Mode() == openedAfter.Mode() &&
		openedAfter.Mode() == pathAfter.Mode() &&
		openedBefore.Size() == openedAfter.Size() &&
		openedAfter.Size() == pathAfter.Size() &&
		openedAfter.Size() == int64(bytesRead) &&
		openedBefore.ModTime().Equal(openedAfter.ModTime()) &&
		openedAfter.ModTime().Equal(pathAfter.ModTime()) &&
		hasSingleLink(openedBefore) && hasSingleLink(openedAfter) && hasSingleLink(pathAfter)
}

//nolint:gocyclo // Each ancestor is checked before, through, and after descriptor traversal.
func openDescriptorWalkedRoot(realPath string) (*os.Root, error) {
	absolute, err := filepath.Abs(realPath)
	if err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(absolute)
	filesystemRoot := volume + string(filepath.Separator)
	current, err := os.OpenRoot(filesystemRoot)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(filesystemRoot, absolute)
	if err != nil {
		current.Close()
		return nil, err
	}
	if relative == "." {
		return current, nil
	}
	for segment := range strings.SplitSeq(relative, string(filepath.Separator)) {
		if segment == "" || segment == "." || segment == ".." {
			current.Close()
			return nil, fmt.Errorf("invalid real ancestor segment %q", segment)
		}
		before, err := current.Lstat(segment)
		if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
			current.Close()
			return nil, fmt.Errorf("real ancestor %q is not a stable directory", segment)
		}
		next, err := current.OpenRoot(segment)
		if err != nil {
			current.Close()
			return nil, err
		}
		opened, openErr := next.Stat(".")
		after, afterErr := current.Lstat(segment)
		if openErr != nil || afterErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.IsDir() ||
			!os.SameFile(before, opened) || !os.SameFile(opened, after) || before.Mode() != after.Mode() {
			next.Close()
			current.Close()
			return nil, fmt.Errorf("real ancestor %q changed during descriptor walk", segment)
		}
		current.Close()
		current = next
	}
	return current, nil
}

func validateSingleLinkReportFile(path string, info os.FileInfo, limit int64) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("report input %q must be a regular non-symlink file", path)
	}
	if !hasSingleLink(info) {
		return fmt.Errorf("report input %q must have exactly one filesystem link", path)
	}
	if info.Size() < 0 || info.Size() > limit {
		return fmt.Errorf("report input %q exceeds %d bytes", path, limit)
	}
	return nil
}

func hasSingleLink(info os.FileInfo) bool {
	if info == nil || info.Sys() == nil {
		return false
	}
	value := reflect.ValueOf(info.Sys())
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	field := value.FieldByName("Nlink")
	if field.CanInt() {
		return field.Int() == 1
	}
	if field.CanUint() {
		return field.Uint() == 1
	}
	return false
}

func readBoundedFileFromStart(file *os.File, path string, limit int64) ([]byte, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind report input %q: %w", path, err)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("report input %q exceeds %d bytes", path, limit)
	}
	return data, nil
}

func validateUniqueJSONDocument(data []byte, depthLimit int) error {
	if depthLimit <= 0 {
		return errors.New("JSON nesting limit must be positive")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := walkUniqueJSONValue(decoder, 0, depthLimit); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("contains multiple JSON values")
		}
		return fmt.Errorf("invalid trailing JSON data: %w", err)
	}
	return nil
}

//nolint:gocyclo // Recursive JSON grammar validation keeps duplicate-key state local to each object.
func walkUniqueJSONValue(decoder *json.Decoder, depth, depthLimit int) error {
	if depth > depthLimit {
		return fmt.Errorf("JSON nesting exceeds %d levels", depthLimit)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, structured := token.(json.Delim)
	if !structured {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if keys[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			keys[key] = true
			if err := walkUniqueJSONValue(decoder, depth+1, depthLimit); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("JSON object is not properly terminated")
		}
	case '[':
		for decoder.More() {
			if err := walkUniqueJSONValue(decoder, depth+1, depthLimit); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("JSON array is not properly terminated")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func cleanGitEnvironment(executable string) []string {
	pathEntries := []string{filepath.Dir(executable), "/usr/bin", "/bin"}
	pathEntries = slices.Compact(pathEntries)
	environment := []string{
		"HOME=/var/empty",
		"LANG=C",
		"LC_ALL=C",
		"PATH=" + strings.Join(pathEntries, string(os.PathListSeparator)),
		"PAGER=cat",
		"XDG_CONFIG_HOME=/var/empty",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	}
	for _, name := range []string{"COMSPEC", "PATHEXT", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func marshalReportWithLimit(value report, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("artifact output limit must be positive")
	}
	output := &boundedJSONBuffer{limit: limit}
	if err := encodeBoundedJSON(output, reflect.ValueOf(value), "", "  "); err != nil {
		if errors.Is(err, errJSONArtifactLimit) {
			return nil, fmt.Errorf("inventory JSON exceeds %d-byte artifact output limit", limit)
		}
		return nil, err
	}
	if _, err := output.Write([]byte{'\n'}); err != nil {
		return nil, fmt.Errorf("inventory JSON exceeds %d-byte artifact output limit", limit)
	}
	return output.Bytes(), nil
}

var errJSONArtifactLimit = errors.New("JSON artifact output limit exceeded")

type boundedJSONBuffer struct {
	buffer bytes.Buffer
	limit  int64
}

func (out *boundedJSONBuffer) Write(data []byte) (int, error) {
	if int64(len(data)) > out.limit-int64(out.buffer.Len()) {
		return 0, errJSONArtifactLimit
	}
	return out.buffer.Write(data)
}

func (out *boundedJSONBuffer) Bytes() []byte { return bytes.Clone(out.buffer.Bytes()) }

func encodeBoundedJSON(out io.Writer, value reflect.Value, prefix, indent string) error {
	if !value.IsValid() {
		return writeBoundedJSON(out, "null")
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return writeBoundedJSON(out, "null")
		}
		return encodeBoundedJSON(out, value.Elem(), prefix, indent)
	}
	//nolint:exhaustive // The report schema deliberately excludes every other reflect.Kind.
	switch value.Kind() {
	case reflect.Struct:
		return encodeBoundedJSONObject(out, value, prefix, indent)
	case reflect.Slice, reflect.Array:
		return encodeBoundedJSONArray(out, value, prefix, indent)
	case reflect.Map:
		return encodeBoundedJSONMap(out, value, prefix, indent)
	case reflect.String:
		return writeBoundedJSONString(out, value.String())
	case reflect.Bool:
		return writeBoundedJSON(out, strconv.FormatBool(value.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return writeBoundedJSON(out, strconv.FormatInt(value.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return writeBoundedJSON(out, strconv.FormatUint(value.Uint(), 10))
	default:
		return fmt.Errorf("encode report JSON: unsupported %s", value.Type())
	}
}

func encodeBoundedJSONObject(out io.Writer, value reflect.Value, prefix, indent string) error {
	type field struct {
		name  string
		value reflect.Value
	}
	fields := make([]field, 0, value.NumField())
	typeOfValue := value.Type()
	for index := 0; index < value.NumField(); index++ {
		definition := typeOfValue.Field(index)
		if definition.PkgPath != "" {
			continue
		}
		name, options := jsonFieldName(definition)
		if name == "" || name == "-" {
			continue
		}
		fieldValue := value.Field(index)
		if strings.Contains(options, "omitempty") && boundedJSONEmpty(fieldValue) {
			continue
		}
		fields = append(fields, field{name: name, value: fieldValue})
	}
	if err := writeBoundedJSON(out, "{"); err != nil {
		return err
	}
	for index, item := range fields {
		if err := writeBoundedJSONElementPrefix(out, index, prefix, indent); err != nil {
			return err
		}
		if err := writeBoundedJSONNameSeparator(out, item.name, indent); err != nil {
			return err
		}
		if err := encodeBoundedJSON(out, item.value, prefix+indent, indent); err != nil {
			return err
		}
	}
	if err := writeBoundedJSONClosingIndent(out, len(fields), prefix, indent); err != nil {
		return err
	}
	return writeBoundedJSON(out, "}")
}

func encodeBoundedJSONArray(out io.Writer, value reflect.Value, prefix, indent string) error {
	if value.Kind() == reflect.Slice && value.IsNil() {
		return writeBoundedJSON(out, "null")
	}
	if err := writeBoundedJSON(out, "["); err != nil {
		return err
	}
	for index := 0; index < value.Len(); index++ {
		if err := writeBoundedJSONElementPrefix(out, index, prefix, indent); err != nil {
			return err
		}
		if err := encodeBoundedJSON(out, value.Index(index), prefix+indent, indent); err != nil {
			return err
		}
	}
	if err := writeBoundedJSONClosingIndent(out, value.Len(), prefix, indent); err != nil {
		return err
	}
	return writeBoundedJSON(out, "]")
}

func encodeBoundedJSONMap(out io.Writer, value reflect.Value, prefix, indent string) error {
	if value.IsNil() {
		return writeBoundedJSON(out, "null")
	}
	if value.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("encode report JSON: unsupported map key %s", value.Type().Key())
	}
	keys := value.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	if err := writeBoundedJSON(out, "{"); err != nil {
		return err
	}
	for index, key := range keys {
		if err := writeBoundedJSONElementPrefix(out, index, prefix, indent); err != nil {
			return err
		}
		if err := writeBoundedJSONNameSeparator(out, key.String(), indent); err != nil {
			return err
		}
		if err := encodeBoundedJSON(out, value.MapIndex(key), prefix+indent, indent); err != nil {
			return err
		}
	}
	if err := writeBoundedJSONClosingIndent(out, len(keys), prefix, indent); err != nil {
		return err
	}
	return writeBoundedJSON(out, "}")
}

func writeBoundedJSONElementPrefix(out io.Writer, index int, prefix, indent string) error {
	separator := ""
	if index > 0 {
		separator = ","
	}
	if indent != "" {
		separator += "\n" + prefix + indent
	}
	return writeBoundedJSON(out, separator)
}

func writeBoundedJSONClosingIndent(out io.Writer, count int, prefix, indent string) error {
	if count == 0 || indent == "" {
		return nil
	}
	return writeBoundedJSON(out, "\n"+prefix)
}

func writeBoundedJSONNameSeparator(out io.Writer, name, indent string) error {
	if err := writeBoundedJSONString(out, name); err != nil {
		return err
	}
	separator := ":"
	if indent != "" {
		separator += " "
	}
	return writeBoundedJSON(out, separator)
}

func jsonFieldName(field reflect.StructField) (string, string) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name, ""
	}
	parts := strings.Split(tag, ",")
	return parts[0], strings.Join(parts[1:], ",")
}

func boundedJSONEmpty(value reflect.Value) bool {
	//nolint:exhaustive // Only kinds used by omitempty fields in the report schema can be empty.
	switch value.Kind() {
	case reflect.Array:
		return value.Len() == 0
	case reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Interface, reflect.Pointer:
		return value.IsNil()
	case reflect.Bool:
		return !value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint() == 0
	}
	return false
}

func writeBoundedJSON(out io.Writer, value string) error {
	_, err := io.WriteString(out, value)
	return err
}

//nolint:gocyclo // Each JSON escape class is handled explicitly so writes stay bounded.
func writeBoundedJSONString(out io.Writer, value string) error {
	if err := writeBoundedJSON(out, `"`); err != nil {
		return err
	}
	for len(value) > 0 {
		byteValue := value[0]
		switch byteValue {
		case '"':
			if err := writeBoundedJSON(out, `\"`); err != nil {
				return err
			}
			value = value[1:]
		case '\\':
			if err := writeBoundedJSON(out, `\\`); err != nil {
				return err
			}
			value = value[1:]
		case '\b':
			if err := writeBoundedJSON(out, `\b`); err != nil {
				return err
			}
			value = value[1:]
		case '\f':
			if err := writeBoundedJSON(out, `\f`); err != nil {
				return err
			}
			value = value[1:]
		case '\n':
			if err := writeBoundedJSON(out, `\n`); err != nil {
				return err
			}
			value = value[1:]
		case '\r':
			if err := writeBoundedJSON(out, `\r`); err != nil {
				return err
			}
			value = value[1:]
		case '\t':
			if err := writeBoundedJSON(out, `\t`); err != nil {
				return err
			}
			value = value[1:]
		default:
			if byteValue < 0x20 {
				if err := writeBoundedJSON(out, fmt.Sprintf(`\u%04x`, byteValue)); err != nil {
					return err
				}
				value = value[1:]
				continue
			}
			runeValue, size := utf8.DecodeRuneInString(value)
			if runeValue == utf8.RuneError && size == 1 {
				if err := writeBoundedJSON(out, "�"); err != nil {
					return err
				}
				value = value[1:]
				continue
			}
			if runeValue == '\u2028' || runeValue == '\u2029' {
				if err := writeBoundedJSON(out, fmt.Sprintf(`\u%04x`, runeValue)); err != nil {
					return err
				}
				value = value[size:]
				continue
			}
			if _, err := io.WriteString(out, value[:size]); err != nil {
				return err
			}
			value = value[size:]
		}
	}
	return writeBoundedJSON(out, `"`)
}

func renderHTMLWithLimit(audit report, inventory *report, limit int64) (string, error) {
	if limit <= 0 {
		return "", errors.New("artifact output limit must be positive")
	}
	return renderBoundedHTML(audit, inventory, limit)
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
	if report.Methodology.GitEvidenceTrust != gitEvidenceTrustDisclosure {
		return errors.New("methodology git_evidence_trust must disclose the required Git toolchain and object-store trust boundary")
	}
	if !validGitTrustInputs(report.Methodology.GitTrustInputs) {
		return errors.New("methodology git_trust_inputs must identify the canonical executable and resolved worktree, Git, common, object, and alternate object directories")
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
		for _, feature := range report.Features {
			diagnosticCount += len(feature.Diagnostics)
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

func validateSemanticReport(report report) error {
	if report.hasInventoryPayload() {
		return errors.New("semantic report must omit inventory, features, and seeds; use the separately supplied pinned inventory")
	}
	return validateReport(report)
}

//nolint:gocyclo // Sequential pinned-evidence cross-checks keep every trust transition visible and fail closed.
func validateAgainstInventory(semantic, inventory report) error {
	if err := validateSemanticReport(semantic); err != nil {
		return err
	}
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
	if !reflect.DeepEqual(semantic.Methodology.GitTrustInputs, inventory.Methodology.GitTrustInputs) {
		return errors.New("semantic report Git trust inputs do not match the pinned inventory")
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
	requirementIDs := map[string]map[string]bool{}
	features := map[string]featureFile{}
	files := map[string]bool{}
	specFeatures := map[string]map[string]bool{}
	for _, file := range inventory.Inventory {
		files[file.Path] = true
		requirements[file.Path] = map[requirementKey]string{}
		requirementIDs[file.Path] = map[string]bool{}
		specFeatures[file.Path] = map[string]bool{}
		for _, requirement := range file.Requirements {
			requirements[file.Path][requirementKey{line: requirement.Line, id: requirement.ID}] = requirement.Excerpt
			requirementIDs[file.Path][requirement.ID] = true
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
		positive := finding.Verdict == "merge-now" || finding.Verdict == "extract-neutral-contract"
		if positive && finding.Classification == "shared-contract" {
			if err := validateSharedContractFeature(finding, features, specFeatures); err != nil {
				return err
			}
		}
		if positive {
			if err := validateOwnershipPlanAgainstInventory(finding, features, specFeatures, requirementIDs); err != nil {
				return err
			}
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

func validateSharedContractFeature(f finding, features map[string]featureFile, specFeatures map[string]map[string]bool) error {
	feature, ok := features[f.BDD.SharedContractFeature]
	if !ok {
		return fmt.Errorf("finding %q shared BDD feature %q is absent from the pinned feature inventory", f.ID, f.BDD.SharedContractFeature)
	}
	related := stringSet(feature.RelatedSpecs)
	for _, owner := range f.CurrentOwners {
		if !related[owner.Path] || !specFeatures[owner.Path][f.BDD.SharedContractFeature] {
			return fmt.Errorf("finding %q shared BDD feature %q must reciprocally link current owner %q", f.ID, f.BDD.SharedContractFeature, owner.Path)
		}
	}
	return nil
}

func validateOwnershipPlanAgainstInventory(f finding, features map[string]featureFile, specFeatures map[string]map[string]bool, requirementIDs map[string]map[string]bool) error {
	for _, entry := range f.OwnershipPlan.CurrentOwners {
		if entry.Action != "retire-normative-ownership" {
			continue
		}
		for _, mapping := range entry.Preservation.Requirements {
			if f.ProposedOwner.State == "existing" && !requirementIDs[f.ProposedOwner.Path][mapping.TargetID] {
				return fmt.Errorf("finding %q preservation target ID %q is absent from pinned proposed owner %q", f.ID, mapping.TargetID, f.ProposedOwner.Path)
			}
		}
		for _, mapping := range entry.Preservation.BDD {
			feature, ok := features[mapping.Feature]
			if !ok || !containsString(feature.RelatedSpecs, entry.Path) || !specFeatures[entry.Path][mapping.Feature] {
				return fmt.Errorf("finding %q preservation BDD feature %q must reciprocally link retired owner %q", f.ID, mapping.Feature, entry.Path)
			}
		}
		expectedFeatures := map[string]bool{}
		for _, featurePath := range f.BDD.Features {
			feature := features[featurePath]
			if containsString(feature.RelatedSpecs, entry.Path) && specFeatures[entry.Path][featurePath] {
				expectedFeatures[featurePath] = true
			}
		}
		mappedFeatures := map[string]bool{}
		for _, mapping := range entry.Preservation.BDD {
			mappedFeatures[mapping.Feature] = true
		}
		if !sameStringSet(mapKeys(expectedFeatures), mapKeys(mappedFeatures)) {
			return fmt.Errorf("finding %q preservation BDD mappings for retired owner %q must exactly cover selected reciprocal features", f.ID, entry.Path)
		}
	}
	return nil
}

func (report report) hasInventoryPayload() bool {
	return report.inventoryPayloadPresent || report.Inventory != nil || report.Features != nil || report.Seeds != nil
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
		for _, diagnostic := range feature.Diagnostics {
			if diagnostic.Line < 1 || !featureDiagnosticKinds[diagnostic.Kind] || strings.TrimSpace(diagnostic.Excerpt) == "" {
				return fmt.Errorf("feature inventory diagnostic in %s is invalid", feature.Path)
			}
		}
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
	if f.BDD.SharedContractFeature != "" && (!validFeaturePath(f.BDD.SharedContractFeature) || !containsString(f.BDD.Features, f.BDD.SharedContractFeature)) {
		return fmt.Errorf("finding %q shared BDD feature must be a selected feature", f.ID)
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
		if len(f.CurrentOwners) < 2 || !allProductSpecPaths(ownerPaths(f.CurrentOwners)) || distinctProductSpecPaths(f.Evidence) < 2 || f.ProposedOwner == nil || !isProductSpecPath(f.ProposedOwner.Path) || !ownerStates[f.ProposedOwner.State] || len(f.BDD.Features) == 0 {
			return fmt.Errorf("positive finding %q requires product SPEC current owners, a product SPEC proposed owner state, and BDD features", f.ID)
		}
		if strings.TrimSpace(f.OwnershipCompleteness) == "" || strings.TrimSpace(f.ProposedOwner.Rationale) == "" || strings.TrimSpace(f.ProposedOwner.NeutralityRationale) == "" {
			return fmt.Errorf("positive finding %q requires owner-completeness plus proposed-owner and neutrality rationales", f.ID)
		}
		if isHarnessSurfacePath(f.ProposedOwner.Path, active) {
			return fmt.Errorf("positive finding %q proposes harness-surface owner %q", f.ID, f.ProposedOwner.Path)
		}
		switch {
		case f.ProposedOwner.State == "existing":
			if !containsString(ownerPaths(f.CurrentOwners), f.ProposedOwner.Path) {
				return fmt.Errorf("positive finding %q must select an existing proposed owner from its pinned current-owner set", f.ID)
			}
		case containsString(ownerPaths(f.CurrentOwners), f.ProposedOwner.Path):
			return fmt.Errorf("positive finding %q cannot mark a current owner as new", f.ID)
		case isStrictDescendantOfCurrentOwner(f.ProposedOwner.Path, ownerPaths(f.CurrentOwners)):
			return fmt.Errorf("positive finding %q cannot propose a new owner beneath a current-owner directory", f.ID)
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
			implementationOwners := ownerPaths(f.CurrentOwners)
			if f.ProposedOwner != nil {
				implementationOwners = append(implementationOwners, f.ProposedOwner.Path)
			}
			if hasHarnessSurfaceOwner(implementationOwners, active) {
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
		if f.Classification == "shared-contract" && strings.TrimSpace(f.BDD.SharedContractFeature) == "" {
			return fmt.Errorf("positive shared-contract finding %q requires bdd.shared_contract_feature", f.ID)
		}
		if f.OwnershipPlan == nil {
			return fmt.Errorf("positive finding %q requires an ownership_plan", f.ID)
		}
		if err := validateOwnershipPlanShape(f); err != nil {
			return fmt.Errorf("positive finding %q ownership_plan: %w", f.ID, err)
		}
	}
	if f.Verdict == "resolve-product-divergence" || f.Verdict == "insufficient-evidence" {
		if strings.TrimSpace(f.Decision) == "" || f.Strength == "strong" {
			return fmt.Errorf("finding %q requires a decision and non-strong strength", f.ID)
		}
	}
	if !positive && (f.ProposedOwner != nil || f.OwnershipPlan != nil || f.BDD.SharedContractFeature != "") {
		return fmt.Errorf("non-positive finding %q cannot carry a canonical owner, ownership plan, or shared BDD feature", f.ID)
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

//nolint:gocyclo // Ownership topology is intentionally validated in one fail-closed sequence.
func validateOwnershipPlanShape(f finding) error {
	plan := f.OwnershipPlan
	if plan.Approval != "pending-maintainer-approval" {
		return errors.New("approval must be pending-maintainer-approval")
	}
	if len(plan.CurrentOwners) != len(f.CurrentOwners) {
		return errors.New("must include exactly one entry for every current owner")
	}
	ownerSet := stringSet(ownerPaths(f.CurrentOwners))
	seen := map[string]bool{}
	for _, entry := range plan.CurrentOwners {
		if !ownerSet[entry.Path] || seen[entry.Path] || strings.TrimSpace(entry.Rationale) == "" {
			return errors.New("must include exact current-owner paths with rationales")
		}
		seen[entry.Path] = true
		switch entry.Action {
		case "retain":
			if entry.Preservation != nil {
				return fmt.Errorf("retained owner %q cannot carry a preservation migration", entry.Path)
			}
		case "retire-normative-ownership":
			if entry.Preservation == nil {
				return fmt.Errorf("retired owner %q requires structured preservation", entry.Path)
			}
			if err := validatePreservationShape(f, entry); err != nil {
				return err
			}
		default:
			return fmt.Errorf("owner %q has unsupported action %q", entry.Path, entry.Action)
		}
	}
	for _, entry := range plan.CurrentOwners {
		if f.ProposedOwner.State == "existing" && entry.Path == f.ProposedOwner.Path {
			if entry.Action != "retain" {
				return fmt.Errorf("existing proposed owner %q must be retained", entry.Path)
			}
			continue
		}
		if entry.Action != "retire-normative-ownership" {
			return fmt.Errorf("current owner %q must retire normative ownership", entry.Path)
		}
	}
	return nil
}

//nolint:gocyclo // Preservation coverage keeps requirement, BDD, applicability, and target-state checks together.
func validatePreservationShape(f finding, entry ownershipPlanOwner) error {
	preservation := entry.Preservation
	ownerEvidence := make([]evidence, 0)
	for _, item := range f.Evidence {
		if item.Path == entry.Path {
			ownerEvidence = append(ownerEvidence, item)
		}
	}
	if len(ownerEvidence) == 0 || len(preservation.Requirements) != len(ownerEvidence) {
		return fmt.Errorf("retired owner %q must map every source requirement evidence record", entry.Path)
	}
	sourceKeys := map[string]bool{}
	for _, item := range ownerEvidence {
		sourceKeys[evidenceKey(item)] = true
	}
	seenSources := map[string]bool{}
	for _, mapping := range preservation.Requirements {
		key := evidenceKey(mapping.Source)
		if !sourceKeys[key] || seenSources[key] || strings.TrimSpace(mapping.TargetID) == "" || !targetStates[mapping.TargetState] || !preservationStrategies[mapping.Strategy] {
			return fmt.Errorf("retired owner %q has invalid requirement preservation", entry.Path)
		}
		if mapping.Strategy == "preserve-id" && mapping.TargetID != mapping.Source.RequirementID {
			return fmt.Errorf("retired owner %q preserve-id mapping must retain %q", entry.Path, mapping.Source.RequirementID)
		}
		if f.ProposedOwner.State == "existing" && mapping.TargetState != "existing" {
			return fmt.Errorf("retired owner %q cannot plan a target in an existing proposed owner", entry.Path)
		}
		if f.ProposedOwner.State == "new" && mapping.TargetState != "planned" {
			return fmt.Errorf("retired owner %q must plan targets for a new proposed owner", entry.Path)
		}
		seenSources[key] = true
	}
	if len(seenSources) != len(sourceKeys) {
		return fmt.Errorf("retired owner %q must map every source requirement evidence record", entry.Path)
	}
	if preservation.ApplicabilityBasis != f.ApplicabilityBasis || !reflect.DeepEqual(preservation.Applicability, f.Applicability) {
		return fmt.Errorf("retired owner %q must copy the finding applicability basis and matrix exactly", entry.Path)
	}
	if len(preservation.BDD) == 0 {
		return fmt.Errorf("retired owner %q requires reciprocal BDD preservation mappings", entry.Path)
	}
	sharedMapped := false
	seenBDD := map[string]bool{}
	for _, mapping := range preservation.BDD {
		if !containsString(f.BDD.Features, mapping.Feature) || mapping.SourceOwner != entry.Path || mapping.TargetOwner != f.ProposedOwner.Path || seenBDD[mapping.Feature] {
			return fmt.Errorf("retired owner %q has invalid BDD preservation", entry.Path)
		}
		if mapping.Feature == f.BDD.SharedContractFeature {
			sharedMapped = true
		}
		seenBDD[mapping.Feature] = true
	}
	if f.Classification == "shared-contract" && !sharedMapped {
		return fmt.Errorf("retired owner %q preservation must include bdd.shared_contract_feature", entry.Path)
	}
	return nil
}

func evidenceKey(item evidence) string {
	return item.Path + "\x00" + strconv.Itoa(item.Line) + "\x00" + item.RequirementID + "\x00" + item.Excerpt
}

func hasHarnessSurfaceOwner(paths []string, active map[string]bool) bool {
	for _, path := range paths {
		if isHarnessSurfacePath(path, active) {
			return true
		}
	}
	return false
}

// isHarnessSurfacePath recognizes a bounded catalog of harness registration
// surfaces plus finite aliases derived from the pinned active-member names.
// It intentionally does not classify arbitrary internal/ or pkg/ paths.
func isHarnessSurfacePath(path string, active map[string]bool) bool {
	harnessSegments := map[string]bool{
		".agents":        true,
		".claude":        true,
		".claude-plugin": true,
		".codex":         true,
		".dear-agent":    true,
		".gemini":        true,
		".opencode":      true,
		".pi":            true,
		".aider":         true,
		".continue":      true,
		".cursor":        true,
		".roo":           true,
		".vscode":        true,
		".windsurf":      true,
		"agysession":     true,
		"claudehooks":    true,
		"claudeui":       true,
		"codexadapter":   true,
		"codexarchive":   true,
		"codexcommand":   true,
		"codexcontrol":   true,
		"codexhooks":     true,
		"codexsession":   true,
		"piadapter":      true,
		"picommand":      true,
		"pisession":      true,
	}
	aliases := activeMemberAliases(active)
	for segment := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if harnessSegments[strings.ToLower(segment)] || aliases[normalizeHarnessAlias(segment)] {
			return true
		}
	}
	return false
}

func activeMemberAliases(active map[string]bool) map[string]bool {
	aliases := map[string]bool{}
	for member := range active {
		normalized := normalizeHarnessAlias(member)
		if normalized == "" {
			continue
		}
		aliases[normalized] = true
		for _, suffix := range []string{"cli", "code", "agent", "harness"} {
			aliases[strings.TrimSuffix(normalized, suffix)] = true
		}
	}
	delete(aliases, "")
	return aliases
}

func normalizeHarnessAlias(value string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return -1
	}, value)
}

func isStrictDescendantOfCurrentOwner(proposed string, owners []string) bool {
	proposedDirectory := filepath.ToSlash(filepath.Dir(proposed))
	for _, owner := range owners {
		ownerDirectory := filepath.ToSlash(filepath.Dir(owner))
		if proposedDirectory != ownerDirectory && strings.HasPrefix(proposedDirectory, ownerDirectory+"/") {
			return true
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
	verdicts               = map[string]bool{"merge-now": true, "extract-neutral-contract": true, "keep-separate": true, "resolve-product-divergence": true, "insufficient-evidence": true}
	relationships          = map[string]bool{"same-observable": true, "overlapping-observables": true, "contradictory-observables": true, "same-vocabulary-only": true, "fixture-or-generated-copy": true}
	classifications        = map[string]bool{"shared-contract": true, "capability-variation": true, "native-adapter": true, "wrapper": true, "fixture": true, "implementation-detail": true}
	confidences            = map[string]bool{"confirmed": true, "likely": true, "tentative": true}
	strengths              = map[string]bool{"strong": true, "moderate": true, "exploratory": true}
	ownerStates            = map[string]bool{"existing": true, "new": true}
	targetStates           = map[string]bool{"existing": true, "planned": true}
	preservationStrategies = map[string]bool{"preserve-id": true, "canonical-reference": true}
	applicabilityBases     = map[string]bool{"active-members": true, "implementation-only": true}
	dispositions           = map[string]bool{"supported": true, "adapted": true, "unsupported": true, "not-applicable": true, "unknown": true}
	bddConsequences        = map[string]bool{"merge": true, "add-matrix": true, "adapter-only": true, "none": true, "resolve": true}
	seedKinds              = map[string]bool{"exact-body": true, "duplicate-id": true, "shared-bdd": true, "identical-file": true, "harness-terminology": true}
	diagnosticKinds        = map[string]bool{"anonymous-requirement": true, "nonconforming-requirement": true, "missing-bdd-feature": true, "nonreciprocal-bdd-feature": true, "malformed-bdd-feature-reference": true, "duplicate-bdd-feature-reference": true, "ambiguous-bdd-traceability-section": true}
	featureDiagnosticKinds = map[string]bool{"missing-feature-spec-reference": true, "malformed-feature-spec-reference": true, "ambiguous-feature-spec-reference": true, "missing-feature-spec": true, "nonreciprocal-feature-spec": true}
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

func validGitTrustInputs(inputs gitTrustInputs) bool {
	if !identityPattern.MatchString(inputs.Executable) ||
		!identityPattern.MatchString(inputs.WorkTreeRoot) ||
		!identityPattern.MatchString(inputs.GitDir) ||
		!identityPattern.MatchString(inputs.CommonDir) ||
		!identityPattern.MatchString(inputs.ObjectDir) ||
		inputs.AlternateObjectDirs == nil {
		return false
	}
	for _, alternate := range inputs.AlternateObjectDirs {
		if !identityPattern.MatchString(alternate) {
			return false
		}
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

func allProductSpecPaths(paths []string) bool {
	for _, path := range paths {
		if !isProductSpecPath(path) {
			return false
		}
	}
	return true
}

func isProductSpecPath(path string) bool {
	if !isSpecPath(path) {
		return false
	}
	for segment := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		switch segment {
		case "fixture", "fixtures", "generated", "migrations", "node_modules", "testdata", "vendor":
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

func renderHTML(audit report, inventory *report) string {
	output, err := renderBoundedHTML(audit, inventory, int64(^uint64(0)>>1))
	if err != nil {
		panic(err)
	}
	return output
}

//nolint:gocyclo // Linear self-contained report assembly keeps escaping and section order directly auditable.
func renderBoundedHTML(audit report, inventory *report, limit int64) (string, error) {
	candidates := append([]finding{}, audit.Candidates...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Rank < candidates[j].Rank })
	seeds := audit.Seeds
	if inventory != nil {
		seeds = inventory.Seeds
	}

	out := newBoundedHTMLBuilder(limit)
	out.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	fmt.Fprintf(out, "<title>SPEC governance audit · %s</title>", esc(audit.Snapshot.Repository))
	out.WriteString(`<style>
:root{--ink:#132238;--muted:#56657a;--paper:#f4f1ea;--card:#fffdf8;--line:#d9d2c5;--accent:#b5412b;--teal:#176b67;--gold:#a66b17;--soft:#ece7dc;--code:#f1eee6}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;color:var(--ink);background:var(--paper);font:16px/1.55 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}a{color:var(--teal)}code{font:0.9em/1.45 ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--code);padding:.08rem .3rem;border-radius:4px;overflow-wrap:anywhere}.shell{width:min(1180px,calc(100% - 2rem));margin:auto}.hero{padding:4.5rem 0 2.3rem;border-bottom:1px solid var(--line);background:linear-gradient(135deg,#fffaf0 0%,#e5efec 100%)}.eyebrow{margin:0 0 .5rem;color:var(--accent);font-weight:800;letter-spacing:.14em;text-transform:uppercase;font-size:.76rem}.hero h1{max-width:850px;margin:.1rem 0 .8rem;font:700 clamp(2.25rem,6vw,4.6rem)/.98 ui-serif,Georgia,serif;letter-spacing:-.04em}.lede{max-width:780px;color:var(--muted);font-size:1.08rem}.snapshot{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:.75rem;margin:1.5rem 0}.snapshot div{padding:.8rem 0;border-top:1px solid var(--line)}.snapshot dt{color:var(--muted);font-size:.75rem;text-transform:uppercase;letter-spacing:.08em}.snapshot dd{margin:.2rem 0 0}.warning{margin:1.2rem 0 0;padding:1rem 1.1rem;border:1px solid #e3b6a5;border-left:5px solid var(--accent);background:#fff5ef}.toc{position:sticky;top:0;z-index:5;background:rgba(244,241,234,.96);border-bottom:1px solid var(--line);backdrop-filter:blur(8px)}.toc .shell{display:flex;gap:1.1rem;overflow-x:auto;padding:.72rem 0}.toc a{white-space:nowrap;text-decoration:none;font-weight:700;font-size:.86rem}main{padding:2.2rem 0 5rem}section{scroll-margin-top:4rem;margin:0 0 3.5rem}h2{font:700 clamp(1.6rem,3vw,2.35rem)/1.05 ui-serif,Georgia,serif;letter-spacing:-.02em;margin:0 0 1rem}h3{font-size:1.25rem;line-height:1.25;margin:.15rem 0}.section-intro{max-width:780px;color:var(--muted)}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:.8rem}.metric{padding:1.05rem;border:1px solid var(--line);background:var(--card);box-shadow:0 3px 0 rgba(19,34,56,.04)}.metric strong{display:block;font:700 2rem/1 ui-serif,Georgia,serif}.metric span{color:var(--muted);font-size:.82rem}.scope-grid,.two-col{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.panel{padding:1.15rem;border:1px solid var(--line);background:var(--card)}.pill,.tag{display:inline-flex;align-items:center;margin:.18rem .25rem .18rem 0;padding:.23rem .55rem;border-radius:999px;background:var(--soft);font-size:.78rem;font-weight:800}.tag.verdict{background:#d9ebe8;color:#0d5753}.tag.warn{background:#f6dfd5;color:#7a2716}.top-pick{padding:1.35rem;border:1px solid #b9cfc9;border-left:6px solid var(--teal);background:#f7fcfa}.top-pick .rank{color:var(--teal);font-weight:900}.topology{display:grid;grid-template-columns:minmax(0,1fr) auto minmax(0,1fr);align-items:center;gap:.8rem;margin:1rem 0}.owner-box{height:100%;padding:.8rem;border:1px solid var(--line);background:var(--card)}.arrow{font-size:1.6rem;color:var(--teal)}.toolbar{display:grid;grid-template-columns:2fr 1fr;gap:.7rem;margin:1rem 0}.toolbar input,.toolbar select{width:100%;padding:.75rem;border:1px solid var(--line);border-radius:0;background:#fff;font:inherit}.finding{margin:1rem 0;padding:1.3rem;border:1px solid var(--line);background:var(--card);box-shadow:0 7px 18px rgba(19,34,56,.05)}.finding-head{display:flex;justify-content:space-between;gap:1rem;align-items:flex-start}.finding-id{color:var(--muted);font:700 .78rem/1.2 ui-monospace,SFMono-Regular,Menlo,monospace}.outcome{font-size:1.04rem}.label{display:block;margin:1rem 0 .35rem;color:var(--muted);font-size:.73rem;font-weight:900;letter-spacing:.09em;text-transform:uppercase}.compact{margin:.35rem 0;padding-left:1.2rem}.compact li{margin:.25rem 0}.matrix{width:100%;border-collapse:collapse;font-size:.88rem}.matrix th,.matrix td{text-align:left;vertical-align:top;padding:.45rem;border-bottom:1px solid var(--line)}.matrix th{color:var(--muted)}.evidence{list-style:none;padding:0}.evidence li{margin:.45rem 0;padding:.7rem;border-left:3px solid #aebdb8;background:#f4f7f4}.decision{padding:.85rem;border-left:4px solid var(--gold);background:#fff8e8}.risk{padding:.8rem;background:#fff3ee}.empty{color:var(--muted);font-style:italic}.seed-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:.7rem}.seed{padding:.85rem;border:1px solid var(--line);background:var(--card)}details{margin-top:.8rem}summary{cursor:pointer;font-weight:800}.source-note{padding:1rem;border:1px dashed var(--teal);background:#f4fbf9}.footer{padding:1.2rem 0 3rem;color:var(--muted);border-top:1px solid var(--line)}[hidden]{display:none!important}
html,body,.toc{overflow-x:hidden}
@media(max-width:720px){.hero{padding-top:3rem}.scope-grid,.two-col,.topology,.toolbar{grid-template-columns:1fr}.arrow{transform:rotate(90deg);justify-self:center}.finding-head{display:block}.shell{width:min(100% - 1rem,1180px)}.matrix{display:block;overflow-x:auto}}
@media print{body{background:#fff}.toc,.toolbar{display:none}.shell{width:100%}.hero{padding:1rem 0;background:#fff}.finding,.panel,.metric{box-shadow:none;break-inside:avoid}details>summary{display:none}details>*{display:block!important}}
</style></head><body>`)
	out.WriteString("<header class=\"hero\"><div class=\"shell\"><p class=\"eyebrow\">Read-only decision evidence</p><h1>SPEC governance audit</h1><p class=\"lede\">Where observable contracts are repeated across harnesses or implementations, where a neutral owner should replace copies, and which similar-looking specifications must stay separate.</p><dl class=\"snapshot\">")
	renderSnapshotItem(out, "Repository", audit.Snapshot.Repository, false)
	renderSnapshotItem(out, "Pinned revision", audit.Snapshot.Revision, true)
	renderSnapshotItem(out, "Revision committed", audit.Snapshot.RevisionCommittedAt, false)
	if audit.Snapshot.GeneratedAt != "" {
		renderSnapshotItem(out, "Report generated", audit.Snapshot.GeneratedAt, false)
	}
	if audit.Snapshot.ComparisonRevision != "" {
		renderSnapshotItem(out, "Comparison only", audit.Snapshot.ComparisonRevision, true)
	}
	out.WriteString("</dl><p class=\"warning\"><strong>Selection precedes change.</strong> Automated validation pins evidence resolved by Git and validates required decision structure; it does not independently authenticate source provenance, prove a semantic relationship, select a canonical owner, or authorize consolidation, deletion, or product mutation.</p></div></header>")
	out.WriteString("<nav class=\"toc\" aria-label=\"Report sections\"><div class=\"shell\"><a href=\"#summary\">Corpus</a><a href=\"#top\">Top decision</a><a href=\"#candidates\">Candidates</a><a href=\"#boundaries\">Keep separate</a><a href=\"#method\">Method</a><a href=\"#limits\">Limits</a></div></nav><main class=\"shell\">")

	out.WriteString("<section id=\"summary\"><h2>Corpus and scope</h2><div class=\"metrics\">")
	renderMetric(out, audit.Summary.SpecFiles, "tracked SPEC files")
	renderMetric(out, audit.Summary.Requirements, "identified EARS requirements")
	renderMetric(out, audit.Summary.Diagnostics, "collector diagnostics")
	renderMetric(out, audit.Summary.CandidateCount, "consolidation candidates")
	for _, verdict := range sortedKeys(audit.Summary.ByVerdict) {
		renderMetric(out, audit.Summary.ByVerdict[verdict], verdict)
	}
	out.WriteString("</div><div class=\"scope-grid\" style=\"margin-top:1rem\"><div class=\"panel\"><span class=\"label\">Roots</span>")
	for _, root := range audit.Scope.Roots {
		fmt.Fprintf(out, "<code>%s</code> ", esc(root))
	}
	out.WriteString("<span class=\"label\">Pinned active members</span>")
	for _, member := range audit.Scope.ActiveMembers {
		fmt.Fprintf(out, "<span class=\"pill\">%s</span>", esc(member))
	}
	out.WriteString("</div><div class=\"panel\"><span class=\"label\">Exclusions</span>")
	if len(audit.Scope.Excluded) == 0 {
		out.WriteString("<p class=\"empty\">No paths were excluded.</p>")
	} else {
		out.WriteString("<ul class=\"compact\">")
		for _, exclusion := range audit.Scope.Excluded {
			fmt.Fprintf(out, "<li><code>%s</code> — %s</li>", esc(exclusion.Path), esc(exclusion.Reason))
		}
		out.WriteString("</ul>")
	}
	out.WriteString("</div></div></section>")

	out.WriteString("<section id=\"top\"><h2>Top maintainer decision</h2>")
	if len(candidates) == 0 {
		out.WriteString("<p class=\"empty\">No supported consolidation candidate was selected.</p>")
	} else {
		top := candidates[0]
		fmt.Fprintf(out, "<div class=\"top-pick\"><span class=\"rank\">Rank %d · %s</span><h3><a href=\"#%s\">%s</a></h3><p>%s</p>", top.Rank, esc(top.ID), esc(top.ID), esc(top.Title), esc(top.SharedOutcome))
		renderTopology(out, top)
		fmt.Fprintf(out, "<p class=\"decision\"><strong>Decision requested:</strong> %s</p></div>", esc(top.Decision))
	}
	out.WriteString("</section>")

	out.WriteString("<section id=\"candidates\"><h2>Ranked consolidation candidates</h2><p class=\"section-intro\">Lexical seeds are not verdicts. Every card below preserves ownership, applicability, exact source evidence, BDD impact, risks, and the maintainer choice.</p><div class=\"toolbar\"><input id=\"filter\" type=\"search\" aria-label=\"Filter findings\" placeholder=\"Filter by path, requirement, owner, or outcome\"><select id=\"verdict\" aria-label=\"Filter by verdict\"><option value=\"\">All verdicts</option>")
	for _, verdict := range sortedKeys(audit.Summary.ByVerdict) {
		fmt.Fprintf(out, "<option value=\"%s\">%s</option>", esc(verdict), esc(verdict))
	}
	out.WriteString("</select></div><div id=\"findings\">")
	for _, finding := range candidates {
		renderFinding(out, finding)
	}
	out.WriteString("</div></section>")

	out.WriteString("<section id=\"boundaries\"><h2>Keep-separate controls</h2><p class=\"section-intro\">These near misses constrain the audit: shared vocabulary, identifiers, or lifecycle shape do not erase adapter, platform, or domain boundaries.</p>")
	if len(audit.NonCandidates) == 0 {
		out.WriteString("<p class=\"empty\">None recorded.</p>")
	}
	for _, finding := range audit.NonCandidates {
		renderFinding(out, finding)
	}
	out.WriteString("</section>")

	out.WriteString("<section id=\"method\"><h2>Method and reproducibility</h2><div class=\"two-col\"><div class=\"panel\"><span class=\"label\">Collector</span><p><code>")
	fmt.Fprintf(out, "%s", esc(audit.Methodology.Collector))
	out.WriteString("</code></p><span class=\"label\">Semantic review</span><p>")
	fmt.Fprintf(out, "%s", esc(audit.Methodology.SemanticReview))
	out.WriteString("</p><span class=\"label\">Git evidence trust boundary</span><p>")
	fmt.Fprintf(out, "%s", esc(audit.Methodology.GitEvidenceTrust))
	out.WriteString("</p><span class=\"label\">Git-resolved trust inputs</span><ul class=\"compact\">")
	fmt.Fprintf(out, "<li>Canonical executable: <code>%s</code></li>", esc(audit.Methodology.GitTrustInputs.Executable))
	fmt.Fprintf(out, "<li>Worktree root: <code>%s</code></li>", esc(audit.Methodology.GitTrustInputs.WorkTreeRoot))
	fmt.Fprintf(out, "<li>Repository Git directory: <code>%s</code></li>", esc(audit.Methodology.GitTrustInputs.GitDir))
	fmt.Fprintf(out, "<li>Common directory: <code>%s</code></li>", esc(audit.Methodology.GitTrustInputs.CommonDir))
	fmt.Fprintf(out, "<li>Object directory: <code>%s</code></li>", esc(audit.Methodology.GitTrustInputs.ObjectDir))
	if len(audit.Methodology.GitTrustInputs.AlternateObjectDirs) == 0 {
		out.WriteString("<li>Alternate object directories: none</li>")
	} else {
		for _, alternate := range audit.Methodology.GitTrustInputs.AlternateObjectDirs {
			fmt.Fprintf(out, "<li>Alternate object directory: <code>%s</code></li>", esc(alternate))
		}
	}
	out.WriteString("</ul><span class=\"label\">Seed kinds</span>")
	for _, kind := range audit.Methodology.SeedKinds {
		fmt.Fprintf(out, "<span class=\"pill\">%s</span>", esc(kind))
	}
	out.WriteString("</div><div class=\"panel\"><span class=\"label\">Reproduce</span><ol class=\"compact\">")
	for _, command := range audit.Methodology.Reproduce {
		fmt.Fprintf(out, "<li><code>%s</code></li>", esc(command))
	}
	out.WriteString("</ol></div></div>")
	renderSeedSummary(out, seeds, inventory)
	proof := "Schema validation was performed without a freshly recomputed Git-resolved inventory."
	if inventory != nil {
		proof = "The supplied inventory was recomputed through Git from the pinned revision before evidence and HTML rendering were accepted. This pins evidence to Git's resolved view but does not attest the trusted repository metadata or object storage."
	}
	fmt.Fprintf(out, "<p class=\"source-note\"><strong>Source disclosure.</strong> %s Structured JSON remains the findings source; this HTML is a complete view, not a second classification store.</p></section>", esc(proof))

	out.WriteString("<section id=\"limits\"><h2>Limitations</h2>")
	if len(audit.Limitations) == 0 {
		out.WriteString("<p class=\"empty\">No limitations recorded.</p>")
	} else {
		renderStringList(out, audit.Limitations, false)
	}
	out.WriteString("</section></main><footer class=\"footer\"><div class=\"shell\">Self-contained offline report · no external scripts, fonts, styles, or data sources.</div></footer>")
	out.WriteString(`<script>
const query=document.getElementById('filter');const verdict=document.getElementById('verdict');
function applyFilters(){const q=query.value.trim().toLowerCase();const v=verdict.value;for(const card of document.querySelectorAll('#findings .finding')){card.hidden=(q&&!card.innerText.toLowerCase().includes(q))||(v&&card.dataset.verdict!==v)}}
query.addEventListener('input',applyFilters);verdict.addEventListener('change',applyFilters);
</script></body></html>
`)
	if err := out.Err(); err != nil {
		return "", fmt.Errorf("rendered HTML exceeds %d-byte artifact output limit: %w", limit, err)
	}
	return out.String(), nil
}

func renderSnapshotItem(out *boundedHTMLBuilder, label, value string, code bool) {
	fmt.Fprintf(out, "<div><dt>%s</dt><dd>", esc(label))
	if code {
		fmt.Fprintf(out, "<code>%s</code>", esc(value))
	} else {
		fmt.Fprintf(out, "%s", esc(value))
	}
	out.WriteString("</dd></div>")
}

func renderMetric(out *boundedHTMLBuilder, value int, label string) {
	fmt.Fprintf(out, "<div class=\"metric\"><strong>%d</strong><span>%s</span></div>", value, esc(label))
}

func renderTopology(out *boundedHTMLBuilder, finding finding) {
	out.WriteString("<div class=\"topology\"><div class=\"owner-box\"><span class=\"label\">Current owners</span><ul class=\"compact\">")
	for _, owner := range finding.CurrentOwners {
		fmt.Fprintf(out, "<li><code>%s</code><br>%s</li>", esc(owner.Path), esc(owner.Rationale))
	}
	out.WriteString("</ul></div><div class=\"arrow\" aria-hidden=\"true\">→</div><div class=\"owner-box\"><span class=\"label\">Proposed canonical owner</span>")
	if finding.ProposedOwner == nil {
		out.WriteString("<p class=\"empty\">No consolidation owner.</p>")
	} else {
		fmt.Fprintf(out, "<p><code>%s</code></p><span class=\"pill\">%s owner</span><p>%s</p><p><strong>Neutrality:</strong> %s</p>", esc(finding.ProposedOwner.Path), esc(finding.ProposedOwner.State), esc(finding.ProposedOwner.Rationale), esc(finding.ProposedOwner.NeutralityRationale))
	}
	out.WriteString("</div></div>")
}

func renderFinding(out *boundedHTMLBuilder, finding finding) {
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
			renderEvidence(out, entry.Evidence, "applicability-evidence")
			out.WriteString("</td></tr>")
		}
		out.WriteString("</tbody></table>")
	}
	out.WriteString("</div></div><details open><summary>Exact source evidence</summary>")
	renderEvidence(out, finding.Evidence, "")
	out.WriteString("</details><div class=\"two-col\"><div><span class=\"label\">BDD consequence</span>")
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
	if finding.BDD.SharedContractFeature != "" {
		fmt.Fprintf(out, "<p><strong>Shared contract feature:</strong> <code>%s</code></p>", esc(finding.BDD.SharedContractFeature))
	}
	out.WriteString("</div><div><span class=\"label\">Ordered recommendation</span>")
	renderStringList(out, finding.Recommendation, true)
	out.WriteString("</div></div>")
	renderOwnershipPlan(out, finding)
	fmt.Fprintf(out, "<p class=\"risk\"><strong>Risk:</strong> %s</p><p class=\"decision\"><strong>Maintainer decision:</strong> %s</p>", esc(finding.Risk), esc(finding.Decision))
	out.WriteString("<details><summary>Finding limitations</summary>")
	if len(finding.Limitations) == 0 {
		out.WriteString("<p class=\"empty\">None recorded.</p>")
	} else {
		renderStringList(out, finding.Limitations, false)
	}
	out.WriteString("</details></article>")
}

func renderOwnershipPlan(out *boundedHTMLBuilder, finding finding) {
	if finding.OwnershipPlan == nil {
		return
	}
	out.WriteString("<details open><summary>Pending ownership and preservation plan</summary>")
	fmt.Fprintf(out, "<p class=\"decision\"><strong>Approval:</strong> %s. This plan does not authorize a change or file deletion.</p>", esc(finding.OwnershipPlan.Approval))
	for _, owner := range finding.OwnershipPlan.CurrentOwners {
		fmt.Fprintf(out, "<div class=\"panel\"><p><code>%s</code> <span class=\"tag\">%s</span></p><p>%s</p>", esc(owner.Path), esc(owner.Action), esc(owner.Rationale))
		if owner.Preservation != nil {
			out.WriteString("<span class=\"label\">Requirement preservation</span><ul class=\"compact\">")
			for _, mapping := range owner.Preservation.Requirements {
				fmt.Fprintf(out, "<li><code>%s:%d %s</code> → <code>%s</code> (%s; %s)</li>", esc(mapping.Source.Path), mapping.Source.Line, esc(mapping.Source.RequirementID), esc(mapping.TargetID), esc(mapping.TargetState), esc(mapping.Strategy))
			}
			out.WriteString("</ul><span class=\"label\">BDD preservation</span><ul class=\"compact\">")
			for _, mapping := range owner.Preservation.BDD {
				fmt.Fprintf(out, "<li><code>%s</code>: <code>%s</code> → <code>%s</code></li>", esc(mapping.Feature), esc(mapping.SourceOwner), esc(mapping.TargetOwner))
			}
			out.WriteString("</ul><span class=\"label\">Copied applicability</span>")
			fmt.Fprintf(out, "<p><span class=\"pill\">%s</span></p>", esc(owner.Preservation.ApplicabilityBasis))
			if len(owner.Preservation.Applicability) == 0 {
				out.WriteString("<p class=\"empty\">No active-member matrix.</p>")
			} else {
				out.WriteString("<table class=\"matrix\"><thead><tr><th>Member</th><th>Disposition</th></tr></thead><tbody>")
				for _, entry := range owner.Preservation.Applicability {
					fmt.Fprintf(out, "<tr><td><code>%s</code></td><td>%s</td></tr>", esc(entry.Member), esc(entry.Disposition))
				}
				out.WriteString("</tbody></table>")
			}
		}
		out.WriteString("</div>")
	}
	out.WriteString("</details>")
}

// renderEvidence renders every supplied record in order. Applicability evidence
// is not a summary: the same pinned requirement may support multiple members
// and each such claim must remain independently visible in the offline report.
func renderEvidence(out *boundedHTMLBuilder, items []evidence, class string) {
	classes := "evidence"
	if class != "" {
		classes += " " + class
	}
	fmt.Fprintf(out, "<ul class=\"%s\">", classes)
	for _, item := range items {
		fmt.Fprintf(out, "<li><code>%s:%d</code>", esc(item.Path), item.Line)
		if item.RequirementID != "" {
			fmt.Fprintf(out, " <span class=\"pill\">%s</span>", esc(item.RequirementID))
		}
		fmt.Fprintf(out, "<br>%s</li>", esc(item.Excerpt))
	}
	out.WriteString("</ul>")
}

func renderStringList(out *boundedHTMLBuilder, items []string, ordered bool) {
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

func renderSeedSummary(out *boundedHTMLBuilder, seeds []seed, inventory *report) {
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
		for _, feature := range inventory.Features {
			for _, diagnostic := range feature.Diagnostics {
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
	out.WriteString("</div><p class=\"section-intro\">Seeds identify review leads only. The pinned Git-resolved inventory JSON retains every seed and diagnostic record; the semantic ledger records the bounded verdicts.</p>")
}

var errHTMLArtifactLimit = errors.New("HTML artifact output limit exceeded")

type boundedHTMLBuilder struct {
	builder strings.Builder
	limit   int64
	err     error
}

func newBoundedHTMLBuilder(limit int64) *boundedHTMLBuilder {
	return &boundedHTMLBuilder{limit: limit}
}

func (out *boundedHTMLBuilder) Write(data []byte) (int, error) {
	if out.err != nil {
		return 0, out.err
	}
	if int64(len(data)) > out.limit-int64(out.builder.Len()) {
		out.err = errHTMLArtifactLimit
		return 0, out.err
	}
	return out.builder.Write(data)
}

func (out *boundedHTMLBuilder) WriteString(text string) {
	if out.err != nil {
		return
	}
	if int64(len(text)) > out.limit-int64(out.builder.Len()) {
		out.err = errHTMLArtifactLimit
		return
	}
	_, _ = out.builder.WriteString(text)
}

func (out *boundedHTMLBuilder) String() string { return out.builder.String() }
func (out *boundedHTMLBuilder) Err() error     { return out.err }

type escapedHTMLText string

func esc(text string) escapedHTMLText { return escapedHTMLText(text) }

// Format escapes directly into the bounded destination. It deliberately avoids
// allocating a fully escaped copy of an untrusted report field before the
// artifact ceiling can stop rendering.
func (text escapedHTMLText) Format(state fmt.State, verb rune) {
	if verb != 's' {
		_, _ = io.WriteString(state, "%!"+string(verb)+"(escapedHTMLText)")
		return
	}
	write := func(value string) bool {
		_, err := io.WriteString(state, value)
		return err == nil
	}
	raw := string(text)
	start := 0
	for index := 0; index < len(raw); index++ {
		var replacement string
		switch raw[index] {
		case '&':
			replacement = "&amp;"
		case '\'':
			replacement = "&#39;"
		case '<':
			replacement = "&lt;"
		case '>':
			replacement = "&gt;"
		case '"':
			replacement = "&#34;"
		default:
			continue
		}
		if start < index {
			if !write(raw[start:index]) {
				return
			}
		}
		if !write(replacement) {
			return
		}
		start = index + 1
	}
	if start < len(raw) {
		_ = write(raw[start:])
	}
}
