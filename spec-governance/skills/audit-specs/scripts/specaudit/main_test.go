package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/spec-governance/internal/gittest"
)

func TestInstalledPluginRunsFromUnrelatedWorkingDirectory(t *testing.T) {
	distributionRoot := specauditDistributionRoot(t)
	installedRoot := t.TempDir()
	for _, relative := range []string{
		"go.mod",
		"go.sum",
		".claude-plugin/plugin.json",
		"scripts/specaudit",
		"earslint/config.go",
		"earslint/earslint.go",
		"skills/audit-specs/scripts/specaudit/main.go",
		"skills/audit-specs/SKILL.md",
		"skills/write-spec/SKILL.md",
	} {
		copyInstalledPluginFile(t, distributionRoot, installedRoot, relative)
	}
	assertInstalledPluginSurface(t, installedRoot)

	unrelated := t.TempDir()
	writeTestFile(t, unrelated, "go.mod", "module attacker.example/lookalike\n\ngo 1.26.5\n")
	writeTestFile(t, unrelated, "skills/audit-specs/scripts/specaudit/main.go", "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"UNTRUSTED-CWD-SPECAUDIT\") }\n")
	auditedRepository := t.TempDir()
	gitTest(t, auditedRepository, "init", "-q")
	gittest.HardenRepo(t, auditedRepository)
	gitTest(t, auditedRepository, "config", "user.email", "test@example.com")
	gitTest(t, auditedRepository, "config", "user.name", "Test")
	writeTestFile(t, auditedRepository, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, auditedRepository, "trusted/SPEC.md", "# Trusted\n\n**TRUSTED-01** When an installed audit runs, the system shall read the requested repository.\n")
	gitTest(t, auditedRepository, "add", ".")
	gitTest(t, auditedRepository, "commit", "-qm", "trusted inventory")
	revision := strings.TrimSpace(gitTest(t, auditedRepository, "rev-parse", "HEAD"))

	ambientRepository := t.TempDir()
	gitTest(t, ambientRepository, "init", "-q")
	gittest.HardenRepo(t, ambientRepository)
	gitTest(t, ambientRepository, "config", "user.email", "test@example.com")
	gitTest(t, ambientRepository, "config", "user.name", "Test")
	writeTestFile(t, ambientRepository, "wrong/SPEC.md", "# Wrong\n\n**WRONG-01** When ambient Git state leaks, the system shall read an unrelated repository.\n")
	gitTest(t, ambientRepository, "add", ".")
	gitTest(t, ambientRepository, "commit", "-qm", "ambient inventory")

	maliciousGoEnv := filepath.Join(t.TempDir(), "go.env")
	if err := os.WriteFile(maliciousGoEnv, []byte("GOFLAGS=-mod=vendor\nGOTOOLCHAIN=go1.1\n"), 0o600); err != nil {
		t.Fatalf("write hostile Go environment: %v", err)
	}
	command := exec.Command(
		filepath.Join(installedRoot, "scripts", "specaudit"),
		"inventory", "-repo", auditedRepository, "-repository", "owner/trusted", "-revision", revision,
	)
	command.Dir = unrelated
	command.Env = append(os.Environ(),
		"GOWORK="+filepath.Join(unrelated, "go.work"),
		"GOFLAGS=-mod=vendor",
		"GOENV="+maliciousGoEnv,
		"GOTOOLCHAIN=go1.1",
		"GIT_DIR="+filepath.Join(ambientRepository, ".git"),
		"GIT_WORK_TREE="+ambientRepository,
		"GIT_INDEX_FILE="+filepath.Join(ambientRepository, ".git", "index"),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run installed specaudit from unrelated working directory: %v\n%s", err, output)
	}
	var inventoryReport report
	if err := json.Unmarshal(output, &inventoryReport); err != nil {
		t.Fatalf("decode installed specaudit inventory: %v\n%s", err, output)
	}
	if inventoryReport.Snapshot.Repository != "owner/trusted" || len(inventoryReport.Inventory) != 1 {
		t.Fatalf("installed specaudit selected the wrong repository: %#v", inventoryReport.Snapshot)
	}
	if got := inventoryReport.Inventory[0].Requirements; len(got) != 1 || got[0].ID != "TRUSTED-01" {
		t.Fatalf("installed specaudit omitted trusted EARS evidence: %#v", got)
	}
	if strings.Contains(string(output), "UNTRUSTED-CWD-SPECAUDIT") {
		t.Fatalf("installed execution selected the active-working-directory lookalike: %s", output)
	}
}

