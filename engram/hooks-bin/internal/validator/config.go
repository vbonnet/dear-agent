package validator

// Category represents a group of related patterns that share enforcement policy.
type Category string

const (
	// CategoryFileOps groups file-reading tool patterns (ls, grep, cat, head/tail, sed, awk, find).
	CategoryFileOps Category = "FILE_OPERATIONS"
	// CategoryForLoop groups loop-related patterns (while loop).
	CategoryForLoop Category = "FOR_LOOP"
	// CategoryRedirection groups redirection patterns (redirection to system path, command substitution).
	CategoryRedirection Category = "REDIRECTION"
	// CategoryEchoPrintf groups echo/printf patterns.
	CategoryEchoPrintf Category = "ECHO_PRINTF"
	// CategoryDangerous marks patterns that are always hard-blocked.
	CategoryDangerous Category = "DANGEROUS"
)

// Mode controls how a category is enforced.
type Mode string

// Enforcement mode values for pattern categories.
const (
	ModeBlock Mode = "block"
	ModeWarn  Mode = "warn"
)

// Config holds the enforcement policy for each pattern category.
type Config struct {
	Modes map[Category]Mode
}

// DefaultConfig returns the default config where all categories block.
func DefaultConfig() *Config {
	return &Config{
		Modes: map[Category]Mode{
			CategoryFileOps:     ModeBlock,
			CategoryForLoop:     ModeBlock,
			CategoryRedirection: ModeBlock,
			CategoryEchoPrintf:  ModeBlock,
			CategoryDangerous:   ModeBlock,
		},
	}
}

// CategoryMode returns the enforcement mode for a category.
// Dangerous patterns always return ModeBlock regardless of config.
func (c *Config) CategoryMode(cat Category) Mode {
	if cat == CategoryDangerous {
		return ModeBlock
	}
	if m, ok := c.Modes[cat]; ok {
		return m
	}
	return ModeBlock
}

// patternCategory maps each pattern index to its category.
// Patterns not listed here default to CategoryDangerous.
var patternCategory = map[int]Category{
	// FILE_OPERATIONS: tool-redirect patterns
	10: CategoryFileOps, // find
	20: CategoryFileOps, // ls
	21: CategoryFileOps, // grep/rg
	22: CategoryFileOps, // cat
	23: CategoryFileOps, // head/tail
	24: CategoryFileOps, // sed (standalone)
	25: CategoryFileOps, // awk

	// FOR_LOOP
	2: CategoryForLoop, // while loop

	// REDIRECTION
	7:  CategoryRedirection, // redirection to system path
	27: CategoryRedirection, // command substitution $()

	// ECHO_PRINTF
	26: CategoryEchoPrintf, // echo/printf
}

// PatternCategory returns the category for a pattern index.
func PatternCategory(idx int) Category {
	if cat, ok := patternCategory[idx]; ok {
		return cat
	}
	return CategoryDangerous
}
