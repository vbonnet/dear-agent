package output

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStdoutWriter_Success(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writer := NewStdoutWriter(false)
	writer.Success("test message")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "✓") {
		t.Errorf("Success() should contain checkmark, got: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("Success() should contain message, got: %s", output)
	}
}

func TestStdoutWriter_Info(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writer := NewStdoutWriter(false)
	writer.Info("info message")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "info message") {
		t.Errorf("Info() should contain message, got: %s", output)
	}
}

func TestStdoutWriter_Progress_Verbose(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writer := NewStdoutWriter(true)
	writer.Progress("progress message")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "progress message") {
		t.Errorf("Progress() in verbose mode should show message, got: %s", output)
	}
	if !strings.Contains(output, "→") {
		t.Errorf("Progress() should contain arrow, got: %s", output)
	}
}

func TestStdoutWriter_Progress_NotVerbose(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writer := NewStdoutWriter(false)
	writer.Progress("progress message")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output != "" {
		t.Errorf("Progress() in non-verbose mode should not show output, got: %s", output)
	}
}

func TestStdoutWriter_Table(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writer := NewStdoutWriter(false)
	headers := []string{"Name", "Status"}
	rows := [][]string{
		{"repo1", "cloned"},
		{"repo2", "missing"},
	}
	writer.Table(headers, rows)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Name") {
		t.Errorf("Table() should contain headers, got: %s", output)
	}
	if !strings.Contains(output, "repo1") {
		t.Errorf("Table() should contain row data, got: %s", output)
	}
	if !strings.Contains(output, "---") {
		t.Errorf("Table() should contain separator, got: %s", output)
	}
}

func TestStdoutWriter_Table_Empty(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writer := NewStdoutWriter(false)
	writer.Table([]string{}, [][]string{})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output != "" {
		t.Errorf("Table() with empty headers should produce no output, got: %s", output)
	}
}

func TestStdoutWriter_Error(t *testing.T) {
	// Capture stderr
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	writer := NewStdoutWriter(false)
	writer.Error("test error message")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "✗") {
		t.Errorf("Error() should contain X mark, got: %s", output)
	}
	if !strings.Contains(output, "test error message") {
		t.Errorf("Error() should contain message, got: %s", output)
	}
}
