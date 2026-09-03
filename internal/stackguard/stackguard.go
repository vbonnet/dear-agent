// Package stackguard decides whether a pull request that presents itself as
// part of a stack is actually wired as one.
//
// The stakes are not cosmetic. When each pull request's base is the previous
// one's head, merging the tip cascades: GitHub merges the whole chain in order
// and every pull request keeps its own commits, description and review history.
// When the descriptions merely say "stack" and every base targets the trunk,
// merging the tip lands it alone against the trunk and strands the siblings,
// which then have to be closed, destroying the descriptions and review threads
// they carried. A falsely labelled stack is a data-loss trap, not untidiness.
//
// A description can also announce "Stack 2/5" while the parent has been rebased
// out from under the child. That reads as a stack, is not one, and leaves
// review looking at a diff that contains the parent's work. The rules here turn
// both claims into something a machine can refute.
//
// The package is pure: callers supply the pull request, the set of open head
// branches, and an ancestry oracle, and receive findings. Every rule is
// evaluated from data GitHub and git already hold, never from prose judgment.
package stackguard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Finding codes. Blocking codes describe a description that misrepresents the
// branch topology; advisory codes describe hygiene that -strict promotes.
const (
	// CodeClaimContradictsBase reports a declared stack position that the
	// actual base ref cannot support.
	CodeClaimContradictsBase = "STACK-01"
	// CodeParentNotAPullRequest reports a non-trunk base branch that carries
	// no open pull request of its own.
	CodeParentNotAPullRequest = "STACK-02"
	// CodeStaleLink reports a base ref that is no longer an ancestor of the
	// head, which is a stack awaiting a restack.
	CodeStaleLink = "STACK-03"
	// CodeDeclaredBaseMismatch reports a body that names a base branch other
	// than the one the pull request targets.
	CodeDeclaredBaseMismatch = "STACK-04"
	// CodeUnmarkedChain reports a pull request wired onto a sibling whose
	// description never says so.
	CodeUnmarkedChain = "STACK-05"
	// CodeUnregisteredStack reports a chain GitHub has not been told about,
	// so `gh stack` cannot sync or restack it.
	CodeUnregisteredStack = "STACK-06"
	// CodeRegistrationMismatch reports a body whose announced position or
	// size disagrees with the registered stack.
	CodeRegistrationMismatch = "STACK-07"
	// CodeAncestryUnknown reports that the ancestry oracle failed, so the
	// structural rules could not be decided.
	CodeAncestryUnknown = "STACK-08"
)

// PullRequest is the pull request under review.
type PullRequest struct {
	Number  int
	Title   string
	Body    string
	BaseRef string
	HeadRef string

	// StackNumber, StackPosition and StackSize carry GitHub's registered
	// stack for this pull request. All three are zero when the pull request
	// belongs to no stack.
	StackNumber   int
	StackPosition int
	StackSize     int

	// LowerEntriesOpen records whether any registered entry below this one is
	// still open. A stack that has fully drained leaves nothing to orphan, so
	// its tip correctly targets the trunk.
	LowerEntriesOpen bool

	// RegistrationUnread records that the registration could not be read at
	// all, which must not be reported as an absent registration.
	RegistrationUnread bool
}

// Repository is the world the pull request is judged against.
type Repository struct {
	// Trunk is the branch a stack bottom legitimately targets.
	Trunk string
	// OpenHeads maps every open pull request's head branch to its number.
	OpenHeads map[string]int
	// OpenBases maps a branch to an open pull request that targets it. A
	// branch present here has something stacked on it, which is what makes a
	// trunk-targeting pull request a genuine stack bottom rather than a
	// lone claim.
	OpenBases map[string]int
	// Descends reports whether head is a descendant of base.
	Descends func(base, head string) (bool, error)
	// Strict promotes the advisory findings to blocking.
	Strict bool
}

// Finding is one rule outcome.
type Finding struct {
	Code     string
	Blocking bool
	Detail   string
	Remedy   string
}

