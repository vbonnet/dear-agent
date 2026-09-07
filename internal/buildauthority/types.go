package buildauthority

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Request names the seven caller-supplied authorities. Every policy decision,
// including platform, roles, commands, limits, and output names, is package
// owned.
type Request struct {
	GoExecutable  string
	GitExecutable string
	GOROOT        string
	GOMODCACHE    string
	StateRoot     string
	Repository    string
	Revision      string
}

// Digest is one SHA-256 claim.
type Digest [32]byte

// RoleReceipt identifies one member of the fixed staged pair.
type RoleReceipt struct {
	OutputName     string
	MainPackage    string
	ArtifactSHA256 Digest
}

// Receipt is the immutable, path-free public claim returned by a staged pair.
// It intentionally contains no artifact or workspace handle.
type Receipt struct {
	AuthorityVersion              string
	Revision                      string
	Platform                      string
	GoExecutableSHA256            Digest
	GitExecutableSHA256           Digest
	GOROOTManifestSHA256          Digest
	RepositoryManifestSHA256      Digest
	SourceObjectContentSHA256     Digest
	SourceTreeManifestSHA256      Digest
	GOMODCACHEManifestSHA256      Digest
	SelectedModulesManifestSHA256 Digest
	ProtectedStamp                string
	Roles                         [2]RoleReceipt
}

// StagedPair is a sealed capability. Copying the interface aliases one private
// owner rather than copying synchronization or descriptors.
type StagedPair interface {
	Receipt() Receipt
	Close() error
	privateSeal()
}

// Phase is a fixed public-safe failure phase.
type Phase string

// PhaseRequest through PhaseClose are the only report phases.
const (
	PhaseRequest            Phase = "request"
	PhaseAuthority          Phase = "authority"
	PhaseWorkspace          Phase = "workspace"
	PhaseSource             Phase = "source"
	PhaseDependencies       Phase = "dependencies"
	PhaseBuildAGM           Phase = "build-agm"
	PhaseVerifyAGM          Phase = "verify-agm"
	PhaseBuildDiskWatchdog  Phase = "build-disk-watchdog"
	PhaseVerifyDiskWatchdog Phase = "verify-disk-watchdog"
	PhaseFinal              Phase = "final"
	PhaseClose              Phase = "close"
)

// Operation is a fixed public-safe failure operation.
type Operation string

// OperationValidate through OperationCloseRoot are the only report operations.
const (
	OperationValidate     Operation = "validate"
	OperationOpen         Operation = "open"
	OperationProbe        Operation = "probe"
	OperationWalk         Operation = "walk"
	OperationParse        Operation = "parse"
	OperationHash         Operation = "hash"
	OperationCopy         Operation = "copy"
	OperationExecute      Operation = "execute"
	OperationCompare      Operation = "compare"
	OperationQuiesce      Operation = "quiesce"
	OperationCloseNonRoot Operation = "close-nonroot"
	OperationRemove       Operation = "remove"
	OperationCloseRoot    Operation = "close-root"
)

// CauseCode is a fixed public-safe failure classification.
type CauseCode string

// CauseInvalidRequest through CauseInternalInvariant are the only report causes.
const (
	CauseInvalidRequest    CauseCode = "invalid-request"
	CauseNotFound          CauseCode = "not-found"
	CausePermission        CauseCode = "permission"
	CauseMalformed         CauseCode = "malformed"
	CauseUnsupported       CauseCode = "unsupported"
	CauseUnstable          CauseCode = "unstable"
	CauseLimit             CauseCode = "limit"
	CauseCanceled          CauseCode = "canceled"
	CauseDeadline          CauseCode = "deadline"
	CauseChildStart        CauseCode = "child-start"
	CauseChildExit         CauseCode = "child-exit"
	CauseChildDrain        CauseCode = "child-drain"
	CauseChildSurvivor     CauseCode = "child-survivor"
	CauseIdentity          CauseCode = "identity"
	CauseDescriptorClose   CauseCode = "descriptor-close"
	CauseCleanup           CauseCode = "cleanup"
	CauseInternalInvariant CauseCode = "internal-invariant"
)

