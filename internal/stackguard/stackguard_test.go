package stackguard_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/stackguard"
)

// chain builds a repository whose open pull requests are the supplied head
// branches and whose ancestry answers come from the supplied descendant set.
func chain(trunk string, heads map[string]int, descends map[string]bool) stackguard.Repository {
	return stackguard.Repository{
		Trunk:     trunk,
		OpenHeads: heads,
		Descends: func(base, head string) (bool, error) {
			return descends[base+".."+head], nil
		},
	}
}

// stackedOn records that a pull request targets the given branch, which is what
// makes the branch a genuine stack bottom rather than a lone claim.
func stackedOn(repo stackguard.Repository, base string, child int) stackguard.Repository {
	if repo.OpenBases == nil {
		repo.OpenBases = map[string]int{}
	}
	repo.OpenBases[base] = child
	return repo
}

func codes(findings []stackguard.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.Code)
	}
	return out
}

func requireCodes(t *testing.T, findings []stackguard.Finding, want ...string) {
	t.Helper()
	got := codes(findings)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("findings = %v, want %v", got, want)
	}
}

// The fixture that lies: the body announces a current mid-stack position while
// the base ref still points at the trunk, so nothing is stacked.
func TestClaimedPositionContradictsTrunkBase(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1400,
		Body:    "Stack 2/5. Base: refactor/parent.\n",
		BaseRef: "main",
		HeadRef: "fix/child",
	}
	repo := chain("main", map[string]int{"fix/child": 1400, "refactor/parent": 1399}, nil)

	findings := stackguard.Check(pr, repo)

	requireCodes(t, findings, stackguard.CodeClaimContradictsBase, stackguard.CodeDeclaredBaseMismatch)
	if !findings[0].Blocking {
		t.Fatalf("a claim contradicting the base must block")
	}
}

// A stack that has drained is the counter-case. Once every lower pull request
// has merged, the tip legitimately targets the trunk while its description
// still recounts the position it held. #1345 reads exactly this way, and
// blocking it would make the guard wrong about a correct pull request.
func TestRetrospectivePositionOverTrunkIsNotAViolation(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1345,
		Body:    "This is 4/4 of the #1133 SPEC-contract-guard stack. #1301, #1302 and\n#1303 have all merged; this is rebased directly onto main.\n",
		BaseRef: "main",
		HeadRef: "stack/4-digest-launch",
	}
	repo := chain("main", map[string]int{"stack/4-digest-launch": 1345}, nil)

	for _, finding := range stackguard.Check(pr, repo) {
		if finding.Blocking {
			t.Fatalf("a drained stack tip must not block, got %s: %s", finding.Code, finding.Detail)
		}
	}
}

// The fixture that is honestly wired: base is the parent's head, the head
// descends from it, and the registered stack agrees with the body.
func TestCorrectlyWiredStackPasses(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:        1380,
		Body:          "Stack 2/5. Base: refactor/ce-1hu9-84-durable-guards.\n",
		BaseRef:       "refactor/ce-1hu9-84-durable-guards",
		HeadRef:       "fix/ce-1hu9-84-exact-tmux",
		StackNumber:   1436,
		StackPosition: 2,
		StackSize:     5,
	}
	repo := chain("main",
		map[string]int{
			"refactor/ce-1hu9-84-durable-guards": 1379,
			"fix/ce-1hu9-84-exact-tmux":          1380,
		},
		map[string]bool{"refactor/ce-1hu9-84-durable-guards..fix/ce-1hu9-84-exact-tmux": true},
	)

	if findings := stackguard.Check(pr, repo); len(findings) != 0 {
		t.Fatalf("a correctly wired stack must be clean, got %v", codes(findings))
	}
}

