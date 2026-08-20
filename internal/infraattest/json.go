package infraattest

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil || limit <= 0 {
		return nil, reject(CodeInvalidInput)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, reject(CodeInvalidInput)
	}
	if int64(len(raw)) > limit {
		return nil, reject(CodeInputTooLarge)
	}
	return raw, nil
}

func hashBounded(reader io.Reader, limit int64) (string, error) {
	if reader == nil || limit <= 0 {
		return "", reject(CodeInvalidInput)
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(reader, limit+1))
	if err != nil {
		return "", reject(CodeInvalidInput)
	}
	if n > limit {
		return "", reject(CodeInputTooLarge)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type jsonLimits struct {
	nodes          int
	maxStringBytes int
	// numberBudget is the aggregate normalised-number bytes still available to
	// this document. Every expansion is charged against it before it is built,
	// so a document cannot spend more on normalised numbers than it was allowed
	// to occupy raw, however compactly its exponents were written.
	numberBudget int
}

// CanonicalPrivateJSON canonicalises bounded private JSON, rejecting duplicated
// keys, trailing bytes, oversized input, and unsupported encodings so callers
// outside this package can satisfy INFRA-ATTEST-06 with the same parser.
func CanonicalPrivateJSON(raw []byte, maxRawBytes int) ([]byte, error) {
	return canonicalJSONWithLimits(raw, maxRawBytes, MaxJSONStringBytes)
}

func canonicalJSON(raw []byte) ([]byte, error) {
	return canonicalJSONWithLimits(raw, MaxEvidenceBytes, MaxJSONStringBytes)
}

func canonicalJSONWithLimits(raw []byte, maxRawBytes, maxStringBytes int) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxRawBytes || maxStringBytes <= 0 || !utf8.Valid(raw) {
		if len(raw) > maxRawBytes {
			return nil, reject(CodeInputTooLarge)
		}
		return nil, reject(CodeMalformedJSON)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	limits := &jsonLimits{maxStringBytes: maxStringBytes, numberBudget: maxRawBytes}
	value, err := decodeUniqueValue(decoder, limits, 0)
	if err != nil {
		return nil, reject(CodeMalformedJSON)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, reject(CodeMalformedJSON)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, reject(CodeMalformedJSON)
	}
	return canonical, nil
}

func decodeUniqueValue(decoder *json.Decoder, limits *jsonLimits, depth int) (any, error) {
	if depth > MaxJSONDepth || limits.nodes >= MaxJSONNodes {
		return nil, errors.New("json limits exceeded")
	}
	limits.nodes++
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			return decodeUniqueObject(decoder, limits, depth)
		case '[':
			return decodeUniqueArray(decoder, limits, depth)
		default:
			return nil, errors.New("unexpected delimiter")
		}
	case string:
		if len(typed) > limits.maxStringBytes {
			return nil, errors.New("string too large")
		}
		return typed, nil
	case json.Number:
		return normalizeJSONNumber(typed, limits)
	case bool, nil:
		return typed, nil
	default:
		return nil, errors.New("unsupported token")
	}
}

// normalizeJSONNumber gives every finite JSON decimal one exact, exponent-free
// representation. This keeps commitments stable across spellings such as 1,
// 1.0, and 10e-1 without rounding through binary floating point.
//
// The expansion is priced before it is bought. A literal such as 1e4000000
// occupies nine input bytes and normalises to four million, so measuring the
// result would mean allocating it first, and the node bound alone would then
// admit input orders of magnitude past every declared byte bound. The width is
// therefore computed from the digits and the exponent, then charged against
// both the per-number bound and the document's remaining aggregate budget, so
// an over-budget number is refused rather than built and discarded.
func normalizeJSONNumber(number json.Number, limits *jsonLimits) (json.Number, error) {
	raw := number.String()
	if len(raw) == 0 || len(raw) > MaxJSONNumberBytes {
		return "", errors.New("number too large")
	}
	negative := strings.HasPrefix(raw, "-")
	unsigned := strings.TrimPrefix(raw, "-")
	mantissa := unsigned
	exponent := int64(0)
	if separator := strings.IndexAny(unsigned, "eE"); separator >= 0 {
		mantissa = unsigned[:separator]
		parsed, err := strconv.ParseInt(unsigned[separator+1:], 10, 64)
		if err != nil {
			return "", errors.New("invalid number exponent")
		}
		exponent = parsed
	}
	integer, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(integer+fraction, "0")
	if digits == "" {
		if err := chargeNumberBudget(limits, len("0")); err != nil {
			return "", err
		}
		return json.Number("0"), nil
	}
	trimmedDigits := strings.TrimRight(digits, "0")
	exponent -= int64(len(fraction))
	exponent += int64(len(digits) - len(trimmedDigits))
	digits = trimmedDigits
	// Bounding the exponent first keeps the width arithmetic below inside int
	// and refuses the compact-literal expansion at its source.
	if exponent < -int64(MaxJSONNumberBytes)-int64(len(digits)) || exponent > int64(MaxJSONNumberBytes) {
		return "", errors.New("normalized number too large")
	}

	decimalPosition := len(digits) + int(exponent)
	width := plainDecimalWidth(len(digits), decimalPosition, negative)
	if width > MaxJSONNumberBytes {
		return "", errors.New("normalized number too large")
	}
	if err := chargeNumberBudget(limits, width); err != nil {
		return "", err
	}
	normalized := plainDecimal(digits, decimalPosition, negative)
	if len(normalized) != width {
		return "", errors.New("normalized number width mismatch")
	}
	return json.Number(normalized), nil
}

