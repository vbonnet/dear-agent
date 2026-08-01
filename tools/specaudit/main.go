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
	"go/ast"
	"go/parser"
	"go/token"
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

	gherkin "github.com/cucumber/gherkin/go/v26"
	"github.com/cucumber/messages/go/v21"
	"github.com/vbonnet/dear-agent/internal/earslint"
)

const schemaVersion = "spec-audit/v3"

const gitEvidenceTrustDisclosure = "The collector trusts the PATH-selected Git executable, repository Git metadata, the common object store, and configured object alternates. It disables replacement objects and lazy fetching and resolves evidence from the pinned commit through Git; it does not independently authenticate source provenance or object-store integrity."
const runtimeStatusUnverified = "UNVERIFIED"

const (
	activeHarnessUnavailableLimitation     = "Active harness inventory was unavailable at the pinned revision."
	activeHarnessUnparseableLimitation     = "Active harness inventory could not be parsed at the pinned revision."
	deprecatedHarnessUnparseableLimitation = "Deprecated harness inventory could not be parsed at the pinned revision."
	adapterCatalogIncompleteLimitation     = "Adapter-scope catalog exceeded its bounded or unambiguous pinned metadata contract."
)

const (
	centralHarnessRegistryPath     = "agm/internal/harnessregistry/registry.go"
	inPackageHarnessRegistryPath   = "agm/internal/agent/harnesses.go"
	harnessAliasSourcePath         = "agm/internal/agent/validate.go"
	harnessConfigSurfaceSourcePath = "agm/internal/configdirparity/coverage.go"
	marketplaceSurfaceSourcePath   = "agm/internal/marketplaceparity/coverage.go"
	openAIAdapterSourcePath        = "agm/internal/agent/openai_adapter.go"
	maxGitOutputBytes              = 16 * 1024 * 1024
	maxGitInputBytes               = 1 * 1024 * 1024
	maxReportInputBytes            = 32 * 1024 * 1024
	maxCorpusBytes                 = 64 * 1024 * 1024
	maxArtifactOutputBytes         = 64 * 1024 * 1024
	maxGitCommandDuration          = 30 * time.Second
	maxGitWaitDelay                = 250 * time.Millisecond
	maxInventoryDuration           = 60 * time.Second
	maxInventoryFiles              = 10_000
	maxJSONDepth                   = 128
	maxJSONTokens                  = 1_000_000
	maxJSONElements                = 250_000
	maxJSONAggregateStringBytes    = 16 * 1024 * 1024
	maxJSONStringBytes             = 64 * 1024
	maxJSONCollectionItems         = 10_000
	maxReportFindings              = 4096
	maxReportLimitations           = 1024
	maxReportLimitationBytes       = 16 * 1024
	maxBatchHeaderBytes            = 128
	maxGitExecutableIdentityBytes  = 128 * 1024 * 1024
	maxAlternateConfigBytes        = 1024 * 1024
	maxAlternateObjectRoutes       = 1024
	maxAdapterScopes               = 64
	maxAdapterScopeNames           = 32
	maxAdapterScopeEvidence        = 16
	maxAdapterCatalogSourceBytes   = 256 * 1024
	maxFeatureBytes                = 256 * 1024
	maxFeatureLineBytes            = 16 * 1024
	maxFeatureStructuralTokens     = 4096
	maxFeatureTags                 = 2048
	maxFeatureTagBytes             = 128 * 1024
	maxFeatureComments             = 4096
	maxFeatureCommentBytes         = 192 * 1024
	maxFeatureDescriptionLines     = 4096
	maxFeatureDescriptionBytes     = 192 * 1024
	maxFeatureDocStrings           = 256
	maxFeatureDocStringSeparators  = 2 * maxFeatureDocStrings
	maxFeatureDocStringLines       = 4096
	maxFeatureDocStringBytes       = 192 * 1024
	maxFeatureScenarios            = 256
	maxScenarioSteps               = 256
	maxScenarioExamples            = 32
	maxScenarioTableRows           = 256
	maxTableCellsPerRow            = 64
	maxScenarioCases               = 128
	maxScenarioOutcomes            = 128
	maxScenarioNameBytes           = 512
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

// markdownFenceState tracks the exact opening delimiter so example blocks
// cannot be ended by a shorter or different delimiter. This intentionally
// models only the fenced-block boundary needed by the audit parsers.
type markdownFenceState struct {
	delimiter  byte
	openingRun int
}

// consume reports whether line is a fence boundary. An opening fence has a
// run of at least three backticks or tildes; a closing fence must use that
// same delimiter and be at least as long as its opener.
func (state *markdownFenceState) consume(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}
	delimiter := trimmed[0]
	if delimiter != '`' && delimiter != '~' {
		return false
	}
	run := 0
	for run < len(trimmed) && trimmed[run] == delimiter {
		run++
	}
	if run < 3 {
		return false
	}
	if state.openingRun == 0 {
		state.delimiter = delimiter
		state.openingRun = run
		return true
	}
	// CommonMark closing fences cannot carry an info string or other content.
	// Without this check, a code example line such as ````go could expose the
	// following example text as normative input.
	if delimiter != state.delimiter || run < state.openingRun || run != len(trimmed) {
		return false
	}
	state.delimiter = 0
	state.openingRun = 0
	return true
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
	Roots         []string       `json:"roots"`
	Excluded      []exclusion    `json:"excluded"`
	ActiveMembers []string       `json:"active_members"`
	AdapterScopes []adapterScope `json:"adapter_scopes"`
}

// adapterScope is a bounded, pinned catalog entry used only to reject a
// harness- or adapter-local proposed normative owner. It is evidence, not a
// declaration that every implementation directory is adapter-specific.
type adapterScope struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Lifecycle string          `json:"lifecycle"`
	Names     []string        `json:"names"`
	Evidence  []scopeEvidence `json:"evidence"`
}

type scopeEvidence struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Excerpt string `json:"excerpt"`
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
	RuntimeStatus    string         `json:"runtime_status"`
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
	Path         string            `json:"path"`
	SHA256       string            `json:"sha256"`
	RelatedSpecs []string          `json:"related_specs"`
	Scenarios    []featureScenario `json:"scenarios"`
	Diagnostics  []diagnostic      `json:"diagnostics,omitempty"`
}

// featureScenario retains only the bounded Gherkin structure needed to prove
// that a selected shared contract is executable across the declared members.
// It is deliberately not a general-purpose Gherkin AST.
type featureScenario struct {
	Line                  int                  `json:"line"`
	Name                  string               `json:"name"`
	Kind                  string               `json:"kind"`
	Outcomes              []scenarioOutcome    `json:"outcomes"`
	MemberColumn          string               `json:"member_column,omitempty"`
	UsesMemberPlaceholder bool                 `json:"uses_member_placeholder,omitempty"`
	MemberCases           []scenarioMemberCase `json:"member_cases"`
}

type scenarioOutcome struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type scenarioMemberCase struct {
	Line   int    `json:"line"`
	Member string `json:"member"`
	Source string `json:"source"`
}

type requirement struct {
	ID      string `json:"id"`
	Line    int    `json:"line"`
	Body    string `json:"body"`
	Excerpt string `json:"excerpt"`
}

type requirementKey struct {
	line int
	id   string
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
	Features               []string     `json:"features"`
	SharedContractFeature  string       `json:"shared_contract_feature,omitempty"`
	SharedContractScenario *scenarioRef `json:"shared_contract_scenario,omitempty"`
	Consequence            string       `json:"consequence"`
}