func TestGitOutputIsBounded(t *testing.T) {
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gittest.HardenRepo(t, repository)
	gitTest(t, repository, "config", "user.email", "test@example.com")
	gitTest(t, repository, "config", "user.name", "Test")
	writeTestFile(t, repository, "large.txt", strings.Repeat("x", 256))
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "large output")

	if _, err := gitWithOutputLimit(repository, 64, "show", "HEAD:large.txt"); err == nil || !strings.Contains(err.Error(), "output exceeds 64 bytes") {
		t.Fatalf("gitWithOutputLimit() error = %v, want bounded-output rejection", err)
	}
}

func specauditDistributionRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve specaudit test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
}

func assertInstalledPluginSurface(t *testing.T, root string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatalf("read installed native manifest: %v", err)
	}
	var manifest struct {
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode installed native manifest: %v", err)
	}
	if got, want := strings.Join(manifest.Skills, ","), "./skills/audit-specs,./skills/write-spec"; got != want {
		t.Fatalf("installed native manifest skills = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatalf("read installed skills: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("installed skill surface = %#v, want audit-specs and write-spec only", entries)
	}
	got := []string{entries[0].Name(), entries[1].Name()}
	if !slices.Equal(got, []string{"audit-specs", "write-spec"}) {
		t.Fatalf("installed skill surface = %#v, want audit-specs and write-spec only", entries)
	}
	for _, forbidden := range []string{".mcp.json", "mcp.json", ".lsp.json", "lsp.json", "hooks", "agents", "commands"} {
		if _, err := os.Lstat(filepath.Join(root, forbidden)); err == nil {
			t.Fatalf("installed plugin unexpectedly exposes %s", forbidden)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect installed plugin %s: %v", forbidden, err)
		}
	}
}

func copyInstalledPluginFile(t *testing.T, sourceRoot, destinationRoot, relative string) {
	t.Helper()
	source := filepath.Join(sourceRoot, relative)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read installed plugin source %s: %v", relative, err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatalf("stat installed plugin source %s: %v", relative, err)
	}
	destination := filepath.Join(destinationRoot, relative)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("create installed plugin directory for %s: %v", relative, err)
	}
	if err := os.WriteFile(destination, data, info.Mode().Perm()); err != nil {
		t.Fatalf("write installed plugin file %s: %v", relative, err)
	}
}

func TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When a request runs, the system shall persist identity.\n\n## BDD Traceability\n\n- Feature: `test/bdd/shared.feature`\n")
	writeTestFile(t, repo, "two/SPEC.md", "# Two\n\n**ONE-01** When a request runs, the system shall persist identity.\n\n## BDD Traceability\n\n- Feature: `test/bdd/shared.feature`\n")
	writeTestFile(t, repo, "three/SPEC.md", "# One\n\n**ONE-01** When a request runs, the system shall persist identity.\n\n## BDD Traceability\n\n- Feature: `test/bdd/shared.feature`\n")
	writeTestFile(t, repo, "test/bdd/shared.feature", "# SPEC: one/SPEC.md\n# RELATED-SPEC: two/SPEC.md\n# RELATED-SPEC: three/SPEC.md\nFeature: Shared\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "initial")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeTestFile(t, repo, "one/SPEC.md", "not tracked at this revision\n")

	var first, second, stderr bytes.Buffer
	args := []string{"inventory", "-repo", repo, "-repository", "owner/repo", "-revision", revision}
	if code := run(args, &first, &stderr); code != 0 {
		t.Fatalf("first inventory=%d: %s", code, stderr.String())
	}
	if code := run(args, &second, &stderr); code != 0 {
		t.Fatalf("second inventory=%d: %s", code, stderr.String())
	}
	if first.String() != second.String() {
		t.Fatal("inventory JSON is not deterministic")
	}
	var got report
	if err := json.Unmarshal(first.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.Repository != "owner/repo" || got.Snapshot.Revision != revision || got.Summary.SpecFiles != 3 || got.Summary.Requirements != 3 {
		t.Fatalf("unexpected inventory summary: %#v", got.Summary)
	}
	if len(got.Seeds) != 4 {
		t.Fatalf("seeds=%d, want exact-body, duplicate-id, shared-bdd, identical-file", len(got.Seeds))
	}
	if got.Inventory[0].Requirements[0].Excerpt == "not tracked at this revision" {
		t.Fatal("inventory read working tree instead of pinned revision")
	}
}

func TestInventoryIgnoresGitReplacementObjectsAndInheritedRepositoryOverrides(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# Original\n\n**ORIGINAL-01** When an audit reads a pinned revision, the system shall use the original object.\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "original")
	original := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, "one/SPEC.md", "# Forged\n\n**FORGED-01** When an audit reads a pinned revision, the system shall accept substituted content.\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "replacement")
	replacement := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, repo, "replace", original, replacement)
	if substituted := gitTest(t, repo, "show", original+":one/SPEC.md"); !strings.Contains(substituted, "FORGED-01") {
		t.Fatalf("test replacement was not active: %q", substituted)
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "attacker.git"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())

	got, err := inventory(repo, "owner/repo", original)
	if err != nil {
		t.Fatal(err)
	}
	requirement := inventoryRequirement(t, got, "one/SPEC.md")
	if requirement.ID != "ORIGINAL-01" || strings.Contains(requirement.Excerpt, "FORGED-01") {
		t.Fatalf("replacement object changed pinned evidence: %#v", requirement)
	}
}

func TestInventoryIgnoresAmbientGitRepositoryContext(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# Expected\n\n**EXPECTED-01** When an audit pins a repository, the system shall ignore ambient Git repository context.\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "expected repository")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	other := t.TempDir()
	gitTest(t, other, "init", "-q")
	gittest.HardenRepo(t, other)
	gitTest(t, other, "config", "user.email", "test@example.com")
	gitTest(t, other, "config", "user.name", "Test")
	writeTestFile(t, other, "other/SPEC.md", "# Wrong\n\n**WRONG-01** When ambient Git variables are inherited, the system shall read the wrong repository.\n")
	gitTest(t, other, "add", ".")
	gitTest(t, other, "commit", "-qm", "ambient repository")

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	got, err := inventory(repo, "owner/expected", revision)
	if err != nil {
		t.Fatal(err)
	}
	requirement := inventoryRequirement(t, got, "one/SPEC.md")
	if requirement.ID != "EXPECTED-01" || got.Snapshot.Repository != "owner/expected" {
		t.Fatalf("ambient Git context changed pinned evidence: snapshot=%#v requirement=%#v", got.Snapshot, requirement)
	}
}

func TestParseSpecCountsOnlyCanonicalRequirementIDs(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"**Scope:** This is metadata, not a requirement.",
		"**lower-01** When a request runs, the system shall ignore a non-canonical identifier.",
		"**NOHYPHEN** When a request runs, the system shall ignore an identifier without a separator.",
		"**GOOD-01** When a request runs, the system shall count the canonical requirement.",
	}, "\n"))

	if len(parsed.Requirements) != 1 {
		t.Fatalf("requirements=%d, want 1: %#v", len(parsed.Requirements), parsed.Requirements)
	}
	if parsed.Requirements[0].ID != "GOOD-01" {
		t.Fatalf("requirement ID=%q, want GOOD-01", parsed.Requirements[0].ID)
	}
}

