// Command infra-plan-policy is the private-to-public adapter for deterministic
// OpenTofu plan authorization and post-apply receipts. It never prints private
// plan, inventory, backend, state, or provider evidence.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/infraattest"
)

const maxContextBytes = 256 << 10

type evidencePaths struct {
	DesiredRulesetProjection string `json:"desired_ruleset_projection"`
	Inventory                string `json:"inventory"`
	Backend                  string `json:"backend"`
	StateSnapshot            string `json:"state_snapshot"`
	MigrationManifest        string `json:"migration_manifest"`
	ProviderSnapshot         string `json:"provider_snapshot"`
}

type authorizationContext struct {
	Platform          string                   `json:"platform"`
	Source            infraattest.SourceClaims `json:"source"`
	State             stateContext             `json:"state"`
	Ruleset           rulesetContext           `json:"ruleset"`
	PlanProfile       infraattest.PlanProfile  `json:"plan_profile"`
	InventoryComplete bool                     `json:"inventory_complete"`
	IssuedAt          string                   `json:"issued_at"`
	NotBefore         string                   `json:"not_before"`
	ExpiresAt         string                   `json:"expires_at"`
	Nonce             string                   `json:"nonce"`
	Evidence          evidencePaths            `json:"evidence"`
}

type stateContext struct {
	Lineage string `json:"lineage"`
	Serial  uint64 `json:"serial"`
}

type rulesetContext struct {
	Address     string `json:"address"`
	ImmutableID string `json:"immutable_id"`
	Repository  string `json:"repository"`
}

type expectedAuthorizationContext struct {
	SubjectSHA256 string                      `json:"subject_sha256"`
	Source        infraattest.SourceClaims    `json:"source"`
	Toolchain     infraattest.ToolchainClaims `json:"toolchain"`
	State         infraattest.StateClaims     `json:"state"`
	Nonce         string                      `json:"nonce"`
}

type receiptContext struct {
	State        stateContext                   `json:"state"`
	Verification infraattest.VerificationClaims `json:"verification"`
	Nonce        string                         `json:"nonce"`
	Evidence     evidencePaths                  `json:"evidence"`
}

