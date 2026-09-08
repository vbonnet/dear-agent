package buildauthority

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

const machOFixtureSize = 8 << 10

func TestValidateMachOThinProfiles(t *testing.T) {
	t.Parallel()

	goImage := makeThinMachOFixture(t, machOProfileGo, baseMachOCommands()...)
	if err := validateGoMachO(bytes.NewReader(goImage), int64(len(goImage))); err != nil {
		t.Fatalf("valid Go image: %v", err)
	}
	if err := validateGitMachO(bytes.NewReader(goImage), int64(len(goImage))); err == nil {
		t.Fatal("Git profile admitted Go header flags")
	}

	gitImage := makeThinMachOFixture(t, machOProfileGit, baseMachOCommands()...)
	if err := validateGitMachO(bytes.NewReader(gitImage), int64(len(gitImage))); err != nil {
		t.Fatalf("valid Git image: %v", err)
	}
	if err := validateGoMachO(bytes.NewReader(gitImage), int64(len(gitImage))); err == nil {
		t.Fatal("Go profile admitted Git header flags")
	}
}

func TestValidateMachOThinHeaderClosure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func([]byte)
		wantDetail string
	}{
		{name: "cpu", mutate: func(image []byte) { putUint32(image, 4, cpuTypeARM64+1) }},
		{name: "arm64e", mutate: func(image []byte) { putUint32(image, 8, 2) }},
		{name: "capability bits", mutate: func(image []byte) { putUint32(image, 8, 0x80000000) }},
		{name: "file type", mutate: func(image []byte) { putUint32(image, 12, 1) }},
		{name: "flags missing", mutate: func(image []byte) { putUint32(image, 24, machOFlagPIE) }},
		{name: "flags extra", mutate: func(image []byte) { putUint32(image, 24, expectedMachOFlags(machOProfileGo)|0x10) }},
		{name: "reserved", mutate: func(image []byte) { putUint32(image, 28, 1) }},
		{name: "too many commands", mutate: func(image []byte) { putUint32(image, 16, machOMaxCommands+1) }, wantDetail: "load-command bounds exceeded"},
		{name: "too many command bytes", mutate: func(image []byte) { putUint32(image, 20, machOMaxCommandBytes+1) }, wantDetail: "load-command bounds exceeded"},
		{name: "command area outside slice", mutate: func(image []byte) { putUint32(image, 20, uint32(len(image))) }},
		{name: "too few declared commands", mutate: func(image []byte) { putUint32(image, 16, 2) }},
		{name: "too many declared commands", mutate: func(image []byte) { putUint32(image, 16, 4) }},
		{name: "unknown command", mutate: func(image []byte) { putUint32(image, 32, 0x77777777) }},
		{name: "command too small", mutate: func(image []byte) { putUint32(image, 36, 0) }},
		{name: "command unaligned", mutate: func(image []byte) { putUint32(image, 36, 73) }},
		{name: "command outside area", mutate: func(image []byte) { putUint32(image, 36, uint32(len(image))) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			image := makeThinMachOFixture(t, machOProfileGo, baseMachOCommands()...)
			test.mutate(image)
			if test.wantDetail != "" {
				assertInvalidMachODetail(t, image, machOProfileGo, test.wantDetail)
				return
			}
			assertInvalidMachO(t, image, machOProfileGo)
		})
	}

	t.Run("swapped thin", func(t *testing.T) {
		t.Parallel()
		image := makeThinMachOFixture(t, machOProfileGo, baseMachOCommands()...)
		copy(image[:4], []byte{0xfe, 0xed, 0xfa, 0xcf})
		assertInvalidMachO(t, image, machOProfileGo)
	})
}

