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
	pathpkg "path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
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

const (
	inventoryDocumentKind = "inventory"
	ledgerDocumentKind    = "decision-ledger"
)

const gitEvidenceTrustDisclosure = "The collector trusts the PATH-selected Git executable, repository Git metadata, the common object store, and configured object alternates. It disables replacement objects and lazy fetching and resolves evidence from the pinned commit through Git; it does not independently authenticate source provenance or object-store integrity."

const collectorExecutionDisclosure = "Collector execution is self-reported runtime/build metadata only. It does not attest the collector source, compiler, repository, or binary provenance."

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
	maxSupportingEvidenceRecords       = 512
	maxSupportingEvidencePaths         = 256
	maxSupportingEvidenceBlobBytes     = 4 * 1024 * 1024
	maxSupportingEvidenceBytes         = 32 * 1024 * 1024
	maxGitPathBytes                    = 4096
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
	SchemaVersion                string              `json:"schema_version"`
	DocumentKind                 string              `json:"document_kind"`
	InventoryRef                 string              `json:"inventory_ref,omitempty"`
	Snapshot                     snapshot            `json:"snapshot"`
	Scope                        scope               `json:"scope"`
	Summary                      summary             `json:"summary"`
	Methodology                  methodology         `json:"methodology"`
	Inventory                    []specFile          `json:"inventory,omitempty"`
	Features                     []featureFile       `json:"features,omitempty"`
	Seeds                        []seed              `json:"seeds,omitempty"`
	Candidates                   []finding           `json:"candidates,omitempty"`
	NonCandidates                []finding           `json:"non_candidates,omitempty"`
	Exclusions                   []reviewExclusion   `json:"reviewer_exclusions,omitempty"`
	Limitations                  []string            `json:"limitations"`
	CollectorExecution           *collectorExecution `json:"collector_execution,omitempty"`
	CollectorExecutionDisclosure string              `json:"collector_execution_disclosure,omitempty"`

	// inventoryPayloadPresent preserves whether a decoded semantic document
	// embedded an inventory field as null. Non-nil fields are caught directly.
	inventoryPayloadPresent bool
	decisionPayloadPresent  bool
}

// inventoryDocument is the only persisted collector output. It deliberately
// contains no reviewer classification, exclusion, finding, or recommendation.
type inventoryDocument struct {
	SchemaVersion                string             `json:"schema_version"`
	DocumentKind                 string             `json:"document_kind"`
	Snapshot                     snapshot           `json:"snapshot"`
	Scope                        collectionScope    `json:"scope"`
	Summary                      inventorySummary   `json:"summary"`
	Methodology                  methodology        `json:"methodology"`
	CollectorExecution           collectorExecution `json:"collector_execution"`
	CollectorExecutionDisclosure string             `json:"collector_execution_disclosure"`
	Inventory                    []specFile         `json:"inventory"`
	Features                     []featureFile      `json:"features"`
	Seeds                        []seed             `json:"seeds"`
	Limitations                  []string           `json:"limitations"`
}

type collectionScope struct {
	Roots         []string `json:"roots"`
	ActiveMembers []string `json:"active_members"`
}

type inventorySummary struct {
	SpecFiles    int `json:"spec_files"`
	Requirements int `json:"requirements"`
	Diagnostics  int `json:"diagnostics"`
}

// decisionLedger is the only persisted reviewer input. It binds immutable
// collection through inventory_ref and cannot repeat corpus facts or collector
// trust inputs.
type decisionLedger struct {
	SchemaVersion string            `json:"schema_version"`
	DocumentKind  string            `json:"document_kind"`
	InventoryRef  string            `json:"inventory_ref"`
	ReviewScope   reviewScope       `json:"review_scope"`
	Summary       decisionSummary   `json:"summary"`
	Methodology   reviewMethodology `json:"methodology"`
	Candidates    []finding         `json:"candidates"`
	NonCandidates []finding         `json:"non_candidates"`
	Limitations   []string          `json:"limitations"`
}

type reviewScope struct {
	Exclusions []reviewExclusion `json:"exclusions"`
}
type decisionSummary struct {
	CandidateCount int            `json:"candidate_count"`
	ByVerdict      map[string]int `json:"by_verdict"`
}
type reviewMethodology struct {
	SemanticReview string   `json:"semantic_review"`
	Reproduce      []string `json:"reproduce"`
}

// collectorExecution is a self-reported runtime disclosure. It is not an
// attestation of the source, compiler, repository, or binary provenance.
type collectorExecution struct {
	BuildInfoAvailable   bool   `json:"build_info_available"`
	VCSMetadataAvailable bool   `json:"vcs_metadata_available"`
	ModulePath           string `json:"module_path,omitempty"`
	VCSRevision          string `json:"vcs_revision,omitempty"`
	VCSModified          *bool  `json:"vcs_modified,omitempty"`
	GoToolchain          string `json:"go_toolchain"`
	GOOS                 string `json:"goos"`
	GOARCH               string `json:"goarch"`
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

// reviewExclusion records reviewer classification without suppressing
// collection. It is never deletion authority or a waiver for owner evidence.
type reviewExclusion struct {
	Path               string               `json:"path"`
	Classification     string               `json:"classification"`
	Rationale          string               `json:"rationale"`
	SupportingEvidence []supportingEvidence `json:"supporting_evidence"`
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

const (
	pendingMaintainerApproval        = "pending-maintainer-approval"
	retainDistinctContract           = "retain-distinct-contract"
	retireNormativeOwnership         = "retire-normative-ownership"
	retireSelectedNormativeOwnership = "retire-selected-normative-ownership"
	retainDistinct                   = "retain-distinct"
	transferToProposedOwner          = "transfer-to-proposed-owner"
	representAsApplicability         = "represent-as-applicability"
	preservePendingSeparateAudit     = "preserve-in-place-pending-separate-audit"
)

type ownershipPlan struct {
	Status                 string                    `json:"status"`
	DeletionAuthority      bool                      `json:"deletion_authority"`
	OwnerActions           []ownerPreservation       `json:"owner_actions"`
	Requirements           []requirementPreservation `json:"requirements"`
	Features               []featurePreservation     `json:"features"`
	BDDPlannedTransfers    []plannedBDDTransfer      `json:"bdd_planned_transfers"`
	ApplicabilityBasis     string                    `json:"applicability_basis"`
	ApplicabilityRationale string                    `json:"applicability_rationale"`
	Applicability          []applicability           `json:"applicability"`
}

type ownerPreservation struct {
	OwnerPath   string `json:"owner_path"`
	Disposition string `json:"disposition"`
	Rationale   string `json:"rationale"`
}

type requirementPreservation struct {
	ContractEvidence    contractEvidence `json:"contract_evidence"`
	Disposition         string           `json:"disposition"`
	TargetPath          string           `json:"target_path"`
	TargetRequirementID string           `json:"target_requirement_id"`
	TargetState         string           `json:"target_state"`
	Rationale           string           `json:"rationale"`
}

type featurePreservation struct {
	SourceOwner string `json:"source_owner"`
	Path        string `json:"path"`
	Disposition string `json:"disposition"`
	TargetPath  string `json:"target_path"`
	TargetState string `json:"target_state"`
	Rationale   string `json:"rationale"`
}

// plannedBDDTransfer records a maintainer-pending traceability migration for
// a current owner that the selected pinned feature does not yet reciprocally
// name. It is reviewer-authored semantic evidence, not proof that the feature
// covers the same observable or that the transfer has happened.
type plannedBDDTransfer struct {
	SourceOwner      string               `json:"source_owner"`
	TargetOwner      string               `json:"target_owner"`
	TargetFeature    string               `json:"target_feature"`
	BehaviorEvidence []supportingEvidence `json:"behavior_evidence"`
	Rationale        string               `json:"rationale"`
}

type seed struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Key      string     `json:"key"`
	Evidence []evidence `json:"evidence"`
}

type finding struct {
	ID                     string               `json:"id"`
	Rank                   int                  `json:"rank,omitempty"`
	Title                  string               `json:"title"`
	Verdict                string               `json:"verdict"`
	Relationship           string               `json:"relationship"`
	Classification         string               `json:"classification"`
	Confidence             string               `json:"confidence"`
	Strength               string               `json:"strength"`
	CurrentOwners          []ownerClaim         `json:"current_owners"`
	OwnershipCompleteness  string               `json:"ownership_completeness,omitempty"`
	ProposedOwner          *proposedOwnerClaim  `json:"proposed_owner,omitempty"`
	SharedOutcome          string               `json:"shared_outcome"`
	MaterialDifferences    []string             `json:"material_differences"`
	ContractEvidence       []contractEvidence   `json:"contract_evidence,omitempty"`
	SupportingEvidence     []supportingEvidence `json:"supporting_evidence,omitempty"`
	Evidence               []evidence           `json:"-"`
	ApplicabilityBasis     string               `json:"applicability_basis,omitempty"`
	ApplicabilityRationale string               `json:"applicability_rationale,omitempty"`
	Applicability          []applicability      `json:"applicability"`
	BDD                    bddImpact            `json:"bdd"`
	Recommendation         []string             `json:"recommendation"`
	Risk                   string               `json:"risk"`
	Limitations            []string             `json:"limitations"`
	Decision               string               `json:"decision"`
	DecisionStatus         string               `json:"decision_status"`
	OwnershipPlan          *ownershipPlan       `json:"ownership_plan,omitempty"`
	Boundary               string               `json:"boundary,omitempty"`
}

type evidence struct {
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	Line          int    `json:"line"`
	RequirementID string `json:"requirement_id,omitempty"`
	Excerpt       string `json:"excerpt"`
}

// contractEvidence is the only persisted evidence type that can establish an
// owner, proposed owner, or applicability claim.
type contractEvidence struct {
	Path          string `json:"path"`
	Line          int    `json:"line"`
	RequirementID string `json:"requirement_id"`
	Excerpt       string `json:"excerpt"`
}

type contractEvidenceKey struct {
	path          string
	line          int
	requirementID string
	excerpt       string
}

// supportingEvidence is a pinned tracked-blob citation. Its shape cannot
// carry a requirement identifier, preventing it from impersonating a contract.
type supportingEvidence struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Excerpt string `json:"excerpt"`
}

type applicability struct {
	Member             string               `json:"member"`
	Disposition        string               `json:"disposition"`
	ContractEvidence   []contractEvidence   `json:"contract_evidence,omitempty"`
	SupportingEvidence []supportingEvidence `json:"supporting_evidence,omitempty"`
	Evidence           []evidence           `json:"-"`
}

