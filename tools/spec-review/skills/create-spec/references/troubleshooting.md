# Troubleshooting

## Issue: "Project path does not exist"

**Cause:** Invalid project path provided

**Solution:**
```bash
# Use absolute path
python create_spec.py /absolute/path/to/project

# Or relative from current directory
python create_spec.py ./my-project
```

## Issue: "Template not found"

**Cause:** Custom template path is incorrect

**Solution:**
```bash
# Check template exists
ls -la templates/my-template.md

# Use correct path
python create_spec.py /path/to/project --template templates/my-template.md
```

## Issue: Low validation score

**Cause:** Generated SPEC has many placeholders or incomplete sections

**Solution:**
```bash
# Use interactive mode for better answers
python create_spec.py /path/to/project --interactive

# Review and fill in TBD content
# Re-run validation with /review-spec
```

## Issue: "No module named 'lib'"

**Cause:** Python path issue

**Solution:**
```bash
# Run from skill directory
cd skills/create-spec/
python create_spec.py /path/to/project

# Or use CLI adapter
python cli-adapters/claude-code.py /path/to/project
```