// claimPattern matches the repository's canonical marker and the prose forms
// that have appeared in real descriptions. It deliberately requires the word
// "stack" to sit next to positional or dependency language, so that a call
// stack or a stack trace is not read as a claim.
var (
	canonicalPattern   = regexp.MustCompile(`(?im)^\s*stack\s+(\d+)\s*/\s*(\d+)\s*\.\s*base:\s*` + "`?" + `([^\s.` + "`" + `]+)`)
	positionPattern    = regexp.MustCompile(`(?i)\bstack\s+(\d+)\s*/\s*(\d+)\b`)
	ofStackPattern     = regexp.MustCompile(`(?i)\b(\d+)\s*/\s*(\d+)\s+of\s+(?:the\s+)?[^\n]{0,40}?(?:#\d+|stack)`)
	dependencyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bstacked on\b`),
		regexp.MustCompile(`(?i)\b(?:depends on|sits on top of) #\d+`),
	}
	affiliationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bpart of (?:the|a|this) [^\n]{0,40}?stack\b`),
		regexp.MustCompile(`(?i)\bstack of \d+\b`),
	}
)

// claimKind separates the claims that can be refuted from the ones that cannot.
// A positional or present-tense claim asserts where this pull request sits
// right now. An affiliation claim reads the same whether the stack drained or
// was never wired, so it can only ever be advisory.
type claimKind int

const (
	claimNone claimKind = iota
	claimPositional
	claimDependency
	claimAffiliation
)

// claim is what a description asserts about its own position.
type claim struct {
	present      bool
	kind         claimKind
	position     int
	size         int
	declaredBase string
}

// ClaimsStack reports whether the text presents itself as part of a stack.
func ClaimsStack(text string) bool {
	return parseClaim(text).present
}

func parseClaim(text string) claim {
	if match := canonicalPattern.FindStringSubmatch(text); match != nil {
		position, _ := strconv.Atoi(match[1])
		size, _ := strconv.Atoi(match[2])
		return claim{present: true, kind: claimPositional, position: position, size: size, declaredBase: match[3]}
	}
	if match := positionPattern.FindStringSubmatch(text); match != nil {
		position, _ := strconv.Atoi(match[1])
		size, _ := strconv.Atoi(match[2])
		return claim{present: true, kind: claimPositional, position: position, size: size}
	}
	for _, pattern := range dependencyPatterns {
		if pattern.MatchString(text) {
			return claim{present: true, kind: claimDependency}
		}
	}
	// "4/4 of the #1133 stack" is retrospective phrasing: it recounts a
	// position that may since have drained, in which case the tip correctly
	// targets the trunk. It is read as a claim, but not as a claim about where
	// this pull request is based right now.
	if ofStackPattern.MatchString(text) {
		return claim{present: true, kind: claimAffiliation}
	}
	for _, pattern := range affiliationPatterns {
		if pattern.MatchString(text) {
			return claim{present: true, kind: claimAffiliation}
		}
	}
	return claim{}
}

// Check evaluates every rule and returns the findings in rule order.
func Check(pr PullRequest, repo Repository) []Finding {
	eval := evaluation{
		pr:     pr,
		repo:   repo,
		stated: parseClaim(pr.Title + "\n" + pr.Body),
	}
	eval.onTrunk = pr.BaseRef == repo.Trunk
	eval.parent, eval.parentIsPullRequest = repo.OpenHeads[pr.BaseRef]
	eval.chained = !eval.onTrunk && eval.parentIsPullRequest
	eval.child, eval.hasChild = repo.OpenBases[pr.HeadRef]

	var findings []Finding
	for _, rule := range []func() []Finding{
		eval.claimContradictsBase,
		eval.parentNotAPullRequest,
		eval.linkIntegrity,
		eval.declaredBaseMismatch,
		eval.unmarkedChain,
	} {
		findings = append(findings, rule()...)
	}
	// Registration is only worth reporting once the topology itself is sound.
	// Telling an author to register a stack that is not yet a stack buries the
	// finding that actually needs acting on.
	if !Blocking(findings) {
		findings = append(findings, eval.unregisteredStack()...)
	}
	return append(findings, eval.registrationMismatch()...)
}

// evaluation carries the facts every rule shares, so each rule reads as one
// question rather than one branch of a long conditional.
type evaluation struct {
	pr                  PullRequest
	repo                Repository
	stated              claim
	onTrunk             bool
	parent              int
	parentIsPullRequest bool
	chained             bool
	child               int
	hasChild            bool
}

func (e evaluation) finding(code string, blocking bool, detail, remedy string) []Finding {
	return []Finding{{Code: code, Blocking: blocking, Detail: detail, Remedy: remedy}}
}

// advisory findings describe hygiene, and block only under -strict.
func (e evaluation) advisory(code, detail, remedy string) []Finding {
	return e.finding(code, e.repo.Strict, detail, remedy)
}

func (e evaluation) claimContradictsBase() []Finding {
	// A branch with something stacked on it is a real stack bottom. Targeting
	// the trunk is exactly right there, whatever the description says.
	if e.onTrunk && e.hasChild {
		return nil
	}
	if finding := e.registeredMidStackOverTrunk(); finding != nil {
		return finding
	}
	if !e.stated.present {
		return nil
	}
	if e.stated.position == 1 && !e.onTrunk {
		return e.finding(CodeClaimContradictsBase, true,
			fmt.Sprintf("body announces stack position 1/%d but the base ref is %q rather than the trunk %q",
				e.stated.size, e.pr.BaseRef, e.repo.Trunk),
			"the bottom of a stack targets the trunk; correct the position or the base ref")
	}
	if !e.onTrunk {
		return nil
	}
	switch {
	case e.stated.kind == claimPositional && e.stated.position > 1:
		return e.finding(CodeClaimContradictsBase, true,
			e.orphanDetail(fmt.Sprintf("body announces stack position %d/%d but the base ref is the trunk %q",
				e.stated.position, e.stated.size, e.repo.Trunk)),
			fmt.Sprintf("retarget the base at the head branch of stack position %d so a tip merge cascades, or drop the stack marker",
				e.stated.position-1))
	case e.stated.kind == claimDependency:
		return e.finding(CodeClaimContradictsBase, true,
			e.orphanDetail(fmt.Sprintf("body states this is stacked on another pull request but the base ref is the trunk %q",
				e.repo.Trunk)),
			"retarget the base at the head branch of the pull request this depends on, or drop the stack marker")
	case e.stated.kind == claimAffiliation:
		// Ambiguous by construction: a drained stack's tip reads identically
		// to one that was never wired, and blocking would make the guard
		// wrong about a correct pull request.
		return e.advisory(CodeClaimContradictsBase,
			fmt.Sprintf("body associates this with a stack but the base ref is the trunk %q and nothing is stacked on it; if the stack has not drained, a tip merge will orphan its siblings",
				e.repo.Trunk),
			"if lower pull requests are still open, retarget the base at the one below; if they have merged, say so in the description")
	}
	return nil
}

// registeredMidStackOverTrunk refutes the claim from GitHub's own record rather
// than from prose, which no wording can talk its way past.
func (e evaluation) registeredMidStackOverTrunk() []Finding {
	if e.pr.RegistrationUnread || e.pr.StackNumber == 0 || e.pr.StackPosition <= 1 || !e.onTrunk {
		return nil
	}
	// Nothing below is still open, so nothing can be orphaned.
	if !e.pr.LowerEntriesOpen {
		return nil
	}
	return e.finding(CodeClaimContradictsBase, true,
		e.orphanDetail(fmt.Sprintf("stack #%d records this at position %d of %d but the base ref is the trunk %q",
			e.pr.StackNumber, e.pr.StackPosition, e.pr.StackSize, e.repo.Trunk)),
		fmt.Sprintf("retarget the base at the head branch of position %d, or unstack this pull request", e.pr.StackPosition-1))
}

// orphanDetail appends the consequence, because the cost of this defect is not
// obvious from the topology alone.
func (e evaluation) orphanDetail(detail string) string {
	return detail + ", so merging the tip would land it alone against the trunk and orphan the other pull requests, destroying their descriptions and review history when they are closed"
}

func (e evaluation) parentNotAPullRequest() []Finding {
	if !e.stated.present || e.onTrunk || e.parentIsPullRequest {
		return nil
	}
	return e.finding(CodeParentNotAPullRequest, true,
		fmt.Sprintf("base ref %q carries no open pull request, so the claimed parent cannot be reviewed or merged first", e.pr.BaseRef),
		"open a pull request for the base branch, or retarget this one at the trunk")
}

func (e evaluation) linkIntegrity() []Finding {
	if !e.chained {
		return nil
	}
	descends, err := e.repo.Descends(e.pr.BaseRef, e.pr.HeadRef)
	switch {
	case err != nil:
		return e.finding(CodeAncestryUnknown, true,
			fmt.Sprintf("could not decide whether %q descends from %q: %v", e.pr.HeadRef, e.pr.BaseRef, err),
			"fetch both branches and rerun; an undecidable stack link fails closed")
	case !descends:
		return e.finding(CodeStaleLink, true,
			fmt.Sprintf("head %q does not descend from base %q (#%d), so this pull request's diff no longer isolates its own change",
				e.pr.HeadRef, e.pr.BaseRef, e.parent),
			"restack: rebase this branch onto the current parent head, then force-push with a lease")
	}
	return nil
}

func (e evaluation) declaredBaseMismatch() []Finding {
	if e.stated.declaredBase == "" || e.stated.declaredBase == e.pr.BaseRef {
		return nil
	}
	return e.finding(CodeDeclaredBaseMismatch, true,
		fmt.Sprintf("body declares base %q but the pull request targets %q", e.stated.declaredBase, e.pr.BaseRef),
		"make the declared base match the base ref, changing whichever one is wrong")
}

func (e evaluation) unmarkedChain() []Finding {
	if !e.chained || e.stated.present {
		return nil
	}
	return e.advisory(CodeUnmarkedChain,
		fmt.Sprintf("base ref %q is the head of #%d but the description never says this is stacked", e.pr.BaseRef, e.parent),
		"add the canonical marker: Stack <position>/<size>. Base: <branch>.")
}

func (e evaluation) unregisteredStack() []Finding {
	if e.pr.RegistrationUnread || e.pr.StackNumber != 0 || (!e.chained && !e.stated.present) {
		return nil
	}
	return e.advisory(CodeUnregisteredStack,
		"the pull request presents as stacked but belongs to no GitHub stack, so `gh stack` cannot sync or restack it",
		"register the chain bottom to top: gh stack link <pr> <pr> ...")
}

func (e evaluation) registrationMismatch() []Finding {
	if e.pr.RegistrationUnread || e.pr.StackNumber == 0 || e.stated.position == 0 {
		return nil
	}
	mismatch := describeRegistrationMismatch(e.stated, e.pr)
	if mismatch == "" {
		return nil
	}
	return e.finding(CodeRegistrationMismatch, true, mismatch,
		"update the marker to the registered position and size, or relink the stack in the intended order")
}

func describeRegistrationMismatch(stated claim, pr PullRequest) string {
	var problems []string
	if stated.position != pr.StackPosition {
		problems = append(problems, fmt.Sprintf("position %d but is registered at %d", stated.position, pr.StackPosition))
	}
	if stated.size != pr.StackSize {
		problems = append(problems, fmt.Sprintf("size %d but stack #%d holds %d", stated.size, pr.StackNumber, pr.StackSize))
	}
	if len(problems) == 0 {
		return ""
	}
	return "body announces " + strings.Join(problems, " and ")
}

// Blocking reports whether any finding must fail the check.
func Blocking(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Blocking {
			return true
		}
	}
	return false
}