// PR #1380 as it actually stood: base ref correct, but the parent was rebased
// and the child was never restacked, so the head no longer descends from it.
func TestStaleLinkAfterParentRebase(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:        1380,
		Body:          "Stack 2/5. Base: refactor/ce-1hu9-84-durable-guards.\n",
		BaseRef:       "refactor/ce-1hu9-84-durable-guards",
		HeadRef:       "fix/ce-1hu9-84-exact-tmux",
		StackNumber:   1436,
		StackPosition: 2,
		StackSize:     5,
	}
	repo := chain("main",
		map[string]int{
			"refactor/ce-1hu9-84-durable-guards": 1379,
			"fix/ce-1hu9-84-exact-tmux":          1380,
		},
		nil, // head does not descend from base
	)

	findings := stackguard.Check(pr, repo)

	requireCodes(t, findings, stackguard.CodeStaleLink)
	if !findings[0].Blocking {
		t.Fatalf("a stale stack link must block")
	}
	if !strings.Contains(findings[0].Remedy, "restack") {
		t.Fatalf("remedy should name the restack, got %q", findings[0].Remedy)
	}
}

// A body may name a base branch that is not the base the pull request actually
// targets. That is the hand-written stack section with no real wiring.
func TestDeclaredBaseDiffersFromActualBase(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:        1381,
		Body:          "Stack 3/5. Base: fix/ce-1hu9-84-exact-tmux.\n",
		BaseRef:       "fix/ce-1hu9-84-compaction-ledger",
		HeadRef:       "fix/ce-1hu9-84-positive-evidence",
		StackNumber:   1436,
		StackPosition: 3,
		StackSize:     5,
	}
	repo := chain("main",
		map[string]int{
			"fix/ce-1hu9-84-exact-tmux":        1380,
			"fix/ce-1hu9-84-compaction-ledger": 1381,
			"fix/ce-1hu9-84-positive-evidence": 1382,
		},
		map[string]bool{"fix/ce-1hu9-84-compaction-ledger..fix/ce-1hu9-84-positive-evidence": true},
	)

	requireCodes(t, stackguard.Check(pr, repo), stackguard.CodeDeclaredBaseMismatch)
}

// Claiming a stack while basing on a branch that carries no open pull request
// leaves a reader with no sibling to navigate to.
func TestClaimedParentHasNoOpenPullRequest(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1400,
		Body:    "Stacked on the queue refactor.\n",
		BaseRef: "refactor/some-abandoned-branch",
		HeadRef: "fix/child",
	}
	repo := chain("main", map[string]int{"fix/child": 1400},
		map[string]bool{"refactor/some-abandoned-branch..fix/child": true})

	requireCodes(t, stackguard.Check(pr, repo), stackguard.CodeParentNotAPullRequest)
}

// The inverse: correctly wired onto a sibling but the description never says
// so, which is how a reviewer merges a child before its parent.
func TestWiredChainWithoutMarkerIsAdvisory(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1392,
		Body:    "Stops the unsafe queue fallback.\n",
		BaseRef: "refactor/ce-1hu9-9-2-queue-create-attest",
		HeadRef: "fix/ce-1hu9-9-2-queue-no-fallback",
	}
	repo := chain("main",
		map[string]int{
			"refactor/ce-1hu9-9-2-queue-create-attest": 1391,
			"fix/ce-1hu9-9-2-queue-no-fallback":        1392,
		},
		map[string]bool{"refactor/ce-1hu9-9-2-queue-create-attest..fix/ce-1hu9-9-2-queue-no-fallback": true},
	)

	findings := stackguard.Check(pr, repo)

	requireCodes(t, findings, stackguard.CodeUnmarkedChain, stackguard.CodeUnregisteredStack)
	for _, finding := range findings {
		if finding.Blocking {
			t.Fatalf("%s must be advisory by default", finding.Code)
		}
	}
}

