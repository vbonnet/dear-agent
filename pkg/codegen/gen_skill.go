package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const skillTemplate = `---
description: {{.Description}}
argument-hint: "{{.ArgumentHint}}"
allowed-tools: {{.AllowedTools}}
---

# {{.Title}}

I'll {{.ActionVerb}}.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract:
{{range .Fields}}  - ` + "`" + `--{{.FlagName}}` + "`" + `{{if .Default}} (default: {{.Default}}){{end}}{{if .Required}} (required){{end}}: {{.Description}}
{{end}}

**Step 2: Run command**

- Run: ` + "`" + `{{.CLICommand}}{{range .PositionalArgs}} {{.}}{{end}} --output json` + "`" + `
- Forward supplied optional flags as separate argv values. Never construct shell
  syntax by concatenating or interpolating user-controlled text.

**Step 3: Handle result**

- If exit code is not 0:
  - Show error message from stderr
  - Suggest troubleshooting steps
  - Exit gracefully

**Step 4: Display results**

{{if .OutputTable}}Parse the JSON and display a table with columns:

| {{range .OutputTable}}{{.}} | {{end}}
|{{range .OutputTable}} --- |{{end}}
{{else}}Parse JSON output and display formatted results to the user.
{{end}}

**Error Handling**:
- If no results found: suggest broadening filters
- If service error: suggest diagnostic command
`

type skillTemplateData struct {
	Description    string
	ArgumentHint   string
	AllowedTools   string
	Title          string
	ActionVerb     string
	CLICommand     string
	PositionalArgs []string
	Fields         []FieldIR
	OutputTable    []string
}

// GenerateSkills produces markdown skill files for all ops with Skill surfaces and ManualSkill=false.
func GenerateSkills(ops []OpIR, outDir, cliBinary string) error {
	skillOps := filterOps(ops, func(o OpIR) bool {
		return o.Op.Skill != nil && !o.Op.ManualSkill
	})
	if len(skillOps) == 0 {
		return nil
	}
	if cliBinary == "" {
		return fmt.Errorf("CLI binary is required when generating skill invocations")
	}

	skillsDir := filepath.Join(outDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o700); err != nil {
		return err
	}

	tmpl, err := template.New("skill").Parse(skillTemplate)
	if err != nil {
		return err
	}

	for _, op := range skillOps {
		if err := validateSkillAllowedTools(op.Op.Skill.AllowedTools); err != nil {
			return fmt.Errorf("skill %s: %w", op.Op.Skill.SlashCommand, err)
		}
		data := buildSkillData(op, cliBinary)
		outPath := filepath.Join(skillsDir, op.Op.Skill.SlashCommand+".md")

		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, []byte(buf.String()), 0o600); err != nil {
			return err
		}
	}

	return nil
}

func validateSkillAllowedTools(allowedTools string) error {
	if strings.Contains(allowedTools, ":*)") {
		return fmt.Errorf("allowed-tools uses retired colon command syntax")
	}
	return nil
}

func buildSkillData(op OpIR, cliBinary string) skillTemplateData {
	fields := op.SkillFields()
	var positionalArgs []string

	// Build argument hint
	var hints []string
	for _, f := range op.PositionalFields() {
		if f.OmitSkill || f.Hidden {
			continue
		}
		positional := "<" + f.FlagName + ">"
		hints = append(hints, positional)
		positionalArgs = append(positionalArgs, positional)
	}
	for _, f := range fields {
		if f.IsPositional {
			continue
		}
		hint := "--" + f.FlagName
		if len(f.Enum) > 0 {
			hint += " " + strings.Join(f.Enum, "|")
		} else {
			switch f.FlagType {
			case "Int":
				hint += " N"
			case "Bool":
				// no value
			default:
				hint += " <value>"
			}
		}
		hints = append(hints, "["+hint+"]")
	}

	// Build CLI command path
	cliCmd := op.Op.CLI.CommandPath
	if cliCmd == "" {
		cliCmd = op.Op.Name
	}
	if cliBinary != "" {
		cliCmd = cliBinary + " " + cliCmd
	}

	// Capitalize first letter of ActionVerb for description
	desc := op.Op.Skill.ActionVerb
	if len(desc) > 0 {
		desc = strings.ToUpper(desc[:1]) + desc[1:]
	}

	return skillTemplateData{
		Description:    desc,
		ArgumentHint:   strings.Join(hints, " "),
		AllowedTools:   op.Op.Skill.AllowedTools,
		Title:          op.Op.Description,
		ActionVerb:     op.Op.Skill.ActionVerb,
		CLICommand:     cliCmd,
		PositionalArgs: positionalArgs,
		Fields:         fields,
		OutputTable:    op.Op.Skill.OutputTable,
	}
}
