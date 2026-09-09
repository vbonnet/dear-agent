package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReplyResolveRequiresBodyFile(t *testing.T) {
	threadID, bodyFile, err := parseReplyResolveArgs([]string{
		"PRRT_exact",
		"--body-file",
		"reply.md",
	})
	if err != nil {
		t.Fatalf("valid body-file form: %v", err)
	}
	if threadID != "PRRT_exact" || bodyFile != "reply.md" {
		t.Fatalf("parsed (%q, %q), want (PRRT_exact, reply.md)", threadID, bodyFile)
	}

	for name, args := range map[string][]string{
		"positional body":  {"PRRT_exact", "Fixed inline"},
		"missing source":   {"PRRT_exact", "--body-file"},
		"missing flag":     {"PRRT_exact", "reply.md"},
		"duplicate source": {"PRRT_exact", "--body-file", "a.md", "--body-file", "b.md"},
		"empty thread":     {"", "--body-file", "reply.md"},
		"empty path":       {"PRRT_exact", "--body-file", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseReplyResolveArgs(args); err == nil {
				t.Fatalf("parseReplyResolveArgs(%q) succeeded, want usage failure", args)
			}
		})
	}

	argsPath, _ := installFakeGH(t, "{}")
	diagnostics := captureStderr(t, func() {
		if code := cmdReplyResolve(context.Background(), []string{"PRRT_exact", "Fixed inline"}); code == 0 {
			t.Fatal("legacy positional-body command succeeded")
		}
	})
	if !strings.Contains(diagnostics, "--body-file") {
		t.Fatalf("legacy form did not report the safe interface: %q", diagnostics)
	}
	if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
		t.Fatalf("invalid reply input reached GitHub or returned unexpected stat error: %v", err)
	}
}

func TestReplyResolveRejectsInvalidBodySourcesBeforeProviderMutation(t *testing.T) {
	invalidUTF8 := filepath.Join(t.TempDir(), "invalid.md")
	if err := os.WriteFile(invalidUTF8, []byte{0xff, 'x'}, 0o600); err != nil {
		t.Fatalf("write invalid UTF-8 body: %v", err)
	}
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty body: %v", err)
	}
	whitespace := filepath.Join(t.TempDir(), "whitespace.md")
	if err := os.WriteFile(whitespace, []byte(" \r\n\t"), 0o600); err != nil {
		t.Fatalf("write whitespace body: %v", err)
	}
	oversized := filepath.Join(t.TempDir(), "oversized.md")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", maxReplyBodyBytes+1)), 0o600); err != nil {
		t.Fatalf("write oversized body: %v", err)
	}

	for name, args := range map[string][]string{
		"missing file":  {"PRRT_exact", "--body-file", filepath.Join(t.TempDir(), "missing.md")},
		"unreadable":    {"PRRT_exact", "--body-file", t.TempDir()},
		"invalid UTF-8": {"PRRT_exact", "--body-file", invalidUTF8},
		"empty":         {"PRRT_exact", "--body-file", empty},
		"whitespace":    {"PRRT_exact", "--body-file", whitespace},
		"oversized":     {"PRRT_exact", "--body-file", oversized},
		"duplicate": {
			"PRRT_exact", "--body-file", empty, "--body-file", whitespace,
		},
	} {
		t.Run(name, func(t *testing.T) {
			argsPath, _ := installFakeGH(t, "{}")
			diagnostics := captureStderr(t, func() {
				if code := cmdReplyResolve(context.Background(), args); code == 0 {
					t.Fatal("invalid reply source succeeded")
				}
			})
			if strings.TrimSpace(diagnostics) == "" {
				t.Fatal("invalid reply source produced no diagnostic")
			}
			if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
				t.Fatalf("invalid reply source reached GitHub or returned unexpected stat error: %v", err)
			}
		})
	}
}

func TestLoadReplyBodyRejectsOversizedOrEndlessSources(t *testing.T) {
	maxASCII := strings.Repeat("x", maxReplyBodyCharacters)
	got, err := loadReplyBody("-", strings.NewReader(maxASCII))
	if err != nil {
		t.Fatalf("load maximum-sized ASCII body: %v", err)
	}
	if got != maxASCII {
		t.Fatal("maximum-sized ASCII body changed")
	}

	maxCharacters := strings.Repeat("🧪", maxReplyBodyCharacters)
	got, err = loadReplyBody("-", strings.NewReader(maxCharacters))
	if err != nil {
		t.Fatalf("load maximum-sized UTF-8 body: %v", err)
	}
	if got != maxCharacters {
		t.Fatal("maximum-sized UTF-8 body changed")
	}

	if _, err := loadReplyBody("-", strings.NewReader(strings.Repeat("x", maxReplyBodyCharacters+1))); err == nil {
		t.Fatal("character-oversized stdin succeeded")
	}

	endless := &endlessReplyReader{}
	if _, err := loadReplyBody("-", endless); err == nil {
		t.Fatal("endless stdin succeeded")
	}
	if endless.read != maxReplyBodyBytes+1 {
		t.Fatalf("endless stdin read %d bytes, want bounded read of %d", endless.read, maxReplyBodyBytes+1)
	}

	if _, err := loadReplyBody(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("non-regular named source error = %v, want regular-file refusal", err)
	}
}