func TestValidateMachOFATClosure(t *testing.T) {
	t.Parallel()

	arm64 := fatFixtureArchitecture{
		cpuType:    cpuTypeARM64,
		cpuSubtype: cpuSubtypeARM64All,
		align:      3,
		image:      makeThinMachOFixture(t, machOProfileGit, baseMachOCommands()...),
	}
	one, _ := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
	if err := validateGitMachO(bytes.NewReader(one), int64(len(one))); err != nil {
		t.Fatalf("one-row FAT image: %v", err)
	}
	if err := validateGoMachO(bytes.NewReader(one), int64(len(one))); err == nil {
		t.Fatal("Go profile admitted FAT image")
	}

	rows := make([]fatFixtureArchitecture, 0, machOMaxFatRows)
	rows = append(rows, arm64)
	for index := uint32(1); index < machOMaxFatRows; index++ {
		rows = append(rows, fatFixtureArchitecture{
			cpuType:    0x02000000 + index,
			cpuSubtype: index,
			align:      3,
			image:      makeHeaderOnlyMachO(0x02000000+index, index, machOProfileGit),
		})
	}
	thirtyTwo, _ := makeFatMachOFixture(t, rows)
	if err := validateGitMachO(bytes.NewReader(thirtyTwo), int64(len(thirtyTwo))); err != nil {
		t.Fatalf("32-row FAT image: %v", err)
	}

	tests := []struct {
		name  string
		image func(*testing.T) []byte
	}{
		{
			name: "zero rows",
			image: func(*testing.T) []byte {
				image := make([]byte, 32)
				binary.BigEndian.PutUint32(image[0:4], fatMagic32)
				return image
			},
		},
		{
			name: "33 rows",
			image: func(t *testing.T) []byte {
				architectures := make([]fatFixtureArchitecture, 0, machOMaxFatRows+1)
				for index := range machOMaxFatRows {
					cpuType := uint32(index + 1)
					architectures = append(architectures, fatFixtureArchitecture{
						cpuType: cpuType, cpuSubtype: 0, align: 3,
						image: makeHeaderOnlyMachO(cpuType, 0, machOProfileGit),
					})
				}
				architectures = append(architectures, arm64)
				image, _ := makeFatMachOFixture(t, architectures)
				return image
			},
		},
		{
			name: "duplicate row",
			image: func(t *testing.T) []byte {
				image, _ := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64, arm64})
				return image
			},
		},
		{
			name: "no arm64 row",
			image: func(t *testing.T) []byte {
				image, _ := makeFatMachOFixture(t, []fatFixtureArchitecture{{
					cpuType: 7, cpuSubtype: 3, align: 3,
					image: makeHeaderOnlyMachO(7, 3, machOProfileGit),
				}})
				return image
			},
		},
		{
			name: "arm64e row",
			image: func(t *testing.T) []byte {
				architecture := arm64
				architecture.cpuSubtype = 2
				architecture.image = append([]byte(nil), arm64.image...)
				putUint32(architecture.image, 8, 2)
				image, _ := makeFatMachOFixture(t, []fatFixtureArchitecture{architecture})
				return image
			},
		},
		{
			name: "zero slice size",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				binary.BigEndian.PutUint32(image[rows[0]+12:rows[0]+16], 0)
				return image
			},
		},
		{
			name: "alignment exponent",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				binary.BigEndian.PutUint32(image[rows[0]+16:rows[0]+20], 31)
				return image
			},
		},
		{
			name: "misaligned slice",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				offset := binary.BigEndian.Uint32(image[rows[0]+8 : rows[0]+12])
				binary.BigEndian.PutUint32(image[rows[0]+8:rows[0]+12], offset+1)
				return image
			},
		},
		{
			name: "slice overlaps table",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				binary.BigEndian.PutUint32(image[rows[0]+8:rows[0]+12], 24)
				return image
			},
		},
		{
			name: "slice outside file",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				binary.BigEndian.PutUint32(image[rows[0]+12:rows[0]+16], uint32(len(image)))
				return image
			},
		},
		{
			name: "outer inner mismatch",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				binary.BigEndian.PutUint32(image[rows[0]:rows[0]+4], cpuTypeARM64+1)
				return image
			},
		},
		{
			name: "inner non Mach-O",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				offset := binary.BigEndian.Uint32(image[rows[0]+8 : rows[0]+12])
				putUint32(image, int(offset), 0)
				return image
			},
		},
		{
			name: "inner reserved word",
			image: func(t *testing.T) []byte {
				image, rows := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64})
				offset := binary.BigEndian.Uint32(image[rows[0]+8 : rows[0]+12])
				putUint32(image, int(offset)+28, 1)
				return image
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			image := test.image(t)
			assertInvalidMachO(t, image, machOProfileGit)
		})
	}

	t.Run("overlapping slices", func(t *testing.T) {
		t.Parallel()
		other := fatFixtureArchitecture{
			cpuType: 7, cpuSubtype: 3, align: 3,
			image: makeHeaderOnlyMachO(7, 3, machOProfileGit),
		}
		image, rowOffsets := makeFatMachOFixture(t, []fatFixtureArchitecture{arm64, other})
		firstOffset := binary.BigEndian.Uint32(image[rowOffsets[0]+8 : rowOffsets[0]+12])
		binary.BigEndian.PutUint32(image[rowOffsets[1]+8:rowOffsets[1]+12], firstOffset+8)
		assertInvalidMachO(t, image, machOProfileGit)
	})

	for _, magic := range []uint32{fatCigam32, fatMagic64, fatCigam64} {
		t.Run("unsupported fat magic", func(t *testing.T) {
			t.Parallel()
			image := append([]byte(nil), one...)
			binary.BigEndian.PutUint32(image[0:4], magic)
			assertInvalidMachO(t, image, machOProfileGit)
		})
	}
}

func TestValidateMachOFATGitEmptyDataInCodeCursor(t *testing.T) {
	t.Parallel()

	commands := append(baseMachOCommands(), makeLinkeditCommand(machOLCDataInCode, 4096, 0))
	arm64Image := makeThinMachOFixture(t, machOProfileGit, commands...)
	fatImage, _ := makeFatMachOFixture(t, []fatFixtureArchitecture{
		{
			cpuType: 7, cpuSubtype: 3, align: 3,
			image: makeHeaderOnlyMachO(7, 3, machOProfileGit),
		},
		{
			cpuType: cpuTypeARM64, cpuSubtype: cpuSubtypeARM64All, align: 3,
			image: arm64Image,
		},
	})
	if err := validateGitMachO(bytes.NewReader(fatImage), int64(len(fatImage))); err != nil {
		t.Fatalf("FAT Git empty LC_DATA_IN_CODE cursor: %v", err)
	}
}

func TestValidateMachOAllowedCommandSizes(t *testing.T) {
	t.Parallel()

	tests := allowedMachOCommandCases()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			commands := test.commands()
			image := makeThinMachOFixture(t, machOProfileGo, commands...)
			test.prepare(image)
			if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
				t.Fatalf("exact command: %v", err)
			}

			commands = test.commands()
			commands[test.target] = extendMachOCommand(commands[test.target])
			oneOver := makeThinMachOFixture(t, machOProfileGo, commands...)
			test.prepare(oneOver)
			assertInvalidMachO(t, oneOver, machOProfileGo)
		})
	}
}

