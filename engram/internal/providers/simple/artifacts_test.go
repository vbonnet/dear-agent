package simple

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

func TestSimpleFileProvider_ArtifactPathContainment(t *testing.T) {
	t.Parallel()

	storagePath := t.TempDir()
	provider := &SimpleFileProvider{storagePath: storagePath}
	artifactRoot := filepath.Join(storagePath, "_artifacts")

	for _, artifactID := range []string{"artifact-01", "artifact_02", "artifact.03"} {
		t.Run(artifactID, func(t *testing.T) {
			got, err := provider.getArtifactPath(artifactID)
			if err != nil {
				t.Fatalf("getArtifactPath(%q) error = %v", artifactID, err)
			}
			if !strings.HasPrefix(got, artifactRoot+string(filepath.Separator)) {
				t.Fatalf("getArtifactPath(%q) = %q, want beneath %q", artifactID, got, artifactRoot)
			}
		})
	}
}

func FuzzSimpleFileProvider_ArtifactPathContainment(f *testing.F) {
	for _, artifactID := range []string{
		"artifact-01",
		"artifact_02",
		"artifact.03",
		"",
		".",
		"..",
		"../escape",
		"path/file",
		`path\file`,
		"control\x00bad",
	} {
		f.Add(artifactID)
	}

	f.Fuzz(func(t *testing.T, artifactID string) {
		storagePath := t.TempDir()
		provider := &SimpleFileProvider{storagePath: storagePath}
		artifactRoot := filepath.Clean(filepath.Join(storagePath, "_artifacts"))

		got, err := provider.getArtifactPath(artifactID)
		if err != nil {
			return
		}
		if got == artifactRoot || !strings.HasPrefix(got, artifactRoot+string(filepath.Separator)) {
			t.Fatalf("getArtifactPath(%q) = %q, want beneath %q", artifactID, got, artifactRoot)
		}
	})
}