type endlessReplyReader struct {
	read int
}

func (r *endlessReplyReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.read += len(p)
	return len(p), nil
}

func TestLoadReplyBodyPreservesExactBytes(t *testing.T) {
	want := "  leading CRLF\r\nsingle ' double \"\nemoji 🧪 and combining e\u0301\n"
	path := filepath.Join(t.TempDir(), "reply.md")
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	got, err := loadReplyBody(path, nil)
	if err != nil {
		t.Fatalf("load named body file: %v", err)
	}
	if got != want {
		t.Fatalf("named body bytes = %q, want %q", got, want)
	}

	got, err = loadReplyBody("-", strings.NewReader(want))
	if err != nil {
		t.Fatalf("load stdin body: %v", err)
	}
	if got != want {
		t.Fatalf("stdin body bytes = %q, want %q", got, want)
	}

	invalidUTF8 := filepath.Join(t.TempDir(), "invalid.md")
	if err := os.WriteFile(invalidUTF8, []byte{0xff, 'x'}, 0o600); err != nil {
		t.Fatalf("write invalid UTF-8 body: %v", err)
	}
	whitespace := filepath.Join(t.TempDir(), "whitespace.md")
	if err := os.WriteFile(whitespace, []byte(" \r\n\t"), 0o600); err != nil {
		t.Fatalf("write whitespace body: %v", err)
	}
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty body: %v", err)
	}

	for name, tc := range map[string]struct {
		path  string
		stdin io.Reader
	}{
		"missing file":    {path: filepath.Join(t.TempDir(), "missing.md")},
		"empty":           {path: empty},
		"whitespace":      {path: whitespace},
		"invalid UTF-8":   {path: invalidUTF8},
		"stdin missing":   {path: "-", stdin: nil},
		"stdin read fail": {path: "-", stdin: failingReader{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadReplyBody(tc.path, tc.stdin); err == nil {
				t.Fatal("loadReplyBody succeeded, want failure")
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestGHGraphQLSendsQueryAndVariablesAsJSONStdin(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "must-not-exist")
	tick := string(rune(96))
	body := "PAYLOAD-MUST-STAY-DATA ' \" " + tick + "ticks" + tick + "; " +
		"$" + "(touch " + sentinel + "); $" + "{VAR}\r\n" +
		"second line 🧪 e\u0301\n"
	argsPath, inputPath := installFakeGH(
		t,
		"{\"data\":{\"addPullRequestReviewThreadReply\":{\"comment\":{\"id\":\"PRRC_exact\"}}}}",
	)
	if _, err := ghGraphQL(context.Background(), "query", map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("unsupported JSON variable reached the provider boundary")
	}
	if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
		t.Fatalf("JSON encoding failure invoked GitHub or returned unexpected stat error: %v", err)
	}

	id, err := postReply(context.Background(), "PRRT_exact", body)
	if err != nil {
		t.Fatalf("postReply: %v", err)
	}
	if id != "PRRC_exact" {
		t.Fatalf("reply id = %q, want PRRC_exact", id)
	}
	assertFixedGHArgs(t, argsPath)
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("embedded command created sentinel or returned unexpected stat error: %v", err)
	}

	request := readCapturedGraphQLRequest(t, inputPath)
	if request.Query != replyMutation {
		t.Fatalf("query differs from reply mutation")
	}
	var gotBody, gotThreadID string
	if err := json.Unmarshal(request.Variables["body"], &gotBody); err != nil {
		t.Fatalf("decode body variable: %v", err)
	}
	if err := json.Unmarshal(request.Variables["threadId"], &gotThreadID); err != nil {
		t.Fatalf("decode threadId variable: %v", err)
	}
	if gotBody != body {
		t.Fatalf("provider body = %q, want exact %q", gotBody, body)
	}
	if gotThreadID != "PRRT_exact" {
		t.Fatalf("provider threadId = %q, want PRRT_exact", gotThreadID)
	}
	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read captured argv: %v", err)
	}
	if strings.Contains(string(argsRaw), body) || strings.Contains(string(argsRaw), "PAYLOAD-MUST-STAY-DATA") {
		t.Fatalf("reply body leaked into argv: %q", argsRaw)
	}

	if _, err := ghGraphQL(context.Background(), listQuery, map[string]any{
		"owner": "owner",
		"repo":  "repo",
		"pr":    37,
	}); err != nil {
		t.Fatalf("typed list request: %v", err)
	}
	request = readCapturedGraphQLRequest(t, inputPath)
	var gotPR int
	if err := json.Unmarshal(request.Variables["pr"], &gotPR); err != nil {
		t.Fatalf("decode numeric pr variable: %v", err)
	}
	if gotPR != 37 {
		t.Fatalf("provider pr = %d, want numeric 37", gotPR)
	}
	if _, exists := request.Variables["after"]; exists {
		t.Fatal("first-page request included an empty after cursor")
	}
}

