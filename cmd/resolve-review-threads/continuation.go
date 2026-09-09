package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	continuationReceiptVersion       = 3
	maxContinuationReceiptTokenBytes = 4_096
	maxContinuationReceiptIDBytes    = 512
	continuationReceiptMACDomain     = "dear-agent/resolve-review-threads/continuation/v3\x00"
)

// continuationReceipt binds a resolve-only retry to the exact provider
// evidence established by reply-resolve. It deliberately carries neither the
// reply body nor its local source path.
type continuationReceipt struct {
	Version       int    `json:"version"`
	ProviderHost  string `json:"provider_host"`
	ThreadID      string `json:"thread_id"`
	PredecessorID string `json:"predecessor_id"`
	ReplyID       string `json:"reply_id"`
	// Both bodies are bound because GitHub permits editing a review comment
	// without changing its node ID. IDs and adjacency alone therefore cannot
	// prove that the reply still answers the predecessor that was read.
	PredecessorBodySHA256 string `json:"predecessor_body_sha256"`
	BodySHA256            string `json:"body_sha256"`
	PredecessorUpdatedAt  string `json:"predecessor_updated_at"`
	ReplyUpdatedAt        string `json:"reply_updated_at"`
	PredecessorEditCount  int    `json:"predecessor_edit_count"`
	ReplyEditCount        int    `json:"reply_edit_count"`
	PredecessorLastEditID string `json:"predecessor_last_edit_id"`
	ReplyLastEditID       string `json:"reply_last_edit_id"`
	OpeningAuthor         string `json:"opening_author"`
	ReplyAuthor           string `json:"reply_author"`
}

func newContinuationReceipt(
	providerHost string,
	threadID, predecessorID, replyID string,
	predecessorBody, body []byte,
	predecessorUpdatedAt, replyUpdatedAt string,
	predecessorEditCount, replyEditCount int,
	predecessorLastEditID, replyLastEditID string,
	openingAuthor, replyAuthor string,
) (continuationReceipt, error) {
	receipt := continuationReceipt{
		Version:               continuationReceiptVersion,
		ProviderHost:          providerHost,
		ThreadID:              threadID,
		PredecessorID:         predecessorID,
		ReplyID:               replyID,
		PredecessorBodySHA256: exactBodySHA256(predecessorBody),
		BodySHA256:            exactBodySHA256(body),
		PredecessorUpdatedAt:  predecessorUpdatedAt,
		ReplyUpdatedAt:        replyUpdatedAt,
		PredecessorEditCount:  predecessorEditCount,
		ReplyEditCount:        replyEditCount,
		PredecessorLastEditID: predecessorLastEditID,
		ReplyLastEditID:       replyLastEditID,
		OpeningAuthor:         openingAuthor,
		ReplyAuthor:           replyAuthor,
	}
	if err := receipt.validate(); err != nil {
		return continuationReceipt{}, err
	}
	return receipt, nil
}

// continuationIssuer holds every local input needed to mint a continuation
// receipt. Preparing it before the reply mutation ensures a successful post
// cannot be stranded by a later signing-key or provider-host failure.
type continuationIssuer struct {
	key          []byte
	providerHost string
}

// continuationIssuerStateError means the token had the bounded authenticated
// envelope shape, but the local key state could not establish whether it was
// issued here. It is deliberately distinct from malformed receipt data so
// recovery preserves the exact token and points back to the issuing state.
type continuationIssuerStateError struct {
	cause error
}

func (e *continuationIssuerStateError) Error() string {
	return e.cause.Error()
}

func (e *continuationIssuerStateError) Unwrap() error {
	return e.cause
}

func prepareContinuationIssuer() (continuationIssuer, error) {
	providerHost, err := effectiveContinuationProviderHost()
	if err != nil {
		return continuationIssuer{}, err
	}
	key, err := loadOrCreateContinuationReceiptKey()
	if err != nil {
		return continuationIssuer{}, err
	}
	return continuationIssuer{
		key:          key,
		providerHost: providerHost,
	}, nil
}

