// Package specguard evaluates changed SPEC contracts from immutable Git
// snapshots. Staged mode also admits bounded Git-reported worktree path/status
// and index-flag metadata so dirty governed files cannot be silently skipped.
// It does not inspect or attest provider, installation, or runtime state.
package specguard

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/repoinventory"
)

const (
	// SchemaVersion identifies the stable JSON result shape.
	SchemaVersion = "spec-guard/v1"
	// GuardScope is emitted verbatim so consumers cannot mistake a source result
	// for provider-visible or runtime evidence.
	GuardScope = "provider-neutral SPEC source guard"
	// EvidenceClaim is the canonical limit on what a successful or failed
	// local evaluation proves.
	EvidenceClaim = "Semantic validation is limited to local Git index or commit-tree objects. Staged mode additionally inspects only bounded Git-reported tracked and nonignored untracked path/status metadata plus assume-unchanged and skip-worktree index flags to prevent dirty governed paths from being silently skipped; mutable working-tree bodies are not parsed, and no provider, installation, hook registration, or runtime state is inspected."
	// TrustBoundary is the canonical disclosure for trusted local inputs.
	TrustBoundary = "For admitted snapshots, the guard pins and checkpoint-revalidates the canonical worktree root, Git directory, Git common directory, their ancestors, and .git and commondir selectors before and after each Git command. It trusts the selected Git executable, repository metadata and local configuration, object store, configured object alternates, ignore rules used to classify nonignored paths, and filesystem behavior between checkpoints. It sanitizes ambient Git configuration, disables replacement objects and lazy fetching, verifies returned blob bytes against object IDs, requires descendant-process termination before launching Git, and does not attest commit signatures or remote provenance. Repository-local terminal-hook adapters execute mutable checkout code and therefore provide cooperative feedback only; they are not tamper-resistant. Any mandatory immutable enforcement must come from a separately reviewed changed-SPEC CI and provider rollout; this source result does not attest that such enforcement is deployed, has run for a change, or is provider-required."
)

const (
	defaultWallTime       = 45 * time.Second
	defaultGitTime        = 10 * time.Second
	defaultMaxGitOutput   = int64(40 * 1024 * 1024)
	defaultMaxGitInput    = int64(1 * 1024 * 1024)
	defaultMaxCorpusBytes = int64(32 * 1024 * 1024)
	defaultMaxFileBytes   = int64(1 * 1024 * 1024)
	defaultMaxFiles       = 4096
	defaultMaxEntries     = 100_000
	defaultMaxChanged     = 4096
	defaultMaxFindings    = 2048
	defaultMaxPathBytes   = 1024
	defaultMaxRefBytes    = 256
)

// Mode selects the immutable Git snapshot that the guard evaluates.
type Mode string

const (
	// ModeStaged evaluates the current Git index against HEAD.
	ModeStaged Mode = "staged"
	// ModeCommitted evaluates the committed base..HEAD range.
	ModeCommitted Mode = "committed"
)

// Decision is the machine-actionable guard disposition.
type Decision string

const (
	// DecisionAllow means no governed contract changed.
	DecisionAllow Decision = "allow"
	// DecisionBlock means a deterministic policy or safety failure occurred.
	DecisionBlock Decision = "block"
	// DecisionReminder means deterministic checks passed and semantic review remains.
	DecisionReminder Decision = "reminder"
)

// Request is the complete caller interface. Base is required only for
// ModeCommitted; HEAD is intentionally resolved and pinned by the module.
type Request struct {
	Repository string `json:"repository"`
	Mode       Mode   `json:"mode"`
	Base       string `json:"base,omitempty"`
}

