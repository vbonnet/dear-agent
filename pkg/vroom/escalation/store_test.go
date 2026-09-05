package escalation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fileStoreSubprocessMode = "DEAR_AGENT_FILESTORE_SUBPROCESS"

func testStores(t *testing.T) map[string]Store {
	t.Helper()
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return map[string]Store{"mem": NewMemStore(), "file": fs}
}

func testEscalation(id string) *Escalation {
	return &Escalation{
		ID: id, Kind: KindQuestion, Mode: ModeAsync, Question: "question",
		OriginSessionID: "worker", CurrentSessionID: VROOMTrioSessionID,
		Chain: []string{"worker", VROOMTrioSessionID}, Phase: PhaseConferring,
		Confer: &Confer{
			Members:   []string{"a", "b", "c"},
			Quorum:    3,
			Ballots:   []Ballot{{SessionID: "a", Vote: VoteApprove, Answer: "first"}},
			StartedAt: time.Unix(1, 0),
		},
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}
}

func TestStore_CreateGetListRoundTrip(t *testing.T) {
	ctx := context.Background()
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			pending := &Escalation{
				ID: "esc-1", Kind: KindQuestion, Phase: PhaseRouted,
				OriginSessionID: "w1", CurrentSessionID: "sup",
				Chain: []string{"w1", "sup"}, CreatedAt: time.Unix(1, 0),
			}
			resolved := &Escalation{
				ID: "esc-2", Kind: KindDecision, Phase: PhaseAnswered,
				OriginSessionID: "w2", CurrentSessionID: "sup", Answer: "ok",
				Chain: []string{"w2", "sup"}, CreatedAt: time.Unix(2, 0),
			}
			if err := s.Create(ctx, pending); err != nil {
				t.Fatalf("Create pending: %v", err)
			}
			if err := s.Create(ctx, resolved); err != nil {
				t.Fatalf("Create resolved: %v", err)
			}

			got, err := s.Get(ctx, "esc-1")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Question != pending.Question || got.Phase != PhaseRouted {
				t.Errorf("round-trip mismatch: %+v", got)
			}

			if _, err := s.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
				t.Errorf("want ErrNotFound, got %v", err)
			}

			all, err := s.List(ctx, Filter{})
			if err != nil || len(all) != 2 {
				t.Fatalf("List all: n=%d err=%v", len(all), err)
			}
			if all[0].ID != "esc-1" || all[1].ID != "esc-2" {
				t.Errorf("List order wrong: %s, %s", all[0].ID, all[1].ID)
			}

			pendingOnly, err := s.List(ctx, Filter{Pending: true})
			if err != nil || len(pendingOnly) != 1 || pendingOnly[0].ID != "esc-1" {
				t.Errorf("pending filter wrong: %v / %v", pendingOnly, err)
			}

			byHolder, err := s.List(ctx, Filter{CurrentSessionID: "sup", Pending: true})
			if err != nil || len(byHolder) != 1 {
				t.Errorf("holder filter wrong: n=%d err=%v", len(byHolder), err)
			}
		})
	}
}

func TestStore_CreateDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			original := testEscalation("same")
			if err := s.Create(ctx, original); err != nil {
				t.Fatal(err)
			}
			replacement := testEscalation("same")
			replacement.Question = "replacement"
			if err := s.Create(ctx, replacement); !errors.Is(err, ErrAlreadyExists) {
				t.Fatalf("Create duplicate: want ErrAlreadyExists, got %v", err)
			}
			got, err := s.Get(ctx, "same")
			if err != nil {
				t.Fatal(err)
			}
			if got.Question != original.Question {
				t.Fatalf("duplicate Create replaced record: question=%q", got.Question)
			}
		})
	}
}

