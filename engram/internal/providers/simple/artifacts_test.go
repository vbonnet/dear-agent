package simple

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

func TestSimpleFileProvider_ConcurrentArtifactOperations(t *testing.T) {
	provider := &SimpleFileProvider{storagePath: t.TempDir()}
	ctx := context.Background()

	const workers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := range workers {
		wg.Go(func() {
			artifactID := fmt.Sprintf("artifact-%02d", i)
			data := []byte(artifactID)
			if err := provider.StoreArtifact(ctx, artifactID, data); err != nil {
				errCh <- fmt.Errorf("store %s: %w", artifactID, err)
				return
			}
			got, err := provider.GetArtifact(ctx, artifactID)
			if err != nil {
				errCh <- fmt.Errorf("get %s: %w", artifactID, err)
				return
			}
			if string(got) != artifactID {
				errCh <- fmt.Errorf("get %s = %q", artifactID, got)
				return
			}
			if err := provider.DeleteArtifact(ctx, artifactID); err != nil {
				errCh <- fmt.Errorf("delete %s: %w", artifactID, err)
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}
