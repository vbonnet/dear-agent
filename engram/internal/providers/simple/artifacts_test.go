package simple

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

func TestSimpleFileProvider_RejectsArtifactIDTraversal(t *testing.T) {
	provider := &SimpleFileProvider{storagePath: t.TempDir()}
	ctx := context.Background()

	for _, artifactID := range []string{"../escape", "nested/file", `nested\file`, "..hidden"} {
		t.Run(artifactID, func(t *testing.T) {
			if err := provider.StoreArtifact(ctx, artifactID, []byte("data")); !errors.Is(err, consolidation.ErrInvalidNamespace) {
				t.Fatalf("StoreArtifact(%q) error = %v, want ErrInvalidNamespace", artifactID, err)
			}
			if _, err := provider.GetArtifact(ctx, artifactID); !errors.Is(err, consolidation.ErrInvalidNamespace) {
				t.Fatalf("GetArtifact(%q) error = %v, want ErrInvalidNamespace", artifactID, err)
			}
			if err := provider.DeleteArtifact(ctx, artifactID); !errors.Is(err, consolidation.ErrInvalidNamespace) {
				t.Fatalf("DeleteArtifact(%q) error = %v, want ErrInvalidNamespace", artifactID, err)
			}
		})
	}
}
