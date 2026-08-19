// Package infraattest converts bounded private infrastructure evidence into
// deterministic public-safe plan authorizations and reconciliation receipts.
package infraattest

import (
	"io"
	"time"
)

const (
	// AuthorizationSchema and the following constants form the audited public schema and lock catalog.
	AuthorizationSchema = "dear-agent.opentofu-plan-authorization/v1"
	// ReceiptSchema identifies canonical apply receipts.
	ReceiptSchema = "dear-agent.opentofu-apply-receipt/v1"
	// ToolchainSchema identifies the exact toolchain lock manifest.
	ToolchainSchema = "dear-agent.opentofu-toolchain-lock/v1"
	// MigrationSchema identifies the private migration-surface projection.
	MigrationSchema = "dear-agent.opentofu-migration-surface/v1"
	// PlanFormatVersion is the only supported tofu show JSON schema.
	PlanFormatVersion = "1.2"

	// OpenTofuVersion is the only supported OpenTofu release.
	OpenTofuVersion = "1.12.5"
	// OpenTofuTagCommit is the peeled source commit for the supported release tag.
	OpenTofuTagCommit = "230349e959a44fb8eb7b83754f9d9b012f3bdb42"
	// ProviderAddress is the only routine provider source.
	ProviderAddress = "registry.opentofu.org/integrations/github"
	// ProviderVersion is the only supported provider release.
	ProviderVersion = "6.13.0"
	// ProviderTagCommit is the source commit for the supported provider tag.
	ProviderTagCommit = "20728bb30c112287ac3e8243cdb0307a45c6a0a0"
	// DependencyLockSHA256 binds the exact checked-in dependency lock bytes.
	DependencyLockSHA256 = "9beeedb2853295abb3e657a304d936e8f819cc7cf04f3d43a839868e8dbc2e4b"
	// ToolchainManifestSHA256 binds the exact checked-in toolchain manifest bytes.
	ToolchainManifestSHA256 = "ef7eacb829eb74978c7a6176d8a733b30e36f158e1c298d5fd812b7b01bd2e36"

	// MaxPlanJSONBytes bounds decrypted private plan JSON.
	MaxPlanJSONBytes = 16 << 20
	// MaxEncryptedPlanBytes bounds the encrypted plan subject.
	MaxEncryptedPlanBytes = 128 << 20
	// MaxToolBinaryBytes bounds the OpenTofu executable.
	MaxToolBinaryBytes = 256 << 20
	// MaxProviderBinaryBytes bounds the provider executable.
	MaxProviderBinaryBytes = 512 << 20
	// MaxEvidenceBytes bounds each private evidence input.
	MaxEvidenceBytes = 16 << 20
	// MaxClaimsBytes bounds public authorization and receipt claims.
	MaxClaimsBytes = 128 << 10
	// MaxToolchainLockBytes bounds toolchain and dependency lock inputs.
	MaxToolchainLockBytes = 256 << 10
	// MaxCommitmentTTL bounds authorization lifetime.
	MaxCommitmentTTL = 15 * time.Minute
	// CommitmentKeyMinBytes is the minimum HMAC key length.
	CommitmentKeyMinBytes = 32
	// CommitmentNonceBytes is the required commitment nonce length.
	CommitmentNonceBytes = 32
	// MaxJSONDepth bounds private JSON nesting.
	MaxJSONDepth = 64
	// MaxJSONNodes bounds private JSON structural nodes.
	MaxJSONNodes = 500_000
	// MaxJSONStringBytes bounds each private JSON string or key.
	MaxJSONStringBytes = 4 << 20
	// MaxJSONNumberBytes bounds each private JSON number both as written and
	// after normalisation to a plain decimal. An exponent-form literal a few
	// bytes wide expands to an arbitrarily long plain decimal, so the bound has
	// to be charged against the expansion rather than the input. It sits well
	// above the widest exact decimal any IEEE-754 double can require.
	MaxJSONNumberBytes = 4 << 10
)