// plainDecimalWidth reports the byte width plainDecimal will produce for the
// same arguments, so the cost of an expansion is known before it is paid.
func plainDecimalWidth(digits, decimalPosition int, negative bool) int {
	var width int
	switch {
	case decimalPosition <= 0:
		width = len("0.") - decimalPosition + digits
	case decimalPosition >= digits:
		width = decimalPosition
	default:
		width = digits + len(".")
	}
	if negative {
		width += len("-")
	}
	return width
}

// plainDecimal writes digits as an exponent-free decimal with the point at
// decimalPosition, padding with zeros on whichever side the position falls
// outside the digits.
func plainDecimal(digits string, decimalPosition int, negative bool) string {
	var normalized string
	switch {
	case decimalPosition <= 0:
		normalized = "0." + strings.Repeat("0", -decimalPosition) + digits
	case decimalPosition >= len(digits):
		normalized = digits + strings.Repeat("0", decimalPosition-len(digits))
	default:
		normalized = digits[:decimalPosition] + "." + digits[decimalPosition:]
	}
	if negative {
		normalized = "-" + normalized
	}
	return normalized
}

// chargeNumberBudget draws width bytes from the document's aggregate
// normalised-number allowance, refusing the whole document once the allowance
// is gone. Per-number bounds alone cannot do this: MaxJSONNodes numbers each
// just inside the per-number bound still sum far past any input bound.
func chargeNumberBudget(limits *jsonLimits, width int) error {
	if width > limits.numberBudget {
		return errors.New("normalized number budget exhausted")
	}
	limits.numberBudget -= width
	return nil
}

func decodeUniqueObject(decoder *json.Decoder, limits *jsonLimits, depth int) (map[string]any, error) {
	object := make(map[string]any)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok || len(key) > limits.maxStringBytes {
			return nil, errors.New("invalid object key")
		}
		if _, duplicate := object[key]; duplicate {
			return nil, errors.New("duplicate object key")
		}
		value, err := decodeUniqueValue(decoder, limits, depth+1)
		if err != nil {
			return nil, err
		}
		object[key] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return nil, errors.New("unterminated object")
	}
	return object, nil
}

func decodeUniqueArray(decoder *json.Decoder, limits *jsonLimits, depth int) ([]any, error) {
	array := make([]any, 0)
	for decoder.More() {
		value, err := decodeUniqueValue(decoder, limits, depth+1)
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') {
		return nil, errors.New("unterminated array")
	}
	return array, nil
}

func decodeStrict(raw []byte, target any) ([]byte, error) {
	return decodeStrictWithLimits(raw, target, MaxEvidenceBytes, MaxJSONStringBytes)
}

func decodeStrictWithLimits(raw []byte, target any, maxRawBytes, maxStringBytes int) ([]byte, error) {
	canonical, err := canonicalJSONWithLimits(raw, maxRawBytes, maxStringBytes)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, reject(CodeMalformedJSON)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, reject(CodeMalformedJSON)
	}
	return canonical, nil
}

func canonicalOpaque(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxEvidenceBytes || bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		if len(raw) > MaxEvidenceBytes {
			return nil, reject(CodeInputTooLarge)
		}
		return nil, reject(CodeInvalidInput)
	}
	// bytes.ReplaceAll always returns a fresh slice, so appending below never
	// mutates the caller's buffer.
	normalized := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	normalized = bytes.TrimRight(normalized, " \t\n")
	return append(normalized, '\n'), nil
}

func commitment(key, nonce []byte, domain string, payload []byte) (string, error) {
	if len(key) < CommitmentKeyMinBytes || len(nonce) != CommitmentNonceBytes || domain == "" {
		return "", reject(CodeInvalidInput)
	}
	mac := hmac.New(sha256.New, key)
	writeCommitmentPart(mac, []byte("dear-agent/infraattest/v1"))
	writeCommitmentPart(mac, []byte(domain))
	writeCommitmentPart(mac, nonce)
	writeCommitmentPart(mac, payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func writeCommitmentPart(mac hash.Hash, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	if _, err := mac.Write(size[:]); err != nil {
		panic(err)
	}
	if _, err := mac.Write(value); err != nil {
		panic(err)
	}
}

func encodeNonce(nonce []byte) (string, error) {
	if len(nonce) != CommitmentNonceBytes {
		return "", reject(CodeInvalidInput)
	}
	return base64.RawURLEncoding.EncodeToString(nonce), nil
}

func decodeNonce(encoded string) ([]byte, error) {
	nonce, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(nonce) != CommitmentNonceBytes || base64.RawURLEncoding.EncodeToString(nonce) != encoded {
		return nil, reject(CodeInvalidInput)
	}
	return nonce, nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validGitOID(value string) bool {
	if (len(value) != 40 && len(value) != 64) || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