func TestParseSpecSkipsFencesAndRecordsUnidentifiedRequirements(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"```markdown",
		"**FENCED-01** When a sample runs, the system shall not count it.",
		"- Feature: `agm/test/bdd/features/fenced.feature`",
		"```",
		"",
		"When a requirement has no ID, the system shall record a diagnostic.",
		"**REAL-01** When the source is parsed, the system shall count real requirements.",
		"",
		"## BDD Traceability",
		"",
		"- Feature: `agm/test/bdd/features/real.feature`",
	}, "\n"))

	if len(parsed.Requirements) != 1 || parsed.Requirements[0].ID != "REAL-01" {
		t.Fatalf("requirements=%#v, want REAL-01 only", parsed.Requirements)
	}
	if len(parsed.Diagnostics) != 1 || parsed.Diagnostics[0].Kind != "anonymous-requirement" {
		t.Fatalf("diagnostics=%#v, want one anonymous requirement", parsed.Diagnostics)
	}
	if len(parsed.BDDFeatures) != 1 || parsed.BDDFeatures[0].Path != "agm/test/bdd/features/real.feature" {
		t.Fatalf("BDD features=%#v, want only the traceability-section feature", parsed.BDDFeatures)
	}
}

func TestValidateRejectsInvalidPositiveFinding(t *testing.T) {
	report := validReport()
	report.Candidates = []finding{{
		ID: "SPEC-CLUSTER-001", Rank: 1, Title: "bad", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the behavior"}}, ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "already owns the neutral behavior"}, SharedOutcome: "same", MaterialDifferences: []string{"none observed"}, Evidence: []evidence{{Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}},
		ApplicabilityBasis: "active-members", ApplicabilityRationale: "supported by the active member", Applicability: []applicability{{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{{Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}}}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "merge"}, Recommendation: []string{"merge"}, Risk: "bounded", Decision: "approve",
	}}
	report.Summary.CandidateCount = 1
	report.Summary.ByVerdict = map[string]int{"merge-now": 1}
	if err := validateReport(report); err == nil || !strings.Contains(err.Error(), "two SPEC owners") {
		t.Fatalf("validateReport error=%v, want positive evidence rejection", err)
	}
}

func TestValidateFindingOwnerStateCoherence(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	candidateFinding := semanticReport.Candidates[0]
	active := map[string]bool{"codex-cli": true, "pi-cli": true}

	candidateFinding.ProposedOwner = &proposedOwnerClaim{Path: "shared/SPEC.md", State: "new", Rationale: "A new implemented shared seam is required."}
	if err := validateFinding(candidateFinding, false, active); err != nil {
		t.Fatalf("new proposed owner should be valid: %v", err)
	}
	if err := validateAgainstInventory(report{
		SchemaVersion: schemaVersion,
		Snapshot:      semanticReport.Snapshot,
		Scope:         semanticReport.Scope,
		Summary:       semanticReport.Summary,
		Methodology:   semanticReport.Methodology,
		Candidates:    []finding{candidateFinding},
		NonCandidates: []finding{},
		Limitations:   semanticReport.Limitations,
	}, inventoryReport); err != nil {
		t.Fatalf("absent new owner should authenticate: %v", err)
	}

	candidateFinding.Verdict = "insufficient-evidence"
	candidateFinding.Strength = "moderate"
	if err := validateFinding(candidateFinding, false, active); err == nil || !strings.Contains(err.Error(), "cannot select a canonical owner") {
		t.Fatalf("non-positive proposed owner error = %v", err)
	}
}