var causeOrder = [...]CauseCode{
	CauseInvalidRequest,
	CauseNotFound,
	CausePermission,
	CauseMalformed,
	CauseUnsupported,
	CauseUnstable,
	CauseLimit,
	CauseCanceled,
	CauseDeadline,
	CauseChildStart,
	CauseChildExit,
	CauseChildDrain,
	CauseChildSurvivor,
	CauseIdentity,
	CauseDescriptorClose,
	CauseCleanup,
	CauseInternalInvariant,
}

// ChildDiagnostic contains only bounded metadata about child diagnostics. It
// never contains the captured bytes.
type ChildDiagnostic struct {
	ExitStatusObserved bool
	ExitStatus         int
	DiagnosticBytes    uint64
	Truncated          bool
	DiagnosticSHA256   Digest
}

// FileIdentity is the last descriptor-bound root identity made available for
// explicit recovery. A path alone never authorizes deletion.
type FileIdentity struct {
	Device     uint64
	Inode      uint64
	UID        uint32
	Mode       uint32
	Filesystem [2]int32
}

// RecoveryInfo is present only when an allocated task child remains. TaskPath
// is an untrusted locator for later liveness handling, not deletion authority.
type RecoveryInfo struct {
	TaskPath  string
	StateRoot FileIdentity
	TaskRoot  FileIdentity
}

// FailureRecord is one sanitized failure category.
type FailureRecord struct {
	Phase     Phase
	Operation Operation
	Causes    []CauseCode
	Child     *ChildDiagnostic
}

// FailureReport retains at most one record for each deterministic category.
// Cleanup represents either uncertain quiescence or removal, which cannot run
// in the same transaction. Recovery is deliberately excluded from Error text.
type FailureReport struct {
	Primary         *FailureRecord
	DescriptorClose *FailureRecord
	Cleanup         *FailureRecord
	Recovery        *RecoveryInfo
}

// Refusal is the typed public failure surface returned by Stage and Close.
// Report always returns a deep copy and Refusal has no raw-cause unwrap chain.
type Refusal interface {
	error
	Report() FailureReport
}

type failureError struct {
	report FailureReport
	text   string
}

func (e *failureError) Error() string { return e.text }

func (e *failureError) Report() FailureReport { return cloneFailureReport(e.report) }