func (i continuationIssuer) issue(
	threadID, predecessorID, replyID string,
	predecessorBody, body []byte,
	predecessorUpdatedAt, replyUpdatedAt string,
	predecessorEditCount, replyEditCount int,
	predecessorLastEditID, replyLastEditID string,
	openingAuthor, replyAuthor string,
) (string, error) {
	receipt, err := newContinuationReceipt(
		i.providerHost,
		threadID,
		predecessorID,
		replyID,
		predecessorBody,
		body,
		predecessorUpdatedAt,
		replyUpdatedAt,
		predecessorEditCount,
		replyEditCount,
		predecessorLastEditID,
		replyLastEditID,
		openingAuthor,
		replyAuthor,
	)
	if err != nil {
		return "", err
	}
	token, err := encodeContinuationReceiptWithKey(receipt, i.key)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (i continuationIssuer) preflightKnownFields(
	threadID, predecessorID string,
	predecessorBody, body []byte,
	predecessorUpdatedAt string,
	predecessorEditCount int,
	predecessorLastEditID string,
	openingAuthor string,
) error {
	// Use maximum-sized still-valid reply fields so a successful preflight also
	// proves the eventual signed token cannot exceed the argv safety bound.
	replyID := strings.Repeat("R", maxContinuationReceiptIDBytes)
	if replyID == predecessorID {
		replyID = strings.Repeat("S", maxContinuationReceiptIDBytes)
	}
	replyAuthor := strings.Repeat("r", 256)
	if replyAuthor == openingAuthor {
		replyAuthor = strings.Repeat("s", 256)
	}
	receipt, err := newContinuationReceipt(
		i.providerHost,
		threadID,
		predecessorID,
		replyID,
		predecessorBody,
		body,
		predecessorUpdatedAt,
		"9999-12-31T23:59:59.999999999Z",
		predecessorEditCount,
		2_147_483_647,
		predecessorLastEditID,
		strings.Repeat("E", maxContinuationReceiptIDBytes),
		openingAuthor,
		replyAuthor,
	)
	if err != nil {
		return err
	}
	if _, err := encodeContinuationReceiptWithKey(receipt, i.key); err != nil {
		return err
	}
	return nil
}

func exactBodySHA256(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validExactBodySHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

// validContinuationReceiptID accepts the alphabets used by GitHub's legacy
// base64 and current typed node IDs. Keeping IDs to this closed ASCII set is
// also important because recovery diagnostics render them for a human; an
// opaque receipt must not be able to inject terminal controls or shell syntax.
func validContinuationReceiptID(value string) bool {
	if value == "" || len(value) > maxContinuationReceiptIDBytes {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || strings.ContainsRune("_+-/=", rune(c)) {
			continue
		}
		return false
	}
	return true
}

func validContinuationAuthor(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validContinuationTimestamp(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && !parsed.IsZero()
}

func validContinuationEditRevision(count int, lastEditID string) bool {
	if count < 0 {
		return false
	}
	if count == 0 {
		return lastEditID == ""
	}
	return validContinuationReceiptID(lastEditID)
}

func effectiveContinuationProviderHost() (string, error) {
	raw := os.Getenv("GH_HOST")
	if strings.TrimSpace(raw) == "" {
		return "github.com", nil
	}
	return canonicalContinuationProviderHost(raw)
}

func canonicalContinuationProviderHost(value string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if host == "" || len(host) > 255 || strings.ContainsAny(host, "/\\@?#") {
		return "", errors.New("GH_HOST must name one provider hostname without a scheme, path, or credentials")
	}
	for _, r := range host {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", errors.New("GH_HOST contains unsafe whitespace or control data")
		}
	}
	return host, nil
}

func (r continuationReceipt) validate() error {
	if r.Version != continuationReceiptVersion {
		return fmt.Errorf("unsupported version %d", r.Version)
	}
	providerHost, err := canonicalContinuationProviderHost(r.ProviderHost)
	if err != nil || providerHost != r.ProviderHost {
		return errors.New("provider host is not a canonical safe hostname")
	}
	if err := r.validateIDsAndDigests(); err != nil {
		return err
	}
	if err := r.validateTimestamps(); err != nil {
		return err
	}
	if err := r.validateEditRevisions(); err != nil {
		return err
	}
	return r.validateAuthors()
}

func (r continuationReceipt) validateIDsAndDigests() error {
	ids := []struct {
		name  string
		value string
	}{
		{name: "thread ID", value: r.ThreadID},
		{name: "predecessor ID", value: r.PredecessorID},
		{name: "reply ID", value: r.ReplyID},
	}
	for _, id := range ids {
		if !validContinuationReceiptID(id.value) || !utf8.ValidString(id.value) {
			return fmt.Errorf("%s is invalid or exceeds %d bytes", id.name, maxContinuationReceiptIDBytes)
		}
	}
	if r.PredecessorID == r.ReplyID {
		return errors.New("predecessor ID and reply ID must differ")
	}
	if !validExactBodySHA256(r.PredecessorBodySHA256) {
		return errors.New("predecessor body digest must be a lowercase SHA-256 value")
	}
	if !validExactBodySHA256(r.BodySHA256) {
		return errors.New("reply body digest must be a lowercase SHA-256 value")
	}
	return nil
}

func (r continuationReceipt) validateTimestamps() error {
	for _, timestamp := range []struct {
		name  string
		value string
	}{
		{name: "predecessor updatedAt", value: r.PredecessorUpdatedAt},
		{name: "reply updatedAt", value: r.ReplyUpdatedAt},
	} {
		if !validContinuationTimestamp(timestamp.value) {
			return fmt.Errorf("%s must be a nonzero RFC3339 timestamp", timestamp.name)
		}
	}
	return nil
}

func (r continuationReceipt) validateEditRevisions() error {
	if !validContinuationEditRevision(r.PredecessorEditCount, r.PredecessorLastEditID) ||
		!validContinuationEditRevision(r.ReplyEditCount, r.ReplyLastEditID) {
		return errors.New("comment edit revisions must bind a nonnegative count to the corresponding last edit node ID")
	}
	return nil
}

func (r continuationReceipt) validateAuthors() error {
	if !validContinuationAuthor(r.OpeningAuthor) || !validContinuationAuthor(r.ReplyAuthor) {
		return errors.New("opening and reply authors must be nonempty safe identities")
	}
	if r.OpeningAuthor == r.ReplyAuthor {
		return errors.New("opening and reply authors must be distinct")
	}
	return nil
}

func encodeContinuationReceiptWithKey(receipt continuationReceipt, key []byte) (string, error) {
	if err := receipt.validate(); err != nil {
		return "", err
	}
	if len(key) != continuationReceiptKeyBytes {
		return "", fmt.Errorf("continuation receipt signing key must contain exactly %d bytes", continuationReceiptKeyBytes)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("encode continuation receipt: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(continuationReceiptMACDomain))
	_, _ = mac.Write(raw)
	token := base64.RawURLEncoding.EncodeToString(raw) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if len(token) > maxContinuationReceiptTokenBytes {
		return "", fmt.Errorf("continuation receipt exceeds %d bytes", maxContinuationReceiptTokenBytes)
	}
	return token, nil
}

func decodeContinuationReceipt(token string) (continuationReceipt, error) {
	raw, suppliedMAC, err := decodeContinuationReceiptEnvelope(token)
	if err != nil {
		return continuationReceipt{}, err
	}
	if err := authenticateContinuationReceipt(raw, suppliedMAC); err != nil {
		return continuationReceipt{}, err
	}
	return decodeCanonicalContinuationReceipt(raw)
}

func decodeContinuationReceiptEnvelope(token string) ([]byte, []byte, error) {
	if token == "" || len(token) > maxContinuationReceiptTokenBytes {
		return nil, nil, fmt.Errorf(
			"receipt must contain between 1 and %d bytes",
			maxContinuationReceiptTokenBytes,
		)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, nil, errors.New("receipt must contain one authenticated payload and signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, errors.New("receipt payload is not valid base64url data")
	}
	if base64.RawURLEncoding.EncodeToString(raw) != parts[0] {
		return nil, nil, errors.New("receipt payload is not canonical base64url data")
	}
	suppliedMAC, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, errors.New("receipt signature is not canonical HMAC-SHA-256 data")
	}
	if len(suppliedMAC) != sha256.Size {
		return nil, nil, errors.New("receipt signature is not canonical HMAC-SHA-256 data")
	}
	if base64.RawURLEncoding.EncodeToString(suppliedMAC) != parts[1] {
		return nil, nil, errors.New("receipt signature is not canonical HMAC-SHA-256 data")
	}
	return raw, suppliedMAC, nil
}

func authenticateContinuationReceipt(raw, suppliedMAC []byte) error {
	key, err := loadContinuationReceiptKey()
	if err != nil {
		return &continuationIssuerStateError{cause: fmt.Errorf(
			"load local continuation receipt signing key: %w",
			err,
		)}
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(continuationReceiptMACDomain))
	_, _ = mac.Write(raw)
	if !hmac.Equal(suppliedMAC, mac.Sum(nil)) {
		return &continuationIssuerStateError{cause: errors.New(
			"receipt signature does not authenticate under the selected local continuation key",
		)}
	}
	return nil
}

func decodeCanonicalContinuationReceipt(raw []byte) (continuationReceipt, error) {
	fields, err := decodeUniqueContinuationReceiptObject(raw)
	if err != nil {
		return continuationReceipt{}, err
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return continuationReceipt{}, fmt.Errorf("normalize receipt fields: %w", err)
	}
	var receipt continuationReceipt
	if err := json.Unmarshal(normalized, &receipt); err != nil {
		return continuationReceipt{}, fmt.Errorf("decode receipt fields: %w", err)
	}
	if err := receipt.validate(); err != nil {
		return continuationReceipt{}, err
	}
	canonical, err := json.Marshal(receipt)
	if err != nil {
		return continuationReceipt{}, fmt.Errorf("canonicalize continuation receipt: %w", err)
	}
	if !bytes.Equal(canonical, raw) {
		return continuationReceipt{}, errors.New("receipt JSON is not in canonical encoded form")
	}
	return receipt, nil
}

var continuationReceiptFields = map[string]struct{}{
	"version":                  {},
	"provider_host":            {},
	"thread_id":                {},
	"predecessor_id":           {},
	"reply_id":                 {},
	"predecessor_body_sha256":  {},
	"body_sha256":              {},
	"predecessor_updated_at":   {},
	"reply_updated_at":         {},
	"predecessor_edit_count":   {},
	"reply_edit_count":         {},
	"predecessor_last_edit_id": {},
	"reply_last_edit_id":       {},
	"opening_author":           {},
	"reply_author":             {},
}

func decodeUniqueContinuationReceiptObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode receipt: %w", err)
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("receipt must be a JSON object")
	}
	fields := make(map[string]json.RawMessage, len(continuationReceiptFields))
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode receipt field: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("receipt contains a non-string field name")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("receipt contains duplicate field %q", key)
		}
		if _, known := continuationReceiptFields[key]; !known {
			return nil, fmt.Errorf("receipt contains unknown field %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode receipt field %q: %w", key, err)
		}
		fields[key] = value
	}
	closing, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode receipt: %w", err)
	}
	if delim, ok := closing.(json.Delim); !ok || delim != '}' {
		return nil, errors.New("receipt JSON object was not closed")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("receipt contains trailing data")
	}
	return fields, nil
}

