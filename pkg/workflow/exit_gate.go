package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrExitGateFailed is the canonical error for an exit-gate failure.
// Wrapped with the gate's reason; the runner treats any wrapped
// instance as "block this node from succeeding".
var ErrExitGateFailed = errors.New("workflow: exit gate failed")

// ExitGateContext is the input the evaluator gets per node. The
// evaluator reads outputs (typed map; "report.path", "report.frontmatter.confidence")
// when a gate references them via Target. WorkflowDir is the directory
// of the workflow's YAML file — used to resolve relative Schema paths.
type ExitGateContext struct {
	NodeID      string
	RunID       string
	Inputs      map[string]string
	Outputs     map[string]any
	WorkflowDir string
	// Env is the workflow.env map passed through to bash gates so
	// declared environment variables resolve as the YAML author expects.
	Env map[string]string
}

// EvaluateExitGates runs gates in declared order and short-circuits on
// the first failure. Returns nil when every gate passes; returns a
// wrapped ErrExitGateFailed otherwise.
//
// The evaluator is deliberately small — five kinds, no plugin surface
// in v1. Each kind is implemented inline so future readers can audit
// the entire DOD logic in one file.
func EvaluateExitGates(ctx context.Context, gates []ExitGate, gctx ExitGateContext) error {
	for i, g := range gates {
		if err := evaluateGate(ctx, &g, gctx); err != nil {
			return fmt.Errorf("gate[%d] kind=%s: %w", i, g.Kind, err)
		}
	}
	return nil
}

func evaluateGate(ctx context.Context, g *ExitGate, gctx ExitGateContext) error {
	switch g.Kind {
	case GateBash:
		return evaluateBashGate(ctx, g, gctx)
	case GateTestCmd:
		return evaluateBashGate(ctx, g, gctx) // identical mechanics; the kind label is for the audit log
	case GateRegexMatch:
		return evaluateRegexGate(g, gctx)
	case GateJSONSchema:
		return evaluateJSONSchemaGate(g, gctx)
	case GateConfidenceScore:
		return evaluateConfidenceGate(g, gctx)
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrExitGateFailed, g.Kind)
	}
}

// evaluateBashGate runs g.Cmd via /bin/sh -c. Inputs and outputs are
// exposed as INPUT_* and OUTPUT_* env vars (matching the bash node
// convention) so gate scripts can read them without template
// interpolation risk.
func evaluateBashGate(ctx context.Context, g *ExitGate, gctx ExitGateContext) error {
	if g.Cmd == "" {
		return fmt.Errorf("%w: empty cmd", ErrExitGateFailed)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", g.Cmd)
	env := os.Environ()
	for k, v := range gctx.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range gctx.Inputs {
		env = append(env, envVarKey("INPUT_", k)+"="+v)
	}
	for k, v := range gctx.Outputs {
		env = append(env, envVarKey("OUTPUT_", k)+"="+stringifyOutput(v))
	}
	cmd.Env = env
	if gctx.WorkflowDir != "" {
		cmd.Dir = gctx.WorkflowDir
	}
	out, runErr := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()
	want := g.SuccessExit
	if exitCode == want {
		return nil
	}
	return fmt.Errorf("%w: exit=%d want=%d output=%s err=%w", ErrExitGateFailed, exitCode, want, strings.TrimSpace(string(out)), runErr)
}

// evaluateRegexGate compiles g.Pattern and matches against the value
// at g.Target. A pattern with a syntax error fails the gate with a
// pattern error so the YAML author sees the typo.
func evaluateRegexGate(g *ExitGate, gctx ExitGateContext) error {
	re, err := regexp.Compile(g.Pattern)
	if err != nil {
		return fmt.Errorf("%w: bad pattern %q: %w", ErrExitGateFailed, g.Pattern, err)
	}
	val, err := lookupTarget(g.Target, gctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrExitGateFailed, err)
	}
	str := stringifyOutput(val)
	if !re.MatchString(str) {
		return fmt.Errorf("%w: %q does not match %s", ErrExitGateFailed, str, g.Pattern)
	}
	return nil
}

// evaluateJSONSchemaGate is a minimal validator — covers the cases the
// research surfaced (type, required, enum, minimum). Anything more
// elaborate goes to a real schema lib in Phase 2.
//
// The schema file may be referenced by an absolute path or a path
// relative to gctx.WorkflowDir.
func evaluateJSONSchemaGate(g *ExitGate, gctx ExitGateContext) error {
	schemaPath := g.Schema
	if !filepath.IsAbs(schemaPath) && gctx.WorkflowDir != "" {
		schemaPath = filepath.Join(gctx.WorkflowDir, schemaPath)
	}
	raw, err := os.ReadFile(schemaPath) //nolint:gosec // schemaPath comes from operator-controlled YAML
	if err != nil {
		return fmt.Errorf("%w: read schema %s: %w", ErrExitGateFailed, schemaPath, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return fmt.Errorf("%w: parse schema: %w", ErrExitGateFailed, err)
	}
	val, err := lookupTarget(g.Target, gctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrExitGateFailed, err)
	}
	// Inputs come in as strings; let the validator interpret them as
	// JSON if the schema declares a non-string type.
	value := normaliseForSchema(val)
	if err := miniValidate(value, schema); err != nil {
		return fmt.Errorf("%w: %w", ErrExitGateFailed, err)
	}
	return nil
}