// SourceClaims binds an authorization to the canonical source tree.
type SourceClaims struct {
	Repository              string `json:"repository"`
	Ref                     string `json:"ref"`
	CommitSHA               string `json:"commit_sha"`
	TreeSHA                 string `json:"tree_sha"`
	CanonicalRulesetBlobSHA string `json:"canonical_ruleset_blob_sha"`
}

// ProviderClaims identifies one authenticated provider distribution.
type ProviderClaims struct {
	Address       string `json:"address"`
	Version       string `json:"version"`
	TagCommit     string `json:"tag_commit"`
	ArchiveSHA256 string `json:"archive_sha256"`
	BinarySHA256  string `json:"binary_sha256"`
}

// ToolchainClaims identifies the exact evaluated toolchain and lock inputs.
type ToolchainClaims struct {
	Platform                 string           `json:"platform"`
	OpenTofuVersion          string           `json:"opentofu_version"`
	OpenTofuTagCommit        string           `json:"opentofu_tag_commit"`
	OpenTofuArchiveSHA256    string           `json:"opentofu_archive_sha256"`
	OpenTofuBinarySHA256     string           `json:"opentofu_binary_sha256"`
	ToolchainManifestSHA256  string           `json:"toolchain_manifest_sha256"`
	DependencyLockfileSHA256 string           `json:"dependency_lockfile_sha256"`
	Providers                []ProviderClaims `json:"providers"`
}

// StateClaims exposes only a keyed lineage commitment and state serial.
type StateClaims struct {
	LineageHMACSHA256 string `json:"lineage_hmac_sha256"`
	Serial            uint64 `json:"serial"`
}

// PlanProfile records plan-mode choices that affect classification.
type PlanProfile struct {
	Mode     string   `json:"mode"`
	Refresh  bool     `json:"refresh"`
	Targets  []string `json:"targets"`
	Excludes []string `json:"excludes"`
	Replaces []string `json:"replaces"`
}

// FreshnessClaims defines the bounded authorization validity window.
type FreshnessClaims struct {
	PlanGeneratedAt string `json:"plan_generated_at"`
	IssuedAt        string `json:"issued_at"`
	NotBefore       string `json:"not_before"`
	ExpiresAt       string `json:"expires_at"`
}

// ClassificationClaims records the routine plan class and human gate status.
type ClassificationClaims struct {
	Kind                       string `json:"kind"`
	HumanAuthorizationRequired bool   `json:"human_authorization_required"`
}

// AuthorizationCommitments contains domain-separated private evidence commitments.
type AuthorizationCommitments struct {
	Nonce                      string `json:"nonce"`
	InventoryHMACSHA256        string `json:"inventory_hmac_sha256"`
	BackendHMACSHA256          string `json:"backend_hmac_sha256"`
	StateSnapshotHMACSHA256    string `json:"state_snapshot_hmac_sha256"`
	MigrationSurfaceHMACSHA256 string `json:"migration_surface_hmac_sha256"`
	ProviderSnapshotHMACSHA256 string `json:"provider_snapshot_hmac_sha256"`
	ChangeProjectionHMACSHA256 string `json:"change_projection_hmac_sha256"`
}

// AuthorizationClaims is the canonical public-safe routine authorization.
type AuthorizationClaims struct {
	Schema          string                   `json:"schema"`
	Decision        string                   `json:"decision"`
	Operation       string                   `json:"operation"`
	SubjectSHA256   string                   `json:"subject_sha256"`
	Source          SourceClaims             `json:"source"`
	Toolchain       ToolchainClaims          `json:"toolchain"`
	PrivateEvidence AuthorizationCommitments `json:"private_evidence"`
	State           StateClaims              `json:"state"`
	Classification  ClassificationClaims     `json:"classification"`
	PlanProfile     PlanProfile              `json:"plan_profile"`
	Freshness       FreshnessClaims          `json:"freshness"`
}

