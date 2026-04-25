// Package enforcement detects anti-pattern violations in commands and content.
//
// It uses a pattern database of compiled regular expressions to identify
// unsafe or prohibited operations such as bypass flags, direct file
// modifications in read-only directories, and other policy violations.
// Both PCRE-style and RE2-compatible pattern sets are supported, allowing
// use in both general detection and PreToolUse hook contexts.
package enforcement
