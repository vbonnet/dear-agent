package buildauthority

import (
	"context"
	"strings"
)

// sourceObjectPathInventory is the value-only name and path-local prerequisite
// proof that follows the selective administrative walk. It contains no bytes,
// digest, parsed chain membership, or handle and therefore cannot authorize
// object parsing, copying, or C1 sealing.
type sourceObjectPathInventory struct {
	rows               []sourceObjectPathRow
	contentFileCount   uint64
	auxiliaryFileCount uint64
}

type sourceObjectPathRow struct {
	path string
	role sourceObjectPathRole
	key  string
}

type sourceObjectPathRole uint8

const (
	sourceObjectDirectoryPath sourceObjectPathRole = iota + 1
	sourceObjectLooseContentPath
	sourceObjectPackContentPath
	sourceObjectPackIndexPath
	sourceObjectPackAuxiliaryPath
	sourceObjectCommitGraphPath
	sourceObjectCommitGraphChainPath
	sourceObjectSplitCommitGraphPath
	sourceObjectPackListPath
	sourceObjectMultiPackIndexPath
	sourceObjectMultiPackIndexAuxiliaryPath
	sourceObjectMultiPackIndexChainPath
	sourceObjectMultiPackIndexLayerPath
	sourceObjectMultiPackIndexLayerAuxiliaryPath
)

func (role sourceObjectPathRole) content() bool {
	switch role {
	case sourceObjectLooseContentPath, sourceObjectPackContentPath,
		sourceObjectPackIndexPath:
		return true
	case sourceObjectDirectoryPath, sourceObjectPackAuxiliaryPath,
		sourceObjectCommitGraphPath, sourceObjectCommitGraphChainPath,
		sourceObjectSplitCommitGraphPath, sourceObjectPackListPath,
		sourceObjectMultiPackIndexPath, sourceObjectMultiPackIndexAuxiliaryPath,
		sourceObjectMultiPackIndexChainPath, sourceObjectMultiPackIndexLayerPath,
		sourceObjectMultiPackIndexLayerAuxiliaryPath:
		return false
	}
	return false
}

func (role sourceObjectPathRole) auxiliary() bool {
	switch role {
	case sourceObjectPackAuxiliaryPath, sourceObjectCommitGraphPath,
		sourceObjectCommitGraphChainPath, sourceObjectSplitCommitGraphPath,
		sourceObjectPackListPath, sourceObjectMultiPackIndexPath,
		sourceObjectMultiPackIndexAuxiliaryPath, sourceObjectMultiPackIndexChainPath,
		sourceObjectMultiPackIndexLayerPath, sourceObjectMultiPackIndexLayerAuxiliaryPath:
		return true
	case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
		sourceObjectPackContentPath, sourceObjectPackIndexPath:
		return false
	}
	return false
}

func (inventory *sourceObjectPathInventory) valid(
	format repositoryObjectFormat,
	administration *sourceAdministrativeInventory,
) bool {
	if inventory == nil || administration == nil || format.hexWidth() == 0 ||
		len(inventory.rows) == 0 ||
		len(inventory.rows) > maxRepositoryEntries+1 {
		return false
	}
	contentFileCount, auxiliaryFileCount, rowsMatch :=
		sourceObjectPathRowsMatchAdministration(inventory.rows, administration, format)
	return rowsMatch &&
		contentFileCount == inventory.contentFileCount &&
		auxiliaryFileCount == inventory.auxiliaryFileCount &&
		sourceObjectPathTopologyValid(inventory.rows) &&
		sourceObjectPathPrerequisitesAdmitted(inventory.rows)
}

func sourceObjectPathRowsMatchAdministration(
	rows []sourceObjectPathRow,
	administration *sourceAdministrativeInventory,
	format repositoryObjectFormat,
) (uint64, uint64, bool) {
	rowIndex := 0
	var contentFileCount uint64
	var auxiliaryFileCount uint64
	for index := range administration.rows {
		administrativeRow := administration.rows[index]
		if !sourceObjectAdministrativePath(administrativeRow.path) {
			continue
		}
		expected, ok := classifySourceObjectPathRow(administrativeRow, format)
		if !ok || rowIndex >= len(rows) || rows[rowIndex] != expected {
			return 0, 0, false
		}
		if expected.role.content() {
			contentFileCount++
		}
		if expected.role.auxiliary() {
			auxiliaryFileCount++
		}
		rowIndex++
	}
	return contentFileCount, auxiliaryFileCount, rowIndex == len(rows)
}