// Finding records one deterministic policy or safety failure.
type Finding struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// Result is deterministic for a pinned index or commit pair. A reminder means
// deterministic checks passed but semantic neutrality still needs review.
type Result struct {
	SchemaVersion string    `json:"schema_version"`
	Scope         string    `json:"scope"`
	Decision      Decision  `json:"decision"`
	Mode          Mode      `json:"mode"`
	Source        string    `json:"source"`
	Base          string    `json:"base,omitempty"`
	Head          string    `json:"head,omitempty"`
	SnapshotID    string    `json:"snapshot_id,omitempty"`
	Changed       []string  `json:"changed"`
	Findings      []Finding `json:"findings"`
	Reminder      string    `json:"reminder,omitempty"`
	EvidenceClaim string    `json:"evidence_claim"`
	TrustBoundary string    `json:"trust_boundary"`
}

type guardLimits struct {
	wallTime       time.Duration
	gitTime        time.Duration
	maxGitOutput   int64
	maxGitInput    int64
	maxCorpusBytes int64
	maxFileBytes   int64
	maxFiles       int
	maxEntries     int
	maxChanged     int
	maxFindings    int
	maxPathBytes   int
	maxRefBytes    int
}

func defaultLimits() guardLimits {
	return guardLimits{
		wallTime:       defaultWallTime,
		gitTime:        defaultGitTime,
		maxGitOutput:   defaultMaxGitOutput,
		maxGitInput:    defaultMaxGitInput,
		maxCorpusBytes: defaultMaxCorpusBytes,
		maxFileBytes:   defaultMaxFileBytes,
		maxFiles:       defaultMaxFiles,
		maxEntries:     defaultMaxEntries,
		maxChanged:     defaultMaxChanged,
		maxFindings:    defaultMaxFindings,
		maxPathBytes:   defaultMaxPathBytes,
		maxRefBytes:    defaultMaxRefBytes,
	}
}

type guardDependencies struct {
	gitExecutable            string
	afterAdmissionGitCommand func()
	afterRepositoryAdmission func()
	afterGitCommand          func()
	afterDirtyWorktreeRead   func()
	afterFinalDirtyRead      func()
	afterIndexRead           func()
}

// Evaluate applies the complete deterministic policy through one deep module
// interface. Policy violations are returned as DecisionBlock, not Go errors,
// so every caller can emit the same typed result.
func Evaluate(ctx context.Context, request Request) Result {
	return evaluate(ctx, request, defaultLimits(), guardDependencies{})
}

func evaluate(ctx context.Context, request Request, limits guardLimits, dependencies guardDependencies) Result {
	result := baseResult(request.Mode)
	if failure := validateRequest(ctx, request, limits); failure != nil {
		result.Findings = []Finding{failure.finding()}
		return result
	}

	guardCtx, cancel := context.WithTimeout(ctx, limits.wallTime)
	defer cancel()

	executable := dependencies.gitExecutable
	if executable == "" {
		resolved, failure := resolveGitExecutable()
		if failure != nil {
			result.Findings = []Finding{failure.finding()}
			return result
		}
		executable = resolved
	}
	git := gitClient{executable: executable, limits: limits, afterCommand: dependencies.afterAdmissionGitCommand}
	repository, failure := git.admitRepository(guardCtx, request.Repository)
	if failure != nil {
		result.Findings = []Finding{failure.finding()}
		return result
	}
	git.repository = repository
	git.afterCommand = dependencies.afterGitCommand
	if dependencies.afterRepositoryAdmission != nil {
		dependencies.afterRepositoryAdmission()
	}
	root := repository.root

	var snapshot gitSnapshot
	switch request.Mode {
	case ModeStaged:
		result.Source = "Git index object IDs compared with pinned HEAD after bounded dirty-worktree path/status and index-flag admission"
		snapshot, failure = git.stagedSnapshot(guardCtx, root, dependencies.afterDirtyWorktreeRead, dependencies.afterFinalDirtyRead, dependencies.afterIndexRead)
	case ModeCommitted:
		result.Source = "Git commit-tree object IDs from pinned base..HEAD"
		snapshot, failure = git.committedSnapshot(guardCtx, root, request.Base)
	}
	if failure != nil {
		result.Findings = []Finding{failure.finding()}
		return result
	}
	result.Base = snapshot.base
	result.Head = snapshot.head
	result.SnapshotID = snapshot.identity

	collector := findingCollector{limit: limits.maxFindings}
	changedGoverned := make(map[string]change)
	for _, changed := range snapshot.changed {
		if !isGovernedPath(changed.path) {
			continue
		}
		changedGoverned[changed.path] = changed
		result.Changed = append(result.Changed, changed.path)
	}
	sort.Strings(result.Changed)
	if len(changedGoverned) == 0 {
		result.Decision = DecisionAllow
		return result
	}

	if failure := validateSnapshot(guardCtx, snapshot, changedGoverned, &collector); failure != nil {
		collector.add(failure.finding())
	}
	result.Findings = collector.sorted()
	if len(result.Findings) != 0 {
		result.Decision = DecisionBlock
		return result
	}

	result.Decision = DecisionReminder
	result.Reminder = "Deterministic object, dirty-worktree admission, EARS, ownership-path, and reciprocal BDD checks passed; semantic contract consolidation, harness/implementation neutrality, and any changed-path retirement or stable-ID migration still require review. Repository-local hook feedback is cooperative. A separately reviewed changed-SPEC CI and provider rollout is required for mandatory immutable enforcement, which this result does not attest."
	return result
}

