package buildauthority

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type sealedExecutableBracket struct {
	physical   sealedPhysicalRootBracket
	retained   *retainedExecutable
	leaf       *retainedLeaf
	leafValue  retainedLeaf
	digest     Digest
	parentPath string
	base       string
	profile    machOProfile
}

type sealedGOROOTRoleBinding struct {
	capture    *treeCapture
	root       *retainedDirectory
	rootValue  retainedDirectory
	digest     Digest
	policy     treePolicy
	relative   string
	binding    gorootExecutableClaim
	pathClaims []authorityPathClaim
}

func sealExecutableBracket(
	physicalRoot *physicalRootAuthority,
	retained *retainedExecutable,
	profile machOProfile,
) (sealedExecutableBracket, *authorityPrimitiveFailure) {
	physical, failure := sealPhysicalRootBracket(physicalRoot)
	if failure != nil {
		return sealedExecutableBracket{}, failure
	}
	if retained == nil || retained.leaf == nil || retained.leaf.descriptor == nil ||
		(profile != machOProfileGo && profile != machOProfileGit) {
		return sealedExecutableBracket{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	leaf := retained.leaf
	parentPath, base, err := splitExecutablePath(leaf.path)
	components, componentErr := absolutePathComponents(parentPath)
	if err != nil || componentErr != nil || len(leaf.pathClaims) != len(components)+1 ||
		len(leaf.pathClaims) == 0 || leaf.pathClaims[0] != physicalRoot.claim ||
		!validSealedAuthorityPathClaims(leaf.pathClaims) {
		return sealedExecutableBracket{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if err := validateExecutableSnapshot(leaf.snapshot, leaf.mount, leaf.pathClaims); err != nil {
		return sealedExecutableBracket{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	leafValue := *leaf
	leafValue.pathClaims = append([]authorityPathClaim(nil), leaf.pathClaims...)
	return sealedExecutableBracket{
		physical:   physical,
		retained:   retained,
		leaf:       leaf,
		leafValue:  leafValue,
		digest:     retained.digest,
		parentPath: parentPath,
		base:       base,
		profile:    profile,
	}, nil
}

func sealGOROOTRoleBinding(
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
	relative string,
) (sealedGOROOTRoleBinding, *authorityPrimitiveFailure) {
	if _, failure := sealPhysicalRootBracket(physicalRoot); failure != nil {
		return sealedGOROOTRoleBinding{}, failure
	}
	if goroot == nil || goroot.root == nil || goroot.root.root == nil ||
		goroot.root.descriptor == nil || goroot.policy != gorootPolicy() ||
		goroot.digest == (Digest{}) {
		return sealedGOROOTRoleBinding{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	components, componentErr := absolutePathComponents(goroot.root.path)
	if componentErr != nil || len(goroot.root.pathClaims) != len(components)+1 ||
		len(goroot.root.pathClaims) == 0 ||
		goroot.root.pathClaims[0] != physicalRoot.claim ||
		!validSealedAuthorityPathClaims(goroot.root.pathClaims) ||
		!validSealedTreeRoot(*goroot.root, goroot.policy) {
		return sealedGOROOTRoleBinding{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	binding, err := gorootExecutableClaimFromCapture(goroot, relative)
	if err != nil {
		return sealedGOROOTRoleBinding{}, authorityPrimitiveFailureFromError(
			OperationCompare,
			err,
			CauseIdentity,
		)
	}
	rootValue := *goroot.root
	rootValue.pathClaims = append([]authorityPathClaim(nil), goroot.root.pathClaims...)
	return sealedGOROOTRoleBinding{
		capture:    goroot,
		root:       goroot.root,
		rootValue:  rootValue,
		digest:     goroot.digest,
		policy:     goroot.policy,
		relative:   relative,
		binding:    binding,
		pathClaims: append([]authorityPathClaim(nil), goroot.root.pathClaims...),
	}, nil
}

func (revalidator *preflightAuthorityRevalidator) revalidateGoAuthority(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
	authority *goAuthority,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	if ctx == nil || !revalidator.validForBrackets() || authority == nil ||
		authority.seal != validGoAuthority || authority.retained == nil {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
		return outcome
	}
	sealed, failure := sealExecutableBracket(
		physicalRoot,
		authority.retained,
		machOProfileGo,
	)
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	binding, failure := sealGOROOTRoleBinding(physicalRoot, goroot, goGOROOTRelativePath)
	if failure == nil {
		failure = compareExecutableRoleBinding(ctx, sealed, binding.binding)
	}
	if failure == nil && (sealed.base != goExecutableBase || sealed.digest != goExecutableDigest()) {
		failure = newAuthorityPrimitiveFailure(OperationCompare, CauseUnsupported)
	}
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	outcome = revalidator.revalidateExecutableBracket(ctx, physicalRoot, sealed)
	outcome.addPrimitive(compareGoAuthoritySeal(ctx, authority, sealed, binding))
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) revalidateCompilerAuthority(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
	authority *compilerAuthority,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	if ctx == nil || !revalidator.validForBrackets() || authority == nil ||
		authority.seal != validCompilerAuthority || authority.retained == nil {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
		return outcome
	}
	sealed, failure := sealExecutableBracket(
		physicalRoot,
		authority.retained,
		machOProfileGo,
	)
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	binding, failure := sealGOROOTRoleBinding(physicalRoot, goroot, compilerGOROOTRelativePath)
	if failure == nil {
		failure = compareExecutableRoleBinding(ctx, sealed, binding.binding)
	}
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	outcome = revalidator.revalidateExecutableBracket(ctx, physicalRoot, sealed)
	outcome.addPrimitive(compareCompilerAuthoritySeal(ctx, authority, sealed, binding))
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) revalidateGitAuthority(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	authority *gitAuthority,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	if ctx == nil || !revalidator.validForBrackets() || authority == nil ||
		authority.seal != validGitAuthority || authority.retained == nil {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
		return outcome
	}
	sealed, failure := sealExecutableBracket(
		physicalRoot,
		authority.retained,
		machOProfileGit,
	)
	if failure == nil && (sealed.base != gitExecutableBase || sealed.digest != gitExecutableDigest()) {
		failure = newAuthorityPrimitiveFailure(OperationCompare, CauseUnsupported)
	}
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	outcome = revalidator.revalidateExecutableBracket(ctx, physicalRoot, sealed)
	outcome.addPrimitive(compareGitAuthoritySeal(ctx, authority, sealed))
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) revalidateExecutableBracket(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	sealed sealedExecutableBracket,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	if failure := revalidator.observeExecutableDescriptor(
		ctx,
		sealed,
		sealed.leafValue.descriptor,
	); failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	parent, pathOutcome := revalidator.resolveClaimedAuthorityPath(
		ctx,
		physicalRoot,
		sealed.parentPath,
		sealed.leafValue.pathClaims,
	)
	outcome.absorb(pathOutcome)
	if parent != nil {
		revalidator.useFreshExecutableLeaf(ctx, parent, sealed, &outcome)
	}
	// The parent path is always rebound after a leaf use or leaf close failure.
	trailing, trailingOutcome := revalidator.resolveClaimedAuthorityPath(
		ctx,
		physicalRoot,
		sealed.parentPath,
		sealed.leafValue.pathClaims,
	)
	outcome.absorb(trailingOutcome)
	if trailing != nil {
		trailing.closeInto(&outcome)
	}
	outcome.addPrimitive(compareExecutableBracketSeal(ctx, sealed))
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) useFreshExecutableLeaf(
	ctx context.Context,
	parent *resolvedAuthorityPath,
	sealed sealedExecutableBracket,
	outcome *authorityUseOutcome,
) {
	acquisition, openFailure := revalidator.primitives.brackets.openTypedRelativeNoFollow(
		ctx,
		parent.descriptor,
		sealed.base,
		entryRegular,
		authorityExpectedPresentOpen,
	)
	owner, acquisitionFailure := ownAuthorityAcquisition(acquisition, openFailure)
	outcome.addPrimitive(acquisitionFailure)
	parent.closeInto(outcome)
	if acquisitionFailure == nil && owner != nil {
		outcome.addPrimitive(revalidator.observeExecutableDescriptor(ctx, sealed, owner.file))
	}
	if owner != nil {
		owner.closeInto(outcome)
	}
}

func (revalidator *preflightAuthorityRevalidator) observeExecutableDescriptor(
	ctx context.Context,
	sealed sealedExecutableBracket,
	descriptor *os.File,
) *authorityPrimitiveFailure {
	beforeComparison, failure := revalidator.probeDescriptorObservation(ctx, descriptor)
	if failure != nil {
		return failure
	}
	before := beforeComparison.value
	if failure := compareRetainedLeafObservation(ctx, &sealed.leafValue, before); failure != nil {
		return failure
	}
	if failure := validateDeferredAuthorityPolicy(ctx, beforeComparison.policyFailure); failure != nil {
		return failure
	}
	if failure := revalidator.primitives.validateFilesystem(ctx, before.mount); failure != nil {
		return failure
	}
	if err := validateExecutableSnapshot(
		before.snapshot,
		before.mount,
		sealed.leafValue.pathClaims,
	); err != nil {
		return authorityPrimitiveFailureFromError(OperationValidate, err, CauseUnsupported)
	}
	digest, failure := revalidator.primitives.brackets.hashDescriptor(
		ctx,
		descriptor,
		before.snapshot.size,
		maxExecutableBytes,
	)
	if failure != nil {
		return failure
	}
	if digest != sealed.digest {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if failure := revalidator.primitives.brackets.parseMachO(
		ctx,
		descriptor,
		before.snapshot.size,
		sealed.profile,
	); failure != nil {
		return failure
	}
	afterComparison, failure := revalidator.probeDescriptorObservation(ctx, descriptor)
	if failure != nil {
		return failure
	}
	after := afterComparison.value
	if failure := compareAuthorityObservations(ctx, before, after); failure != nil {
		return failure
	}
	if failure := validateDeferredAuthorityPolicy(ctx, afterComparison.policyFailure); failure != nil {
		return failure
	}
	return revalidator.primitives.validateFilesystem(ctx, after.mount)
}

func compareExecutableRoleBinding(
	ctx context.Context,
	sealed sealedExecutableBracket,
	binding gorootExecutableClaim,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	leaf := sealed.leafValue
	if binding.path == "" || len(binding.parentClaims) == 0 || binding.leaf.path == "" {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if leaf.path != binding.path {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if err := compareAuthorityPathClaims(binding.parentClaims, leaf.pathClaims); err != nil {
		return authorityPrimitiveFailureFromError(OperationCompare, err, CauseIdentity)
	}
	parentMount := binding.parentClaims[len(binding.parentClaims)-1].mount
	if !sameFilesystemObject(leaf.snapshot.identity, binding.leaf.file.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if leaf.snapshot != binding.leaf.file || leaf.mount != parentMount ||
		leaf.aclDigest != binding.leaf.aclDigest || sealed.digest != binding.leaf.content {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareExecutableBracketSeal(
	ctx context.Context,
	sealed sealedExecutableBracket,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	retained := sealed.retained
	leaf := sealed.leaf
	if retained == nil || retained.leaf != leaf || leaf == nil ||
		retained.digest != sealed.digest || leaf.path != sealed.leafValue.path ||
		leaf.descriptor != sealed.leafValue.descriptor ||
		leaf.snapshot != sealed.leafValue.snapshot || leaf.mount != sealed.leafValue.mount ||
		leaf.aclDigest != sealed.leafValue.aclDigest ||
		!equalAuthorityPathClaimSlices(leaf.pathClaims, sealed.leafValue.pathClaims) ||
		!samePhysicalRootBracketSeal(sealed.physical) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareGOROOTRoleBindingSeal(
	ctx context.Context,
	sealed sealedGOROOTRoleBinding,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if failure := compareGOROOTRoleBindingState(sealed); failure != nil {
		return failure
	}
	goroot := sealed.capture
	if !hasUniqueGOROOTRoleRows(goroot.entries, sealed.relative) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	current, err := gorootExecutableClaimFromCapture(goroot, sealed.relative)
	if err != nil {
		return authorityPrimitiveFailureFromError(OperationCompare, err, CauseIdentity)
	}
	return compareGOROOTExecutableClaims(ctx, sealed.binding, current)
}

func compareGOROOTRoleBindingState(
	sealed sealedGOROOTRoleBinding,
) *authorityPrimitiveFailure {
	goroot := sealed.capture
	if goroot == nil || goroot.root == nil || goroot.root != sealed.root ||
		goroot.root.root != sealed.rootValue.root ||
		goroot.root.descriptor != sealed.rootValue.descriptor ||
		goroot.digest != sealed.digest || goroot.policy != sealed.policy {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	root := goroot.root
	if root.path != sealed.rootValue.path ||
		!sameFilesystemObject(root.snapshot.identity, sealed.rootValue.snapshot.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if err := compareAuthorityPathClaims(sealed.pathClaims, root.pathClaims); err != nil {
		return authorityPrimitiveFailureFromError(OperationCompare, err, CauseIdentity)
	}
	if root.snapshot != sealed.rootValue.snapshot || root.mount != sealed.rootValue.mount ||
		root.aclDigest != sealed.rootValue.aclDigest {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func hasUniqueGOROOTRoleRows(entries []entrySnapshot, relative string) bool {
	components := strings.Split(relative, string(filepath.Separator))
	if len(components) < 2 {
		return false
	}
	for index := 1; index < len(components); index++ {
		if countCapturedEntries(entries, filepath.Join(components[:index]...)) != 1 {
			return false
		}
	}
	return countCapturedEntries(entries, relative) == 1
}

func countCapturedEntries(entries []entrySnapshot, path string) int {
	count := 0
	for _, entry := range entries {
		if entry.path == path {
			count++
		}
	}
	return count
}

func compareGOROOTExecutableClaims(
	ctx context.Context,
	expected gorootExecutableClaim,
	current gorootExecutableClaim,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if current.path != expected.path || current.leaf.path != expected.leaf.path ||
		current.leaf.kind != expected.leaf.kind {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if err := compareAuthorityPathClaims(expected.parentClaims, current.parentClaims); err != nil {
		return authorityPrimitiveFailureFromError(OperationCompare, err, CauseIdentity)
	}
	if !sameFilesystemObject(expected.leaf.file.identity, current.leaf.file.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if current.leaf != expected.leaf {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareGoAuthoritySeal(
	ctx context.Context,
	authority *goAuthority,
	sealed sealedExecutableBracket,
	binding sealedGOROOTRoleBinding,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if authority == nil || authority.seal != validGoAuthority || authority.retained != sealed.retained {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if failure := compareExecutableBracketSeal(ctx, sealed); failure != nil {
		return failure
	}
	return compareGOROOTRoleBindingSeal(ctx, binding)
}

func compareCompilerAuthoritySeal(
	ctx context.Context,
	authority *compilerAuthority,
	sealed sealedExecutableBracket,
	binding sealedGOROOTRoleBinding,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if authority == nil || authority.seal != validCompilerAuthority || authority.retained != sealed.retained {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if failure := compareExecutableBracketSeal(ctx, sealed); failure != nil {
		return failure
	}
	return compareGOROOTRoleBindingSeal(ctx, binding)
}

func compareGitAuthoritySeal(
	ctx context.Context,
	authority *gitAuthority,
	sealed sealedExecutableBracket,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if authority == nil || authority.seal != validGitAuthority || authority.retained != sealed.retained {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return compareExecutableBracketSeal(ctx, sealed)
}
