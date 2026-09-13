//go:build darwin && arm64

package buildauthority

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDarwinSourceAdministrativeInventoryRetainsOnlyValueRows(t *testing.T) {
	for _, packedRefsPresent := range []bool{false, true} {
		name := "absent"
		if packedRefsPresent {
			name = "present"
		}
		t.Run(name, func(t *testing.T) {
			repository, _ := writeDarwinSourceConstructionRepository(t)
			makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git", "refs", "heads"))
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(repository, ".git", "HEAD"),
				[]byte("ref: refs/heads/main\n"),
			)
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(repository, ".git", "refs", "heads", "main"),
				[]byte(strings.Repeat("1", 40)+"\n"),
			)
			if packedRefsPresent {
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(repository, ".git", "packed-refs"),
					[]byte(strings.Repeat("1", 40)+" refs/heads/main\n"),
				)
			}

			primitives := requireDarwinSourcePrimitives(t)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			packed := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			if !packed.proved() || !owner.validPackedRefsRetention() {
				t.Fatalf("packed refs stage = %+v; owner %+v", packed, owner)
			}
			inventory := runSourceAdministrativeInventoryStage(
				context.Background(),
				owner,
				primitives,
			)
			if !inventory.proved() || !owner.validAdministrativeRetention() {
				t.Fatalf("administrative inventory = %+v; owner %+v", inventory, owner)
			}
			paths := make([]string, len(owner.git.administration.rows))
			for index, row := range owner.git.administration.rows {
				paths[index] = row.path
				if row.kind != sourceObservedDirectory && row.kind != sourceObservedRegular {
					t.Fatalf("retained non-value row %+v", row)
				}
			}
			want := []string{
				".git",
				".git/HEAD",
				".git/config",
				".git/objects",
			}
			if packedRefsPresent {
				want = append(want, ".git/packed-refs")
			}
			want = append(want, ".git/refs", ".git/refs/heads", ".git/refs/heads/main")
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("administrative paths = %q, want %q", paths, want)
			}
			if owner.state != sourceConstructionActive {
				t.Fatalf("inventory prematurely changed owner state: %+v", owner)
			}

			var closeOutcome sourceUseOutcome
			owner.closeIntoWith(primitives, &closeOutcome)
			if !closeOutcome.proved() {
				t.Fatalf("inventory owner close = %+v", closeOutcome)
			}
		})
	}
}

func TestDarwinSourceAdministrativeInventoryConsumesMultipleDirectoryBatches(t *testing.T) {
	repository, _ := writeDarwinSourceConstructionRepository(t)
	const inertFiles = sourceDirectoryReadBatchSize + 1
	for index := range inertFiles {
		writeDarwinSourceConstructionFile(
			t,
			filepath.Join(repository, ".git", fmt.Sprintf("batch-%03d", index)),
			nil,
		)
	}

	primitives := requireDarwinSourcePrimitives(t)
	owner, initial := retainSourceConstructionWith(
		context.Background(),
		sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
		primitives,
	)
	if owner == nil || !initial.proved() {
		t.Fatalf("initial source construction = %+v / %+v", owner, initial)
	}
	packed := runSourceConstructionPackedRefsStage(context.Background(), owner, primitives)
	if !packed.proved() {
		t.Fatalf("packed refs stage = %+v", packed)
	}
	inventory := runSourceAdministrativeInventoryStage(context.Background(), owner, primitives)
	if !inventory.proved() || !owner.validAdministrativeRetention() {
		t.Fatalf("multi-batch administrative inventory = %+v; owner %+v", inventory, owner)
	}
	if got, want := len(owner.git.administration.rows), inertFiles+3; got != want {
		t.Fatalf("multi-batch administrative rows = %d, want %d", got, want)
	}

	var closeOutcome sourceUseOutcome
	owner.closeIntoWith(primitives, &closeOutcome)
	if !closeOutcome.proved() {
		t.Fatalf("multi-batch inventory owner close = %+v", closeOutcome)
	}
}

func TestDarwinSourceAdministrativeInventoryRejectsForbiddenTopology(t *testing.T) {
	for _, test := range []struct {
		name    string
		arrange func(*testing.T, string)
	}{
		{
			name: "symlink",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git", "hooks"))
				if err := os.Symlink(
					"../config",
					filepath.Join(repository, ".git", "hooks", "config-link"),
				); err != nil {
					t.Fatalf("make forbidden source symlink: %v", err)
				}
			},
		},
		{
			name: "unknown object subtree",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(
					t,
					filepath.Join(repository, ".git", "objects", "unknown", "nested"),
				)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(repository, ".git", "objects", "unknown", "nested", "row"),
					nil,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, _ := writeDarwinSourceConstructionRepository(t)
			test.arrange(t, repository)
			primitives := requireDarwinSourcePrimitives(t)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			packed := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			if !packed.proved() {
				t.Fatalf("packed refs stage = %+v", packed)
			}
			inventory := runSourceAdministrativeInventoryStage(
				context.Background(),
				owner,
				primitives,
			)
			requireFailureRecord(
				t,
				inventory.primary,
				PhaseSource,
				OperationValidate,
				CauseUnsupported,
			)
			if inventory.descriptorClose != nil || owner.state != sourceConstructionClosed ||
				owner.git.administration != nil {
				t.Fatalf("forbidden inventory outcome = %+v; owner %+v", inventory, owner)
			}
		})
	}
}

func runSourceAdministrativeInventoryStage(
	ctx context.Context,
	owner *sourceConstructionOwner,
	primitives sourcePrimitives,
) sourceUseOutcome {
	builder := &sourceConstructionBuilder{
		ctx:        ctx,
		primitives: primitives,
		owner:      owner,
	}
	retained := builder.retainSourceAdministrativeInventory()
	if retained && builder.outcome.proved() {
		return builder.outcome
	}
	if retained || builder.outcome.proved() {
		builder.outcome.addPrimitive(newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		))
	}
	if !builder.outcome.proved() {
		builder.owner.closeIntoWith(primitives, &builder.outcome)
	}
	return builder.outcome
}
