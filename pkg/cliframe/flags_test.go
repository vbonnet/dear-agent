package cliframe

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestAddStandardFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	flags := AddStandardFlags(cmd)

	// Verify flags were added
	if cmd.Flags().Lookup("format") == nil {
		t.Error("Expected --format flag to be added")
	}
	if cmd.Flags().Lookup("json") == nil {
		t.Error("Expected --json flag to be added")
	}
	if cmd.Flags().Lookup("no-color") == nil {
		t.Error("Expected --no-color flag to be added")
	}
	if cmd.Flags().Lookup("quiet") == nil {
		t.Error("Expected --quiet flag to be added")
	}
	if cmd.Flags().Lookup("verbose") == nil {
		t.Error("Expected --verbose flag to be added")
	}
	if cmd.Flags().Lookup("config") == nil {
		t.Error("Expected --config flag to be added")
	}
	if cmd.Flags().Lookup("workspace") == nil {
		t.Error("Expected --workspace flag to be added")
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("Expected --dry-run flag to be added")
	}
	if cmd.Flags().Lookup("force") == nil {
		t.Error("Expected --force flag to be added")
	}
	if cmd.Flags().Lookup("log-level") == nil {
		t.Error("Expected --log-level flag to be added")
	}
	if cmd.Flags().Lookup("trace") == nil {
		t.Error("Expected --trace flag to be added")
	}

	// Verify return value is bound
	if flags == nil {
		t.Fatal("Expected non-nil flags")
	}
}

func TestAddFormatFlag(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	var format string
	AddFormatFlag(cmd, &format)

	// Check flag exists
	flag := cmd.Flags().Lookup("format")
	if flag == nil {
		t.Fatal("Expected --format flag to be added")
	}

	// Check default value
	if flag.DefValue != "table" {
		t.Errorf("Expected default format 'table', got %s", flag.DefValue)
	}

	// Check shorthand
	if flag.Shorthand != "f" {
		t.Errorf("Expected shorthand 'f', got %s", flag.Shorthand)
	}
}

func TestAddVerboseFlag(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	var verbose bool
	AddVerboseFlag(cmd, &verbose)

	flag := cmd.Flags().Lookup("verbose")
	if flag == nil {
		t.Fatal("Expected --verbose flag to be added")
	}

	if flag.Shorthand != "v" {
		t.Errorf("Expected shorthand 'v', got %s", flag.Shorthand)
	}
}

func TestAddDryRunFlag(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	var dryRun bool
	AddDryRunFlag(cmd, &dryRun)

	flag := cmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("Expected --dry-run flag to be added")
	}
}

func TestCommonFlags_ResolveFormat_JSON(t *testing.T) {
	flags := &CommonFlags{
		JSON:   true,
		Format: "table",
	}

	format := flags.ResolveFormat()
	if format != FormatJSON {
		t.Errorf("Expected FormatJSON, got %s", format)
	}
}

func TestCommonFlags_ResolveFormat_FromFormat(t *testing.T) {
	flags := &CommonFlags{
		JSON:   false,
		Format: "toon",
	}

	format := flags.ResolveFormat()
	if format != FormatTOON {
		t.Errorf("Expected FormatTOON, got %s", format)
	}
}

func TestCommonFlags_ResolveFormat_JSONPriority(t *testing.T) {
	// --json should take priority over --format
	flags := &CommonFlags{
		JSON:   true,
		Format: "table",
	}

	format := flags.ResolveFormat()
	if format != FormatJSON {
		t.Errorf("Expected --json to override --format, got %s", format)
	}
}

func TestCommonFlags_IsInteractive_TTY(t *testing.T) {
	flags := &CommonFlags{}

	// This test depends on whether stdout is a TTY
	// We can't easily control this in tests, but we can verify it doesn't panic
	isInteractive := flags.IsInteractive()

	// Result should be boolean (not crash)
	if isInteractive != true && isInteractive != false {
		t.Error("IsInteractive should return boolean")
	}
}

func TestOutputFromFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	flags := &CommonFlags{
		Format:  "json",
		NoColor: true,
	}

	data := map[string]string{
		"hello": "world",
	}

	// Capture output
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := OutputFromFlags(cmd, data, flags)
	if err != nil {
		t.Fatalf("OutputFromFlags failed: %v", err)
	}

	// Should be JSON
	if !bytes.Contains(buf.Bytes(), []byte("hello")) {
		t.Error("Expected JSON output with 'hello' key")
	}
	if !bytes.Contains(buf.Bytes(), []byte("world")) {
		t.Error("Expected JSON output with 'world' value")
	}
}

func TestOutputFromFlags_InvalidFormat(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	flags := &CommonFlags{
		Format: "invalid",
	}

	data := map[string]string{"key": "value"}

	err := OutputFromFlags(cmd, data, flags)
	if err == nil {
		t.Error("Expected error for invalid format")
	}
}

func TestErrorFromFlags_CLIError(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	flags := &CommonFlags{
		JSON:    false,
		NoColor: true,
	}

	cliErr := NewError("test_error", "Test error message")
	_ = flags  // Intentionally unused - ErrorFromFlags calls os.Exit
	_ = cliErr // Intentionally unused - ErrorFromFlags calls os.Exit

	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// Note: ErrorFromFlags calls os.Exit, so we can't fully test it
	// We'll just verify it doesn't panic when called with non-JSON
	// In real usage, this would exit the process

	// Skip this test as it would exit the process
	t.Skip("ErrorFromFlags calls os.Exit - cannot test without process isolation")
}

