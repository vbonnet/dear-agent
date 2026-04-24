// configure-claude-settings manages ~/.claude/settings.json entries.
//
// Usage:
//
//	configure-claude-settings set 'hooks.PreToolUse[+]' '{"hooks":[{"type":"command","command":"~/.claude/hooks/my-hook","timeout":5}]}'
//	configure-claude-settings set 'enabledPlugins.engram@engram' 'true'
//	configure-claude-settings remove 'hooks.PreToolUse' --match-command '~/.claude/hooks/my-hook'
//	configure-claude-settings remove 'enabledPlugins.engram@engram'
//	configure-claude-settings get 'hooks'
//	configure-claude-settings validate
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "set":
		if err := runSet(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "remove":
		if err := runRemove(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "get":
		if err := runGet(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "validate":
		if err := runValidate(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "add-hook":
		if err := runAddHook(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "remove-hook":
		if err := runRemoveHook(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "add-plugin":
		if err := runAddPlugin(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "remove-plugin":
		if err := runRemovePlugin(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "cleanup-dirs":
		if err := runCleanupDirs(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "--help", "-h", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`configure-claude-settings — Manage ~/.claude/settings.json

Commands:
  set <key> <json-value>      Set a key (dot-separated path). Use [+] suffix to append to array.
  remove <key>                Remove a key. Use --match-command <cmd> to remove specific hook entries.
  get [key]                   Get value at key (or whole file if no key).
  validate                    Validate settings.json is valid JSON.
  add-hook <event> <command>  Add a hook entry (idempotent). Flags: --timeout N, --matcher M
  remove-hook <event> <cmd>   Remove a hook by command path.
  add-plugin <name>           Enable a plugin (e.g., "engram@engram").
  remove-plugin <name>        Disable a plugin.
  cleanup-dirs                Remove additionalDirectories entries pointing to non-existent paths.

Options:
  --file <path>               Use alternate settings file (default: ~/.claude/settings.json)
  --dry-run                   Show what would change without writing.

Examples:
  configure-claude-settings add-hook PreToolUse ~/.claude/hooks/my-hook --timeout 5
  configure-claude-settings remove-hook PreToolUse ~/.claude/hooks/my-hook
  configure-claude-settings add-plugin "engram@engram"
  configure-claude-settings remove-plugin "engram@engram"
  configure-claude-settings validate`)
}

// --- Settings file I/O ---

func settingsPath(args []string) string {
	for i, a := range args {
		if a == "--file" && i+1 < len(args) {
			return args[i+1]
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	}
	return filepath.Join(home, ".claude", "settings.json")
}

func isDryRun(args []string) bool {
	for _, a := range args {
		if a == "--dry-run" {
			return true
		}
	}
	return false
}

// filterFlags removes --flag and --flag value pairs from args, returning positional args only.
func filterFlags(args []string) []string {
	var pos []string
	skip := false
	for i, a := range args {
		if skip {
			skip = false
			continue
		}
		if strings.HasPrefix(a, "--") {
			// Check if this flag takes a value
			switch a {
			case "--file", "--timeout", "--matcher", "--match-command":
				skip = true // skip next arg (the value)
			}
			continue
		}
		_ = i
		pos = append(pos, a)
	}
	return pos
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func readSettings(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func writeSettings(path string, settings map[string]interface{}, dryRun bool) error {
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	output = append(output, '\n') // POSIX newline

	if dryRun {
		fmt.Printf("[dry-run] Would write to %s:\n%s", path, string(output))
		return nil
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(path, output, 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// --- Commands ---

func runSet(args []string) error {
	path := settingsPath(args)
	dryRun := isDryRun(args)
	pos := filterFlags(args)

	if len(pos) < 2 {
		return fmt.Errorf("usage: set <key> <json-value>")
	}
	key := pos[0]
	valueStr := pos[1]

	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	// Parse the value as JSON
	var value interface{}
	if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
		// Treat as raw string
		value = valueStr
	}

	// Handle [+] suffix for array append
	if strings.HasSuffix(key, "[+]") {
		key = strings.TrimSuffix(key, "[+]")
		existing := getNestedValue(settings, key)
		var arr []interface{}
		if existArr, ok := existing.([]interface{}); ok {
			arr = existArr
		}
		arr = append(arr, value)
		value = arr
	}

	setNestedValue(settings, key, value)

	if err := writeSettings(path, settings, dryRun); err != nil {
		return err
	}
	if !dryRun {
		fmt.Printf("Set %s in %s\n", key, path)
	}
	return nil
}

func runRemove(args []string) error {
	path := settingsPath(args)
	dryRun := isDryRun(args)
	matchCmd := flagValue(args, "--match-command")
	pos := filterFlags(args)

	if len(pos) < 1 {
		return fmt.Errorf("usage: remove <key> [--match-command <cmd>]")
	}
	key := pos[0]

	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	if matchCmd != "" {
		// Remove specific hook entry by command from an array
		removed := removeHookByCommand(settings, key, matchCmd)
		if !removed {
			fmt.Printf("No hook with command %q found under %s\n", matchCmd, key)
			return nil
		}
	} else {
		removeNestedKey(settings, key)
	}

	if err := writeSettings(path, settings, dryRun); err != nil {
		return err
	}
	if !dryRun {
		fmt.Printf("Removed %s from %s\n", key, path)
	}
	return nil
}

func runGet(args []string) error {
	path := settingsPath(args)
	pos := filterFlags(args)

	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	var value interface{} = settings
	if len(pos) > 0 && pos[0] != "" {
		value = getNestedValue(settings, pos[0])
		if value == nil {
			return fmt.Errorf("key %q not found", pos[0])
		}
	}

	output, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
}

func runValidate() error {
	path := settingsPath(os.Args[2:])
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%s does not exist (valid — will be created on first use)\n", path)
			return nil
		}
		return err
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	fmt.Printf("%s is valid JSON\n", path)
	return nil
}

func runAddHook(args []string) error {
	path := settingsPath(args)
	dryRun := isDryRun(args)
	timeout := flagValue(args, "--timeout")
	matcher := flagValue(args, "--matcher")
	pos := filterFlags(args)

	if len(pos) < 2 {
		return fmt.Errorf("usage: add-hook <event> <command> [--timeout N] [--matcher M]")
	}
	event := pos[0]
	command := pos[1]

	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	// Get or create hooks map
	hooksMap, _ := settings["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = make(map[string]interface{})
	}

	// Check for duplicate
	if hookExists(hooksMap, event, command) {
		fmt.Printf("Hook already registered: %s → %s\n", event, command)
		return nil
	}

	// Build hook entry
	hookEntry := map[string]interface{}{
		"command": command,
		"type":    "command",
	}
	if timeout != "" {
		var t int
		if _, err := fmt.Sscanf(timeout, "%d", &t); err == nil {
			hookEntry["timeout"] = t
		}
	}

	groupEntry := map[string]interface{}{
		"hooks": []interface{}{hookEntry},
	}
	if matcher != "" {
		groupEntry["matcher"] = matcher
	}

	// Append to event array
	var eventGroups []interface{}
	if existing, ok := hooksMap[event]; ok {
		if arr, ok := existing.([]interface{}); ok {
			eventGroups = arr
		}
	}
	eventGroups = append(eventGroups, groupEntry)
	hooksMap[event] = eventGroups
	settings["hooks"] = hooksMap

	if err := writeSettings(path, settings, dryRun); err != nil {
		return err
	}
	if !dryRun {
		fmt.Printf("Added hook: %s → %s\n", event, command)
	}
	return nil
}

func runRemoveHook(args []string) error {
	path := settingsPath(args)
	dryRun := isDryRun(args)
	pos := filterFlags(args)

	if len(pos) < 2 {
		return fmt.Errorf("usage: remove-hook <event> <command>")
	}
	event := pos[0]
	command := pos[1]

	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	hooksMap, _ := settings["hooks"].(map[string]interface{})
	if hooksMap == nil {
		fmt.Printf("No hooks configured\n")
		return nil
	}

	key := "hooks." + event
	removed := removeHookByCommand(settings, key, command)
	if !removed {
		fmt.Printf("Hook not found: %s → %s\n", event, command)
		return nil
	}

	if err := writeSettings(path, settings, dryRun); err != nil {
		return err
	}
	if !dryRun {
		fmt.Printf("Removed hook: %s → %s\n", event, command)
	}
	return nil
}

func runAddPlugin(args []string) error {
	path := settingsPath(args)
	dryRun := isDryRun(args)
	pos := filterFlags(args)

	if len(pos) < 1 {
		return fmt.Errorf("usage: add-plugin <name>")
	}
	name := pos[0]

	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	plugins, _ := settings["enabledPlugins"].(map[string]interface{})
	if plugins == nil {
		plugins = make(map[string]interface{})
	}

	if existing, ok := plugins[name]; ok && existing == true {
		fmt.Printf("Plugin already enabled: %s\n", name)
		return nil
	}

	plugins[name] = true
	settings["enabledPlugins"] = plugins

	if err := writeSettings(path, settings, dryRun); err != nil {
		return err
	}
	if !dryRun {
		fmt.Printf("Enabled plugin: %s\n", name)
	}
	return nil
}

func runRemovePlugin(args []string) error {
	path := settingsPath(args)
	dryRun := isDryRun(args)
	pos := filterFlags(args)

	if len(pos) < 1 {
		return fmt.Errorf("usage: remove-plugin <name>")
	}
	name := pos[0]

	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	plugins, _ := settings["enabledPlugins"].(map[string]interface{})
	if plugins == nil {
		fmt.Printf("No plugins configured\n")
		return nil
	}

	if _, ok := plugins[name]; !ok {
		fmt.Printf("Plugin not found: %s\n", name)
		return nil
	}

	delete(plugins, name)
	settings["enabledPlugins"] = plugins

	if err := writeSettings(path, settings, dryRun); err != nil {
		return err
	}
	if !dryRun {
		fmt.Printf("Disabled plugin: %s\n", name)
	}
	return nil
}

// CleanupDirsResult holds the outcome of a cleanup-dirs operation.
type CleanupDirsResult struct {
	Before  int
	After   int
	Removed []string
}

// cleanupDirs removes additionalDirectories entries pointing to non-existent paths.
// Exported as a function so it can be called from other packages (e.g., agm admin).
func cleanupDirs(settingsFile string, dryRun bool) (*CleanupDirsResult, error) {
	settings, err := readSettings(settingsFile)
	if err != nil {
		return nil, err
	}

	dirsRaw, ok := settings["additionalDirectories"]
	if !ok {
		return &CleanupDirsResult{}, nil
	}

	arr, ok := dirsRaw.([]interface{})
	if !ok {
		return &CleanupDirsResult{}, nil
	}

	valid := make([]interface{}, 0)
	var removed []string
	for _, entry := range arr {
		dir, ok := entry.(string)
		if !ok {
			valid = append(valid, entry)
			continue
		}
		// Expand ~ to home directory
		expanded := dir
		if strings.HasPrefix(expanded, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				expanded = filepath.Join(home, expanded[2:])
			}
		}
		if _, err := os.Stat(expanded); err != nil {
			removed = append(removed, dir)
		} else {
			valid = append(valid, entry)
		}
	}

	result := &CleanupDirsResult{
		Before:  len(arr),
		After:   len(valid),
		Removed: removed,
	}

	if len(removed) == 0 {
		return result, nil
	}

	settings["additionalDirectories"] = valid
	if err := writeSettings(settingsFile, settings, dryRun); err != nil {
		return nil, err
	}

	return result, nil
}

func runCleanupDirs(args []string) error {
	path := settingsPath(args)
	dry := isDryRun(args)

	result, err := cleanupDirs(path, dry)
	if err != nil {
		return err
	}

	if len(result.Removed) == 0 {
		fmt.Printf("No stale directories found (%d entries checked)\n", result.Before)
		return nil
	}

	action := "Removed"
	if dry {
		action = "Would remove"
	}
	fmt.Printf("%s %d stale director%s (%d → %d)\n",
		action, len(result.Removed),
		pluralSuffix(len(result.Removed), "y", "ies"),
		result.Before, result.After)
	for _, d := range result.Removed {
		fmt.Printf("  - %s\n", d)
	}
	return nil
}

func pluralSuffix(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// --- Nested key helpers ---

func getNestedValue(m map[string]interface{}, key string) interface{} {
	parts := strings.Split(key, ".")
	var current interface{} = m
	for _, part := range parts {
		cm, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = cm[part]
	}
	return current
}

func setNestedValue(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[part] = next
		}
		current = next
	}
}

func removeNestedKey(m map[string]interface{}, key string) {
	parts := strings.Split(key, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			delete(current, part)
			return
		}
		next, ok := current[part].(map[string]interface{})
		if !ok {
			return
		}
		current = next
	}
}

func hookExists(hooksMap map[string]interface{}, event, command string) bool {
	eventGroups, ok := hooksMap[event]
	if !ok {
		return false
	}
	arr, ok := eventGroups.([]interface{})
	if !ok {
		return false
	}
	for _, group := range arr {
		gm, ok := group.(map[string]interface{})
		if !ok {
			continue
		}
		hooks, ok := gm["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooks {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, ok := hm["command"].(string); ok && cmd == command {
				return true
			}
		}
	}
	return false
}

func removeHookByCommand(settings map[string]interface{}, dotPath, command string) bool {
	parts := strings.Split(dotPath, ".")
	// Navigate to parent
	var current interface{} = settings
	for _, part := range parts[:len(parts)-1] {
		cm, ok := current.(map[string]interface{})
		if !ok {
			return false
		}
		current = cm[part]
	}

	parentMap, ok := current.(map[string]interface{})
	if !ok {
		return false
	}
	eventKey := parts[len(parts)-1]
	arr, ok := parentMap[eventKey].([]interface{})
	if !ok {
		return false
	}

	var newArr []interface{}
	removed := false
	for _, group := range arr {
		gm, ok := group.(map[string]interface{})
		if !ok {
			newArr = append(newArr, group)
			continue
		}
		hooks, ok := gm["hooks"].([]interface{})
		if !ok {
			newArr = append(newArr, group)
			continue
		}
		var newHooks []interface{}
		for _, h := range hooks {
			hm, ok := h.(map[string]interface{})
			if !ok {
				newHooks = append(newHooks, h)
				continue
			}
			if cmd, ok := hm["command"].(string); ok && cmd == command {
				removed = true
				continue
			}
			newHooks = append(newHooks, h)
		}
		if len(newHooks) > 0 {
			gm["hooks"] = newHooks
			newArr = append(newArr, gm)
		}
	}

	if removed {
		if len(newArr) == 0 {
			delete(parentMap, eventKey)
		} else {
			parentMap[eventKey] = newArr
		}
	}
	return removed
}