type scenarioRef struct {
	Line int    `json:"line"`
	Name string `json:"name"`
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
	fmt.Fprintf(stdout, "specaudit: valid %s report\n", schemaVersion)
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
		rawBody := bodies[blob.oid]
		if !utf8.Valid(rawBody) {
			return report{}, fmt.Errorf("pinned textual blob %q is not valid UTF-8", blob.path)
		}
		body := string(rawBody)
		path := blob.path
		switch {
		case filepath.Base(path) == "SPEC.md":
			files = append(files, parseSpec(path, body))
		case strings.HasSuffix(path, ".feature"):
			feature, refs, parseErr := parseFeature(ctx, path, body)
			if parseErr != nil {
				return report{}, parseErr
			}
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
	adapterScopes, active, activeLimitations, projectionErr := adapterScopesFromPinnedBodiesContext(ctx, blobs, bodies)
	if projectionErr != nil {
		if err := inventoryContextError(ctx, limits.wallTime); err != nil {
			return report{}, err
		}
		return report{}, fmt.Errorf("project pinned adapter catalog: %w", projectionErr)
	}
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
		Scope:         scope{Roots: []string{"."}, Excluded: []exclusion{}, ActiveMembers: active, AdapterScopes: adapterScopes},
		Summary:       summary{SpecFiles: len(files), Requirements: requirementCount, Diagnostics: diagnosticCount, CandidateCount: 0, ByVerdict: map[string]int{}},
		Methodology: methodology{
			Collector:        "go run ./tools/specaudit inventory",
			SeedKinds:        []string{"exact-body", "duplicate-id", "shared-bdd", "identical-file", "harness-terminology"},
			SemanticReview:   "Seeds are bounded lexical leads; harness terminology records at most one matching requirement per SPEC path, and source plus BDD review determines every finding verdict.",
			RuntimeStatus:    runtimeStatusUnverified,
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
		if !selectedPinnedPathBytes(rawPath) {
			continue
		}
		if !utf8.Valid(rawPath) {
			return nil, errors.New("selected pinned Git path is not valid UTF-8")
		}
		path := string(rawPath)
		size, err := pinnedGitBlobSize(fields, path)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(path, ".feature") && size > maxFeatureBytes {
			return nil, fmt.Errorf("pinned BDD feature %q exceeds %d-byte per-file limit", path, maxFeatureBytes)
		}
		if selectedAdapterCatalogSourcePath(path) && size > maxAdapterCatalogSourceBytes {
			return nil, fmt.Errorf("pinned adapter-catalog source %q exceeds %d-byte per-file limit", path, maxAdapterCatalogSourceBytes)
		}
		if err := budget.consumeFile(path, size); err != nil {
			return nil, err
		}
		blobs = append(blobs, pinnedBlob{path: filepath.ToSlash(path), oid: fields[2], size: size})
	}
	return blobs, nil
}

func selectedPinnedPathBytes(path []byte) bool {
	if bytes.Equal(path, []byte("SPEC.md")) || bytes.HasSuffix(path, []byte("/SPEC.md")) || bytes.HasSuffix(path, []byte(".feature")) {
		return true
	}
	for _, selected := range []string{
		centralHarnessRegistryPath,
		inPackageHarnessRegistryPath,
		harnessAliasSourcePath,
		harnessConfigSurfaceSourcePath,
		marketplaceSurfaceSourcePath,
		openAIAdapterSourcePath,
	} {
		if bytes.Equal(path, []byte(selected)) {
			return true
		}
	}
	return false
}

func selectedAdapterCatalogSourcePath(path string) bool {
	switch filepath.ToSlash(path) {
	case centralHarnessRegistryPath,
		inPackageHarnessRegistryPath,
		harnessAliasSourcePath,
		harnessConfigSurfaceSourcePath,
		marketplaceSurfaceSourcePath,
		openAIAdapterSourcePath:
		return true
	default:
		return false
	}
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

//nolint:gocyclo // Batch authentication keeps each bounded Git framing and text-integrity check explicit.
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
		if !utf8.Valid(body) {
			return nil, fmt.Errorf("pinned textual blob %q is not valid UTF-8", blob.path)
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

type catalogValue struct {
	value         string
	evidence      scopeEvidence
	deprecated    bool
	hasDeprecated bool
}

func adapterScopesFromPinnedBodies(blobs []pinnedBlob, bodies map[string][]byte) ([]adapterScope, []string, []string) {
	scopes, active, limitations, err := adapterScopesFromPinnedBodiesContext(context.Background(), blobs, bodies)
	if err != nil {
		return []adapterScope{}, []string{}, []string{adapterCatalogIncompleteLimitation}
	}
	return scopes, active, limitations
}

//nolint:gocyclo // Catalog assembly keeps each pinned metadata source, context check, and bound visible.
func adapterScopesFromPinnedBodiesContext(ctx context.Context, blobs []pinnedBlob, bodies map[string][]byte) ([]adapterScope, []string, []string, error) {
	if err := ctx.Err(); err != nil {
		return []adapterScope{}, []string{}, nil, err
	}
	registryPath := centralHarnessRegistryPath
	registryBody, available := pinnedBodyForPath(blobs, bodies, registryPath)
	if !available {
		registryPath = inPackageHarnessRegistryPath
		registryBody, available = pinnedBodyForPath(blobs, bodies, registryPath)
	}
	if !available {
		return []adapterScope{}, []string{}, []string{activeHarnessUnavailableLimitation}, nil
	}
	if !catalogGoSourceWithinLimit(registryBody) {
		return []adapterScope{}, []string{}, []string{adapterCatalogIncompleteLimitation}, nil
	}
	if err := ctx.Err(); err != nil {
		return []adapterScope{}, []string{}, nil, err
	}
	activeValues, activeOK := goStringSliceValues(registryPath, registryBody, "activeHarnesses")
	if err := ctx.Err(); err != nil {
		return []adapterScope{}, []string{}, nil, err
	}
	if !activeOK {
		return []adapterScope{}, []string{}, []string{adapterCatalogIncompleteLimitation}, nil
	}
	if err := ctx.Err(); err != nil {
		return []adapterScope{}, []string{}, nil, err
	}
	deprecatedValues, deprecatedOK := goStringSliceValues(registryPath, registryBody, "deprecatedHarnesses")
	if err := ctx.Err(); err != nil {
		return []adapterScope{}, []string{}, nil, err
	}
	if !deprecatedOK {
		return []adapterScope{}, []string{}, []string{adapterCatalogIncompleteLimitation}, nil
	}
	scopesByID := map[string]*adapterScope{}
	catalogOverflow := false
	addScope := func(item catalogValue, lifecycle string) {
		id := item.value
		if id == "" || strings.TrimSpace(id) != id || len([]byte(id)) > maxScenarioNameBytes {
			catalogOverflow = true
			return
		}
		if len(scopesByID) >= maxAdapterScopes && scopesByID[id] == nil {
			catalogOverflow = true
			return
		}
		scope := scopesByID[id]
		if scope == nil {
			scope = &adapterScope{ID: id, Kind: "harness", Lifecycle: lifecycle, Names: []string{id}, Evidence: []scopeEvidence{}}
			scopesByID[id] = scope
		}
		if scope.Lifecycle != lifecycle || !appendScopeEvidence(scope, item.evidence) {
			catalogOverflow = true
		}
	}
	for _, item := range activeValues {
		addScope(item, "active")
	}
	for _, item := range deprecatedValues {
		addScope(item, "deprecated")
	}
	aliasPath := harnessAliasSourcePath
	aliasBody, aliasAvailable := pinnedBodyForPath(blobs, bodies, aliasPath)
	if registryPath == centralHarnessRegistryPath && strings.Contains(registryBody, "func NormalizeHarnessName") {
		aliasPath = registryPath
		aliasBody = registryBody
		aliasAvailable = true
	}
	if aliasAvailable {
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
		aliasesByCanonical, parsed := goNormalizeHarnessAliases(aliasPath, aliasBody)
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
		if !parsed {
			catalogOverflow = true
		}
		for canonical, aliases := range aliasesByCanonical {
			scope := scopesByID[canonical]
			if scope == nil {
				catalogOverflow = true
				continue
			}
			for _, alias := range aliases {
				if !appendScopeName(scope, alias.value) || !appendScopeEvidence(scope, alias.evidence) {
					catalogOverflow = true
				}
			}
		}
	} else {
		catalogOverflow = true
	}
	if body, ok := pinnedBodyForPath(blobs, bodies, harnessConfigSurfaceSourcePath); ok {
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
		configByCanonical, parsed := goHarnessConfigNames(harnessConfigSurfaceSourcePath, body)
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
		if !parsed {
			catalogOverflow = true
		}
		for canonical, names := range configByCanonical {
			scope := scopesByID[canonical]
			if scope == nil {
				catalogOverflow = true
				continue
			}
			for _, name := range names {
				deprecated := scope.Lifecycle == "deprecated"
				if !name.hasDeprecated || name.deprecated != deprecated || !appendScopeName(scope, name.value) || !appendScopeEvidence(scope, name.evidence) {
					catalogOverflow = true
				}
			}
		}
		for id, scope := range scopesByID {
			if scope.Kind == "harness" && len(configByCanonical[id]) != 1 {
				catalogOverflow = true
			}
		}
	} else {
		catalogOverflow = true
	}
	if body, ok := pinnedBodyForPath(blobs, bodies, marketplaceSurfaceSourcePath); ok {
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
		claudeRoot, neutralRoot, parsed := goMarketplaceCatalogRoots(marketplaceSurfaceSourcePath, body)
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
		claudeScope := scopesByID["claude-code"]
		if !parsed || claudeScope == nil || claudeRoot.value == neutralRoot.value || !appendScopeName(claudeScope, claudeRoot.value) || !appendScopeEvidence(claudeScope, claudeRoot.evidence) {
			catalogOverflow = true
		}
		for _, scope := range scopesByID {
			if containsString(scope.Names, neutralRoot.value) {
				catalogOverflow = true
			}
		}
	} else {
		catalogOverflow = true
	}
	if body, ok := pinnedBodyForPath(blobs, bodies, openAIAdapterSourcePath); ok {
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
		if name, evidence, parsed := goAdapterName(openAIAdapterSourcePath, body, "OpenAIAdapter"); parsed {
			if name == "" || len([]byte(name)) > maxScenarioNameBytes || scopesByID[name] != nil || len(scopesByID) >= maxAdapterScopes {
				catalogOverflow = true
			} else {
				scopesByID[name] = &adapterScope{ID: name, Kind: "compatibility-adapter", Lifecycle: "compatibility", Names: []string{name}, Evidence: []scopeEvidence{evidence}}
			}
		} else {
			catalogOverflow = true
		}
		if err := ctx.Err(); err != nil {
			return []adapterScope{}, []string{}, nil, err
		}
	} else {
		// The compatibility adapter is part of the required, pinned catalog.
		// Omitting it would make a positive finding unable to reject an
		// adapter-local proposed owner.
		catalogOverflow = true
	}
	if catalogOverflow {
		return []adapterScope{}, []string{}, []string{adapterCatalogIncompleteLimitation}, nil
	}
	scopes := make([]adapterScope, 0, len(scopesByID))
	active := make([]string, 0, len(activeValues))
	for _, scope := range scopesByID {
		sort.Strings(scope.Names)
		sort.Slice(scope.Evidence, func(i, j int) bool {
			if scope.Evidence[i].Path != scope.Evidence[j].Path {
				return scope.Evidence[i].Path < scope.Evidence[j].Path
			}
			return scope.Evidence[i].Line < scope.Evidence[j].Line
		})
		scopes = append(scopes, *scope)
		if scope.Lifecycle == "active" {
			active = append(active, scope.ID)
		}
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].ID < scopes[j].ID })
	sort.Strings(active)
	return scopes, active, nil, nil
}

func catalogGoSourceWithinLimit(body string) bool {
	return len(body) <= maxAdapterCatalogSourceBytes
}

func appendScopeName(scope *adapterScope, name string) bool {
	if name == "" || strings.TrimSpace(name) != name || len([]byte(name)) > maxScenarioNameBytes {
		return false
	}
	if containsString(scope.Names, name) {
		return true
	}
	if len(scope.Names) >= maxAdapterScopeNames {
		return false
	}
	scope.Names = append(scope.Names, name)
	return true
}

func appendScopeEvidence(scope *adapterScope, item scopeEvidence) bool {
	if item.Path == "" || item.Line < 1 || item.Excerpt == "" {
		return false
	}
	if slices.Contains(scope.Evidence, item) {
		return true
	}
	if len(scope.Evidence) >= maxAdapterScopeEvidence {
		return false
	}
	scope.Evidence = append(scope.Evidence, item)
	return true
}

//nolint:gocyclo // The whole-file AST walk fails closed at every duplicate or unsupported target declaration.
func goStringSliceValues(path, body, variable string) ([]catalogValue, bool) {
	if !catalogGoSourceWithinLimit(body) {
		return nil, false
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, body, parser.SkipObjectResolution)
	if err != nil {
		return nil, false
	}
	occurrences := 0
	ast.Inspect(file, func(node ast.Node) bool {
		switch candidate := node.(type) {
		case *ast.ValueSpec:
			for _, name := range candidate.Names {
				if name.Name == variable {
					occurrences++
				}
			}
		case *ast.TypeSpec:
			if candidate.Name.Name == variable {
				occurrences++
			}
		case *ast.ImportSpec:
			if candidate.Name != nil && candidate.Name.Name == variable {
				occurrences++
			}
		case *ast.FuncDecl:
			// A method name is selected through a receiver type, not introduced
			// into package scope. Only a package function can collide with the
			// package registry declaration.
			if candidate.Recv == nil && candidate.Name.Name == variable {
				occurrences++
			}
			if fieldListBindsName(candidate.Recv, variable) {
				occurrences++
			}
		case *ast.FuncType:
			// Parameters (including closure and type parameters) and named
			// results introduce lexical bindings. Struct/interface fields use
			// the same AST node type, so they must only be considered here.
			if fieldListBindsName(candidate.TypeParams, variable) || fieldListBindsName(candidate.Params, variable) || fieldListBindsName(candidate.Results, variable) {
				occurrences++
			}
		case *ast.AssignStmt:
			for _, expression := range candidate.Lhs {
				if assignmentTargetReferences(expression, variable) {
					occurrences++
				}
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{candidate.Key, candidate.Value} {
				if expression != nil && assignmentTargetReferences(expression, variable) {
					occurrences++
				}
			}
		case *ast.IncDecStmt:
			if assignmentTargetReferences(candidate.X, variable) {
				occurrences++
			}
		}
		return true
	})
	validOccurrence := false
	var result []catalogValue
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range general.Specs {
			valueSpec, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			targetIndex := -1
			for index, name := range valueSpec.Names {
				if name.Name == variable {
					targetIndex = index
				}
			}
			if targetIndex < 0 {
				continue
			}
			if general.Tok != token.VAR || len(valueSpec.Names) != 1 || targetIndex != 0 || len(valueSpec.Values) != 1 {
				continue
			}
			composite, ok := valueSpec.Values[0].(*ast.CompositeLit)
			if !ok || !isStringSliceComposite(composite) {
				continue
			}
			values := make([]catalogValue, 0, len(composite.Elts))
			supported := true
			for _, element := range composite.Elts {
				literal, ok := element.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					supported = false
					break
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil || value == "" || strings.TrimSpace(value) != value {
					supported = false
					break
				}
				line := fileSet.Position(literal.Pos()).Line
				values = append(values, catalogValue{value: value, evidence: sourceEvidence(path, body, line)})
			}
			if supported {
				validOccurrence = true
				result = values
			}
		}
	}
	if occurrences != 1 || !validOccurrence {
		return nil, false
	}
	return result, true
}

func fieldListBindsName(fields *ast.FieldList, variable string) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			if name.Name == variable {
				return true
			}
		}
	}
	return false
}

// assignmentTargetReferences follows only the storage location selected by a
// legal assignment target. Selector field names and index expressions are not
// lexical references to the package registry variable, but a registry rooted
// target such as activeHarnesses[0] remains a mutation of that variable.
func assignmentTargetReferences(expression ast.Expr, variable string) bool {
	switch target := expression.(type) {
	case *ast.Ident:
		return target.Name == variable
	case *ast.ParenExpr:
		return assignmentTargetReferences(target.X, variable)
	case *ast.StarExpr:
		return assignmentTargetReferences(target.X, variable)
	case *ast.SelectorExpr:
		return assignmentTargetReferences(target.X, variable)
	case *ast.IndexExpr:
		return assignmentTargetReferences(target.X, variable)
	case *ast.IndexListExpr:
		return assignmentTargetReferences(target.X, variable)
	case *ast.SliceExpr:
		return assignmentTargetReferences(target.X, variable)
	default:
		return false
	}
}

func isStringSliceComposite(composite *ast.CompositeLit) bool {
	array, ok := composite.Type.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	element, ok := array.Elt.(*ast.Ident)
	return ok && element.Name == "string"
}

//nolint:gocyclo // Exhaustive fail-closed AST validation keeps every accepted alias shape explicit.
func goNormalizeHarnessAliases(path, body string) (map[string][]catalogValue, bool) {
	result := map[string][]catalogValue{}
	if !catalogGoSourceWithinLimit(body) {
		return result, false
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, body, parser.SkipObjectResolution)
	if err != nil {
		return result, false
	}
	found := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "NormalizeHarnessName" || function.Body == nil {
			continue
		}
		if found || function.Recv != nil || len(function.Body.List) != 1 {
			return map[string][]catalogValue{}, false
		}
		found = true
		switchStatement, ok := function.Body.List[0].(*ast.SwitchStmt)
		tag, tagOK := switchStatementTag(switchStatement)
		if !ok || !tagOK || tag != "name" {
			return map[string][]catalogValue{}, false
		}
		defaultSeen := false
		seenAliases := map[string]bool{}
		for _, statement := range switchStatement.Body.List {
			clause, ok := statement.(*ast.CaseClause)
			if !ok {
				return map[string][]catalogValue{}, false
			}
			if len(clause.List) == 0 {
				if defaultSeen || !returnsIdentifierExactly(clause.Body, "name") {
					return map[string][]catalogValue{}, false
				}
				defaultSeen = true
				continue
			}
			canonical, ok := returnedString(clause.Body)
			if !ok || canonical == "" || strings.TrimSpace(canonical) != canonical {
				return map[string][]catalogValue{}, false
			}
			for _, expression := range clause.List {
				literal, ok := expression.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return map[string][]catalogValue{}, false
				}
				alias, err := strconv.Unquote(literal.Value)
				if err != nil || alias == "" || strings.TrimSpace(alias) != alias || seenAliases[alias] {
					return map[string][]catalogValue{}, false
				}
				seenAliases[alias] = true
				line := fileSet.Position(literal.Pos()).Line
				result[canonical] = append(result[canonical], catalogValue{value: alias, evidence: sourceEvidence(path, body, line)})
			}
		}
		if !defaultSeen {
			return map[string][]catalogValue{}, false
		}
	}
	return result, found
}

//nolint:gocyclo // The narrow AST projection intentionally exposes every accepted node shape.
func goHarnessConfigNames(path, body string) (map[string][]catalogValue, bool) {
	result := map[string][]catalogValue{}
	if !catalogGoSourceWithinLimit(body) {
		return result, false
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, body, parser.SkipObjectResolution)
	if err != nil {
		return result, false
	}
	found := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "SurfaceForHarness" || function.Body == nil {
			continue
		}
		if found || function.Recv != nil || len(function.Body.List) != 1 {
			return map[string][]catalogValue{}, false
		}
		found = true
		switchStatement, ok := function.Body.List[0].(*ast.SwitchStmt)
		if !ok || !isNormalizedHarnessSwitch(switchStatement) {
			return map[string][]catalogValue{}, false
		}
		defaultSeen := false
		seenCanonical := map[string]bool{}
		for _, statement := range switchStatement.Body.List {
			clause, ok := statement.(*ast.CaseClause)
			if !ok {
				return map[string][]catalogValue{}, false
			}
			if len(clause.List) == 0 {
				if defaultSeen || !returnsEmptyDirectorySurface(clause.Body) {
					return map[string][]catalogValue{}, false
				}
				defaultSeen = true
				continue
			}
			if len(clause.List) != 1 {
				return map[string][]catalogValue{}, false
			}
			canonicalLiteral, ok := clause.List[0].(*ast.BasicLit)
			if !ok || canonicalLiteral.Kind != token.STRING {
				return map[string][]catalogValue{}, false
			}
			canonical, err := strconv.Unquote(canonicalLiteral.Value)
			if err != nil || canonical == "" || strings.TrimSpace(canonical) != canonical || seenCanonical[canonical] {
				return map[string][]catalogValue{}, false
			}
			seenCanonical[canonical] = true
			configRoot, evidence, deprecated, valid := parseHarnessConfigReturn(path, body, fileSet, canonical, clause.Body)
			if !valid {
				return map[string][]catalogValue{}, false
			}
			result[canonical] = append(result[canonical], catalogValue{value: configRoot, evidence: evidence, deprecated: deprecated, hasDeprecated: true})
		}
		if !defaultSeen {
			return map[string][]catalogValue{}, false
		}
	}
	return result, found
}

