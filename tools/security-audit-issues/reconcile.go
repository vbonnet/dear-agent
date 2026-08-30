package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	issueLabel        = "security-audit"
	issueTitle        = "security-audit: workflow findings"
	issueLabelColor   = "d93f0b"
	issueLabelDetail  = "CI workflow hygiene finding from the Security Audit"
	managedMarker     = "<!-- managed-by: tools/security-audit-issues -->"
	digestPrefix      = "<!-- findings-sha256: "
	observationPrefix = "The Security Audit found CI hygiene issues as of "
	cleanComment      = "All workflow hygiene checks pass; auto-closing."
	providerTimeout   = 30 * time.Second
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type findings struct {
	compromised       string
	unpinned          string
	permissions       string
	pullRequestTarget string
}

func findingsFromEnvironment(getenv func(string) string) findings {
	return findings{
		compromised:       getenv("compromised"),
		unpinned:          getenv("unpinned"),
		permissions:       getenv("perm_findings"),
		pullRequestTarget: getenv("prt_hits"),
	}
}

func (f findings) any() bool {
	return strings.TrimSpace(f.compromised) != "" ||
		strings.TrimSpace(f.unpinned) != "" ||
		strings.TrimSpace(f.permissions) != "" ||
		strings.TrimSpace(f.pullRequestTarget) != ""
}

type providerRunner interface {
	run(context.Context, string, ...string) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) run(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, detail)
	}
	return stdout.Bytes(), nil
}

type reconciliation struct {
	action    reconciliationAction
	canonical int
	closed    []int
	ignored   int
}

type reconciliationAction uint8

const (
	actionClean reconciliationAction = iota
	actionCreated
	actionUpdated
	actionUnchanged
	actionClosed
)

func (r reconciliation) summary() string {
	ignored := ""
	if r.ignored > 0 {
		ignored = fmt.Sprintf("; unrelated labelled items ignored: %d", r.ignored)
	}
	switch r.action {
	case actionCreated:
		return "security-audit issue created" + ignored
	case actionUpdated:
		if len(r.closed) == 0 {
			return fmt.Sprintf("security-audit issue #%d updated%s", r.canonical, ignored)
		}
		return fmt.Sprintf("security-audit issue #%d updated; duplicate issues closed: %v%s", r.canonical, r.closed, ignored)
	case actionUnchanged:
		if len(r.closed) == 0 {
			return fmt.Sprintf("security-audit issue #%d already current%s", r.canonical, ignored)
		}
		return fmt.Sprintf("security-audit issue #%d already current; duplicate issues closed: %v%s", r.canonical, r.closed, ignored)
	case actionClosed:
		return fmt.Sprintf("security-audit clean; command-owned issues closed: %v%s", r.closed, ignored)
	case actionClean:
		return "security-audit clean; no command-owned issue is open" + ignored
	default:
		return "security-audit reconciliation completed with an unknown outcome"
	}
}

func reconcile(ctx context.Context, runner providerRunner, repository string, snapshot findings, observedAt time.Time) (reconciliation, error) {
	if err := validateRepository(repository); err != nil {
		return reconciliation{}, err
	}
	if err := ensureLabel(ctx, runner, repository); err != nil {
		return reconciliation{}, err
	}
	inventory, err := listOpenIssues(ctx, runner, repository)
	if err != nil {
		return reconciliation{}, err
	}
	if !snapshot.any() {
		closed := make([]int, 0, len(inventory.owned))
		for _, issue := range inventory.owned {
			if err := closeIssue(ctx, runner, repository, issue.number); err != nil {
				return reconciliation{}, err
			}
			closed = append(closed, issue.number)
		}
		if len(closed) == 0 {
			return reconciliation{action: actionClean, ignored: inventory.ignored}, nil
		}
		return reconciliation{action: actionClosed, closed: closed, ignored: inventory.ignored}, nil
	}

	body := renderIssueBody(snapshot, observedAt)
	if len(inventory.owned) == 0 {
		if err := createIssue(ctx, runner, repository, body); err != nil {
			return reconciliation{}, err
		}
		return reconciliation{action: actionCreated, ignored: inventory.ignored}, nil
	}

	canonical := inventory.owned[0]
	action := actionUnchanged
	if !bodyMatchesSnapshot(canonical.body, snapshot) {
		if err := updateIssue(ctx, runner, repository, canonical.number, body); err != nil {
			return reconciliation{}, err
		}
		action = actionUpdated
	}
	closed := make([]int, 0, len(inventory.owned)-1)
	for _, duplicate := range inventory.owned[1:] {
		comment := fmt.Sprintf("Duplicate of the command-owned rolling issue #%d; auto-closing.", canonical.number)
		if err := closeIssueWithComment(ctx, runner, repository, duplicate.number, comment); err != nil {
			return reconciliation{}, err
		}
		closed = append(closed, duplicate.number)
	}
	return reconciliation{
		action:    action,
		canonical: canonical.number,
		closed:    closed,
		ignored:   inventory.ignored,
	}, nil
}

func validateRepository(repository string) error {
	if !repositoryPattern.MatchString(repository) {
		return errors.New("-repo or GITHUB_REPOSITORY must be an owner/repository pair")
	}
	owner, name, _ := strings.Cut(repository, "/")
	if owner == "." || owner == ".." || name == "." || name == ".." {
		return errors.New("-repo or GITHUB_REPOSITORY contains an invalid path segment")
	}
	return nil
}

