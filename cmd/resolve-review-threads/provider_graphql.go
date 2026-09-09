// GraphQL subprocess transport and payload-safe provider error classification.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const maxClassifiedProviderDiagnosticBytes = 4096

var providerAccessDeniedMarkers = []string{
	"resource not accessible by integration",
	"resource not accessible by personal access token",
	"must have push access",
	"must have push permission",
	"permission denied",
	"access denied",
	"bad credentials",
	"requires authentication",
	"authentication required",
	"unauthorized",
}

// providerAccessDeniedError carries only a redacted category and the process
// exit error. Raw provider diagnostics are intentionally absent because a
// body-bearing gh debug stream can contain the complete reply payload.
type providerAccessDeniedError struct {
	cause error
}

func (e *providerAccessDeniedError) Error() string {
	return fmt.Sprintf("provider access denied: %v", e.cause)
}

func (e *providerAccessDeniedError) Unwrap() error { return e.cause }

type streamMarker struct {
	pattern string
	matched int
	active  bool
}

func (m *streamMarker) reset() {
	m.matched = 0
	m.active = true
}

func (m *streamMarker) advance(b byte) bool {
	if !m.active || b != m.pattern[m.matched] {
		m.active = false
		return false
	}
	m.matched++
	return m.matched == len(m.pattern)
}

type diagnosticMatchStage uint8

const (
	diagnosticLeadingSpace diagnosticMatchStage = iota
	diagnosticGraphQLPrefix
	diagnosticGraphQLSpace
	diagnosticMarker
)

const graphqlDiagnosticPrefix = "graphql:"

// redactedProviderDiagnosticClassifier recognizes authorization failures from
// gh-owned diagnostic lines while retaining no stderr text. Debug request and
// response dumps start with other prefixes and are discarded before their
// content is inspected, so reply-body words cannot manufacture a category.
type redactedProviderDiagnosticClassifier struct {
	prefixMatched int
	discardLine   bool
	diagnostic    bool
	inspected     int
	accessDenied  bool
	markers       []streamMarker
	stage         diagnosticMatchStage
	graphqlMatch  int
}

func newRedactedProviderDiagnosticClassifier() *redactedProviderDiagnosticClassifier {
	c := &redactedProviderDiagnosticClassifier{
		markers: make([]streamMarker, 0, len(providerAccessDeniedMarkers)),
	}
	for _, marker := range providerAccessDeniedMarkers {
		c.markers = append(c.markers, streamMarker{pattern: marker, active: true})
	}
	return c
}

func (c *redactedProviderDiagnosticClassifier) Write(p []byte) (int, error) {
	for _, raw := range p {
		switch {
		case raw == '\n':
			c.resetLine()
		case c.discardLine || c.accessDenied:
		case !c.diagnostic:
			c.advanceTrustedPrefix(raw)
		default:
			c.advanceDiagnostic(raw)
		}
	}
	return len(p), nil
}

func (c *redactedProviderDiagnosticClassifier) advanceTrustedPrefix(raw byte) {
	const trustedPrefix = "gh:"
	if raw != trustedPrefix[c.prefixMatched] {
		c.discardLine = true
		return
	}
	c.prefixMatched++
	if c.prefixMatched == len(trustedPrefix) {
		c.diagnostic = true
		c.stage = diagnosticLeadingSpace
	}
}

func (c *redactedProviderDiagnosticClassifier) advanceDiagnostic(raw byte) {
	if c.inspected >= maxClassifiedProviderDiagnosticBytes {
		c.discardLine = true
		return
	}
	c.inspected++
	b := lowerASCII(raw)

	switch c.stage {
	case diagnosticLeadingSpace:
		c.advanceDiagnosticStart(b)
	case diagnosticGraphQLPrefix:
		c.advanceGraphQLPrefix(b)
	case diagnosticGraphQLSpace:
		if isProviderDiagnosticSpace(b) {
			return
		}
		c.stage = diagnosticMarker
		c.advanceMarkers(b)
	case diagnosticMarker:
		c.advanceMarkers(b)
	}
}

func (c *redactedProviderDiagnosticClassifier) advanceDiagnosticStart(b byte) {
	if isProviderDiagnosticSpace(b) {
		return
	}
	if b == graphqlDiagnosticPrefix[0] {
		c.stage = diagnosticGraphQLPrefix
		c.graphqlMatch = 1
		return
	}
	c.stage = diagnosticMarker
	c.advanceMarkers(b)
}

func (c *redactedProviderDiagnosticClassifier) advanceGraphQLPrefix(b byte) {
	if b != graphqlDiagnosticPrefix[c.graphqlMatch] {
		c.discardLine = true
		return
	}
	c.graphqlMatch++
	if c.graphqlMatch == len(graphqlDiagnosticPrefix) {
		c.stage = diagnosticGraphQLSpace
	}
}

func (c *redactedProviderDiagnosticClassifier) resetLine() {
	c.prefixMatched = 0
	c.discardLine = false
	c.diagnostic = false
	c.stage = diagnosticLeadingSpace
	c.graphqlMatch = 0
	for i := range c.markers {
		c.markers[i].reset()
	}
}

func (c *redactedProviderDiagnosticClassifier) advanceMarkers(b byte) {
	stillActive := false
	for i := range c.markers {
		if c.markers[i].advance(b) {
			c.accessDenied = true
			return
		}
		stillActive = stillActive || c.markers[i].active
	}
	if !stillActive {
		c.discardLine = true
	}
}

func (c *redactedProviderDiagnosticClassifier) AccessDenied() bool {
	return c.accessDenied
}

func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func isProviderDiagnosticSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

// ghGraphQL sends one typed GraphQL envelope through standard input and
// returns stdout. Query text and variable values never enter child argv.
// Body-free failures preserve gh's stderr. Body-bearing failures retain only
// the redacted access-denied category because gh debug output can echo the
// request envelope, including the reply body.
func ghGraphQL(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	payload, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{Query: query, Variables: variables})
	if err != nil {
		return nil, fmt.Errorf("encode gh api graphql request: %w", err)
	}

	// #nosec G702 -- fixed executable and fixed argv; request data is stdin.
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql", "--input", "-")
	cmd.Stdin = bytes.NewReader(payload)
	var out, errBuf bytes.Buffer
	classifier := newRedactedProviderDiagnosticClassifier()
	_, carriesReplyBody := variables["body"]
	cmd.Stdout = &out
	if carriesReplyBody {
		cmd.Stderr = classifier
		cmd.Env = withoutProviderDebugEnvironment(os.Environ())
	} else {
		cmd.Stderr = io.MultiWriter(&errBuf, classifier)
	}
	if err := cmd.Run(); err != nil {
		cause := err
		if classifier.AccessDenied() {
			cause = &providerAccessDeniedError{cause: err}
		}
		if carriesReplyBody {
			return nil, fmt.Errorf("gh api graphql: %w (provider diagnostics suppressed because the request contains a reply body)", cause)
		}
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("gh api graphql: %w: %s", cause, msg)
		}
		return nil, fmt.Errorf("gh api graphql: %w", cause)
	}
	return out.Bytes(), nil
}

func withoutProviderDebugEnvironment(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "GH_DEBUG") || strings.EqualFold(key, "DEBUG") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