func TestValidateMachOSingletonMultiplicity(t *testing.T) {
	t.Parallel()

	linkedit := func(code uint32) []byte { return makeLinkeditCommand(code, 7000, 1) }
	tests := []struct {
		name     string
		commands func() [][]byte
		prepare  func([]byte)
	}{
		{name: "main", commands: func() [][]byte { return append(baseMachOCommands(), makeMainCommand(4096)) }},
		{name: "dylinker", commands: func() [][]byte { return append(baseMachOCommands(), makeDylinkerCommand("/usr/lib/dyld")) }},
		{name: "symtab", commands: func() [][]byte {
			return append(baseMachOCommands(), makeSymtabCommand(7000, 1, 7016, 1), makeSymtabCommand(7000, 1, 7016, 1))
		}, prepare: prepareSymtabPayload},
		{name: "dysymtab", commands: func() [][]byte {
			return append(baseMachOCommands(), makeSymtabCommand(7000, 1, 7016, 1), makeDysymtabCommand(), makeDysymtabCommand())
		}, prepare: prepareSymtabPayload},
		{name: "uuid", commands: func() [][]byte {
			return append(baseMachOCommands(), makeFixedCommand(machOLCUUID, 24), makeFixedCommand(machOLCUUID, 24))
		}},
		{name: "code signature", commands: func() [][]byte {
			return append(baseMachOCommands(), linkedit(machOLCCodeSignature), linkedit(machOLCCodeSignature))
		}},
		{name: "dyld info", commands: func() [][]byte {
			return append(baseMachOCommands(), makeFixedCommand(machOLCDyldInfoOnly, 48), makeFixedCommand(machOLCDyldInfoOnly, 48))
		}},
		{name: "function starts", commands: func() [][]byte {
			return append(baseMachOCommands(), linkedit(machOLCFunctionStarts), linkedit(machOLCFunctionStarts))
		}},
		{name: "data in code", commands: func() [][]byte {
			return append(baseMachOCommands(), linkedit(machOLCDataInCode), linkedit(machOLCDataInCode))
		}},
		{name: "source version", commands: func() [][]byte {
			return append(baseMachOCommands(), makeFixedCommand(machOLCSourceVersion, 16), makeFixedCommand(machOLCSourceVersion, 16))
		}},
		{name: "build version", commands: func() [][]byte {
			return append(baseMachOCommands(), makeBuildVersionCommand(1), makeBuildVersionCommand(1))
		}},
		{name: "exports trie", commands: func() [][]byte {
			return append(baseMachOCommands(), linkedit(machOLCDyldExportsTrie), linkedit(machOLCDyldExportsTrie))
		}},
		{name: "chained fixups", commands: func() [][]byte {
			return append(baseMachOCommands(), linkedit(machOLCDyldChainedFixups), linkedit(machOLCDyldChainedFixups))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			image := makeThinMachOFixture(t, machOProfileGo, test.commands()...)
			if test.prepare != nil {
				test.prepare(image)
			}
			assertInvalidMachO(t, image, machOProfileGo)
		})
	}

	t.Run("repeatable commands with unique names", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(),
			makeSegmentCommand("__ZERO", 0x10000, 0x1000, 0, 0, 0, nil),
			makeDylibCommand(machOLCLoadDylib, "/usr/lib/libA.dylib"),
			makeDylibCommand(machOLCLoadWeakDylib, "/usr/lib/libB.dylib"),
		)
		image := makeThinMachOFixture(t, machOProfileGo, commands...)
		if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
			t.Fatalf("repeatable unique commands: %v", err)
		}
	})

	t.Run("duplicate segment name", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(), makeSegmentCommand("__TEXT", 0x10000, 0x1000, 0, 0, 0, nil))
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("duplicate dylib path", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(),
			makeDylibCommand(machOLCLoadDylib, "/usr/lib/libSame.dylib"),
			makeDylibCommand(machOLCLoadWeakDylib, "/usr/lib/libSame.dylib"),
		)
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})
}