func parseContinueResolveArgs(args []string) (receiptToken, bodyFile string, err error) {
	const commandUsage = "usage: continue-resolve <receipt> --body-file <path|->"
	if len(args) != 3 || args[1] != "--body-file" || args[0] == "" || args[2] == "" {
		return "", "", errors.New(commandUsage)
	}
	return args[0], args[2], nil
}

// cmdContinueResolve completes a reply-resolve operation whose exact reply
// was already observed. It has no path to postReply: every input and provider
// invariant is checked before the only mutation, resolveWithEvidence.
func cmdContinueResolve(ctx context.Context, rest []string) int {
	receiptToken, bodyFile, err := parseContinueResolveArgs(rest)
	if err != nil {
		return fail("%v", err)
	}
	receipt, err := decodeContinuationReceipt(receiptToken)
	if err != nil {
		if issuerStateErr, ok := errors.AsType[*continuationIssuerStateError](err); ok {
			return fail("continuation receipt authentication state could not be restored: %v; no body source was read and no provider request was made\n%s",
				issuerStateErr, continuationIssuerStateRecoveryGuidance(receiptToken, bodyFile))
		}
		return fail("invalid continuation receipt: %v; no provider request was made\n%s",
			err, invalidContinuationReceiptGuidance(bodyFile))
	}
	providerHost, err := effectiveContinuationProviderHost()
	if err != nil {
		return fail("invalid continuation provider host: %v; no body source was read and no provider request was made\n%s",
			err, continuationIssuerStateRecoveryGuidance(receiptToken, bodyFile))
	}
	if providerHost != receipt.ProviderHost {
		return fail("continuation receipt was issued for provider host %s, but the current provider host is %s; no body source was read and no provider request was made\n%s",
			receipt.ProviderHost, providerHost, continuationIssuerStateRecoveryGuidance(receiptToken, bodyFile))
	}
	body, err := loadReplyBody(bodyFile, os.Stdin)
	if err != nil {
		return fail("%v", err)
	}
	if exactBodySHA256([]byte(body)) != receipt.BodySHA256 {
		return fail("the reply-body source does not match the continuation receipt; no provider request was made\n%s",
			inspectContinuationMismatchGuidance(receipt.ThreadID, receiptToken, bodyFile))
	}
	guidance := continuationRecoveryGuidance(receipt.ThreadID, receiptToken, bodyFile)

	history, err := fetchAllComments(ctx, receipt.ThreadID)
	if err != nil {
		return fail("provider state is unverified: continuation history could not be read, so no mutation was attempted: %v\n%s",
			err, providerReadRecoveryGuidance(err, inspectContinuationGuidance(receipt.ThreadID, receiptToken, bodyFile)))
	}
	if err := validateContinuationHistory(receipt, history); err != nil {
		return reconcileContinuationHistoryMismatch(ctx, receipt, err, guidance)
	}
	msg, _, resolveErr := resolveWithEvidence(ctx, receipt.ThreadID, false, resolutionEvidence{
		LastID:                       receipt.ReplyID,
		PredecessorID:                receipt.PredecessorID,
		PredecessorBodySHA256:        receipt.PredecessorBodySHA256,
		BodySHA256:                   receipt.BodySHA256,
		PredecessorUpdatedAt:         receipt.PredecessorUpdatedAt,
		ReplyUpdatedAt:               receipt.ReplyUpdatedAt,
		PredecessorEditCount:         receipt.PredecessorEditCount,
		ReplyEditCount:               receipt.ReplyEditCount,
		PredecessorEditCountPresent:  true,
		ReplyEditCountPresent:        true,
		PredecessorLastEditID:        receipt.PredecessorLastEditID,
		ReplyLastEditID:              receipt.ReplyLastEditID,
		PredecessorLastEditIDPresent: true,
		ReplyLastEditIDPresent:       true,
		OpeningAuthor:                receipt.OpeningAuthor,
		ReplyAuthor:                  receipt.ReplyAuthor,
	})
	if resolveErr != nil {
		if message, handled := replyResolutionEvidenceFailure(
			receipt.ThreadID,
			resolveErr,
			guidance,
		); handled {
			return fail("%s", message)
		}
		if isAccessDenied(resolveErr) {
			return fail("the exact reply is still current, but GitHub refused the resolution: %v\n"+
				"this is an access problem: repair `gh` credentials before retrying the same continuation:\n%s",
				resolveErr, guidance.unchanged)
		}
		return fail("the exact reply is still current, but the thread is NOT resolved: %v\n"+
			"retry only through this resolve-only continuation:\n%s",
			resolveErr, guidance.unchanged)
	}
	fmt.Println(msg)
	return 0
}

