package pisession

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateID(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"019f8554-828e-76c3-b41e-f01f732c8c7b", "pi.session_1"} {
		if err := ValidateID(id); err != nil {
			t.Fatalf("ValidateID(%q): %v", id, err)
		}
	}
	for _, id := range []string{"", "-leading", "trailing-", "../escape", "bad id", "bad;id"} {
		if err := ValidateID(id); err == nil {
			t.Fatalf("ValidateID(%q) unexpectedly succeeded", id)
		}
	}
}

func TestEnsureRootCreatesPrivateAbsoluteDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "sessions")
	got, err := EnsureRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("root = %q, want absolute", got)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("root mode = %o", info.Mode().Perm())
	}
}

func TestSessionRootRejectsRelativeAndSymlinkDirectories(t *testing.T) {
	if _, err := ValidateRoot("relative/sessions"); err == nil {
		t.Fatal("relative Pi session root was accepted")
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "sessions")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRoot(link); err == nil {
		t.Fatal("symlink Pi session root was accepted")
	}
	if _, err := FindTranscript(link, "native-id"); err == nil {
		t.Fatal("Pi transcript discovery followed a symlink root")
	}
}

func TestValidateCodingAgentDirNormalizesAndRejectsUnsafeTargets(t *testing.T) {
	if got, err := ValidateCodingAgentDir(""); err != nil || got != "" {
		t.Fatalf("empty coding agent dir = %q, %v", got, err)
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "pi agent")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ValidateCodingAgentDir(dir)
	if err != nil || got != dir {
		t.Fatalf("validated coding agent dir = %q, %v; want %q", got, err, dir)
	}
	t.Chdir(parent)
	got, err = ValidateCodingAgentDir("pi agent")
	if err != nil || got != dir {
		t.Fatalf("normalized relative coding agent dir = %q, %v; want %q", got, err, dir)
	}
	link := filepath.Join(t.TempDir(), "pi-link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCodingAgentDir(link); err == nil {
		t.Fatal("symlink Pi coding agent directory was accepted")
	}
	if _, err := ValidateCodingAgentDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing Pi coding agent directory was accepted")
	}
}

func TestResolveCodingAgentDirDistinguishesPersistedDefaultFromLegacyMetadata(t *testing.T) {
	if got := ResolveCodingAgentDir("", true, "/caller/config"); got != "" {
		t.Fatalf("persisted native default resolved to %q", got)
	}
	if got := ResolveCodingAgentDir("/persisted/config", false, "/caller/config"); got != "/persisted/config" {
		t.Fatalf("persisted custom directory resolved to %q", got)
	}
	if got := ResolveCodingAgentDir("", false, "/caller/config"); got != "/caller/config" {
		t.Fatalf("legacy metadata resolved to %q", got)
	}
}

func TestFindTranscriptMatchesExactHeaderID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "2026-old_wrong.jsonl"), `{"type":"session","version":3,"id":"wrong","timestamp":"2026-01-01T00:00:00Z","cwd":"/tmp"}`+"\n")
	want := filepath.Join(dir, "2026-right_target.jsonl")
	writeFixture(t, want, `{"type":"session","version":3,"id":"target-id","timestamp":"2026-07-21T00:00:00Z","cwd":"/tmp"}`+"\n")
	writeFixture(t, filepath.Join(dir, "2026-newer_other.jsonl"), `{"type":"session","version":3,"id":"target-id-prefix","timestamp":"2026-07-22T00:00:00Z","cwd":"/tmp"}`+"\n")

	got, err := FindTranscript(dir, "target-id")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FindTranscript = %q, want %q", got, want)
	}
}

func TestFindTranscriptRejectsUnsafeInputsAndOversizedHeader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := FindTranscript(dir, "../escape"); err == nil {
		t.Fatal("expected unsafe id rejection")
	}
	writeFixture(t, filepath.Join(dir, "huge.jsonl"), string(make([]byte, maxHeaderBytes+1)))
	if _, err := FindTranscript(dir, "missing"); err == nil {
		t.Fatal("expected missing transcript error")
	}
}

func TestTranscriptReadersRejectSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	writeFixture(t, target, `{"type":"session","id":"symlink-id","cwd":"/work"}`+"\n")
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMetadata(link); err == nil {
		t.Fatal("ReadMetadata followed a Pi transcript symlink")
	}
	if _, _, err := ImportNativeFile(t.TempDir(), link); err == nil {
		t.Fatal("ImportNativeFile followed a Pi transcript symlink")
	}
}