type bddImpact struct {
	Features         []string             `json:"features"`
	PlannedTransfers []plannedBDDTransfer `json:"planned_transfers"`
	Consequence      string               `json:"consequence"`
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
	document := inventoryDocumentFromReport(report)
	if err := validateInventoryDocumentV2(document); err != nil {
		fmt.Fprintf(stderr, "specaudit inventory: generated invalid report: %v\n", err)
		return 1
	}
	data, err := marshalReportWithLimit(document, maxArtifactOutputBytes)
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

//nolint:gocyclo // Ordered validation keeps every fail-closed ledger transition explicit.
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
	if err := requireAuthenticatedInputPlatform(); err != nil {
		fmt.Fprintf(stderr, "specaudit validate: %v\n", err)
		return 2
	}
	ledger, err := readDecisionLedgerV2(*input)
	if err != nil {
		fmt.Fprintf(stderr, "specaudit validate: %v\n", err)
		return 2
	}
	{
		inventoryDocument, inventoryErr := readInventoryDocumentV2(*inventoryPath)
		if inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit validate: read inventory: %v\n", inventoryErr)
			return 2
		}
		inventoryRef, inventoryErr := canonicalInventoryRefV2(inventoryDocument)
		if inventoryErr != nil || ledger.InventoryRef != inventoryRef {
			fmt.Fprintln(stderr, "specaudit validate: decision ledger inventory_ref does not match the supplied canonical inventory digest")
			return 1
		}
		inventoryReport := reportFromInventoryDocument(inventoryDocument)
		auditReport := reviewReportFromLedger(ledger, inventoryReport)
		auditReport.InventoryRef, inventoryErr = canonicalInventoryRef(inventoryReport)
		if inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit validate: canonicalize inventory: %v\n", inventoryErr)
			return 1
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
			if inventoryErr = validateSupportingEvidenceAgainstRepo(auditReport, inventoryReport, *repoPath); inventoryErr != nil {
				fmt.Fprintf(stderr, "specaudit validate: %v\n", inventoryErr)
				return 1
			}
		}
	}
	fmt.Fprintln(stdout, "specaudit: valid spec-audit/v2 decision ledger")
	return 0
}

//nolint:gocyclo // Ordered validation keeps every fail-closed render transition explicit.
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
	if err := requireAuthenticatedInputPlatform(); err != nil {
		fmt.Fprintf(stderr, "specaudit render: %v\n", err)
		return 2
	}
	ledger, err := readDecisionLedgerV2(*input)
	if err != nil {
		fmt.Fprintf(stderr, "specaudit render: %v\n", err)
		return 2
	}
	var inventoryReport *report
	var auditReport report
	{
		inventoryDocument, inventoryErr := readInventoryDocumentV2(*inventoryPath)
		if inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit render: read inventory: %v\n", inventoryErr)
			return 2
		}
		inventoryRef, inventoryErr := canonicalInventoryRefV2(inventoryDocument)
		if inventoryErr != nil || ledger.InventoryRef != inventoryRef {
			fmt.Fprintln(stderr, "specaudit render: decision ledger inventory_ref does not match the supplied canonical inventory digest")
			return 1
		}
		decodedInventory := reportFromInventoryDocument(inventoryDocument)
		auditReport = reviewReportFromLedger(ledger, decodedInventory)
		auditReport.InventoryRef, inventoryErr = canonicalInventoryRef(decodedInventory)
		if inventoryErr != nil {
			fmt.Fprintf(stderr, "specaudit render: canonicalize inventory: %v\n", inventoryErr)
			return 1
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
			if inventoryErr = validateSupportingEvidenceAgainstRepo(auditReport, decodedInventory, *repoPath); inventoryErr != nil {
				fmt.Fprintf(stderr, "specaudit render: %v\n", inventoryErr)
				return 1
			}
		}
		inventoryReport = &decodedInventory
	}
	htmlOutput, err := renderHTMLWithLimit(presentationReport(auditReport, *inventoryReport), inventoryReport, maxArtifactOutputBytes)
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
		DocumentKind:  inventoryDocumentKind,
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
		CollectorExecution:           new(currentCollectorExecution()),
		CollectorExecutionDisclosure: collectorExecutionDisclosure,
		Inventory:                    files,
		Features:                     features,
		Seeds:                        collectSeeds(files, active),
		Limitations:                  activeLimitations,
	}, nil
}

func currentCollectorExecution() collectorExecution {
	execution := collectorExecution{GoToolchain: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if info, ok := debug.ReadBuildInfo(); ok {
		execution.BuildInfoAvailable = true
		execution.ModulePath = info.Main.Path
		if info.GoVersion != "" {
			execution.GoToolchain = info.GoVersion
		}
		var revision string
		var modified *bool
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				parsed, err := strconv.ParseBool(setting.Value)
				if err == nil {
					modified = &parsed
				}
			}
		}
		if shaPattern.MatchString(revision) && modified != nil {
			execution.VCSMetadataAvailable = true
			execution.VCSRevision = revision
			execution.VCSModified = modified
		}
	}
	return execution
}

func requireAuthenticatedInputPlatform() error {
	return authenticatedInputPlatform(runtime.GOOS)
}

func authenticatedInputPlatform(goos string) error {
	if goos != "darwin" && goos != "linux" {
		return fmt.Errorf("validate and render are supported only on darwin or linux; %s is not authenticated by no-follow descriptor traversal", goos)
	}
	return nil
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
	if !regularGitBlobMode(fields[0]) || fields[1] != "blob" || !shaPattern.MatchString(fields[2]) {
		return 0, fmt.Errorf("pinned inventory object %q is not a regular Git blob with a 40-hex object ID", path)
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

func regularGitBlobMode(mode string) bool { return mode == "100644" || mode == "100755" }

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
		size, sizeErr := strconv.Atoi(fields[2])
		if sizeErr != nil || int64(size) != blob.size || size < 0 {
			return nil, fmt.Errorf("pinned object %s returned an unexpected size", blob.oid)
		}
		body := make([]byte, size)
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
		active, limitations := activeMembersFromBody(body)
		return active, append(limitations, "Active members were extracted by the dear-agent registry adapter from "+canonicalActiveHarnessRegistryPath+"; this is not a portable registry contract.")
	}
	if body, ok := pinnedBodyForPath(blobs, bodies, legacyActiveHarnessRegistryPath); ok {
		active, limitations := activeMembersFromBody(body)
		return active, append(limitations, "Active members were extracted by the legacy dear-agent registry adapter from "+legacyActiveHarnessRegistryPath+"; this is not a portable registry contract.")
	}
	return []string{}, []string{"Active harness inventory was unavailable at the pinned revision; no active-member parity finding is supported.", "The collector currently has only dear-agent-specific registry adapters; a generic pinned registry seam remains future work."}
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
		identical[file.SHA256] = append(identical[file.SHA256], evidence{Kind: "supporting", Path: file.Path, Line: 1, Excerpt: "identical full SPEC body"})
		for _, req := range file.Requirements {
			e := evidence{Kind: "normative-contract", Path: file.Path, Line: req.Line, RequirementID: req.ID, Excerpt: req.Excerpt}
			bodies[req.Body] = append(bodies[req.Body], bodyEntry{key: req.Body, evidence: e})
			ids[req.ID] = append(ids[req.ID], e)
		}
		for _, ref := range file.BDDFeatures {
			features[ref.Path] = append(features[ref.Path], evidence{Kind: "supporting", Path: file.Path, Line: ref.Line, Excerpt: ref.Path})
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
	return evidence{Kind: "normative-contract", Path: path, Line: requirement.Line, RequirementID: requirement.ID, Excerpt: requirement.Excerpt}
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

//nolint:gocyclo // Explicit AST admission rejects every ambiguous registry shape fail closed.
func activeMembersFromBody(body string) ([]string, []string) {
	file, err := parser.ParseFile(token.NewFileSet(), "registry.go", body, 0)
	if err != nil {
		return []string{}, []string{"Active harness inventory could not be parsed at the pinned revision."}
	}
	var values []ast.Expr
	found := false
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.VAR {
			continue
		}
		for _, declarationSpec := range general.Specs {
			value, ok := declarationSpec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			isActiveDeclaration := false
			for _, name := range value.Names {
				isActiveDeclaration = isActiveDeclaration || name.Name == "activeHarnesses"
			}
			if !isActiveDeclaration {
				continue
			}
			if found || len(value.Names) != 1 || len(value.Values) != 1 {
				return []string{}, []string{"Active harness inventory could not be parsed at the pinned revision."}
			}
			literal, ok := value.Values[0].(*ast.CompositeLit)
			if !ok || !isStringSliceType(literal.Type) {
				return []string{}, []string{"Active harness inventory could not be parsed at the pinned revision."}
			}
			values, found = literal.Elts, true
		}
	}
	if !found {
		return []string{}, []string{"Active harness inventory could not be parsed at the pinned revision."}
	}
	active := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		literal, ok := value.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return []string{}, []string{"Active harness inventory could not be parsed at the pinned revision."}
		}
		member, err := strconv.Unquote(literal.Value)
		if err != nil || member == "" || seen[member] {
			return []string{}, []string{"Active harness inventory could not be parsed at the pinned revision."}
		}
		seen[member] = true
		active = append(active, member)
	}
	sort.Strings(active)
	return active, nil
}