func TestStrictPromotesAdvisoryFindings(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1392,
		BaseRef: "refactor/parent",
		HeadRef: "fix/child",
	}
	repo := chain("main",
		map[string]int{"refactor/parent": 1391, "fix/child": 1392},
		map[string]bool{"refactor/parent..fix/child": true},
	)
	repo.Strict = true

	for _, finding := range stackguard.Check(pr, repo) {
		if !finding.Blocking {
			t.Fatalf("%s must block under -strict", finding.Code)
		}
	}
}

// A registered stack whose shape disagrees with the announced position is a
// description that lies in the other direction.
func TestRegisteredPositionDisagreesWithBody(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:        1383,
		Body:          "Stack 4/5. Base: fix/parent.\n",
		BaseRef:       "fix/parent",
		HeadRef:       "fix/child",
		StackNumber:   1436,
		StackPosition: 5,
		StackSize:     5,
	}
	repo := chain("main",
		map[string]int{"fix/parent": 1382, "fix/child": 1383},
		map[string]bool{"fix/parent..fix/child": true},
	)

	requireCodes(t, stackguard.Check(pr, repo), stackguard.CodeRegistrationMismatch)
}

// The bottom of a stack legitimately targets the trunk.
func TestBottomOfStackOnTrunkPasses(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:        1379,
		Body:          "Stack 1/5. Base: main.\n",
		BaseRef:       "main",
		HeadRef:       "refactor/ce-1hu9-84-durable-guards",
		StackNumber:   1436,
		StackPosition: 1,
		StackSize:     5,
	}
	repo := chain("main", map[string]int{"refactor/ce-1hu9-84-durable-guards": 1379}, nil)

	if findings := stackguard.Check(pr, repo); len(findings) != 0 {
		t.Fatalf("stack bottom on trunk must be clean, got %v", codes(findings))
	}
}

// An ordinary pull request that never mentions a stack is not this check's
// business.
func TestUnstackedPullRequestIsIgnored(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1404,
		Body:    "Restores parser locality.\n",
		BaseRef: "main",
		HeadRef: "refactor/instructionlint-parser",
	}
	repo := chain("main", map[string]int{"refactor/instructionlint-parser": 1404}, nil)

	if findings := stackguard.Check(pr, repo); len(findings) != 0 {
		t.Fatalf("a plain pull request must be clean, got %v", codes(findings))
	}
}

// Registration is a preview field. When it cannot be read the structural rules
// must still run and the registration rules must stay silent.
func TestUnknownRegistrationSuppressesRegistrationRules(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:             1380,
		Body:               "Stack 2/5. Base: refactor/parent.\n",
		BaseRef:            "refactor/parent",
		HeadRef:            "fix/child",
		RegistrationUnread: true,
	}
	repo := chain("main",
		map[string]int{"refactor/parent": 1379, "fix/child": 1380},
		nil, // still stale
	)

	requireCodes(t, stackguard.Check(pr, repo), stackguard.CodeStaleLink)
}

func TestAncestryErrorSurfacesAsOperationalFailure(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1380,
		Body:    "Stack 2/5. Base: refactor/parent.\n",
		BaseRef: "refactor/parent",
		HeadRef: "fix/child",
	}
	repo := stackguard.Repository{
		Trunk:     "main",
		OpenHeads: map[string]int{"refactor/parent": 1379, "fix/child": 1380},
		Descends: func(string, string) (bool, error) {
			return false, errors.New("object not found")
		},
	}

	findings := stackguard.Check(pr, repo)

	requireCodes(t, findings, stackguard.CodeAncestryUnknown)
	if !findings[0].Blocking {
		t.Fatalf("an unreadable ancestry must fail closed")
	}
}

func TestMarkerRecognisesTheRepositoryProseForms(t *testing.T) {
	for _, body := range []string{
		"Stack 2/5. Base: fix/parent.",
		"stack 2 / 5",
		"This is 4/4 of the #1133 stack.",
		"Stacked on #1379.",
		"Part of the queue-privacy stack.",
	} {
		if !stackguard.ClaimsStack(body) {
			t.Fatalf("body %q should read as a stack claim", body)
		}
	}
	for _, body := range []string{
		"Restores parser locality.",
		"Fixes the call stack overflow in the parser.",
		"Adds a stack trace to the error path.",
	} {
		if stackguard.ClaimsStack(body) {
			t.Fatalf("body %q should not read as a stack claim", body)
		}
	}
}