func baseResult(mode Mode) Result {
	return Result{
		SchemaVersion: SchemaVersion,
		Scope:         GuardScope,
		Decision:      DecisionBlock,
		Mode:          mode,
		Changed:       []string{},
		Findings:      []Finding{},
		EvidenceClaim: EvidenceClaim,
		TrustBoundary: TrustBoundary,
	}
}

//nolint:gocyclo // Explicit field-by-field admission keeps the small interface fail closed.
func validateRequest(ctx context.Context, request Request, limits guardLimits) *guardFailure {
	if ctx == nil {
		return fail("invalid-input", "", "context is required")
	}
	if limits.wallTime <= 0 || limits.gitTime <= 0 || limits.maxGitOutput <= 0 ||
		limits.maxGitInput <= 0 || limits.maxCorpusBytes <= 0 || limits.maxFileBytes <= 0 ||
		limits.maxFiles <= 0 || limits.maxEntries <= 0 || limits.maxChanged <= 0 ||
		limits.maxFindings <= 0 || limits.maxPathBytes <= 0 || limits.maxRefBytes <= 0 {
		return fail("invalid-limits", "", "all guard limits must be positive")
	}
	if request.Repository == "" || len(request.Repository) > 4096 || strings.IndexByte(request.Repository, 0) >= 0 {
		return fail("invalid-input", "", "repository must be a non-empty bounded path")
	}
	switch request.Mode {
	case ModeStaged:
		if request.Base != "" {
			return fail("invalid-input", "", "base is not accepted in staged mode")
		}
	case ModeCommitted:
		if !validRevision(request.Base, limits.maxRefBytes) {
			return fail("invalid-input", "", "committed mode requires a bounded, unambiguous base revision")
		}
	default:
		return fail("invalid-input", "", "mode must be staged or committed")
	}
	return nil
}