// ReceiptCommitments contains post-application private evidence commitments.
type ReceiptCommitments struct {
	Nonce                      string `json:"nonce"`
	InventoryHMACSHA256        string `json:"inventory_hmac_sha256"`
	BackendHMACSHA256          string `json:"backend_hmac_sha256"`
	StateSnapshotHMACSHA256    string `json:"state_snapshot_hmac_sha256"`
	MigrationSurfaceHMACSHA256 string `json:"migration_surface_hmac_sha256"`
	ProviderSnapshotHMACSHA256 string `json:"provider_snapshot_hmac_sha256"`
}

// VerificationClaims records required post-application observations.
type VerificationClaims struct {
	ProviderVisible  bool `json:"provider_visible"`
	NoDrift          bool `json:"no_drift"`
	SourceParity     bool `json:"source_parity"`
	BehavioralCanary bool `json:"behavioral_canary"`
}

// ReceiptClaims is the canonical public-safe reconciliation receipt.
type ReceiptClaims struct {
	Schema                    string             `json:"schema"`
	Decision                  string             `json:"decision"`
	Operation                 string             `json:"operation"`
	AuthorizationClaimsSHA256 string             `json:"authorization_claims_sha256"`
	AppliedPlanSHA256         string             `json:"applied_plan_sha256"`
	Source                    SourceClaims       `json:"source"`
	Toolchain                 ToolchainClaims    `json:"toolchain"`
	PrivateEvidence           ReceiptCommitments `json:"private_evidence"`
	State                     StateClaims        `json:"state"`
	Verification              VerificationClaims `json:"verification"`
	ObservedAt                string             `json:"observed_at"`
}

// PrivateState carries private state identity into commitment evaluation.
type PrivateState struct {
	Lineage string
	Serial  uint64
}

// RulesetBinding identifies the only existing ruleset eligible for restoration.
type RulesetBinding struct {
	Address     string
	ImmutableID string
	Repository  string
}

// AuthorizationRequest is the single private evaluation seam. Readers may
// contain secrets; the module consumes them in memory and returns only public-
// safe claims. None of their content is included in errors. A trusted adapter
// must stream PlanJSON directly from the exact EncryptedPlan through the pinned
// OpenTofu binary; transport construction is the only layer that can establish
// that provenance relationship between the two readers.
type AuthorizationRequest struct {
	EncryptedPlan            io.Reader
	PlanJSON                 io.Reader
	OpenTofuBinary           io.Reader
	ProviderBinary           io.Reader
	DependencyLockfile       io.Reader
	ToolchainManifest        io.Reader
	DesiredRulesetProjection io.Reader
	Inventory                io.Reader
	Backend                  io.Reader
	StateSnapshot            io.Reader
	MigrationManifest        io.Reader
	ProviderSnapshot         io.Reader
	BaselineReceipt          io.Reader

	Platform          string
	Source            SourceClaims
	State             PrivateState
	Ruleset           RulesetBinding
	PlanProfile       PlanProfile
	InventoryComplete bool
	IssuedAt          time.Time
	NotBefore         time.Time
	ExpiresAt         time.Time
	Now               time.Time
	HMACKey           []byte
	Nonce             []byte
}

// ExpectedAuthorization contains current public bindings for verification.
type ExpectedAuthorization struct {
	SubjectSHA256 string
	Source        SourceClaims
	Toolchain     ToolchainClaims
	State         StateClaims
	Nonce         string
	Now           time.Time
}

// ReceiptRequest contains private post-application evidence for receipt issuance.
type ReceiptRequest struct {
	Authorization     io.Reader
	Inventory         io.Reader
	Backend           io.Reader
	StateSnapshot     io.Reader
	MigrationManifest io.Reader
	ProviderSnapshot  io.Reader

	State        PrivateState
	Verification VerificationClaims
	ObservedAt   time.Time
	HMACKey      []byte
	Nonce        []byte
}

// ExpectedReceipt contains current public receipt bindings for verification.
type ExpectedReceipt struct {
	AuthorizationClaimsSHA256 string
	AppliedPlanSHA256         string
	Source                    SourceClaims
	Toolchain                 ToolchainClaims
	State                     StateClaims
	Nonce                     string
}