func TestRenderIsOfflineAndEscapesEvidence(t *testing.T) {
	report := validReport()
	report.NonCandidates = []finding{{
		ID: "SPEC-CLUSTER-002", Title: "native <adapter>", Verdict: "keep-separate", Relationship: "same-vocabulary-only", Classification: "native-adapter", Confidence: "confirmed", Strength: "moderate",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the native behavior"}}, SharedOutcome: "separate", MaterialDifferences: []string{"native path"}, Evidence: []evidence{{Path: "one/SPEC.md", Line: 2, RequirementID: "ONE-01", Excerpt: "<script>alert(1)</script>"}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "adapter-only"}, Recommendation: []string{"keep it"}, Risk: "bounded", Limitations: []string{"sentinel limitation"}, Decision: "retain", Boundary: "native behavior differs",
	}}
	report.Summary.ByVerdict = map[string]int{"keep-separate": 1}
	report.Snapshot.ComparisonRevision = strings.Repeat("b", 40)
	report.Scope.Excluded = []exclusion{{Path: "vendor", Reason: "dependency corpus"}}
	output := renderHTML(report, nil)
	if strings.Contains(output, "src=\"http") || strings.Contains(output, "href=\"http") || strings.Contains(output, "<script>alert") {
		t.Fatalf("renderer leaked external runtime or unsafe evidence: %s", output)
	}
	if !strings.Contains(output, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("renderer did not escape evidence: %s", output)
	}
	for _, sentinel := range []string{"SPEC-CLUSTER-002", "same-vocabulary-only", "native-adapter", "one/SPEC.md", "native path", "agm/test/bdd/features/example.feature", "keep it", "sentinel limitation", "dependency corpus", strings.Repeat("b", 40)} {
		if !strings.Contains(output, sentinel) {
			t.Fatalf("renderer omitted %q", sentinel)
		}
	}
}

func TestRenderRetainsEscapedApplicabilityEvidenceRecords(t *testing.T) {
	report := validReport()
	applicabilityEvidence := evidence{
		Path:          "one/SPEC.md",
		Line:          17,
		RequirementID: "ONE-01",
		Excerpt:       `<img src=x onerror="alert('unsafe')">`,
	}
	report.Candidates = []finding{{
		ID: "SPEC-CLUSTER-001", Rank: 1, Title: "applicability evidence", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the behavior"}, {Path: "two/SPEC.md", Rationale: "also owns the behavior"}}, ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "owns the canonical behavior"},
		SharedOutcome: "same", MaterialDifferences: []string{"none observed"}, Evidence: []evidence{{Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}, {Path: "two/SPEC.md", Line: 1, RequirementID: "TWO-01", Excerpt: "two"}},
		ApplicabilityBasis: "active-members", ApplicabilityRationale: "both claims use the same pinned evidence", Applicability: []applicability{{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{applicabilityEvidence, applicabilityEvidence}}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "merge"}, Recommendation: []string{"merge"}, Risk: "bounded", Decision: "approve",
	}}
	report.Summary.CandidateCount = 1
	report.Summary.ByVerdict = map[string]int{"merge-now": 1}

	output := renderHTML(report, nil)
	const escapedExcerpt = "&lt;img src=x onerror=&#34;alert(&#39;unsafe&#39;)&#34;&gt;"
	if strings.Contains(output, `<img src=x onerror="alert('unsafe')">`) {
		t.Fatalf("renderer emitted unescaped applicability excerpt: %s", output)
	}
	if got := strings.Count(output, escapedExcerpt); got != 2 {
		t.Fatalf("escaped applicability excerpts=%d, want 2 to preserve duplicate records", got)
	}
	if got := strings.Count(output, "one/SPEC.md:17"); got != 2 {
		t.Fatalf("applicability path and line occurrences=%d, want 2", got)
	}
	if got := strings.Count(output, "one/SPEC.md:17</code> <span class=\"pill\">ONE-01</span><br>"); got != 2 {
		t.Fatalf("applicability requirement ID occurrences=%d, want 2", got)
	}
}

func TestCommandsRejectFilesystemOutputFlags(t *testing.T) {
	for _, args := range [][]string{
		{"inventory", "-output", "/tmp/inventory.json"},
		{"render", "-output", "/tmp/report.html"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "flag provided but not defined: -output") {
			t.Fatalf("run(%v)=%d stderr=%q, want output-flag rejection", args, code, stderr.String())
		}
	}
}

