package validator

import "testing"

func TestDefaultConfig_AllBlock(t *testing.T) {
	cfg := DefaultConfig()
	categories := []Category{
		CategoryFileOps, CategoryForLoop, CategoryRedirection, CategoryEchoPrintf,
	}
	for _, cat := range categories {
		if m := cfg.CategoryMode(cat); m != ModeBlock {
			t.Errorf("DefaultConfig().CategoryMode(%s) = %s, want %s", cat, m, ModeBlock)
		}
	}
}

func TestDangerousAlwaysBlocks(t *testing.T) {
	cfg := &Config{
		Modes: map[Category]Mode{
			CategoryDangerous: ModeWarn, // attempt to set dangerous to warn
		},
	}
	if m := cfg.CategoryMode(CategoryDangerous); m != ModeBlock {
		t.Errorf("CategoryMode(DANGEROUS) = %s, want block (dangerous must always block)", m)
	}
}

func TestPatternCategory_KnownPatterns(t *testing.T) {
	tests := []struct {
		idx  int
		want Category
	}{
		{10, CategoryFileOps},     // find
		{20, CategoryFileOps},     // ls
		{21, CategoryFileOps},     // grep/rg
		{22, CategoryFileOps},     // cat
		{23, CategoryFileOps},     // head/tail
		{24, CategoryFileOps},     // sed standalone
		{25, CategoryFileOps},     // awk
		{2, CategoryForLoop},      // while loop
		{7, CategoryRedirection},  // redirection to system path
		{27, CategoryRedirection}, // command substitution
		{26, CategoryEchoPrintf},  // echo/printf
	}
	for _, tt := range tests {
		got := PatternCategory(tt.idx)
		if got != tt.want {
			t.Errorf("PatternCategory(%d) = %s, want %s", tt.idx, got, tt.want)
		}
	}
}

func TestPatternCategory_DangerousDefault(t *testing.T) {
	// Patterns not in the map should be DANGEROUS
	dangerousIndices := []int{0, 1, 3, 4, 5, 6, 8, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	for _, idx := range dangerousIndices {
		got := PatternCategory(idx)
		if got != CategoryDangerous {
			t.Errorf("PatternCategory(%d) = %s, want %s", idx, got, CategoryDangerous)
		}
	}
}

func TestValidateCommandWithConfig_WarnMode(t *testing.T) {
	warnFileOps := &Config{
		Modes: map[Category]Mode{
			CategoryFileOps:     ModeWarn,
			CategoryForLoop:     ModeBlock,
			CategoryRedirection: ModeBlock,
			CategoryEchoPrintf:  ModeBlock,
		},
	}

	tests := []struct {
		name        string
		cmd         string
		cfg         *Config
		wantAllowed bool
		wantMode    Mode
		wantPattern string
	}{
		{
			name:        "ls warns in warn mode",
			cmd:         "ls -la /path",
			cfg:         warnFileOps,
			wantAllowed: true,
			wantMode:    ModeWarn,
			wantPattern: "ls (standalone)",
		},
		{
			name:        "cat warns in warn mode",
			cmd:         "cat file.txt",
			cfg:         warnFileOps,
			wantAllowed: true,
			wantMode:    ModeWarn,
			wantPattern: "cat (standalone)",
		},
		{
			name:        "grep warns in warn mode",
			cmd:         "grep pattern file",
			cfg:         warnFileOps,
			wantAllowed: true,
			wantMode:    ModeWarn,
			wantPattern: "grep/rg (standalone)",
		},
		{
			name:        "echo still blocks (not in warn config)",
			cmd:         "echo hello",
			cfg:         warnFileOps,
			wantAllowed: false,
			wantMode:    ModeBlock,
			wantPattern: "echo/printf (standalone)",
		},
		{
			name:        "cd always blocks (dangerous)",
			cmd:         "cd /path",
			cfg:         warnFileOps,
			wantAllowed: false,
			wantMode:    ModeBlock,
			wantPattern: "cd command",
		},
		{
			name:        "rm -rf always blocks (dangerous)",
			cmd:         "rm -rf /tmp/dir",
			cfg:         warnFileOps,
			wantAllowed: false,
			wantMode:    ModeBlock,
			wantPattern: "recursive rm (rm -r / rm -rf)",
		},
		{
			name:        "git --no-verify always blocks (dangerous)",
			cmd:         "git commit --no-verify",
			cfg:         warnFileOps,
			wantAllowed: false,
			wantMode:    ModeBlock,
			wantPattern: "git --no-verify flag (hook bypass)",
		},
		{
			name:        "allowed command returns empty mode",
			cmd:         "go build ./...",
			cfg:         warnFileOps,
			wantAllowed: true,
			wantMode:    "",
			wantPattern: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ValidateCommandWithConfig(tt.cmd, tt.cfg)
			if r.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", r.Allowed, tt.wantAllowed)
			}
			if r.Mode != tt.wantMode {
				t.Errorf("Mode = %s, want %s", r.Mode, tt.wantMode)
			}
			if r.PatternName != tt.wantPattern {
				t.Errorf("PatternName = %q, want %q", r.PatternName, tt.wantPattern)
			}
		})
	}
}

