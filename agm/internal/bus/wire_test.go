package bus

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFrameValidate(t *testing.T) {
	cases := []struct {
		name  string
		frame Frame
		wantErr string // substring; empty means expect nil
	}{
		{"hello ok", Frame{Type: FrameHello, From: "s1"}, ""},
		{"hello missing from", Frame{Type: FrameHello}, "missing from"},
		{"send ok", Frame{Type: FrameSend, From: "s1", To: "s2", Text: "hi"}, ""},
		{"send missing to", Frame{Type: FrameSend, From: "s1"}, "missing to"},
		{"deliver ok", Frame{Type: FrameDeliver, From: "s1", To: "s2"}, ""},
		{"ack needs id", Frame{Type: FrameAck}, "ack: missing id"},
		{"ack ok", Frame{Type: FrameAck, ID: "abc"}, ""},
		{"perm request ok", Frame{
			Type: FramePermissionRequest, From: "w1", To: "s1", ToolName: "Bash",
		}, ""},
		{"perm request missing tool", Frame{
			Type: FramePermissionRequest, From: "w1", To: "s1",
		}, "missing from, to, or tool_name"},
		{"perm verdict ok", Frame{Type: FramePermissionVerdict, ID: "x", Verdict: "allow"}, ""},
		{"perm verdict bad", Frame{Type: FramePermissionVerdict, ID: "x", Verdict: "maybe"}, "verdict must be"},
		{"bye ok", Frame{Type: FrameBye}, ""},
		{"missing type", Frame{From: "s1"}, "missing type"},
		{"unknown type", Frame{Type: "nonsense", From: "s1"}, "unknown frame type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.frame.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestWriteReadFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	frames := []*Frame{
		{Type: FrameHello, From: "s1"},
		{Type: FrameSend, ID: "id-1", From: "s1", To: "s2", Text: "hello world"},
		{
			Type: FramePermissionRequest, ID: "id-2", From: "w1", To: "s1",
			ToolName: "Bash", Description: "list dir", InputPreview: `{"cmd":"ls"}`,
		},
		{Type: FramePermissionVerdict, ID: "id-2", Verdict: "allow"},
		{Type: FrameError, ID: "id-x", Code: ErrUnknownTarget, Message: "no such peer"},
	}
	for _, f := range frames {
		if err := WriteFrame(&buf, f); err != nil {
			t.Fatalf("WriteFrame(%s): %v", f.Type, err)
		}
	}
	r := bufio.NewReader(&buf)
	for i, want := range frames {
		got, err := ReadFrame(r)
		if err != nil {
			t.Fatalf("ReadFrame %d (%s): %v", i, want.Type, err)
		}
		if got.Type != want.Type {
			t.Errorf("frame %d: Type = %q, want %q", i, got.Type, want.Type)
		}
		if got.ID != want.ID || got.From != want.From || got.To != want.To ||
			got.Text != want.Text || got.ToolName != want.ToolName ||
			got.Verdict != want.Verdict || got.Code != want.Code {
			t.Errorf("frame %d: roundtrip mismatch: got %+v, want %+v", i, got, want)
		}
		if got.TS.IsZero() {
			t.Errorf("frame %d: WriteFrame should have stamped TS", i)
		}
	}
	// Channel should be at EOF now.
	if _, err := ReadFrame(r); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF after all frames, got %v", err)
	}
}

func TestReadFrameSkipsBlankLines(t *testing.T) {
	payload := "\n\n{\"type\":\"hello\",\"from\":\"s1\"}\n\n"
	r := bufio.NewReader(strings.NewReader(payload))
	f, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if f.Type != FrameHello || f.From != "s1" {
		t.Errorf("got %+v", f)
	}
	if _, err := ReadFrame(r); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestReadFrameRejectsBadJSON(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("not-json\n"))
	_, err := ReadFrame(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "decode frame") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadFrameTruncated(t *testing.T) {
	// Non-whitespace content without trailing newline → truncated.
	r := bufio.NewReader(strings.NewReader(`{"type":"hello","from":"s1"}`))
	_, err := ReadFrame(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFrameJSONOmitsEmpty(t *testing.T) {
	var buf bytes.Buffer
	f := &Frame{Type: FrameHello, From: "s1"}
	if err := WriteFrame(&buf, f); err != nil {
		t.Fatal(err)
	}
	line := buf.String()
	for _, forbidden := range []string{"\"to\"", "\"text\"", "\"verdict\"", "\"code\"", "\"extra\""} {
		if strings.Contains(line, forbidden) {
			t.Errorf("expected %s to be omitted from %s", forbidden, line)
		}
	}
}