// continuationHistoryEvidenceError preserves whether a complete full-history
// read contradicted the receipt or merely omitted evidence needed to decide.
// The error interface stays small for the command path while reconciliation
// can still carry a conclusive contradiction across its later exact-state
// read.
type continuationHistoryEvidenceError struct {
	cause        error
	contradicted bool
}

func (e *continuationHistoryEvidenceError) Error() string { return e.cause.Error() }

func (e *continuationHistoryEvidenceError) Unwrap() error { return e.cause }

func continuationHistoryContradiction(message string) error {
	return &continuationHistoryEvidenceError{
		cause:        errors.New(message),
		contradicted: true,
	}
}

func continuationHistoryUnverified(message string) error {
	return &continuationHistoryEvidenceError{cause: errors.New(message)}
}

func isConclusiveContinuationHistoryContradiction(err error) bool {
	var evidenceErr *continuationHistoryEvidenceError
	return errors.As(err, &evidenceErr) && evidenceErr.contradicted
}

type continuationHistoryMatch struct {
	opening          tailComment
	predecessor      tailComment
	reply            tailComment
	predecessorIndex int
	replyIndex       int
	historyLength    int
}

func validateContinuationHistory(receipt continuationReceipt, history []tailComment) error {
	if err := receipt.validate(); err != nil {
		return &continuationHistoryEvidenceError{
			cause: fmt.Errorf("invalid continuation receipt: %w", err),
		}
	}
	matched, err := locateContinuationHistory(receipt, history)
	if err != nil {
		return err
	}
	if err := validateContinuationHistoryPlacement(matched); err != nil {
		return err
	}
	if err := validateContinuationHistoryBodies(receipt, matched); err != nil {
		return err
	}
	if err := validateContinuationHistoryRevisions(receipt, matched); err != nil {
		return err
	}
	return validateContinuationHistoryAuthors(receipt, matched)
}