func TestValidateMachOLoaderPaths(t *testing.T) {
	t.Parallel()

	for _, command := range []uint32{
		machOLCLoadDylib,
		machOLCLoadWeakDylib,
		machOLCReexportDylib,
		machOLCLazyLoadDylib,
		machOLCLoadUpwardDylib,
	} {
		t.Run("admitted dylib variant", func(t *testing.T) {
			t.Parallel()
			commands := append(baseMachOCommands(), makeDylibCommand(command, "/System/Library/Frameworks/A.framework/A"))
			image := makeThinMachOFixture(t, machOProfileGo, commands...)
			if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
				t.Fatalf("variant %#x: %v", command, err)
			}
		})
	}

	invalidPaths := []string{
		"usr/lib/libA.dylib",
		"@rpath/libA.dylib",
		"@loader_path/libA.dylib",
		"@executable_path/libA.dylib",
		"/usr/libevil/libA.dylib",
		"/System/LibraryEvil/libA.dylib",
		"/usr/lib/../bin/libA.dylib",
		"/usr/lib//libA.dylib",
		"/usr/lib/",
		"/usr/lib/lib A.dylib",
		"/usr/lib/lib\nA.dylib",
	}
	for _, loadedPath := range invalidPaths {
		t.Run("refused path "+strings.ReplaceAll(loadedPath, "/", "_"), func(t *testing.T) {
			t.Parallel()
			commands := append(baseMachOCommands(), makeDylibCommand(machOLCLoadDylib, loadedPath))
			assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
		})
	}

	t.Run("wrong dylib string offset", func(t *testing.T) {
		t.Parallel()
		command := makeDylibCommand(machOLCLoadDylib, "/usr/lib/libA.dylib")
		putUint32(command, 8, 25)
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), command)...), machOProfileGo)
	})

	t.Run("wrong dylinker string offset", func(t *testing.T) {
		t.Parallel()
		commands := baseMachOCommands()
		putUint32(commands[1], 8, 13)
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("nonzero string padding", func(t *testing.T) {
		t.Parallel()
		command := makeDylibCommand(machOLCLoadDylib, "/usr/lib/a")
		command[len(command)-1] = 1
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), command)...), machOProfileGo)
	})

	t.Run("wrong dylinker", func(t *testing.T) {
		t.Parallel()
		commands := baseMachOCommands()
		commands[1] = makeDylinkerCommand("/usr/lib/not-dyld")
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("missing dylinker", func(t *testing.T) {
		t.Parallel()
		commands := baseMachOCommands()
		commands = append(commands[:1], commands[2:]...)
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("Git libxcselect", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(), makeDylibCommand(machOLCLoadDylib, "/usr/lib/libxcselect.dylib"))
		gitImage := makeThinMachOFixture(t, machOProfileGit, commands...)
		assertInvalidMachO(t, gitImage, machOProfileGit)
		goImage := makeThinMachOFixture(t, machOProfileGo, commands...)
		if err := validateGoMachO(bytes.NewReader(goImage), int64(len(goImage))); err != nil {
			t.Fatalf("common Go loader policy should admit system path: %v", err)
		}
	})

	for _, forbidden := range []uint32{0x8000001c, 0x27, 0xd, 0x10, 0x80007777} {
		t.Run("forbidden path or required command", func(t *testing.T) {
			t.Parallel()
			command := makeFixedCommand(forbidden, 8)
			assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), command)...), machOProfileGo)
		})
	}
}