func isNormalizedHarnessSwitch(statement *ast.SwitchStmt) bool {
	if statement == nil || statement.Init != nil || statement.Tag == nil || statement.Body == nil {
		return false
	}
	call, ok := statement.Tag.(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageName, packageOK := selector.X.(*ast.Ident)
	argument, argumentOK := call.Args[0].(*ast.Ident)
	return packageOK && argumentOK && packageName.Name == "agent" && selector.Sel.Name == "NormalizeHarnessName" && argument.Name == "harness"
}

func switchStatementTag(statement *ast.SwitchStmt) (string, bool) {
	if statement == nil || statement.Init != nil || statement.Tag == nil || statement.Body == nil {
		return "", false
	}
	identifier, ok := statement.Tag.(*ast.Ident)
	if !ok {
		return "", false
	}
	return identifier.Name, true
}

//nolint:gocyclo // Exhaustive fail-closed AST validation keeps every accepted config-surface field explicit.
func parseHarnessConfigReturn(path, body string, fileSet *token.FileSet, canonical string, statements []ast.Stmt) (string, scopeEvidence, bool, bool) {
	if len(statements) != 1 {
		return "", scopeEvidence{}, false, false
	}
	returned, ok := statements[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 || !identifierIs(returned.Results[1], "true") {
		return "", scopeEvidence{}, false, false
	}
	composite, ok := returned.Results[0].(*ast.CompositeLit)
	if !ok {
		return "", scopeEvidence{}, false, false
	}
	typeName, typeOK := composite.Type.(*ast.Ident)
	if !typeOK || typeName.Name != "DirectorySurface" {
		return "", scopeEvidence{}, false, false
	}
	values := map[string]string{}
	deprecated := false
	deprecatedSeen := false
	var directoryEvidence scopeEvidence
	for _, element := range composite.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return "", scopeEvidence{}, false, false
		}
		key, keyOK := keyed.Key.(*ast.Ident)
		if !keyOK || values[key.Name] != "" || (key.Name == "Deprecated" && deprecatedSeen) {
			return "", scopeEvidence{}, false, false
		}
		switch key.Name {
		case "Harness", "Directory", "Purpose":
			literal, ok := keyed.Value.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return "", scopeEvidence{}, false, false
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || value == "" || strings.TrimSpace(value) != value {
				return "", scopeEvidence{}, false, false
			}
			values[key.Name] = value
			if key.Name == "Directory" {
				directoryEvidence = sourceEvidence(path, body, fileSet.Position(literal.Pos()).Line)
			}
		case "Deprecated":
			identifier, ok := keyed.Value.(*ast.Ident)
			if !ok || (identifier.Name != "true" && identifier.Name != "false") {
				return "", scopeEvidence{}, false, false
			}
			deprecated = identifier.Name == "true"
			deprecatedSeen = true
		default:
			return "", scopeEvidence{}, false, false
		}
	}
	configRoot := values["Directory"]
	if values["Harness"] != canonical || values["Purpose"] == "" || configRoot == "" || !strings.HasPrefix(configRoot, ".") || configRoot == "." || strings.ContainsAny(configRoot, "/\\") {
		return "", scopeEvidence{}, false, false
	}
	return configRoot, directoryEvidence, deprecated, true
}

func returnsEmptyDirectorySurface(statements []ast.Stmt) bool {
	if len(statements) != 1 {
		return false
	}
	returned, ok := statements[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 || !identifierIs(returned.Results[1], "false") {
		return false
	}
	composite, ok := returned.Results[0].(*ast.CompositeLit)
	if !ok {
		return false
	}
	typeName, typeOK := composite.Type.(*ast.Ident)
	return typeOK && typeName.Name == "DirectorySurface" && len(composite.Elts) == 0
}

func identifierIs(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

//nolint:gocyclo // Strict constant projection fails closed for every unsupported AST shape.
func goMarketplaceCatalogRoots(path, body string) (catalogValue, catalogValue, bool) {
	if !catalogGoSourceWithinLimit(body) {
		return catalogValue{}, catalogValue{}, false
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, body, parser.SkipObjectResolution)
	if err != nil {
		return catalogValue{}, catalogValue{}, false
	}
	values := map[string]catalogValue{}
	targets := map[string]bool{"ClaudeCatalogPath": true, "NeutralCatalogPath": true}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, specification := range general.Specs {
			valueSpec, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range valueSpec.Names {
				if !targets[name.Name] {
					continue
				}
				if _, duplicate := values[name.Name]; duplicate || len(valueSpec.Values) != len(valueSpec.Names) {
					return catalogValue{}, catalogValue{}, false
				}
				literal, ok := valueSpec.Values[index].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return catalogValue{}, catalogValue{}, false
				}
				catalogPath, err := strconv.Unquote(literal.Value)
				if err != nil {
					return catalogValue{}, catalogValue{}, false
				}
				root, ok := marketplaceCatalogRoot(catalogPath)
				if !ok {
					return catalogValue{}, catalogValue{}, false
				}
				values[name.Name] = catalogValue{value: root, evidence: sourceEvidence(path, body, fileSet.Position(literal.Pos()).Line)}
			}
		}
	}
	claude, claudeOK := values["ClaudeCatalogPath"]
	neutral, neutralOK := values["NeutralCatalogPath"]
	return claude, neutral, claudeOK && neutralOK
}

func marketplaceCatalogRoot(path string) (string, bool) {
	if strings.TrimSpace(path) != path || !validPath(path) || filepath.Base(path) != "marketplace.json" {
		return "", false
	}
	root := filepath.ToSlash(filepath.Dir(path))
	if root == "." || !strings.HasPrefix(root, ".") || strings.Contains(root, "/") {
		return "", false
	}
	return root, true
}

func goAdapterName(path, body, receiver string) (string, scopeEvidence, bool) {
	if !catalogGoSourceWithinLimit(body) {
		return "", scopeEvidence{}, false
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, body, parser.SkipObjectResolution)
	if err != nil {
		return "", scopeEvidence{}, false
	}
	found := false
	var name string
	var evidence scopeEvidence
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Name" || receiverTypeName(function.Recv) != receiver || function.Body == nil {
			continue
		}
		if found {
			return "", scopeEvidence{}, false
		}
		found = true
		name, ok = returnedString(function.Body.List)
		if !ok || name == "" || strings.TrimSpace(name) != name {
			return "", scopeEvidence{}, false
		}
		line := fileSet.Position(function.Body.List[0].Pos()).Line
		evidence = sourceEvidence(path, body, line)
	}
	return name, evidence, found
}

