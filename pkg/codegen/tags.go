package codegen

import (
	"fmt"
	"strconv"
	"strings"
)

// FieldTag represents a parsed ef:"..." struct tag.
type FieldTag struct {
	Name     string
	Pos      int // -1 if not positional
	Short    string
	Default  string
	Enum     []string
	Required bool
	Flatten  bool
	Hidden   bool
	Omit     map[string]bool
}

// ParseFieldTag parses an ef:"..." struct tag value into a FieldTag.
//
// Grammar:
//
//	ef:"field_name[,option]*"
//
// Options:
//
//	pos=N         positional arg index (int)
//	short=X       single-letter short flag
//	default=V     default value (string)
//	enum=a|b|c    pipe-separated allowed values
//	required      boolean flag
//	flatten       boolean flag
//	hidden        boolean flag
//	omit=cli|mcp  pipe-separated surface names to omit from
func ParseFieldTag(tag string) (*FieldTag, error) {
	if tag == "" {
		return nil, fmt.Errorf("empty ef tag")
	}

	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		return nil, fmt.Errorf("empty field name in ef tag")
	}

	ft := &FieldTag{
		Name: parts[0],
		Pos:  -1,
		Omit: make(map[string]bool),
	}

	for _, opt := range parts[1:] {
		key, val, hasVal := strings.Cut(opt, "=")
		if err := applyFieldTagOption(ft, key, val, hasVal); err != nil {
			return nil, err
		}
	}

	return ft, nil
}

// applyFieldTagOption applies a single key[=val] option to ft.
//nolint:gocyclo // reason: linear switch over many tag options
func applyFieldTagOption(ft *FieldTag, key, val string, hasVal bool) error {
	switch key {
	case "pos":
		if !hasVal {
			return fmt.Errorf("pos option requires a value")
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("pos value must be an integer: %w", err)
		}
		ft.Pos = n
	case "short":
		if !hasVal || len(val) != 1 {
			return fmt.Errorf("short option requires a single character")
		}
		ft.Short = val
	case "default":
		if !hasVal {
			return fmt.Errorf("default option requires a value")
		}
		ft.Default = val
	case "enum":
		if !hasVal || val == "" {
			return fmt.Errorf("enum option requires pipe-separated values")
		}
		ft.Enum = strings.Split(val, "|")
	case "required":
		ft.Required = true
	case "flatten":
		ft.Flatten = true
	case "hidden":
		ft.Hidden = true
	case "omit":
		if !hasVal || val == "" {
			return fmt.Errorf("omit option requires pipe-separated surface names")
		}
		for _, s := range strings.Split(val, "|") {
			ft.Omit[s] = true
		}
	default:
		return fmt.Errorf("unknown ef tag option: %q", key)
	}
	return nil
}

// ParseDescTag extracts the description from a desc:"..." struct tag value.
func ParseDescTag(tag string) string {
	return tag
}