func TestValidateMachOSegmentAndSectionExtents(t *testing.T) {
	t.Parallel()

	t.Run("segment file overflow", func(t *testing.T) {
		t.Parallel()
		commands := baseMachOCommands()
		commands[0] = makeSegmentCommand("__TEXT", 0, machOFixtureSize, machOFixtureSize-1, 2, machOVMProtectionExecute, nil)
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("segment arithmetic overflow", func(t *testing.T) {
		t.Parallel()
		commands := baseMachOCommands()
		commands[0] = makeSegmentCommand("__TEXT", ^uint64(0)-1, 3, 0, machOFixtureSize, machOVMProtectionExecute, nil)
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("zero file extent requires zero offset", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(), makeSegmentCommand("__ZERO", 0x10000, 1, 1, 0, 0, nil))
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("segment overlap", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(), makeSegmentCommand("__OVER", 0x10000, 32, 16, 32, 0, nil))
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	for _, sectionType := range []uint32{machOSectionZeroFill, machOSectionGBZeroFill, machOSectionThreadZeroFill} {
		t.Run("admitted zerofill", func(t *testing.T) {
			t.Parallel()
			section := makeSection(4096, 16, 0, 0, 0, sectionType)
			commands := baseMachOCommands()
			commands[0] = makeSegmentCommand("__TEXT", 0, machOFixtureSize, 0, machOFixtureSize, machOVMProtectionExecute, [][]byte{section})
			image := makeThinMachOFixture(t, machOProfileGo, commands...)
			if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
				t.Fatalf("zerofill %#x: %v", sectionType, err)
			}
		})
	}

	t.Run("zerofill file offset", func(t *testing.T) {
		t.Parallel()
		section := makeSection(4096, 16, 1, 0, 0, machOSectionZeroFill)
		commands := baseMachOCommands()
		commands[0] = makeSegmentCommand("__TEXT", 0, machOFixtureSize, 0, machOFixtureSize, machOVMProtectionExecute, [][]byte{section})
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("file-backed section exact", func(t *testing.T) {
		t.Parallel()
		section := makeSection(4096, 16, 4096, machOFixtureSize-8, 1, 0)
		commands := baseMachOCommands()
		commands[0] = makeSegmentCommand("__TEXT", 0, machOFixtureSize, 0, machOFixtureSize, machOVMProtectionExecute, [][]byte{section})
		image := makeThinMachOFixture(t, machOProfileGo, commands...)
		if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
			t.Fatalf("section exact extent: %v", err)
		}
	})

	sectionCases := []struct {
		name    string
		section []byte
	}{
		{name: "VM outside", section: makeSection(machOFixtureSize, 1, 4096, 0, 0, 0)},
		{name: "file outside", section: makeSection(4096, 16, machOFixtureSize-8, 0, 0, 0)},
		{name: "zero file extent nonzero offset", section: makeSection(4096, 0, 1, 0, 0, 0)},
		{name: "relocation outside", section: makeSection(4096, 16, 4096, machOFixtureSize-7, 1, 0)},
		{name: "zero relocation nonzero offset", section: makeSection(4096, 16, 4096, 1, 0, 0)},
	}
	for _, test := range sectionCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			commands := baseMachOCommands()
			commands[0] = makeSegmentCommand("__TEXT", 0, machOFixtureSize, 0, machOFixtureSize, machOVMProtectionExecute, [][]byte{test.section})
			assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
		})
	}
}

func TestValidateMachOSymbolTables(t *testing.T) {
	t.Parallel()

	t.Run("symbol and string exact extents", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(), makeSymtabCommand(machOFixtureSize-16, 1, machOFixtureSize-1, 1))
		image := makeThinMachOFixture(t, machOProfileGo, commands...)
		if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
			t.Fatalf("exact symtab extents: %v", err)
		}
	})

	tests := []struct {
		name    string
		command []byte
		prepare func([]byte)
	}{
		{name: "symbol one over", command: makeSymtabCommand(machOFixtureSize-15, 1, 7000, 1)},
		{name: "string one over", command: makeSymtabCommand(7000, 1, machOFixtureSize, 1)},
		{name: "zero symbols nonzero offset", command: makeSymtabCommand(1, 0, 0, 0)},
		{name: "zero strings nonzero offset", command: makeSymtabCommand(0, 0, 1, 0)},
		{name: "string index outside", command: makeSymtabCommand(7000, 1, 7016, 1), prepare: func(image []byte) { putUint32(image, 7000, 1); image[7016] = 0 }},
		{name: "string index unterminated", command: makeSymtabCommand(7000, 1, 7016, 2), prepare: func(image []byte) { putUint32(image, 7000, 1); image[7016], image[7017] = 0, 'x' }},
		{name: "no string terminator", command: makeSymtabCommand(7000, 1, 7016, 1), prepare: func(image []byte) { image[7016] = 'x' }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			commands := append(baseMachOCommands(), test.command)
			image := makeThinMachOFixture(t, machOProfileGo, commands...)
			if test.prepare != nil {
				test.prepare(image)
			}
			assertInvalidMachO(t, image, machOProfileGo)
		})
	}

	t.Run("string index reaches later NUL", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(), makeSymtabCommand(7000, 1, 7016, 3))
		image := makeThinMachOFixture(t, machOProfileGo, commands...)
		putUint32(image, 7000, 1)
		copy(image[7016:7019], []byte{0, 'x', 0})
		if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
			t.Fatalf("later NUL: %v", err)
		}
	})

	t.Run("dysymtab requires symtab", func(t *testing.T) {
		t.Parallel()
		commands := append(baseMachOCommands(), makeDysymtabCommand())
		assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
	})

	t.Run("symbol partition boundary", func(t *testing.T) {
		t.Parallel()
		dysymtab := makeDysymtabCommand()
		putUint32(dysymtab, 8, 2)
		putUint32(dysymtab, 12, 1)
		commands := append(baseMachOCommands(), makeSymtabCommand(7000, 3, 7060, 1), dysymtab)
		image := makeThinMachOFixture(t, machOProfileGo, commands...)
		if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
			t.Fatalf("partition boundary: %v", err)
		}
		putUint32(image, commandFileOffset(commands, len(commands)-1)+12, 2)
		assertInvalidMachO(t, image, machOProfileGo)
	})

	dysymtabExtents := []struct {
		name       string
		offsetWord int
		countWord  int
		width      uint32
	}{
		{name: "toc", offsetWord: 32, countWord: 36, width: 8},
		{name: "module", offsetWord: 40, countWord: 44, width: 56},
		{name: "external refs", offsetWord: 48, countWord: 52, width: 4},
		{name: "indirect symbols", offsetWord: 56, countWord: 60, width: 4},
		{name: "external relocations", offsetWord: 64, countWord: 68, width: 8},
		{name: "local relocations", offsetWord: 72, countWord: 76, width: 8},
	}
	for _, test := range dysymtabExtents {
		t.Run(test.name+" row width", func(t *testing.T) {
			t.Parallel()
			dysymtab := makeDysymtabCommand()
			putUint32(dysymtab, test.offsetWord, machOFixtureSize-int(test.width))
			putUint32(dysymtab, test.countWord, 1)
			commands := append(baseMachOCommands(), makeSymtabCommand(7000, 1, 7016, 1), dysymtab)
			image := makeThinMachOFixture(t, machOProfileGo, commands...)
			prepareSymtabPayload(image)
			if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
				t.Fatalf("exact row extent: %v", err)
			}
			putUint32(image, commandFileOffset(commands, len(commands)-1)+test.offsetWord, machOFixtureSize-int(test.width)+1)
			assertInvalidMachO(t, image, machOProfileGo)
		})
		t.Run(test.name+" empty extent requires zero offset", func(t *testing.T) {
			t.Parallel()
			dysymtab := makeDysymtabCommand()
			putUint32(dysymtab, test.offsetWord, 1)
			commands := append(baseMachOCommands(), makeSymtabCommand(7000, 1, 7016, 1), dysymtab)
			image := makeThinMachOFixture(t, machOProfileGo, commands...)
			prepareSymtabPayload(image)
			assertInvalidMachO(t, image, machOProfileGo)
		})
	}
}