func returnedString(statements []ast.Stmt) (string, bool) {
	if len(statements) != 1 {
		return "", false
	}
	returned, ok := statements[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return "", false
	}
	literal, ok := returned.Results[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func returnsIdentifierExactly(statements []ast.Stmt, name string) bool {
	if len(statements) != 1 {
		return false
	}
	returned, ok := statements[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	identifier, ok := returned.Results[0].(*ast.Ident)
	return ok && identifier.Name == name
}

func receiverTypeName(receivers *ast.FieldList) string {
	if receivers == nil || len(receivers.List) != 1 {
		return ""
	}
	typeExpression := receivers.List[0].Type
	if pointer, ok := typeExpression.(*ast.StarExpr); ok {
		typeExpression = pointer.X
	}
	identifier, _ := typeExpression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func sourceEvidence(path, body string, line int) scopeEvidence {
	lines := strings.Split(body, "\n")
	excerpt := ""
	if line > 0 && line <= len(lines) {
		excerpt = strings.TrimSpace(lines[line-1])
	}
	return scopeEvidence{Path: path, Line: line, Excerpt: excerpt}
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
	var fence markdownFenceState
	traceability := specTraceabilityState{seenFeatures: map[string]bool{}}
	for index, line := range strings.Split(body, "\n") {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(line)
		if fence.consume(line) {
			continue
		}
		if fence.openingRun != 0 {
			continue
		}
		traceability.observeHeading(trimmed, lineNumber)
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
		traceability.collect(&file, trimmed, lineNumber)
	}
	return file
}

type specTraceabilityState struct {
	inSupportedSection           bool
	featureBearingSectionSeen    bool
	currentSectionFeatureBearing bool
	ignoreCurrentSectionFeatures bool
	currentSectionHeading        diagnostic
	seenFeatures                 map[string]bool
}

func (state *specTraceabilityState) observeHeading(line string, lineNumber int) {
	heading := markdownHeadingPattern.FindStringSubmatch(line)
	if len(heading) == 0 || len(heading[1]) > 2 {
		return
	}
	state.inSupportedSection = false
	state.currentSectionFeatureBearing = false
	state.ignoreCurrentSectionFeatures = false
	traceabilityHeading := strings.ToLower(strings.TrimSpace(heading[2]))
	if heading[1] != "##" || !specTraceabilityHeadings[traceabilityHeading] {
		return
	}
	state.inSupportedSection = true
	state.currentSectionHeading = diagnostic{Line: lineNumber, Kind: "ambiguous-bdd-traceability-section", Excerpt: line}
}

func (state *specTraceabilityState) collect(file *specFile, line string, lineNumber int) {
	if state.inSupportedSection && looksLikeBDDFeatureClaim(line) && !state.currentSectionFeatureBearing {
		state.currentSectionFeatureBearing = true
		if state.featureBearingSectionSeen {
			state.ignoreCurrentSectionFeatures = true
			file.Diagnostics = append(file.Diagnostics, state.currentSectionHeading)
		} else {
			state.featureBearingSectionSeen = true
		}
	}
	collectSpecBDDTraceability(file, line, lineNumber, state.inSupportedSection, !state.ignoreCurrentSectionFeatures, state.seenFeatures)
}

func looksLikeBDDFeatureClaim(line string) bool {
	if len(bddFeatureEntryPattern.FindStringSubmatch(line)) != 0 {
		return true
	}
	return strings.HasPrefix(line, "-") &&
		(strings.Contains(strings.ToLower(line), "feature:") || strings.Contains(line, ".feature"))
}

func collectSpecBDDTraceability(file *specFile, line string, lineNumber int, inTraceability, collectLinks bool, seen map[string]bool) {
	if !inTraceability {
		return
	}
	match := bddFeatureEntryPattern.FindStringSubmatch(line)
	if len(match) == 0 || !specTraceabilityLabels[strings.ToLower(strings.TrimSpace(match[1]))] {
		if looksLikeBDDFeatureClaim(line) {
			file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "malformed-bdd-feature-reference", Excerpt: line})
		}
		return
	}
	feature := filepath.ToSlash(match[2])
	if feature != filepath.ToSlash(filepath.Clean(feature)) || !validFeaturePath(feature) {
		file.Diagnostics = append(file.Diagnostics, diagnostic{Line: lineNumber, Kind: "malformed-bdd-feature-reference", Excerpt: line})
		return
	}
	if !collectLinks {
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

//nolint:gocyclo // Marker validation and bounded official Gherkin parsing share one fail-closed pass.
func parseFeature(ctx context.Context, path, body string) (featureFile, []bddRef, error) {
	if len(body) > maxFeatureBytes {
		return featureFile{}, nil, fmt.Errorf("pinned BDD feature %q exceeds %d-byte per-file limit", path, maxFeatureBytes)
	}
	digest := sha256.Sum256([]byte(body))
	file := featureFile{Path: filepath.ToSlash(path), SHA256: fmt.Sprintf("%x", digest), RelatedSpecs: []string{}, Scenarios: []featureScenario{}}
	related := make([]string, 0)
	refs := make([]bddRef, 0)
	seen := map[string]bool{}
	var fence markdownFenceState
	primaryMarkers := 0
	for index, line := range strings.Split(body, "\n") {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(line)
		if fence.consume(line) {
			continue
		}
		if fence.openingRun != 0 {
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
		value = strings.TrimSpace(value)
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
	scenarios, structuralDiagnostics, err := parseFeatureScenarios(ctx, path, body)
	if err != nil {
		return featureFile{}, nil, err
	}
	file.Scenarios = scenarios
	file.Diagnostics = append(file.Diagnostics, structuralDiagnostics...)
	return file, refs, nil
}

func parseFeatureScenarios(ctx context.Context, path, body string) ([]featureScenario, []diagnostic, error) {
	source := featureBodyWithoutMarkdownFences(body)
	nextID := 0
	document, err := parseBoundedGherkinDocument(ctx, path, source, func() string {
		nextID++
		return strconv.Itoa(nextID)
	})
	if err != nil || document == nil || document.Feature == nil {
		var boundError *gherkinBoundError
		if errors.As(err, &boundError) {
			return nil, nil, err
		}
		excerpt := "feature has no parseable Gherkin document"
		if err != nil {
			excerpt = "Gherkin parse failed: " + err.Error()
		}
		return []featureScenario{}, []diagnostic{{Line: 1, Kind: "malformed-gherkin-structure", Excerpt: excerpt}}, nil
	}
	if err := validateGherkinDocumentBounds(path, document); err != nil {
		return nil, nil, err
	}
	gherkinScenarios := make([]*messages.Scenario, 0)
	for _, child := range document.Feature.Children {
		switch {
		case child.Scenario != nil:
			gherkinScenarios = append(gherkinScenarios, child.Scenario)
		case child.Rule != nil:
			for _, ruleChild := range child.Rule.Children {
				if ruleChild.Scenario != nil {
					gherkinScenarios = append(gherkinScenarios, ruleChild.Scenario)
				}
			}
		}
	}
	diagnostics := make([]diagnostic, 0)
	scenarios := make([]featureScenario, 0, len(gherkinScenarios))
	for _, sourceScenario := range gherkinScenarios {
		scenario, scenarioDiagnostics, ok := projectFeatureScenario(sourceScenario)
		diagnostics = append(diagnostics, scenarioDiagnostics...)
		if ok {
			scenarios = append(scenarios, scenario)
		}
	}
	return scenarios, diagnostics, nil
}

type gherkinBoundError struct {
	err error
}

func (err *gherkinBoundError) Error() string {
	return err.err.Error()
}

func (err *gherkinBoundError) Unwrap() error {
	return err.err
}

type boundedGherkinState struct {
	ctx      context.Context
	path     string
	err      *gherkinBoundError
	parseErr error
}

func (state *boundedGherkinState) fail(err error) error {
	if state.err == nil {
		state.err = &gherkinBoundError{err: err}
	}
	return state.err
}

func (state *boundedGherkinState) checkContext() error {
	if state.err != nil {
		return state.err
	}
	if state.parseErr != nil {
		return state.parseErr
	}
	if err := state.ctx.Err(); err != nil {
		return state.fail(fmt.Errorf("pinned BDD feature %q Gherkin preflight canceled: %w", state.path, err))
	}
	return nil
}

func (state *boundedGherkinState) aborted() bool {
	return state.err != nil || state.parseErr != nil
}

func (state *boundedGherkinState) stopParse(err error) error {
	if state.parseErr == nil {
		state.parseErr = err
	}
	return state.parseErr
}

type boundedGherkinScanner struct {
	delegate gherkin.Scanner
	state    *boundedGherkinState
	lastLine int
}

func (scanner *boundedGherkinScanner) Scan() (*gherkin.Line, bool, error) {
	if scanner.state.aborted() {
		return scanner.eof(), true, scanner.state.checkContext()
	}
	if err := scanner.state.checkContext(); err != nil {
		return scanner.eof(), true, err
	}
	line, atEOF, err := scanner.delegate.Scan()
	if err != nil {
		boundError := scanner.state.fail(fmt.Errorf("pinned BDD feature %q exceeds %d-byte line limit or could not be scanned: %w", scanner.state.path, maxFeatureLineBytes, err))
		return scanner.eof(), true, boundError
	}
	if line != nil {
		scanner.lastLine = line.LineNumber
		if len(line.LineText) > maxFeatureLineBytes {
			boundError := scanner.state.fail(fmt.Errorf("pinned BDD feature %q exceeds %d-byte line limit at line %d", scanner.state.path, maxFeatureLineBytes, line.LineNumber))
			return scanner.eof(), true, boundError
		}
	}
	if err := scanner.state.checkContext(); err != nil {
		return scanner.eof(), true, err
	}
	return line, atEOF, nil
}

func (scanner *boundedGherkinScanner) eof() *gherkin.Line {
	return &gherkin.Line{LineNumber: scanner.lastLine + 1, AtEof: true}
}

type boundedGherkinBuilder struct {
	delegate                    gherkin.AstBuilder
	state                       *boundedGherkinState
	structuralTokens            int
	scenarios                   int
	steps                       int
	outcomes                    int
	examples                    int
	tableRows                   int
	exampleCases                int
	afterOutcome                bool
	inExamples                  bool
	currentExamplesHasTableHead bool
	tags                        int
	tagBytes                    int
	comments                    int
	commentBytes                int
	descriptionLines            int
	descriptionBytes            int
	docStrings                  int
	docStringSeparators         int
	docStringSeparatorBytes     int
	docStringLines              int
	docStringBytes              int
	inDocString                 bool
}

func (builder *boundedGherkinBuilder) GetGherkinDocument() *messages.GherkinDocument {
	if builder.state.err != nil {
		return nil
	}
	return builder.delegate.GetGherkinDocument()
}

func (builder *boundedGherkinBuilder) Reset() {
	builder.structuralTokens = 0
	builder.scenarios = 0
	builder.tags = 0
	builder.tagBytes = 0
	builder.comments = 0
	builder.commentBytes = 0
	builder.descriptionLines = 0
	builder.descriptionBytes = 0
	builder.docStrings = 0
	builder.docStringSeparators = 0
	builder.docStringSeparatorBytes = 0
	builder.docStringLines = 0
	builder.docStringBytes = 0
	builder.inDocString = false
	builder.resetScenario()
	builder.state.err = nil
	builder.state.parseErr = nil
	builder.delegate.Reset()
	_ = builder.state.checkContext()
}

func (builder *boundedGherkinBuilder) StartRule(rule gherkin.RuleType) (bool, error) {
	if builder.state.aborted() {
		return false, nil
	}
	if err := builder.state.checkContext(); err != nil {
		return false, err
	}
	ok, err := builder.delegate.StartRule(rule)
	if err != nil {
		return false, builder.state.stopParse(err)
	}
	return ok, nil
}

func (builder *boundedGherkinBuilder) EndRule(rule gherkin.RuleType) (bool, error) {
	if builder.state.aborted() {
		return false, nil
	}
	if err := builder.state.checkContext(); err != nil {
		return false, err
	}
	ok, err := builder.delegate.EndRule(rule)
	if err != nil {
		return false, builder.state.stopParse(err)
	}
	return ok, nil
}

func (builder *boundedGherkinBuilder) Build(token *gherkin.Token) (bool, error) {
	if builder.state.aborted() {
		return false, nil
	}
	if err := builder.state.checkContext(); err != nil {
		return false, err
	}
	if err := builder.observe(token); err != nil {
		return false, builder.state.fail(err)
	}
	ok, err := builder.delegate.Build(token)
	if err != nil {
		return false, builder.state.stopParse(err)
	}
	return ok, nil
}

//nolint:gocyclo // Each official Gherkin token maps directly to one pre-AST resource bound.
func (builder *boundedGherkinBuilder) observe(token *gherkin.Token) error {
	switch token.Type { //nolint:exhaustive // Structural and ignorable tokens continue to the existing bounded-token gate below.
	case gherkin.TokenTypeTagLine:
		for _, item := range token.Items {
			builder.tags++
			builder.tagBytes += len(item.Text)
			if builder.tags > maxFeatureTags || builder.tagBytes > maxFeatureTagBytes {
				return fmt.Errorf("pinned BDD feature %q exceeds retained tag limit (%d items or %d bytes)", builder.state.path, maxFeatureTags, maxFeatureTagBytes)
			}
		}
		return nil
	case gherkin.TokenTypeComment:
		builder.comments++
		builder.commentBytes += len(token.Text)
		if builder.comments > maxFeatureComments || builder.commentBytes > maxFeatureCommentBytes {
			return fmt.Errorf("pinned BDD feature %q exceeds retained comment limit (%d comments or %d bytes)", builder.state.path, maxFeatureComments, maxFeatureCommentBytes)
		}
		return nil
	case gherkin.TokenTypeDocStringSeparator:
		builder.docStringSeparators++
		builder.docStringSeparatorBytes += len(token.Keyword) + len(token.Text)
		if !builder.inDocString {
			builder.docStrings++
			builder.inDocString = true
		} else {
			builder.inDocString = false
		}
		if builder.docStrings > maxFeatureDocStrings || builder.docStringSeparators > maxFeatureDocStringSeparators || builder.docStringSeparatorBytes > maxFeatureDocStringBytes {
			return fmt.Errorf("pinned BDD feature %q exceeds retained DocString delimiter limit (%d DocStrings, %d separators, or %d bytes)", builder.state.path, maxFeatureDocStrings, maxFeatureDocStringSeparators, maxFeatureDocStringBytes)
		}
		return nil
	case gherkin.TokenTypeOther:
		if builder.inDocString {
			builder.docStringLines++
			builder.docStringBytes += len(token.Text)
			if builder.docStringLines > 1 {
				builder.docStringBytes++
			}
			if builder.docStringLines > maxFeatureDocStringLines || builder.docStringBytes > maxFeatureDocStringBytes {
				return fmt.Errorf("pinned BDD feature %q exceeds retained DocString content limit (%d lines or %d bytes)", builder.state.path, maxFeatureDocStringLines, maxFeatureDocStringBytes)
			}
			return nil
		}
		builder.descriptionLines++
		builder.descriptionBytes += len(token.Text)
		if builder.descriptionLines > 1 {
			builder.descriptionBytes++
		}
		if builder.descriptionLines > maxFeatureDescriptionLines || builder.descriptionBytes > maxFeatureDescriptionBytes {
			return fmt.Errorf("pinned BDD feature %q exceeds retained description limit (%d lines or %d bytes)", builder.state.path, maxFeatureDescriptionLines, maxFeatureDescriptionBytes)
		}
		return nil
	default:
	}
	if !isBoundedGherkinStructuralToken(token.Type) {
		return nil
	}
	builder.structuralTokens++
	if builder.structuralTokens > maxFeatureStructuralTokens {
		return fmt.Errorf("pinned BDD feature %q exceeds %d structural-token limit", builder.state.path, maxFeatureStructuralTokens)
	}
	switch token.Type {
	case gherkin.TokenTypeScenarioLine:
		builder.scenarios++
		if builder.scenarios > maxFeatureScenarios {
			return fmt.Errorf("pinned BDD feature %q exceeds %d-scenario limit", builder.state.path, maxFeatureScenarios)
		}
		if len(token.Text) > maxScenarioNameBytes {
			return fmt.Errorf("pinned BDD feature %q has a scenario name exceeding %d bytes", builder.state.path, maxScenarioNameBytes)
		}
		builder.resetScenario()
	case gherkin.TokenTypeBackgroundLine:
		builder.resetScenario()
	case gherkin.TokenTypeStepLine:
		builder.inExamples = false
		builder.steps++
		if builder.steps > maxScenarioSteps {
			return fmt.Errorf("pinned BDD feature %q exceeds %d-step per-scenario limit", builder.state.path, maxScenarioSteps)
		}
		builder.observeStepKeyword(token.KeywordType)
		if builder.outcomes > maxScenarioOutcomes {
			return fmt.Errorf("pinned BDD feature %q exceeds %d-outcome per-scenario limit", builder.state.path, maxScenarioOutcomes)
		}
	case gherkin.TokenTypeExamplesLine:
		builder.examples++
		if builder.examples > maxScenarioExamples {
			return fmt.Errorf("pinned BDD feature %q exceeds %d-Examples per-scenario limit", builder.state.path, maxScenarioExamples)
		}
		builder.inExamples = true
		builder.currentExamplesHasTableHead = false
	case gherkin.TokenTypeTableRow:
		builder.tableRows++
		if builder.tableRows > maxScenarioTableRows {
			return fmt.Errorf("pinned BDD feature %q exceeds %d-table-row per-scenario limit", builder.state.path, maxScenarioTableRows)
		}
		if len(token.Items) > maxTableCellsPerRow {
			return fmt.Errorf("pinned BDD feature %q has a table row exceeding %d cells", builder.state.path, maxTableCellsPerRow)
		}
		if builder.inExamples && builder.currentExamplesHasTableHead {
			builder.exampleCases++
			if builder.exampleCases > maxScenarioCases {
				return fmt.Errorf("pinned BDD feature %q exceeds %d Examples cases per scenario", builder.state.path, maxScenarioCases)
			}
		} else if builder.inExamples {
			builder.currentExamplesHasTableHead = true
		}
	case gherkin.TokenTypeFeatureLine, gherkin.TokenTypeRuleLine:
	case gherkin.TokenTypeNone,
		gherkin.TokenTypeEOF,
		gherkin.TokenTypeEmpty,
		gherkin.TokenTypeComment,
		gherkin.TokenTypeTagLine,
		gherkin.TokenTypeDocStringSeparator,
		gherkin.TokenTypeLanguage,
		gherkin.TokenTypeOther:
		return nil
	}
	return nil
}

func (builder *boundedGherkinBuilder) observeStepKeyword(keywordType messages.StepKeywordType) {
	isOutcome := keywordType == messages.StepKeywordType_OUTCOME
	isConjunction := keywordType == messages.StepKeywordType_CONJUNCTION
	if !isOutcome && (!isConjunction || !builder.afterOutcome) {
		builder.afterOutcome = false
		return
	}
	builder.afterOutcome = true
	builder.outcomes++
}

func (builder *boundedGherkinBuilder) resetScenario() {
	builder.steps = 0
	builder.outcomes = 0
	builder.examples = 0
	builder.tableRows = 0
	builder.exampleCases = 0
	builder.afterOutcome = false
	builder.inExamples = false
	builder.currentExamplesHasTableHead = false
}

func isBoundedGherkinStructuralToken(tokenType gherkin.TokenType) bool {
	switch tokenType {
	case gherkin.TokenTypeFeatureLine,
		gherkin.TokenTypeRuleLine,
		gherkin.TokenTypeBackgroundLine,
		gherkin.TokenTypeScenarioLine,
		gherkin.TokenTypeExamplesLine,
		gherkin.TokenTypeStepLine,
		gherkin.TokenTypeTableRow:
		return true
	case gherkin.TokenTypeNone,
		gherkin.TokenTypeEOF,
		gherkin.TokenTypeEmpty,
		gherkin.TokenTypeComment,
		gherkin.TokenTypeTagLine,
		gherkin.TokenTypeDocStringSeparator,
		gherkin.TokenTypeLanguage,
		gherkin.TokenTypeOther:
		return false
	}
	return false
}

func parseBoundedGherkinDocument(ctx context.Context, path, source string, newID func() string) (*messages.GherkinDocument, error) {
	state := &boundedGherkinState{ctx: ctx, path: path}
	builder := &boundedGherkinBuilder{delegate: gherkin.NewAstBuilder(newID), state: state}
	parser := gherkin.NewParser(builder)
	parser.StopAtFirstError(true)
	scanner := &boundedGherkinScanner{delegate: gherkin.NewScanner(strings.NewReader(source)), state: state}
	err := parser.Parse(scanner, gherkin.NewMatcher(gherkin.DialectsBuiltin()))
	if state.err != nil {
		return nil, state.err
	}
	if state.parseErr != nil {
		return nil, state.parseErr
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, &gherkinBoundError{err: fmt.Errorf("pinned BDD feature %q Gherkin preflight canceled: %w", path, contextErr)}
	}
	return builder.GetGherkinDocument(), err
}

func preflightGherkinSource(ctx context.Context, path, source string) error {
	nextID := 0
	_, err := parseBoundedGherkinDocument(ctx, path, source, func() string {
		nextID++
		return strconv.Itoa(nextID)
	})
	return err
}

type retainedGherkinASTBudget struct {
	tags                    int
	tagBytes                int
	comments                int
	commentBytes            int
	descriptionLines        int
	descriptionBytes        int
	docStrings              int
	docStringDelimiterBytes int
	docStringLines          int
	docStringBytes          int
}

func (budget *retainedGherkinASTBudget) addTags(path string, tags []*messages.Tag) error {
	for _, tag := range tags {
		if tag == nil {
			return fmt.Errorf("pinned BDD feature %q contains a nil retained tag", path)
		}
		budget.tags++
		budget.tagBytes += len(tag.Name)
		if budget.tags > maxFeatureTags || budget.tagBytes > maxFeatureTagBytes {
			return fmt.Errorf("pinned BDD feature %q parsed AST exceeds retained tag limit (%d items or %d bytes)", path, maxFeatureTags, maxFeatureTagBytes)
		}
	}
	return nil
}

func (budget *retainedGherkinASTBudget) addDescription(path, description string) error {
	if description == "" {
		return nil
	}
	budget.descriptionLines += strings.Count(description, "\n") + 1
	budget.descriptionBytes += len(description)
	if budget.descriptionLines > maxFeatureDescriptionLines || budget.descriptionBytes > maxFeatureDescriptionBytes {
		return fmt.Errorf("pinned BDD feature %q parsed AST exceeds retained description limit (%d lines or %d bytes)", path, maxFeatureDescriptionLines, maxFeatureDescriptionBytes)
	}
	return nil
}

func (budget *retainedGherkinASTBudget) addDocString(path string, docString *messages.DocString) error {
	if docString == nil {
		return nil
	}
	budget.docStrings++
	budget.docStringDelimiterBytes += len(docString.Delimiter) + len(docString.MediaType)
	if docString.Content != "" {
		budget.docStringLines += strings.Count(docString.Content, "\n") + 1
		budget.docStringBytes += len(docString.Content)
	}
	if budget.docStrings > maxFeatureDocStrings || budget.docStringDelimiterBytes > maxFeatureDocStringBytes || budget.docStringLines > maxFeatureDocStringLines || budget.docStringBytes > maxFeatureDocStringBytes {
		return fmt.Errorf("pinned BDD feature %q parsed AST exceeds retained DocString limit (%d nodes, %d lines, or %d bytes)", path, maxFeatureDocStrings, maxFeatureDocStringLines, maxFeatureDocStringBytes)
	}
	return nil
}

func (budget *retainedGherkinASTBudget) addSteps(path string, steps []*messages.Step) error {
	for _, step := range steps {
		if step == nil {
			return fmt.Errorf("pinned BDD feature %q contains a nil retained step", path)
		}
		if err := budget.addDocString(path, step.DocString); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // The traversal mirrors every Gherkin AST location that retains tags, comments, descriptions, or DocStrings.
func validateGherkinRetainedSurfaces(path string, document *messages.GherkinDocument) error {
	budget := &retainedGherkinASTBudget{}
	for _, comment := range document.Comments {
		if comment == nil {
			return fmt.Errorf("pinned BDD feature %q contains a nil retained comment", path)
		}
		budget.comments++
		budget.commentBytes += len(comment.Text)
		if budget.comments > maxFeatureComments || budget.commentBytes > maxFeatureCommentBytes {
			return fmt.Errorf("pinned BDD feature %q parsed AST exceeds retained comment limit (%d comments or %d bytes)", path, maxFeatureComments, maxFeatureCommentBytes)
		}
	}
	feature := document.Feature
	if err := budget.addTags(path, feature.Tags); err != nil {
		return err
	}
	if err := budget.addDescription(path, feature.Description); err != nil {
		return err
	}
	for _, child := range feature.Children {
		if child == nil {
			return fmt.Errorf("pinned BDD feature %q contains a nil Feature child", path)
		}
		if child.Background != nil {
			if err := budget.addDescription(path, child.Background.Description); err != nil {
				return err
			}
			if err := budget.addSteps(path, child.Background.Steps); err != nil {
				return err
			}
		}
		if child.Scenario != nil {
			if err := validateRetainedScenarioSurfaces(path, budget, child.Scenario); err != nil {
				return err
			}
		}
		if child.Rule != nil {
			if err := budget.addTags(path, child.Rule.Tags); err != nil {
				return err
			}
			if err := budget.addDescription(path, child.Rule.Description); err != nil {
				return err
			}
			for _, ruleChild := range child.Rule.Children {
				if ruleChild == nil {
					return fmt.Errorf("pinned BDD feature %q contains a nil Rule child", path)
				}
				if ruleChild.Background != nil {
					if err := budget.addDescription(path, ruleChild.Background.Description); err != nil {
						return err
					}
					if err := budget.addSteps(path, ruleChild.Background.Steps); err != nil {
						return err
					}
				}
				if ruleChild.Scenario != nil {
					if err := validateRetainedScenarioSurfaces(path, budget, ruleChild.Scenario); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validateRetainedScenarioSurfaces(path string, budget *retainedGherkinASTBudget, scenario *messages.Scenario) error {
	if len(scenario.Name) > maxScenarioNameBytes {
		return fmt.Errorf("pinned BDD feature %q parsed AST has a scenario name exceeding %d bytes", path, maxScenarioNameBytes)
	}
	if err := budget.addTags(path, scenario.Tags); err != nil {
		return err
	}
	if err := budget.addDescription(path, scenario.Description); err != nil {
		return err
	}
	if err := budget.addSteps(path, scenario.Steps); err != nil {
		return err
	}
	for _, examples := range scenario.Examples {
		if examples == nil {
			return fmt.Errorf("pinned BDD feature %q contains nil retained Examples", path)
		}
		if err := budget.addTags(path, examples.Tags); err != nil {
			return err
		}
		if err := budget.addDescription(path, examples.Description); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // Structural and retained-surface AST defenses remain visible in one ordered validation pass.
func validateGherkinDocumentBounds(path string, document *messages.GherkinDocument) error {
	if document == nil || document.Feature == nil {
		return fmt.Errorf("pinned BDD feature %q has no parsed Feature AST", path)
	}
	if err := validateGherkinRetainedSurfaces(path, document); err != nil {
		return err
	}
	scenarios := 0
	for _, child := range document.Feature.Children {
		if child.Background != nil {
			if err := validateGherkinSteps(path, messageLine(child.Background.Location), child.Background.Steps); err != nil {
				return err
			}
		}
		if child.Scenario != nil {
			scenarios++
			if scenarios > maxFeatureScenarios {
				return fmt.Errorf("pinned BDD feature %q exceeds %d-scenario limit", path, maxFeatureScenarios)
			}
			if err := validateGherkinScenarioBounds(path, child.Scenario); err != nil {
				return err
			}
		}
		if child.Rule != nil {
			for _, ruleChild := range child.Rule.Children {
				if ruleChild.Background != nil {
					if err := validateGherkinSteps(path, messageLine(ruleChild.Background.Location), ruleChild.Background.Steps); err != nil {
						return err
					}
				}
				if ruleChild.Scenario != nil {
					scenarios++
					if scenarios > maxFeatureScenarios {
						return fmt.Errorf("pinned BDD feature %q exceeds %d-scenario limit", path, maxFeatureScenarios)
					}
					if err := validateGherkinScenarioBounds(path, ruleChild.Scenario); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validateGherkinScenarioBounds(path string, scenario *messages.Scenario) error {
	line := messageLine(scenario.Location)
	if err := validateGherkinSteps(path, line, scenario.Steps); err != nil {
		return err
	}
	if len(scenario.Examples) > maxScenarioExamples {
		return fmt.Errorf("pinned BDD feature %q scenario at line %d exceeds %d Examples blocks", path, line, maxScenarioExamples)
	}
	exampleCases := 0
	tableRows := 0
	for _, examples := range scenario.Examples {
		if examples.TableHeader != nil {
			tableRows++
			if err := validateGherkinTableRow(path, examples.TableHeader); err != nil {
				return err
			}
		}
		exampleCases += len(examples.TableBody)
		tableRows += len(examples.TableBody)
		for _, row := range examples.TableBody {
			if err := validateGherkinTableRow(path, row); err != nil {
				return err
			}
		}
	}
	if exampleCases > maxScenarioCases {
		return fmt.Errorf("pinned BDD feature %q scenario at line %d exceeds %d Examples cases", path, line, maxScenarioCases)
	}
	if tableRows > maxScenarioTableRows {
		return fmt.Errorf("pinned BDD feature %q scenario at line %d exceeds %d table rows", path, line, maxScenarioTableRows)
	}
	return nil
}

func validateGherkinSteps(path string, line int, steps []*messages.Step) error {
	if len(steps) > maxScenarioSteps {
		return fmt.Errorf("pinned BDD feature %q scenario/background at line %d exceeds %d steps", path, line, maxScenarioSteps)
	}
	outcomes := 0
	afterOutcome := false
	tableRows := 0
	for _, step := range steps {
		isOutcome := step.KeywordType == messages.StepKeywordType_OUTCOME
		isConjunction := step.KeywordType == messages.StepKeywordType_CONJUNCTION
		if !isOutcome && (!isConjunction || !afterOutcome) {
			afterOutcome = false
		} else {
			afterOutcome = true
			outcomes++
		}
		if step.DataTable == nil {
			continue
		}
		tableRows += len(step.DataTable.Rows)
		for _, row := range step.DataTable.Rows {
			if err := validateGherkinTableRow(path, row); err != nil {
				return err
			}
		}
	}
	if outcomes > maxScenarioOutcomes {
		return fmt.Errorf("pinned BDD feature %q scenario/background at line %d exceeds %d outcomes", path, line, maxScenarioOutcomes)
	}
	if tableRows > maxScenarioTableRows {
		return fmt.Errorf("pinned BDD feature %q scenario/background at line %d exceeds %d step table rows", path, line, maxScenarioTableRows)
	}
	return nil
}

func validateGherkinTableRow(path string, row *messages.TableRow) error {
	if row == nil || len(row.Cells) > maxTableCellsPerRow {
		return fmt.Errorf("pinned BDD feature %q has a table row exceeding %d cells", path, maxTableCellsPerRow)
	}
	return nil
}

func featureBodyWithoutMarkdownFences(body string) string {
	lines := strings.Split(body, "\n")
	var fence markdownFenceState
	for index, line := range lines {
		if fence.openingRun == 0 {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, "````") {
				// Exactly three backticks are valid Gherkin DocString syntax.
				continue
			}
		}
		wasOpen := fence.openingRun != 0
		boundary := fence.consume(line)
		if boundary || wasOpen || fence.openingRun != 0 {
			lines[index] = ""
		}
	}
	return strings.Join(lines, "\n")
}

//nolint:gocyclo // Bounded AST projection keeps outcomes, examples, and diagnostics co-located.
func projectFeatureScenario(source *messages.Scenario) (featureScenario, []diagnostic, bool) {
	line := messageLine(source.Location)
	if strings.TrimSpace(source.Name) == "" || len([]byte(source.Name)) > maxScenarioNameBytes {
		return featureScenario{}, []diagnostic{{Line: line, Kind: "gherkin-structure-limit-exceeded", Excerpt: "scenario name is empty or exceeds bounded inventory limit"}}, false
	}
	kind := "scenario"
	if len(source.Examples) > 0 {
		kind = "scenario-outline"
	}
	scenario := featureScenario{Line: line, Name: source.Name, Kind: kind, Outcomes: []scenarioOutcome{}, MemberCases: []scenarioMemberCase{}}
	diagnostics := make([]diagnostic, 0)
	afterOutcome := false
	for _, step := range source.Steps {
		isOutcome := step.KeywordType == messages.StepKeywordType_OUTCOME
		isConjunction := step.KeywordType == messages.StepKeywordType_CONJUNCTION
		if !isOutcome && (!isConjunction || !afterOutcome) {
			afterOutcome = false
			continue
		}
		afterOutcome = true
		scenario.Outcomes = append(scenario.Outcomes, scenarioOutcome{Line: messageLine(step.Location), Text: step.Text})
	}
	memberExamples := 0
	nonMemberExamples := 0
	for _, examples := range source.Examples {
		column, cases, exampleDiagnostics := projectMemberCases(examples)
		diagnostics = append(diagnostics, exampleDiagnostics...)
		if column == "" {
			nonMemberExamples++
			continue
		}
		memberExamples++
		if scenario.MemberColumn != "" && scenario.MemberColumn != column {
			diagnostics = append(diagnostics, diagnostic{Line: messageLine(examples.Location), Kind: "malformed-gherkin-member-cases", Excerpt: "scenario uses inconsistent harness/member columns"})
			continue
		}
		scenario.MemberColumn = column
		scenario.MemberCases = append(scenario.MemberCases, cases...)
	}
	if memberExamples > 0 && nonMemberExamples > 0 {
		diagnostics = append(diagnostics, diagnostic{Line: line, Kind: "malformed-gherkin-member-cases", Excerpt: "scenario mixes harness/member and non-member Examples tables"})
	}
	if scenario.MemberColumn != "" {
		placeholder := "<" + scenario.MemberColumn + ">"
		for _, step := range source.Steps {
			if strings.Contains(step.Text, placeholder) {
				scenario.UsesMemberPlaceholder = true
				break
			}
		}
	}
	return scenario, diagnostics, true
}

func projectMemberCases(examples *messages.Examples) (string, []scenarioMemberCase, []diagnostic) {
	if examples.TableHeader == nil {
		return "", nil, []diagnostic{{Line: messageLine(examples.Location), Kind: "malformed-gherkin-member-cases", Excerpt: "examples block has no table header"}}
	}
	headers := make([]string, 0, len(examples.TableHeader.Cells))
	seenHeaders := map[string]bool{}
	memberColumn := -1
	memberName := ""
	for index, cell := range examples.TableHeader.Cells {
		header := strings.ToLower(strings.TrimSpace(cell.Value))
		if header == "" || seenHeaders[header] {
			return "", nil, []diagnostic{{Line: messageLine(examples.TableHeader.Location), Kind: "malformed-gherkin-member-cases", Excerpt: "examples table headers must be nonempty and unique"}}
		}
		seenHeaders[header] = true
		headers = append(headers, header)
		if header == "harness" || header == "member" {
			if memberColumn >= 0 {
				return "", nil, []diagnostic{{Line: messageLine(examples.TableHeader.Location), Kind: "malformed-gherkin-member-cases", Excerpt: "examples table has multiple harness/member columns"}}
			}
			memberColumn = index
			memberName = header
		}
	}
	if memberColumn < 0 {
		return "", []scenarioMemberCase{}, nil
	}
	cases := make([]scenarioMemberCase, 0, len(examples.TableBody))
	for _, row := range examples.TableBody {
		if len(row.Cells) != len(headers) {
			return "", nil, []diagnostic{{Line: messageLine(row.Location), Kind: "malformed-gherkin-member-cases", Excerpt: "examples table row is not rectangular"}}
		}
		member := strings.TrimSpace(row.Cells[memberColumn].Value)
		if member == "" {
			return "", nil, []diagnostic{{Line: messageLine(row.Location), Kind: "malformed-gherkin-member-cases", Excerpt: "examples member case is empty"}}
		}
		cases = append(cases, scenarioMemberCase{Line: messageLine(row.Location), Member: member, Source: "examples-" + memberName})
	}
	return memberName, cases, nil
}

func messageLine(location *messages.Location) int {
	if location == nil || location.Line < 1 || location.Line > int64(^uint(0)>>1) {
		return 1
	}
	return int(location.Line)
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
	if !utf8.Valid(data) {
		return report{}, fmt.Errorf("decode %s: report input is not valid UTF-8", path)
	}
	inventoryPayloadPresent, err := validateBoundedReportJSON(data, reflect.TypeFor[report](), defaultReportJSONLimits())
	if err != nil {
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
	decoded.inventoryPayloadPresent = inventoryPayloadPresent
	return decoded, nil
}

type jsonResourceLimits struct {
	depth                int
	tokens               int
	elements             int
	aggregateStringBytes int
	stringBytes          int
}

func defaultReportJSONLimits() jsonResourceLimits {
	return jsonResourceLimits{
		depth:                maxJSONDepth,
		tokens:               maxJSONTokens,
		elements:             maxJSONElements,
		aggregateStringBytes: maxJSONAggregateStringBytes,
		stringBytes:          maxJSONStringBytes,
	}
}

type jsonResourceBudget struct {
	limits                  jsonResourceLimits
	tokens                  int
	elements                int
	aggregateStringBytes    int
	inventoryPayloadPresent bool
}

type jsonValueLimit struct {
	items       int
	stringBytes int
}

func (budget *jsonResourceBudget) token(decoder *json.Decoder) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if budget.tokens >= budget.limits.tokens {
		return nil, fmt.Errorf("JSON token count exceeds %d", budget.limits.tokens)
	}
	budget.tokens++
	return token, nil
}

func (budget *jsonResourceBudget) consumeSyntheticToken() error {
	if budget.tokens >= budget.limits.tokens {
		return fmt.Errorf("JSON token count exceeds %d", budget.limits.tokens)
	}
	budget.tokens++
	return nil
}

func (budget *jsonResourceBudget) consumeElement(path string, localCount, localLimit int) error {
	if localLimit <= 0 {
		return fmt.Errorf("JSON schema collection %s has no positive item ceiling", path)
	}
	if localCount >= localLimit {
		return fmt.Errorf("JSON collection %s exceeds %d elements", path, localLimit)
	}
	if budget.elements >= budget.limits.elements {
		return fmt.Errorf("JSON aggregate element count exceeds %d", budget.limits.elements)
	}
	budget.elements++
	return nil
}

func (budget *jsonResourceBudget) consumeString(path, value string, fieldLimit int) error {
	limit := fieldLimit
	if limit <= 0 || limit > budget.limits.stringBytes {
		limit = budget.limits.stringBytes
	}
	if len([]byte(value)) > limit {
		return fmt.Errorf("JSON string %s exceeds %d bytes", path, limit)
	}
	if len(value) > budget.limits.aggregateStringBytes-budget.aggregateStringBytes {
		return fmt.Errorf("JSON aggregate decoded string content exceeds %d bytes", budget.limits.aggregateStringBytes)
	}
	budget.aggregateStringBytes += len(value)
	return nil
}

// validateBoundedReportJSON walks the report once with decoder.Token before
// decoding it into Go collections. It combines duplicate/depth/trailing checks,
// exact schema keys, aggregate ceilings, and explicit field collection/string
// ceilings so no RawMessage object graph is materialized during preflight.
func validateBoundedReportJSON(data []byte, valueType reflect.Type, limits jsonResourceLimits) (bool, error) {
	if limits.depth <= 0 || limits.tokens <= 0 || limits.elements <= 0 || limits.aggregateStringBytes <= 0 || limits.stringBytes <= 0 {
		return false, errors.New("JSON resource limits must all be positive")
	}
	if !utf8.Valid(data) {
		return false, errors.New("JSON input is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	budget := &jsonResourceBudget{limits: limits}
	if err := walkBoundedReportJSONValue(decoder, budget, valueType, jsonValueLimit{}, "$", 0); err != nil {
		return false, err
	}
	if _, err := budget.token(decoder); !errors.Is(err, io.EOF) {
		if err == nil {
			return false, errors.New("contains multiple JSON values")
		}
		return false, fmt.Errorf("invalid trailing JSON data: %w", err)
	}
	return budget.inventoryPayloadPresent, nil
}

//nolint:gocyclo // Each JSON grammar and schema shape is kept explicit for fail-closed streaming validation.
func walkBoundedReportJSONValue(decoder *json.Decoder, budget *jsonResourceBudget, valueType reflect.Type, valueLimit jsonValueLimit, path string, depth int) error {
	if depth > budget.limits.depth {
		return fmt.Errorf("JSON nesting exceeds %d levels", budget.limits.depth)
	}
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	token, err := budget.token(decoder)
	if err != nil {
		return err
	}
	if token == nil {
		return nil
	}
	delimiter, structured := token.(json.Delim)
	if !structured {
		if text, ok := token.(string); ok {
			if valueType.Kind() != reflect.String {
				return fmt.Errorf("JSON value %s must match %s", path, valueType)
			}
			return budget.consumeString(path, text, valueLimit.stringBytes)
		}
		if number, ok := token.(json.Number); ok && len(number.String()) > budget.limits.stringBytes {
			return fmt.Errorf("JSON numeric token %s exceeds %d bytes", path, budget.limits.stringBytes)
		}
		return nil
	}
	switch valueType.Kind() { //nolint:exhaustive // The report schema uses only the handled composite kinds.
	case reflect.Struct:
		if delimiter != '{' {
			return fmt.Errorf("JSON value %s must be an object for %s", path, valueType)
		}
		fields := jsonStructFields(valueType)
		keys := map[string]bool{}
		memberCount := 0
		for decoder.More() {
			if err := budget.consumeElement(path, memberCount, len(fields)); err != nil {
				return err
			}
			memberCount++
			keyToken, err := budget.token(decoder)
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if err := budget.consumeString(path+".<key>", key, maxJSONStringBytes); err != nil {
				return err
			}
			if keys[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			keys[key] = true
			field, ok := fields[key]
			if !ok {
				return fmt.Errorf("non-exact or unknown JSON object key %q", key)
			}
			fieldLimit, err := reportJSONFieldLimit(valueType, key, field.Type)
			if err != nil {
				return err
			}
			if path == "$" && (key == "inventory" || key == "features" || key == "seeds") {
				budget.inventoryPayloadPresent = true
			}
			if err := walkBoundedReportJSONValue(decoder, budget, field.Type, fieldLimit, path+"."+key, depth+1); err != nil {
				return err
			}
		}
		closing, err := budget.token(decoder)
		if err != nil || closing != json.Delim('}') {
			return errors.New("JSON object is not properly terminated")
		}
	case reflect.Slice, reflect.Array:
		if delimiter != '[' {
			return fmt.Errorf("JSON value %s must be an array for %s", path, valueType)
		}
		count := 0
		for decoder.More() {
			if err := budget.consumeElement(path, count, valueLimit.items); err != nil {
				return err
			}
			count++
			if err := walkBoundedReportJSONValue(decoder, budget, valueType.Elem(), jsonValueLimit{stringBytes: valueLimit.stringBytes}, path+"[]", depth+1); err != nil {
				return err
			}
		}
		closing, err := budget.token(decoder)
		if err != nil || closing != json.Delim(']') {
			return errors.New("JSON array is not properly terminated")
		}
	case reflect.Map:
		if delimiter != '{' || valueType.Key().Kind() != reflect.String {
			return fmt.Errorf("JSON value %s must be a string-keyed object for %s", path, valueType)
		}
		keys := map[string]bool{}
		count := 0
		for decoder.More() {
			if err := budget.consumeElement(path, count, valueLimit.items); err != nil {
				return err
			}
			count++
			keyToken, err := budget.token(decoder)
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if err := budget.consumeString(path+".<key>", key, valueLimit.stringBytes); err != nil {
				return err
			}
			if keys[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			keys[key] = true
			if err := walkBoundedReportJSONValue(decoder, budget, valueType.Elem(), jsonValueLimit{}, path+"[]", depth+1); err != nil {
				return err
			}
		}
		closing, err := budget.token(decoder)
		if err != nil || closing != json.Delim('}') {
			return errors.New("JSON object is not properly terminated")
		}
	default:
		return fmt.Errorf("JSON value %s unexpectedly contains delimiter %q", path, delimiter)
	}
	return nil
}

func jsonStructFields(valueType reflect.Type) map[string]reflect.StructField {
	fields := make(map[string]reflect.StructField, valueType.NumField())
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
		fields[name] = field
	}
	return fields
}

func reportJSONFieldLimit(owner reflect.Type, name string, fieldType reflect.Type) (jsonValueLimit, error) {
	base := fieldType
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	switch base.Kind() { //nolint:exhaustive // Only variable-sized report fields require explicit resource bounds.
	case reflect.String:
		return jsonValueLimit{stringBytes: maxJSONStringBytes}, nil
	case reflect.Slice, reflect.Array, reflect.Map:
		items, ok := reportJSONCollectionLimit(owner, name)
		if !ok {
			return jsonValueLimit{}, fmt.Errorf("JSON schema collection %s.%s lacks an explicit item ceiling", owner, name)
		}
		stringBytes := maxJSONStringBytes
		if (owner == reflect.TypeFor[report]() || owner == reflect.TypeFor[finding]()) && name == "limitations" {
			stringBytes = maxReportLimitationBytes
		}
		return jsonValueLimit{items: items, stringBytes: stringBytes}, nil
	default:
		return jsonValueLimit{}, nil
	}
}

//nolint:gocyclo // Every variable-sized report field has a deliberate local ceiling in addition to aggregate JSON ceilings.
func reportJSONCollectionLimit(owner reflect.Type, name string) (int, bool) {
	switch owner {
	case reflect.TypeFor[report]():
		switch name {
		case "inventory", "features", "seeds":
			return maxInventoryFiles, true
		case "candidates", "non_candidates":
			return maxReportFindings, true
		case "limitations":
			return maxReportLimitations, true
		}
	case reflect.TypeFor[scope]():
		switch name {
		case "roots", "excluded":
			return maxJSONCollectionItems, true
		case "active_members", "adapter_scopes":
			return maxAdapterScopes, true
		}
	case reflect.TypeFor[adapterScope]():
		switch name {
		case "names":
			return maxAdapterScopeNames, true
		case "evidence":
			return maxAdapterScopeEvidence, true
		}
	case reflect.TypeFor[summary]():
		if name == "by_verdict" {
			return len(verdicts), true
		}
	case reflect.TypeFor[methodology]():
		switch name {
		case "seed_kinds":
			return len(seedKinds), true
		case "reproduce":
			return 256, true
		}
	case reflect.TypeFor[gitTrustInputs]():
		if name == "alternate_object_dirs" {
			return maxAlternateObjectRoutes, true
		}
	case reflect.TypeFor[specFile]():
		switch name {
		case "requirements", "diagnostics":
			return maxFeatureStructuralTokens, true
		case "bdd_features":
			return maxJSONCollectionItems, true
		}
	case reflect.TypeFor[featureFile]():
		switch name {
		case "related_specs", "diagnostics":
			return maxJSONCollectionItems, true
		case "scenarios":
			return maxFeatureScenarios, true
		}
	case reflect.TypeFor[featureScenario]():
		switch name {
		case "outcomes":
			return maxScenarioOutcomes, true
		case "member_cases":
			return maxScenarioCases, true
		}
	case reflect.TypeFor[seed]():
		if name == "evidence" {
			return maxJSONCollectionItems, true
		}
	case reflect.TypeFor[finding]():
		switch name {
		case "current_owners", "material_differences", "recommendation":
			return maxReportFindings, true
		case "evidence":
			return maxJSONCollectionItems, true
		case "applicability":
			return maxAdapterScopes, true
		case "limitations":
			return maxReportLimitations, true
		}
	case reflect.TypeFor[applicability]():
		if name == "evidence" {
			return maxJSONCollectionItems, true
		}
	case reflect.TypeFor[bddImpact]():
		if name == "features" {
			return maxJSONCollectionItems, true
		}
	case reflect.TypeFor[ownershipPlan]():
		if name == "current_owners" {
			return maxReportFindings, true
		}
	case reflect.TypeFor[preservationPlan]():
		switch name {
		case "requirements", "bdd":
			return maxJSONCollectionItems, true
		case "applicability":
			return maxAdapterScopes, true
		}
	}
	return 0, false
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
	if err := validateReportResourceBounds(value); err != nil {
		return nil, fmt.Errorf("inventory JSON exceeds bounded reader resource contract: %w", err)
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
	if !utf8.ValidString(value) {
		return errors.New("cannot encode invalid UTF-8 in JSON artifact")
	}
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

func validateReportResourceBounds(value report) error {
	budget := &jsonResourceBudget{limits: defaultReportJSONLimits()}
	return validateReportValueResources(reflect.ValueOf(value), reflect.TypeFor[report](), jsonValueLimit{}, "$", 0, budget)
}

//nolint:gocyclo // This mirrors the bounded JSON encoder shape so generated reports cannot exceed their own reader's resource contract.
func validateReportValueResources(value reflect.Value, valueType reflect.Type, valueLimit jsonValueLimit, path string, depth int, budget *jsonResourceBudget) error {
	if depth > budget.limits.depth {
		return fmt.Errorf("JSON nesting exceeds %d levels", budget.limits.depth)
	}
	if err := budget.consumeSyntheticToken(); err != nil {
		return err
	}
	for valueType.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
		valueType = valueType.Elem()
	}
	switch valueType.Kind() { //nolint:exhaustive // The report schema contains only the handled JSON-compatible kinds.
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("JSON string %s is not valid UTF-8", path)
		}
		return budget.consumeString(path, value.String(), valueLimit.stringBytes)
	case reflect.Struct:
		memberCount := 0
		for field := range valueType.Fields() {
			if !field.IsExported() {
				continue
			}
			name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fieldValue := value.FieldByIndex(field.Index)
			if strings.Contains(options, "omitempty") && boundedJSONEmpty(fieldValue) {
				continue
			}
			if err := budget.consumeElement(path, memberCount, len(jsonStructFields(valueType))); err != nil {
				return err
			}
			memberCount++
			if err := budget.consumeSyntheticToken(); err != nil {
				return err
			}
			if err := budget.consumeString(path+".<key>", name, maxJSONStringBytes); err != nil {
				return err
			}
			fieldLimit, err := reportJSONFieldLimit(valueType, name, field.Type)
			if err != nil {
				return err
			}
			if err := validateReportValueResources(fieldValue, field.Type, fieldLimit, path+"."+name, depth+1, budget); err != nil {
				return err
			}
		}
		return budget.consumeSyntheticToken()
	case reflect.Slice, reflect.Array:
		if valueType.Kind() == reflect.Slice && value.IsNil() {
			return nil
		}
		for index := 0; index < value.Len(); index++ {
			if err := budget.consumeElement(path, index, valueLimit.items); err != nil {
				return err
			}
			if err := validateReportValueResources(value.Index(index), valueType.Elem(), jsonValueLimit{stringBytes: valueLimit.stringBytes}, path+"[]", depth+1, budget); err != nil {
				return err
			}
		}
		return budget.consumeSyntheticToken()
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		if valueType.Key().Kind() != reflect.String {
			return fmt.Errorf("JSON map %s does not have string keys", path)
		}
		keys := value.MapKeys()
		for index, key := range keys {
			if err := budget.consumeElement(path, index, valueLimit.items); err != nil {
				return err
			}
			if !utf8.ValidString(key.String()) {
				return fmt.Errorf("JSON map key %s is not valid UTF-8", path)
			}
			if err := budget.consumeSyntheticToken(); err != nil {
				return err
			}
			if err := budget.consumeString(path+".<key>", key.String(), valueLimit.stringBytes); err != nil {
				return err
			}
			if err := validateReportValueResources(value.MapIndex(key), valueType.Elem(), jsonValueLimit{}, path+"[]", depth+1, budget); err != nil {
				return err
			}
		}
		return budget.consumeSyntheticToken()
	default:
		return nil
	}
}

//nolint:gocyclo // Exhaustive fail-closed schema validation is intentionally kept as one ordered guard sequence.
func validateReport(report report) error {
	if err := validateReportResourceBounds(report); err != nil {
		return fmt.Errorf("report exceeds bounded JSON resource contract: %w", err)
	}
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
	if !validExclusions(report.Scope.Excluded) || !uniqueStrings(report.Scope.ActiveMembers) || !validAdapterScopes(report.Scope.AdapterScopes, report.Scope.ActiveMembers) {
		return errors.New("scope exclusions, active_members, or adapter_scopes are invalid or inconsistent")
	}
	if strings.TrimSpace(report.Methodology.Collector) == "" || strings.TrimSpace(report.Methodology.SemanticReview) == "" || len(report.Methodology.Reproduce) == 0 {
		return errors.New("methodology requires collector, semantic_review, and reproduce commands")
	}
	if report.Methodology.RuntimeStatus != runtimeStatusUnverified {
		return fmt.Errorf("methodology runtime_status must be %q", runtimeStatusUnverified)
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
	if hasIncompleteAdapterCatalog(report.Limitations) {
		for _, finding := range allFindings(report) {
			if finding.Verdict == "merge-now" || finding.Verdict == "extract-neutral-contract" {
				return fmt.Errorf("positive finding %q is forbidden while the pinned adapter-scope catalog is incomplete", finding.ID)
			}
		}
	}
	for _, finding := range report.Candidates {
		if err := validateFinding(finding, false, active, report.Scope.AdapterScopes); err != nil {
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
		if err := validateFinding(finding, true, active, report.Scope.AdapterScopes); err != nil {
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

func hasIncompleteAdapterCatalog(limitations []string) bool {
	for _, limitation := range limitations {
		switch limitation {
		case activeHarnessUnavailableLimitation,
			activeHarnessUnparseableLimitation,
			deprecatedHarnessUnparseableLimitation,
			adapterCatalogIncompleteLimitation:
			return true
		}
	}
	return false
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
	if !reflect.DeepEqual(semantic.Scope.Roots, inventory.Scope.Roots) ||
		!reflect.DeepEqual(semantic.Scope.Excluded, inventory.Scope.Excluded) ||
		!reflect.DeepEqual(semantic.Scope.AdapterScopes, inventory.Scope.AdapterScopes) {
		return errors.New("semantic report scope roots, exclusions, or adapter scopes do not match the pinned inventory")
	}
	if !sameStringSet(semantic.Scope.ActiveMembers, inventory.Scope.ActiveMembers) {
		return errors.New("semantic report active members do not match the pinned inventory")
	}
	if !reflect.DeepEqual(semantic.Methodology.GitTrustInputs, inventory.Methodology.GitTrustInputs) {
		return errors.New("semantic report Git trust inputs do not match the pinned inventory")
	}
	if semantic.Methodology.RuntimeStatus != inventory.Methodology.RuntimeStatus {
		return errors.New("semantic report runtime status does not match the pinned inventory")
	}
	for _, limitation := range inventory.Limitations {
		if !containsString(semantic.Limitations, limitation) {
			return fmt.Errorf("semantic report omits inventory limitation %q", limitation)
		}
	}

	requirements := map[string]map[requirementKey]string{}
	requirementIDs := map[string]map[string]bool{}
	pinnedRequirementEvidence := map[string][]evidence{}
	features := map[string]featureFile{}
	files := map[string]bool{}
	specFeatures := map[string]map[string]bool{}
	for _, file := range inventory.Inventory {
		files[file.Path] = true
		requirements[file.Path] = map[requirementKey]string{}
		requirementIDs[file.Path] = map[string]bool{}
		pinnedRequirementEvidence[file.Path] = []evidence{}
		specFeatures[file.Path] = map[string]bool{}
		for _, requirement := range file.Requirements {
			requirements[file.Path][requirementKey{line: requirement.Line, id: requirement.ID}] = requirement.Excerpt
			requirementIDs[file.Path][requirement.ID] = true
			pinnedRequirementEvidence[file.Path] = append(pinnedRequirementEvidence[file.Path], evidenceForRequirement(file.Path, requirement))
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
		if positive {
			if err := validateSharedContractFeature(finding, features, specFeatures); err != nil {
				return err
			}
			if err := validateOwnershipPlanAgainstInventory(finding, features, specFeatures, requirementIDs, pinnedRequirementEvidence); err != nil {
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

//nolint:gocyclo // Exact reciprocal, scenario, outcome, and member proof is one fail-closed gate.
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
	if f.BDD.SharedContractScenario == nil {
		return fmt.Errorf("finding %q shared BDD feature %q requires an exact scenario reference", f.ID, f.BDD.SharedContractFeature)
	}
	var selected *featureScenario
	for index := range feature.Scenarios {
		scenario := &feature.Scenarios[index]
		if scenario.Line == f.BDD.SharedContractScenario.Line && scenario.Name == f.BDD.SharedContractScenario.Name {
			if selected != nil {
				return fmt.Errorf("finding %q shared BDD scenario reference is ambiguous", f.ID)
			}
			selected = scenario
		}
	}
	if selected == nil {
		return fmt.Errorf("finding %q shared BDD scenario %q at line %d is absent from the pinned feature inventory", f.ID, f.BDD.SharedContractScenario.Name, f.BDD.SharedContractScenario.Line)
	}
	for _, item := range feature.Diagnostics {
		if item.Kind == "malformed-gherkin-structure" || item.Kind == "gherkin-structure-limit-exceeded" || (item.Kind == "malformed-gherkin-member-cases" && diagnosticBelongsToScenario(item, *selected, feature.Scenarios)) {
			return fmt.Errorf("finding %q shared BDD feature %q has incomplete structural inventory: %s", f.ID, f.BDD.SharedContractFeature, item.Kind)
		}
	}
	if selected.Kind != "scenario-outline" || len(selected.Outcomes) == 0 || (selected.MemberColumn != "harness" && selected.MemberColumn != "member") || !selected.UsesMemberPlaceholder {
		return fmt.Errorf("finding %q shared BDD scenario must be an observable outline that uses its harness/member examples column", f.ID)
	}
	expectedMembers := map[string]bool{}
	for _, item := range f.Applicability {
		if item.Disposition != "not-applicable" {
			expectedMembers[item.Member] = true
		}
	}
	if len(expectedMembers) < 2 {
		return fmt.Errorf("finding %q shared BDD scenario requires at least two applicable member cases", f.ID)
	}
	caseMembers := map[string]bool{}
	for _, item := range selected.MemberCases {
		if item.Source != "examples-"+selected.MemberColumn || caseMembers[item.Member] {
			return fmt.Errorf("finding %q shared BDD scenario has invalid or duplicate member cases", f.ID)
		}
		caseMembers[item.Member] = true
	}
	if !sameStringSet(mapKeys(expectedMembers), mapKeys(caseMembers)) {
		return fmt.Errorf("finding %q shared BDD scenario member cases must exactly cover applicable members", f.ID)
	}
	return nil
}

// diagnosticBelongsToScenario uses the next projected scenario as a stable
// upper boundary. Example-table diagnostics are scenario-local: an unrelated
// malformed outline must not invalidate the explicitly selected shared proof.
func diagnosticBelongsToScenario(item diagnostic, selected featureScenario, scenarios []featureScenario) bool {
	if item.Line < selected.Line {
		return false
	}
	nextLine := int(^uint(0) >> 1)
	for _, scenario := range scenarios {
		if scenario.Line > selected.Line && scenario.Line < nextLine {
			nextLine = scenario.Line
		}
	}
	return item.Line < nextLine
}

//nolint:gocyclo // Exact requirement and BDD retirement coverage stays in one pinned-evidence gate.
func validateOwnershipPlanAgainstInventory(f finding, features map[string]featureFile, specFeatures map[string]map[string]bool, requirementIDs map[string]map[string]bool, pinnedRequirements map[string][]evidence) error {
	for _, entry := range f.OwnershipPlan.CurrentOwners {
		if entry.Action != "retire-normative-ownership" {
			continue
		}
		expectedRequirements := map[string]bool{}
		for _, item := range pinnedRequirements[entry.Path] {
			expectedRequirements[evidenceKey(item)] = true
		}
		mappedRequirements := map[string]bool{}
		for _, mapping := range entry.Preservation.Requirements {
			key := evidenceKey(mapping.Source)
			if !expectedRequirements[key] || mappedRequirements[key] {
				return fmt.Errorf("finding %q retired owner %q requirement preservation must exactly match every pinned requirement", f.ID, entry.Path)
			}
			mappedRequirements[key] = true
			if f.ProposedOwner.State == "existing" && !requirementIDs[f.ProposedOwner.Path][mapping.TargetID] {
				return fmt.Errorf("finding %q preservation target ID %q is absent from pinned proposed owner %q", f.ID, mapping.TargetID, f.ProposedOwner.Path)
			}
		}
		if !sameStringSet(mapKeys(expectedRequirements), mapKeys(mappedRequirements)) {
			return fmt.Errorf("finding %q retired owner %q requirement preservation must exactly cover all pinned requirements", f.ID, entry.Path)
		}
		for _, mapping := range entry.Preservation.BDD {
			feature, ok := features[mapping.Feature]
			if !ok || !containsString(feature.RelatedSpecs, entry.Path) || !specFeatures[entry.Path][mapping.Feature] {
				return fmt.Errorf("finding %q preservation BDD feature %q must reciprocally link retired owner %q", f.ID, mapping.Feature, entry.Path)
			}
		}
		expectedFeatures := map[string]bool{}
		for featurePath := range specFeatures[entry.Path] {
			feature, ok := features[featurePath]
			if ok && containsString(feature.RelatedSpecs, entry.Path) {
				expectedFeatures[featurePath] = true
			}
		}
		mappedFeatures := map[string]bool{}
		for _, mapping := range entry.Preservation.BDD {
			mappedFeatures[mapping.Feature] = true
		}
		if !sameStringSet(mapKeys(expectedFeatures), mapKeys(mappedFeatures)) {
			return fmt.Errorf("finding %q preservation BDD mappings for retired owner %q must exactly cover all pinned reciprocal features", f.ID, entry.Path)
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

//nolint:gocyclo // Feature and projected Gherkin invariants are validated together.
func validateFeatures(features []featureFile) error {
	seen := map[string]bool{}
	for _, feature := range features {
		if !validFeaturePath(feature.Path) || !digestPattern.MatchString(feature.SHA256) || seen[feature.Path] || !uniqueSpecPaths(feature.RelatedSpecs) {
			return fmt.Errorf("feature inventory has invalid or duplicate path %q", feature.Path)
		}
		seen[feature.Path] = true
		if len(feature.Scenarios) > maxFeatureScenarios {
			return fmt.Errorf("feature inventory %s exceeds bounded scenario count", feature.Path)
		}
		seenScenarios := map[string]bool{}
		for _, scenario := range feature.Scenarios {
			key := strconv.Itoa(scenario.Line) + "\x00" + scenario.Name
			if scenario.Line < 1 || strings.TrimSpace(scenario.Name) == "" || len([]byte(scenario.Name)) > maxScenarioNameBytes || !featureScenarioKinds[scenario.Kind] || seenScenarios[key] || len(scenario.Outcomes) > maxScenarioOutcomes || len(scenario.MemberCases) > maxScenarioCases {
				return fmt.Errorf("feature inventory scenario in %s is invalid or duplicated", feature.Path)
			}
			seenScenarios[key] = true
			for _, outcome := range scenario.Outcomes {
				if outcome.Line < scenario.Line || strings.TrimSpace(outcome.Text) == "" {
					return fmt.Errorf("feature inventory outcome in %s is invalid", feature.Path)
				}
			}
			if scenario.MemberColumn != "" && scenario.MemberColumn != "harness" && scenario.MemberColumn != "member" {
				return fmt.Errorf("feature inventory member column in %s is invalid", feature.Path)
			}
			for _, memberCase := range scenario.MemberCases {
				if memberCase.Line < scenario.Line || strings.TrimSpace(memberCase.Member) == "" || memberCase.Source != "examples-"+scenario.MemberColumn {
					return fmt.Errorf("feature inventory member case in %s is invalid", feature.Path)
				}
			}
		}
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
func validateFinding(f finding, nonCandidate bool, active map[string]bool, adapterScopes []adapterScope) error {
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
	if f.Verdict == "extract-neutral-contract" && f.Relationship != "same-observable" && f.Relationship != "overlapping-observables" {
		return fmt.Errorf("extract-neutral-contract finding %q requires same or overlapping observables", f.ID)
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
	if f.ApplicabilityBasis != "" && !applicabilityBases[f.ApplicabilityBasis] {
		return fmt.Errorf("finding %q has invalid applicability basis %q", f.ID, f.ApplicabilityBasis)
	}
	if !bddConsequences[f.BDD.Consequence] || !uniqueRelativeFeaturePaths(f.BDD.Features) {
		return fmt.Errorf("finding %q has invalid BDD impact", f.ID)
	}
	if f.BDD.SharedContractFeature != "" && (!validFeaturePath(f.BDD.SharedContractFeature) || !containsString(f.BDD.Features, f.BDD.SharedContractFeature)) {
		return fmt.Errorf("finding %q shared BDD feature must be a selected feature", f.ID)
	}
	if f.BDD.SharedContractScenario != nil && (f.BDD.SharedContractScenario.Line < 1 || strings.TrimSpace(f.BDD.SharedContractScenario.Name) == "" || len([]byte(f.BDD.SharedContractScenario.Name)) > maxScenarioNameBytes) {
		return fmt.Errorf("finding %q shared BDD scenario reference is invalid", f.ID)
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
		if !allProductSpecPaths(ownerPaths(f.CurrentOwners)) || f.ProposedOwner == nil || !isProductSpecPath(f.ProposedOwner.Path) || !ownerStates[f.ProposedOwner.State] || len(f.BDD.Features) == 0 {
			return fmt.Errorf("positive finding %q requires product SPEC current owners, a product SPEC proposed owner state, and BDD features", f.ID)
		}
		switch f.Verdict {
		case "merge-now":
			if len(f.CurrentOwners) < 2 || distinctProductSpecPaths(f.Evidence) < 2 || f.ProposedOwner.State != "existing" {
				return fmt.Errorf("merge-now finding %q requires at least two pinned current owners and an existing proposed owner", f.ID)
			}
		case "extract-neutral-contract":
			if len(f.CurrentOwners) == 1 && f.ProposedOwner.State != "new" {
				return fmt.Errorf("single-owner extract-neutral-contract finding %q requires a new proposed owner", f.ID)
			}
		}
		if strings.TrimSpace(f.OwnershipCompleteness) == "" || strings.TrimSpace(f.ProposedOwner.Rationale) == "" || strings.TrimSpace(f.ProposedOwner.NeutralityRationale) == "" {
			return fmt.Errorf("positive finding %q requires owner-completeness plus proposed-owner and neutrality rationales", f.ID)
		}
		if len(adapterScopes) == 0 {
			return fmt.Errorf("positive finding %q requires a pinned adapter-scope catalog", f.ID)
		}
		if isHarnessSurfacePath(f.ProposedOwner.Path, adapterScopes) {
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
		if f.ApplicabilityBasis != "active-members" {
			return fmt.Errorf("positive finding %q requires active-members applicability; implementation-only is non-positive", f.ID)
		}
		if strings.TrimSpace(f.ApplicabilityRationale) == "" {
			return fmt.Errorf("positive finding %q requires applicability rationale", f.ID)
		}
		for _, entry := range f.Applicability {
			if entry.Disposition == "unknown" {
				return fmt.Errorf("positive finding %q has unresolved applicability for active member %q", f.ID, entry.Member)
			}
		}
		if len(active) == 0 || len(seenMembers) != len(active) {
			return fmt.Errorf("positive finding %q must cover every pinned active member", f.ID)
		}
		if strings.TrimSpace(f.BDD.SharedContractFeature) == "" || f.BDD.SharedContractScenario == nil {
			return fmt.Errorf("positive finding %q requires bdd.shared_contract_feature and bdd.shared_contract_scenario regardless of classification", f.ID)
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
	if !positive && (f.ProposedOwner != nil || f.OwnershipPlan != nil || f.BDD.SharedContractFeature != "" || f.BDD.SharedContractScenario != nil) {
		return fmt.Errorf("non-positive finding %q cannot carry a canonical owner, ownership plan, or shared BDD proof", f.ID)
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
	if len(ownerEvidence) == 0 || len(preservation.Requirements) < len(ownerEvidence) {
		return fmt.Errorf("retired owner %q must map at least every cited source requirement", entry.Path)
	}
	citedSourceKeys := map[string]bool{}
	for _, item := range ownerEvidence {
		citedSourceKeys[evidenceKey(item)] = true
	}
	seenSources := map[string]bool{}
	for _, mapping := range preservation.Requirements {
		key := evidenceKey(mapping.Source)
		if mapping.Source.Path != entry.Path || mapping.Source.RequirementID == "" || mapping.Source.Line < 1 || strings.TrimSpace(mapping.Source.Excerpt) == "" || seenSources[key] || strings.TrimSpace(mapping.TargetID) == "" || !targetStates[mapping.TargetState] || !preservationStrategies[mapping.Strategy] {
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
	for key := range citedSourceKeys {
		if !seenSources[key] {
			return fmt.Errorf("retired owner %q must map every cited source requirement", entry.Path)
		}
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
		if !validFeaturePath(mapping.Feature) || mapping.SourceOwner != entry.Path || mapping.TargetOwner != f.ProposedOwner.Path || seenBDD[mapping.Feature] {
			return fmt.Errorf("retired owner %q has invalid BDD preservation", entry.Path)
		}
		if mapping.Feature == f.BDD.SharedContractFeature {
			sharedMapped = true
		}
		seenBDD[mapping.Feature] = true
	}
	if !sharedMapped {
		return fmt.Errorf("retired owner %q preservation must include bdd.shared_contract_feature", entry.Path)
	}
	return nil
}

func evidenceKey(item evidence) string {
	return item.Path + "\x00" + strconv.Itoa(item.Line) + "\x00" + item.RequirementID + "\x00" + item.Excerpt
}

// isHarnessSurfacePath recognizes only exact config roots and normalized path
// segments derived from the pinned adapter catalog. A finite suffix catalog
// covers concrete adapter packages without classifying arbitrary internal/ or
// pkg/ directories.
func isHarnessSurfacePath(path string, scopes []adapterScope) bool {
	exact, normalized := adapterSurfaceSegments(scopes)
	for segment := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		lower := strings.ToLower(segment)
		normalizedSegment := normalizeHarnessAlias(segment)
		if exact[lower] || normalized[normalizedSegment] {
			return true
		}
	}
	return false
}

func adapterSurfaceSegments(scopes []adapterScope) (map[string]bool, map[string]bool) {
	exact := map[string]bool{}
	normalized := map[string]bool{}
	for _, scope := range scopes {
		for _, name := range scope.Names {
			lower := strings.ToLower(strings.TrimSpace(name))
			if strings.HasPrefix(lower, ".") {
				exact[lower] = true
				continue
			}
			base := normalizeHarnessAlias(name)
			if base == "" {
				continue
			}
			bases := map[string]bool{base: true}
			for _, suffix := range []string{"cli", "code", "agent", "harness"} {
				trimmed := strings.TrimSuffix(base, suffix)
				if trimmed != "" {
					bases[trimmed] = true
				}
			}
			for alias := range bases {
				normalized[alias] = true
				for _, suffix := range []string{"adapter", "archive", "command", "control", "hooks", "session", "ui"} {
					normalized[alias+suffix] = true
				}
			}
		}
	}
	return exact, normalized
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
	proposedDirectory := pathDirectory(proposed)
	for _, owner := range owners {
		ownerDirectory := pathDirectory(owner)
		if isStrictDirectoryDescendant(proposedDirectory, ownerDirectory) {
			return true
		}
	}
	return false
}

func pathDirectory(path string) string {
	directory := filepath.ToSlash(filepath.Dir(path))
	if directory == "" {
		return "."
	}
	return directory
}

func isStrictDirectoryDescendant(path, ancestor string) bool {
	if path == ancestor {
		return false
	}
	if ancestor == "." {
		return path != "."
	}
	return strings.HasPrefix(path, ancestor+"/")
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
	featureDiagnosticKinds = map[string]bool{"missing-feature-spec-reference": true, "malformed-feature-spec-reference": true, "ambiguous-feature-spec-reference": true, "missing-feature-spec": true, "nonreciprocal-feature-spec": true, "malformed-gherkin-structure": true, "malformed-gherkin-member-cases": true, "gherkin-structure-limit-exceeded": true}
	featureScenarioKinds   = map[string]bool{"scenario": true, "scenario-outline": true}
	adapterScopeKinds      = map[string]bool{"harness": true, "compatibility-adapter": true}
	adapterScopeLifecycles = map[string]bool{"active": true, "deprecated": true, "compatibility": true}
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

//nolint:gocyclo // Catalog bounds, evidence, and active-lifecycle coherence form one schema check.
func validAdapterScopes(scopes []adapterScope, activeMembers []string) bool {
	if len(scopes) > maxAdapterScopes {
		return false
	}
	seen := map[string]bool{}
	active := map[string]bool{}
	for _, scope := range scopes {
		if strings.TrimSpace(scope.ID) == "" || seen[scope.ID] || !adapterScopeKinds[scope.Kind] || !adapterScopeLifecycles[scope.Lifecycle] || len(scope.Names) == 0 || len(scope.Names) > maxAdapterScopeNames || len(scope.Evidence) == 0 || len(scope.Evidence) > maxAdapterScopeEvidence || !uniqueNonemptyStrings(scope.Names) || !containsString(scope.Names, scope.ID) {
			return false
		}
		seen[scope.ID] = true
		if scope.Lifecycle == "active" {
			active[scope.ID] = true
		}
		for _, item := range scope.Evidence {
			if !validPath(item.Path) || item.Line < 1 || strings.TrimSpace(item.Excerpt) == "" {
				return false
			}
		}
	}
	return sameStringSet(activeMembers, mapKeys(active))
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
	if !utf8.ValidString(path) || path == "" || strings.TrimSpace(path) != path || filepath.IsAbs(path) || strings.Contains(path, "\\") || strings.ContainsRune(path, '\x00') {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return path == clean && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
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
	out.WriteString("<span class=\"label\">Pinned adapter-scope catalog</span>")
	if len(audit.Scope.AdapterScopes) == 0 {
		out.WriteString("<p class=\"empty\">No adapter-scope metadata was available.</p>")
	} else {
		out.WriteString("<ul class=\"compact\">")
		for _, scope := range audit.Scope.AdapterScopes {
			fmt.Fprintf(out, "<li><code>%s</code> · %s · %s; names: <code>%s</code>", esc(scope.ID), esc(scope.Kind), esc(scope.Lifecycle), esc(strings.Join(scope.Names, ", ")))
			out.WriteString("<ul class=\"compact\">")
			for _, item := range scope.Evidence {
				fmt.Fprintf(out, "<li><code>%s:%d</code> — %s</li>", esc(item.Path), item.Line, esc(item.Excerpt))
			}
			out.WriteString("</ul></li>")
		}
		out.WriteString("</ul>")
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
		renderFinding(out, finding, inventory)
	}
	out.WriteString("</div></section>")

	out.WriteString("<section id=\"boundaries\"><h2>Keep-separate controls</h2><p class=\"section-intro\">These near misses constrain the audit: shared vocabulary, identifiers, or lifecycle shape do not erase adapter, platform, or domain boundaries.</p>")
	if len(audit.NonCandidates) == 0 {
		out.WriteString("<p class=\"empty\">None recorded.</p>")
	}
	for _, finding := range audit.NonCandidates {
		renderFinding(out, finding, inventory)
	}
	out.WriteString("</section>")

	out.WriteString("<section id=\"method\"><h2>Method and reproducibility</h2><div class=\"two-col\"><div class=\"panel\"><span class=\"label\">Collector</span><p><code>")
	fmt.Fprintf(out, "%s", esc(audit.Methodology.Collector))
	out.WriteString("</code></p><span class=\"label\">Semantic review</span><p>")
	fmt.Fprintf(out, "%s", esc(audit.Methodology.SemanticReview))
	out.WriteString("</p><span class=\"label\">Runtime status</span><p><span class=\"tag warn\">")
	fmt.Fprintf(out, "%s", esc(audit.Methodology.RuntimeStatus))
	out.WriteString("</span></p><span class=\"label\">Git evidence trust boundary</span><p>")
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

func renderFinding(out *boundedHTMLBuilder, finding finding, inventory *report) {
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
	if finding.BDD.SharedContractScenario != nil {
		fmt.Fprintf(out, "<p><strong>Shared contract scenario:</strong> <code>%s:%d</code> — %s</p>", esc(finding.BDD.SharedContractFeature), finding.BDD.SharedContractScenario.Line, esc(finding.BDD.SharedContractScenario.Name))
		renderSelectedScenarioProof(out, finding, inventory)
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

func renderSelectedScenarioProof(out *boundedHTMLBuilder, finding finding, inventory *report) {
	if inventory == nil || finding.BDD.SharedContractScenario == nil {
		return
	}
	var selected *featureScenario
	for featureIndex := range inventory.Features {
		feature := &inventory.Features[featureIndex]
		if feature.Path != finding.BDD.SharedContractFeature {
			continue
		}
		for scenarioIndex := range feature.Scenarios {
			scenario := &feature.Scenarios[scenarioIndex]
			if scenario.Line == finding.BDD.SharedContractScenario.Line && scenario.Name == finding.BDD.SharedContractScenario.Name {
				selected = scenario
				break
			}
		}
		break
	}
	if selected == nil {
		return
	}
	fmt.Fprintf(out, "<div class=\"panel\"><span class=\"label\">Pinned selected-scenario proof</span><p><strong>Kind:</strong> <code>%s</code><br><strong>Member column:</strong> <code>%s</code><br><strong>Uses member placeholder:</strong> <code>%t</code></p>", esc(selected.Kind), esc(selected.MemberColumn), selected.UsesMemberPlaceholder)
	out.WriteString("<strong>Observable outcomes</strong><ul class=\"compact\">")
	for _, outcome := range selected.Outcomes {
		fmt.Fprintf(out, "<li><code>line %d</code> — %s</li>", outcome.Line, esc(outcome.Text))
	}
	out.WriteString("</ul><strong>Examples member cases</strong><table class=\"matrix\"><thead><tr><th>Line</th><th>Member</th><th>Source</th></tr></thead><tbody>")
	for _, memberCase := range selected.MemberCases {
		fmt.Fprintf(out, "<tr><td>%d</td><td><code>%s</code></td><td><code>%s</code></td></tr>", memberCase.Line, esc(memberCase.Member), esc(memberCase.Source))
	}
	out.WriteString("</tbody></table></div>")
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
				fmt.Fprintf(out, "<li><code>%s:%d</code> <span class=\"pill\">%s</span><br><span class=\"source-note\">%s</span><br>→ <code>%s</code> (%s; %s)</li>", esc(mapping.Source.Path), mapping.Source.Line, esc(mapping.Source.RequirementID), esc(mapping.Source.Excerpt), esc(mapping.TargetID), esc(mapping.TargetState), esc(mapping.Strategy))
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