func TestManagedTranscriptMetadataAndRemovalStayWithinRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "managed.jsonl")
	writeFixture(t, path, `{"type":"session","id":"managed-id","cwd":"/work"}`+"\n")
	wantModified := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, wantModified, wantModified); err != nil {
		t.Fatal(err)
	}
	gotModified, err := TranscriptModTime(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if !gotModified.Equal(wantModified) {
		t.Fatalf("TranscriptModTime = %s, want %s", gotModified, wantModified)
	}

	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	writeFixture(t, outside, `{"type":"session","id":"outside-id","cwd":"/work"}`+"\n")
	if err := RemoveTranscript(root, outside); err == nil {
		t.Fatal("RemoveTranscript accepted a path outside the managed root")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside transcript was modified: %v", err)
	}

	link := filepath.Join(root, "link.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTranscript(root, link); err == nil {
		t.Fatal("RemoveTranscript followed a symlink")
	}
	if err := RemoveTranscript(root, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("managed transcript still exists after removal: %v", err)
	}
}

func TestFindTranscriptTreeFindsNestedExactIDAndRejectsDuplicates(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "--work-project--")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(nested, "session.jsonl")
	writeFixture(t, want, `{"type":"session","id":"tree-id","cwd":"/work/project"}`+"\n")
	got, err := FindTranscriptTree(root, "tree-id")
	if err != nil || got != want {
		t.Fatalf("FindTranscriptTree = %q, %v; want %q", got, err, want)
	}
	secondDir := filepath.Join(root, "duplicate")
	if err := os.MkdirAll(secondDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(secondDir, "session.jsonl"), `{"type":"session","id":"tree-id","cwd":"/work/project"}`+"\n")
	if _, err := FindTranscriptTree(root, "tree-id"); err == nil {
		t.Fatal("duplicate Pi native IDs were accepted")
	}
}

func TestImportNativeFileCopiesBoundedTranscript(t *testing.T) {
	source := filepath.Join(t.TempDir(), "native.jsonl")
	writeFixture(t, source, `{"type":"session","id":"file-import","cwd":"/work"}`+"\n")
	metadata, path, err := ImportNativeFile(t.TempDir(), source)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ID != "file-import" || path == source {
		t.Fatalf("metadata/path = %#v/%q", metadata, path)
	}
}

func TestReadMessagesProjectsSupportedRoles(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path, "{malformed\n"+
		`{"type":"session","version":3,"id":"target","timestamp":"2026-07-21T00:00:00Z","cwd":"/tmp"}`+"\n"+
		`{"type":"message","id":"u1","parentId":null,"timestamp":"2026-07-21T00:00:01Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`+"\n"+
		`{"type":"model_change","id":"m1","parentId":"u1","timestamp":"2026-07-21T00:00:02Z","provider":"anthropic","modelId":"claude-sonnet-4-6"}`+"\n"+
		`{"type":"message","id":"a1","parentId":"m1","timestamp":"2026-07-21T00:00:03Z","message":{"role":"assistant","provider":"anthropic","model":"claude-sonnet-4-6","content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"world"},{"type":"toolCall","name":"read"}]}}`+"\n"+
		`{"type":"message","id":"t1","parentId":"a1","timestamp":"2026-07-21T00:00:04Z","message":{"role":"toolResult","toolCallId":"x","content":[{"type":"text","text":"tool output"}]}}`+"\n")

	got, err := ReadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("ReadMessages len = %d, want 3: %#v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "hello" {
		t.Fatalf("user message = %#v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "world" {
		t.Fatalf("assistant message = %#v", got[1])
	}
	if got[2].Role != "tool" || got[2].Content != "tool output" {
		t.Fatalf("tool message = %#v", got[2])
	}
	if got[1].Timestamp != time.Date(2026, 7, 21, 0, 0, 3, 0, time.UTC) {
		t.Fatalf("assistant timestamp = %s", got[1].Timestamp)
	}
}

func TestReadUsageUsesLatestAssistantContextAndNativeCost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path,
		`{"type":"session","id":"usage-id","cwd":"/work"}`+"\n"+
			`{"type":"message","timestamp":"2026-07-21T00:00:01Z","message":{"role":"assistant","model":"claude-sonnet-4-6","usage":{"input":100,"output":20,"cacheRead":300,"cacheWrite":40,"cost":{"total":0.12}}}}`+"\n"+
			`{"type":"message","timestamp":"2026-07-21T00:00:02Z","message":{"role":"toolResult","usage":{"input":5,"output":0,"cacheRead":0,"cacheWrite":0,"cost":{"total":0.01}}}}`+"\n"+
			`{"type":"message","timestamp":"2026-07-21T00:00:03Z","message":{"role":"assistant","model":"claude-sonnet-4-6","usage":{"input":200,"output":30,"cacheRead":600,"cacheWrite":50,"cost":{"total":0.23}}}}`+"\n")

	usage, err := ReadUsage(path)
	if err != nil {
		t.Fatal(err)
	}
	if usage.ContextTokens != 850 || usage.Model != "claude-sonnet-4-6" || usage.AssistantCalls != 2 {
		t.Fatalf("usage context = %#v", usage)
	}
	if usage.InputTokens != 1295 || usage.OutputTokens != 50 {
		t.Fatalf("usage totals = %#v", usage)
	}
	if usage.CumulativeCost < 0.359999 || usage.CumulativeCost > 0.360001 {
		t.Fatalf("usage cost = %f", usage.CumulativeCost)
	}
	if usage.LastAssistantAt != time.Date(2026, 7, 21, 0, 0, 3, 0, time.UTC) {
		t.Fatalf("usage timestamp = %s", usage.LastAssistantAt)
	}
}