type issueRow struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	PullRequest *struct{} `json:"pull_request"`
}

type openIssue struct {
	number int
	body   string
}

type issueInventory struct {
	owned   []openIssue
	ignored int
}

func listOpenIssues(ctx context.Context, runner providerRunner, repository string) (issueInventory, error) {
	endpoint := "repos/" + repository + "/issues?state=open&labels=" + issueLabel + "&per_page=100"
	out, err := runProvider(ctx, runner, "", "api", "--paginate", "--slurp", endpoint)
	if err != nil {
		return issueInventory{}, fmt.Errorf("list open security-audit issues: %w", err)
	}
	var pages [][]issueRow
	if err := json.Unmarshal(out, &pages); err != nil {
		return issueInventory{}, fmt.Errorf("parse open security-audit issues: %w", err)
	}
	if len(pages) == 0 {
		return issueInventory{}, errors.New("parse open security-audit issues: expected at least one response page")
	}
	seen := make(map[int]bool)
	inventory := issueInventory{}
	for page, rows := range pages {
		if rows == nil {
			return issueInventory{}, fmt.Errorf("parse open security-audit issues: page %d is not an array", page+1)
		}
		for _, row := range rows {
			if row.Number <= 0 {
				return issueInventory{}, fmt.Errorf("parse open security-audit issues: invalid issue number %d", row.Number)
			}
			if seen[row.Number] {
				continue
			}
			seen[row.Number] = true
			if row.PullRequest != nil || row.Title != issueTitle {
				inventory.ignored++
				continue
			}
			inventory.owned = append(inventory.owned, openIssue{number: row.Number, body: row.Body})
		}
	}
	slices.SortFunc(inventory.owned, func(a, b openIssue) int { return cmp.Compare(a.number, b.number) })
	return inventory, nil
}

func ensureLabel(ctx context.Context, runner providerRunner, repository string) error {
	_, err := runProvider(ctx, runner, "", "label", "create", issueLabel,
		"--repo", repository, "--color", issueLabelColor,
		"--description", issueLabelDetail, "--force")
	if err != nil {
		return fmt.Errorf("reconcile security-audit label: %w", err)
	}
	return nil
}

func createIssue(ctx context.Context, runner providerRunner, repository, body string) error {
	_, err := runProvider(ctx, runner, body, "issue", "create", "--repo", repository,
		"--title", issueTitle, "--body-file", "-", "--label", issueLabel)
	if err != nil {
		return fmt.Errorf("create security-audit issue: %w", err)
	}
	return nil
}

func updateIssue(ctx context.Context, runner providerRunner, repository string, number int, body string) error {
	_, err := runProvider(ctx, runner, body, "issue", "edit", strconv.Itoa(number),
		"--repo", repository, "--body-file", "-")
	if err != nil {
		return fmt.Errorf("update security-audit issue #%d: %w", number, err)
	}
	return nil
}

func closeIssue(ctx context.Context, runner providerRunner, repository string, number int) error {
	return closeIssueWithComment(ctx, runner, repository, number, cleanComment)
}

func closeIssueWithComment(ctx context.Context, runner providerRunner, repository string, number int, comment string) error {
	_, err := runProvider(ctx, runner, "", "issue", "close", strconv.Itoa(number),
		"--repo", repository, "--comment", comment)
	if err != nil {
		return fmt.Errorf("close security-audit issue #%d: %w", number, err)
	}
	return nil
}

func runProvider(ctx context.Context, runner providerRunner, stdin string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	return runner.run(commandCtx, stdin, args...)
}

func renderIssueBody(snapshot findings, observedAt time.Time) string {
	return fmt.Sprintf(`%s
%s
%s%s:

### Known-compromised action versions
%s

### Third-party actions not pinned by SHA
%s

### Permissions findings
%s

### pull_request_target usage
%s

See %s for the rules. Auto-managed; closes once every check passes.
`, managedMarker, findingsDigestMarker(snapshot), observationPrefix,
		observedAt.UTC().Format(time.RFC3339), markdownFinding(snapshot.compromised),
		codeFinding(snapshot.unpinned), markdownFinding(snapshot.permissions),
		codeFinding(snapshot.pullRequestTarget), "`.github/workflows/security-audit.yml`")
}

func bodyMatchesSnapshot(body string, snapshot findings) bool {
	lines := strings.Split(body, "\n")
	if len(lines) < 3 || lines[0] != managedMarker || lines[1] != findingsDigestMarker(snapshot) {
		return false
	}
	timestamp := strings.TrimSuffix(strings.TrimPrefix(lines[2], observationPrefix), ":")
	if timestamp == lines[2] || !strings.HasSuffix(lines[2], ":") {
		return false
	}
	observedAt, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false
	}
	return body == renderIssueBody(snapshot, observedAt)
}

func findingsDigestMarker(snapshot findings) string {
	var encoded strings.Builder
	for _, value := range [...]string{
		snapshot.compromised,
		snapshot.unpinned,
		snapshot.permissions,
		snapshot.pullRequestTarget,
	} {
		value = strings.TrimSpace(value)
		encoded.WriteString(strconv.Itoa(len(value)))
		encoded.WriteByte(':')
		encoded.WriteString(value)
	}
	digest := sha256.Sum256([]byte(encoded.String()))
	return fmt.Sprintf("%s%x -->", digestPrefix, digest)
}

func markdownFinding(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_(none)_"
	}
	return value
}

func codeFinding(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "(none)"
	}
	return "```\n" + value + "\n```"
}