// evaluateConfidenceGate reads g.Target as a numeric value and checks
// that it is ≥ g.Min.
func evaluateConfidenceGate(g *ExitGate, gctx ExitGateContext) error {
	val, err := lookupTarget(g.Target, gctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrExitGateFailed, err)
	}
	num, err := toFloat(val)
	if err != nil {
		return fmt.Errorf("%w: target %q is not numeric: %w", ErrExitGateFailed, g.Target, err)
	}
	if num < g.Min {
		return fmt.Errorf("%w: confidence %v < min %v", ErrExitGateFailed, num, g.Min)
	}
	return nil
}

// lookupTarget resolves a dotted-path expression rooted at outputs or
// inputs. Examples:
//
//	outputs.report.path
//	outputs.report.frontmatter.confidence
//	inputs.target_dir
//
// The leading scope ("outputs" or "inputs") is case-insensitive. The
// remaining segments are walked through nested maps; a missing segment
// returns an error so the YAML author sees the typo.
func lookupTarget(target string, gctx ExitGateContext) (any, error) {
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("target %q must start with outputs.<key> or inputs.<key>", target)
	}
	scope := strings.ToLower(parts[0])
	parts = parts[1:]
	var cur any
	switch scope {
	case "outputs":
		cur = gctx.Outputs
	case "inputs":
		// Reify inputs as a map[string]any so the walker is uniform.
		m := make(map[string]any, len(gctx.Inputs))
		for k, v := range gctx.Inputs {
			m[k] = v
		}
		cur = m
	default:
		return nil, fmt.Errorf("unknown scope %q (expected outputs|inputs)", scope)
	}
	for _, p := range parts {
		switch m := cur.(type) {
		case map[string]any:
			next, ok := m[p]
			if !ok {
				return nil, fmt.Errorf("path %q: key %q not found", target, p)
			}
			cur = next
		case map[string]string:
			next, ok := m[p]
			if !ok {
				return nil, fmt.Errorf("path %q: key %q not found", target, p)
			}
			cur = next
		default:
			return nil, fmt.Errorf("path %q: cannot traverse into %T at %q", target, cur, p)
		}
	}
	return cur, nil
}

// stringifyOutput renders an output value as the bash gate sees it —
// scalars become their string form; structured values are JSON-encoded
// so the gate script can `jq` over them.
func stringifyOutput(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// toFloat coerces a value to float64. Strings are parsed; numeric
// types pass through. Anything else returns an error.
func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(x), 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float", v)
	}
}

// normaliseForSchema attempts to round-trip a string value through
// JSON so a schema declaring `type: number` can validate against an
// input string "0.7".
func normaliseForSchema(v any) any {
	if s, ok := v.(string); ok {
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			return parsed
		}
	}
	return v
}

// miniValidate is a tiny JSON Schema interpreter: handles `type`,
// `required`, `enum`, `minimum`, `maximum`, and `properties`. Enough
// for the smoke-test schemas the research surfaced; not a substitute
// for a real schema lib in Phase 2.
//
//nolint:gocyclo // straight-line schema dispatch is clearer inline
func miniValidate(value any, schema map[string]any) error {
	if t, ok := schema["type"].(string); ok {
		if err := checkSchemaType(value, t); err != nil {
			return err
		}
	}
	if enum, ok := schema["enum"].([]any); ok {
		matched := false
		for _, e := range enum {
			if equal(value, e) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("value %v not in enum %v", value, enum)
		}
	}
	if minv, ok := schema["minimum"]; ok {
		if mv, err := toFloat(minv); err == nil {
			if vv, err := toFloat(value); err == nil && vv < mv {
				return fmt.Errorf("value %v < minimum %v", vv, mv)
			}
		}
	}
	if maxv, ok := schema["maximum"]; ok {
		if mv, err := toFloat(maxv); err == nil {
			if vv, err := toFloat(value); err == nil && vv > mv {
				return fmt.Errorf("value %v > maximum %v", vv, mv)
			}
		}
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("value is not an object but properties were declared")
		}
		for key, sub := range props {
			subSchema, ok := sub.(map[string]any)
			if !ok {
				continue
			}
			if v, present := obj[key]; present {
				if err := miniValidate(v, subSchema); err != nil {
					return fmt.Errorf("property %q: %w", key, err)
				}
			}
		}
	}
	if req, ok := schema["required"].([]any); ok {
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("required declared but value is not an object")
		}
		for _, r := range req {
			name, _ := r.(string)
			if _, present := obj[name]; !present {
				return fmt.Errorf("required property %q missing", name)
			}
		}
	}
	return nil
}

func checkSchemaType(value any, want string) error {
	switch want {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("type %T != string", value)
		}
	case "number":
		if _, err := toFloat(value); err != nil {
			return fmt.Errorf("type %T != number", value)
		}
	case "integer":
		f, err := toFloat(value)
		if err != nil || f != float64(int64(f)) {
			return fmt.Errorf("type %T != integer", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("type %T != boolean", value)
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("type %T != object", value)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("type %T != array", value)
		}
	}
	return nil
}

func equal(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