func TestReadUsagePreservesLatestAssistantProviderProvenance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path,
		`{"type":"session","id":"usage-provider-id","cwd":"/work"}`+"\n"+
			`{"type":"message","timestamp":"2026-07-21T00:00:01Z","message":{"role":"assistant","provider":"ollama","model":"qwen2.5-coder:7b","usage":{"input":100,"output":20}}}`+"\n"+
			`{"type":"message","timestamp":"2026-07-21T00:00:02Z","message":{"role":"assistant","provider":"openrouter","model":"qwen/qwen3.6-max-preview","usage":{"input":200,"output":30}}}`+"\n")

	usage, err := ReadUsage(path)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Model != "openrouter/qwen/qwen3.6-max-preview" {
		t.Fatalf("usage model = %q, want provider-qualified latest model", usage.Model)
	}
}

func TestReadUsagePreservesOpaqueProviderPrefixedModelID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path,
		`{"type":"session","id":"usage-opaque-model-id","cwd":"/work"}`+"\n"+
			`{"type":"message","timestamp":"2026-07-21T00:00:01Z","message":{"role":"assistant","provider":"acme","model":"acme/foo","usage":{"input":100,"output":20}}}`+"\n")

	usage, err := ReadUsage(path)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Model != "acme/acme/foo" {
		t.Fatalf("usage model = %q, want provider plus complete opaque model ID", usage.Model)
	}
}

func TestReadModelUsesLatestNativeProviderProvenance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path,
		`{"type":"session","id":"model-id","cwd":"/work"}`+"\n"+
			`{"type":"model_change","provider":"anthropic","modelId":"claude-sonnet-4-6"}`+"\n"+
			`{"type":"message","message":{"role":"assistant","provider":"openai","model":"gpt-5.6-terra","content":[]}}`+"\n")
	model, err := ReadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if model != "openai/gpt-5.6-terra" {
		t.Fatalf("ReadModel = %q", model)
	}
}

func TestReadModelPreservesOpaqueProviderPrefixedModelID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path,
		`{"type":"session","id":"model-opaque-id","cwd":"/work"}`+"\n"+
			`{"type":"model_change","provider":"acme","modelId":"acme/foo"}`+"\n")

	model, err := ReadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if model != "acme/acme/foo" {
		t.Fatalf("ReadModel = %q, want provider plus complete opaque model ID", model)
	}
}

func TestReadModelWithoutNativeProvenanceIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path, `{"type":"session","id":"model-id","cwd":"/work"}`+"\n")
	model, err := ReadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if model != "" {
		t.Fatalf("ReadModel = %q, want empty", model)
	}
}

func TestReadNativeReturnsBoundedRegularTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	want := []byte(`{"type":"session","id":"native-export","cwd":"/work"}` + "\n")
	writeFixture(t, path, string(want))
	got, err := ReadNative(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("ReadNative = %q, want %q", got, want)
	}
}

func TestImportNativeValidatesAndInstallsPrivateCopy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := t.TempDir()
	data := []byte(`{"type":"session","version":3,"id":"imported-id","timestamp":"2026-07-21T00:00:00Z","cwd":"` + cwd + `"}` + "\n" +
		`{"type":"message","id":"u1","parentId":null,"timestamp":"2026-07-21T00:00:01Z","message":{"role":"user","content":"hello"}}` + "\n")
	metadata, path, err := ImportNative(root, data)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ID != "imported-id" || metadata.CWD != cwd {
		t.Fatalf("metadata = %#v", metadata)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if _, _, err := ImportNative(root, []byte(`{"type":"session","id":"../escape","cwd":"/tmp"}`+"\n")); err == nil {
		t.Fatal("expected unsafe import id rejection")
	}
}

func TestImportNativeRejectsMalformedOrUnboundedJSONLLines(t *testing.T) {
	root := t.TempDir()
	header := `{"type":"session","id":"invalid-jsonl","cwd":"/tmp"}` + "\n"
	for name, data := range map[string][]byte{
		"malformed":    []byte(header + "{malformed\n"),
		"missing type": []byte(header + `{"message":{"role":"user"}}` + "\n"),
		"oversized":    []byte(header + `{"type":"message","value":"` + strings.Repeat("x", maxLineBytes) + `"}` + "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ImportNative(root, data); err == nil {
				t.Fatal("invalid Pi JSONL import unexpectedly succeeded")
			}
		})
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