//nolint:gocyclo // Contract, graph, and reciprocity checks stay behind the one module interface.
func validateSnapshot(ctx context.Context, snapshot gitSnapshot, changed map[string]change, findings *findingCollector) *guardFailure {
	specs := make(map[string]specDocument)
	features := make(map[string]featureDocument)
	owners := make(map[string]string)
	ownerErrors := make(map[string]error)
	for filePath, file := range snapshot.files {
		if ctx.Err() != nil {
			return fail("guard-time-limit", "", "SPEC policy evaluation exceeded its wall-time limit")
		}
		switch {
		case isSpecPath(filePath):
			specs[filePath] = parseSpecDocument(filePath, file.body)
		case isSpecOwnerPath(filePath):
			localSpecPath := path.Join(path.Dir(filePath), "SPEC.md")
			target, err := ParseSpecOwner(file.body, localSpecPath)
			if err != nil {
				ownerErrors[filePath] = err
			} else {
				owners[filePath] = target
			}
		case isFeaturePath(filePath):
			features[filePath] = parseFeatureDocument(file.body)
		}
	}

	affectedSpecs := make(map[string]bool)
	affectedFeatures := make(map[string]bool)
	ownerAffectedSpecs := make(map[string]bool)
	requireImplementationOwnership := func(implementationDir, evidencePath string) {
		localSpecPath := path.Join(implementationDir, "SPEC.md")
		ownerPath := path.Join(implementationDir, "SPEC.owner")
		_, localExists := specs[localSpecPath]
		target, ownerExists := owners[ownerPath]
		ownerErr, invalidOwnerExists := ownerErrors[ownerPath]
		switch {
		case localExists && (ownerExists || invalidOwnerExists):
			findings.add(Finding{Code: "ambiguous-spec-owner", Path: ownerPath, Message: "implementation directory declares both SPEC.md and SPEC.owner"})
		case localExists:
			affectedSpecs[localSpecPath] = true
			ownerAffectedSpecs[localSpecPath] = true
		case invalidOwnerExists:
			findings.add(Finding{Code: "invalid-spec-owner", Path: ownerPath, Message: ownerErr.Error()})
		case ownerExists:
			if _, exists := specs[target]; !exists {
				findings.add(Finding{Code: "missing-spec-owner-target", Path: ownerPath, Message: fmt.Sprintf("canonical shared SPEC %q is absent from the selected Git snapshot", target)})
				return
			}
			affectedSpecs[target] = true
			ownerAffectedSpecs[target] = true
		default:
			findings.add(Finding{
				Code:    "missing-implementation-spec-owner",
				Path:    evidencePath,
				Message: "a surviving changed implementation directory must retain a SPEC.owner edge or permitted local SPEC ownership",
			})
		}
	}
	for filePath, item := range changed {
		if item.status == "D" {
			// Deletions are not an unconditional bypass or failure. Validate every
			// surviving reciprocal edge that still names the deleted path. A
			// same-change relocation or deliberate retirement can therefore reach
			// the mandatory semantic review, while dangling graph edges still
			// fail deterministically from the selected immutable snapshot.
			switch {
			case isSpecPath(filePath):
				for featurePath, document := range features {
					if contains(document.relatedSpecs(), filePath) {
						affectedFeatures[featurePath] = true
					}
				}
				for ownerPath, target := range owners {
					if target == filePath {
						findings.add(Finding{Code: "missing-spec-owner-target", Path: ownerPath, Message: fmt.Sprintf("canonical shared SPEC %q is absent from the selected Git snapshot", target)})
					}
				}
			case isFeaturePath(filePath):
				for specPath, document := range specs {
					if contains(document.features, filePath) {
						affectedSpecs[specPath] = true
					}
				}
			case isSpecOwnerPath(filePath):
				implementationDirs := make(map[string]bool)
				if implementationDir := path.Dir(filePath); snapshot.implementationDirs[implementationDir] {
					implementationDirs[implementationDir] = true
				}
				for _, implementationChange := range snapshot.changed {
					if implementationChange.status != "D" && snapshot.implementationPaths[implementationChange.path] {
						implementationDirs[path.Dir(implementationChange.path)] = true
					}
				}
				for implementationDir := range implementationDirs {
					evidencePath := path.Join(implementationDir, "SPEC.owner")
					requireImplementationOwnership(implementationDir, evidencePath)
				}
			}
			continue
		}
		switch {
		case isSpecPath(filePath):
			affectedSpecs[filePath] = true
			ownerPath := path.Join(path.Dir(filePath), "SPEC.owner")
			if _, exists := snapshot.files[ownerPath]; exists {
				findings.add(Finding{Code: "ambiguous-spec-owner", Path: ownerPath, Message: "implementation directory declares both SPEC.md and SPEC.owner"})
			}
		case isSpecOwnerPath(filePath):
			if !snapshot.implementationDirs[path.Dir(filePath)] {
				findings.add(Finding{Code: "orphan-spec-owner", Path: filePath, Message: "SPEC.owner does not belong to an implementation directory in the selected Git snapshot"})
				continue
			}
			if err := ownerErrors[filePath]; err != nil {
				findings.add(Finding{Code: "invalid-spec-owner", Path: filePath, Message: err.Error()})
				continue
			}
			target := owners[filePath]
			localSpecPath := path.Join(path.Dir(filePath), "SPEC.md")
			if _, exists := specs[localSpecPath]; exists {
				findings.add(Finding{Code: "ambiguous-spec-owner", Path: filePath, Message: "implementation directory declares both SPEC.md and SPEC.owner"})
				continue
			}
			if _, exists := specs[target]; !exists {
				findings.add(Finding{Code: "missing-spec-owner-target", Path: filePath, Message: fmt.Sprintf("canonical shared SPEC %q is absent from the selected Git snapshot", target)})
				continue
			}
			affectedSpecs[target] = true
			ownerAffectedSpecs[target] = true
		case isFeaturePath(filePath):
			affectedFeatures[filePath] = true
		}
	}
	featuresBySpec, specsByFeature := buildGraphIndexes(specs, features)
	if !expandAffectedGraph(ctx, specs, features, featuresBySpec, specsByFeature, affectedSpecs, affectedFeatures) {
		return fail("guard-time-limit", "", "SPEC graph evaluation exceeded its wall-time limit")
	}

	for specPath := range affectedSpecs {
		if ctx.Err() != nil {
			return fail("guard-time-limit", "", "SPEC policy evaluation exceeded its wall-time limit")
		}
		document, exists := specs[specPath]
		if !exists {
			findings.add(Finding{Code: "missing-spec-object", Path: specPath, Message: "affected SPEC is absent from the selected Git snapshot"})
			continue
		}
		_, directlyChanged := changed[specPath]
		strictlyAffected := directlyChanged || ownerAffectedSpecs[specPath]
		for _, issue := range document.issues {
			if issue.category == issueEARS && !strictlyAffected {
				continue
			}
			findings.add(issue.finding(specPath))
		}
		if strictlyAffected {
			if IsHarnessRegistrationOwner(specPath) {
				findings.add(Finding{Code: "non-neutral-spec-owner", Path: specPath, Message: "normative SPEC ownership is forbidden beneath a harness configuration or registration root"})
			}
			if len(document.features) == 0 {
				findings.add(Finding{Code: "missing-bdd-traceability", Path: specPath, Message: "a changed SPEC must reference at least one reciprocal BDD feature"})
			}
			primary := false
			for _, featurePath := range document.features {
				if feature, ok := features[featurePath]; ok && len(feature.primary) == 1 && feature.primary[0] == specPath {
					primary = true
				}
			}
			if !primary {
				findings.add(Finding{Code: "missing-primary-contract-feature", Path: specPath, Message: "a changed SPEC must be the single primary owner of at least one referenced BDD feature"})
			}
		}
		for _, featurePath := range document.features {
			feature, ok := features[featurePath]
			switch {
			case !ok:
				findings.add(Finding{Code: "missing-bdd-feature", Path: specPath, Message: fmt.Sprintf("referenced BDD feature %q is absent from the selected Git snapshot", featurePath)})
			case !contains(feature.relatedSpecs(), specPath):
				findings.add(Finding{Code: "nonreciprocal-bdd-link", Path: specPath, Message: fmt.Sprintf("BDD feature %q does not link back to this SPEC", featurePath)})
			}
		}
		for _, featurePath := range featuresBySpec[specPath] {
			if !contains(document.features, featurePath) {
				findings.add(Finding{Code: "nonreciprocal-spec-link", Path: specPath, Message: fmt.Sprintf("BDD feature %q links this SPEC but the SPEC does not link back", featurePath)})
			}
		}
	}

	for featurePath := range affectedFeatures {
		if ctx.Err() != nil {
			return fail("guard-time-limit", "", "SPEC policy evaluation exceeded its wall-time limit")
		}
		document, exists := features[featurePath]
		if !exists {
			findings.add(Finding{Code: "missing-feature-object", Path: featurePath, Message: "affected BDD feature is absent from the selected Git snapshot"})
			continue
		}
		for _, issue := range document.issues {
			findings.add(issue.finding(featurePath))
		}
		if len(document.primary) != 1 {
			findings.add(Finding{Code: "bdd-primary-owner-count", Path: featurePath, Message: "a BDD feature must declare exactly one primary # SPEC owner"})
		} else if IsHarnessRegistrationOwner(document.primary[0]) {
			findings.add(Finding{Code: "non-neutral-feature-owner", Path: featurePath, Message: fmt.Sprintf("primary owner %q is beneath a harness configuration or registration root", document.primary[0])})
		}
		if document.executableScenarios == 0 {
			findings.add(Finding{Code: "missing-executable-scenario", Path: featurePath, Message: "a BDD feature must contain a concrete Scenario with a Then assertion, or a Scenario Outline with a Then assertion and nonempty Examples"})
		}
		for _, specPath := range document.relatedSpecs() {
			spec, ok := specs[specPath]
			switch {
			case !ok:
				findings.add(Finding{Code: "missing-related-spec", Path: featurePath, Message: fmt.Sprintf("related SPEC %q is absent from the selected Git snapshot", specPath)})
			case !contains(spec.features, featurePath):
				findings.add(Finding{Code: "nonreciprocal-feature-link", Path: featurePath, Message: fmt.Sprintf("related SPEC %q does not link back to this feature", specPath)})
			}
		}
	}
	return nil
}