func TestValidateAuthenticatesFindingsAgainstPinnedGitInventory(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	inventoryPath := filepath.Join(t.TempDir(), "inventory.json")
	reportPath := filepath.Join(t.TempDir(), "findings.json")
	writeJSON(t, inventoryPath, inventoryReport)
	writeJSON(t, reportPath, semanticReport)

	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-input", reportPath, "-inventory", inventoryPath, "-repo", repo}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("validate=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid spec-audit/v1") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	rendered := renderHTML(semanticReport, &inventoryReport)
	for _, sentinel := range []string{semanticReport.Candidates[0].CurrentOwners[0].Rationale, semanticReport.Candidates[0].OwnershipCompleteness, semanticReport.Candidates[0].ProposedOwner.Rationale, "one/SPEC.md:"} {
		if !strings.Contains(rendered, sentinel) {
			t.Fatalf("authenticated renderer omitted ownership proof %q", sentinel)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"validate", "-input", reportPath}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "-inventory and -repo are required") {
		t.Fatalf("validate without proof=%d, stderr=%q", code, stderr.String())
	}
}

func TestAuthenticatedValidationAcceptsBDDReciprocityAcrossFeatures(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{
		"agm/test/bdd/features/one-only.feature",
		"agm/test/bdd/features/two-only.feature",
	}

	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("semantic report should be structurally valid: %v", err)
	}
	if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
		t.Fatalf("inventory should authenticate against the pinned repository: %v", err)
	}
	if err := validateAgainstInventory(semanticReport, inventoryReport); err != nil {
		t.Fatalf("owners covered across reciprocal features should pass: %v", err)
	}
}

func TestAuthenticatedValidationAcceptsNonCandidateWithNoBDDAction(t *testing.T) {
	tests := []struct {
		name     string
		features []string
	}{
		{name: "no selected feature"},
		{name: "existing reciprocal feature", features: []string{"agm/test/bdd/features/shared.feature"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, inventoryReport, semanticReport := auditFixture(t)
			nonCandidate := semanticReport.Candidates[0]
			nonCandidate.Rank = 0
			nonCandidate.Verdict = "keep-separate"
			nonCandidate.ProposedOwner = nil
			nonCandidate.BDD.Features = test.features
			nonCandidate.BDD.Consequence = "none"
			nonCandidate.Boundary = "The observables remain separately owned."
			semanticReport.Candidates = nil
			semanticReport.NonCandidates = []finding{nonCandidate}
			semanticReport.Summary.CandidateCount = 0
			semanticReport.Summary.ByVerdict = map[string]int{"keep-separate": 1}

			if err := validateReport(semanticReport); err != nil {
				t.Fatalf("no-action non-candidate should be structurally valid: %v", err)
			}
			if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
				t.Fatalf("inventory should authenticate against the pinned repository: %v", err)
			}
			if err := validateAgainstInventory(semanticReport, inventoryReport); err != nil {
				t.Fatalf("no-action non-candidate should authenticate: %v", err)
			}
		})
	}
}

func TestAuthenticatedValidationRejectsCurrentOwnerWithoutSelectedFeature(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{"agm/test/bdd/features/one-only.feature"}

	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("semantic report should be structurally valid: %v", err)
	}
	if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
		t.Fatalf("inventory should authenticate against the pinned repository: %v", err)
	}
	err := validateAgainstInventory(semanticReport, inventoryReport)
	if err == nil || !strings.Contains(err.Error(), "do not reciprocally name current owner \"two/SPEC.md") {
		t.Fatalf("uncovered current-owner error=%v, want authenticated owner-degree rejection", err)
	}
}

func TestAuthenticatedValidationRejectsBDDFeatureWithoutCurrentOwner(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{
		"agm/test/bdd/features/shared.feature",
		"agm/test/bdd/features/three-only.feature",
	}

	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("semantic report should be structurally valid: %v", err)
	}
	if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
		t.Fatalf("inventory should authenticate against the pinned repository: %v", err)
	}
	err := validateAgainstInventory(semanticReport, inventoryReport)
	if err == nil || !strings.Contains(err.Error(), "three-only.feature\" does not reciprocally name any current owner") {
		t.Fatalf("unrelated BDD feature error=%v, want current-owner rejection", err)
	}
}