func locateContinuationHistory(
	receipt continuationReceipt,
	history []tailComment,
) (continuationHistoryMatch, error) {
	matched := continuationHistoryMatch{
		predecessorIndex: -1,
		replyIndex:       -1,
		historyLength:    len(history),
	}
	for i, comment := range history {
		if i == 0 {
			matched.opening = comment
		}
		if comment.ID == "" {
			return continuationHistoryMatch{}, continuationHistoryUnverified("provider history omitted a comment ID")
		}
		switch comment.ID {
		case receipt.PredecessorID:
			if matched.predecessorIndex >= 0 {
				return continuationHistoryMatch{}, continuationHistoryContradiction("provider history duplicated the named predecessor")
			}
			matched.predecessor = comment
			matched.predecessorIndex = i
		case receipt.ReplyID:
			if matched.replyIndex >= 0 {
				return continuationHistoryMatch{}, continuationHistoryContradiction("provider history duplicated the named reply")
			}
			matched.reply = comment
			matched.replyIndex = i
		}
	}
	if matched.predecessorIndex < 0 || matched.replyIndex < 0 {
		return continuationHistoryMatch{}, continuationHistoryUnverified("provider history does not contain both named comments")
	}
	return matched, nil
}

func validateContinuationHistoryPlacement(matched continuationHistoryMatch) error {
	if matched.replyIndex != matched.predecessorIndex+1 {
		return continuationHistoryContradiction("the named reply no longer directly follows its original predecessor")
	}
	if matched.replyIndex != matched.historyLength-1 {
		return continuationHistoryContradiction("the named reply is no longer the current tail")
	}
	return nil
}