func TestValidateMachOOtherCommandExtents(t *testing.T) {
	t.Parallel()

	linkeditCommands := []struct {
		name string
		code uint32
	}{
		{name: "code signature", code: machOLCCodeSignature},
		{name: "function starts", code: machOLCFunctionStarts},
		{name: "data in code", code: machOLCDataInCode},
		{name: "exports trie", code: machOLCDyldExportsTrie},
		{name: "chained fixups", code: machOLCDyldChainedFixups},
	}
	for _, command := range linkeditCommands {
		t.Run(command.name, func(t *testing.T) {
			t.Parallel()
			valid := []struct {
				name   string
				offset int
				size   int
			}{
				{name: "interior empty", offset: 4096, size: 0},
				{name: "EOF empty", offset: machOFixtureSize, size: 0},
				{name: "nonempty exact", offset: machOFixtureSize - 1, size: 1},
			}
			for _, extent := range valid {
				t.Run(extent.name, func(t *testing.T) {
					t.Parallel()
					candidate := makeLinkeditCommand(command.code, extent.offset, extent.size)
					image := makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), candidate)...)
					if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
						t.Fatalf("valid extent (%d,%d): %v", extent.offset, extent.size, err)
					}
				})
			}

			invalid := []struct {
				name   string
				offset int
				size   int
			}{
				{name: "empty one past EOF", offset: machOFixtureSize + 1, size: 0},
				{name: "nonempty one over", offset: machOFixtureSize, size: 1},
			}
			for _, extent := range invalid {
				t.Run(extent.name, func(t *testing.T) {
					t.Parallel()
					candidate := makeLinkeditCommand(command.code, extent.offset, extent.size)
					image := makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), candidate)...)
					assertInvalidMachO(t, image, machOProfileGo)
				})
			}
		})
	}

	t.Run("dyld info five extents", func(t *testing.T) {
		t.Parallel()
		for field := 8; field < 48; field += 8 {
			command := makeFixedCommand(machOLCDyldInfoOnly, 48)
			putUint32(command, field, machOFixtureSize-1)
			putUint32(command, field+4, 1)
			image := makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), command)...)
			if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
				t.Fatalf("field %d exact: %v", field, err)
			}
			putUint32(command, field, machOFixtureSize)
			assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), command)...), machOProfileGo)
		}
	})

	dyldInfoPairs := []struct {
		name  string
		field int
	}{
		{name: "rebase", field: 8},
		{name: "bind", field: 16},
		{name: "weak bind", field: 24},
		{name: "lazy bind", field: 32},
		{name: "export", field: 40},
	}
	for _, pair := range dyldInfoPairs {
		t.Run("dyld info "+pair.name+" empty extent requires zero offset", func(t *testing.T) {
			t.Parallel()
			command := makeFixedCommand(machOLCDyldInfoOnly, 48)
			putUint32(command, pair.field, 1)
			image := makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), command)...)
			assertInvalidMachO(t, image, machOProfileGo)
		})
	}

	t.Run("main entry at segment start", func(t *testing.T) {
		t.Parallel()
		commands := baseMachOCommands()
		commands[2] = makeMainCommand(0)
		image := makeThinMachOFixture(t, machOProfileGo, commands...)
		if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
			t.Fatalf("entry at start: %v", err)
		}
	})

	for _, test := range []struct {
		name     string
		entry    uint64
		initProt uint32
	}{
		{name: "entry at end", entry: machOFixtureSize, initProt: machOVMProtectionExecute},
		{name: "nonexecutable segment", entry: 4096, initProt: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			commands := baseMachOCommands()
			commands[0] = makeSegmentCommand("__TEXT", 0, machOFixtureSize, 0, machOFixtureSize, test.initProt, nil)
			commands[2] = makeMainCommand(test.entry)
			assertInvalidMachO(t, makeThinMachOFixture(t, machOProfileGo, commands...), machOProfileGo)
		})
	}
}

func TestValidateMachOBuildVersion(t *testing.T) {
	t.Parallel()

	commands := append(baseMachOCommands(), makeBuildVersionCommand(machOMaxBuildTools))
	image := makeThinMachOFixture(t, machOProfileGo, commands...)
	if err := validateGoMachO(bytes.NewReader(image), int64(len(image))); err != nil {
		t.Fatalf("64 build tools: %v", err)
	}
	tooManyTools := makeThinMachOFixture(t, machOProfileGo,
		append(baseMachOCommands(), makeBuildVersionCommand(machOMaxBuildTools+1))...)
	assertInvalidMachODetail(t, tooManyTools, machOProfileGo, "LC_BUILD_VERSION tool count exceeds policy")

	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{name: "platform", mutate: func(command []byte) { putUint32(command, 8, 2) }},
		{name: "zero tool", mutate: func(command []byte) { putUint32(command, 24, 0) }},
		{name: "duplicate tool", mutate: func(command []byte) { putUint32(command, 32, 1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			command := makeBuildVersionCommand(2)
			test.mutate(command)
			image := makeThinMachOFixture(t, machOProfileGo, append(baseMachOCommands(), command)...)
			assertInvalidMachO(t, image, machOProfileGo)
		})
	}
}

func TestValidateMachOPanicAndReaderBounds(t *testing.T) {
	t.Parallel()

	if err := validateGoMachO(panickingReaderAt{}, 32); err == nil {
		t.Fatal("panic was admitted")
	}
	if err := validateGoMachO(nil, 32); err == nil {
		t.Fatal("nil reader was admitted")
	}
	if err := validateGoMachO(bytes.NewReader(make([]byte, 31)), 31); err == nil {
		t.Fatal("short container was admitted")
	}
	image := makeThinMachOFixture(t, machOProfileGo, baseMachOCommands()...)
	assertInvalidMachODetail(t, image, machOProfileGo, "container size is outside policy", int64(maxExecutableBytes)+1)
	if err := validateGoMachO(shortReaderAt{ReaderAt: bytes.NewReader(image)}, int64(len(image))); err == nil {
		t.Fatal("short ReaderAt read was admitted")
	}
}

type panickingReaderAt struct{}

func (panickingReaderAt) ReadAt([]byte, int64) (int, error) {
	panic("hostile ReaderAt")
}

type shortReaderAt struct {
	io.ReaderAt
}

func (reader shortReaderAt) ReadAt(buffer []byte, offset int64) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	n, _ := reader.ReaderAt.ReadAt(buffer[:len(buffer)-1], offset)
	return n, errors.New("short read")
}