func implementationDirs(entries []treeEntry) map[string]bool {
	dirs := make(map[string]bool)
	for _, entry := range entries {
		if isImplementationEntry(entry) {
			dirs[path.Dir(entry.path)] = true
		}
	}
	return dirs
}

func implementationPaths(entries []treeEntry) map[string]bool {
	paths := make(map[string]bool)
	for _, entry := range entries {
		if isImplementationEntry(entry) {
			paths[entry.path] = true
		}
	}
	return paths
}

func isImplementationEntry(entry treeEntry) bool {
	regular := entry.stage == 0 && (entry.objectType == "" || entry.objectType == "blob") && (entry.mode == "100644" || entry.mode == "100755")
	return repoinventory.IsImplementationSource(entry.path, regular, entry.mode == "100755")
}

func buildGraphIndexes(specs map[string]specDocument, features map[string]featureDocument) (map[string][]string, map[string][]string) {
	featuresBySpec := make(map[string][]string)
	specsByFeature := make(map[string][]string)
	for specPath, document := range specs {
		for _, featurePath := range document.features {
			specsByFeature[featurePath] = append(specsByFeature[featurePath], specPath)
		}
	}
	for featurePath, document := range features {
		for _, specPath := range document.relatedSpecs() {
			featuresBySpec[specPath] = append(featuresBySpec[specPath], featurePath)
		}
	}
	return featuresBySpec, specsByFeature
}