func validateContinuationHistoryBodies(receipt continuationReceipt, matched continuationHistoryMatch) error {
	if exactBodySHA256([]byte(matched.predecessor.Body)) != receipt.PredecessorBodySHA256 {
		return continuationHistoryContradiction("the provider-visible predecessor does not match the receipt digest")
	}
	if exactBodySHA256([]byte(matched.reply.Body)) != receipt.BodySHA256 {
		return continuationHistoryContradiction("the provider-visible reply does not match the receipt digest")
	}
	return nil
}

func validateContinuationHistoryRevisions(receipt continuationReceipt, matched continuationHistoryMatch) error {
	if matched.predecessor.UpdatedAt == "" || matched.reply.UpdatedAt == "" {
		return continuationHistoryUnverified("provider history omitted a named comment update timestamp")
	}
	if matched.predecessor.UpdatedAt != receipt.PredecessorUpdatedAt ||
		matched.reply.UpdatedAt != receipt.ReplyUpdatedAt {
		return continuationHistoryContradiction("a named comment update timestamp changed after receipt issuance")
	}
	if !matched.predecessor.EditCountPresent || !matched.reply.EditCountPresent ||
		!matched.predecessor.LastEditIDPresent || !matched.reply.LastEditIDPresent {
		return continuationHistoryUnverified("provider history omitted a named comment edit revision")
	}
	if matched.predecessor.EditCount != receipt.PredecessorEditCount ||
		matched.reply.EditCount != receipt.ReplyEditCount ||
		matched.predecessor.LastEditID != receipt.PredecessorLastEditID ||
		matched.reply.LastEditID != receipt.ReplyLastEditID {
		return continuationHistoryContradiction("a named comment edit revision changed after receipt issuance")
	}
	return nil
}

func validateContinuationHistoryAuthors(receipt continuationReceipt, matched continuationHistoryMatch) error {
	if matched.opening.Login == "" || matched.reply.Login == "" {
		return continuationHistoryUnverified("provider history omitted the opening or reply author needed by the receipt")
	}
	if matched.opening.Login != receipt.OpeningAuthor || matched.reply.Login != receipt.ReplyAuthor ||
		matched.opening.Login == matched.reply.Login {
		return continuationHistoryContradiction("provider history author evidence no longer matches the issued independent answer")
	}
	return nil
}