type machOCommandCase struct {
	name     string
	commands func() [][]byte
	target   int
	prepare  func([]byte)
}

func allowedMachOCommandCases() []machOCommandCase {
	baseCase := func(name string, target int) machOCommandCase {
		return machOCommandCase{name: name, commands: baseMachOCommands, target: target, prepare: func([]byte) {}}
	}
	extra := func(name string, command func() []byte) machOCommandCase {
		return machOCommandCase{
			name: name,
			commands: func() [][]byte {
				return append(baseMachOCommands(), command())
			},
			target:  3,
			prepare: func([]byte) {},
		}
	}
	cases := []machOCommandCase{
		baseCase("segment", 0),
		baseCase("dylinker", 1),
		baseCase("main", 2),
		extra("uuid", func() []byte { return makeFixedCommand(machOLCUUID, 24) }),
		extra("code signature", func() []byte { return makeLinkeditCommand(machOLCCodeSignature, 7000, 1) }),
		extra("dyld info", func() []byte { return makeFixedCommand(machOLCDyldInfoOnly, 48) }),
		extra("function starts", func() []byte { return makeLinkeditCommand(machOLCFunctionStarts, 7000, 1) }),
		extra("data in code", func() []byte { return makeLinkeditCommand(machOLCDataInCode, 7000, 1) }),
		extra("source version", func() []byte { return makeFixedCommand(machOLCSourceVersion, 16) }),
		extra("build version", func() []byte { return makeBuildVersionCommand(1) }),
		extra("exports trie", func() []byte { return makeLinkeditCommand(machOLCDyldExportsTrie, 7000, 1) }),
		extra("chained fixups", func() []byte { return makeLinkeditCommand(machOLCDyldChainedFixups, 7000, 1) }),
	}
	for _, command := range []struct {
		name string
		code uint32
	}{
		{name: "load dylib", code: machOLCLoadDylib},
		{name: "load weak dylib", code: machOLCLoadWeakDylib},
		{name: "reexport dylib", code: machOLCReexportDylib},
		{name: "lazy load dylib", code: machOLCLazyLoadDylib},
		{name: "load upward dylib", code: machOLCLoadUpwardDylib},
	} {
		cases = append(cases, extra(command.name, func() []byte {
			return makeDylibCommand(command.code, "/usr/lib/"+strings.ReplaceAll(command.name, " ", "-")+".dylib")
		}))
	}
	cases = append(cases,
		machOCommandCase{
			name: "symtab",
			commands: func() [][]byte {
				return append(baseMachOCommands(), makeSymtabCommand(7000, 1, 7016, 1))
			},
			target:  3,
			prepare: prepareSymtabPayload,
		},
		machOCommandCase{
			name: "dysymtab",
			commands: func() [][]byte {
				return append(baseMachOCommands(), makeSymtabCommand(7000, 1, 7016, 1), makeDysymtabCommand())
			},
			target:  4,
			prepare: prepareSymtabPayload,
		},
	)
	return cases
}

func baseMachOCommands() [][]byte {
	return [][]byte{
		makeSegmentCommand("__TEXT", 0, machOFixtureSize, 0, machOFixtureSize, machOVMProtectionExecute, nil),
		makeDylinkerCommand("/usr/lib/dyld"),
		makeMainCommand(4096),
	}
}

func makeThinMachOFixture(t *testing.T, profile machOProfile, commands ...[]byte) []byte {
	t.Helper()
	commandBytes := 0
	for _, command := range commands {
		if len(command) < 8 || len(command)%8 != 0 {
			t.Fatalf("test command has invalid size %d", len(command))
		}
		commandBytes += len(command)
	}
	if 32+commandBytes > machOFixtureSize {
		t.Fatalf("test commands exceed fixture size: %d", commandBytes)
	}
	image := make([]byte, machOFixtureSize)
	putUint32(image, 0, machOMagic64)
	putUint32(image, 4, cpuTypeARM64)
	putUint32(image, 8, cpuSubtypeARM64All)
	putUint32(image, 12, machOFileExecute)
	putUint32(image, 16, len(commands))
	putUint32(image, 20, commandBytes)
	putUint32(image, 24, expectedMachOFlags(profile))
	offset := 32
	for _, command := range commands {
		copy(image[offset:], command)
		offset += len(command)
	}
	return image
}

func makeHeaderOnlyMachO(cpuType, cpuSubtype uint32, profile machOProfile) []byte {
	header := make([]byte, 32)
	putUint32(header, 0, machOMagic64)
	putUint32(header, 4, cpuType)
	putUint32(header, 8, cpuSubtype)
	putUint32(header, 12, machOFileExecute)
	putUint32(header, 24, expectedMachOFlags(profile))
	return header
}

type fatFixtureArchitecture struct {
	cpuType    uint32
	cpuSubtype uint32
	align      uint32
	image      []byte
}

func makeFatMachOFixture(t *testing.T, architectures []fatFixtureArchitecture) ([]byte, []int) {
	t.Helper()
	tableEnd := 8 + len(architectures)*20
	offsets := make([]int, len(architectures))
	cursor := tableEnd
	for index, architecture := range architectures {
		alignment := 1 << architecture.align
		cursor = (cursor + alignment - 1) &^ (alignment - 1)
		offsets[index] = cursor
		cursor += len(architecture.image)
	}
	image := make([]byte, cursor)
	binary.BigEndian.PutUint32(image[0:4], fatMagic32)
	binary.BigEndian.PutUint32(image[4:8], uint32(len(architectures)))
	rowOffsets := make([]int, len(architectures))
	for index, architecture := range architectures {
		row := 8 + index*20
		rowOffsets[index] = row
		binary.BigEndian.PutUint32(image[row:row+4], architecture.cpuType)
		binary.BigEndian.PutUint32(image[row+4:row+8], architecture.cpuSubtype)
		binary.BigEndian.PutUint32(image[row+8:row+12], uint32(offsets[index]))
		binary.BigEndian.PutUint32(image[row+12:row+16], uint32(len(architecture.image)))
		binary.BigEndian.PutUint32(image[row+16:row+20], architecture.align)
		copy(image[offsets[index]:], architecture.image)
	}
	return image, rowOffsets
}

