// codex-hook-json provides the small, fixed JSON surface used by attested
// Codex hooks without trusting a package-manager jq binary.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	raw, nullInput, variables, filter, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "codex-hook-json: %v\n", err)
		return 2
	}
	var value any
	if nullInput {
		value, err = constructOutput(normalizeFilter(filter), variables)
	} else {
		var document any
		decoder := json.NewDecoder(stdin)
		decoder.UseNumber()
		if err = decoder.Decode(&document); err == nil {
			err = requireEOF(decoder)
		}
		if err == nil {
			value, err = extractInput(normalizeFilter(filter), document)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "codex-hook-json: %v\n", err)
		return 2
	}
	if value == nil {
		return 0
	}
	if raw {
		switch typed := value.(type) {
		case string:
			fmt.Fprintln(stdout, typed)
		case bool:
			fmt.Fprintln(stdout, typed)
		default:
			encoded, marshalErr := json.Marshal(typed)
			if marshalErr != nil {
				fmt.Fprintf(stderr, "codex-hook-json: encode raw value: %v\n", marshalErr)
				return 2
			}
			fmt.Fprintln(stdout, string(encoded))
		}
		return 0
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "codex-hook-json: encode output: %v\n", err)
		return 2
	}
	return 0
}

func parseArgs(args []string) (raw, nullInput bool, variables map[string]string, filter string, err error) {
	variables = make(map[string]string)
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "-r":
			raw = true
		case "-n", "-cn", "-nc":
			nullInput = true
		case "-c":
		case "--arg":
			if index+2 >= len(args) {
				return false, false, nil, "", fmt.Errorf("--arg requires a name and value")
			}
			variables[args[index+1]] = args[index+2]
			index += 2
		default:
			if filter != "" {
				return false, false, nil, "", fmt.Errorf("unexpected argument %q", args[index])
			}
			filter = args[index]
		}
	}
	if filter == "" {
		return false, false, nil, "", fmt.Errorf("a supported filter is required")
	}
	return raw, nullInput, variables, filter, nil
}

func normalizeFilter(filter string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, filter)
}

func extractInput(filter string, document any) (any, error) {
	switch filter {
	case ".tool_name//empty":
		return nestedString(document, "tool_name"), nil
	case ".tool_input.command//empty":
		return nestedString(document, "tool_input", "command"), nil
	case ".tool_input.file_path//empty":
		return nestedString(document, "tool_input", "file_path"), nil
	case ".hook_event_name//empty":
		return nestedString(document, "hook_event_name"), nil
	case ".session_id//empty":
		return nestedString(document, "session_id"), nil
	case ".cwd//empty":
		return nestedString(document, "cwd"), nil
	case ".stop_hook_active//false":
		value := nested(document, "stop_hook_active")
		if boolean, ok := value.(bool); ok {
			return boolean, nil
		}
		return false, nil
	case `.tool_inputas$t|[($t.content//empty),($t.new_string//empty),(($t.edits//[])[]|.new_string//empty)]|join("\n")`:
		return combinedEditText(document), nil
	case `if(.tool_input|type)=="string"then.tool_inputelse(.tool_input.patch//.tool_input.input//.tool_input.content//empty)end`:
		return patchText(document), nil
	default:
		return nil, fmt.Errorf("unsupported input filter %q", filter)
	}
}

func constructOutput(filter string, variables map[string]string) (any, error) {
	switch filter {
	case `{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"deny",additionalContext:$msg}}`:
		return map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PreToolUse", "permissionDecision": "deny", "additionalContext": variables["msg"],
		}}, nil
	case `{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$msg}}`:
		return preToolContext(variables["msg"]), nil
	case `{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}`:
		return preToolContext(variables["c"]), nil
	case `{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$m},permissionDecision:"deny",denialReason:$m}`:
		return map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName": "PreToolUse", "additionalContext": variables["m"],
			},
			"permissionDecision": "deny",
			"denialReason":       variables["m"],
		}, nil
	case `{decision:"block",reason:$r}`:
		return map[string]any{"decision": "block", "reason": variables["r"]}, nil
	case `{hookSpecificOutput:{hookEventName:$e,additionalContext:$c}}`:
		return map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": variables["e"], "additionalContext": variables["c"],
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported output filter %q", filter)
	}
}

func preToolContext(message string) map[string]any {
	return map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "additionalContext": message,
	}}
}

func nested(document any, keys ...string) any {
	current := document
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[key]
	}
	return current
}

func nestedString(document any, keys ...string) any {
	value := nested(document, keys...)
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return nil
}

func combinedEditText(document any) string {
	input, _ := nested(document, "tool_input").(map[string]any)
	if input == nil {
		return ""
	}
	var parts []string
	for _, key := range []string{"content", "new_string"} {
		if text, ok := input[key].(string); ok && text != "" {
			parts = append(parts, text)
		}
	}
	if edits, ok := input["edits"].([]any); ok {
		for _, rawEdit := range edits {
			edit, _ := rawEdit.(map[string]any)
			if text, ok := edit["new_string"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func patchText(document any) any {
	input := nested(document, "tool_input")
	if text, ok := input.(string); ok {
		if text == "" {
			return nil
		}
		return text
	}
	object, _ := input.(map[string]any)
	for _, key := range []string{"patch", "input", "content"} {
		if text, ok := object[key].(string); ok && text != "" {
			return text
		}
	}
	return nil
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}