func TestStore_UpdateContract(t *testing.T) {
	ctx := context.Background()
	abort := errors.New("abort mutation")
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			if err := s.Create(ctx, testEscalation("update")); err != nil {
				t.Fatal(err)
			}

			calls := 0
			var retained *Escalation
			updated, err := s.Update(ctx, "update", func(current *Escalation) error {
				calls++
				retained = current
				current.Chain = append(current.Chain, "b")
				current.Confer.Members[0] = "member-updated"
				current.Confer.Ballots = append(current.Confer.Ballots, Ballot{SessionID: "b", Vote: VoteReject})
				return nil
			})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if calls != 1 {
				t.Fatalf("mutation called %d times, want once", calls)
			}

			retained.Chain[0] = "retained-alias"
			retained.Confer.Members[0] = "retained-member-alias"
			retained.Confer.Ballots[0].Answer = "retained-ballot-alias"
			updated.Chain[0] = "returned-alias"
			updated.Confer.Members[0] = "returned-member-alias"
			updated.Confer.Ballots[0].Answer = "returned-ballot-alias"

			beforeAbort, err := s.Get(ctx, "update")
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Update(ctx, "update", func(current *Escalation) error {
				current.Chain[0] = "rolled-back"
				current.Confer.Members[0] = "rolled-back"
				current.Confer.Ballots[0].Answer = "rolled-back"
				return abort
			})
			if !errors.Is(err, abort) {
				t.Fatalf("Update rollback: want callback error, got %v", err)
			}
			afterAbort, err := s.Get(ctx, "update")
			if err != nil {
				t.Fatal(err)
			}
			assertEscalationNestedEqual(t, afterAbort, beforeAbort)

			_, err = s.Update(ctx, "update", func(current *Escalation) error {
				current.ID = "changed"
				current.Chain[0] = "must-not-commit"
				return nil
			})
			if err == nil {
				t.Fatal("Update ID mutation succeeded, want error")
			}
			afterIDChange, err := s.Get(ctx, "update")
			if err != nil {
				t.Fatal(err)
			}
			assertEscalationNestedEqual(t, afterIDChange, beforeAbort)

			if _, err := s.Update(ctx, "missing", func(*Escalation) error { return nil }); !errors.Is(err, ErrNotFound) {
				t.Fatalf("Update missing: want ErrNotFound, got %v", err)
			}
		})
	}
}

func TestStore_DeepCopyIsolation(t *testing.T) {
	ctx := context.Background()
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			input := testEscalation("copies")
			if err := s.Create(ctx, input); err != nil {
				t.Fatal(err)
			}
			input.Chain[0] = "input-alias"
			input.Confer.Members[0] = "input-member-alias"
			input.Confer.Ballots[0].Answer = "input-ballot-alias"

			first, err := s.Get(ctx, "copies")
			if err != nil {
				t.Fatal(err)
			}
			first.Chain[0] = "get-alias"
			first.Confer.Members[0] = "get-member-alias"
			first.Confer.Ballots[0].Answer = "get-ballot-alias"

			listed, err := s.List(ctx, Filter{})
			if err != nil || len(listed) != 1 {
				t.Fatalf("List: n=%d err=%v", len(listed), err)
			}
			listed[0].Chain[0] = "list-alias"
			listed[0].Confer.Members[0] = "list-member-alias"
			listed[0].Confer.Ballots[0].Answer = "list-ballot-alias"

			got, err := s.Get(ctx, "copies")
			if err != nil {
				t.Fatal(err)
			}
			if got.Chain[0] != "worker" || got.Confer.Members[0] != "a" || got.Confer.Ballots[0].Answer != "first" {
				t.Fatalf("stored nested state was aliased: %+v", got)
			}
		})
	}
}

func TestStore_RejectsBadID(t *testing.T) {
	ctx := context.Background()
	badIDs := []string{
		"", ".", "..", "../escape", "a/b", `a\b`, "nul\x00id",
		"alternate:stream", "space id", "unicode-é", "MixedCase", "con", "nul.txt", "com1", "lpt9.log",
	}
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			for _, id := range badIDs {
				if err := s.Create(ctx, &Escalation{ID: id}); err == nil {
					t.Errorf("Create accepted invalid id %q", id)
				}
				if _, err := s.Get(ctx, id); err == nil {
					t.Errorf("Get accepted invalid id %q", id)
				}
				if _, err := s.Update(ctx, id, func(*Escalation) error { return nil }); err == nil {
					t.Errorf("Update accepted invalid id %q", id)
				}
			}
			if err := s.Create(ctx, nil); err == nil {
				t.Error("Create accepted nil escalation")
			}
		})
	}
}

