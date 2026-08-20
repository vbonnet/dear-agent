package infraattest

import (
	"encoding/base64"
	"io"
	"regexp"
	"strings"
)

const minimumCiphertextBytes = 28

var encryptionMetadataKeyPattern = regexp.MustCompile(`^key_provider\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

type encryptedPlanEnvelope struct {
	EncryptionVersion string            `json:"encryption_version"`
	Meta              map[string]string `json:"meta"`
	EncryptedData     string            `json:"encrypted_data"`
}

func readEncryptedPlan(reader io.Reader) ([]byte, error) {
	raw, err := readBounded(reader, MaxEncryptedPlanBytes)
	if err != nil {
		return nil, err
	}
	var envelope encryptedPlanEnvelope
	if _, err := decodeStrictWithLimits(raw, &envelope, MaxEncryptedPlanBytes, MaxEncryptedPlanBytes); err != nil {
		return nil, reject(CodeInvalidInput)
	}
	if envelope.EncryptionVersion != "v0" || len(envelope.Meta) == 0 || len(envelope.Meta) > 16 ||
		!validBase64Payload(envelope.EncryptedData, minimumCiphertextBytes) {
		return nil, reject(CodeInvalidInput)
	}
	for key, value := range envelope.Meta {
		if !encryptionMetadataKeyPattern.MatchString(key) || !validBase64Payload(value, 1) {
			return nil, reject(CodeInvalidInput)
		}
	}
	return raw, nil
}

func validBase64Payload(value string, minimumDecodedBytes int64) bool {
	if value == "" || int64(base64.StdEncoding.DecodedLen(len(value))) < minimumDecodedBytes {
		return false
	}
	decoded, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(value)))
	return err == nil && decoded >= minimumDecodedBytes
}