type expectedReceiptContext struct {
	AuthorizationClaimsSHA256 string                      `json:"authorization_claims_sha256"`
	AppliedPlanSHA256         string                      `json:"applied_plan_sha256"`
	Source                    infraattest.SourceClaims    `json:"source"`
	Toolchain                 infraattest.ToolchainClaims `json:"toolchain"`
	State                     infraattest.StateClaims     `json:"state"`
	Nonce                     string                      `json:"nonce"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now()))
}

func run(args []string, stdout, stderr io.Writer, now time.Time) int {
	now = canonicalCLITime(now)
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "authorize":
		err = runAuthorize(args[1:], stdout, stderr, now)
	case "verify-authorization":
		err = runVerifyAuthorization(args[1:], stdout, stderr, now)
	case "receipt":
		err = runReceipt(args[1:], stdout, stderr, now)
	case "verify-receipt":
		err = runVerifyReceipt(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		usage(stderr)
		return 2
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "infra-plan-policy: infra plan policy rejected: %s\n", infraattest.RejectionCode(err))
		return 3
	}
	return 0
}

func canonicalCLITime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Second)
}

// parseWithHelp parses fs, printing its usage to stderr only when the
// caller explicitly asked for help (-h/--help). Any other parse error
// (unknown flag, bad value) yields flag.ErrHelp's usage text too via fs's
// own default error handling, but we discard that entirely rather than
// write it anywhere: a caller-supplied flag can carry private evidence in
// its name or value, and echoing flag's own parse-error message would leak
// it — this command never prints private input.
func parseWithHelp(fs *flag.FlagSet, stderr io.Writer, args []string) error {
	var usage bytes.Buffer
	fs.SetOutput(&usage)
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		_, _ = usage.WriteTo(stderr)
	}
	return err
}

func runAuthorize(args []string, stdout, stderr io.Writer, now time.Time) error {
	fs := flag.NewFlagSet("authorize", flag.ContinueOnError)
	contextPath := fs.String("context", "", "private authorization context JSON")
	planPath := fs.String("plan", "", "encrypted saved plan")
	planJSONPath := fs.String("plan-json", "", "non-regular stream of private tofu show -json output")
	tofuPath := fs.String("tofu", "", "pinned OpenTofu executable")
	providerPath := fs.String("provider", "", "pinned provider executable")
	lockPath := fs.String("lockfile", "", "dependency lock file")
	manifestPath := fs.String("toolchain-lock", "", "toolchain lock manifest")
	baselinePath := fs.String("baseline", "", "latest signed receipt claims")
	keyPath := fs.String("hmac-key", "", "base64url HMAC key file")
	if err := parseWithHelp(fs, stderr, args); err != nil {
		return err
	}
	if fs.NArg() != 0 ||
		anyEmpty(*contextPath, *planPath, *planJSONPath, *tofuPath, *providerPath, *lockPath, *manifestPath, *baselinePath, *keyPath) {
		return errors.New("invalid input")
	}
	var context authorizationContext
	if err := loadJSON(*contextPath, &context); err != nil {
		return err
	}
	issuedAt, err := parseUTC(context.IssuedAt)
	if err != nil {
		return err
	}
	notBefore, err := parseUTC(context.NotBefore)
	if err != nil {
		return err
	}
	expiresAt, err := parseUTC(context.ExpiresAt)
	if err != nil {
		return err
	}
	nonce, err := parseNonce(context.Nonce)
	if err != nil {
		return err
	}
	key, err := loadKey(*keyPath)
	if err != nil {
		return err
	}
	defer clear(key)
	readers, err := openAuthorizationReaders(
		*planPath, *planJSONPath, *tofuPath, *providerPath, *lockPath, *manifestPath, *baselinePath, context.Evidence,
	)
	if err != nil {
		return err
	}
	defer readers.close()
	claims, err := infraattest.Authorize(infraattest.AuthorizationRequest{
		EncryptedPlan:            readers.plan,
		PlanJSON:                 readers.planJSON,
		OpenTofuBinary:           readers.tofu,
		ProviderBinary:           readers.provider,
		DependencyLockfile:       readers.lockfile,
		ToolchainManifest:        readers.toolchain,
		DesiredRulesetProjection: readers.desired,
		Inventory:                readers.inventory,
		Backend:                  readers.backend,
		StateSnapshot:            readers.state,
		MigrationManifest:        readers.migration,
		ProviderSnapshot:         readers.providerSnapshot,
		BaselineReceipt:          readers.baseline,
		Platform:                 context.Platform,
		Source:                   context.Source,
		State:                    infraattest.PrivateState{Lineage: context.State.Lineage, Serial: context.State.Serial},
		Ruleset: infraattest.RulesetBinding{
			Address: context.Ruleset.Address, ImmutableID: context.Ruleset.ImmutableID,
			Repository: context.Ruleset.Repository,
		},
		PlanProfile:       context.PlanProfile,
		InventoryComplete: context.InventoryComplete,
		IssuedAt:          issuedAt,
		NotBefore:         notBefore,
		ExpiresAt:         expiresAt,
		Now:               now,
		HMACKey:           key,
		Nonce:             nonce,
	})
	if err != nil {
		return err
	}
	_, err = stdout.Write(claims)
	return err
}

func runVerifyAuthorization(args []string, stdout, stderr io.Writer, now time.Time) error {
	fs := flag.NewFlagSet("verify-authorization", flag.ContinueOnError)
	claimsPath := fs.String("claims", "", "canonical authorization claims")
	expectedPath := fs.String("expected", "", "current expected public bindings")
	if err := parseWithHelp(fs, stderr, args); err != nil {
		return err
	}
	if fs.NArg() != 0 || anyEmpty(*claimsPath, *expectedPath) {
		return errors.New("invalid input")
	}
	claims, err := loadBytes(*claimsPath, infraattest.MaxClaimsBytes)
	if err != nil {
		return err
	}
	var expected expectedAuthorizationContext
	if err := loadJSON(*expectedPath, &expected); err != nil {
		return err
	}
	if _, err := infraattest.VerifyAuthorization(claims, infraattest.ExpectedAuthorization{
		SubjectSHA256: expected.SubjectSHA256,
		Source:        expected.Source,
		Toolchain:     expected.Toolchain,
		State:         expected.State,
		Nonce:         expected.Nonce,
		Now:           now,
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, `{"status":"authorized"}`)
	return err
}

func runReceipt(args []string, stdout, stderr io.Writer, now time.Time) error {
	fs := flag.NewFlagSet("receipt", flag.ContinueOnError)
	contextPath := fs.String("context", "", "private receipt context JSON")
	authorizationPath := fs.String("authorization", "", "canonical authorization claims")
	keyPath := fs.String("hmac-key", "", "base64url HMAC key file")
	if err := parseWithHelp(fs, stderr, args); err != nil {
		return err
	}
	if fs.NArg() != 0 || anyEmpty(*contextPath, *authorizationPath, *keyPath) {
		return errors.New("invalid input")
	}
	var context receiptContext
	if err := loadJSON(*contextPath, &context); err != nil {
		return err
	}
	nonce, err := parseNonce(context.Nonce)
	if err != nil {
		return err
	}
	key, err := loadKey(*keyPath)
	if err != nil {
		return err
	}
	defer clear(key)
	readers, err := openReceiptReaders(*authorizationPath, context.Evidence)
	if err != nil {
		return err
	}
	defer readers.close()
	receipt, err := infraattest.IssueReceipt(infraattest.ReceiptRequest{
		Authorization:     readers.authorization,
		Inventory:         readers.inventory,
		Backend:           readers.backend,
		StateSnapshot:     readers.state,
		MigrationManifest: readers.migration,
		ProviderSnapshot:  readers.providerSnapshot,
		State:             infraattest.PrivateState{Lineage: context.State.Lineage, Serial: context.State.Serial},
		Verification:      context.Verification,
		ObservedAt:        now,
		HMACKey:           key,
		Nonce:             nonce,
	})
	if err != nil {
		return err
	}
	_, err = stdout.Write(receipt)
	return err
}

func runVerifyReceipt(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("verify-receipt", flag.ContinueOnError)
	receiptPath := fs.String("receipt", "", "canonical receipt claims")
	expectedPath := fs.String("expected", "", "current expected public bindings")
	if err := parseWithHelp(fs, stderr, args); err != nil {
		return err
	}
	if fs.NArg() != 0 || anyEmpty(*receiptPath, *expectedPath) {
		return errors.New("invalid input")
	}
	receipt, err := loadBytes(*receiptPath, infraattest.MaxClaimsBytes)
	if err != nil {
		return err
	}
	var expected expectedReceiptContext
	if err := loadJSON(*expectedPath, &expected); err != nil {
		return err
	}
	if _, err := infraattest.VerifyReceipt(receipt, infraattest.ExpectedReceipt{
		AuthorizationClaimsSHA256: expected.AuthorizationClaimsSHA256,
		AppliedPlanSHA256:         expected.AppliedPlanSHA256,
		Source:                    expected.Source,
		Toolchain:                 expected.Toolchain,
		State:                     expected.State,
		Nonce:                     expected.Nonce,
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, `{"status":"reconciled"}`)
	return err
}

type authorizationReaders struct {
	files            []*os.File
	plan             io.Reader
	planJSON         io.Reader
	tofu             io.Reader
	provider         io.Reader
	lockfile         io.Reader
	toolchain        io.Reader
	desired          io.Reader
	inventory        io.Reader
	backend          io.Reader
	state            io.Reader
	migration        io.Reader
	providerSnapshot io.Reader
	baseline         io.Reader
}

func openAuthorizationReaders(
	plan, planJSON, tofu, provider, lockfile, toolchain, baseline string,
	paths evidencePaths,
) (*authorizationReaders, error) {
	allPaths := []string{
		plan, planJSON, tofu, provider, lockfile, toolchain,
		paths.DesiredRulesetProjection, paths.Inventory, paths.Backend,
		paths.StateSnapshot, paths.MigrationManifest, paths.ProviderSnapshot,
		baseline,
	}
	files, err := openFiles(allPaths)
	if err != nil {
		return nil, err
	}
	planJSONInfo, err := files[1].Stat()
	if err != nil || planJSONInfo.Mode().IsRegular() {
		closeFiles(files)
		return nil, errors.New("invalid input")
	}
	return &authorizationReaders{
		files: files, plan: files[0], planJSON: files[1], tofu: files[2], provider: files[3],
		lockfile: files[4], toolchain: files[5], desired: files[6], inventory: files[7],
		backend: files[8], state: files[9], migration: files[10], providerSnapshot: files[11], baseline: files[12],
	}, nil
}

func (readers *authorizationReaders) close() {
	closeFiles(readers.files)
}

type receiptReaders struct {
	files            []*os.File
	authorization    io.Reader
	inventory        io.Reader
	backend          io.Reader
	state            io.Reader
	migration        io.Reader
	providerSnapshot io.Reader
}

func openReceiptReaders(authorization string, paths evidencePaths) (*receiptReaders, error) {
	files, err := openFiles([]string{
		authorization, paths.Inventory, paths.Backend, paths.StateSnapshot,
		paths.MigrationManifest, paths.ProviderSnapshot,
	})
	if err != nil {
		return nil, err
	}
	return &receiptReaders{
		files: files, authorization: files[0], inventory: files[1], backend: files[2],
		state: files[3], migration: files[4], providerSnapshot: files[5],
	}, nil
}

func (readers *receiptReaders) close() {
	closeFiles(readers.files)
}

func openFiles(paths []string) ([]*os.File, error) {
	files := make([]*os.File, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			closeFiles(files)
			return nil, errors.New("invalid input")
		}
		file, err := os.Open(path)
		if err != nil {
			closeFiles(files)
			return nil, errors.New("invalid input")
		}
		files = append(files, file)
	}
	return files, nil
}

// closeFiles releases read-only inputs. Close cannot report a lost write for a
// read-only descriptor, and the caller is already returning a rejection, so the
// discard below is deliberate rather than a swallowed failure.
func closeFiles(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

func loadJSON(path string, target any) error {
	raw, err := loadBytes(path, maxContextBytes)
	if err != nil {
		return err
	}
	// INFRA-ATTEST-06 requires duplicated private JSON to be withheld, but
	// encoding/json silently keeps the last occurrence of a repeated key and
	// DisallowUnknownFields does not catch it. Canonicalise through the same
	// bounded unique-key parser used for private evidence before decoding.
	canonical, err := infraattest.CanonicalPrivateJSON(raw, maxContextBytes)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid input")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid input")
	}
	return nil
}

func loadBytes(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("invalid input")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(raw) > limit {
		return nil, errors.New("invalid input")
	}
	return raw, nil
}

func loadKey(path string) ([]byte, error) {
	raw, err := loadBytes(path, 1024)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) < infraattest.CommitmentKeyMinBytes {
		return nil, errors.New("invalid input")
	}
	return key, nil
}

func parseNonce(value string) ([]byte, error) {
	nonce, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(nonce) != infraattest.CommitmentNonceBytes ||
		base64.RawURLEncoding.EncodeToString(nonce) != value {
		return nil, errors.New("invalid input")
	}
	return nonce, nil
}

func parseUTC(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || parsed.Location() != time.UTC || parsed.Nanosecond() != 0 {
		return time.Time{}, errors.New("invalid input")
	}
	return parsed, nil
}

func anyEmpty(values ...string) bool {
	return slices.Contains(values, "")
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: infra-plan-policy <authorize|verify-authorization|receipt|verify-receipt> [flags]")
}