func TestStore_CanceledContextDoesNotMutateOrReturnPartialList(t *testing.T) {
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			background := context.Background()
			if err := s.Create(background, testEscalation("existing")); err != nil {
				t.Fatalf("seed: %v", err)
			}
			ctx, cancel := context.WithCancel(background)
			cancel()

			called := false
			if _, err := s.Update(ctx, "existing", func(*Escalation) error {
				called = true
				return nil
			}); !errors.Is(err, context.Canceled) {
				t.Fatalf("Update: want context.Canceled, got %v", err)
			}
			if called {
				t.Fatal("Update invoked mutation after cancellation")
			}
			if got, err := s.List(ctx, Filter{}); !errors.Is(err, context.Canceled) || got != nil {
				t.Fatalf("List: got=%v err=%v, want nil/context.Canceled", got, err)
			}
		})
	}
}

func TestFileStore_Persists(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	fs1, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs1.Create(ctx, &Escalation{ID: "p1", Phase: PhaseRouted, CreatedAt: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs2.Get(ctx, "p1"); err != nil {
		t.Errorf("fresh FileStore did not see persisted escalation: %v", err)
	}
}

func TestFileStore_IndependentStoresSerializeUpdates(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	fs1, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs1.Create(ctx, &Escalation{ID: "shared", Chain: []string{"origin"}}); err != nil {
		t.Fatal(err)
	}

	const updates = 32
	start := make(chan struct{})
	errs := make(chan error, updates)
	var wg sync.WaitGroup
	var callbacksInFlight atomic.Int32
	var callbacksOverlapped atomic.Bool
	for i := range updates {
		store := fs1
		if i%2 == 1 {
			store = fs2
		}
		wg.Add(1)
		go func(store *FileStore) {
			defer wg.Done()
			<-start
			_, err := store.Update(ctx, "shared", func(current *Escalation) error {
				if callbacksInFlight.Add(1) != 1 {
					callbacksOverlapped.Store(true)
				}
				defer callbacksInFlight.Add(-1)
				for range 8 {
					runtime.Gosched()
				}
				current.Chain = append(current.Chain, "update")
				return nil
			})
			errs <- err
		}(store)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Update: %v", err)
		}
	}
	if callbacksOverlapped.Load() {
		t.Fatal("independent FileStore mutation callbacks overlapped")
	}
	got, err := fs1.Get(ctx, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Chain) != updates+1 {
		t.Fatalf("lost updates: chain length=%d, want %d", len(got.Chain), updates+1)
	}
}

func TestFileStore_SeparateProcessesSerializeUpdates(t *testing.T) {
	if worker := os.Getenv(fileStoreSubprocessMode); worker != "" {
		runFileStoreSubprocessWorker(t, worker)
		return
	}
	switch runtime.GOOS {
	case "darwin", "linux", "freebsd", "windows":
	default:
		t.Skipf("cross-process FileStore locking unsupported on %s", runtime.GOOS)
	}

	dir := t.TempDir()
	ctx := context.Background()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, &Escalation{ID: "process-shared", Chain: []string{"origin"}}); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	const (
		workers          = 4
		updatesPerWorker = 8
	)
	startPath := filepath.Join(dir, "start")
	type child struct {
		cmd *exec.Cmd
		out bytes.Buffer
	}
	children := make([]child, workers)
	for i := range workers {
		worker := fmt.Sprintf("worker-%d", i)
		cmd := exec.Command(executable, "-test.run=^TestFileStore_SeparateProcessesSerializeUpdates$", "-test.v")
		cmd.Env = append(os.Environ(),
			fileStoreSubprocessMode+"="+worker,
			"DEAR_AGENT_FILESTORE_DIR="+dir,
			"DEAR_AGENT_FILESTORE_START="+startPath,
			fmt.Sprintf("DEAR_AGENT_FILESTORE_UPDATES=%d", updatesPerWorker),
		)
		children[i].cmd = cmd
		cmd.Stdout = &children[i].out
		cmd.Stderr = &children[i].out
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s: %v", worker, err)
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for i := range workers {
		readyPath := filepath.Join(dir, fmt.Sprintf("worker-%d.ready", i))
		for {
			if _, err := os.Stat(readyPath); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", readyPath)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err := os.WriteFile(startPath, []byte("start"), 0o600); err != nil {
		t.Fatalf("release subprocesses: %v", err)
	}
	for i := range children {
		if err := children[i].cmd.Wait(); err != nil {
			t.Fatalf("worker %d: %v\n%s", i, err, children[i].out.String())
		}
	}

	got, err := store.Get(ctx, "process-shared")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Chain) != 1+workers*updatesPerWorker {
		t.Fatalf("chain length=%d, want %d: %v", len(got.Chain), 1+workers*updatesPerWorker, got.Chain)
	}
	counts := make(map[string]int, workers)
	for _, entry := range got.Chain[1:] {
		counts[entry]++
	}
	for i := range workers {
		worker := fmt.Sprintf("worker-%d", i)
		if counts[worker] != updatesPerWorker {
			t.Fatalf("updates from %s=%d, want %d", worker, counts[worker], updatesPerWorker)
		}
	}
}

func runFileStoreSubprocessWorker(t *testing.T, worker string) {
	t.Helper()
	dir := os.Getenv("DEAR_AGENT_FILESTORE_DIR")
	startPath := os.Getenv("DEAR_AGENT_FILESTORE_START")
	var updates int
	if _, err := fmt.Sscan(os.Getenv("DEAR_AGENT_FILESTORE_UPDATES"), &updates); err != nil || updates <= 0 {
		t.Fatalf("invalid worker update count: %v", err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	readyPath := filepath.Join(dir, worker+".ready")
	if err := os.WriteFile(readyPath, []byte("ready"), 0o600); err != nil {
		t.Fatalf("write ready marker: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(startPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for start marker")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for range updates {
		if _, err := store.Update(context.Background(), "process-shared", func(current *Escalation) error {
			time.Sleep(time.Millisecond)
			current.Chain = append(current.Chain, worker)
			return nil
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
}

func TestWaitForStoreFileLock_CancelsAfterContention(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempted := make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once
	go func() {
		done <- waitForStoreFileLock(ctx, func() (bool, error) {
			once.Do(func() { close(attempted) })
			return false, nil
		})
	}()
	select {
	case <-attempted:
	case <-time.After(time.Second):
		t.Fatal("lock wait did not attempt acquisition")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait error=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lock wait did not stop after cancellation")
	}
}

func TestFileStore_LockWaitHonorsContextCancellation(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	fs1, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs1.Create(ctx, &Escalation{ID: "locked"}); err != nil {
		t.Fatal(err)
	}

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := fs1.Update(ctx, "locked", func(current *Escalation) error {
			close(firstEntered)
			<-releaseFirst
			current.Chain = append(current.Chain, "first")
			return nil
		})
		firstDone <- err
	}()
	<-firstEntered

	waitCtx, cancel := context.WithCancel(ctx)
	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		_, err := fs2.Update(waitCtx, "locked", func(current *Escalation) error {
			current.Chain = append(current.Chain, "must-not-run")
			return nil
		})
		secondDone <- err
	}()
	<-secondStarted
	cancel()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		close(releaseFirst)
		<-firstDone
		t.Fatalf("blocked Update: want context.Canceled, got %v", err)
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Update: %v", err)
	}
	got, err := fs1.Get(ctx, "locked")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Chain) != 1 || got.Chain[0] != "first" {
		t.Fatalf("canceled callback mutated state: chain=%v", got.Chain)
	}
}

func TestFileStore_IndependentStoresCreateOnce(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	fs1, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for i, store := range []*FileStore{fs1, fs2} {
		go func(i int, store *FileStore) {
			<-start
			errs <- store.Create(ctx, &Escalation{ID: "create-once", Question: string(rune('a' + i))})
		}(i, store)
	}
	close(start)
	err1, err2 := <-errs, <-errs
	if (err1 == nil) == (err2 == nil) {
		t.Fatalf("Create results = (%v, %v), want one success", err1, err2)
	}
	loser := err1
	if loser == nil {
		loser = err2
	}
	if !errors.Is(loser, ErrAlreadyExists) {
		t.Fatalf("Create loser: want ErrAlreadyExists, got %v", loser)
	}
}

func assertEscalationNestedEqual(t *testing.T, got, want *Escalation) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("escalation changed:\n got=%+v\nwant=%+v", got, want)
	}
}