type graphNode struct {
	path   string
	isSpec bool
}

func expandAffectedGraph(
	ctx context.Context,
	specs map[string]specDocument,
	features map[string]featureDocument,
	featuresBySpec map[string][]string,
	specsByFeature map[string][]string,
	affectedSpecs map[string]bool,
	affectedFeatures map[string]bool,
) bool {
	queue := make([]graphNode, 0, len(affectedSpecs)+len(affectedFeatures))
	for specPath := range affectedSpecs {
		queue = append(queue, graphNode{path: specPath, isSpec: true})
	}
	for featurePath := range affectedFeatures {
		queue = append(queue, graphNode{path: featurePath})
	}
	for index := 0; index < len(queue); index++ {
		if ctx.Err() != nil {
			return false
		}
		node := queue[index]
		if node.isSpec {
			neighbors := append([]string{}, specs[node.path].features...)
			neighbors = append(neighbors, featuresBySpec[node.path]...)
			for _, featurePath := range neighbors {
				if !affectedFeatures[featurePath] {
					affectedFeatures[featurePath] = true
					queue = append(queue, graphNode{path: featurePath})
				}
			}
			continue
		}
		neighbors := append([]string{}, features[node.path].relatedSpecs()...)
		neighbors = append(neighbors, specsByFeature[node.path]...)
		for _, specPath := range neighbors {
			if !affectedSpecs[specPath] {
				affectedSpecs[specPath] = true
				queue = append(queue, graphNode{path: specPath, isSpec: true})
			}
		}
	}
	return true
}

