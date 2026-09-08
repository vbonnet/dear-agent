package buildauthority

import (
	"bytes"
	"encoding/binary"
	"io"
	pathpkg "path"
	"sort"
	"strings"
)

// machOProfile selects the two closed header/container policies. Go itself,
// active Go tools, and produced outputs share the thin profile; Git has the
// distinct header flags and is the only authority that may be FAT32.
type machOProfile uint8

const (
	machOProfileGo machOProfile = iota + 1
	machOProfileGit
)

const (
	machOMagic64 = uint32(0xfeedfacf)
	fatMagic32   = uint32(0xcafebabe)
	fatCigam32   = uint32(0xbebafeca)
	fatMagic64   = uint32(0xcafebabf)
	fatCigam64   = uint32(0xbfbafeca)

	cpuTypeARM64       = uint32(0x0100000c)
	cpuSubtypeARM64All = uint32(0)
	machOFileExecute   = uint32(0x2)

	machOFlagNoUndefs = uint32(0x1)
	machOFlagDyldLink = uint32(0x4)
	machOFlagTwoLevel = uint32(0x80)
	machOFlagPIE      = uint32(0x200000)

	machOLCSegment64         = uint32(0x19)
	machOLCSymtab            = uint32(0x2)
	machOLCDysymtab          = uint32(0xb)
	machOLCLoadDylib         = uint32(0xc)
	machOLCLoadWeakDylib     = uint32(0x80000018)
	machOLCReexportDylib     = uint32(0x8000001f)
	machOLCLazyLoadDylib     = uint32(0x20)
	machOLCLoadUpwardDylib   = uint32(0x80000023)
	machOLCLoadDylinker      = uint32(0xe)
	machOLCUUID              = uint32(0x1b)
	machOLCCodeSignature     = uint32(0x1d)
	machOLCDyldInfoOnly      = uint32(0x80000022)
	machOLCMain              = uint32(0x80000028)
	machOLCFunctionStarts    = uint32(0x26)
	machOLCDataInCode        = uint32(0x29)
	machOLCSourceVersion     = uint32(0x2a)
	machOLCBuildVersion      = uint32(0x32)
	machOLCDyldExportsTrie   = uint32(0x80000033)
	machOLCDyldChainedFixups = uint32(0x80000034)

	machOSectionTypeMask         = uint32(0xff)
	machOSectionZeroFill         = uint32(0x1)
	machOSectionGBZeroFill       = uint32(0x0c)
	machOSectionThreadZeroFill   = uint32(0x12)
	machOVMProtectionExecute     = uint32(0x4)
	machOPlatformMacOS           = uint32(1)
	machOMaxCommands             = uint32(65_536)
	machOMaxCommandBytes         = uint32(16 << 20)
	machOMaxFatRows              = uint32(32)
	machOMaxFatAlignmentExponent = uint32(30)
	machOMaxBuildTools           = uint32(64)
)

type machOValidationError struct {
	detail string
}

func (err *machOValidationError) Error() string {
	return "malformed Mach-O: " + err.detail
}

func invalidMachO(detail string) error {
	return &machOValidationError{detail: detail}
}

// validateGoMachO validates the common thin policy used by the Go executable,
// active Go tools, and produced outputs.
func validateGoMachO(reader io.ReaderAt, size int64) error {
	return validateMachO(reader, size, machOProfileGo)
}

// validateGitMachO validates the Git-specific thin/FAT32 and header policy.
func validateGitMachO(reader io.ReaderAt, size int64) error {
	return validateMachO(reader, size, machOProfileGit)
}

// validateMachO is deliberately panic-contained because ReaderAt and hostile
// offsets meet at this boundary. All arithmetic and allocation is bounded
// before conversion to int or int64.
func validateMachO(reader io.ReaderAt, size int64, profile machOProfile) (err error) {
	defer func() {
		if recover() != nil {
			err = invalidMachO("parser panic")
		}
	}()

	if reader == nil {
		return invalidMachO("nil reader")
	}
	if size < 32 || size > int64(maxExecutableBytes) {
		return invalidMachO("container size is outside policy")
	}
	if profile != machOProfileGo && profile != machOProfileGit {
		return invalidMachO("unknown parser profile")
	}

	container := checkedMachOReader{reader: reader, size: uint64(size)}
	magicBytes, readErr := container.read(0, 4)
	if readErr != nil {
		return readErr
	}

	if binary.LittleEndian.Uint32(magicBytes) == machOMagic64 {
		return validateMachOSlice(container, profile, nil)
	}

	magic := binary.BigEndian.Uint32(magicBytes)
	switch magic {
	case fatMagic32:
		if profile != machOProfileGit {
			return invalidMachO("FAT container is not admitted for this profile")
		}
		return validateFatMachO(container)
	case fatCigam32, fatMagic64, fatCigam64:
		return invalidMachO("unsupported FAT encoding")
	default:
		return invalidMachO("unsupported magic")
	}
}