// The harmful shape: several pull requests describe themselves as one stack
// while every base ref targets the trunk. Merging the tip lands it alone
// against the trunk and strands the siblings, whose descriptions and review
// history are then lost when they are closed. A claim with no position number
// must not escape that with an advisory.
func TestPresentTenseDependencyOverTrunkBlocks(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1500,
		Body:    "Stacked on #1499.\n",
		BaseRef: "main",
		HeadRef: "fix/child",
	}
	repo := chain("main", map[string]int{"fix/child": 1500, "fix/parent": 1499}, nil)

	findings := stackguard.Check(pr, repo)

	requireCodes(t, findings, stackguard.CodeClaimContradictsBase)
	if !findings[0].Blocking {
		t.Fatalf("a present-tense dependency over a trunk base must block")
	}
	if !strings.Contains(findings[0].Detail, "orphan") {
		t.Fatalf("the finding must name the consequence, got %q", findings[0].Detail)
	}
}

// A genuine stack bottom targets the trunk and says so. Something is stacked on
// it, which is what tells the two cases apart.
func TestGenuineStackBottomWithDependencyClaimPasses(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:        1379,
		Body:          "Part of the compaction-evidence stack.\n",
		BaseRef:       "main",
		HeadRef:       "refactor/parent",
		StackNumber:   1436,
		StackPosition: 1,
		StackSize:     5,
	}
	repo := stackedOn(chain("main", map[string]int{"refactor/parent": 1379, "fix/child": 1380}, nil),
		"refactor/parent", 1380)

	if findings := stackguard.Check(pr, repo); len(findings) != 0 {
		t.Fatalf("a real stack bottom must be clean, got %v", codes(findings))
	}
}

// Registration is authoritative. A pull request GitHub records mid-stack while
// its base targets the trunk cannot cascade, whatever the prose says, so long
// as something below it is still open to be orphaned.
func TestRegisteredMidStackOverTrunkBlocks(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:           1501,
		Body:             "Adds the thing.\n",
		BaseRef:          "main",
		HeadRef:          "fix/child",
		StackNumber:      1436,
		StackPosition:    3,
		StackSize:        5,
		LowerEntriesOpen: true,
	}
	repo := chain("main", map[string]int{"fix/child": 1501}, nil)

	findings := stackguard.Check(pr, repo)

	requireCodes(t, findings, stackguard.CodeClaimContradictsBase)
	if !findings[0].Blocking {
		t.Fatalf("a registered mid-stack pull request over the trunk must block")
	}
}

// An affiliation claim over a trunk base with nothing stacked on it is the
// ambiguous case: it reads the same whether the stack drained or was never
// wired. It stays advisory so the guard is never wrong about a correct tip.
func TestAffiliationClaimOverTrunkIsAdvisory(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:  1502,
		Body:    "Part of the queue-privacy stack.\n",
		BaseRef: "main",
		HeadRef: "fix/tip",
	}
	repo := chain("main", map[string]int{"fix/tip": 1502}, nil)

	for _, finding := range stackguard.Check(pr, repo) {
		if finding.Blocking {
			t.Fatalf("an ambiguous affiliation claim must not block, got %s", finding.Code)
		}
	}
}