func isStringSliceType(expression ast.Expr) bool {
	array, ok := expression.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	name, ok := array.Elt.(*ast.Ident)
	return ok && name.Name == "string"
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

func readInventoryDocumentV2(path string) (inventoryDocument, error) {
	return readTypedDocument(path, validateInventoryDocumentV2)
}

func readDecisionLedgerV2(path string) (decisionLedger, error) {
	return readTypedDocument(path, validateDecisionLedgerV2)
}

func readTypedDocument[T any](path string, validate func(T) error) (T, error) {
	var document T
	data, err := readStableBoundedFile(path, maxReportInputBytes)
	if err != nil {
		return document, err
	}
	if err := validateUniqueJSONDocument(data, maxJSONDepth); err != nil {
		return document, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := validateExactJSONFieldNames(data, reflect.TypeFor[T]()); err != nil {
		return document, fmt.Errorf("decode %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return document, fmt.Errorf("decode %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return document, fmt.Errorf("decode %s: contains multiple JSON values", path)
	}
	if err := validate(document); err != nil {
		return document, err
	}
	return document, nil
}

func inventoryDocumentFromReport(source report) inventoryDocument {
	return inventoryDocument{SchemaVersion: schemaVersion, DocumentKind: inventoryDocumentKind, Snapshot: source.Snapshot,
		Scope:       collectionScope{Roots: source.Scope.Roots, ActiveMembers: source.Scope.ActiveMembers},
		Summary:     inventorySummary{SpecFiles: source.Summary.SpecFiles, Requirements: source.Summary.Requirements, Diagnostics: source.Summary.Diagnostics},
		Methodology: source.Methodology, CollectorExecution: *source.CollectorExecution, CollectorExecutionDisclosure: source.CollectorExecutionDisclosure, Inventory: source.Inventory, Features: source.Features, Seeds: source.Seeds, Limitations: source.Limitations}
}

func reportFromInventoryDocument(source inventoryDocument) report {
	collector := source.CollectorExecution
	return report{SchemaVersion: source.SchemaVersion, DocumentKind: source.DocumentKind, Snapshot: source.Snapshot,
		Scope:       scope{Roots: source.Scope.Roots, Excluded: []exclusion{}, ActiveMembers: source.Scope.ActiveMembers},
		Summary:     summary{SpecFiles: source.Summary.SpecFiles, Requirements: source.Summary.Requirements, Diagnostics: source.Summary.Diagnostics, ByVerdict: map[string]int{}},
		Methodology: source.Methodology, CollectorExecution: &collector, CollectorExecutionDisclosure: source.CollectorExecutionDisclosure, Inventory: source.Inventory, Features: source.Features, Seeds: source.Seeds, Limitations: source.Limitations}
}

func canonicalInventoryRefV2(document inventoryDocument) (string, error) {
	if err := validateInventoryDocumentV2(document); err != nil {
		return "", err
	}
	data, err := marshalReportWithLimit(document, maxArtifactOutputBytes)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + fmt.Sprintf("%x", digest), nil
}

func reviewReportFromLedger(ledger decisionLedger, inventory report) report {
	candidates := make([]finding, len(ledger.Candidates))
	for index, finding := range ledger.Candidates {
		candidates[index] = findingFromPersistedEvidence(finding)
	}
	nonCandidates := make([]finding, len(ledger.NonCandidates))
	for index, finding := range ledger.NonCandidates {
		nonCandidates[index] = findingFromPersistedEvidence(finding)
	}
	return report{SchemaVersion: schemaVersion, DocumentKind: ledgerDocumentKind, InventoryRef: ledger.InventoryRef,
		Snapshot: inventory.Snapshot, Scope: scope{Roots: inventory.Scope.Roots, ActiveMembers: inventory.Scope.ActiveMembers},
		Summary:     summary{SpecFiles: inventory.Summary.SpecFiles, Requirements: inventory.Summary.Requirements, Diagnostics: inventory.Summary.Diagnostics, CandidateCount: ledger.Summary.CandidateCount, ByVerdict: ledger.Summary.ByVerdict},
		Methodology: methodology{Collector: inventory.Methodology.Collector, SeedKinds: inventory.Methodology.SeedKinds, SemanticReview: ledger.Methodology.SemanticReview, GitEvidenceTrust: inventory.Methodology.GitEvidenceTrust, GitTrustInputs: inventory.Methodology.GitTrustInputs, Reproduce: ledger.Methodology.Reproduce},
		Candidates:  candidates, NonCandidates: nonCandidates, Exclusions: ledger.ReviewScope.Exclusions, Limitations: ledger.Limitations}
}

func decisionLedgerFromReport(source, inventory report) (decisionLedger, error) {
	ref, err := canonicalInventoryRefV2(inventoryDocumentFromReport(inventory))
	if err != nil {
		return decisionLedger{}, err
	}
	candidates := make([]finding, len(source.Candidates))
	for index, finding := range source.Candidates {
		candidates[index] = findingForPersistence(finding)
	}
	nonCandidates := make([]finding, len(source.NonCandidates))
	for index, finding := range source.NonCandidates {
		nonCandidates[index] = findingForPersistence(finding)
	}
	return decisionLedger{SchemaVersion: schemaVersion, DocumentKind: ledgerDocumentKind, InventoryRef: ref,
		ReviewScope: reviewScope{Exclusions: source.Exclusions},
		Summary:     decisionSummary{CandidateCount: source.Summary.CandidateCount, ByVerdict: source.Summary.ByVerdict},
		Methodology: reviewMethodology{SemanticReview: source.Methodology.SemanticReview, Reproduce: source.Methodology.Reproduce},
		Candidates:  candidates, NonCandidates: nonCandidates, Limitations: source.Limitations}, nil
}

func findingForPersistence(source finding) finding {
	result := source
	result.Applicability = append([]applicability(nil), source.Applicability...)
	result.ContractEvidence, result.SupportingEvidence = splitPersistedEvidence(source.Evidence)
	result.Evidence = nil
	for index := range result.Applicability {
		result.Applicability[index].ContractEvidence, result.Applicability[index].SupportingEvidence = splitPersistedEvidence(source.Applicability[index].Evidence)
		result.Applicability[index].Evidence = nil
	}
	if result.OwnershipPlan != nil {
		plan := *result.OwnershipPlan
		plan.Applicability = append([]applicability(nil), source.OwnershipPlan.Applicability...)
		for index := range plan.Applicability {
			plan.Applicability[index].ContractEvidence, plan.Applicability[index].SupportingEvidence = splitPersistedEvidence(source.OwnershipPlan.Applicability[index].Evidence)
			plan.Applicability[index].Evidence = nil
		}
		result.OwnershipPlan = &plan
	}
	return result
}

func findingFromPersistedEvidence(source finding) finding {
	result := source
	result.Applicability = append([]applicability(nil), source.Applicability...)
	result.Evidence = joinPersistedEvidence(source.ContractEvidence, source.SupportingEvidence)
	for index := range result.Applicability {
		result.Applicability[index].Evidence = joinPersistedEvidence(source.Applicability[index].ContractEvidence, source.Applicability[index].SupportingEvidence)
	}
	if result.OwnershipPlan != nil {
		plan := *result.OwnershipPlan
		plan.Applicability = append([]applicability(nil), source.OwnershipPlan.Applicability...)
		for index := range plan.Applicability {
			plan.Applicability[index].Evidence = joinPersistedEvidence(plan.Applicability[index].ContractEvidence, plan.Applicability[index].SupportingEvidence)
		}
		result.OwnershipPlan = &plan
	}
	return result
}

func splitPersistedEvidence(source []evidence) ([]contractEvidence, []supportingEvidence) {
	contracts := []contractEvidence{}
	supporting := []supportingEvidence{}
	for _, item := range source {
		switch item.Kind {
		case "normative-contract":
			contracts = append(contracts, contractEvidence{Path: item.Path, Line: item.Line, RequirementID: item.RequirementID, Excerpt: item.Excerpt})
		case "supporting":
			supporting = append(supporting, supportingEvidence{Path: item.Path, Line: item.Line, Excerpt: item.Excerpt})
		}
	}
	return contracts, supporting
}

func joinPersistedEvidence(contracts []contractEvidence, supporting []supportingEvidence) []evidence {
	result := make([]evidence, 0, len(contracts)+len(supporting))
	for _, item := range contracts {
		result = append(result, evidence{Kind: "normative-contract", Path: item.Path, Line: item.Line, RequirementID: item.RequirementID, Excerpt: item.Excerpt})
	}
	for _, item := range supporting {
		result = append(result, evidence{Kind: "supporting", Path: item.Path, Line: item.Line, Excerpt: item.Excerpt})
	}
	return result
}

func validateInventoryDocumentV2(document inventoryDocument) error {
	if document.SchemaVersion != schemaVersion || document.DocumentKind != inventoryDocumentKind {
		return errors.New("inventory must be a spec-audit/v2 inventory document")
	}
	if document.CollectorExecutionDisclosure != collectorExecutionDisclosure || !validCollectorExecution(document.CollectorExecution) {
		return errors.New("inventory collector_execution is incomplete")
	}
	view := report{SchemaVersion: document.SchemaVersion, DocumentKind: document.DocumentKind, Snapshot: document.Snapshot,
		Scope:       scope{Roots: document.Scope.Roots, Excluded: []exclusion{}, ActiveMembers: document.Scope.ActiveMembers},
		Summary:     summary{SpecFiles: document.Summary.SpecFiles, Requirements: document.Summary.Requirements, Diagnostics: document.Summary.Diagnostics, ByVerdict: map[string]int{}},
		Methodology: document.Methodology, CollectorExecution: &document.CollectorExecution, CollectorExecutionDisclosure: document.CollectorExecutionDisclosure, Inventory: document.Inventory, Features: document.Features, Seeds: document.Seeds, Limitations: document.Limitations}
	return validateInventoryDocument(view)
}

//nolint:gocyclo // Sequential schema checks keep each persisted trust boundary explicit.
func validateDecisionLedgerV2(document decisionLedger) error {
	if document.SchemaVersion != schemaVersion || document.DocumentKind != ledgerDocumentKind {
		return errors.New("decision ledger must be a spec-audit/v2 decision-ledger document")
	}
	if !strings.HasPrefix(document.InventoryRef, "sha256:") || !identityPattern.MatchString(document.InventoryRef) {
		return errors.New("decision ledger inventory_ref must be a canonical sha256 inventory digest")
	}
	if !validReviewExclusions(document.ReviewScope.Exclusions) || strings.TrimSpace(document.Methodology.SemanticReview) == "" || len(document.Methodology.Reproduce) == 0 {
		return errors.New("decision ledger review scope or methodology is incomplete")
	}
	for _, finding := range append(append([]finding{}, document.Candidates...), document.NonCandidates...) {
		if finding.DecisionStatus != pendingMaintainerApproval {
			return fmt.Errorf("finding %q must have decision_status %q", finding.ID, pendingMaintainerApproval)
		}
		positive := finding.Verdict == "merge-now" || finding.Verdict == "extract-neutral-contract"
		if positive {
			if err := validateNewProposedOwnerDirectory(finding); err != nil {
				return fmt.Errorf("finding %q proposed_owner: %w", finding.ID, err)
			}
		}
		if err := validatePersistedEvidence(finding.ContractEvidence, finding.SupportingEvidence); err != nil {
			return fmt.Errorf("finding %q: %w", finding.ID, err)
		}
		for _, entry := range finding.Applicability {
			if len(entry.ContractEvidence) == 0 {
				return fmt.Errorf("finding %q applicability for %q requires contract_evidence", finding.ID, entry.Member)
			}
			if err := validatePersistedEvidence(entry.ContractEvidence, entry.SupportingEvidence); err != nil {
				return fmt.Errorf("finding %q applicability for %q: %w", finding.ID, entry.Member, err)
			}
		}
		if err := validatePersistedPlannedBDDTransfers(finding.BDD.PlannedTransfers); err != nil {
			return fmt.Errorf("finding %q planned BDD transfers: %w", finding.ID, err)
		}
		if err := validatePersistedOwnershipPlan(finding.OwnershipPlan); err != nil {
			return fmt.Errorf("finding %q ownership_plan: %w", finding.ID, err)
		}
		if positive && finding.OwnershipPlan == nil {
			return fmt.Errorf("finding %q requires ownership_plan", finding.ID)
		}
		if !positive && finding.OwnershipPlan != nil {
			return fmt.Errorf("non-positive finding %q cannot include ownership_plan", finding.ID)
		}
	}
	return nil
}

func validatePersistedOwnershipPlan(plan *ownershipPlan) error {
	if plan == nil {
		return nil
	}
	if plan.Status != pendingMaintainerApproval || plan.DeletionAuthority {
		return errors.New("must be pending maintainer approval and cannot authorize deletion")
	}
	if !applicabilityBases[plan.ApplicabilityBasis] || strings.TrimSpace(plan.ApplicabilityRationale) == "" {
		return errors.New("must copy a valid applicability basis and rationale")
	}
	if err := validatePersistedPlannedBDDTransfers(plan.BDDPlannedTransfers); err != nil {
		return fmt.Errorf("planned BDD transfers: %w", err)
	}
	for _, entry := range plan.Applicability {
		if len(entry.ContractEvidence) == 0 {
			return errors.New("applicability entries require contract_evidence")
		}
		if err := validatePersistedEvidence(entry.ContractEvidence, entry.SupportingEvidence); err != nil {
			return err
		}
	}
	return nil
}

func validatePersistedPlannedBDDTransfers(transfers []plannedBDDTransfer) error {
	seenSources := map[string]bool{}
	for _, transfer := range transfers {
		if !isSpecPath(transfer.SourceOwner) || !isSpecPath(transfer.TargetOwner) || !validFeaturePath(transfer.TargetFeature) || strings.TrimSpace(transfer.Rationale) == "" {
			return errors.New("entries require SPEC source and target owners, a feature target, and rationale")
		}
		if seenSources[transfer.SourceOwner] {
			return fmt.Errorf("source owner %q is duplicated", transfer.SourceOwner)
		}
		seenSources[transfer.SourceOwner] = true
		if len(transfer.BehaviorEvidence) == 0 {
			return fmt.Errorf("source owner %q requires behavior_evidence", transfer.SourceOwner)
		}
		if err := validatePersistedEvidence(nil, transfer.BehaviorEvidence); err != nil {
			return fmt.Errorf("source owner %q behavior_evidence: %w", transfer.SourceOwner, err)
		}
		seenEvidence := map[string]bool{}
		for _, item := range transfer.BehaviorEvidence {
			if item.Path != transfer.TargetFeature {
				return fmt.Errorf("source owner %q behavior_evidence must cite target_feature", transfer.SourceOwner)
			}
			key := item.Path + "\x00" + strconv.Itoa(item.Line) + "\x00" + item.Excerpt
			if seenEvidence[key] {
				return fmt.Errorf("source owner %q repeats behavior_evidence", transfer.SourceOwner)
			}
			seenEvidence[key] = true
		}
	}
	return nil
}

func validatePersistedEvidence(contracts []contractEvidence, supporting []supportingEvidence) error {
	if len(contracts)+len(supporting) == 0 {
		return errors.New("evidence is required")
	}
	for _, item := range contracts {
		if !validPath(item.Path) || item.Line < 1 || strings.TrimSpace(item.RequirementID) == "" || strings.TrimSpace(item.Excerpt) == "" {
			return errors.New("contract_evidence is incomplete")
		}
	}
	for _, item := range supporting {
		if !validPath(item.Path) || item.Line < 1 || strings.TrimSpace(item.Excerpt) == "" {
			return errors.New("supporting_evidence is incomplete")
		}
	}
	return nil
}

func readReportWithLimit(path string, limit int64) (report, error) {
	data, err := readStableBoundedFile(path, limit)
	if err != nil {
		return report{}, err
	}
	if err := validateUniqueJSONDocument(data, maxJSONDepth); err != nil {
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
	for _, key := range []string{"inventory", "features", "seeds", "collector_execution"} {
		if _, ok := topLevel[key]; ok {
			decoded.inventoryPayloadPresent = true
			break
		}
	}
	for _, key := range []string{"candidates", "non_candidates", "reviewer_exclusions", "inventory_ref"} {
		if _, ok := topLevel[key]; ok {
			decoded.decisionPayloadPresent = true
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

//nolint:gocyclo // Recursive JSON shape validation is clearer as an explicit kind dispatch.
func validateExactJSONValue(raw json.RawMessage, valueType reflect.Type) error {
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	switch valueType.Kind() { //nolint:exhaustive // Only recursive container kinds require child-field validation.
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
		"GIT_LITERAL_PATHSPECS=1",
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

func marshalReportWithLimit(value any, limit int64) ([]byte, error) {
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
	if err := validateCrossFindingRequirementMappings(report); err != nil {
		return err
	}
	if report.Summary.CandidateCount != len(report.Candidates) {
		return fmt.Errorf("summary candidate_count=%d, want %d", report.Summary.CandidateCount, len(report.Candidates))
	}
	if report.Summary.SpecFiles < 0 || report.Summary.Requirements < 0 || report.Summary.Diagnostics < 0 || !equalHistogram(report.Summary.ByVerdict, verdictHistogram(report)) {
		return errors.New("summary counts or by_verdict do not match report findings")
	}
	return nil
}

type requirementMappingTarget struct {
	findingID           string
	disposition         string
	targetPath          string
	targetRequirementID string
	targetState         string
}

// validateCrossFindingRequirementMappings prevents one pinned normative record
// from acquiring mutually incompatible migration instructions in otherwise
// individually valid findings. Identical mappings may appear in overlapping
// analytical views, but disposition or target drift must be resolved before the
// ledger can be treated as a coherent maintainer-pending plan.
func validateCrossFindingRequirementMappings(document report) error {
	seen := map[contractEvidenceKey]requirementMappingTarget{}
	for _, candidate := range document.Candidates {
		if candidate.OwnershipPlan == nil {
			continue
		}
		selected := map[contractEvidenceKey]bool{}
		for _, item := range candidate.ContractEvidence {
			selected[contractEvidenceKey{path: item.Path, line: item.Line, requirementID: item.RequirementID, excerpt: item.Excerpt}] = true
		}
		for _, item := range candidate.Evidence {
			if item.Kind == "normative-contract" {
				selected[contractEvidenceKey{path: item.Path, line: item.Line, requirementID: item.RequirementID, excerpt: item.Excerpt}] = true
			}
		}
		for _, entry := range candidate.OwnershipPlan.Requirements {
			key := contractEvidenceKey{
				path:          entry.ContractEvidence.Path,
				line:          entry.ContractEvidence.Line,
				requirementID: entry.ContractEvidence.RequirementID,
				excerpt:       entry.ContractEvidence.Excerpt,
			}
			if !selected[key] {
				continue
			}
			target := requirementMappingTarget{
				findingID:           candidate.ID,
				disposition:         entry.Disposition,
				targetPath:          entry.TargetPath,
				targetRequirementID: entry.TargetRequirementID,
				targetState:         entry.TargetState,
			}
			prior, exists := seen[key]
			if !exists {
				seen[key] = target
				continue
			}
			if prior.disposition != target.disposition || prior.targetPath != target.targetPath || prior.targetRequirementID != target.targetRequirementID || prior.targetState != target.targetState {
				return fmt.Errorf("findings %q and %q assign conflicting ownership mappings to %s:%d requirement %q", prior.findingID, candidate.ID, key.path, key.line, key.requirementID)
			}
		}
	}
	return nil
}

// validateInventoryDocument accepts only immutable, collector-produced facts.
// Reviewer classifications and exclusions belong exclusively to a separate
// decision ledger and therefore cannot affect collection.
func validateInventoryDocument(document report) error {
	if document.DocumentKind != inventoryDocumentKind {
		return fmt.Errorf("document_kind must be %q", inventoryDocumentKind)
	}
	if len(document.Scope.Excluded) != 0 {
		return errors.New("inventory scope exclusions must be empty; collection is complete")
	}
	if document.InventoryRef != "" || document.decisionPayloadPresent || len(document.Candidates) != 0 || len(document.NonCandidates) != 0 || len(document.Exclusions) != 0 {
		return errors.New("inventory must not contain reviewer decisions, exclusions, or inventory_ref")
	}
	if document.Inventory == nil || document.Features == nil || document.Seeds == nil {
		return errors.New("inventory must contain complete immutable inventory, features, seeds, and collector_execution")
	}
	if document.CollectorExecution == nil || document.CollectorExecutionDisclosure != collectorExecutionDisclosure || !validCollectorExecution(*document.CollectorExecution) {
		return errors.New("inventory collector_execution disclosure is incomplete or invalid")
	}
	return validateReport(document)
}

// validateDecisionLedger accepts reviewer judgments which refer to, but do
// not copy or filter, the independently collected pinned inventory.
func validateDecisionLedger(document report) error {
	if document.DocumentKind != ledgerDocumentKind {
		return fmt.Errorf("document_kind must be %q", ledgerDocumentKind)
	}
	if !strings.HasPrefix(document.InventoryRef, "sha256:") || !identityPattern.MatchString(document.InventoryRef) {
		return errors.New("decision ledger inventory_ref must be a canonical sha256 inventory digest")
	}
	if document.hasInventoryPayload() {
		return errors.New("semantic report must omit inventory, features, and seeds (including collector_execution); decision ledgers use the separately supplied pinned inventory")
	}
	if len(document.Scope.Excluded) != 0 {
		return errors.New("decision ledger scope exclusions must be empty; use reviewer_exclusions")
	}
	if !validReviewExclusions(document.Exclusions) {
		return errors.New("decision ledger reviewer_exclusions must be unique repository-relative classifications with rationale and supporting evidence")
	}
	return validateReport(document)
}

func validCollectorExecution(execution collectorExecution) bool {
	if strings.TrimSpace(execution.GoToolchain) == "" || strings.TrimSpace(execution.GOOS) == "" || strings.TrimSpace(execution.GOARCH) == "" {
		return false
	}
	if execution.BuildInfoAvailable != (strings.TrimSpace(execution.ModulePath) != "") {
		return false
	}
	if execution.VCSMetadataAvailable {
		return shaPattern.MatchString(execution.VCSRevision) && execution.VCSModified != nil
	}
	return execution.VCSRevision == "" && execution.VCSModified == nil
}

//nolint:gocyclo // Sequential pinned-evidence cross-checks keep every trust transition visible and fail closed.
func validateAgainstInventory(semantic, inventory report) error {
	if err := validateDecisionLedger(semantic); err != nil {
		return err
	}
	if err := validateInventoryDocument(inventory); err != nil {
		return fmt.Errorf("inventory report is invalid: %w", err)
	}
	canonicalRef, err := canonicalInventoryRef(inventory)
	if err != nil {
		return fmt.Errorf("canonicalize inventory: %w", err)
	}
	if semantic.InventoryRef != canonicalRef {
		return errors.New("decision ledger inventory_ref does not match the supplied canonical inventory digest")
	}
	if semantic.Snapshot.Repository != inventory.Snapshot.Repository || semantic.Snapshot.Revision != inventory.Snapshot.Revision || semantic.Snapshot.RevisionCommittedAt != inventory.Snapshot.RevisionCommittedAt {
		return errors.New("semantic report and inventory must name the same repository, pinned revision, and revision timestamp")
	}
	if semantic.Summary.SpecFiles != inventory.Summary.SpecFiles || semantic.Summary.Requirements != inventory.Summary.Requirements || semantic.Summary.Diagnostics != inventory.Summary.Diagnostics {
		return errors.New("semantic report corpus counts do not match the pinned inventory")
	}
	if !reflect.DeepEqual(semantic.Scope.Roots, inventory.Scope.Roots) {
		return errors.New("decision ledger scope roots do not match the pinned inventory")
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
	excludedFromReview := map[string]bool{}
	for _, exclusion := range semantic.Exclusions {
		if !files[exclusion.Path] && features[exclusion.Path].Path == "" {
			return fmt.Errorf("reviewer exclusion path %q does not resolve in the pinned inventory", exclusion.Path)
		}
		excludedFromReview[exclusion.Path] = true
	}

	for _, finding := range allFindings(semantic) {
		positive := finding.Verdict == "merge-now" || finding.Verdict == "extract-neutral-contract"
		for _, owner := range finding.CurrentOwners {
			if !files[owner.Path] {
				return fmt.Errorf("finding %q names current owner %q outside the pinned inventory", finding.ID, owner.Path)
			}
			if positive && excludedFromReview[owner.Path] {
				return fmt.Errorf("positive finding %q cannot select reviewer-excluded current owner %q", finding.ID, owner.Path)
			}
		}
		evidenceOwners := map[string]bool{}
		for _, item := range finding.Evidence {
			if item.Kind == "supporting" {
				continue
			}
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
			normativeCount := 0
			for _, item := range applicabilityEntry.Evidence {
				if item.Kind == "supporting" {
					continue
				}
				normativeCount++
				if !files[item.Path] || item.RequirementID == "" {
					return fmt.Errorf("finding %q applicability for %q has evidence outside the pinned requirement inventory", finding.ID, applicabilityEntry.Member)
				}
				excerpt, ok := requirements[item.Path][requirementKey{line: item.Line, id: item.RequirementID}]
				if !ok || strings.TrimSpace(item.Excerpt) != strings.TrimSpace(excerpt) {
					return fmt.Errorf("finding %q applicability for %q does not exactly match pinned evidence", finding.ID, applicabilityEntry.Member)
				}
			}
			if normativeCount == 0 {
				return fmt.Errorf("finding %q applicability for %q has no normative-contract evidence", finding.ID, applicabilityEntry.Member)
			}
		}
		if finding.ProposedOwner != nil && finding.ProposedOwner.State == "existing" && !files[finding.ProposedOwner.Path] {
			return fmt.Errorf("finding %q marks proposed owner %q existing, but it is absent from the pinned inventory", finding.ID, finding.ProposedOwner.Path)
		}
		if finding.ProposedOwner != nil && finding.ProposedOwner.State == "new" && files[finding.ProposedOwner.Path] {
			return fmt.Errorf("finding %q marks proposed owner %q new, but it already exists in the pinned inventory", finding.ID, finding.ProposedOwner.Path)
		}
		if positive && finding.ProposedOwner != nil && excludedFromReview[finding.ProposedOwner.Path] {
			return fmt.Errorf("positive finding %q cannot select reviewer-excluded proposed owner %q", finding.ID, finding.ProposedOwner.Path)
		}
		coveredOwners := map[string]bool{}
		for _, featurePath := range finding.BDD.Features {
			feature, ok := features[featurePath]
			if !ok {
				return fmt.Errorf("finding %q BDD feature %q is absent from the pinned feature inventory", finding.ID, featurePath)
			}
			if positive && excludedFromReview[featurePath] {
				return fmt.Errorf("positive finding %q cannot select reviewer-excluded BDD feature %q", finding.ID, featurePath)
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
		plannedOwners := map[string]bool{}
		if positive {
			currentOwners := stringSet(ownerPaths(finding.CurrentOwners))
			selectedFeatures := stringSet(finding.BDD.Features)
			for _, transfer := range finding.BDD.PlannedTransfers {
				if !currentOwners[transfer.SourceOwner] || plannedOwners[transfer.SourceOwner] {
					return fmt.Errorf("finding %q planned BDD transfer source %q must name one uncovered current owner exactly once", finding.ID, transfer.SourceOwner)
				}
				if coveredOwners[transfer.SourceOwner] {
					return fmt.Errorf("finding %q current owner %q cannot have both reciprocal and planned BDD coverage", finding.ID, transfer.SourceOwner)
				}
				if finding.ProposedOwner == nil || finding.ProposedOwner.State != "existing" || transfer.TargetOwner != finding.ProposedOwner.Path {
					return fmt.Errorf("finding %q planned BDD transfer for %q must target the existing proposed owner", finding.ID, transfer.SourceOwner)
				}
				if !selectedFeatures[transfer.TargetFeature] || excludedFromReview[transfer.TargetFeature] {
					return fmt.Errorf("finding %q planned BDD transfer for %q must target a selected, non-excluded feature", finding.ID, transfer.SourceOwner)
				}
				targetFeature, ok := features[transfer.TargetFeature]
				if !ok || !containsString(targetFeature.RelatedSpecs, transfer.TargetOwner) || !specFeatures[transfer.TargetOwner][transfer.TargetFeature] {
					return fmt.Errorf("finding %q planned BDD transfer for %q must target an existing feature reciprocal with proposed owner %q", finding.ID, transfer.SourceOwner, transfer.TargetOwner)
				}
				plannedOwners[transfer.SourceOwner] = true
			}
		}
		for _, owner := range finding.CurrentOwners {
			if positive && coveredOwners[owner.Path] == plannedOwners[owner.Path] {
				return fmt.Errorf("positive finding %q current owner %q must have exactly one of reciprocal or planned BDD coverage", finding.ID, owner.Path)
			}
			if !positive && len(finding.BDD.Features) > 0 && !coveredOwners[owner.Path] {
				return fmt.Errorf("finding %q BDD features do not reciprocally name current owner %q", finding.ID, owner.Path)
			}
		}
		if positive {
			if err := validateOwnershipPlanAgainstInventory(finding, inventory, features, specFeatures); err != nil {
				return fmt.Errorf("finding %q ownership_plan: %w", finding.ID, err)
			}
		}
	}
	return nil
}

//nolint:gocyclo // Full preservation validation keeps requirement and feature accounting in one auditable pass.
func validateOwnershipPlanAgainstInventory(finding finding, inventory report, features map[string]featureFile, specFeatures map[string]map[string]bool) error {
	plan := finding.OwnershipPlan
	if plan == nil || plan.Status != pendingMaintainerApproval || plan.DeletionAuthority {
		return errors.New("must be pending maintainer approval and cannot authorize deletion")
	}
	if plan.ApplicabilityBasis != finding.ApplicabilityBasis || plan.ApplicabilityRationale != finding.ApplicabilityRationale || !reflect.DeepEqual(plan.Applicability, finding.Applicability) {
		return errors.New("applicability basis, rationale, and matrix must exactly match the finding")
	}
	if !reflect.DeepEqual(plan.BDDPlannedTransfers, finding.BDD.PlannedTransfers) {
		return errors.New("planned BDD transfers must exactly match the finding")
	}
	owners := map[string]bool{}
	expectedRequirements := map[contractEvidenceKey]bool{}
	expectedFeatures := map[featurePreservationKey]bool{}
	allRequirements := map[string]map[string]bool{}
	selectedRequirements := map[contractEvidenceKey]bool{}
	selectedFeatures := stringSet(finding.BDD.Features)
	for _, item := range finding.Evidence {
		if item.Kind == "normative-contract" {
			selectedRequirements[contractEvidenceKey{path: item.Path, line: item.Line, requirementID: item.RequirementID, excerpt: item.Excerpt}] = true
		}
	}
	for _, owner := range finding.CurrentOwners {
		owners[owner.Path] = true
		for _, file := range inventory.Inventory {
			if file.Path != owner.Path {
				continue
			}
			for _, requirement := range file.Requirements {
				expectedRequirements[contractEvidenceKey{path: file.Path, line: requirement.Line, requirementID: requirement.ID, excerpt: requirement.Excerpt}] = true
				if allRequirements[file.Path] == nil {
					allRequirements[file.Path] = map[string]bool{}
				}
				allRequirements[file.Path][requirement.ID] = true
			}
		}
		for featurePath := range specFeatures[owner.Path] {
			feature, ok := features[featurePath]
			if ok && containsString(feature.RelatedSpecs, owner.Path) {
				expectedFeatures[featurePreservationKey{owner: owner.Path, path: featurePath}] = true
			}
		}
	}
	ownerActions := map[string]string{}
	for _, action := range plan.OwnerActions {
		if !owners[action.OwnerPath] || ownerActions[action.OwnerPath] != "" || strings.TrimSpace(action.Rationale) == "" {
			return errors.New("owner_actions must cover each current owner exactly once with rationale")
		}
		switch action.Disposition {
		case retainDistinctContract, retireNormativeOwnership, retireSelectedNormativeOwnership:
		default:
			return errors.New("owner_actions contain an unsupported disposition")
		}
		ownerActions[action.OwnerPath] = action.Disposition
	}
	if !sameStringSet(mapKeys(owners), mapKeys(ownerActionPaths(ownerActions))) {
		return errors.New("owner_actions do not exactly cover current owners")
	}
	for ownerPath, action := range ownerActions {
		if action == retainDistinctContract && (finding.ProposedOwner == nil || ownerPath != finding.ProposedOwner.Path) {
			return errors.New("only the existing proposed owner may retain the selected normative contract")
		}
		if isHarnessRegistrationOwner(ownerPath) && action == retainDistinctContract {
			return errors.New("a harness registration current owner cannot retain normative ownership")
		}
	}
	if finding.ProposedOwner != nil && finding.ProposedOwner.State == "existing" && ownerActions[finding.ProposedOwner.Path] != retainDistinctContract {
		return errors.New("the existing proposed owner must retain the canonical contract")
	}
	requirements := map[contractEvidenceKey]bool{}
	for _, entry := range plan.Requirements {
		key := contractEvidenceKey{path: entry.ContractEvidence.Path, line: entry.ContractEvidence.Line, requirementID: entry.ContractEvidence.RequirementID, excerpt: entry.ContractEvidence.Excerpt}
		if !expectedRequirements[key] || requirements[key] || !validRequirementOwnershipMapping(entry, finding.ProposedOwner, ownerActions[key.path], allRequirements, selectedRequirements[key]) || strings.TrimSpace(entry.Rationale) == "" {
			return errors.New("requirements must map each full current-owner contract evidence record exactly once")
		}
		requirements[key] = true
	}
	if !sameContractEvidenceKeys(expectedRequirements, requirements) {
		return errors.New("requirements do not fully preserve every current-owner requirement")
	}
	featureMappings := map[featurePreservationKey]bool{}
	preservedSelectedFeature := false
	for _, entry := range plan.Features {
		feature, ok := features[entry.Path]
		key := featurePreservationKey{owner: entry.SourceOwner, path: entry.Path}
		if !expectedFeatures[key] || featureMappings[key] || !ok || strings.TrimSpace(entry.Rationale) == "" {
			return errors.New("features must map each reciprocal current-owner BDD feature exactly once")
		}
		if !validFeatureOwnershipMapping(entry, finding.ProposedOwner, ownerActions[entry.SourceOwner], feature, selectedFeatures[entry.Path]) {
			return errors.New("BDD feature mapping has invalid disposition or target")
		}
		if selectedFeatures[entry.Path] && entry.Disposition == preservePendingSeparateAudit {
			preservedSelectedFeature = true
		}
		featureMappings[key] = true
	}
	if !sameFeaturePreservationKeys(expectedFeatures, featureMappings) {
		return errors.New("features do not fully preserve every reciprocal current-owner BDD feature")
	}
	if preservedSelectedFeature && finding.BDD.Consequence != "add-matrix" && finding.BDD.Consequence != "preserve-residual" {
		return errors.New("preserved selected BDD feature requires add-matrix or preserve-residual consequence")
	}
	if finding.BDD.Consequence == "preserve-residual" && !preservedSelectedFeature {
		return errors.New("preserve-residual consequence requires a selected BDD feature preserved at its source")
	}
	return nil
}

type featurePreservationKey struct{ owner, path string }

//nolint:gocyclo // Closed disposition combinations are intentionally enumerated in one predicate.
func validRequirementOwnershipMapping(entry requirementPreservation, proposed *proposedOwnerClaim, action string, requirements map[string]map[string]bool, selected bool) bool {
	if !validPath(entry.TargetPath) || entry.TargetRequirementID == "" || (entry.TargetState != "existing" && entry.TargetState != "planned") {
		return false
	}
	if action == retainDistinctContract {
		return entry.Disposition == retainDistinct && entry.TargetPath == entry.ContractEvidence.Path && entry.TargetRequirementID == entry.ContractEvidence.RequirementID && entry.TargetState == "existing"
	}
	if action == retireSelectedNormativeOwnership && !selected {
		return entry.Disposition == preservePendingSeparateAudit && entry.TargetPath == entry.ContractEvidence.Path && entry.TargetRequirementID == entry.ContractEvidence.RequirementID && entry.TargetState == "existing"
	}
	if !selected || (action != retireNormativeOwnership && action != retireSelectedNormativeOwnership) || proposed == nil || entry.TargetPath != proposed.Path || !validProposedTargetState(proposed, entry.TargetState) {
		return false
	}
	if entry.Disposition != transferToProposedOwner && entry.Disposition != representAsApplicability {
		return false
	}
	return entry.TargetState != "existing" || requirements[entry.TargetPath][entry.TargetRequirementID]
}

//nolint:gocyclo // Closed feature-preservation combinations are intentionally enumerated in one predicate.
func validFeatureOwnershipMapping(entry featurePreservation, proposed *proposedOwnerClaim, action string, feature featureFile, selected bool) bool {
	if !validPath(entry.TargetPath) || (entry.TargetState != "existing" && entry.TargetState != "planned") {
		return false
	}
	if action == retainDistinctContract {
		return entry.Disposition == retainDistinct && entry.TargetPath == entry.SourceOwner && entry.TargetState == "existing" && containsString(feature.RelatedSpecs, entry.SourceOwner)
	}
	if action == retireSelectedNormativeOwnership && entry.Disposition == preservePendingSeparateAudit {
		return entry.TargetPath == entry.SourceOwner && entry.TargetState == "existing" && containsString(feature.RelatedSpecs, entry.SourceOwner)
	}
	if !selected || (action != retireNormativeOwnership && action != retireSelectedNormativeOwnership) || proposed == nil || entry.TargetPath != proposed.Path || !validProposedTargetState(proposed, entry.TargetState) || (entry.Disposition != transferToProposedOwner && entry.Disposition != representAsApplicability) {
		return false
	}
	return entry.TargetState != "existing" || containsString(feature.RelatedSpecs, proposed.Path)
}

func validProposedTargetState(proposed *proposedOwnerClaim, state string) bool {
	if proposed == nil {
		return false
	}
	if proposed.State == "new" {
		return state == "planned"
	}
	return state == "existing" || state == "planned"
}

func ownerActionPaths(values map[string]string) map[string]bool {
	result := map[string]bool{}
	for path := range values {
		result[path] = true
	}
	return result
}

func sameFeaturePreservationKeys(left, right map[featurePreservationKey]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if !right[key] {
			return false
		}
	}
	return true
}

func sameContractEvidenceKeys(left, right map[contractEvidenceKey]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if !right[key] {
			return false
		}
	}
	return true
}

func (report report) hasInventoryPayload() bool {
	return report.inventoryPayloadPresent || report.Inventory != nil || report.Features != nil || report.Seeds != nil
}

func canonicalInventoryRef(document report) (string, error) {
	if err := validateInventoryDocument(document); err != nil {
		return "", err
	}
	data, err := marshalReportWithLimit(document, maxArtifactOutputBytes)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + fmt.Sprintf("%x", digest), nil
}

func presentationReport(ledger, inventory report) report {
	view := ledger
	view.Snapshot = inventory.Snapshot
	view.Scope = inventory.Scope
	view.Scope.Excluded = reviewExclusionsForPresentation(ledger.Exclusions)
	view.Summary.SpecFiles = inventory.Summary.SpecFiles
	view.Summary.Requirements = inventory.Summary.Requirements
	view.Summary.Diagnostics = inventory.Summary.Diagnostics
	view.Methodology = inventory.Methodology
	view.Limitations = append([]string{}, inventory.Limitations...)
	view.CollectorExecution = inventory.CollectorExecution
	view.CollectorExecutionDisclosure = inventory.CollectorExecutionDisclosure
	return view
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
		supplied.CollectorExecutionDisclosure != recomputed.CollectorExecutionDisclosure ||
		!reflect.DeepEqual(supplied.Inventory, recomputed.Inventory) ||
		!reflect.DeepEqual(supplied.Features, recomputed.Features) ||
		!reflect.DeepEqual(supplied.Seeds, recomputed.Seeds) ||
		!reflect.DeepEqual(supplied.Limitations, recomputed.Limitations) {
		return errors.New("supplied inventory does not match a fresh Git-object inventory at the pinned revision")
	}
	// CollectorExecution is an explicitly non-attesting record of the original
	// collection process. Requiring it to equal this validator's build and
	// platform would make a pinned inventory impossible to validate across
	// supported hosts while falsely suggesting that replay authenticated the
	// original binary. validateInventoryDocument already checks its closed
	// structural invariants and fixed disclosure.
	return nil
}

// validateSupportingEvidenceAgainstRepo resolves supporting citations from the
// pinned tree rather than from the worktree. Supporting citations may explain
// an assessment, but validation deliberately never lets them establish owners,
// a proposed owner, or applicability without normative-contract evidence.
func validateSupportingEvidenceAgainstRepo(ledger, inventory report, repoPath string) error {
	if err := requireAuthenticatedInputPlatform(); err != nil {
		return err
	}
	records, err := supportingEvidenceRecords(ledger)
	if err != nil || len(records) == 0 {
		return err
	}
	executable, err := trustedGitExecutable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxGitCommandDuration)
	defer cancel()
	blobs, err := resolveSupportingEvidenceBlobs(ctx, executable, repoPath, inventory.Snapshot.Revision, records)
	if err != nil {
		return err
	}
	batch := supportingBlobSlice(blobs)
	bodies, err := readPinnedBlobBodies(ctx, executable, repoPath, batch, maxSupportingEvidenceBytes, maxGitCommandDuration)
	if err != nil {
		return fmt.Errorf("read supporting evidence blobs: %w", err)
	}
	return validateSupportingEvidenceBodies(ctx, records, blobs, bodies)
}

func validateSupportingEvidenceBodies(ctx context.Context, records []supportingEvidenceRecord, blobs map[string]pinnedBlob, bodies map[string][]byte) error {
	byPath := map[string][]supportingEvidenceRecord{}
	for _, record := range records {
		byPath[record.item.Path] = append(byPath[record.item.Path], record)
	}
	paths := mapKeys(mapFromSupportingPaths(byPath))
	sort.Strings(paths)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("supporting evidence validation exceeded shared deadline: %w", err)
		}
		blob := blobs[path]
		body, ok := bodies[blob.oid]
		if !ok {
			return fmt.Errorf("supporting evidence %q was absent from the pinned batch", path)
		}
		lines := strings.Split(string(body), "\n")
		for _, record := range byPath[path] {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("supporting evidence validation exceeded shared deadline: %w", err)
			}
			if record.item.Line > len(lines) || strings.TrimSpace(lines[record.item.Line-1]) != strings.TrimSpace(record.item.Excerpt) {
				return fmt.Errorf("%s supporting evidence %s:%d does not exactly match the pinned blob", record.findingID, record.item.Path, record.item.Line)
			}
		}
	}
	return nil
}

func mapFromSupportingPaths(values map[string][]supportingEvidenceRecord) map[string]bool {
	result := make(map[string]bool, len(values))
	for path := range values {
		result[path] = true
	}
	return result
}

type supportingEvidenceRecord struct {
	findingID string
	item      evidence
}

//nolint:gocyclo // Evidence sources are enumerated explicitly to preserve attribution and budgets.
func supportingEvidenceRecords(ledger report) ([]supportingEvidenceRecord, error) {
	records := make([]supportingEvidenceRecord, 0)
	seenRecords := map[string]bool{}
	count := 0
	appendRecord := func(owner string, item evidence) error {
		if !validPath(item.Path) || item.Line < 1 {
			return fmt.Errorf("%s supporting evidence has invalid path or line", owner)
		}
		count++
		if count > maxSupportingEvidenceRecords {
			return fmt.Errorf("supporting evidence exceeds %d-record limit", maxSupportingEvidenceRecords)
		}
		key := item.Path + "\x00" + strconv.Itoa(item.Line) + "\x00" + item.Excerpt
		if seenRecords[key] {
			return nil
		}
		seenRecords[key] = true
		records = append(records, supportingEvidenceRecord{findingID: owner, item: item})
		return nil
	}
	for _, finding := range allFindings(ledger) {
		for _, items := range [][]evidence{finding.Evidence, applicabilityEvidence(finding.Applicability)} {
			for _, item := range items {
				if item.Kind != "supporting" {
					continue
				}
				if err := appendRecord("finding "+finding.ID, item); err != nil {
					return nil, err
				}
			}
		}
		for _, transfer := range finding.BDD.PlannedTransfers {
			for _, item := range transfer.BehaviorEvidence {
				if err := appendRecord("finding "+finding.ID+" planned BDD transfer "+transfer.SourceOwner, evidence{Kind: "supporting", Path: item.Path, Line: item.Line, Excerpt: item.Excerpt}); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, exclusion := range ledger.Exclusions {
		for _, item := range exclusion.SupportingEvidence {
			if err := appendRecord("review exclusion "+exclusion.Path, evidence{Kind: "supporting", Path: item.Path, Line: item.Line, Excerpt: item.Excerpt}); err != nil {
				return nil, err
			}
		}
	}
	paths := map[string]bool{}
	for _, record := range records {
		paths[record.item.Path] = true
	}
	if len(paths) > maxSupportingEvidencePaths {
		return nil, fmt.Errorf("supporting evidence exceeds %d-unique-path limit", maxSupportingEvidencePaths)
	}
	return records, nil
}

//nolint:gocyclo // Batched Git evidence resolution keeps each fail-closed bound and identity check visible.
func resolveSupportingEvidenceBlobs(ctx context.Context, executable gitExecutable, repoPath, revision string, records []supportingEvidenceRecord) (map[string]pinnedBlob, error) {
	paths := make([]string, 0, len(records))
	requested := map[string]bool{}
	for _, record := range records {
		if !requested[record.item.Path] {
			requested[record.item.Path] = true
			paths = append(paths, record.item.Path)
		}
	}
	sort.Strings(paths)
	arguments := append([]string{"ls-tree", "-r", "-z", "--long", revision, "--"}, paths...)
	outputLimit, err := supportingMetadataOutputLimit(paths)
	if err != nil {
		return nil, err
	}
	output, err := gitBytesWithContext(ctx, executable, repoPath, outputLimit, nil, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list supporting evidence from pinned revision: %w", err)
	}
	blobs := make(map[string]pinnedBlob, len(paths))
	var total int64
	for rawEntry := range bytes.SplitSeq(output, []byte{0}) {
		if len(rawEntry) == 0 {
			continue
		}
		metadata, rawPath, found := bytes.Cut(rawEntry, []byte{'\t'})
		if !found {
			return nil, errors.New("parse supporting evidence tree entry: missing path separator")
		}
		path := string(rawPath)
		if !requested[path] || blobs[path].path != "" {
			return nil, fmt.Errorf("pinned supporting evidence returned unexpected or duplicate path %q", path)
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 4 {
			return nil, fmt.Errorf("parse supporting evidence tree entry %q: invalid metadata", path)
		}
		size, err := pinnedGitBlobSize(fields, path)
		if err != nil {
			return nil, err
		}
		if size > maxSupportingEvidenceBlobBytes || size > maxSupportingEvidenceBytes-total {
			return nil, fmt.Errorf("pinned supporting evidence %q exceeds blob or aggregate byte limit", path)
		}
		total += size
		blobs[path] = pinnedBlob{path: path, oid: fields[2], size: size}
	}
	if len(blobs) != len(paths) {
		for _, path := range paths {
			if blobs[path].path == "" {
				return nil, fmt.Errorf("pinned supporting evidence path %q is absent or is not a regular blob", path)
			}
		}
	}
	return blobs, nil
}

func supportingMetadataOutputLimit(paths []string) (int64, error) {
	var outputLimit int64
	for _, path := range paths {
		if int64(len(path)) > maxGitOutputBytes-outputLimit-maxBatchHeaderBytes-1 {
			return 0, errors.New("supporting evidence metadata exceeds bounded Git output limit")
		}
		outputLimit += int64(len(path)) + maxBatchHeaderBytes + 1
	}
	if outputLimit <= 0 || outputLimit > maxGitOutputBytes {
		return 0, errors.New("supporting evidence metadata exceeds bounded Git output limit")
	}
	return outputLimit, nil
}

func supportingBlobSlice(blobs map[string]pinnedBlob) []pinnedBlob {
	result := make([]pinnedBlob, 0, len(blobs))
	for _, blob := range blobs {
		result = append(result, blob)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	return result
}

func applicabilityEvidence(entries []applicability) []evidence {
	items := []evidence{}
	for _, entry := range entries {
		items = append(items, entry.Evidence...)
	}
	return items
}

func validatePinnedSupportingEvidence(repoPath, revision string, item evidence) error {
	ledger := report{Candidates: []finding{{ID: "single", Evidence: []evidence{item}}}}
	inventory := report{Snapshot: snapshot{Revision: revision}}
	return validateSupportingEvidenceAgainstRepo(ledger, inventory, repoPath)
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
	if f.DecisionStatus != pendingMaintainerApproval {
		return fmt.Errorf("finding %q must have decision_status %q", f.ID, pendingMaintainerApproval)
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
	if !sameStringSet(ownerPaths(f.CurrentOwners), evidencePathsByKind(f.Evidence, "normative-contract")) {
		return fmt.Errorf("finding %q current owners must exactly match its source-evidence paths", f.ID)
	}
	positive := f.Verdict == "merge-now" || f.Verdict == "extract-neutral-contract"
	if !bddConsequences[f.BDD.Consequence] || !uniqueRelativeFeaturePaths(f.BDD.Features) {
		return fmt.Errorf("finding %q has invalid BDD impact", f.ID)
	}
	if err := validatePersistedPlannedBDDTransfers(f.BDD.PlannedTransfers); err != nil {
		return fmt.Errorf("finding %q has invalid planned BDD transfers: %w", f.ID, err)
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
			return fmt.Errorf("positive finding %q requires owner-completeness, proposed-owner rationale, and neutrality rationale", f.ID)
		}
		if isHarnessRegistrationOwner(f.ProposedOwner.Path) {
			return fmt.Errorf("positive finding %q cannot select harness registration proposed owner %q", f.ID, f.ProposedOwner.Path)
		}
		if err := validateNewProposedOwnerDirectory(f); err != nil {
			return fmt.Errorf("positive finding %q proposed_owner: %w", f.ID, err)
		}
		if f.OwnershipPlan == nil || f.OwnershipPlan.Status != pendingMaintainerApproval || f.OwnershipPlan.DeletionAuthority {
			return fmt.Errorf("positive finding %q requires a pending non-deletion ownership_plan", f.ID)
		}
		if f.ProposedOwner.State == "existing" {
			if !containsString(ownerPaths(f.CurrentOwners), f.ProposedOwner.Path) {
				return fmt.Errorf("positive finding %q must include existing proposed owner %q in current owners", f.ID, f.ProposedOwner.Path)
			}
		} else if containsString(ownerPaths(f.CurrentOwners), f.ProposedOwner.Path) {
			return fmt.Errorf("positive finding %q cannot mark a current owner as new", f.ID)
		}
		if !reflect.DeepEqual(f.OwnershipPlan.BDDPlannedTransfers, f.BDD.PlannedTransfers) {
			return fmt.Errorf("positive finding %q ownership_plan must exactly copy planned BDD transfers", f.ID)
		}
		if err := validateOwnerActionIntent(f); err != nil {
			return fmt.Errorf("positive finding %q owner_actions: %w", f.ID, err)
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
		case "non-harness-domain":
			if len(seenMembers) != 0 {
				if len(active) == 0 || len(seenMembers) != len(active) {
					return fmt.Errorf("non-harness-domain finding %q must omit harness rows or mark every active member", f.ID)
				}
				for _, entry := range f.Applicability {
					if entry.Disposition != "not-applicable" {
						return fmt.Errorf("non-harness-domain finding %q has non-applicable harness disposition %q", f.ID, entry.Disposition)
					}
				}
			}
		}
	}
	if f.ApplicabilityBasis == "active-members" && len(active) == 0 {
		return fmt.Errorf("finding %q cannot claim active-member parity because the pinned dear-agent registry adapter found no active members", f.ID)
	}
	if f.Verdict == "resolve-product-divergence" || f.Verdict == "insufficient-evidence" {
		if strings.TrimSpace(f.Decision) == "" || f.Strength == "strong" {
			return fmt.Errorf("finding %q requires a decision and non-strong strength", f.ID)
		}
	}
	if !positive && f.ProposedOwner != nil {
		return fmt.Errorf("non-positive finding %q cannot select a canonical owner", f.ID)
	}
	if !positive && f.OwnershipPlan != nil {
		return fmt.Errorf("non-positive finding %q cannot select an ownership_plan", f.ID)
	}
	if !positive && len(f.BDD.PlannedTransfers) != 0 {
		return fmt.Errorf("non-positive finding %q cannot select planned BDD transfers", f.ID)
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

func evidencePathsByKind(items []evidence, kind string) []string {
	paths := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		if item.Kind == kind && !seen[item.Path] {
			paths = append(paths, item.Path)
			seen[item.Path] = true
		}
	}
	return paths
}

func isHarnessRegistrationOwner(path string) bool {
	directory := pathpkg.Dir(filepath.ToSlash(path))
	segments := strings.Split(directory, "/")
	// Intrinsic registration surfaces stay local even when nested beneath a
	// logical implementation seam. Check them before applying the internal/cmd
	// exception so paths such as agm/internal/.codex/SPEC.md cannot hide.
	for index, rawSegment := range segments {
		segment := strings.ToLower(rawSegment)
		if isDottedHarnessRegistrationRoot(segment) || segment == "plugins" || strings.HasSuffix(segment, "-plugin") {
			return true
		}
		// A harness grouping is implementation-local by construction. Treat
		// unknown future members conservatively instead of waiting for an alias
		// table update to close the path.
		if (segment == "harness" || segment == "harnesses") && index+1 < len(segments) {
			return true
		}
		if index == 0 && isHarnessRegistrationAlias(segment) {
			return true
		}
	}
	for _, rawSegment := range segments {
		// Exact internal and cmd segments are eligible logical module seams.
		// Harness-like package names beneath them describe implementations when
		// no intrinsic registration marker was present.
		if rawSegment == "internal" || rawSegment == "cmd" {
			return false
		}
	}
	return false
}

func isHarnessRegistrationAlias(segment string) bool {
	_, ok := harnessRegistrationAuthority[normalizeHarnessRegistrationAlias(strings.TrimPrefix(segment, "."))]
	return ok
}

func isDottedHarnessRegistrationRoot(segment string) bool {
	if !strings.HasPrefix(segment, ".") {
		return false
	}
	alias := strings.TrimPrefix(segment, ".")
	alias = strings.TrimSuffix(alias, "-plugin")
	return isHarnessRegistrationAlias(alias)
}

func normalizeHarnessRegistrationAlias(value string) string {
	return strings.Map(func(character rune) rune {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			return character
		case character >= 'A' && character <= 'Z':
			return character + ('a' - 'A')
		default:
			return -1
		}
	}, value)
}

// harnessRegistrationAuthority is the closed authority for normalized
// canonical harness IDs, accepted aliases, and known configuration roots.
// `.dear-agent` is intentionally absent because it is a product catalog.
var harnessRegistrationAuthority = map[string]string{
	"agents":      "agents",
	"agy":         "agy",
	"agycli":      "agy",
	"antigravity": "agy",
	"aider":       "aider",
	"claude":      "claude-code",
	"claudecode":  "claude-code",
	"codex":       "codex-cli",
	"codexcli":    "codex-cli",
	"continue":    "continue",
	"cursor":      "cursor",
	"deepsec":     "deepsec",
	"gemini":      "gemini-cli",
	"geminicli":   "gemini-cli",
	"opencode":    "opencode-cli",
	"opencodecli": "opencode-cli",
	"pi":          "pi-cli",
	"picli":       "pi-cli",
	"roo":         "roo",
	"vscode":      "vscode",
	"windsurf":    "windsurf",
}

func validateNewProposedOwnerDirectory(f finding) error {
	if f.ProposedOwner == nil || f.ProposedOwner.State != "new" || !isSpecPath(f.ProposedOwner.Path) {
		return nil
	}
	proposedDirectory := pathpkg.Dir(f.ProposedOwner.Path)
	for _, owner := range f.CurrentOwners {
		if !isSpecPath(owner.Path) {
			continue
		}
		currentDirectory := pathpkg.Dir(owner.Path)
		if currentDirectory == "." || proposedDirectory == currentDirectory || strings.HasPrefix(proposedDirectory, currentDirectory+"/") {
			return fmt.Errorf("new proposed owner %q must be outside current-owner directory %q", f.ProposedOwner.Path, currentDirectory)
		}
	}
	return nil
}

//nolint:gocyclo // Closed owner-action policy is clearer as one explicit validation sequence.
func validateOwnerActionIntent(f finding) error {
	if f.ProposedOwner == nil || f.OwnershipPlan == nil {
		return errors.New("requires a proposed owner and ownership plan")
	}
	owners := stringSet(ownerPaths(f.CurrentOwners))
	actions := map[string]string{}
	for _, action := range f.OwnershipPlan.OwnerActions {
		if !owners[action.OwnerPath] || actions[action.OwnerPath] != "" || strings.TrimSpace(action.Rationale) == "" {
			return errors.New("must cover each current owner exactly once with rationale")
		}
		switch action.Disposition {
		case retainDistinctContract, retireNormativeOwnership, retireSelectedNormativeOwnership:
		default:
			return fmt.Errorf("owner %q has unsupported disposition %q", action.OwnerPath, action.Disposition)
		}
		actions[action.OwnerPath] = action.Disposition
	}
	if !sameStringSet(mapKeys(owners), mapKeys(ownerActionPaths(actions))) {
		return errors.New("must exactly cover current owners")
	}
	for ownerPath, action := range actions {
		if isHarnessRegistrationOwner(ownerPath) && action == retainDistinctContract {
			return fmt.Errorf("harness registration current owner %q cannot retain normative ownership", ownerPath)
		}
		if action == retainDistinctContract && ownerPath != f.ProposedOwner.Path {
			return fmt.Errorf("non-proposed owner %q cannot retain the selected normative contract", ownerPath)
		}
	}
	if f.ProposedOwner.State == "existing" && actions[f.ProposedOwner.Path] != retainDistinctContract {
		return fmt.Errorf("existing proposed owner %q must retain the canonical contract", f.ProposedOwner.Path)
	}
	return nil
}

func validateEvidence(items []evidence) error {
	if len(items) == 0 {
		return errors.New("evidence is required")
	}
	for _, item := range items {
		if !evidenceKinds[item.Kind] || !validPath(item.Path) || item.Line < 1 || strings.TrimSpace(item.Excerpt) == "" {
			return errors.New("evidence requires a supported kind, relative path, positive line, and excerpt")
		}
		if item.Kind == "normative-contract" && strings.TrimSpace(item.RequirementID) == "" {
			return errors.New("normative-contract evidence requires a requirement identifier")
		}
		if item.Kind == "supporting" && item.RequirementID != "" {
			return errors.New("supporting evidence cannot claim a normative requirement identifier")
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

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

var (
	verdicts                       = map[string]bool{"merge-now": true, "extract-neutral-contract": true, "keep-separate": true, "resolve-product-divergence": true, "insufficient-evidence": true}
	relationships                  = map[string]bool{"same-observable": true, "overlapping-observables": true, "contradictory-observables": true, "same-vocabulary-only": true, "fixture-or-generated-copy": true}
	classifications                = map[string]bool{"shared-contract": true, "capability-variation": true, "wrapper": true, "fixture": true, "implementation-detail": true}
	confidences                    = map[string]bool{"confirmed": true, "likely": true, "tentative": true}
	strengths                      = map[string]bool{"strong": true, "moderate": true, "exploratory": true}
	ownerStates                    = map[string]bool{"existing": true, "new": true}
	applicabilityBases             = map[string]bool{"active-members": true, "non-harness-domain": true}
	dispositions                   = map[string]bool{"supported": true, "adapted": true, "unsupported": true, "not-applicable": true, "unknown": true}
	bddConsequences                = map[string]bool{"merge": true, "add-matrix": true, "preserve-residual": true, "applicability-specific": true, "none": true, "resolve": true}
	seedKinds                      = map[string]bool{"exact-body": true, "duplicate-id": true, "shared-bdd": true, "identical-file": true, "harness-terminology": true}
	evidenceKinds                  = map[string]bool{"normative-contract": true, "supporting": true}
	diagnosticKinds                = map[string]bool{"anonymous-requirement": true, "nonconforming-requirement": true, "missing-bdd-feature": true, "nonreciprocal-bdd-feature": true, "malformed-bdd-feature-reference": true, "duplicate-bdd-feature-reference": true, "ambiguous-bdd-traceability-section": true}
	featureDiagnosticKinds         = map[string]bool{"missing-feature-spec-reference": true, "malformed-feature-spec-reference": true, "ambiguous-feature-spec-reference": true, "missing-feature-spec": true, "nonreciprocal-feature-spec": true}
	reviewExclusionClassifications = map[string]bool{"fixture": true, "generated": true, "third-party": true, "archived": true, "nested-repository": true}
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

func validReviewExclusions(items []reviewExclusion) bool {
	seen := map[string]bool{}
	for _, item := range items {
		if !validPath(item.Path) || !reviewExclusionClassifications[item.Classification] || strings.TrimSpace(item.Rationale) == "" || len(item.SupportingEvidence) == 0 || seen[item.Path] {
			return false
		}
		if err := validatePersistedEvidence(nil, item.SupportingEvidence); err != nil {
			return false
		}
		seen[item.Path] = true
	}
	return true
}

func reviewExclusionsForPresentation(items []reviewExclusion) []exclusion {
	result := make([]exclusion, 0, len(items))
	for _, item := range items {
		result = append(result, exclusion{Path: item.Path, Reason: item.Classification + ": " + item.Rationale})
	}
	return result
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
	if strings.TrimSpace(path) == "" || len(path) > maxGitPathBytes || !utf8.ValidString(path) || filepath.IsAbs(path) || strings.Contains(path, "\\") {
		return false
	}
	for _, value := range path {
		if value < 0x20 || value == 0x7f {
			return false
		}
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean == path && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
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
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;color:var(--ink);background:var(--paper);font:16px/1.55 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}a{color:var(--teal)}code{font:0.9em/1.45 ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--code);padding:.08rem .3rem;border-radius:4px;overflow-wrap:anywhere}.shell{width:min(1180px,calc(100% - 2rem));margin:auto}.hero{padding:4.5rem 0 2.3rem;border-bottom:1px solid var(--line);background:linear-gradient(135deg,#fffaf0 0%,#e5efec 100%)}.eyebrow{margin:0 0 .5rem;color:var(--accent);font-weight:800;letter-spacing:.14em;text-transform:uppercase;font-size:.76rem}.hero h1{max-width:850px;margin:.1rem 0 .8rem;font:700 clamp(2.25rem,6vw,4.6rem)/.98 ui-serif,Georgia,serif;letter-spacing:-.04em}.lede{max-width:780px;color:var(--muted);font-size:1.08rem}.snapshot{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:.75rem;margin:1.5rem 0}.snapshot div{padding:.8rem 0;border-top:1px solid var(--line)}.snapshot dt{color:var(--muted);font-size:.75rem;text-transform:uppercase;letter-spacing:.08em}.snapshot dd{margin:.2rem 0 0}.warning{margin:1.2rem 0 0;padding:1rem 1.1rem;border:1px solid #e3b6a5;border-left:5px solid var(--accent);background:#fff5ef}.toc{position:sticky;top:0;z-index:5;background:rgba(244,241,234,.96);border-bottom:1px solid var(--line);backdrop-filter:blur(8px)}.toc .shell{display:flex;gap:1.1rem;overflow-x:auto;padding:.72rem 0}.toc a{white-space:nowrap;text-decoration:none;font-weight:700;font-size:.86rem}main{padding:2.2rem 0 5rem}section{scroll-margin-top:4rem;margin:0 0 3.5rem}h2{font:700 clamp(1.6rem,3vw,2.35rem)/1.05 ui-serif,Georgia,serif;letter-spacing:-.02em;margin:0 0 1rem}h3{font-size:1.25rem;line-height:1.25;margin:.15rem 0}.section-intro{max-width:780px;color:var(--muted)}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:.8rem}.metric{padding:1.05rem;border:1px solid var(--line);background:var(--card);box-shadow:0 3px 0 rgba(19,34,56,.04)}.metric strong{display:block;font:700 2rem/1 ui-serif,Georgia,serif}.metric span{color:var(--muted);font-size:.82rem}.scope-grid,.two-col{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.scope-grid>*,.two-col>*,.topology>*,.toolbar>*{min-width:0}.panel{padding:1.15rem;border:1px solid var(--line);background:var(--card)}.pill,.tag{display:inline-flex;align-items:center;margin:.18rem .25rem .18rem 0;padding:.23rem .55rem;border-radius:999px;background:var(--soft);font-size:.78rem;font-weight:800}.tag.verdict{background:#d9ebe8;color:#0d5753}.tag.warn{background:#f6dfd5;color:#7a2716}.top-pick{padding:1.35rem;border:1px solid #b9cfc9;border-left:6px solid var(--teal);background:#f7fcfa}.top-pick .rank{color:var(--teal);font-weight:900}.topology{display:grid;grid-template-columns:minmax(0,1fr) auto minmax(0,1fr);align-items:center;gap:.8rem;margin:1rem 0}.owner-box{height:100%;padding:.8rem;border:1px solid var(--line);background:var(--card)}.arrow{font-size:1.6rem;color:var(--teal)}.toolbar{display:grid;grid-template-columns:2fr 1fr;gap:.7rem;margin:1rem 0}.toolbar input,.toolbar select{width:100%;padding:.75rem;border:1px solid var(--line);border-radius:0;background:#fff;font:inherit}.finding{margin:1rem 0;padding:1.3rem;border:1px solid var(--line);background:var(--card);box-shadow:0 7px 18px rgba(19,34,56,.05)}.finding-head{display:flex;justify-content:space-between;gap:1rem;align-items:flex-start}.finding-id{color:var(--muted);font:700 .78rem/1.2 ui-monospace,SFMono-Regular,Menlo,monospace}.outcome{font-size:1.04rem}.label{display:block;margin:1rem 0 .35rem;color:var(--muted);font-size:.73rem;font-weight:900;letter-spacing:.09em;text-transform:uppercase}.compact{margin:.35rem 0;padding-left:1.2rem}.compact li{margin:.25rem 0}.matrix{width:100%;border-collapse:collapse;font-size:.88rem}.matrix th,.matrix td{text-align:left;vertical-align:top;padding:.45rem;border-bottom:1px solid var(--line)}.matrix th{color:var(--muted)}.evidence{list-style:none;padding:0}.evidence li{margin:.45rem 0;padding:.7rem;border-left:3px solid #aebdb8;background:#f4f7f4}.decision{padding:.85rem;border-left:4px solid var(--gold);background:#fff8e8}.risk{padding:.8rem;background:#fff3ee}.empty{color:var(--muted);font-style:italic}.seed-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:.7rem}.seed{padding:.85rem;border:1px solid var(--line);background:var(--card)}details{margin-top:.8rem}summary{cursor:pointer;font-weight:800}.source-note{padding:1rem;border:1px dashed var(--teal);background:#f4fbf9}.footer{padding:1.2rem 0 3rem;color:var(--muted);border-top:1px solid var(--line)}[hidden]{display:none!important}
html,body,.toc{overflow-x:hidden}
@media(max-width:720px){.hero{padding-top:3rem}.scope-grid,.two-col,.topology,.toolbar{grid-template-columns:minmax(0,1fr)}.arrow{transform:rotate(90deg);justify-self:center}.finding-head{display:block}.shell{width:min(100% - 1rem,1180px)}.matrix{display:block;overflow-x:auto}}
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
	if audit.CollectorExecution != nil {
		out.WriteString("<p class=\"source-note\"><strong>Collector execution disclosure.</strong> ")
		fmt.Fprintf(out, "%s", esc(audit.CollectorExecutionDisclosure))
		out.WriteString("</p>")
	}
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
		fmt.Fprintf(out, "<p><code>%s</code></p><span class=\"pill\">%s owner</span><p>%s</p><p><strong>Neutrality basis:</strong> %s</p>", esc(finding.ProposedOwner.Path), esc(finding.ProposedOwner.State), esc(finding.ProposedOwner.Rationale), esc(finding.ProposedOwner.NeutralityRationale))
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
	if finding.DecisionStatus != "" {
		fmt.Fprintf(out, "<p class=\"source-note\"><strong>Decision status:</strong> %s</p>", esc(finding.DecisionStatus))
	}
	renderOwnershipPlan(out, finding.OwnershipPlan)
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
	renderPlannedBDDTransfers(out, finding.BDD.PlannedTransfers)
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

func renderOwnershipPlan(out *boundedHTMLBuilder, plan *ownershipPlan) {
	if plan == nil {
		return
	}
	out.WriteString("<details open><summary>Maintainer-pending ownership preservation plan</summary>")
	fmt.Fprintf(out, "<p class=\"source-note\"><strong>Status:</strong> %s. This plan preserves traceability for review and is not deletion authority.</p>", esc(plan.Status))
	out.WriteString("<span class=\"label\">Owner actions</span><ul class=\"compact\">")
	for _, action := range plan.OwnerActions {
		fmt.Fprintf(out, "<li><code>%s</code> · %s — %s</li>", esc(action.OwnerPath), esc(action.Disposition), esc(action.Rationale))
	}
	out.WriteString("</ul><span class=\"label\">Requirement preservation</span><ul class=\"compact\">")
	for _, entry := range plan.Requirements {
		fmt.Fprintf(out, "<li><code>%s:%d %s</code> → <code>%s %s</code> (%s, %s)</li>", esc(entry.ContractEvidence.Path), entry.ContractEvidence.Line, esc(entry.ContractEvidence.RequirementID), esc(entry.TargetPath), esc(entry.TargetRequirementID), esc(entry.TargetState), esc(entry.Disposition))
	}
	out.WriteString("</ul><span class=\"label\">BDD preservation</span><ul class=\"compact\">")
	for _, entry := range plan.Features {
		fmt.Fprintf(out, "<li><code>%s</code> via <code>%s</code> → <code>%s</code> (%s, %s)</li>", esc(entry.Path), esc(entry.SourceOwner), esc(entry.TargetPath), esc(entry.TargetState), esc(entry.Disposition))
	}
	out.WriteString("</ul>")
	renderPlannedBDDTransfers(out, plan.BDDPlannedTransfers)
	out.WriteString("</details>")
}

func renderPlannedBDDTransfers(out *boundedHTMLBuilder, transfers []plannedBDDTransfer) {
	if len(transfers) == 0 {
		return
	}
	out.WriteString("<span class=\"label\">Planned BDD traceability transfers</span><ul class=\"compact\">")
	for _, transfer := range transfers {
		fmt.Fprintf(out, "<li><span class=\"tag\">PLANNED</span> <code>%s</code> → <code>%s</code> via <code>%s</code><br>%s", esc(transfer.SourceOwner), esc(transfer.TargetOwner), esc(transfer.TargetFeature), esc(transfer.Rationale))
		items := make([]evidence, 0, len(transfer.BehaviorEvidence))
		for _, item := range transfer.BehaviorEvidence {
			items = append(items, evidence{Kind: "supporting", Path: item.Path, Line: item.Line, Excerpt: item.Excerpt})
		}
		renderEvidence(out, items, "planned-bdd-evidence")
		out.WriteString("</li>")
	}
	out.WriteString("</ul><p class=\"source-note\">PLANNED entries are maintainer-pending traceability work, not current reciprocal links or deterministic semantic proof.</p>")
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
		fmt.Fprintf(out, "<br><span class=\"pill\">%s</span> %s</li>", esc(item.Kind), esc(item.Excerpt))
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
