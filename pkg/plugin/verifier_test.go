package plugin

import (
	"context"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// verifierFakePlugin is the VerifierProvider counterpart to
// hookFakePlugin and checkFakePlugin in registry_test.go. It satisfies
// the embedded Plugin via *fakePlugin and adds Verifiers() so the
// type assertion in Registry.Register sees a VerifierProvider.
type verifierFakePlugin struct {
	*fakePlugin
	verifiers []audit.Verifier
}

func (v *verifierFakePlugin) Verifiers() []audit.Verifier { return v.verifiers }

// stubVerifier is the smallest Verifier the plugin tests can use. We
// keep it local to this test file so the production package does not
// export a test helper.
type stubVerifier struct{ name string }

func (s stubVerifier) Name() string      { return s.name }
func (stubVerifier) Description() string { return "" }
func (stubVerifier) ReviewDepth() string { return audit.ReviewDepthAdversarial }
func (stubVerifier) Verify(context.Context, audit.VerifyTarget) ([]audit.Finding, error) {
	return nil, nil
}

func TestRegistryRegisterDeclaresVerifiersWithoutImplementing(t *testing.T) {
	r := NewRegistry()
	p := &fakePlugin{manifest: makeManifest("bad", CapabilityVerifiers)}
	err := r.Register(p)
	if err == nil || !strings.Contains(err.Error(), "VerifierProvider") {
		t.Fatalf("expected VerifierProvider mismatch, got %v", err)
	}
}

func TestRegistryRegisterImplementsVerifiersWithoutDeclaring(t *testing.T) {
	r := NewRegistry()
	p := &verifierFakePlugin{
		fakePlugin: &fakePlugin{manifest: makeManifest("bad")}, // no capability
		verifiers:  []audit.Verifier{stubVerifier{name: "v"}},
	}
	err := r.Register(p)
	if err == nil || !strings.Contains(err.Error(), "VerifierProvider") {
		t.Fatalf("expected VerifierProvider mismatch, got %v", err)
	}
}

func TestRegistryApplyVerifiersHappyPath(t *testing.T) {
	r := NewRegistry()
	p := &verifierFakePlugin{
		fakePlugin: &fakePlugin{manifest: makeManifest("verifier-host", CapabilityVerifiers)},
		verifiers:  []audit.Verifier{stubVerifier{name: "v1"}, stubVerifier{name: "v2"}},
	}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	target := audit.NewRegistry()
	if err := r.ApplyVerifiers(target); err != nil {
		t.Fatalf("ApplyVerifiers: %v", err)
	}
	got := target.Verifiers()
	if len(got) != 2 {
		t.Fatalf("target Verifiers = %d, want 2", len(got))
	}
}

func TestRegistryApplyVerifiersNilTarget(t *testing.T) {
	r := NewRegistry()
	if err := r.ApplyVerifiers(nil); err == nil {
		t.Fatal("expected nil-target error")
	}
}

func TestRegistryApplyVerifiersDuplicateName(t *testing.T) {
	r := NewRegistry()
	p1 := &verifierFakePlugin{
		fakePlugin: &fakePlugin{manifest: makeManifest("p1", CapabilityVerifiers)},
		verifiers:  []audit.Verifier{stubVerifier{name: "dup"}},
	}
	p2 := &verifierFakePlugin{
		fakePlugin: &fakePlugin{manifest: makeManifest("p2", CapabilityVerifiers)},
		verifiers:  []audit.Verifier{stubVerifier{name: "dup"}},
	}
	if err := r.Register(p1); err != nil {
		t.Fatalf("register p1: %v", err)
	}
	if err := r.Register(p2); err != nil {
		t.Fatalf("register p2: %v", err)
	}
	target := audit.NewRegistry()
	err := r.ApplyVerifiers(target)
	if err == nil || !strings.Contains(err.Error(), "p2") {
		t.Fatalf("expected p2 collision error, got %v", err)
	}
}

func TestCapabilityVerifiersIsValid(t *testing.T) {
	if !CapabilityVerifiers.IsValid() {
		t.Error("CapabilityVerifiers.IsValid() = false")
	}
}