func TestValidateCommandWithConfig_AllWarn(t *testing.T) {
	allWarn := &Config{
		Modes: map[Category]Mode{
			CategoryFileOps:     ModeWarn,
			CategoryForLoop:     ModeWarn,
			CategoryRedirection: ModeWarn,
			CategoryEchoPrintf:  ModeWarn,
		},
	}

	// Soft-warnable patterns should be allowed with warn mode
	warnCmds := []struct {
		cmd  string
		name string
	}{
		{"ls -la", "ls"},
		{"cat file.txt", "cat"},
		{"grep pattern file", "grep"},
		{"head -n 5 file", "head"},
		{"tail -n 5 file", "tail"},
		{"sed 's/a/b/' file", "sed"},
		{"awk '{print}' file", "awk"},
		{"find . -name '*.go'", "find"},
		{"while true; do sleep 1; done", "while loop (semicolon catches first)"},
		{"echo hello", "echo"},
	}

	for _, tc := range warnCmds {
		t.Run(tc.name, func(t *testing.T) {
			r := ValidateCommandWithConfig(tc.cmd, allWarn)
			if !r.Allowed {
				t.Errorf("cmd %q should be allowed in all-warn mode, but was blocked (pattern: %s)", tc.cmd, r.PatternName)
			}
			// Should have warn mode set (pattern matched but allowed)
			if r.PatternName != "" && r.Mode != ModeWarn {
				t.Errorf("cmd %q matched pattern %q but mode = %s, want warn", tc.cmd, r.PatternName, r.Mode)
			}
		})
	}

	// Dangerous patterns should still block even in all-warn config
	blockCmds := []struct {
		cmd  string
		name string
	}{
		{"cd /path", "cd"},
		{"rm -rf /tmp", "rm -rf"},
		{"git commit --no-verify", "no-verify"},
		{"git checkout main", "checkout main"},
		{"git stash", "stash"},
	}

	for _, tc := range blockCmds {
		t.Run("dangerous_"+tc.name, func(t *testing.T) {
			r := ValidateCommandWithConfig(tc.cmd, allWarn)
			if r.Allowed {
				t.Errorf("dangerous cmd %q should still block in all-warn mode", tc.cmd)
			}
			if r.Mode != ModeBlock {
				t.Errorf("dangerous cmd %q mode = %s, want block", tc.cmd, r.Mode)
			}
		})
	}
}

func TestValidateCommandWithConfig_NilConfig(t *testing.T) {
	// nil config should behave like DefaultConfig (all block)
	r := ValidateCommandWithConfig("ls -la", nil)
	if r.Allowed {
		t.Error("nil config should block ls (default is all-block)")
	}
	if r.Mode != ModeBlock {
		t.Errorf("Mode = %s, want block", r.Mode)
	}
}

func TestValidateCommand_BackwardsCompatible(t *testing.T) {
	// Ensure the original ValidateCommand still works the same way
	ok, name, _ := ValidateCommand("ls -la")
	if ok {
		t.Error("ValidateCommand should block ls")
	}
	if name != "ls (standalone)" {
		t.Errorf("pattern name = %q, want 'ls (standalone)'", name)
	}

	ok, name, _ = ValidateCommand("go build ./...")
	if !ok {
		t.Error("ValidateCommand should allow go build")
	}
	if name != "" {
		t.Errorf("pattern name = %q, want empty", name)
	}
}