type checkedMachOReader struct {
	reader io.ReaderAt
	base   uint64
	size   uint64
}

func (reader checkedMachOReader) slice(offset, size uint64) (checkedMachOReader, error) {
	if !boundedExtent(offset, size, reader.size) {
		return checkedMachOReader{}, invalidMachO("slice extent is outside container")
	}
	base, ok := checkedAdd64(reader.base, offset)
	if !ok {
		return checkedMachOReader{}, invalidMachO("slice base overflows")
	}
	return checkedMachOReader{reader: reader.reader, base: base, size: size}, nil
}

func (reader checkedMachOReader) read(offset, size uint64) ([]byte, error) {
	if size > maxExecutableBytes {
		return nil, invalidMachO("read size overflows int")
	}
	buffer := make([]byte, int(size))
	if err := reader.readInto(offset, buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}

func (reader checkedMachOReader) readInto(offset uint64, buffer []byte) error {
	size := uint64(len(buffer))
	if !boundedExtent(offset, size, reader.size) {
		return invalidMachO("read extent is outside slice")
	}
	absolute, ok := checkedAdd64(reader.base, offset)
	if !ok || absolute > uint64(^uint64(0)>>1) {
		return invalidMachO("read offset overflows int64")
	}
	n, err := reader.reader.ReadAt(buffer, int64(absolute))
	if n != len(buffer) || err != nil {
		return invalidMachO("short or failed ReaderAt read")
	}
	return nil
}

func checkedAdd64(left, right uint64) (uint64, bool) {
	result := left + right
	return result, result >= left
}

func checkedMul64(left, right uint64) (uint64, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	if left > ^uint64(0)/right {
		return 0, false
	}
	return left * right, true
}

func boundedExtent(offset, size, limit uint64) bool {
	end, ok := checkedAdd64(offset, size)
	return ok && end <= limit
}

func canonicalExtent(offset, size, limit uint64) bool {
	if size == 0 {
		return offset == 0
	}
	return boundedExtent(offset, size, limit)
}

type fatMachOArch struct {
	cpuType    uint32
	cpuSubtype uint32
	offset     uint64
	size       uint64
	align      uint32
}

//nolint:gocyclo // FAT admission is one ordered, closed structural proof.
func validateFatMachO(container checkedMachOReader) error {
	header, err := container.read(0, 8)
	if err != nil {
		return err
	}
	rowCount := binary.BigEndian.Uint32(header[4:8])
	if rowCount == 0 || rowCount > machOMaxFatRows {
		return invalidMachO("FAT row count is outside policy")
	}
	rowBytes, ok := checkedMul64(uint64(rowCount), 20)
	if !ok {
		return invalidMachO("FAT table size overflows")
	}
	tableEnd, ok := checkedAdd64(8, rowBytes)
	if !ok || tableEnd > container.size {
		return invalidMachO("FAT table is truncated")
	}
	table, err := container.read(8, rowBytes)
	if err != nil {
		return err
	}

	architectures := make([]fatMachOArch, 0, rowCount)
	seen := make(map[[2]uint32]struct{}, rowCount)
	for index := range rowCount {
		row := table[index*20 : index*20+20]
		architecture := fatMachOArch{
			cpuType:    binary.BigEndian.Uint32(row[0:4]),
			cpuSubtype: binary.BigEndian.Uint32(row[4:8]),
			offset:     uint64(binary.BigEndian.Uint32(row[8:12])),
			size:       uint64(binary.BigEndian.Uint32(row[12:16])),
			align:      binary.BigEndian.Uint32(row[16:20]),
		}
		key := [2]uint32{architecture.cpuType, architecture.cpuSubtype}
		if _, duplicate := seen[key]; duplicate {
			return invalidMachO("duplicate FAT architecture row")
		}
		seen[key] = struct{}{}
		if architecture.size == 0 || architecture.offset < tableEnd {
			return invalidMachO("invalid FAT slice placement")
		}
		if architecture.align > machOMaxFatAlignmentExponent {
			return invalidMachO("FAT alignment exponent is outside policy")
		}
		alignment := uint64(1) << architecture.align
		if architecture.offset&(alignment-1) != 0 {
			return invalidMachO("misaligned FAT slice")
		}
		if !boundedExtent(architecture.offset, architecture.size, container.size) {
			return invalidMachO("FAT slice is outside container")
		}
		architectures = append(architectures, architecture)
	}

	byOffset := append([]fatMachOArch(nil), architectures...)
	sort.Slice(byOffset, func(left, right int) bool {
		if byOffset[left].offset == byOffset[right].offset {
			return byOffset[left].size < byOffset[right].size
		}
		return byOffset[left].offset < byOffset[right].offset
	})
	for index := 1; index < len(byOffset); index++ {
		previousEnd, _ := checkedAdd64(byOffset[index-1].offset, byOffset[index-1].size)
		if byOffset[index].offset < previousEnd {
			return invalidMachO("overlapping FAT slices")
		}
	}

	var selected checkedMachOReader
	arm64Rows := 0
	for _, architecture := range architectures {
		slice, sliceErr := container.slice(architecture.offset, architecture.size)
		if sliceErr != nil {
			return sliceErr
		}
		if slice.size < 32 {
			return invalidMachO("FAT slice is smaller than a 64-bit header")
		}
		header, readErr := slice.read(0, 32)
		if readErr != nil {
			return readErr
		}
		if binary.LittleEndian.Uint32(header[0:4]) != machOMagic64 {
			return invalidMachO("FAT slice is not little-endian Mach-O 64")
		}
		if binary.LittleEndian.Uint32(header[4:8]) != architecture.cpuType ||
			binary.LittleEndian.Uint32(header[8:12]) != architecture.cpuSubtype {
			return invalidMachO("FAT outer and inner architecture disagree")
		}
		if binary.LittleEndian.Uint32(header[12:16]) != machOFileExecute ||
			binary.LittleEndian.Uint32(header[24:28]) != expectedMachOFlags(machOProfileGit) ||
			binary.LittleEndian.Uint32(header[28:32]) != 0 {
			return invalidMachO("FAT slice header violates Git policy")
		}

		if architecture.cpuType == cpuTypeARM64 {
			if architecture.cpuSubtype != cpuSubtypeARM64All {
				return invalidMachO("non-ALL ARM64 FAT subtype")
			}
			arm64Rows++
			selected = slice
		}
	}
	if arm64Rows != 1 {
		return invalidMachO("FAT container must have exactly one ARM64/ALL row")
	}
	return validateMachOSlice(selected, machOProfileGit, nil)
}

func expectedMachOFlags(profile machOProfile) uint32 {
	if profile == machOProfileGit {
		return machOFlagNoUndefs | machOFlagDyldLink | machOFlagTwoLevel | machOFlagPIE
	}
	return machOFlagDyldLink | machOFlagPIE
}

type machOSliceArchitecture struct {
	cpuType    uint32
	cpuSubtype uint32
}

type machOSegment struct {
	fileOffset        uint64
	fileSize          uint64
	initialProtection uint32
}

type machOSymtab struct {
	symbolCount uint32
}

type machODysymtab struct {
	localIndex          uint32
	localCount          uint32
	externalIndex       uint32
	externalCount       uint32
	undefinedIndex      uint32
	undefinedCount      uint32
	tocOffset           uint32
	tocCount            uint32
	moduleOffset        uint32
	moduleCount         uint32
	externalRefOffset   uint32
	externalRefCount    uint32
	indirectOffset      uint32
	indirectCount       uint32
	externalRelocOffset uint32
	externalRelocCount  uint32
	localRelocOffset    uint32
	localRelocCount     uint32
}

type machOCommandState struct {
	slice         checkedMachOReader
	profile       machOProfile
	segments      []machOSegment
	segmentNames  map[[16]byte]struct{}
	dylibPaths    map[string]struct{}
	singletons    map[uint32]struct{}
	symtab        *machOSymtab
	dysymtab      *machODysymtab
	mainEntry     uint64
	mainCount     uint32
	dylinkerCount uint32
}

//nolint:gocyclo // Keeping header and command-consumption gates together makes exact ordering auditable.
func validateMachOSlice(slice checkedMachOReader, profile machOProfile, outer *machOSliceArchitecture) error {
	header, err := slice.read(0, 32)
	if err != nil {
		return err
	}
	if binary.LittleEndian.Uint32(header[0:4]) != machOMagic64 {
		return invalidMachO("slice is not little-endian Mach-O 64")
	}
	cpuType := binary.LittleEndian.Uint32(header[4:8])
	cpuSubtype := binary.LittleEndian.Uint32(header[8:12])
	if outer != nil && (cpuType != outer.cpuType || cpuSubtype != outer.cpuSubtype) {
		return invalidMachO("outer and inner architecture disagree")
	}
	if cpuType != cpuTypeARM64 || cpuSubtype != cpuSubtypeARM64All {
		return invalidMachO("slice is not ARM64/ALL")
	}
	if binary.LittleEndian.Uint32(header[12:16]) != machOFileExecute {
		return invalidMachO("slice is not MH_EXECUTE")
	}
	commandCount := binary.LittleEndian.Uint32(header[16:20])
	commandBytes := binary.LittleEndian.Uint32(header[20:24])
	if commandCount > machOMaxCommands || commandBytes > machOMaxCommandBytes {
		return invalidMachO("load-command bounds exceeded")
	}
	if binary.LittleEndian.Uint32(header[24:28]) != expectedMachOFlags(profile) {
		return invalidMachO("header flags violate profile")
	}
	if binary.LittleEndian.Uint32(header[28:32]) != 0 {
		return invalidMachO("reserved header word is nonzero")
	}
	if !boundedExtent(32, uint64(commandBytes), slice.size) {
		return invalidMachO("load-command area exceeds slice")
	}
	commands, err := slice.read(32, uint64(commandBytes))
	if err != nil {
		return err
	}
	state := machOCommandState{
		slice:        slice,
		profile:      profile,
		segmentNames: make(map[[16]byte]struct{}),
		dylibPaths:   make(map[string]struct{}),
		singletons:   make(map[uint32]struct{}),
	}
	offset := uint64(0)
	for range commandCount {
		if !boundedExtent(offset, 8, uint64(len(commands))) {
			return invalidMachO("truncated load command")
		}
		command := binary.LittleEndian.Uint32(commands[offset : offset+4])
		commandSize := binary.LittleEndian.Uint32(commands[offset+4 : offset+8])
		if commandSize < 8 || commandSize%8 != 0 {
			return invalidMachO("invalid load-command size")
		}
		end, ok := checkedAdd64(offset, uint64(commandSize))
		if !ok || end > uint64(len(commands)) {
			return invalidMachO("load command exceeds command area")
		}
		if err := state.parseCommand(command, commands[offset:end]); err != nil {
			return err
		}
		offset = end
	}
	if offset != uint64(len(commands)) {
		return invalidMachO("load-command sizes do not consume sizeofcmds")
	}
	if state.mainCount != 1 || state.dylinkerCount != 1 {
		return invalidMachO("required singleton load command is missing or duplicated")
	}
	if state.dysymtab != nil {
		if state.symtab == nil {
			return invalidMachO("LC_DYSYMTAB requires LC_SYMTAB")
		}
		if err := state.validateDysymtab(); err != nil {
			return err
		}
	}
	if err := state.validateSegmentsAndEntry(); err != nil {
		return err
	}
	return nil
}

//nolint:gocyclo // The switch is the auditable closed load-command allowlist.
func (state *machOCommandState) parseCommand(command uint32, data []byte) error {
	switch command {
	case machOLCSegment64:
		return state.parseSegment(data)
	case machOLCSymtab:
		if err := state.markSingleton(command); err != nil {
			return err
		}
		return state.parseSymtab(data)
	case machOLCDysymtab:
		if err := state.markSingleton(command); err != nil {
			return err
		}
		return state.parseDysymtab(data)
	case machOLCLoadDylib, machOLCLoadWeakDylib, machOLCReexportDylib,
		machOLCLazyLoadDylib, machOLCLoadUpwardDylib:
		return state.parseDylib(data)
	case machOLCLoadDylinker:
		state.dylinkerCount++
		if state.dylinkerCount != 1 {
			return invalidMachO("duplicate LC_LOAD_DYLINKER")
		}
		return state.parseDylinker(data)
	case machOLCUUID:
		if err := state.markSingleton(command); err != nil {
			return err
		}
		return requireCommandSize(data, 24)
	case machOLCCodeSignature, machOLCFunctionStarts, machOLCDataInCode,
		machOLCDyldExportsTrie, machOLCDyldChainedFixups:
		if err := state.markSingleton(command); err != nil {
			return err
		}
		return state.parseLinkeditData(data)
	case machOLCDyldInfoOnly:
		if err := state.markSingleton(command); err != nil {
			return err
		}
		return state.parseDyldInfo(data)
	case machOLCMain:
		state.mainCount++
		if state.mainCount != 1 {
			return invalidMachO("duplicate LC_MAIN")
		}
		return state.parseMain(data)
	case machOLCSourceVersion:
		if err := state.markSingleton(command); err != nil {
			return err
		}
		return requireCommandSize(data, 16)
	case machOLCBuildVersion:
		if err := state.markSingleton(command); err != nil {
			return err
		}
		return state.parseBuildVersion(data)
	default:
		return invalidMachO("load command is outside the closed allowlist")
	}
}

func (state *machOCommandState) markSingleton(command uint32) error {
	if _, duplicate := state.singletons[command]; duplicate {
		return invalidMachO("duplicate singleton load command")
	}
	state.singletons[command] = struct{}{}
	return nil
}

func requireCommandSize(command []byte, exact int) error {
	if len(command) != exact {
		return invalidMachO("load command has wrong fixed size")
	}
	return nil
}

//nolint:gocyclo // The branches mirror the fixed segment and section extent grammar.
func (state *machOCommandState) parseSegment(command []byte) error {
	if len(command) < 72 {
		return invalidMachO("LC_SEGMENT_64 is truncated")
	}
	sectionCount := binary.LittleEndian.Uint32(command[64:68])
	sectionBytes, ok := checkedMul64(uint64(sectionCount), 80)
	if !ok {
		return invalidMachO("section table size overflows")
	}
	exactSize, ok := checkedAdd64(72, sectionBytes)
	if !ok || exactSize != uint64(len(command)) {
		return invalidMachO("LC_SEGMENT_64 has wrong size")
	}
	var name [16]byte
	copy(name[:], command[8:24])
	if _, duplicate := state.segmentNames[name]; duplicate {
		return invalidMachO("duplicate segment name")
	}
	state.segmentNames[name] = struct{}{}

	virtualAddress := binary.LittleEndian.Uint64(command[24:32])
	virtualSize := binary.LittleEndian.Uint64(command[32:40])
	virtualEnd, ok := checkedAdd64(virtualAddress, virtualSize)
	if !ok {
		return invalidMachO("segment VM range overflows")
	}
	fileOffset := binary.LittleEndian.Uint64(command[40:48])
	fileSize := binary.LittleEndian.Uint64(command[48:56])
	if !canonicalExtent(fileOffset, fileSize, state.slice.size) {
		return invalidMachO("segment file range is invalid")
	}
	fileEnd, _ := checkedAdd64(fileOffset, fileSize)
	state.segments = append(state.segments, machOSegment{
		fileOffset:        fileOffset,
		fileSize:          fileSize,
		initialProtection: binary.LittleEndian.Uint32(command[60:64]),
	})

	for index := range sectionCount {
		start := uint64(72) + uint64(index)*80
		section := command[start : start+80]
		address := binary.LittleEndian.Uint64(section[32:40])
		size := binary.LittleEndian.Uint64(section[40:48])
		end, rangeOK := checkedAdd64(address, size)
		if !rangeOK || address < virtualAddress || end > virtualEnd {
			return invalidMachO("section VM range is outside segment")
		}

		offset := uint64(binary.LittleEndian.Uint32(section[48:52]))
		sectionType := binary.LittleEndian.Uint32(section[64:68]) & machOSectionTypeMask
		zeroFill := sectionType == machOSectionZeroFill ||
			sectionType == machOSectionGBZeroFill ||
			sectionType == machOSectionThreadZeroFill
		if zeroFill {
			if offset != 0 {
				return invalidMachO("zerofill section has a file offset")
			}
		} else {
			if !canonicalExtent(offset, size, state.slice.size) {
				return invalidMachO("section file range is outside slice")
			}
			sectionEnd, _ := checkedAdd64(offset, size)
			if offset < fileOffset || sectionEnd > fileEnd {
				return invalidMachO("section file range is outside segment")
			}
		}

		relocationOffset := uint64(binary.LittleEndian.Uint32(section[56:60]))
		relocationCount := uint64(binary.LittleEndian.Uint32(section[60:64]))
		relocationBytes, multiplyOK := checkedMul64(relocationCount, 8)
		if !multiplyOK || !canonicalExtent(relocationOffset, relocationBytes, state.slice.size) {
			return invalidMachO("section relocation range is invalid")
		}
	}
	return nil
}

func (state *machOCommandState) parseSymtab(command []byte) error {
	if err := requireCommandSize(command, 24); err != nil {
		return err
	}
	symbolOffset := uint64(binary.LittleEndian.Uint32(command[8:12]))
	symbolCount := binary.LittleEndian.Uint32(command[12:16])
	symbolBytes, ok := checkedMul64(uint64(symbolCount), 16)
	if !ok || !canonicalExtent(symbolOffset, symbolBytes, state.slice.size) {
		return invalidMachO("symbol table extent is invalid")
	}
	stringOffset := uint64(binary.LittleEndian.Uint32(command[16:20]))
	stringSize := uint64(binary.LittleEndian.Uint32(command[20:24]))
	if !canonicalExtent(stringOffset, stringSize, state.slice.size) {
		return invalidMachO("string table extent is invalid")
	}
	stringTable, err := state.slice.read(stringOffset, stringSize)
	if err != nil {
		return err
	}
	lastNUL := bytes.LastIndexByte(stringTable, 0)
	if symbolCount != 0 && lastNUL < 0 {
		return invalidMachO("symbol string table has no terminator")
	}

	const symbolChunkBytes = 64 << 10
	buffer := make([]byte, symbolChunkBytes)
	for consumed := uint64(0); consumed < symbolBytes; {
		remaining := symbolBytes - consumed
		chunkSize := min(remaining, uint64(len(buffer)))
		chunk := buffer[:chunkSize]
		if err := state.slice.readInto(symbolOffset+consumed, chunk); err != nil {
			return err
		}
		for offset := 0; offset < len(chunk); offset += 16 {
			stringIndex := binary.LittleEndian.Uint32(chunk[offset : offset+4])
			if uint64(stringIndex) >= stringSize || int64(stringIndex) > int64(lastNUL) {
				return invalidMachO("symbol string index is unterminated or out of range")
			}
		}
		consumed += chunkSize
	}
	state.symtab = &machOSymtab{symbolCount: symbolCount}
	return nil
}

func (state *machOCommandState) parseDysymtab(command []byte) error {
	if err := requireCommandSize(command, 80); err != nil {
		return err
	}
	state.dysymtab = &machODysymtab{
		localIndex:          binary.LittleEndian.Uint32(command[8:12]),
		localCount:          binary.LittleEndian.Uint32(command[12:16]),
		externalIndex:       binary.LittleEndian.Uint32(command[16:20]),
		externalCount:       binary.LittleEndian.Uint32(command[20:24]),
		undefinedIndex:      binary.LittleEndian.Uint32(command[24:28]),
		undefinedCount:      binary.LittleEndian.Uint32(command[28:32]),
		tocOffset:           binary.LittleEndian.Uint32(command[32:36]),
		tocCount:            binary.LittleEndian.Uint32(command[36:40]),
		moduleOffset:        binary.LittleEndian.Uint32(command[40:44]),
		moduleCount:         binary.LittleEndian.Uint32(command[44:48]),
		externalRefOffset:   binary.LittleEndian.Uint32(command[48:52]),
		externalRefCount:    binary.LittleEndian.Uint32(command[52:56]),
		indirectOffset:      binary.LittleEndian.Uint32(command[56:60]),
		indirectCount:       binary.LittleEndian.Uint32(command[60:64]),
		externalRelocOffset: binary.LittleEndian.Uint32(command[64:68]),
		externalRelocCount:  binary.LittleEndian.Uint32(command[68:72]),
		localRelocOffset:    binary.LittleEndian.Uint32(command[72:76]),
		localRelocCount:     binary.LittleEndian.Uint32(command[76:80]),
	}
	return nil
}

func (state *machOCommandState) validateDysymtab() error {
	table := state.dysymtab
	if !boundedCountRange(table.localIndex, table.localCount, state.symtab.symbolCount) ||
		!boundedCountRange(table.externalIndex, table.externalCount, state.symtab.symbolCount) ||
		!boundedCountRange(table.undefinedIndex, table.undefinedCount, state.symtab.symbolCount) {
		return invalidMachO("dynamic symbol partition exceeds symbol table")
	}
	extents := [...]struct {
		offset uint32
		count  uint32
		width  uint64
	}{
		{table.tocOffset, table.tocCount, 8},
		{table.moduleOffset, table.moduleCount, 56},
		{table.externalRefOffset, table.externalRefCount, 4},
		{table.indirectOffset, table.indirectCount, 4},
		{table.externalRelocOffset, table.externalRelocCount, 8},
		{table.localRelocOffset, table.localRelocCount, 8},
	}
	for _, extent := range extents {
		bytes, ok := checkedMul64(uint64(extent.count), extent.width)
		if !ok || !canonicalExtent(uint64(extent.offset), bytes, state.slice.size) {
			return invalidMachO("dynamic symbol table extent is invalid")
		}
	}
	return nil
}

func boundedCountRange(index, count, limit uint32) bool {
	end := uint64(index) + uint64(count)
	return end <= uint64(limit)
}

func (state *machOCommandState) parseDylib(command []byte) error {
	if len(command) < 24 || binary.LittleEndian.Uint32(command[8:12]) != 24 {
		return invalidMachO("dylib command has invalid string offset")
	}
	loadedPath, err := parseMachOPath(command, 24)
	if err != nil {
		return err
	}
	if !isSystemDylibPath(loadedPath) {
		return invalidMachO("dylib path is outside system trust base")
	}
	if _, duplicate := state.dylibPaths[loadedPath]; duplicate {
		return invalidMachO("duplicate direct dylib path")
	}
	state.dylibPaths[loadedPath] = struct{}{}
	if state.profile == machOProfileGit && strings.Contains(loadedPath, "libxcselect") {
		return invalidMachO("Git loads libxcselect")
	}
	return nil
}

func (state *machOCommandState) parseDylinker(command []byte) error {
	if len(command) < 12 || binary.LittleEndian.Uint32(command[8:12]) != 12 {
		return invalidMachO("dylinker command has invalid string offset")
	}
	loadedPath, err := parseMachOPath(command, 12)
	if err != nil {
		return err
	}
	if loadedPath != "/usr/lib/dyld" {
		return invalidMachO("unexpected dynamic linker")
	}
	return nil
}

func parseMachOPath(command []byte, start int) (string, error) {
	if start >= len(command) {
		return "", invalidMachO("path command is truncated")
	}
	remainder := command[start:]
	terminator := bytes.IndexByte(remainder, 0)
	if terminator <= 0 {
		return "", invalidMachO("path is empty or unterminated")
	}
	used := start + terminator + 1
	exactSize := (used + 7) &^ 7
	if exactSize != len(command) {
		return "", invalidMachO("path command has noncanonical size")
	}
	for _, value := range command[used:] {
		if value != 0 {
			return "", invalidMachO("path command has nonzero padding")
		}
	}
	pathBytes := remainder[:terminator]
	for _, value := range pathBytes {
		if value < 0x21 || value > 0x7e {
			return "", invalidMachO("path contains a non-printable byte")
		}
	}
	loadedPath := string(pathBytes)
	if !strings.HasPrefix(loadedPath, "/") || pathpkg.Clean(loadedPath) != loadedPath {
		return "", invalidMachO("path is not clean and absolute")
	}
	return loadedPath, nil
}

func isSystemDylibPath(loadedPath string) bool {
	return (strings.HasPrefix(loadedPath, "/usr/lib/") && len(loadedPath) > len("/usr/lib/")) ||
		(strings.HasPrefix(loadedPath, "/System/Library/") && len(loadedPath) > len("/System/Library/"))
}

func (state *machOCommandState) parseLinkeditData(command []byte) error {
	if err := requireCommandSize(command, 16); err != nil {
		return err
	}
	offset := uint64(binary.LittleEndian.Uint32(command[8:12]))
	size := uint64(binary.LittleEndian.Uint32(command[12:16]))
	// Apple's linker preserves an in-slice insertion cursor in dataoff for an
	// empty linkedit_data_command blob. Model the pair as a checked half-open
	// extent: unlike the other file/table/relocation families, zero size does
	// not make offset a canonical sentinel and never authorizes a read.
	if !boundedExtent(offset, size, state.slice.size) {
		return invalidMachO("linkedit data extent is invalid")
	}
	return nil
}

func (state *machOCommandState) parseDyldInfo(command []byte) error {
	if err := requireCommandSize(command, 48); err != nil {
		return err
	}
	for offset := 8; offset < 48; offset += 8 {
		dataOffset := uint64(binary.LittleEndian.Uint32(command[offset : offset+4]))
		dataSize := uint64(binary.LittleEndian.Uint32(command[offset+4 : offset+8]))
		if !canonicalExtent(dataOffset, dataSize, state.slice.size) {
			return invalidMachO("dyld-info extent is invalid")
		}
	}
	return nil
}

func (state *machOCommandState) parseMain(command []byte) error {
	if err := requireCommandSize(command, 24); err != nil {
		return err
	}
	state.mainEntry = binary.LittleEndian.Uint64(command[8:16])
	return nil
}

func (state *machOCommandState) parseBuildVersion(command []byte) error {
	if len(command) < 24 {
		return invalidMachO("LC_BUILD_VERSION is truncated")
	}
	if binary.LittleEndian.Uint32(command[8:12]) != machOPlatformMacOS {
		return invalidMachO("LC_BUILD_VERSION platform is not macOS")
	}
	toolCount := binary.LittleEndian.Uint32(command[20:24])
	if toolCount > machOMaxBuildTools {
		return invalidMachO("LC_BUILD_VERSION tool count exceeds policy")
	}
	toolBytes, ok := checkedMul64(uint64(toolCount), 8)
	if !ok {
		return invalidMachO("LC_BUILD_VERSION tool table overflows")
	}
	exactSize, ok := checkedAdd64(24, toolBytes)
	if !ok || exactSize != uint64(len(command)) {
		return invalidMachO("LC_BUILD_VERSION has wrong size")
	}
	seen := make(map[uint32]struct{}, toolCount)
	for index := range toolCount {
		offset := 24 + int(index)*8
		toolID := binary.LittleEndian.Uint32(command[offset : offset+4])
		if toolID == 0 {
			return invalidMachO("LC_BUILD_VERSION has zero tool ID")
		}
		if _, duplicate := seen[toolID]; duplicate {
			return invalidMachO("LC_BUILD_VERSION has duplicate tool ID")
		}
		seen[toolID] = struct{}{}
		// The following uint32 is, by definition, exactly the required packed
		// 16-bit-major, 8-bit-minor, 8-bit-patch representation. No bit lies
		// outside those fields, so no lossy numeric normalization is performed.
		_ = binary.LittleEndian.Uint32(command[offset+4 : offset+8])
	}
	return nil
}

func (state *machOCommandState) validateSegmentsAndEntry() error {
	fileBacked := make([]machOSegment, 0, len(state.segments))
	for _, segment := range state.segments {
		if segment.fileSize != 0 {
			fileBacked = append(fileBacked, segment)
		}
	}
	sort.Slice(fileBacked, func(left, right int) bool {
		if fileBacked[left].fileOffset == fileBacked[right].fileOffset {
			return fileBacked[left].fileSize < fileBacked[right].fileSize
		}
		return fileBacked[left].fileOffset < fileBacked[right].fileOffset
	})
	for index := 1; index < len(fileBacked); index++ {
		previousEnd, _ := checkedAdd64(fileBacked[index-1].fileOffset, fileBacked[index-1].fileSize)
		if fileBacked[index].fileOffset < previousEnd {
			return invalidMachO("overlapping segment file ranges")
		}
	}

	entryMatches := 0
	for _, segment := range fileBacked {
		end, _ := checkedAdd64(segment.fileOffset, segment.fileSize)
		if state.mainEntry >= segment.fileOffset && state.mainEntry < end &&
			segment.initialProtection&machOVMProtectionExecute != 0 {
			entryMatches++
		}
	}
	if entryMatches != 1 {
		return invalidMachO("LC_MAIN entry is not in one executable file-backed segment")
	}
	return nil
}