func makeFixedCommand(code uint32, size int) []byte {
	command := make([]byte, size)
	putUint32(command, 0, code)
	putUint32(command, 4, size)
	return command
}

func extendMachOCommand(command []byte) []byte {
	extended := append(append([]byte(nil), command...), make([]byte, 8)...)
	putUint32(extended, 4, len(extended))
	return extended
}

func makeSegmentCommand(name string, vmAddress, vmSize, fileOffset, fileSize uint64, initialProtection uint32, sections [][]byte) []byte {
	command := makeFixedCommand(machOLCSegment64, 72+80*len(sections))
	copy(command[8:24], name)
	binary.LittleEndian.PutUint64(command[24:32], vmAddress)
	binary.LittleEndian.PutUint64(command[32:40], vmSize)
	binary.LittleEndian.PutUint64(command[40:48], fileOffset)
	binary.LittleEndian.PutUint64(command[48:56], fileSize)
	putUint32(command, 60, initialProtection)
	putUint32(command, 64, len(sections))
	for index, section := range sections {
		copy(command[72+index*80:], section)
	}
	return command
}

func makeSection(address, size uint64, offset, relocationOffset, relocationCount, flags uint32) []byte {
	section := make([]byte, 80)
	binary.LittleEndian.PutUint64(section[32:40], address)
	binary.LittleEndian.PutUint64(section[40:48], size)
	putUint32(section, 48, offset)
	putUint32(section, 56, relocationOffset)
	putUint32(section, 60, relocationCount)
	putUint32(section, 64, flags)
	return section
}

func makeDylibCommand(code uint32, loadedPath string) []byte {
	return makePathMachOCommand(code, 24, loadedPath)
}

func makeDylinkerCommand(loadedPath string) []byte {
	return makePathMachOCommand(machOLCLoadDylinker, 12, loadedPath)
}

func makePathMachOCommand(code uint32, start int, loadedPath string) []byte {
	size := (start + len(loadedPath) + 1 + 7) &^ 7
	command := makeFixedCommand(code, size)
	putUint32(command, 8, start)
	copy(command[start:], loadedPath)
	return command
}

func makeMainCommand(entry uint64) []byte {
	command := makeFixedCommand(machOLCMain, 24)
	binary.LittleEndian.PutUint64(command[8:16], entry)
	return command
}

func makeSymtabCommand(symbolOffset, symbolCount, stringOffset, stringSize int) []byte {
	command := makeFixedCommand(machOLCSymtab, 24)
	putUint32(command, 8, symbolOffset)
	putUint32(command, 12, symbolCount)
	putUint32(command, 16, stringOffset)
	putUint32(command, 20, stringSize)
	return command
}

func prepareSymtabPayload(image []byte) {
	image[7016] = 0
}

func makeDysymtabCommand() []byte {
	return makeFixedCommand(machOLCDysymtab, 80)
}

func makeLinkeditCommand(code uint32, offset, size int) []byte {
	command := makeFixedCommand(code, 16)
	putUint32(command, 8, offset)
	putUint32(command, 12, size)
	return command
}

func makeBuildVersionCommand(toolCount uint32) []byte {
	command := makeFixedCommand(machOLCBuildVersion, 24+int(toolCount)*8)
	putUint32(command, 8, machOPlatformMacOS)
	putUint32(command, 12, 0x000f0000)
	putUint32(command, 16, 0x000f0100)
	putUint32(command, 20, toolCount)
	for index := range toolCount {
		offset := 24 + int(index)*8
		putUint32(command, offset, index+1)
		putUint32(command, offset+4, 0x00010002+int(index))
	}
	return command
}

func commandFileOffset(commands [][]byte, target int) int {
	offset := 32
	for index := range target {
		offset += len(commands[index])
	}
	return offset
}

func putUint32(buffer []byte, offset int, value any) {
	var converted uint32
	switch value := value.(type) {
	case int:
		converted = uint32(value)
	case uint32:
		converted = value
	default:
		panic("unsupported test integer")
	}
	binary.LittleEndian.PutUint32(buffer[offset:offset+4], converted)
}

func assertInvalidMachO(t *testing.T, image []byte, profile machOProfile) {
	t.Helper()
	if err := validateMachO(bytes.NewReader(image), int64(len(image)), profile); err == nil {
		t.Fatal("malformed Mach-O was admitted")
	}
}

func assertInvalidMachODetail(t *testing.T, image []byte, profile machOProfile, want string, sizes ...int64) {
	t.Helper()
	size := int64(len(image))
	if len(sizes) != 0 {
		size = sizes[0]
	}
	err := validateMachO(bytes.NewReader(image), size, profile)
	var malformed *machOValidationError
	if !errors.As(err, &malformed) {
		t.Fatalf("error = %T %v, want Mach-O detail %q", err, err, want)
	}
	if malformed.detail != want {
		t.Fatalf("detail = %q, want %q", malformed.detail, want)
	}
}