func newRefusal(report FailureReport) *failureError {
	normalized := cloneFailureReport(report)
	normalized.Primary = normalizeFailureRecord(normalized.Primary)
	normalized.DescriptorClose = normalizeFailureRecord(normalized.DescriptorClose)
	normalized.Cleanup = normalizeFailureRecord(normalized.Cleanup)
	if normalized.Primary == nil && normalized.DescriptorClose == nil && normalized.Cleanup == nil {
		normalized.Primary = &FailureRecord{
			Phase:     PhaseRequest,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
	}
	return &failureError{report: normalized, text: renderFailure(normalized)}
}

func normalizeFailureRecord(record *FailureRecord) *FailureRecord {
	if record == nil {
		return nil
	}
	if !validPhase(record.Phase) {
		record.Phase = PhaseRequest
		record.Causes = append(record.Causes, CauseInternalInvariant)
	}
	if !validOperation(record.Operation) {
		record.Operation = OperationValidate
		record.Causes = append(record.Causes, CauseInternalInvariant)
	}
	ranks := make(map[CauseCode]int, len(causeOrder))
	for i, cause := range causeOrder {
		ranks[cause] = i
	}
	seen := make(map[CauseCode]struct{}, len(record.Causes))
	causes := make([]CauseCode, 0, len(record.Causes)+1)
	for _, cause := range record.Causes {
		if _, ok := ranks[cause]; !ok {
			cause = CauseInternalInvariant
		}
		if _, ok := seen[cause]; ok {
			continue
		}
		seen[cause] = struct{}{}
		causes = append(causes, cause)
	}
	if len(causes) == 0 {
		causes = append(causes, CauseInternalInvariant)
	}
	sort.Slice(causes, func(i, j int) bool { return ranks[causes[i]] < ranks[causes[j]] })
	record.Causes = causes
	return record
}

func validPhase(phase Phase) bool {
	switch phase {
	case PhaseRequest, PhaseAuthority, PhaseWorkspace, PhaseSource,
		PhaseDependencies, PhaseBuildAGM, PhaseVerifyAGM,
		PhaseBuildDiskWatchdog, PhaseVerifyDiskWatchdog, PhaseFinal, PhaseClose:
		return true
	default:
		return false
	}
}

func validOperation(operation Operation) bool {
	switch operation {
	case OperationValidate, OperationOpen, OperationProbe, OperationWalk,
		OperationParse, OperationHash, OperationCopy, OperationExecute,
		OperationCompare, OperationQuiesce, OperationCloseNonRoot,
		OperationRemove, OperationCloseRoot:
		return true
	default:
		return false
	}
}

func cloneFailureReport(report FailureReport) FailureReport {
	cloned := report
	cloned.Primary = cloneFailureRecord(report.Primary)
	cloned.DescriptorClose = cloneFailureRecord(report.DescriptorClose)
	cloned.Cleanup = cloneFailureRecord(report.Cleanup)
	if report.Recovery != nil {
		recovery := *report.Recovery
		cloned.Recovery = &recovery
	}
	return cloned
}

func cloneFailureRecord(record *FailureRecord) *FailureRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	cloned.Causes = append([]CauseCode(nil), record.Causes...)
	if record.Child != nil {
		child := *record.Child
		cloned.Child = &child
	}
	return &cloned
}

func renderFailure(report FailureReport) string {
	var builder strings.Builder
	builder.WriteString("buildauthority:")
	slots := [...]struct {
		name   string
		record *FailureRecord
	}{
		{name: "primary", record: report.Primary},
		{name: "descriptor-close", record: report.DescriptorClose},
		{name: "cleanup", record: report.Cleanup},
	}
	written := false
	for _, slot := range slots {
		if slot.record == nil {
			continue
		}
		if written {
			builder.WriteString(";")
		}
		written = true
		fmt.Fprintf(&builder, " %s=%s/%s/", slot.name, slot.record.Phase, slot.record.Operation)
		for i, cause := range slot.record.Causes {
			if i > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(string(cause))
		}
		if child := slot.record.Child; child != nil {
			builder.WriteString(" child(exit=")
			if child.ExitStatusObserved {
				fmt.Fprintf(&builder, "%d", child.ExitStatus)
			} else {
				builder.WriteString("none")
			}
			fmt.Fprintf(
				&builder,
				",bytes=%d,truncated=%t,sha256=%s)",
				child.DiagnosticBytes,
				child.Truncated,
				hex.EncodeToString(child.DiagnosticSHA256[:]),
			)
		}
	}
	return builder.String()
}

type pairOwner struct {
	receipt Receipt
	close   func() error
	once    sync.Once
	err     error
}

type stagedPair struct{ owner *pairOwner }

var _ StagedPair = (*stagedPair)(nil)

func newStagedPair(receipt Receipt, closeFn func() error) StagedPair {
	return &stagedPair{owner: &pairOwner{receipt: receipt, close: closeFn}}
}

func (*stagedPair) privateSeal() {}

func (pair *stagedPair) Receipt() Receipt {
	if pair == nil || pair.owner == nil {
		return Receipt{}
	}
	return pair.owner.receipt
}

func (pair *stagedPair) Close() error {
	if pair == nil || pair.owner == nil {
		return newRefusal(FailureReport{Primary: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseInternalInvariant},
		}})
	}
	pair.owner.once.Do(func() {
		if pair.owner.close != nil {
			pair.owner.err = pair.owner.close()
		}
	})
	return pair.owner.err
}