func TestAuthenticatedValidationRejectsForgedEvidenceAndUnsafeVerdicts(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)

	tests := []struct {
		name   string
		mutate func(*report, *report)
	}{
		{
			name: "forged inventory",
			mutate: func(_ *report, inventory *report) {
				inventory.Inventory[0].SHA256 = strings.Repeat("f", 64)
			},
		},
		{
			name: "fake evidence line",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Evidence[0].Line++
			},
		},
		{
			name: "omitted owner evidence",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Evidence = semantic.Candidates[0].Evidence[:1]
			},
		},
		{
			name: "omitted current owner declaration",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].CurrentOwners = semantic.Candidates[0].CurrentOwners[:1]
			},
		},
		{
			name: "unrelated existing proposed owner",
			mutate: func(semantic *report, inventory *report) {
				_ = inventoryRequirement(t, *inventory, "three/SPEC.md")
				semantic.Candidates[0].ProposedOwner = &proposedOwnerClaim{Path: "three/SPEC.md", State: "existing", Rationale: "plausible but unrelated"}
			},
		},
		{
			name: "blank current owner rationale",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].CurrentOwners[0].Rationale = ""
			},
		},
		{
			name: "missing ownership completeness",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].OwnershipCompleteness = ""
			},
		},
		{
			name: "blank proposed owner rationale",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].ProposedOwner.Rationale = ""
			},
		},
		{
			name: "current owner marked new",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].ProposedOwner.State = "new"
			},
		},
		{
			name: "incomplete active matrix",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Applicability = semantic.Candidates[0].Applicability[:1]
			},
		},
		{
			name: "legacy shared disposition",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Applicability[0].Disposition = "shared"
			},
		},
		{
			name: "unknown applicability on positive finding",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Applicability[0].Disposition = "unknown"
			},
		},
		{
			name: "tentative positive",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Confidence = "tentative"
				semantic.Candidates[0].Strength = "moderate"
			},
		},
		{
			name: "missing rank",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Rank = 0
			},
		},
		{
			name: "one sided BDD",
			mutate: func(_ *report, inventory *report) {
				for index := range inventory.Features {
					if inventory.Features[index].Path == "agm/test/bdd/features/shared.feature" {
						inventory.Features[index].RelatedSpecs = []string{"one/SPEC.md"}
						return
					}
				}
				t.Fatal("shared feature missing from authenticated fixture")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			semantic := cloneReport(t, semanticReport)
			inventory := cloneReport(t, inventoryReport)
			test.mutate(&semantic, &inventory)
			if err := validateReport(semantic); err != nil {
				return
			}
			if err := validateAgainstInventory(semantic, inventory); err != nil {
				return
			}
			if err := validateInventoryAgainstRepo(inventory, repo); err != nil {
				return
			}
			t.Fatal("unsafe mutation passed authenticated validation")
		})
	}
}