// IsHarnessRegistrationOwner reports whether a normative owner path is below
// a harness configuration, registration, or plugin root.
func IsHarnessRegistrationOwner(specPath string) bool {
	directory := path.Dir(specPath)
	segments := strings.Split(directory, "/")
	for index, rawSegment := range segments {
		segment := strings.ToLower(rawSegment)
		// Intrinsic registration roots remain local even below logical internal
		// or cmd seams. A harness/harnesses collection is also intrinsically
		// local for every member, including future IDs not in the pinned table.
		if isDottedHarnessRoot(segment) || segment == "plugins" || strings.HasSuffix(segment, "-plugin") {
			return true
		}
		if (segment == "harness" || segment == "harnesses") && index+1 < len(segments) {
			return true
		}
		// Bare top-level names use the deliberately pinned authority below.
		if index == 0 && isHarnessAlias(segment) {
			return true
		}
	}
	return false
}

func isHarnessRegistrationOwner(specPath string) bool { return IsHarnessRegistrationOwner(specPath) }

func isHarnessAlias(segment string) bool {
	_, ok := harnessSurfaceAuthority[normalizeHarnessSurfaceAlias(strings.TrimPrefix(segment, "."))]
	return ok
}

func isDottedHarnessRoot(segment string) bool {
	if !strings.HasPrefix(segment, ".") {
		return false
	}
	alias := strings.TrimPrefix(segment, ".")
	alias = strings.TrimSuffix(alias, "-plugin")
	return isHarnessAlias(alias)
}

func normalizeHarnessSurfaceAlias(value string) string {
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

// harnessSurfaceAuthority is the pinned closed authority for bare top-level
// harness IDs and aliases. Structural dotted/plugin and harness/<member> roots
// do not depend on this table. A canonical harness-registry change must update
// this authority and its boundary tests in the same change. Values name the
// canonical entry and make alias drift reviewable. `.dear-agent` is
// intentionally absent: it is a product contract root.
var harnessSurfaceAuthority = map[string]string{
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

func contains(values []string, wanted string) bool {
	return slices.Contains(values, wanted)
}

type findingCollector struct {
	findings []Finding
	limit    int
	overflow bool
}

func (collector *findingCollector) add(finding Finding) {
	if len(collector.findings) < collector.limit {
		collector.findings = append(collector.findings, finding)
		return
	}
	collector.overflow = true
}

func (collector *findingCollector) sorted() []Finding {
	if collector.findings == nil {
		collector.findings = []Finding{}
	}
	if collector.overflow && len(collector.findings) != 0 {
		collector.findings[len(collector.findings)-1] = Finding{
			Code:    "finding-limit",
			Message: fmt.Sprintf("guard findings exceeded the %d-entry safety limit", collector.limit),
		}
	}
	sort.Slice(collector.findings, func(i, j int) bool {
		left, right := collector.findings[i], collector.findings[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.Message < right.Message
	})
	return collector.findings
}

type guardFailure struct {
	code    string
	path    string
	message string
}

func fail(code, filePath, message string) *guardFailure {
	return &guardFailure{code: code, path: filePath, message: message}
}

func (failure *guardFailure) finding() Finding {
	return Finding{Code: failure.code, Path: failure.path, Message: failure.message}
}