// #1218 is the counter-case: registered at position 4 of 4, based on the trunk,
// but every entry below it has merged. There is nothing left to orphan, so the
// trunk base is correct and blocking it would make the guard wrong about a
// correct pull request.
func TestRegisteredTipOverTrunkWithDrainedLowerEntriesPasses(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:           1218,
		Title:            "feat(router): quota-aware model routing from the CodexBar meter",
		BaseRef:          "main",
		HeadRef:          "feat/codexbar-quota-meter",
		StackNumber:      1329,
		StackPosition:    4,
		StackSize:        4,
		LowerEntriesOpen: false,
	}
	repo := chain("main", map[string]int{"feat/codexbar-quota-meter": 1218}, nil)

	for _, finding := range stackguard.Check(pr, repo) {
		if finding.Blocking {
			t.Fatalf("a drained registered stack tip must not block, got %s: %s", finding.Code, finding.Detail)
		}
	}
}

// A pull request that documents stacks is not claiming to be in one. This is
// the body of #1439, the pull request that introduced this package: it quotes
// markers in prose and in a table of examples. Reading those as claims blocked
// the guard's own pull request, which is how the defect was found.
func TestDocumentationAboutStacksIsNotAClaim(t *testing.T) {
	body := "" +
		"#1380 and #1381 both carried \"Stack N/5\" markers and both had correct base refs.\n\n" +
		"STACK-01 separates three kinds of claim:\n\n" +
		"| Claim | Example | Over a trunk base |\n" +
		"| --- | --- | --- |\n" +
		"| Positional | \"Stack 2/5\" | **blocks** |\n" +
		"| Present-tense dependency | \"Stacked on #1379\" | **blocks** |\n" +
		"| Affiliation | \"part of the X stack\" | advisory |\n\n" +
		"Mark each description with the canonical marker:\n\n" +
		"```\nStack 2/5. Base: refactor/parent.\n```\n\n" +
		"See `Stacked on #1379` for the dependency form.\n"

	pr := stackguard.PullRequest{
		Number:  1439,
		Title:   "feat(stack): refuse pull requests that only claim to be stacked",
		Body:    body,
		BaseRef: "main",
		HeadRef: "feat/stack-integrity-guard",
	}
	repo := chain("main", map[string]int{"feat/stack-integrity-guard": 1439}, nil)

	if findings := stackguard.Check(pr, repo); len(findings) != 0 {
		t.Fatalf("documentation about stacks must not read as a claim, got %v", codes(findings))
	}
	if stackguard.ClaimsStack(body) {
		t.Fatalf("quoted, tabulated and fenced markers must not count as a claim")
	}
}

// The canonical marker on its own line is still a claim, even in a body that
// also contains fenced examples.
func TestRealMarkerBesideDocumentationStillCounts(t *testing.T) {
	body := "Stack 2/5. Base: refactor/parent.\n\nSee the table:\n\n| x | \"Stack 9/9\" |\n"
	if !stackguard.ClaimsStack(body) {
		t.Fatalf("a real line-anchored marker must still be a claim")
	}
}

// Descends is an injected oracle. A Repository built without it must fail
// closed on the ancestry question rather than panicking the whole guard.
func TestCheckWithoutAncestryOracleFailsClosed(t *testing.T) {
	pr := stackguard.PullRequest{
		Number:        2,
		Title:         "second slice",
		Body:          "Stacked on #1.",
		BaseRef:       "stack/one",
		HeadRef:       "stack/two",
		StackNumber:   1,
		StackSize:     2,
		StackPosition: 2,
	}

	findings := stackguard.Check(pr, stackguard.Repository{
		Trunk:     "main",
		OpenBases: map[string]int{"stack/one": 1},
		OpenHeads: map[string]int{"stack/one": 1, "stack/two": 2},
	})

	if !stackguard.Blocking(findings) {
		t.Fatalf("stackguard.Check() = %+v, want a blocking finding with no ancestry oracle", findings)
	}
	found := false
	for _, f := range findings {
		if f.Code == stackguard.CodeAncestryUnknown {
			found = true
		}
	}
	if !found {
		t.Errorf("findings %+v do not include %s", findings, stackguard.CodeAncestryUnknown)
	}
}