func auditFixture(t *testing.T) (string, report, report) {
	t.Helper()
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When a request runs, the system shall preserve identity.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/shared.feature`\n- Feature: `agm/test/bdd/features/one-only.feature`\n")
	writeTestFile(t, repo, "two/SPEC.md", "# Two\n\n**TWO-01** When a request runs, the system shall preserve identity.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/shared.feature`\n- Feature: `agm/test/bdd/features/two-only.feature`\n")
	writeTestFile(t, repo, "three/SPEC.md", "# Three\n\n**THREE-01** When a separate request runs, the system shall emit an unrelated metric.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/three-only.feature`\n")
	writeTestFile(t, repo, "agm/test/bdd/features/shared.feature", "# SPEC: one/SPEC.md\n# RELATED-SPEC: two/SPEC.md\nFeature: Shared identity\n")
	writeTestFile(t, repo, "agm/test/bdd/features/one-only.feature", "# SPEC: one/SPEC.md\nFeature: One-only identity\n")
	writeTestFile(t, repo, "agm/test/bdd/features/two-only.feature", "# SPEC: two/SPEC.md\nFeature: Two-only identity\n")
	writeTestFile(t, repo, "agm/test/bdd/features/three-only.feature", "# SPEC: three/SPEC.md\nFeature: Three-only metric\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "fixture")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	inventoryReport, err := inventory(repo, "owner/repo", revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventoryReport.Inventory) != 3 {
		t.Fatalf("inventory files=%d, want 3", len(inventoryReport.Inventory))
	}
	first := inventoryRequirement(t, inventoryReport, "one/SPEC.md")
	second := inventoryRequirement(t, inventoryReport, "two/SPEC.md")
	semantic := report{
		SchemaVersion: schemaVersion,
		Snapshot: snapshot{
			Repository:          inventoryReport.Snapshot.Repository,
			Revision:            inventoryReport.Snapshot.Revision,
			RevisionCommittedAt: inventoryReport.Snapshot.RevisionCommittedAt,
			GeneratedAt:         "2026-07-31T12:00:00Z",
		},
		Scope: inventoryReport.Scope,
		Summary: summary{
			SpecFiles: 3, Requirements: 3, Diagnostics: 0, CandidateCount: 1,
			ByVerdict: map[string]int{"merge-now": 1},
		},
		Methodology: methodology{Collector: "specaudit inventory", SeedKinds: []string{"exact-body"}, SemanticReview: "source and BDD review", Reproduce: []string{"specaudit validate fixture"}},
		Candidates: []finding{{
			ID: "SPEC-CLUSTER-001", Rank: 1, Title: "Shared identity", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
			CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "ONE-01 normatively claims the shared request outcome."}, {Path: "two/SPEC.md", Rationale: "TWO-01 independently claims the same request outcome."}}, OwnershipCompleteness: "The exact-body seed and repository search found only these two normative paths.",
			ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "ONE-01 already states the complete shared observable."},
			SharedOutcome: "Requests preserve identity.", MaterialDifferences: []string{"Only the owner path differs."}, Evidence: []evidence{{Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}, {Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}},
			ApplicabilityBasis: "active-members", ApplicabilityRationale: "The shared contract applies to both pinned active members.",
			Applicability: []applicability{
				{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{{Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}}},
				{Member: "pi-cli", Disposition: "supported", Evidence: []evidence{{Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}}},
			},
			BDD: bddImpact{Features: []string{"agm/test/bdd/features/shared.feature"}, Consequence: "merge"}, Recommendation: []string{"Keep ONE-01 as canonical."}, Risk: "Traceability could be lost.", Decision: "Approve one owner.",
		}},
		NonCandidates: []finding{}, Limitations: append([]string{}, inventoryReport.Limitations...),
	}
	return repo, inventoryReport, semantic
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func cloneReport(t *testing.T, source report) report {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var cloned report
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func inventoryRequirement(t *testing.T, source report, path string) requirement {
	t.Helper()
	for _, file := range source.Inventory {
		if file.Path == path && len(file.Requirements) > 0 {
			return file.Requirements[0]
		}
	}
	t.Fatalf("inventory has no requirement for %s", path)
	return requirement{}
}

func validReport() report {
	return report{
		SchemaVersion: schemaVersion,
		Snapshot:      snapshot{Repository: "owner/repo", Revision: strings.Repeat("a", 40), RevisionCommittedAt: "2026-07-30T00:00:00Z", GeneratedAt: "2026-07-31T00:00:00Z"},
		Scope:         scope{Roots: []string{"."}, Excluded: []exclusion{}, ActiveMembers: []string{"codex-cli"}},
		Summary:       summary{SpecFiles: 1, Requirements: 1, CandidateCount: 0, ByVerdict: map[string]int{}},
		Methodology:   methodology{Collector: "test", SeedKinds: []string{"exact-body"}, SemanticReview: "review", Reproduce: []string{"specaudit inventory -repo . -revision abc"}},
		Candidates:    []finding{}, NonCandidates: []finding{}, Limitations: []string{},
	}
}

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gittest.Run(t, dir, args...)
}

func writeTestFile(t *testing.T, root, path, data string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