func TestGHGraphQLSuppressesReplyBodyEchoedByChildStderr(t *testing.T) {
	body := "PAYLOAD-MUST-STAY-DATA ' \" `cmd` $(touch sentinel) ${VAR}\r\n🧪 e\u0301\n"
	_, inputPath := installEchoingFailingGH(t)

	_, err := ghGraphQL(context.Background(), replyMutation, map[string]any{
		"threadId": "PRRT_exact",
		"body":     body,
	})
	if err == nil {
		t.Fatal("failing gh command succeeded")
	}
	if strings.Contains(err.Error(), body) || strings.Contains(err.Error(), "PAYLOAD-MUST-STAY-DATA") {
		t.Fatalf("provider stderr leaked the reply body: %q", err)
	}
	if !strings.Contains(err.Error(), "provider diagnostics suppressed") {
		t.Fatalf("failure does not explain stderr suppression: %q", err)
	}

	request := readCapturedGraphQLRequest(t, inputPath)
	var gotBody string
	if err := json.Unmarshal(request.Variables["body"], &gotBody); err != nil {
		t.Fatalf("decode body variable: %v", err)
	}
	if gotBody != body {
		t.Fatalf("provider body = %q, want exact %q", gotBody, body)
	}
}

func TestGHGraphQLPreservesBodyFreeChildStderr(t *testing.T) {
	_, _ = installEchoingFailingGH(t)

	_, err := ghGraphQL(context.Background(), listQuery, map[string]any{
		"owner": "BODY-FREE-DIAGNOSTIC",
		"repo":  "repo",
		"pr":    37,
	})
	if err == nil {
		t.Fatal("failing gh command succeeded")
	}
	if !strings.Contains(err.Error(), "BODY-FREE-DIAGNOSTIC") {
		t.Fatalf("body-free provider stderr was not retained: %q", err)
	}
}

func TestRetryAdviceDoesNotRenderReplyBody(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "must-not-exist")
	tick := string(rune(96))
	body := "PAYLOAD-MUST-STAY-DATA " + tick + "cmd" + tick + " $" + "(touch " + sentinel + ")"
	installFakeGH(t, "{\"data\":{\"addPullRequestReviewThreadReply\":{}}}")

	diagnostics := captureStderr(t, func() {
		if _, code := postReplyOrExit(context.Background(), "PRRT_exact", body); code == 0 {
			t.Fatal("missing reply ID must fail")
		}
	})
	if strings.Contains(diagnostics, body) || strings.Contains(diagnostics, "PAYLOAD-MUST-STAY-DATA") {
		t.Fatalf("retry diagnostics rendered reply body: %q", diagnostics)
	}
	if !strings.Contains(diagnostics, "unchanged --body-file source") {
		t.Fatalf("retry diagnostics omit body-file recovery contract: %q", diagnostics)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("retry diagnostics executed embedded command or returned unexpected stat error: %v", err)
	}
}

type capturedGraphQLRequest struct {
	Query     string
	Variables map[string]json.RawMessage
}

func readCapturedGraphQLRequest(t *testing.T, path string) capturedGraphQLRequest {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read captured GraphQL request: %v", err)
	}
	var request capturedGraphQLRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode captured GraphQL request %q: %v", raw, err)
	}
	return request
}

func installFakeGH(t *testing.T, response string) (argsPath, inputPath string) {
	t.Helper()
	dir := t.TempDir()
	argsPath = filepath.Join(dir, "args")
	inputPath = filepath.Join(dir, "input.json")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$GH_ARGS_FILE\"\n" +
		"cat > \"$GH_INPUT_FILE\"\n" +
		"printf '%s' \"$GH_RESPONSE\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_ARGS_FILE", argsPath)
	t.Setenv("GH_INPUT_FILE", inputPath)
	t.Setenv("GH_RESPONSE", response)
	return argsPath, inputPath
}

func installEchoingFailingGH(t *testing.T) (argsPath, inputPath string) {
	t.Helper()
	dir := t.TempDir()
	argsPath = filepath.Join(dir, "args")
	inputPath = filepath.Join(dir, "input.json")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$GH_ARGS_FILE\"\n" +
		"cat > \"$GH_INPUT_FILE\"\n" +
		"cat \"$GH_INPUT_FILE\" >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write failing fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_ARGS_FILE", argsPath)
	t.Setenv("GH_INPUT_FILE", inputPath)
	return argsPath, inputPath
}

func assertFixedGHArgs(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read captured argv: %v", err)
	}
	got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	want := []string{"api", "graphql", "--input", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gh argv = %q, want %q", got, want)
	}
}

func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("open stderr pipe: %v", err)
	}
	previous := os.Stderr
	os.Stderr = writeEnd
	defer func() {
		os.Stderr = previous
		_ = readEnd.Close()
		_ = writeEnd.Close()
	}()

	run()
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	raw, err := io.ReadAll(readEnd)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(raw)
}