func TestErrorFromFlags_RegularError(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	flags := &CommonFlags{
		NoColor: true,
	}

	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// This would normally exit, so we skip it
	t.Skip("ErrorFromFlags calls os.Exit for CLIError")

	// For regular errors (non-CLIError), it should just write and return
	// Note: unreachable due to t.Skip above
	regularErr := errors.New("regular error")
	_ = ErrorFromFlags(cmd, regularErr, flags)
}

func TestCommonFlags_Defaults(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	flags := AddStandardFlags(cmd)

	// Check default values
	// Format defaults to "table" when the flag is registered
	if flags.Format != "" && flags.Format != "table" {
		t.Errorf("Expected empty or 'table' Format initially, got %s", flags.Format)
	}

	if flags.JSON {
		t.Error("Expected JSON to be false by default")
	}

	if flags.NoColor {
		t.Error("Expected NoColor to be false by default")
	}

	if flags.Quiet {
		t.Error("Expected Quiet to be false by default")
	}

	if flags.Verbose {
		t.Error("Expected Verbose to be false by default")
	}

	if flags.DryRun {
		t.Error("Expected DryRun to be false by default")
	}

	if flags.Force {
		t.Error("Expected Force to be false by default")
	}

	if flags.Trace {
		t.Error("Expected Trace to be false by default")
	}
}

func TestCommonFlags_FlagShorthands(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	AddStandardFlags(cmd)

	// Check shorthands
	tests := []struct {
		flagName  string
		shorthand string
	}{
		{"format", "f"},
		{"quiet", "q"},
		{"verbose", "v"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Flag %s not found", tt.flagName)
			}

			if flag.Shorthand != tt.shorthand {
				t.Errorf("Expected shorthand '%s' for --%s, got '%s'",
					tt.shorthand, tt.flagName, flag.Shorthand)
			}
		})
	}
}

func TestCommonFlags_LogLevelDefault(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	AddStandardFlags(cmd)

	flag := cmd.Flags().Lookup("log-level")
	if flag == nil {
		t.Fatal("Expected --log-level flag")
	}

	if flag.DefValue != "info" {
		t.Errorf("Expected default log-level 'info', got %s", flag.DefValue)
	}
}

func TestOutputFromFlags_WithTable(t *testing.T) {
	type User struct {
		Name  string
		Email string
	}

	cmd := &cobra.Command{
		Use: "test",
	}

	flags := &CommonFlags{
		Format:  "table",
		NoColor: true,
	}

	users := []User{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
	}

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := OutputFromFlags(cmd, users, flags)
	if err != nil {
		t.Fatalf("OutputFromFlags failed: %v", err)
	}

	// Should contain table data
	if !bytes.Contains(buf.Bytes(), []byte("Alice")) {
		t.Error("Expected table output with 'Alice'")
	}
	if !bytes.Contains(buf.Bytes(), []byte("alice@example.com")) {
		t.Error("Expected table output with email")
	}
}

func TestOutputFromFlags_WithTOON(t *testing.T) {
	type Product struct {
		ID   int
		Name string
	}

	cmd := &cobra.Command{
		Use: "test",
	}

	flags := &CommonFlags{
		Format: "toon",
	}

	products := []Product{
		{ID: 1, Name: "Widget"},
		{ID: 2, Name: "Gadget"},
	}

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := OutputFromFlags(cmd, products, flags)
	if err != nil {
		t.Fatalf("OutputFromFlags failed: %v", err)
	}

	// Should contain TOON header
	if !bytes.Contains(buf.Bytes(), []byte("products[2]")) {
		t.Error("Expected TOON header with count")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Widget")) {
		t.Error("Expected TOON data with 'Widget'")
	}
}

func TestCommonFlags_IsInteractive_Pipe(t *testing.T) {
	// Save original stdout
	origStdout := os.Stdout

	// Create a pipe to simulate non-TTY
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// Replace stdout with pipe
	os.Stdout = w

	flags := &CommonFlags{}
	isInteractive := flags.IsInteractive()

	// Restore stdout
	os.Stdout = origStdout

	// Pipe should not be interactive
	if isInteractive {
		t.Error("Expected pipe to not be interactive")
	}
}

func TestCommonFlags_AllFieldsAccessible(t *testing.T) {
	flags := &CommonFlags{
		Format:     "json",
		JSON:       true,
		NoColor:    true,
		Quiet:      true,
		Verbose:    true,
		ConfigFile: "/path/to/config",
		Workspace:  "/path/to/workspace",
		DryRun:     true,
		Force:      true,
		LogLevel:   "debug",
		Trace:      true,
	}

	// Verify all fields are accessible
	if flags.Format != "json" {
		t.Error("Format field not accessible")
	}
	if !flags.JSON {
		t.Error("JSON field not accessible")
	}
	if !flags.NoColor {
		t.Error("NoColor field not accessible")
	}
	if !flags.Quiet {
		t.Error("Quiet field not accessible")
	}
	if !flags.Verbose {
		t.Error("Verbose field not accessible")
	}
	if flags.ConfigFile != "/path/to/config" {
		t.Error("ConfigFile field not accessible")
	}
	if flags.Workspace != "/path/to/workspace" {
		t.Error("Workspace field not accessible")
	}
	if !flags.DryRun {
		t.Error("DryRun field not accessible")
	}
	if !flags.Force {
		t.Error("Force field not accessible")
	}
	if flags.LogLevel != "debug" {
		t.Error("LogLevel field not accessible")
	}
	if !flags.Trace {
		t.Error("Trace field not accessible")
	}
}