func (builder *sourceConstructionBuilder) retainSourceObjectPathInventory() bool {
	if failure := builder.validateSourceObjectPathInventoryRequest(); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	inventory, failure := deriveSourceObjectPathInventory(
		builder.ctx,
		builder.owner.config.claim.objectFormat,
		builder.owner.git.administration,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = validateSourceObjectPathInventoryOwner(
		builder.ctx,
		builder.owner,
		inventory,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	builder.owner.objects.paths = inventory
	return true
}

func (builder *sourceConstructionBuilder) validateSourceObjectPathInventoryRequest() *sourcePrimitiveFailure {
	if builder == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if builder.primitives == nil || builder.owner == nil ||
		!builder.owner.validAdministrativeRetention() {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func deriveSourceObjectPathInventory(
	ctx context.Context,
	format repositoryObjectFormat,
	administration *sourceAdministrativeInventory,
) (*sourceObjectPathInventory, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return nil, failure
	}
	if administration == nil || format.hexWidth() == 0 {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	rows := make([]sourceObjectPathRow, 0, len(administration.rows))
	var contentFileCount uint64
	var auxiliaryFileCount uint64
	for index := range administration.rows {
		if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
			return nil, failure
		}
		administrativeRow := administration.rows[index]
		if !sourceObjectAdministrativePath(administrativeRow.path) {
			continue
		}
		row, ok := classifySourceObjectPathRow(administrativeRow, format)
		if !ok {
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		rows = append(rows, row)
		if row.role.content() {
			contentFileCount++
		}
		if row.role.auxiliary() {
			auxiliaryFileCount++
		}
	}
	topologyValid := sourceObjectPathTopologyValid(rows)
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return nil, failure
	}
	if !topologyValid {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	prerequisitesAdmitted := sourceObjectPathPrerequisitesAdmitted(rows)
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return nil, failure
	}
	if !prerequisitesAdmitted {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	return &sourceObjectPathInventory{
		rows:               rows,
		contentFileCount:   contentFileCount,
		auxiliaryFileCount: auxiliaryFileCount,
	}, nil
}

func validateSourceObjectPathInventoryOwner(
	ctx context.Context,
	owner *sourceConstructionOwner,
	inventory *sourceObjectPathInventory,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if owner == nil || inventory == nil || !owner.validAdministrativeRetention() ||
		owner.objects == nil || owner.objects.paths != nil ||
		!inventory.valid(owner.config.claim.objectFormat, owner.git.administration) {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func sourceObjectAdministrativePath(path string) bool {
	return path == ".git/objects" || strings.HasPrefix(path, ".git/objects/")
}

func classifySourceObjectPathRow(
	row sourceAdministrativeRow,
	format repositoryObjectFormat,
) (sourceObjectPathRow, bool) {
	objectPath, ok := sourceObjectPathComponents(row, format)
	if !ok {
		return sourceObjectPathRow{}, false
	}
	if len(objectPath) == 0 {
		if row.kind != sourceObservedDirectory {
			return sourceObjectPathRow{}, false
		}
		return sourceObjectPathRow{path: row.path, role: sourceObjectDirectoryPath}, true
	}
	if row.kind == sourceObservedDirectory {
		if !isRecognizedSourceObjectDirectory(objectPath) {
			return sourceObjectPathRow{}, false
		}
		return sourceObjectPathRow{path: row.path, role: sourceObjectDirectoryPath}, true
	}
	if row.kind != sourceObservedRegular {
		return sourceObjectPathRow{}, false
	}
	return classifySourceObjectRegularPath(row.path, objectPath, format)
}

func sourceObjectPathComponents(
	row sourceAdministrativeRow,
	format repositoryObjectFormat,
) ([]string, bool) {
	components, ok := splitNormalizedSourcePath(row.path)
	if format.hexWidth() == 0 || !ok || len(components) < 2 || components[0] != ".git" ||
		components[1] != "objects" || row.class != sourceAuthorityAndManifest {
		return nil, false
	}
	return components[2:], true
}

func classifySourceObjectRegularPath(
	path string,
	objectPath []string,
	format repositoryObjectFormat,
) (sourceObjectPathRow, bool) {
	if !isRecognizedSourceObjectFile(objectPath, format) {
		return sourceObjectPathRow{}, false
	}
	if len(objectPath) == 2 && isLowerHexOfWidth(objectPath[0], 2) &&
		isLowerHexOfWidth(objectPath[1], format.hexWidth()-2) {
		return sourceObjectPathRow{
			path: path,
			role: sourceObjectLooseContentPath,
			key:  objectPath[0] + objectPath[1],
		}, true
	}
	if objectPath[0] == "info" {
		return classifySourceObjectInfoPath(path, objectPath, format)
	}
	if objectPath[0] == "pack" {
		return classifySourceObjectPackPath(path, objectPath, format)
	}
	return sourceObjectPathRow{}, false
}

func classifySourceObjectInfoPath(
	path string,
	components []string,
	format repositoryObjectFormat,
) (sourceObjectPathRow, bool) {
	if len(components) == 2 {
		switch components[1] {
		case "commit-graph":
			return sourceObjectPathRow{path: path, role: sourceObjectCommitGraphPath}, true
		case "packs":
			return sourceObjectPathRow{path: path, role: sourceObjectPackListPath}, true
		default:
			return sourceObjectPathRow{}, false
		}
	}
	if len(components) != 3 || components[1] != "commit-graphs" {
		return sourceObjectPathRow{}, false
	}
	if components[2] == "commit-graph-chain" {
		return sourceObjectPathRow{path: path, role: sourceObjectCommitGraphChainPath}, true
	}
	key, ok := sourceObjectHashFileKey(components[2], "graph-", ".graph", format)
	if !ok {
		return sourceObjectPathRow{}, false
	}
	return sourceObjectPathRow{
		path: path,
		role: sourceObjectSplitCommitGraphPath,
		key:  key,
	}, true
}

func classifySourceObjectPackPath(
	path string,
	components []string,
	format repositoryObjectFormat,
) (sourceObjectPathRow, bool) {
	if len(components) == 3 && components[1] == "multi-pack-index.d" {
		return classifySourceObjectMultiPackIndexLayerPath(
			path,
			components[2],
			format,
		)
	}
	if len(components) != 2 {
		return sourceObjectPathRow{}, false
	}
	name := components[1]
	if name == "multi-pack-index" {
		return sourceObjectPathRow{path: path, role: sourceObjectMultiPackIndexPath}, true
	}
	if key, ok := sourceObjectHashFileKey(name, "pack-", ".pack", format); ok {
		return sourceObjectPathRow{path: path, role: sourceObjectPackContentPath, key: key}, true
	}
	if key, ok := sourceObjectHashFileKey(name, "pack-", ".idx", format); ok {
		return sourceObjectPathRow{path: path, role: sourceObjectPackIndexPath, key: key}, true
	}
	for _, suffix := range [...]string{".rev", ".bitmap", ".keep", ".mtimes"} {
		if key, ok := sourceObjectHashFileKey(name, "pack-", suffix, format); ok {
			return sourceObjectPathRow{
				path: path,
				role: sourceObjectPackAuxiliaryPath,
				key:  key,
			}, true
		}
	}
	for _, suffix := range [...]string{".rev", ".bitmap"} {
		if key, ok := sourceObjectHashFileKey(
			name,
			"multi-pack-index-",
			suffix,
			format,
		); ok {
			return sourceObjectPathRow{
				path: path,
				role: sourceObjectMultiPackIndexAuxiliaryPath,
				key:  key,
			}, true
		}
	}
	return sourceObjectPathRow{}, false
}

func classifySourceObjectMultiPackIndexLayerPath(
	path string,
	name string,
	format repositoryObjectFormat,
) (sourceObjectPathRow, bool) {
	if name == "multi-pack-index-chain" {
		return sourceObjectPathRow{path: path, role: sourceObjectMultiPackIndexChainPath}, true
	}
	if key, ok := sourceObjectHashFileKey(
		name,
		"multi-pack-index-",
		".midx",
		format,
	); ok {
		return sourceObjectPathRow{
			path: path,
			role: sourceObjectMultiPackIndexLayerPath,
			key:  key,
		}, true
	}
	for _, suffix := range [...]string{".rev", ".bitmap"} {
		if key, ok := sourceObjectHashFileKey(
			name,
			"multi-pack-index-",
			suffix,
			format,
		); ok {
			return sourceObjectPathRow{
				path: path,
				role: sourceObjectMultiPackIndexLayerAuxiliaryPath,
				key:  key,
			}, true
		}
	}
	return sourceObjectPathRow{}, false
}

func sourceObjectHashFileKey(
	name string,
	prefix string,
	suffix string,
	format repositoryObjectFormat,
) (string, bool) {
	if !matchesSourceObjectHashFile(name, prefix, suffix, format) {
		return "", false
	}
	return name[len(prefix) : len(name)-len(suffix)], true
}

func sourceObjectPathTopologyValid(rows []sourceObjectPathRow) bool {
	if len(rows) == 0 || rows[0] != (sourceObjectPathRow{
		path: ".git/objects",
		role: sourceObjectDirectoryPath,
	}) {
		return false
	}
	directories := make(map[string]bool)
	for index := range rows {
		row := rows[index]
		if index > 0 && rows[index-1].path >= row.path {
			return false
		}
		if row.role == sourceObjectDirectoryPath {
			if row.key != "" {
				return false
			}
			directories[row.path] = true
		}
	}
	for index := 1; index < len(rows); index++ {
		parentEnd := strings.LastIndexByte(rows[index].path, '/')
		if parentEnd < len(".git/objects") || !directories[rows[index].path[:parentEnd]] {
			return false
		}
	}
	return true
}

// sourceObjectPathPrerequisitesAdmitted proves only relationships decidable
// from admitted pathnames. Chain membership, checksum/name binding, pack-list
// membership, and every auxiliary byte grammar remain deliberately unproved.
func sourceObjectPathPrerequisitesAdmitted(rows []sourceObjectPathRow) bool {
	index := indexSourceObjectPathPrerequisites(rows)
	if index.commitGraph && index.commitGraphChain {
		return false
	}
	for rowIndex := range rows {
		if !sourceObjectPathPrerequisiteAdmitted(rows[rowIndex], index) {
			return false
		}
	}
	return true
}

type sourceObjectPathPrerequisiteIndex struct {
	packParts            map[string]uint8
	multiPackIndexLayers map[string]bool
	commitGraph          bool
	commitGraphChain     bool
	multiPackIndex       bool
	multiPackIndexChain  bool
}

func indexSourceObjectPathPrerequisites(rows []sourceObjectPathRow) sourceObjectPathPrerequisiteIndex {
	index := sourceObjectPathPrerequisiteIndex{
		packParts:            make(map[string]uint8),
		multiPackIndexLayers: make(map[string]bool),
	}
	for rowIndex := range rows {
		row := rows[rowIndex]
		switch row.role {
		case sourceObjectPackContentPath:
			index.packParts[row.key] |= 1
		case sourceObjectPackIndexPath:
			index.packParts[row.key] |= 2
		case sourceObjectCommitGraphPath:
			index.commitGraph = true
		case sourceObjectCommitGraphChainPath:
			index.commitGraphChain = true
		case sourceObjectMultiPackIndexPath:
			index.multiPackIndex = true
		case sourceObjectMultiPackIndexChainPath:
			index.multiPackIndexChain = true
		case sourceObjectMultiPackIndexLayerPath:
			index.multiPackIndexLayers[row.key] = true
		case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
			sourceObjectPackAuxiliaryPath, sourceObjectSplitCommitGraphPath,
			sourceObjectPackListPath, sourceObjectMultiPackIndexAuxiliaryPath,
			sourceObjectMultiPackIndexLayerAuxiliaryPath:
		}
	}
	return index
}

func sourceObjectPathPrerequisiteAdmitted(
	row sourceObjectPathRow,
	index sourceObjectPathPrerequisiteIndex,
) bool {
	switch row.role {
	case sourceObjectPackContentPath, sourceObjectPackIndexPath,
		sourceObjectPackAuxiliaryPath:
		return index.packParts[row.key] == 3
	case sourceObjectSplitCommitGraphPath:
		return index.commitGraphChain
	case sourceObjectMultiPackIndexAuxiliaryPath:
		return index.multiPackIndex
	case sourceObjectMultiPackIndexLayerPath:
		return index.multiPackIndexChain
	case sourceObjectMultiPackIndexLayerAuxiliaryPath:
		return index.multiPackIndexChain && index.multiPackIndexLayers[row.key]
	case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
		sourceObjectCommitGraphPath, sourceObjectCommitGraphChainPath,
		sourceObjectPackListPath, sourceObjectMultiPackIndexPath,
		sourceObjectMultiPackIndexChainPath:
		return true
	}
	return false
}
